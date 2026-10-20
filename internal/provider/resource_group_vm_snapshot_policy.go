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
	_ resource.Resource                = &groupVMSnapshotPolicyResource{}
	_ resource.ResourceWithConfigure   = &groupVMSnapshotPolicyResource{}
	_ resource.ResourceWithImportState = &groupVMSnapshotPolicyResource{}
)

func NewGroupVMSnapshotPolicyResource() resource.Resource {
	return &groupVMSnapshotPolicyResource{}
}

type groupVMSnapshotPolicyResource struct {
	client *client.Client
}

// A per-group singleton (it always exists, with defaults): the Terraform id
// is the group id, Create is a PUT, and Delete puts the defaults back.
type groupVMSnapshotPolicyResourceModel struct {
	ID      types.String `tfsdk:"id"`
	GroupID types.String `tfsdk:"group_id"`
	Enabled types.Bool   `tfsdk:"enabled"`
	Keep    types.Int64  `tfsdk:"keep"`
}

func (r *groupVMSnapshotPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_vm_snapshot_policy"
}

func (r *groupVMSnapshotPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's VM-aware upgrade policy. For devices that are VMs on a Proxmox VE or libvirt/KVM host Rustinion also manages (matched by SMBIOS UUID, MAC address or hostname), the host snapshots the VM before a firmware upgrade or an OS-update install, the upgrade waits for the verified snapshot, and the device page offers a rollback to it. When the host can't take one, the upgrade goes ahead with the configuration backup only. Only Rustinion's own snapshots (rustinion-pre-...) are pruned, down to keep per VM. Destroying this resource puts the defaults back (on, keep 2).",
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
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(client.DefaultGroupVMSnapshotPolicy.Enabled),
				MarkdownDescription: "Snapshot the VM on its managed hypervisor before upgrades. Default true.",
			},
			"keep": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(client.DefaultGroupVMSnapshotPolicy.Keep),
				MarkdownDescription: "Rustinion snapshots kept per VM, 1-10. Default 2.",
				Validators:          []validator.Int64{int64validator.Between(1, 10)},
			},
		},
	}
}

func (r *groupVMSnapshotPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func groupVMSnapshotPolicyToClient(m groupVMSnapshotPolicyResourceModel) client.GroupVMSnapshotPolicy {
	p := client.DefaultGroupVMSnapshotPolicy
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		p.Enabled = m.Enabled.ValueBool()
	}
	if !m.Keep.IsNull() && !m.Keep.IsUnknown() {
		p.Keep = m.Keep.ValueInt64()
	}
	return p
}

func groupVMSnapshotPolicyFromClient(groupID string, p *client.GroupVMSnapshotPolicy) groupVMSnapshotPolicyResourceModel {
	return groupVMSnapshotPolicyResourceModel{
		ID:      types.StringValue(groupID),
		GroupID: types.StringValue(groupID),
		Enabled: types.BoolValue(p.Enabled),
		Keep:    types.Int64Value(p.Keep),
	}
}

func (r *groupVMSnapshotPolicyResource) put(ctx context.Context, m groupVMSnapshotPolicyResourceModel) (*groupVMSnapshotPolicyResourceModel, error) {
	groupID := m.GroupID.ValueString()
	saved, err := r.client.PutGroupVMSnapshotPolicy(ctx, groupID, groupVMSnapshotPolicyToClient(m))
	if err != nil {
		return nil, err
	}
	out := groupVMSnapshotPolicyFromClient(groupID, saved)
	return &out, nil
}

func (r *groupVMSnapshotPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupVMSnapshotPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting group VM snapshot policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupVMSnapshotPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupVMSnapshotPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	policy, err := r.client.GetGroupVMSnapshotPolicy(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group VM snapshot policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupVMSnapshotPolicyFromClient(groupID, policy))...)
}

func (r *groupVMSnapshotPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupVMSnapshotPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating group VM snapshot policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

// Delete puts the defaults back: the policy is part of the group.
func (r *groupVMSnapshotPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupVMSnapshotPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupVMSnapshotPolicy(ctx, state.GroupID.ValueString(), client.DefaultGroupVMSnapshotPolicy)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting group VM snapshot policy", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_vm_snapshot_policy.routers <group-id>`.
func (r *groupVMSnapshotPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
