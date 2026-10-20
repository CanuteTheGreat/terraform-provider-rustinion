package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupPortablePolicyResource{}
	_ resource.ResourceWithConfigure   = &groupPortablePolicyResource{}
	_ resource.ResourceWithImportState = &groupPortablePolicyResource{}
)

func NewGroupPortablePolicyResource() resource.Resource {
	return &groupPortablePolicyResource{}
}

type groupPortablePolicyResource struct {
	client *client.Client
}

// groupPortablePolicyResourceModel: a per-group singleton (it always
// exists, with defaults), so the Terraform id is the group id, Create is
// a PUT, and Delete puts the defaults back.
type groupPortablePolicyResourceModel struct {
	ID                types.String `tfsdk:"id"`
	GroupID           types.String `tfsdk:"group_id"`
	PortableDevices   types.Bool   `tfsdk:"portable_devices"`
	PortableGraceDays types.Int64  `tfsdk:"portable_grace_days"`
	AsleepGraceHours  types.Int64  `tfsdk:"asleep_grace_hours"`
}

func (r *groupPortablePolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_portable_policy"
}

func (r *groupPortablePolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's portable-device policy. Portable devices (laptops, travel machines) that stop checking in show as Away instead of raising stale/offline alerts, incidents or emails, their compliance clocks pause and no restarts are scheduled for them; after the grace period they raise one low-severity notice. A device's own Portable setting (device page, or PUT /api/v1/customer/devices/{id}/portable) overrides the group's. The asleep grace applies to every device in the group whose agent announced sleep or shutdown. Destroying this resource puts the defaults back (not portable, 14 days, 48 hours).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as group_id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The group this policy belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"portable_devices": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(client.DefaultGroupPortablePolicy.PortableDevices),
				MarkdownDescription: "Devices in this group are portable by default.",
			},
			"portable_grace_days": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(client.DefaultGroupPortablePolicy.PortableGraceDays),
				MarkdownDescription: "Days a portable device may be away before its one notice, 1-365. Default 14.",
				Validators:          []validator.Int64{int64validator.Between(1, 365)},
			},
			"asleep_grace_hours": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(client.DefaultGroupPortablePolicy.AsleepGraceHours),
				MarkdownDescription: "Hours a device that announced sleep or shutdown shows as Asleep without stale alerts, 1-336. Default 48.",
				Validators:          []validator.Int64{int64validator.Between(1, 336)},
			},
		},
	}
}

func (r *groupPortablePolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func groupPortablePolicyToClient(m groupPortablePolicyResourceModel) client.GroupPortablePolicy {
	p := client.DefaultGroupPortablePolicy
	if !m.PortableDevices.IsNull() && !m.PortableDevices.IsUnknown() {
		p.PortableDevices = m.PortableDevices.ValueBool()
	}
	if !m.PortableGraceDays.IsNull() && !m.PortableGraceDays.IsUnknown() {
		p.PortableGraceDays = m.PortableGraceDays.ValueInt64()
	}
	if !m.AsleepGraceHours.IsNull() && !m.AsleepGraceHours.IsUnknown() {
		p.AsleepGraceHours = m.AsleepGraceHours.ValueInt64()
	}
	return p
}

func groupPortablePolicyFromClient(groupID string, p *client.GroupPortablePolicy) groupPortablePolicyResourceModel {
	return groupPortablePolicyResourceModel{
		ID:                types.StringValue(groupID),
		GroupID:           types.StringValue(groupID),
		PortableDevices:   types.BoolValue(p.PortableDevices),
		PortableGraceDays: types.Int64Value(p.PortableGraceDays),
		AsleepGraceHours:  types.Int64Value(p.AsleepGraceHours),
	}
}

func (r *groupPortablePolicyResource) put(ctx context.Context, m groupPortablePolicyResourceModel) (*groupPortablePolicyResourceModel, error) {
	groupID := m.GroupID.ValueString()
	saved, err := r.client.PutGroupPortablePolicy(ctx, groupID, groupPortablePolicyToClient(m))
	if err != nil {
		return nil, err
	}
	out := groupPortablePolicyFromClient(groupID, saved)
	return &out, nil
}

func (r *groupPortablePolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupPortablePolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting group portable policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupPortablePolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupPortablePolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	policy, err := r.client.GetGroupPortablePolicy(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group portable policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupPortablePolicyFromClient(groupID, policy))...)
}

func (r *groupPortablePolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupPortablePolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating group portable policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

// Delete puts the defaults back: the policy is part of the group and
// can't be removed on its own.
func (r *groupPortablePolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupPortablePolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupPortablePolicy(ctx, state.GroupID.ValueString(), client.DefaultGroupPortablePolicy)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting group portable policy", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_portable_policy.laptops <group-id>`.
func (r *groupPortablePolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
