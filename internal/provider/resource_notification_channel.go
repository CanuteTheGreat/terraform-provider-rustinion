package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &notificationChannelResource{}
	_ resource.ResourceWithConfigure   = &notificationChannelResource{}
	_ resource.ResourceWithImportState = &notificationChannelResource{}
)

func NewNotificationChannelResource() resource.Resource {
	return &notificationChannelResource{}
}

type notificationChannelResource struct {
	client *client.Client
}

// notificationChannelResourceModel — unlike alert rules/dashboards/
// synthetic checks, a notification channel is tenant-wide, not scoped
// to one group (see notifications.rs's own doc comment: it's routed to
// a tenant/group/rule separately via `notification_routes`, which this
// provider does not manage yet — see the README's own limitations
// section).
type notificationChannelResourceModel struct {
	ID                    types.String         `tfsdk:"id"`
	Name                  types.String         `tfsdk:"name"`
	ChannelType           types.String         `tfsdk:"channel_type"`
	Config                jsontypes.Normalized `tfsdk:"config"`
	Enabled               types.Bool           `tfsdk:"enabled"`
	GroupingEnabled       types.Bool           `tfsdk:"grouping_enabled"`
	GroupingDimension     types.String         `tfsdk:"grouping_dimension"`
	GroupingWindowMinutes types.Int64          `tfsdk:"grouping_window_minutes"`
	DigestIntervalMinutes types.Int64          `tfsdk:"digest_interval_minutes"`
	SubjectTemplate       types.String         `tfsdk:"subject_template"`
	BodyTemplate          types.String         `tfsdk:"body_template"`
	PayloadTemplate       types.String         `tfsdk:"payload_template"`
	Severities            types.Set            `tfsdk:"severities"`
}

func (r *notificationChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_channel"
}

func (r *notificationChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion notification channel (Slack, generic webhook, email, PagerDuty, Opsgenie, or Twilio SMS/voice call). See notifications.rs in the main repo for exactly how each channel_type is dispatched.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"channel_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of: email, slack, webhook, pagerduty, opsgenie, sms, voice, ntfy.",
				Validators:          []validator.String{stringvalidator.OneOf("email", "slack", "webhook", "pagerduty", "opsgenie", "sms", "voice", "ntfy")},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"config": schema.StringAttribute{
				Required:   true,
				CustomType: jsontypes.NormalizedType{},
				Sensitive:  true,
				MarkdownDescription: "The channel's config as a JSON string, shape depending on channel_type: " +
					"slack: {\"webhook_url\": \"...\"}; " +
					"webhook: {\"url\": \"...\", \"secret\": \"...\"} (secret optional); " +
					"email: {\"recipients\": [\"...\"]}; " +
					"pagerduty: {\"routing_key\": \"...\"}; " +
					"opsgenie: {\"api_key\": \"...\", \"region\": \"us\"|\"eu\"} (region optional, defaults to us); " +
					"sms/voice: {\"account_sid\": \"...\", \"auth_token\": \"...\", \"from_number\": \"+1...\", \"to_numbers\": [\"+1...\"]} " +
					"(a real Twilio account — sms sends a text via Twilio's Messages API, voice places a call and reads the alert aloud via Twilio's Calls API). " +
					"Marked sensitive since every shape but email holds a secret.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"grouping_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Opt in to alert grouping/deduplication for this channel. Defaults to false.",
			},
			"grouping_dimension": schema.StringAttribute{
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString("rule"),
				Validators: []validator.String{stringvalidator.OneOf("rule", "device", "group")},
			},
			"grouping_window_minutes": schema.Int64Attribute{
				Optional:   true,
				Computed:   true,
				Default:    int64default.StaticInt64(15),
				Validators: []validator.Int64{int64validator.AtLeast(1)},
			},
			"digest_interval_minutes": schema.Int64Attribute{
				Optional:   true,
				Computed:   true,
				Default:    int64default.StaticInt64(15),
				Validators: []validator.Int64{int64validator.AtLeast(1)},
			},
			"subject_template": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Custom alert subject, with `{{variable}}` substitution. Omit for the built-in format. Available variables: `reason`, `hostname`, `group`, `detail`, `severity`, `device_id`, `group_id`, `package`. An unknown variable renders as empty.",
			},
			"body_template": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Custom alert body, same `{{variable}}` substitution as `subject_template`. Omit for the built-in format.",
			},
			"payload_template": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Webhook channels only: the entire JSON request body, replacing the default `{\"subject\",\"body\",\"severity\"}` envelope. Substituted values are JSON-escaped automatically, so a detail containing quotes stays valid. Rejected at apply time if it doesn't parse as JSON once variables are filled in.",
			},
			"severities": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Finding severities this channel gets: any of `critical`, `warning`, `info`. Omit for the channel type's default: pagerduty, opsgenie, sms and voice page a person, so they get `critical` only; email, slack and webhook get every severity. An empty set sends nothing.",
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.OneOf("critical", "warning", "info")),
				},
			},
		},
	}
}

func (r *notificationChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	r.client = c
}

func notificationChannelToClient(m notificationChannelResourceModel) client.NotificationChannel {
	return client.NotificationChannel{
		Name:                  m.Name.ValueString(),
		ChannelType:           m.ChannelType.ValueString(),
		Config:                json.RawMessage(m.Config.ValueString()),
		Enabled:               m.Enabled.ValueBool(),
		GroupingEnabled:       m.GroupingEnabled.ValueBool(),
		GroupingDimension:     m.GroupingDimension.ValueString(),
		GroupingWindowMinutes: m.GroupingWindowMinutes.ValueInt64(),
		DigestIntervalMinutes: m.DigestIntervalMinutes.ValueInt64(),
		SubjectTemplate:       optionalString(m.SubjectTemplate),
		BodyTemplate:          optionalString(m.BodyTemplate),
		PayloadTemplate:       optionalString(m.PayloadTemplate),
		Severities:            severitiesToClient(m.Severities),
	}
}

// An unset set means "the channel type's default" (JSON null).
func severitiesToClient(v types.Set) []string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	out := []string{}
	for _, e := range v.Elements() {
		if s, ok := e.(types.String); ok {
			out = append(out, s.ValueString())
		}
	}
	return out
}

func severitiesFromClient(v []string) types.Set {
	if v == nil {
		return types.SetNull(types.StringType)
	}
	elems := make([]attr.Value, 0, len(v))
	for _, s := range v {
		elems = append(elems, types.StringValue(s))
	}
	return types.SetValueMust(types.StringType, elems)
}

// An unset Terraform attribute becomes a JSON null, which is how the
// server spells "no template, use the built-in format" — distinct from
// an empty template.
func optionalString(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func stringOrNull(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

func notificationChannelFromClient(ch *client.NotificationChannel) notificationChannelResourceModel {
	return notificationChannelResourceModel{
		ID:                    types.StringValue(ch.ID),
		Name:                  types.StringValue(ch.Name),
		ChannelType:           types.StringValue(ch.ChannelType),
		Config:                jsontypes.NewNormalizedValue(string(ch.Config)),
		Enabled:               types.BoolValue(ch.Enabled),
		GroupingEnabled:       types.BoolValue(ch.GroupingEnabled),
		GroupingDimension:     types.StringValue(ch.GroupingDimension),
		GroupingWindowMinutes: types.Int64Value(ch.GroupingWindowMinutes),
		DigestIntervalMinutes: types.Int64Value(ch.DigestIntervalMinutes),
		SubjectTemplate:       stringOrNull(ch.SubjectTemplate),
		BodyTemplate:          stringOrNull(ch.BodyTemplate),
		PayloadTemplate:       stringOrNull(ch.PayloadTemplate),
		Severities:            severitiesFromClient(ch.Severities),
	}
}

func (r *notificationChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan notificationChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateNotificationChannel(ctx, notificationChannelToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating notification channel", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, notificationChannelFromClient(created))...)
}

func (r *notificationChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state notificationChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ch, err := r.client.GetNotificationChannel(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading notification channel", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, notificationChannelFromClient(ch))...)
}

func (r *notificationChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan notificationChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state notificationChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateNotificationChannel(ctx, state.ID.ValueString(), notificationChannelToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating notification channel", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, notificationChannelFromClient(updated))...)
}

func (r *notificationChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state notificationChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteNotificationChannel(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting notification channel", err.Error())
	}
}

func (r *notificationChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
