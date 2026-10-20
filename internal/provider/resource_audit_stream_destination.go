package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource              = &auditStreamDestinationResource{}
	_ resource.ResourceWithConfigure = &auditStreamDestinationResource{}
	// Deliberately NOT ResourceWithImportState: an imported destination's
	// signing_secret (for a webhook destination) can never be recovered
	// (the customer API never returns it after creation, by design), so
	// an import would always produce a resource with an empty/wrong
	// secret in state — worse than not supporting import at all. Create
	// a new destination through Terraform instead of importing an
	// existing one. This applies to a splunk_hec destination too, for
	// consistency, even though its own hec_token has no such write-once
	// problem — one resource, one real import story.
)

func NewAuditStreamDestinationResource() resource.Resource {
	return &auditStreamDestinationResource{}
}

type auditStreamDestinationResource struct {
	client *client.Client
}

// auditStreamDestinationResourceModel's `SigningSecret` is the one
// attribute handling a webhook destination's own write-once secret: it
// is `Sensitive: true, Computed: true`, populated only from the real
// `Create` response (the sole place the customer API ever returns it),
// and Read deliberately preserves whatever is already in state for this
// one field rather than overwriting it with the always-empty value
// `GetAuditStreamDestination` returns — see `Read`'s own doc comment for
// why. `HECToken` needs no such workaround at all: it's a customer-
// *supplied* secret (like `notification_channel`'s own `config`) with no
// "shown once" property, so it's an ordinary, freely round-tripping
// attribute — just `Sensitive: true` for display purposes.
type auditStreamDestinationResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	DestinationType      types.String `tfsdk:"destination_type"`
	WebhookURL           types.String `tfsdk:"webhook_url"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	SigningSecret        types.String `tfsdk:"signing_secret"`
	HECEndpoint          types.String `tfsdk:"hec_endpoint"`
	HECToken             types.String `tfsdk:"hec_token"`
	HECIndex             types.String `tfsdk:"hec_index"`
	HECSourcetype        types.String `tfsdk:"hec_sourcetype"`
	SyslogHost           types.String `tfsdk:"syslog_host"`
	SyslogPort           types.Int64  `tfsdk:"syslog_port"`
	SyslogUseTLS         types.Bool   `tfsdk:"syslog_use_tls"`
	LastDeliveredAt      types.String `tfsdk:"last_delivered_at"`
	LastDeliveryFailedAt types.String `tfsdk:"last_delivery_failed_at"`
	LastDeliveryError    types.String `tfsdk:"last_delivery_error"`
}

func (r *auditStreamDestinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_audit_stream_destination"
}

func (r *auditStreamDestinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion audit-log streaming export destination: continuous delivery of this tenant's own admin_audit_log events to an external SIEM — a generic HMAC-signed webhook, a native Splunk HTTP Event Collector endpoint, or a native RFC 5424 syslog forwarder. One destination per tenant.",
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
			"destination_type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("webhook"),
				MarkdownDescription: "\"webhook\" (default, a generic HMAC-signed delivery), \"splunk_hec\" (a native Splunk HTTP Event Collector client), or \"syslog\" (a native RFC 5424 syslog forwarder over TCP, RFC 6587 framed, optionally TLS). Fixed at creation — switching types means destroying and recreating this resource.",
				Validators:          []validator.String{stringvalidator.OneOf("webhook", "splunk_hec", "syslog")},
			},
			"webhook_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Required for destination_type = \"webhook\"; not used for \"splunk_hec\".",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Defaults to true on create.",
			},
			"signing_secret": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "webhook only. Server-generated HMAC-SHA256 secret for verifying the X-Rustinion-Signature header on every delivery. Only ever known at creation time — the API never returns it again, so this value is preserved from state on every later refresh rather than re-read (there is nothing to re-read). Rotation is not supported in v1: change this by tainting and recreating the resource. Always empty for a splunk_hec destination.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"hec_endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Required for destination_type = \"splunk_hec\": the customer's own full HEC URL, e.g. \"https://splunk.example.com:8088/services/collector/event\". Not used for \"webhook\".",
			},
			"hec_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Required for destination_type = \"splunk_hec\": the customer's own HEC token, sent as the \"Authorization: Splunk <token>\" header. Unlike signing_secret, this is a customer-supplied secret the API freely returns on every read, so it round-trips normally through this attribute (no state-preservation workaround needed) — but it is not updatable via Update (no rotation without recreate, matching signing_secret's own stated v1 scope cut).",
			},
			"hec_index": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "splunk_hec only, optional: Splunk applies its own default index when this is unset.",
			},
			"hec_sourcetype": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "splunk_hec only, optional: Splunk applies its own default sourcetype when this is unset.",
			},
			"syslog_host": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Required for destination_type = \"syslog\": the receiver's hostname or IP. Not used for the other two types.",
			},
			"syslog_port": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Required for destination_type = \"syslog\", 1-65535.",
			},
			"syslog_use_tls": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "syslog only. true wraps the TCP connection in TLS (server-certificate validation only, no client-certificate/mutual-TLS support in v1) before sending; false (the default when set for a syslog destination) sends over plain TCP. syslog has no secret/token concept at all — this flag, or bare network trust when it's false, is the entire real \"authentication\" story for this delivery type.",
			},
			"last_delivered_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 timestamp of the last successfully delivered event, or this destination's own creation time if nothing has been delivered yet.",
			},
			"last_delivery_failed_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 timestamp of the most recent delivery failure, empty if the destination is currently healthy.",
			},
			"last_delivery_error": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The most recent delivery failure's own detail, empty if the destination is currently healthy.",
			},
		},
	}
}

func (r *auditStreamDestinationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// auditStreamDestinationFromClient fills every attribute EXCEPT
// signing_secret from a real API response — callers set that one
// separately (from Create's own response) or preserve it from state (in
// Read/Update), per this resource's own doc comment. hec_token has no
// such special handling — it's filled here like every other attribute.
// `types.StringPointerValue` (not `types.StringValue`) is required for
// every field that's `*string` on the client struct — see that struct's
// own doc comment for why (a real "Provider produced inconsistent
// result after apply" failure on an actual acceptance-test run before
// this was fixed).
func auditStreamDestinationFromClient(d *client.AuditStreamDestination) auditStreamDestinationResourceModel {
	return auditStreamDestinationResourceModel{
		ID:                   types.StringValue(d.ID),
		Name:                 types.StringValue(d.Name),
		DestinationType:      types.StringValue(d.DestinationType),
		WebhookURL:           types.StringPointerValue(d.WebhookURL),
		Enabled:              types.BoolValue(d.Enabled),
		HECEndpoint:          types.StringPointerValue(d.HECEndpoint),
		HECToken:             types.StringPointerValue(d.HECToken),
		HECIndex:             types.StringPointerValue(d.HECIndex),
		HECSourcetype:        types.StringPointerValue(d.HECSourcetype),
		SyslogHost:           types.StringPointerValue(d.SyslogHost),
		SyslogPort:           types.Int64PointerValue(d.SyslogPort),
		SyslogUseTLS:         types.BoolPointerValue(d.SyslogUseTLS),
		LastDeliveredAt:      types.StringValue(d.LastDeliveredAt),
		LastDeliveryFailedAt: types.StringValue(d.LastDeliveryFailedAt),
		LastDeliveryError:    types.StringValue(d.LastDeliveryError),
	}
}

func (r *auditStreamDestinationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan auditStreamDestinationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateAuditStreamDestination(ctx, client.AuditStreamDestination{
		Name:            plan.Name.ValueString(),
		DestinationType: plan.DestinationType.ValueString(),
		WebhookURL:      plan.WebhookURL.ValueStringPointer(),
		HECEndpoint:     plan.HECEndpoint.ValueStringPointer(),
		HECToken:        plan.HECToken.ValueStringPointer(),
		HECIndex:        plan.HECIndex.ValueStringPointer(),
		HECSourcetype:   plan.HECSourcetype.ValueStringPointer(),
		SyslogHost:      plan.SyslogHost.ValueStringPointer(),
		SyslogPort:      plan.SyslogPort.ValueInt64Pointer(),
		SyslogUseTLS:    plan.SyslogUseTLS.ValueBoolPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating audit stream destination", err.Error())
		return
	}

	state := auditStreamDestinationFromClient(created)
	// The one and only place this ever comes from a real API response.
	state.SigningSecret = types.StringValue(created.SigningSecret)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Read is called on every `terraform plan`/refresh. GetAuditStreamDestination
// never returns signing_secret (the customer API deliberately never
// echoes it back after creation), so overwriting state's own value with
// the client struct's always-empty field would either wipe a real secret
// out of state or, worse, cause a permanent "wants to change
// signing_secret" diff on every plan. Preserve whatever is already in
// state for that one attribute; update everything else, including
// hec_token, from the real response.
func (r *auditStreamDestinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state auditStreamDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.GetAuditStreamDestination(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading audit stream destination", err.Error())
		return
	}

	newState := auditStreamDestinationFromClient(d)
	newState.SigningSecret = state.SigningSecret // preserved, see doc comment above
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *auditStreamDestinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan auditStreamDestinationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state auditStreamDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateAuditStreamDestination(ctx, state.ID.ValueString(), client.AuditStreamDestinationUpdate{
		Name:          plan.Name.ValueString(),
		WebhookURL:    plan.WebhookURL.ValueString(),
		HECEndpoint:   plan.HECEndpoint.ValueString(),
		HECIndex:      plan.HECIndex.ValueString(),
		HECSourcetype: plan.HECSourcetype.ValueString(),
		SyslogHost:    plan.SyslogHost.ValueString(),
		SyslogPort:    plan.SyslogPort.ValueInt64(),
		SyslogUseTLS:  plan.SyslogUseTLS.ValueBool(),
		Enabled:       plan.Enabled.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating audit stream destination", err.Error())
		return
	}

	newState := auditStreamDestinationFromClient(updated)
	newState.SigningSecret = state.SigningSecret // never updatable, see doc comment above
	// hec_token itself is not part of the Update request body (see the
	// client struct's own doc comment — no rotation via Update), but
	// unlike signing_secret, the server's own real response to this
	// call still includes its current, unchanged, decrypted value — no
	// preservation-from-state needed here, `auditStreamDestinationFromClient`
	// already filled it in correctly above.
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *auditStreamDestinationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state auditStreamDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteAuditStreamDestination(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting audit stream destination", err.Error())
	}
}
