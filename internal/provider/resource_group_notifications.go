package provider

import (
	"context"
	"fmt"

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
	_ resource.Resource                = &groupNotificationsResource{}
	_ resource.ResourceWithConfigure   = &groupNotificationsResource{}
	_ resource.ResourceWithImportState = &groupNotificationsResource{}
)

func NewGroupNotificationsResource() resource.Resource {
	return &groupNotificationsResource{}
}

type groupNotificationsResource struct {
	client *client.Client
}

// A per-group singleton: the Terraform id is the group id, Create is a PUT,
// Delete puts "normal" back.
type groupNotificationsResourceModel struct {
	ID      types.String `tfsdk:"id"`
	GroupID types.String `tfsdk:"group_id"`
	Mode    types.String `tfsdk:"mode"`
}

func (r *groupNotificationsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_notifications"
}

func (r *groupNotificationsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's Notifications setting: \"normal\" (the organization's routing and the group's alert tuning decide), \"digest_only\" (nothing is ever pushed for the group's devices - no ntfy/alert webhook, PagerDuty, notification channels or escalation policies - for any reason or severity, emergencies included; findings are still recorded and listed in the email digest; use it for test devices) or \"silent\" (not emailed either). Destroying this resource puts \"normal\" back.",
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
			"mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(client.DefaultGroupNotificationsMode),
				MarkdownDescription: "`normal`, `digest_only` or `silent`. Default `normal`.",
				Validators:          []validator.String{stringvalidator.OneOf("normal", "digest_only", "silent")},
			},
		},
	}
}

func (r *groupNotificationsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func groupNotificationsFromClient(groupID string, p *client.GroupNotifications) groupNotificationsResourceModel {
	return groupNotificationsResourceModel{
		ID:      types.StringValue(groupID),
		GroupID: types.StringValue(groupID),
		Mode:    types.StringValue(p.Mode),
	}
}

func (r *groupNotificationsResource) put(ctx context.Context, m groupNotificationsResourceModel) (*groupNotificationsResourceModel, error) {
	groupID := m.GroupID.ValueString()
	mode := client.DefaultGroupNotificationsMode
	if !m.Mode.IsNull() && !m.Mode.IsUnknown() {
		mode = m.Mode.ValueString()
	}
	saved, err := r.client.PutGroupNotifications(ctx, groupID, mode)
	if err != nil {
		return nil, err
	}
	out := groupNotificationsFromClient(groupID, saved)
	return &out, nil
}

func (r *groupNotificationsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupNotificationsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting group notifications", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupNotificationsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupNotificationsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupNotifications(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group notifications", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupNotificationsFromClient(groupID, p))...)
}

func (r *groupNotificationsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupNotificationsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating group notifications", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

// Delete puts "normal" back: the setting is part of the group.
func (r *groupNotificationsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupNotificationsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupNotifications(ctx, state.GroupID.ValueString(), client.DefaultGroupNotificationsMode)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting group notifications", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_notifications.test_devices <group-id>`.
func (r *groupNotificationsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
