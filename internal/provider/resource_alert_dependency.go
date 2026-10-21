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
	_ resource.Resource                = &alertDependencyResource{}
	_ resource.ResourceWithConfigure   = &alertDependencyResource{}
	_ resource.ResourceWithImportState = &alertDependencyResource{}
)

func NewAlertDependencyResource() resource.Resource {
	return &alertDependencyResource{}
}

type alertDependencyResource struct {
	client *client.Client
}

// alertDependencyResourceModel: every attribute is RequiresReplace (the
// server has create/read/delete only), same shape as rustinion_silence.
type alertDependencyResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	DeviceID               types.String `tfsdk:"device_id"`
	GroupID                types.String `tfsdk:"group_id"`
	ParentDeviceID         types.String `tfsdk:"parent_device_id"`
	ParentSnmpTargetID     types.String `tfsdk:"parent_snmp_target_id"`
	ParentSyntheticCheckID types.String `tfsdk:"parent_synthetic_check_id"`
	CreatedAt              types.String `tfsdk:"created_at"`
}

func (r *alertDependencyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_dependency"
}

func replaceString(desc string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: desc,
		Validators:          validators,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

func (r *alertDependencyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion alert dependency: while the upstream (a router running the agent, an SNMP-monitored router/switch, or an uptime check on an uplink) is down, the alerts of the device (or every device of the group) behind it are suppressed, so one outage sends one alert. Set exactly one of device_id/group_id and exactly one parent_*. Device dependencies replace group ones; with neither, Rustinion uses the device's reported default gateway and LLDP neighbors. Every attribute forces a replace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned dependency ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"device_id": replaceString("The device whose alerts depend on the upstream.",
				stringvalidator.ExactlyOneOf(path.MatchRoot("group_id"))),
			"group_id": replaceString("Every device of this group depends on the upstream (unless the device has its own dependencies)."),
			"parent_device_id": replaceString("Upstream: another enrolled device (down = its agent hasn't reached Rustinion for 3 minutes).",
				stringvalidator.ExactlyOneOf(path.MatchRoot("parent_snmp_target_id"), path.MatchRoot("parent_synthetic_check_id"))),
			"parent_snmp_target_id":     replaceString("Upstream: an SNMP target (down = its latest poll was unreachable)."),
			"parent_synthetic_check_id": replaceString("Upstream: an uptime check (down = its latest run failed)."),
			"created_at": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *alertDependencyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func alertDepOptString(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func alertDependencyFromClient(d *client.AlertDependency) alertDependencyResourceModel {
	return alertDependencyResourceModel{
		ID:                     types.StringValue(d.ID),
		DeviceID:               types.StringPointerValue(d.DeviceID),
		GroupID:                types.StringPointerValue(d.GroupID),
		ParentDeviceID:         types.StringPointerValue(d.ParentDeviceID),
		ParentSnmpTargetID:     types.StringPointerValue(d.ParentSnmpTargetID),
		ParentSyntheticCheckID: types.StringPointerValue(d.ParentSyntheticCheckID),
		CreatedAt:              types.StringValue(d.CreatedAt),
	}
}

func (r *alertDependencyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertDependencyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateAlertDependency(ctx, client.AlertDependency{
		DeviceID:               alertDepOptString(plan.DeviceID),
		GroupID:                alertDepOptString(plan.GroupID),
		ParentDeviceID:         alertDepOptString(plan.ParentDeviceID),
		ParentSnmpTargetID:     alertDepOptString(plan.ParentSnmpTargetID),
		ParentSyntheticCheckID: alertDepOptString(plan.ParentSyntheticCheckID),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating alert dependency", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, alertDependencyFromClient(created))...)
}

func (r *alertDependencyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertDependencyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetAlertDependency(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading alert dependency", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, alertDependencyFromClient(d))...)
}

// Update is unreachable: every attribute is RequiresReplace.
func (r *alertDependencyResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"alert dependencies cannot be updated in place",
		"Every attribute on rustinion_alert_dependency is RequiresReplace; this method should never be invoked.",
	)
}

func (r *alertDependencyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertDependencyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAlertDependency(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting alert dependency", err.Error())
	}
}

func (r *alertDependencyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
