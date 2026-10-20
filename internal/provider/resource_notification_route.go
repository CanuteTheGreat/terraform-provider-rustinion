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
	_ resource.Resource                = &notificationRouteResource{}
	_ resource.ResourceWithConfigure   = &notificationRouteResource{}
	_ resource.ResourceWithImportState = &notificationRouteResource{}
)

func NewNotificationRouteResource() resource.Resource {
	return &notificationRouteResource{}
}

type notificationRouteResource struct {
	client *client.Client
}

// notificationRouteResourceModel routes a rustinion_notification_channel
// to the findings it should receive: every finding in the tenant
// ("tenant"), one group ("group" + group_id), one composite alert rule
// ("rule" + rule_id — see resource_alert_rule.go), or every finding
// carrying a given device tag ("tag" + tag).
//
// **The "tag" scope only ever matches device-scoped compliance
// findings** (cpu_high, disk_full, stale, malware_detected, and similar
// — the ones tied to a real device with its own `device_tags`). An
// alert-rule/log-alert-rule/SNMP-trap-rule/SLO/synthetic-check-driven
// finding structurally carries no tags at all — it's tied to that
// rule's own id, not a device — so a "tag" route can never match one.
// See notifications.rs's own `channels_for_finding` doc comment in the
// main repo for the full reasoning; this is a real, permanent
// limitation of the underlying data model, not a v1 gap.
//
// Unlike rustinion_notification_channel, a route has no update
// endpoint at all — scope is immutable server-side (see
// routes::notifications::add_route/remove_route's own add/remove-only
// shape), so every attribute here is RequiresReplace and Update should
// never actually run (same "loud failure if it does" convention
// resource_escalation_acknowledgement.go already established). Import
// needs both ids ("channel_id/route_id") since fetching or deleting a
// route always requires its parent channel's id too.
type notificationRouteResourceModel struct {
	ID        types.String `tfsdk:"id"`
	ChannelID types.String `tfsdk:"channel_id"`
	Scope     types.String `tfsdk:"scope"`
	ScopeID   types.String `tfsdk:"scope_id"`
	Tag       types.String `tfsdk:"tag"`
}

func (r *notificationRouteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_route"
}

func (r *notificationRouteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Routes a rustinion_notification_channel to the findings it should receive. \"tag\"-scoped routes only ever match device-scoped compliance findings (disk/CPU/memory/patch-compliance and similar) — never an alert-rule/log-alert/SNMP-trap/SLO/synthetic-check finding, which has no device tags of its own. A route's scope is immutable; changing any attribute destroys and recreates it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned route ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"channel_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the rustinion_notification_channel this route applies to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"scope": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of \"tenant\", \"group\", \"rule\", or \"tag\".",
				Validators: []validator.String{
					stringvalidator.OneOf("tenant", "group", "rule", "tag"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"scope_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The group ID (scope = \"group\") or alert-rule ID (scope = \"rule\") this route applies to. Unused for \"tenant\"/\"tag\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tag": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The device tag (scope = \"tag\") this route matches on — e.g. \"production\". Unused for every other scope. Matches a device-scoped finding whose device carries this exact tag; never matches an alert-rule/log-alert/SNMP-trap/SLO/synthetic-check-driven finding.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *notificationRouteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func notificationRouteFromClient(channelID string, in *client.NotificationRoute) notificationRouteResourceModel {
	scopeID := types.StringNull()
	if in.ScopeID != nil {
		scopeID = types.StringValue(*in.ScopeID)
	}
	tag := types.StringNull()
	if in.Tag != nil {
		tag = types.StringValue(*in.Tag)
	}
	return notificationRouteResourceModel{
		ID:        types.StringValue(in.ID),
		ChannelID: types.StringValue(channelID),
		Scope:     types.StringValue(in.Scope),
		ScopeID:   scopeID,
		Tag:       tag,
	}
}

func (r *notificationRouteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan notificationRouteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	channelID := plan.ChannelID.ValueString()
	var scopeID *string
	if !plan.ScopeID.IsNull() {
		v := plan.ScopeID.ValueString()
		scopeID = &v
	}
	var tag *string
	if !plan.Tag.IsNull() {
		v := plan.Tag.ValueString()
		tag = &v
	}
	created, err := r.client.CreateNotificationRoute(ctx, channelID, client.NotificationRouteCreateInput{
		Scope:   plan.Scope.ValueString(),
		ScopeID: scopeID,
		Tag:     tag,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating notification route", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, notificationRouteFromClient(channelID, created))...)
}

func (r *notificationRouteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state notificationRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	channelID := state.ChannelID.ValueString()
	route, err := r.client.GetNotificationRoute(ctx, channelID, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading notification route", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, notificationRouteFromClient(channelID, route))...)
}

// Update is unreachable in practice — every attribute is RequiresReplace
// and there is nothing else to change, so Terraform always plans a
// destroy+create instead of calling this. Implemented as a loud failure
// rather than silently doing the wrong thing, same convention
// resource_escalation_acknowledgement.go's own Update uses.
func (r *notificationRouteResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"notification routes cannot be updated in place",
		"Every attribute is RequiresReplace and there is nothing else to change; this method should never be invoked. If you see this error, the resource's schema and this method have drifted out of sync.",
	)
}

func (r *notificationRouteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state notificationRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteNotificationRoute(ctx, state.ChannelID.ValueString(), state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting notification route", err.Error())
	}
}

func (r *notificationRouteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	channelID, routeID, found := splitImportID(req.ID)
	if !found {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: channel_id/route_id. Got: %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("channel_id"), channelID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), routeID)...)
}
