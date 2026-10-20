package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// splitImportID splits "schedule_id/override_id" into its two parts.
func splitImportID(id string) (scheduleID, overrideID string, ok bool) {
	scheduleID, overrideID, found := strings.Cut(id, "/")
	if !found || scheduleID == "" || overrideID == "" {
		return "", "", false
	}
	return scheduleID, overrideID, true
}

var (
	_ resource.Resource                = &onCallOverrideResource{}
	_ resource.ResourceWithConfigure   = &onCallOverrideResource{}
	_ resource.ResourceWithImportState = &onCallOverrideResource{}
)

func NewOnCallOverrideResource() resource.Resource {
	return &onCallOverrideResource{}
}

type onCallOverrideResource struct {
	client *client.Client
}

// onCallOverrideResourceModel. A separate resource from
// rustinion_on_call_schedule (not a nested block on it) because an
// override is an independent, time-bounded event with its own
// lifecycle — the server itself has no `PUT` for it either, only
// create/delete (`POST .../overrides`, `DELETE .../overrides/:id`),
// matching the admin UI's own separate add/remove actions. Every
// attribute is `RequiresReplace`: there is nothing to update in place
// server-side, so changing any of them is a genuine delete-then-create,
// not an edit.
type onCallOverrideResourceModel struct {
	ID         types.String `tfsdk:"id"`
	ScheduleID types.String `tfsdk:"schedule_id"`
	UserID     types.String `tfsdk:"user_id"`
	StartsAt   types.String `tfsdk:"starts_at"`
	EndsAt     types.String `tfsdk:"ends_at"`
}

func (r *onCallOverrideResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_on_call_override"
}

func (r *onCallOverrideResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a time-bounded on-call override: a user covers instead of whoever the rotation math would otherwise pick, for [starts_at, ends_at). See on_call.rs in the main repo. Every attribute forces replacement — there is no in-place update, matching the server's own create/delete-only API for overrides.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned override ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"schedule_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the on-call schedule this override applies to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "User covering for this window.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"starts_at": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "RFC3339 timestamp the override begins at.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ends_at": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "RFC3339 timestamp the override ends at. Must be after starts_at — rejected server-side otherwise.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *onCallOverrideResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func onCallOverrideFromClient(scheduleID string, o *client.OnCallOverride) onCallOverrideResourceModel {
	return onCallOverrideResourceModel{
		ID:         types.StringValue(o.ID),
		ScheduleID: types.StringValue(scheduleID),
		UserID:     types.StringValue(o.UserID),
		StartsAt:   types.StringValue(o.StartsAt),
		EndsAt:     types.StringValue(o.EndsAt),
	}
}

func (r *onCallOverrideResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan onCallOverrideResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scheduleID := plan.ScheduleID.ValueString()
	created, err := r.client.CreateOnCallOverride(ctx, scheduleID, client.OnCallOverride{
		UserID:   plan.UserID.ValueString(),
		StartsAt: plan.StartsAt.ValueString(),
		EndsAt:   plan.EndsAt.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating on-call override", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, onCallOverrideFromClient(scheduleID, created))...)
}

func (r *onCallOverrideResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state onCallOverrideResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scheduleID := state.ScheduleID.ValueString()
	o, err := r.client.GetOnCallOverride(ctx, scheduleID, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading on-call override", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, onCallOverrideFromClient(scheduleID, o))...)
}

// Update is unreachable in practice — every attribute above is
// RequiresReplace, so Terraform always plans a destroy+create instead
// of calling this. Implemented as a loud failure rather than silently
// doing the wrong thing if that assumption is ever wrong (e.g. a future
// schema change that removes one of those plan modifiers without
// updating this method to match).
func (r *onCallOverrideResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"on-call overrides cannot be updated in place",
		"Every attribute on rustinion_on_call_override is RequiresReplace; this method should never be invoked. If you see this error, the resource's schema and this method have drifted out of sync.",
	)
}

func (r *onCallOverrideResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state onCallOverrideResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteOnCallOverride(ctx, state.ScheduleID.ValueString(), state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting on-call override", err.Error())
	}
}

// ImportState takes "schedule_id/override_id" — an override's identity
// is only meaningful within its parent schedule (see
// `GetOnCallOverride`'s own doc comment on why there's no standalone
// per-override `GET`), unlike every other resource in this provider
// whose server-assigned id alone is enough to import.
func (r *onCallOverrideResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scheduleID, overrideID, found := splitImportID(req.ID)
	if !found {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: schedule_id/override_id. Got: %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schedule_id"), scheduleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), overrideID)...)
}
