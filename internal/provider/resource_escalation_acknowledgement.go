package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &escalationAcknowledgementResource{}
	_ resource.ResourceWithConfigure   = &escalationAcknowledgementResource{}
	_ resource.ResourceWithImportState = &escalationAcknowledgementResource{}
)

func NewEscalationAcknowledgementResource() resource.Resource {
	return &escalationAcknowledgementResource{}
}

type escalationAcknowledgementResource struct {
	client *client.Client
}

// escalationAcknowledgementResourceModel models a one-shot *action*
// (acknowledging a firing escalation), not persistent server-side
// configuration the way every other resource in this provider does —
// there is no "acknowledgement" object to CRUD, only
// escalation_runs.acknowledged_at, a field the server itself sets in
// response to a POST (see routes::customer_api::acknowledge_escalation
// in the main repo). This resource is Terraform's own established
// pattern for that: applying it performs the action once; Read checks
// whether it's still true and drops the resource from state the moment
// it isn't (the rule cleared and re-fired, or was independently
// acknowledged/un-acknowledged some other way), so a later `apply`
// naturally re-triggers the action rather than silently going stale.
// Delete is a no-op — there is no way to "un-acknowledge" server-side,
// so removing this resource only ever forgets about it in Terraform's
// own state.
type escalationAcknowledgementResourceModel struct {
	ID             types.String `tfsdk:"id"`
	RuleKind       types.String `tfsdk:"rule_kind"`
	RuleID         types.String `tfsdk:"rule_id"`
	CurrentStep    types.Int64  `tfsdk:"current_step"`
	AcknowledgedAt types.String `tfsdk:"acknowledged_at"`
}

func (r *escalationAcknowledgementResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_acknowledgement"
}

func (r *escalationAcknowledgementResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Acknowledges a rustinion_alert_rule or rustinion_log_alert_rule's currently-firing escalation, stopping it from advancing to its next step — the Terraform-managed counterpart to the admin UI's own \"Acknowledge\" button. This models a one-shot action, not persistent configuration: applying it acknowledges the rule's current escalation episode once. Read re-checks the server and drops this resource from state the moment the acknowledgement no longer holds (the rule cleared and fired again, starting a fresh unacknowledged episode) — a later apply then re-acknowledges the new episode. There is no way to un-acknowledge server-side, so destroying this resource only forgets it in Terraform's own state, it does not resume escalation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "\"<rule_kind>/<rule_id>\" — this resource's own import identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"rule_kind": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "\"alert_rule\" or \"log_alert_rule\".",
				Validators:          []validator.String{stringvalidator.OneOf("alert_rule", "log_alert_rule")},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rule_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the alert rule or log alert rule to acknowledge.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"current_step": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The escalation step the run was on at the moment it was acknowledged.",
			},
			"acknowledged_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 timestamp the server recorded the acknowledgement at.",
			},
		},
	}
}

func (r *escalationAcknowledgementResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *escalationAcknowledgementResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan escalationAcknowledgementResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ruleKind := plan.RuleKind.ValueString()
	ruleID := plan.RuleID.ValueString()
	if err := r.client.AcknowledgeEscalation(ctx, ruleKind, ruleID); err != nil {
		switch {
		case client.IsConflict(err):
			resp.Diagnostics.AddError(
				"No active escalation to acknowledge",
				fmt.Sprintf("This rule has no active, unacknowledged escalation right now (nothing is firing, it's already acknowledged, or it's already resolved): %s", err),
			)
		case client.IsNotFound(err):
			resp.Diagnostics.AddError("Rule not found", err.Error())
		default:
			resp.Diagnostics.AddError("Error acknowledging escalation", err.Error())
		}
		return
	}

	status, err := r.client.GetEscalationStatus(ctx, ruleKind, ruleID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading escalation status after acknowledging", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, escalationAcknowledgementFromStatus(ruleKind, ruleID, status))...)
}

func (r *escalationAcknowledgementResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state escalationAcknowledgementResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ruleKind := state.RuleKind.ValueString()
	ruleID := state.RuleID.ValueString()
	status, err := r.client.GetEscalationStatus(ctx, ruleKind, ruleID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading escalation status", err.Error())
		return
	}
	if status.AcknowledgedAt == nil {
		// The episode this resource once acknowledged is gone — either
		// the rule cleared and fired again (a fresh, unacknowledged
		// episode) or it was un-acknowledged some other way. Either
		// way, this resource's own acknowledgement no longer holds;
		// dropping it from state lets a later apply re-acknowledge the
		// current episode instead of silently pretending the old one
		// still applies.
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, escalationAcknowledgementFromStatus(ruleKind, ruleID, status))...)
}

// Update is unreachable in practice — rule_kind and rule_id are the
// only meaningful inputs and both are RequiresReplace, so Terraform
// always plans a destroy+create instead of calling this. Implemented as
// a loud failure rather than silently doing the wrong thing, same
// convention `resource_on_call_override.go`'s own Update uses.
func (r *escalationAcknowledgementResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"escalation acknowledgements cannot be updated in place",
		"rule_kind and rule_id are RequiresReplace and there is nothing else to change; this method should never be invoked. If you see this error, the resource's schema and this method have drifted out of sync.",
	)
}

// Delete is a no-op — there is no server-side "un-acknowledge" endpoint
// (see this resource's own Schema doc comment), so removing this
// resource only ever forgets it in Terraform's state.
func (r *escalationAcknowledgementResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *escalationAcknowledgementResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	ruleKind, ruleID, found := splitImportID(req.ID)
	if !found {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: rule_kind/rule_id. Got: %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rule_kind"), ruleKind)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rule_id"), ruleID)...)
}

func escalationAcknowledgementFromStatus(ruleKind, ruleID string, status *client.EscalationStatus) escalationAcknowledgementResourceModel {
	acknowledgedAt := ""
	if status.AcknowledgedAt != nil {
		acknowledgedAt = *status.AcknowledgedAt
	}
	return escalationAcknowledgementResourceModel{
		ID:             types.StringValue(ruleKind + "/" + ruleID),
		RuleKind:       types.StringValue(ruleKind),
		RuleID:         types.StringValue(ruleID),
		CurrentStep:    types.Int64Value(status.CurrentStep),
		AcknowledgedAt: types.StringValue(acknowledgedAt),
	}
}
