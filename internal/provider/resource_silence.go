package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &silenceResource{}
	_ resource.ResourceWithConfigure   = &silenceResource{}
	_ resource.ResourceWithImportState = &silenceResource{}
)

func NewSilenceResource() resource.Resource {
	return &silenceResource{}
}

type silenceResource struct {
	client *client.Client
}

// silenceResourceModel. Every attribute is `RequiresReplace` — the
// server has no update endpoint for a silence at all (create/read/
// cancel only, matching the admin UI's own `routes::silences::
// create_silence`/`cancel_silence`), so `Update` is unreachable in
// practice — same reasoning as `rustinion_slo`'s own doc comment.
// `cancelled_at` is exposed read-only: deleting this resource cancels
// the silence (`cancelled_at = now()`) rather than removing its row,
// so re-importing an already-cancelled silence is possible and shows
// a real, non-null `cancelled_at`.
type silenceResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	GroupID     types.String `tfsdk:"group_id"`
	DeviceID    types.String `tfsdk:"device_id"`
	AlertRuleID types.String `tfsdk:"alert_rule_id"`
	Tag         types.String `tfsdk:"tag"`
	StartsAt    types.String `tfsdk:"starts_at"`
	EndsAt      types.String `tfsdk:"ends_at"`
	CancelledAt types.String `tfsdk:"cancelled_at"`
	Reason      types.String `tfsdk:"reason"`
}

func (r *silenceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_silence"
}

func (r *silenceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion alert silence: a scoped, time-bounded mute of alert notification dispatch for a known issue. See silences.rs in the main repo for exactly which dispatch paths check this and how it differs from a maintenance window or an escalation policy. At least one of group_id, device_id, alert_rule_id, or tag is required. There is no update endpoint for a silence at all — every attribute here forces a replace. Deleting this resource cancels the silence server-side rather than removing its row.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned silence ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Mutes every alert in this group. Combinable with device_id/alert_rule_id/tag rather than mutually exclusive.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"device_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"alert_rule_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tag": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"starts_at": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "RFC3339 timestamp the silence begins at.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ends_at": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "RFC3339 timestamp the silence ends at. Must be after starts_at — rejected server-side otherwise.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"reason": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cancelled_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 timestamp this silence was cancelled at, if it has been. Null while active.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *silenceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func silenceToClient(m silenceResourceModel) client.Silence {
	s := client.Silence{
		Name:     m.Name.ValueString(),
		StartsAt: m.StartsAt.ValueString(),
		EndsAt:   m.EndsAt.ValueString(),
	}
	if !m.GroupID.IsNull() {
		v := m.GroupID.ValueString()
		s.GroupID = &v
	}
	if !m.DeviceID.IsNull() {
		v := m.DeviceID.ValueString()
		s.DeviceID = &v
	}
	if !m.AlertRuleID.IsNull() {
		v := m.AlertRuleID.ValueString()
		s.AlertRuleID = &v
	}
	if !m.Tag.IsNull() {
		v := m.Tag.ValueString()
		s.Tag = &v
	}
	if !m.Reason.IsNull() {
		v := m.Reason.ValueString()
		s.Reason = &v
	}
	return s
}

func silenceFromClient(s *client.Silence) silenceResourceModel {
	return silenceResourceModel{
		ID:          types.StringValue(s.ID),
		Name:        types.StringValue(s.Name),
		GroupID:     types.StringPointerValue(s.GroupID),
		DeviceID:    types.StringPointerValue(s.DeviceID),
		AlertRuleID: types.StringPointerValue(s.AlertRuleID),
		Tag:         types.StringPointerValue(s.Tag),
		StartsAt:    types.StringValue(s.StartsAt),
		EndsAt:      types.StringValue(s.EndsAt),
		CancelledAt: types.StringPointerValue(s.CancelledAt),
		Reason:      types.StringPointerValue(s.Reason),
	}
}

func (r *silenceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan silenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateSilence(ctx, silenceToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating silence", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, silenceFromClient(created))...)
}

func (r *silenceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state silenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := r.client.GetSilence(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading silence", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, silenceFromClient(s))...)
}

// Update is unreachable in practice — every attribute above is
// RequiresReplace, so Terraform always plans a destroy+create instead
// of calling this. See `resource_on_call_override.go`'s own Update for
// the same defensive-failure reasoning.
func (r *silenceResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"silences cannot be updated in place",
		"Every attribute on rustinion_silence is RequiresReplace; this method should never be invoked. If you see this error, the resource's schema and this method have drifted out of sync.",
	)
}

func (r *silenceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state silenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteSilence(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error cancelling silence", err.Error())
	}
}

func (r *silenceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
