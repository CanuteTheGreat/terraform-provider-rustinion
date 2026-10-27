package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &alertRoutingResource{}
	_ resource.ResourceWithConfigure   = &alertRoutingResource{}
	_ resource.ResourceWithImportState = &alertRoutingResource{}
)

// alertRoutingID is the singleton's id: there is one routing per
// organization (the API key's tenant).
const alertRoutingID = "alert-routing"

func NewAlertRoutingResource() resource.Resource {
	return &alertRoutingResource{}
}

type alertRoutingResource struct {
	client *client.Client
}

type alertRoutingResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	NotifyEmail          types.Bool   `tfsdk:"notify_email"`
	NotifyAllAdmins      types.Bool   `tfsdk:"notify_all_admins"`
	ExtraRecipients      types.List   `tfsdk:"extra_recipients"`
	EmailSeverities      types.Set    `tfsdk:"email_severities"`
	WebhookSeverities    types.Set    `tfsdk:"webhook_severities"`
	PagerdutySeverities  types.Set    `tfsdk:"pagerduty_severities"`
	NtfyPriorityCritical types.String `tfsdk:"ntfy_priority_critical"`
	NtfyPriorityWarning  types.String `tfsdk:"ntfy_priority_warning"`
	NtfyPriorityInfo     types.String `tfsdk:"ntfy_priority_info"`
	NtfyPriorityDigest   types.String `tfsdk:"ntfy_priority_digest"`
	AlertDigestCadence   types.String `tfsdk:"alert_digest_cadence"`
	SummaryDigestCadence types.String `tfsdk:"summary_digest_cadence"`
	DigestHour           types.Int64  `tfsdk:"digest_hour"`
	DigestTimezone       types.String `tfsdk:"digest_timezone"`
}

func (r *alertRoutingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_routing"
}

func severitySet(values ...string) types.Set {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValueMust(types.StringType, elems)
}

func severitySetAttribute(desc string, def ...string) schema.SetAttribute {
	return schema.SetAttribute{
		Optional:            true,
		Computed:            true,
		ElementType:         types.StringType,
		Default:             setdefault.StaticValue(severitySet(def...)),
		MarkdownDescription: desc,
		Validators: []validator.Set{
			setvalidator.ValueStringsAre(stringvalidator.OneOf("critical", "warning", "info")),
		},
	}
}

func ntfyPriorityAttribute(desc, def string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString(def),
		MarkdownDescription: desc,
		Validators:          []validator.String{stringvalidator.OneOf("min", "low", "default", "high", "urgent")},
	}
}

func (r *alertRoutingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages where the organization's compliance findings go by severity, the ntfy priority of each push, and when digests go out. Critical findings go out at once to the destinations that take them; warnings and info wait for the digest at digest_hour in digest_timezone. Push destinations (the digest webhook, e.g. ntfy, and PagerDuty) take critical findings only by default, so a digest never pages. One per organization; destroying it puts the defaults back. The webhook URL itself is set on the tenant page.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `alert-routing`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"notify_email": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Send email at all. Default true.",
			},
			"notify_all_admins": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Email every admin of the organization (besides extra_recipients). Default true.",
			},
			"extra_recipients": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Default:             listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: "More email addresses (a distribution list works). Default none.",
			},
			"email_severities":       severitySetAttribute("Severities sent by email. Default all three.", "critical", "warning", "info"),
			"webhook_severities":     severitySetAttribute("Severities sent to the digest webhook (Slack-compatible or ntfy). Default critical only.", "critical"),
			"pagerduty_severities":   severitySetAttribute("Severities that trigger PagerDuty. Default critical only.", "critical"),
			"ntfy_priority_critical": ntfyPriorityAttribute("ntfy priority of a critical finding. Default urgent.", "urgent"),
			"ntfy_priority_warning":  ntfyPriorityAttribute("ntfy priority of a warning sent on its own. Default low (silent).", "low"),
			"ntfy_priority_info":     ntfyPriorityAttribute("ntfy priority of an info finding. Default min (silent).", "min"),
			"ntfy_priority_digest":   ntfyPriorityAttribute("ntfy priority of a scheduled digest. Default min (silent).", "min"),
			"alert_digest_cadence": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("every_sweep"),
				MarkdownDescription: "per_incident (one email per new critical/warning incident, the moment it fires), every_sweep (one digest of everything flagged), daily or weekly (Mondays). Default every_sweep.",
				Validators:          []validator.String{stringvalidator.OneOf("per_incident", "every_sweep", "daily", "weekly")},
			},
			"summary_digest_cadence": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("weekly"),
				MarkdownDescription: "The summary email of finding types set to \"summary email only\": daily or weekly. Default weekly.",
				Validators:          []validator.String{stringvalidator.OneOf("daily", "weekly")},
			},
			"digest_hour": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(9),
				MarkdownDescription: "Hour of the day (0-23, in digest_timezone) daily and weekly digests go out. Default 9.",
				Validators:          []validator.Int64{int64validator.Between(0, 23)},
			},
			"digest_timezone": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("UTC"),
				MarkdownDescription: "IANA time zone of digest_hour, e.g. America/Los_Angeles. Default UTC.",
			},
		},
	}
}

func (r *alertRoutingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func setStrings(ctx context.Context, v types.Set, fallback []string, diags *diag.Diagnostics) []string {
	if v.IsNull() || v.IsUnknown() {
		return fallback
	}
	var out []string
	diags.Append(v.ElementsAs(ctx, &out, false)...)
	if out == nil {
		out = []string{}
	}
	sort.Strings(out)
	return out
}

func strOr(v types.String, d string) string {
	if v.IsNull() || v.IsUnknown() {
		return d
	}
	return v.ValueString()
}

func alertRoutingToClient(ctx context.Context, m alertRoutingResourceModel) (client.AlertRouting, diag.Diagnostics) {
	var diags diag.Diagnostics
	a := client.DefaultAlertRouting()
	if !m.NotifyEmail.IsNull() && !m.NotifyEmail.IsUnknown() {
		a.NotifyEmail = m.NotifyEmail.ValueBool()
	}
	if !m.NotifyAllAdmins.IsNull() && !m.NotifyAllAdmins.IsUnknown() {
		a.NotifyAllAdmins = m.NotifyAllAdmins.ValueBool()
	}
	if !m.ExtraRecipients.IsNull() && !m.ExtraRecipients.IsUnknown() {
		var list []string
		diags.Append(m.ExtraRecipients.ElementsAs(ctx, &list, false)...)
		if list == nil {
			list = []string{}
		}
		a.ExtraRecipients = list
	}
	a.EmailSeverities = setStrings(ctx, m.EmailSeverities, a.EmailSeverities, &diags)
	a.WebhookSeverities = setStrings(ctx, m.WebhookSeverities, a.WebhookSeverities, &diags)
	a.PagerdutySeverities = setStrings(ctx, m.PagerdutySeverities, a.PagerdutySeverities, &diags)
	a.NtfyPriorities.Critical = strOr(m.NtfyPriorityCritical, a.NtfyPriorities.Critical)
	a.NtfyPriorities.Warning = strOr(m.NtfyPriorityWarning, a.NtfyPriorities.Warning)
	a.NtfyPriorities.Info = strOr(m.NtfyPriorityInfo, a.NtfyPriorities.Info)
	a.NtfyPriorities.Digest = strOr(m.NtfyPriorityDigest, a.NtfyPriorities.Digest)
	a.AlertDigestCadence = strOr(m.AlertDigestCadence, a.AlertDigestCadence)
	a.SummaryDigestCadence = strOr(m.SummaryDigestCadence, a.SummaryDigestCadence)
	if !m.DigestHour.IsNull() && !m.DigestHour.IsUnknown() {
		a.DigestHour = m.DigestHour.ValueInt64()
	}
	a.DigestTimezone = strOr(m.DigestTimezone, a.DigestTimezone)
	return a, diags
}

func alertRoutingFromClient(ctx context.Context, a *client.AlertRouting) (alertRoutingResourceModel, diag.Diagnostics) {
	recipients, diags := types.ListValueFrom(ctx, types.StringType, a.ExtraRecipients)
	return alertRoutingResourceModel{
		ID:                   types.StringValue(alertRoutingID),
		NotifyEmail:          types.BoolValue(a.NotifyEmail),
		NotifyAllAdmins:      types.BoolValue(a.NotifyAllAdmins),
		ExtraRecipients:      recipients,
		EmailSeverities:      severitySet(a.EmailSeverities...),
		WebhookSeverities:    severitySet(a.WebhookSeverities...),
		PagerdutySeverities:  severitySet(a.PagerdutySeverities...),
		NtfyPriorityCritical: types.StringValue(a.NtfyPriorities.Critical),
		NtfyPriorityWarning:  types.StringValue(a.NtfyPriorities.Warning),
		NtfyPriorityInfo:     types.StringValue(a.NtfyPriorities.Info),
		NtfyPriorityDigest:   types.StringValue(a.NtfyPriorities.Digest),
		AlertDigestCadence:   types.StringValue(a.AlertDigestCadence),
		SummaryDigestCadence: types.StringValue(a.SummaryDigestCadence),
		DigestHour:           types.Int64Value(a.DigestHour),
		DigestTimezone:       types.StringValue(a.DigestTimezone),
	}, diags
}

func (r *alertRoutingResource) put(ctx context.Context, m alertRoutingResourceModel) (*alertRoutingResourceModel, diag.Diagnostics) {
	in, diags := alertRoutingToClient(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	saved, err := r.client.PutAlertRouting(ctx, in)
	if err != nil {
		diags.AddError("Error saving alert routing", err.Error())
		return nil, diags
	}
	out, d := alertRoutingFromClient(ctx, saved)
	diags.Append(d...)
	return &out, diags
}

func (r *alertRoutingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertRoutingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, diags := r.put(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *alertRoutingResource) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	a, err := r.client.GetAlertRouting(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading alert routing", err.Error())
		return
	}
	m, diags := alertRoutingFromClient(ctx, a)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *alertRoutingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan alertRoutingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, diags := r.put(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

// Delete puts the defaults back: the routing is part of the organization.
func (r *alertRoutingResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if _, err := r.client.PutAlertRouting(ctx, client.DefaultAlertRouting()); err != nil {
		resp.Diagnostics.AddError("Error resetting alert routing", err.Error())
	}
}

// ImportState: `terraform import rustinion_alert_routing.this alert-routing`.
func (r *alertRoutingResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), alertRoutingID)...)
}
