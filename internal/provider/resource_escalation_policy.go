package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                = &escalationPolicyResource{}
	_ resource.ResourceWithConfigure   = &escalationPolicyResource{}
	_ resource.ResourceWithImportState = &escalationPolicyResource{}
)

func NewEscalationPolicyResource() resource.Resource {
	return &escalationPolicyResource{}
}

type escalationPolicyResource struct {
	client *client.Client
}

// escalationPolicyStepModel is one `step {}` block. Steps have no `id`
// or `step_number` attribute exposed, same reason an alert rule's
// conditions don't: `PUT` deletes and reinserts every step wholesale in
// list order, so a step's server-assigned id/position aren't stable
// across updates — see `client.EscalationStep`'s own doc comment.
// `ChannelIDs` fans out to every one of them independently.
type escalationPolicyStepModel struct {
	ChannelIDs  types.List  `tfsdk:"channel_ids"`
	WaitMinutes types.Int64 `tfsdk:"wait_minutes"`
}

type escalationPolicyResourceModel struct {
	ID               types.String                `tfsdk:"id"`
	GroupID          types.String                `tfsdk:"group_id"`
	Name             types.String                `tfsdk:"name"`
	OnCallScheduleID types.String                `tfsdk:"on_call_schedule_id"`
	Step             []escalationPolicyStepModel `tfsdk:"step"`
}

func (r *escalationPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_policy"
}

func (r *escalationPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion escalation policy: an ordered notification path an alert rule or log alert rule can target via its own escalation_policy_id. Step 1 notifies immediately once the attached rule starts firing; if unacknowledged after that step's own wait_minutes, the next step notifies, and so on, stopping at the last step. See escalation.rs in the main repo for the full step-advancement/acknowledge state machine. Each step fans out to every channel in its channel_ids list independently — a missing/disabled channel or a failed delivery on one never blocks the others.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned escalation policy ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the group this escalation policy belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"on_call_schedule_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "An on-call schedule (in the same group) to name in every step's notification — purely for message enrichment, escalation timing never depends on it. Omit for none.",
			},
		},
		Blocks: map[string]schema.Block{
			"step": schema.ListNestedBlock{
				MarkdownDescription: "One escalation step, in order.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"channel_ids": schema.ListAttribute{
							Required:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Notification channel(s) this step notifies through — every one of them, independently. At least one required.",
							Validators: []validator.List{
								listvalidator.SizeAtLeast(1),
							},
						},
						"wait_minutes": schema.Int64Attribute{
							Optional:            true,
							Computed:            true,
							Default:             int64default.StaticInt64(15),
							MarkdownDescription: "How long this step waits, once reached, for an acknowledgement before escalating further. Meaningless on the last step. Defaults to 15.",
							Validators:          []validator.Int64{int64validator.AtLeast(1)},
						},
					},
				},
			},
		},
	}
}

func (r *escalationPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// escalationPolicyToClient converts the plan/state model into a wire
// request. It needs ctx/diagnostics (unlike every other resource's own
// ToClient function in this provider) purely because of ChannelIDs —
// types.List has no ValueString-style direct accessor, only
// ElementsAs, which can itself produce diagnostics (e.g. an unknown
// element mid-plan).
func escalationPolicyToClient(ctx context.Context, m escalationPolicyResourceModel) (client.EscalationPolicy, diag.Diagnostics) {
	var diags diag.Diagnostics
	steps := make([]client.EscalationStep, 0, len(m.Step))
	for _, s := range m.Step {
		var channelIDs []string
		diags.Append(s.ChannelIDs.ElementsAs(ctx, &channelIDs, false)...)
		steps = append(steps, client.EscalationStep{
			ChannelIDs:  channelIDs,
			WaitMinutes: s.WaitMinutes.ValueInt64(),
		})
	}
	p := client.EscalationPolicy{
		GroupID: m.GroupID.ValueString(),
		Name:    m.Name.ValueString(),
		Steps:   steps,
	}
	if !m.OnCallScheduleID.IsNull() {
		v := m.OnCallScheduleID.ValueString()
		p.OnCallScheduleID = &v
	}
	return p, diags
}

func escalationPolicyFromClient(ctx context.Context, p *client.EscalationPolicy) (escalationPolicyResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	steps := make([]escalationPolicyStepModel, 0, len(p.Steps))
	for _, s := range p.Steps {
		channelIDs, d := types.ListValueFrom(ctx, types.StringType, s.ChannelIDs)
		diags.Append(d...)
		steps = append(steps, escalationPolicyStepModel{
			ChannelIDs:  channelIDs,
			WaitMinutes: types.Int64Value(s.WaitMinutes),
		})
	}
	return escalationPolicyResourceModel{
		ID:               types.StringValue(p.ID),
		GroupID:          types.StringValue(p.GroupID),
		Name:             types.StringValue(p.Name),
		OnCallScheduleID: types.StringPointerValue(p.OnCallScheduleID),
		Step:             steps,
	}, diags
}

func (r *escalationPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan escalationPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := escalationPolicyToClient(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateEscalationPolicy(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Error creating escalation policy", err.Error())
		return
	}
	model, diags := escalationPolicyFromClient(ctx, created)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *escalationPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state escalationPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, err := r.client.GetEscalationPolicy(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading escalation policy", err.Error())
		return
	}
	model, diags := escalationPolicyFromClient(ctx, p)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *escalationPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan escalationPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state escalationPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := escalationPolicyToClient(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateEscalationPolicy(ctx, state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Error updating escalation policy", err.Error())
		return
	}
	model, diags := escalationPolicyFromClient(ctx, updated)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *escalationPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state escalationPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteEscalationPolicy(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting escalation policy", err.Error())
	}
}

func (r *escalationPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
