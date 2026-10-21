package provider

import (
	"context"

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
	_ resource.Resource                = &deviceDisplayNameResource{}
	_ resource.ResourceWithConfigure   = &deviceDisplayNameResource{}
	_ resource.ResourceWithImportState = &deviceDisplayNameResource{}
)

// ---------- rustinion_device_display_name ----------

func NewDeviceDisplayNameResource() resource.Resource {
	return &deviceDisplayNameResource{}
}

type deviceDisplayNameResource struct {
	client *client.Client
}

type deviceDisplayNameResourceModel struct {
	ID          types.String `tfsdk:"id"`
	DeviceID    types.String `tfsdk:"device_id"`
	DisplayName types.String `tfsdk:"display_name"`
	Hostname    types.String `tfsdk:"hostname"`
	Name        types.String `tfsdk:"name"`
}

func (r *deviceDisplayNameResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_display_name"
}

func (r *deviceDisplayNameResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Sets a device's display name: shown instead of the agent-reported hostname in device lists, alerts and digest emails (the hostname stays visible beside it unless the name already contains it). Destroying this resource clears it, so the hostname is used again.",
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
			"display_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The display name, 1-120 characters.",
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 120)},
			},
			"hostname": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The hostname the agent reports.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "What the product calls the device: the display name, with the hostname in brackets unless the name already contains it.",
			},
		},
	}
}

func (r *deviceDisplayNameResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

func ddnOptString(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

func deviceDisplayNameFromClient(deviceID string, d *client.DeviceDisplayName) deviceDisplayNameResourceModel {
	return deviceDisplayNameResourceModel{
		ID:          types.StringValue(deviceID),
		DeviceID:    types.StringValue(deviceID),
		DisplayName: ddnOptString(d.DisplayName),
		Hostname:    ddnOptString(d.Hostname),
		Name:        ddnOptString(d.Name),
	}
}

func (r *deviceDisplayNameResource) put(ctx context.Context, m deviceDisplayNameResourceModel) (*deviceDisplayNameResourceModel, error) {
	deviceID := m.DeviceID.ValueString()
	name := m.DisplayName.ValueString()
	saved, err := r.client.PutDeviceDisplayName(ctx, deviceID, &name)
	if err != nil {
		return nil, err
	}
	out := deviceDisplayNameFromClient(deviceID, saved)
	return &out, nil
}

func (r *deviceDisplayNameResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan deviceDisplayNameResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting device display name", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *deviceDisplayNameResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state deviceDisplayNameResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deviceID := state.DeviceID.ValueString()
	if deviceID == "" {
		deviceID = state.ID.ValueString()
	}
	d, err := r.client.GetDeviceDisplayName(ctx, deviceID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading device display name", err.Error())
		return
	}
	// Cleared outside Terraform: the resource is gone.
	if d.DisplayName == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, deviceDisplayNameFromClient(deviceID, d))...)
}

func (r *deviceDisplayNameResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan deviceDisplayNameResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating device display name", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *deviceDisplayNameResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state deviceDisplayNameResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutDeviceDisplayName(ctx, state.DeviceID.ValueString(), nil)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error clearing device display name", err.Error())
	}
}

// ImportState takes the device id.
func (r *deviceDisplayNameResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("device_id"), req.ID)...)
}
