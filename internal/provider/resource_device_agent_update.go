package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
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
	_ resource.Resource                   = &deviceAgentUpdateResource{}
	_ resource.ResourceWithConfigure      = &deviceAgentUpdateResource{}
	_ resource.ResourceWithImportState    = &deviceAgentUpdateResource{}
	_ resource.ResourceWithValidateConfig = &deviceAgentUpdateResource{}
)

// ---------- rustinion_device_agent_update ----------

func NewDeviceAgentUpdateResource() resource.Resource {
	return &deviceAgentUpdateResource{}
}

type deviceAgentUpdateResource struct {
	client *client.Client
}

type deviceAgentUpdateResourceModel struct {
	ID            types.String `tfsdk:"id"`
	DeviceID      types.String `tfsdk:"device_id"`
	Mode          types.String `tfsdk:"mode"`
	PinnedVersion types.String `tfsdk:"pinned_version"`
	Reason        types.String `tfsdk:"reason"`
	Source        types.String `tfsdk:"source"`
	Explanation   types.String `tfsdk:"explanation"`
}

func (r *deviceAgentUpdateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_agent_update"
}

func (r *deviceAgentUpdateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "One device's own Rustinion agent self-update setting, overriding its group's agent-update policy for that device only: \"follow_group\", \"auto\" (always install new releases), \"paused\" (no new agent release is offered, including by Force update) or \"pinned\" (updated up to pinned_version and kept there; never downgraded). The machine owner's local --no-self-update still wins. Destroying this resource sets the device back to follow_group.",
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
				Default:             stringdefault.StaticString("follow_group"),
				MarkdownDescription: "\"follow_group\", \"auto\", \"paused\" or \"pinned\". Default follow_group.",
				Validators:          []validator.String{stringvalidator.OneOf("follow_group", "auto", "paused", "pinned")},
			},
			"pinned_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A published agent version such as \"0.1.230\". Required when mode is pinned, not allowed otherwise.",
				Validators: []validator.String{stringvalidator.RegexMatches(
					regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`),
					"must be a version like 0.1.230",
				)},
			},
			"reason": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Why this device differs from its group (shown on the device and group pages and in the audit log). Only for auto, paused or pinned; at most 500 characters.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 500),
					stringvalidator.RegexMatches(regexp.MustCompile(`^\S(?s:.*\S)?$`), "must not start or end with spaces"),
				},
			},
			"source": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "device_setting or group_policy.",
			},
			"explanation": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The effective setting in words, as the device page shows it.",
			},
		},
	}
}

// ValidateConfig: pinned needs a version and only pinned takes one.
func (r *deviceAgentUpdateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m deviceAgentUpdateResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Mode.IsUnknown() || m.PinnedVersion.IsUnknown() {
		return
	}
	if m.Reason.IsUnknown() {
		return
	}
	if msg := deviceAgentUpdateConfigError(deviceAgentUpdateMode(m), m.PinnedVersion.ValueString(), m.Reason.ValueString()); msg != "" {
		resp.Diagnostics.AddError("Invalid agent update setting", msg)
	}
}

func deviceAgentUpdateConfigError(mode, pinned, reason string) string {
	if mode == "follow_group" && reason != "" {
		return "reason is only kept for a device-level setting (auto, paused or pinned)"
	}
	if mode == "pinned" && pinned == "" {
		return "pinned_version is required when mode is \"pinned\""
	}
	if mode != "pinned" && pinned != "" {
		return "pinned_version is only used when mode is \"pinned\""
	}
	return ""
}

func (r *deviceAgentUpdateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

func agentUpdateOptString(p *string) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

func deviceAgentUpdateFromClient(deviceID string, p *client.DeviceAgentUpdate) deviceAgentUpdateResourceModel {
	mode := p.Mode
	if mode == "" {
		mode = "follow_group"
	}
	return deviceAgentUpdateResourceModel{
		ID:            types.StringValue(deviceID),
		DeviceID:      types.StringValue(deviceID),
		Mode:          types.StringValue(mode),
		PinnedVersion: agentUpdateOptString(p.PinnedVersion),
		Reason:        agentUpdateOptString(p.Reason),
		Source:        types.StringValue(p.Source),
		Explanation:   types.StringValue(p.Explanation),
	}
}

func deviceAgentUpdateMode(m deviceAgentUpdateResourceModel) string {
	if m.Mode.IsNull() || m.Mode.IsUnknown() || m.Mode.ValueString() == "" {
		return "follow_group"
	}
	return m.Mode.ValueString()
}

func (r *deviceAgentUpdateResource) put(ctx context.Context, m deviceAgentUpdateResourceModel) (*deviceAgentUpdateResourceModel, error) {
	deviceID := m.DeviceID.ValueString()
	saved, err := r.client.PutDeviceAgentUpdate(ctx, deviceID, deviceAgentUpdateMode(m), m.PinnedVersion.ValueString(), m.Reason.ValueString())
	if err != nil {
		return nil, err
	}
	out := deviceAgentUpdateFromClient(deviceID, saved)
	return &out, nil
}

func (r *deviceAgentUpdateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan deviceAgentUpdateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting device agent update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *deviceAgentUpdateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state deviceAgentUpdateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deviceID := state.DeviceID.ValueString()
	if deviceID == "" {
		deviceID = state.ID.ValueString()
	}
	p, err := r.client.GetDeviceAgentUpdate(ctx, deviceID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading device agent update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, deviceAgentUpdateFromClient(deviceID, p))...)
}

func (r *deviceAgentUpdateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan deviceAgentUpdateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating device agent update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *deviceAgentUpdateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state deviceAgentUpdateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutDeviceAgentUpdate(ctx, state.DeviceID.ValueString(), "follow_group", "", "")
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting device agent update", err.Error())
	}
}

// ImportState takes the device id.
func (r *deviceAgentUpdateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("device_id"), req.ID)...)
}
