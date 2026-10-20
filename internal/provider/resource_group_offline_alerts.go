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
	_ resource.Resource                = &groupOfflineAlertsResource{}
	_ resource.ResourceWithConfigure   = &groupOfflineAlertsResource{}
	_ resource.ResourceWithImportState = &groupOfflineAlertsResource{}
)

func NewGroupOfflineAlertsResource() resource.Resource {
	return &groupOfflineAlertsResource{}
}

type groupOfflineAlertsResource struct {
	client *client.Client
}

// A per-group singleton (it always exists, with defaults): the Terraform id
// is the group id, Create is a PUT, Delete puts the defaults back.
type groupOfflineAlertsResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	GroupID              types.String `tfsdk:"group_id"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	WarningAfterMinutes  types.Int64  `tfsdk:"warning_after_minutes"`
	CriticalAfterMinutes types.Int64  `tfsdk:"critical_after_minutes"`
}

func (r *groupOfflineAlertsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_offline_alerts"
}

func (r *groupOfflineAlertsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's \"device not responding\" alert: a device whose agent hasn't reached Rustinion (command connection or checkin) for warning_after_minutes raises a warning, and a critical after critical_after_minutes. Portable, asleep and away devices are never flagged. The group's master alert switch must also be on. Destroying this resource puts the defaults back (on, 10 and 30 minutes).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as group_id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The group these settings belong to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(client.DefaultGroupOfflineAlerts.Enabled),
				MarkdownDescription: "Alert when a device stops responding. Default true.",
			},
			"warning_after_minutes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(client.DefaultGroupOfflineAlerts.WarningAfterMinutes),
				MarkdownDescription: "Minutes without contact before a warning, 1-1440. Default 10.",
				Validators:          []validator.Int64{int64validator.Between(1, 1440)},
			},
			"critical_after_minutes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(client.DefaultGroupOfflineAlerts.CriticalAfterMinutes),
				MarkdownDescription: "Minutes without contact before a critical alert, 1-1440 and not lower than warning_after_minutes. Default 30.",
				Validators:          []validator.Int64{int64validator.Between(1, 1440)},
			},
		},
	}
}

func (r *groupOfflineAlertsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func groupOfflineAlertsToClient(m groupOfflineAlertsResourceModel) client.GroupOfflineAlerts {
	p := client.DefaultGroupOfflineAlerts
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		p.Enabled = m.Enabled.ValueBool()
	}
	if !m.WarningAfterMinutes.IsNull() && !m.WarningAfterMinutes.IsUnknown() {
		p.WarningAfterMinutes = m.WarningAfterMinutes.ValueInt64()
	}
	if !m.CriticalAfterMinutes.IsNull() && !m.CriticalAfterMinutes.IsUnknown() {
		p.CriticalAfterMinutes = m.CriticalAfterMinutes.ValueInt64()
	}
	return p
}

func groupOfflineAlertsFromClient(groupID string, p *client.GroupOfflineAlerts) groupOfflineAlertsResourceModel {
	return groupOfflineAlertsResourceModel{
		ID:                   types.StringValue(groupID),
		GroupID:              types.StringValue(groupID),
		Enabled:              types.BoolValue(p.Enabled),
		WarningAfterMinutes:  types.Int64Value(p.WarningAfterMinutes),
		CriticalAfterMinutes: types.Int64Value(p.CriticalAfterMinutes),
	}
}

func (r *groupOfflineAlertsResource) put(ctx context.Context, m groupOfflineAlertsResourceModel) (*groupOfflineAlertsResourceModel, error) {
	groupID := m.GroupID.ValueString()
	saved, err := r.client.PutGroupOfflineAlerts(ctx, groupID, groupOfflineAlertsToClient(m))
	if err != nil {
		return nil, err
	}
	out := groupOfflineAlertsFromClient(groupID, saved)
	return &out, nil
}

func (r *groupOfflineAlertsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupOfflineAlertsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting group offline alerts", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupOfflineAlertsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupOfflineAlertsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupOfflineAlerts(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group offline alerts", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupOfflineAlertsFromClient(groupID, p))...)
}

func (r *groupOfflineAlertsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupOfflineAlertsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating group offline alerts", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

// Delete puts the defaults back: the settings are part of the group.
func (r *groupOfflineAlertsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupOfflineAlertsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupOfflineAlerts(ctx, state.GroupID.ValueString(), client.DefaultGroupOfflineAlerts)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting group offline alerts", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_offline_alerts.servers <group-id>`.
func (r *groupOfflineAlertsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
