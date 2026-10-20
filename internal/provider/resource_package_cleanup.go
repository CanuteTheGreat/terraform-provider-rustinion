package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupPackageCleanupResource{}
	_ resource.ResourceWithConfigure   = &groupPackageCleanupResource{}
	_ resource.ResourceWithImportState = &groupPackageCleanupResource{}
	_ resource.Resource                = &devicePackageCleanupResource{}
	_ resource.ResourceWithConfigure   = &devicePackageCleanupResource{}
	_ resource.ResourceWithImportState = &devicePackageCleanupResource{}
)

func configureClient(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *client.Client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return nil
	}
	return c
}

// ---------- rustinion_group_package_cleanup ----------

func NewGroupPackageCleanupResource() resource.Resource {
	return &groupPackageCleanupResource{}
}

type groupPackageCleanupResource struct {
	client *client.Client
}

// A per-group singleton: the Terraform id is the group id, Create is a
// PUT, Delete puts the default (on) back.
type groupPackageCleanupResourceModel struct {
	ID      types.String `tfsdk:"id"`
	GroupID types.String `tfsdk:"group_id"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

func (r *groupPackageCleanupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_package_cleanup"
}

func (r *groupPackageCleanupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's package cleanup: devices remove packages that are no longer needed (old kernels, never the running or newest one; orphaned dependencies) and clean their package cache after every OS update and at most daily (Windows component cleanup monthly). Never on router/firewall appliances. On by default; destroying this resource turns it back on. Devices can override it with rustinion_device_package_cleanup.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as group_id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The group this setting belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(client.DefaultGroupPackageCleanup.Enabled),
				MarkdownDescription: "Remove unneeded packages and clean the package cache. Default true.",
			},
		},
	}
}

func (r *groupPackageCleanupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

func groupPackageCleanupToClient(m groupPackageCleanupResourceModel) client.GroupPackageCleanup {
	p := client.DefaultGroupPackageCleanup
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		p.Enabled = m.Enabled.ValueBool()
	}
	return p
}

func groupPackageCleanupFromClient(groupID string, p *client.GroupPackageCleanup) groupPackageCleanupResourceModel {
	return groupPackageCleanupResourceModel{
		ID:      types.StringValue(groupID),
		GroupID: types.StringValue(groupID),
		Enabled: types.BoolValue(p.Enabled),
	}
}

func (r *groupPackageCleanupResource) put(ctx context.Context, m groupPackageCleanupResourceModel) (*groupPackageCleanupResourceModel, error) {
	groupID := m.GroupID.ValueString()
	saved, err := r.client.PutGroupPackageCleanup(ctx, groupID, groupPackageCleanupToClient(m))
	if err != nil {
		return nil, err
	}
	out := groupPackageCleanupFromClient(groupID, saved)
	return &out, nil
}

func (r *groupPackageCleanupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupPackageCleanupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting group package cleanup", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupPackageCleanupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupPackageCleanupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupPackageCleanup(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group package cleanup", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupPackageCleanupFromClient(groupID, p))...)
}

func (r *groupPackageCleanupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupPackageCleanupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating group package cleanup", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupPackageCleanupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupPackageCleanupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupPackageCleanup(ctx, state.GroupID.ValueString(), client.DefaultGroupPackageCleanup)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting group package cleanup", err.Error())
	}
}

// ImportState takes the group id.
func (r *groupPackageCleanupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}

// ---------- rustinion_device_package_cleanup ----------

func NewDevicePackageCleanupResource() resource.Resource {
	return &devicePackageCleanupResource{}
}

type devicePackageCleanupResource struct {
	client *client.Client
}

type devicePackageCleanupResourceModel struct {
	ID       types.String `tfsdk:"id"`
	DeviceID types.String `tfsdk:"device_id"`
	Mode     types.String `tfsdk:"mode"`
	Enabled  types.Bool   `tfsdk:"enabled"`
	Source   types.String `tfsdk:"source"`
}

func (r *devicePackageCleanupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_package_cleanup"
}

func (r *devicePackageCleanupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Overrides package cleanup for one device: \"on\", \"off\" or \"inherit\" (the group's rustinion_group_package_cleanup). The machine owner's local --no-package-cleanup still wins; enabled and source show the effective result. Destroying this resource sets the device back to inherit.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as device_id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"device_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The device.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("inherit"),
				MarkdownDescription: "\"on\", \"off\" or \"inherit\". Default inherit.",
				Validators:          []validator.String{stringvalidator.OneOf("on", "off", "inherit")},
			},
			"enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether cleanup is effectively on for the device.",
			},
			"source": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Where the effective value comes from: device_local_opt_out, device_setting or group_policy.",
			},
		},
	}
}

func (r *devicePackageCleanupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

func devicePackageCleanupFromClient(deviceID string, p *client.DevicePackageCleanup) devicePackageCleanupResourceModel {
	mode := p.Mode
	if mode == "" {
		mode = "inherit"
	}
	return devicePackageCleanupResourceModel{
		ID:       types.StringValue(deviceID),
		DeviceID: types.StringValue(deviceID),
		Mode:     types.StringValue(mode),
		Enabled:  types.BoolValue(p.Enabled),
		Source:   types.StringValue(p.Source),
	}
}

func devicePackageCleanupMode(m devicePackageCleanupResourceModel) string {
	if m.Mode.IsNull() || m.Mode.IsUnknown() || m.Mode.ValueString() == "" {
		return "inherit"
	}
	return m.Mode.ValueString()
}

func (r *devicePackageCleanupResource) put(ctx context.Context, m devicePackageCleanupResourceModel) (*devicePackageCleanupResourceModel, error) {
	deviceID := m.DeviceID.ValueString()
	saved, err := r.client.PutDevicePackageCleanup(ctx, deviceID, devicePackageCleanupMode(m))
	if err != nil {
		return nil, err
	}
	out := devicePackageCleanupFromClient(deviceID, saved)
	return &out, nil
}

func (r *devicePackageCleanupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan devicePackageCleanupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting device package cleanup", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *devicePackageCleanupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state devicePackageCleanupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deviceID := state.DeviceID.ValueString()
	if deviceID == "" {
		deviceID = state.ID.ValueString()
	}
	p, err := r.client.GetDevicePackageCleanup(ctx, deviceID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading device package cleanup", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, devicePackageCleanupFromClient(deviceID, p))...)
}

func (r *devicePackageCleanupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan devicePackageCleanupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating device package cleanup", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *devicePackageCleanupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state devicePackageCleanupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutDevicePackageCleanup(ctx, state.DeviceID.ValueString(), "inherit")
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting device package cleanup", err.Error())
	}
}

// ImportState takes the device id.
func (r *devicePackageCleanupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("device_id"), req.ID)...)
}
