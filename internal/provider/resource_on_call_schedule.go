package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &onCallScheduleResource{}
	_ resource.ResourceWithConfigure   = &onCallScheduleResource{}
	_ resource.ResourceWithImportState = &onCallScheduleResource{}
)

func NewOnCallScheduleResource() resource.Resource {
	return &onCallScheduleResource{}
}

type onCallScheduleResource struct {
	client *client.Client
}

// onCallScheduleMemberModel is one `member {}` block — the rotation
// roster, in order. Members have no `id`/`position` attribute exposed,
// same "order is the position, full-replace on PUT" reasoning as an
// escalation policy's own steps — see `client.OnCallMember`'s own doc
// comment.
type onCallScheduleMemberModel struct {
	UserID types.String `tfsdk:"user_id"`
}

// onCallScheduleResourceModel. `overrides` are deliberately not part of
// this resource at all — see `rustinion_on_call_override`'s own doc
// comment for why they're a separate resource instead of a nested block
// here.
type onCallScheduleResourceModel struct {
	ID           types.String                `tfsdk:"id"`
	GroupID      types.String                `tfsdk:"group_id"`
	Name         types.String                `tfsdk:"name"`
	RotationDays types.Int64                 `tfsdk:"rotation_days"`
	StartedAt    types.String                `tfsdk:"started_at"`
	Member       []onCallScheduleMemberModel `tfsdk:"member"`
}

func (r *onCallScheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_on_call_schedule"
}

func (r *onCallScheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion on-call schedule: a rotation of users an escalation policy can optionally name in its notifications (purely for message enrichment — escalation timing never depends on it). See on_call.rs in the main repo for the rotation math: who's on call is a deterministic, stateless function of (now - started_at) / rotation_days over the member list below, in order. Time-bounded overrides (a real vacation/swap) are a separate resource, rustinion_on_call_override, not a block here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned on-call schedule ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the group this on-call schedule belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"rotation_days": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(7),
				MarkdownDescription: "Rotation length in days — 7 for a weekly rotation. Defaults to 7.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"started_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When this schedule's rotation clock started (RFC3339), server-assigned at creation and never changes afterward.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"member": schema.ListNestedBlock{
				MarkdownDescription: "One rotation member, in order.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"user_id": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
		},
	}
}

func (r *onCallScheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func onCallScheduleToClient(m onCallScheduleResourceModel) client.OnCallSchedule {
	members := make([]client.OnCallMember, 0, len(m.Member))
	for _, mm := range m.Member {
		members = append(members, client.OnCallMember{UserID: mm.UserID.ValueString()})
	}
	return client.OnCallSchedule{
		GroupID:      m.GroupID.ValueString(),
		Name:         m.Name.ValueString(),
		RotationDays: m.RotationDays.ValueInt64(),
		Members:      members,
	}
}

func onCallScheduleFromClient(s *client.OnCallSchedule) onCallScheduleResourceModel {
	members := make([]onCallScheduleMemberModel, 0, len(s.Members))
	for _, mm := range s.Members {
		members = append(members, onCallScheduleMemberModel{UserID: types.StringValue(mm.UserID)})
	}
	return onCallScheduleResourceModel{
		ID:           types.StringValue(s.ID),
		GroupID:      types.StringValue(s.GroupID),
		Name:         types.StringValue(s.Name),
		RotationDays: types.Int64Value(s.RotationDays),
		StartedAt:    types.StringValue(s.StartedAt),
		Member:       members,
	}
}

func (r *onCallScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan onCallScheduleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateOnCallSchedule(ctx, onCallScheduleToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating on-call schedule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, onCallScheduleFromClient(created))...)
}

func (r *onCallScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state onCallScheduleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := r.client.GetOnCallSchedule(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading on-call schedule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, onCallScheduleFromClient(s))...)
}

func (r *onCallScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan onCallScheduleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state onCallScheduleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateOnCallSchedule(ctx, state.ID.ValueString(), onCallScheduleToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating on-call schedule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, onCallScheduleFromClient(updated))...)
}

func (r *onCallScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state onCallScheduleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteOnCallSchedule(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting on-call schedule", err.Error())
	}
}

func (r *onCallScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
