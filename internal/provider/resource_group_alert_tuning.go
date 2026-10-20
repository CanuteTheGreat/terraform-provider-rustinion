package provider

import (
	"context"
	"encoding/json"
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
	_ resource.Resource                = &groupAlertTuningResource{}
	_ resource.ResourceWithConfigure   = &groupAlertTuningResource{}
	_ resource.ResourceWithImportState = &groupAlertTuningResource{}
)

func NewGroupAlertTuningResource() resource.Resource {
	return &groupAlertTuningResource{}
}

type groupAlertTuningResource struct {
	client *client.Client
}

// A per-group singleton like rustinion_group_portable_policy: the id is
// the group id, Create/Update replace the whole tuning, Delete clears it.
type groupAlertTuningResourceModel struct {
	ID       types.String `tfsdk:"id"`
	GroupID  types.String `tfsdk:"group_id"`
	Settings types.String `tfsdk:"settings"`
}

func (r *groupAlertTuningResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_alert_tuning"
}

func (r *groupAlertTuningResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's alert tuning: CPU/memory/disk usage alerts (on/off, warning and critical levels, how long usage has to stay over the line, and the hysteresis margin before an alert clears), which out-of-memory kills alert (all / host_services / global_only), and how each finding type is delivered (immediate / digest / off). Devices can override any key (PUT /api/v1/customer/devices/{id}/alert-tuning). Destroying this resource clears the tuning so the built-in defaults apply.",
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
				MarkdownDescription: "The group this tuning belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"settings": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "JSON object, usually `jsonencode({...})`, e.g. " +
					"`jsonencode({ cpu_alert_enabled = false, disk_alert_threshold_pct = 90, disk_alert_duration_minutes = 30, " +
					"oom_alert_scope = \"host_services\", reason_delivery = { os_update_overdue = \"digest\" } })`. " +
					"Keys left out use the built-in defaults. GET /api/v1/customer/alert-tuning/presets lists ready-made sets.",
			},
		},
	}
}

func (r *groupAlertTuningResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// canonicalSettings re-encodes a settings object the way Terraform's
// jsonencode does (sorted keys, compact), so the server's answer and the
// config compare equal.
func canonicalSettings(m map[string]any) (string, error) {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func parseSettings(s string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("settings must be a JSON object: %w", err)
	}
	return m, nil
}

func (r *groupAlertTuningResource) replace(ctx context.Context, m groupAlertTuningResourceModel) (*groupAlertTuningResourceModel, error) {
	settings, err := parseSettings(m.Settings.ValueString())
	if err != nil {
		return nil, err
	}
	groupID := m.GroupID.ValueString()
	saved, err := r.client.ReplaceGroupAlertTuning(ctx, groupID, settings)
	if err != nil {
		return nil, err
	}
	return alertTuningFromClient(groupID, saved)
}

func alertTuningFromClient(groupID string, t *client.AlertTuning) (*groupAlertTuningResourceModel, error) {
	s, err := canonicalSettings(t.Settings)
	if err != nil {
		return nil, err
	}
	return &groupAlertTuningResourceModel{
		ID:       types.StringValue(groupID),
		GroupID:  types.StringValue(groupID),
		Settings: types.StringValue(s),
	}, nil
}

func (r *groupAlertTuningResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupAlertTuningResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.replace(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting group alert tuning", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupAlertTuningResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupAlertTuningResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	t, err := r.client.GetGroupAlertTuning(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group alert tuning", err.Error())
		return
	}
	m, err := alertTuningFromClient(groupID, t)
	if err != nil {
		resp.Diagnostics.AddError("Error reading group alert tuning", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *groupAlertTuningResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupAlertTuningResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.replace(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating group alert tuning", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupAlertTuningResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupAlertTuningResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.ReplaceGroupAlertTuning(ctx, state.GroupID.ValueString(), map[string]any{})
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error clearing group alert tuning", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_alert_tuning.ci <group-id>`.
func (r *groupAlertTuningResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
