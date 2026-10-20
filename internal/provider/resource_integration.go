package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
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
	_ resource.Resource                = &integrationResource{}
	_ resource.ResourceWithConfigure   = &integrationResource{}
	_ resource.ResourceWithImportState = &integrationResource{}
)

func NewIntegrationResource() resource.Resource {
	return &integrationResource{}
}

type integrationResource struct {
	client *client.Client
}

// integrationResourceModel. `Kind`/`GroupID` are both `RequiresReplace`
// — the server itself only ever updates `config` on a `PUT` (see
// `client.Integration`'s own doc comment), so changing either here is
// modeled the same way this API actually behaves: uninstall, then
// reinstall, not an in-place edit. `DashboardID`/`AlertRuleID`/
// `SyntheticCheckID`/`NotificationChannelID` are read-only: whichever
// subset `install` created for this `kind` (see integrations.rs's own
// module doc comment for which kind creates which; never all four).
type integrationResourceModel struct {
	ID                    types.String         `tfsdk:"id"`
	Kind                  types.String         `tfsdk:"kind"`
	GroupID               types.String         `tfsdk:"group_id"`
	Config                jsontypes.Normalized `tfsdk:"config"`
	DashboardID           types.String         `tfsdk:"dashboard_id"`
	AlertRuleID           types.String         `tfsdk:"alert_rule_id"`
	SyntheticCheckID      types.String         `tfsdk:"synthetic_check_id"`
	NotificationChannelID types.String         `tfsdk:"notification_channel_id"`
}

func (r *integrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration"
}

func (r *integrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Installs a Rustinion integration marketplace entry into one group. Installing always creates a dashboard, plus (depending on kind) a metric alert rule, a synthetic check, or a notification channel — see integrations.rs in the main repo for exactly what each kind creates. Changing kind or group_id replaces the resource (uninstall then reinstall), matching the API's own PUT semantics, which only ever updates config in place.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned integration install ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"kind": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of: aws_cloudwatch, kubernetes, docker, nginx, postgresql, prometheus_remote_write.",
				Validators: []validator.String{
					stringvalidator.OneOf("aws_cloudwatch", "kubernetes", "docker", "nginx", "postgresql", "prometheus_remote_write"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the group to install this integration into.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"config": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				CustomType:          jsontypes.NormalizedType{},
				Default:             stringdefault.StaticString("{}"),
				MarkdownDescription: "Integration-specific config as a JSON string; shape depends on kind (e.g. aws_cloudwatch: {\"access_key_id\": \"...\", \"secret_access_key\": \"...\"}). Several kinds (kubernetes, docker) need no config at all — defaults to {}.",
			},
			"dashboard_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the dashboard this integration's install created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"alert_rule_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the metric alert rule this integration's install created, if its kind creates one.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"synthetic_check_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the synthetic check this integration's install created, if its kind creates one.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"notification_channel_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the notification channel this integration's install created, if its kind creates one.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *integrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func integrationToClient(m integrationResourceModel) client.Integration {
	config := m.Config.ValueString()
	if config == "" {
		config = "{}"
	}
	return client.Integration{
		Kind:    m.Kind.ValueString(),
		GroupID: m.GroupID.ValueString(),
		Config:  json.RawMessage(config),
	}
}

func integrationFromClient(i *client.Integration) integrationResourceModel {
	config := string(i.Config)
	if config == "" {
		config = "{}"
	}
	return integrationResourceModel{
		ID:                    types.StringValue(i.ID),
		Kind:                  types.StringValue(i.Kind),
		GroupID:               types.StringValue(i.GroupID),
		Config:                jsontypes.NewNormalizedValue(config),
		DashboardID:           types.StringPointerValue(i.DashboardID),
		AlertRuleID:           types.StringPointerValue(i.AlertRuleID),
		SyntheticCheckID:      types.StringPointerValue(i.SyntheticCheckID),
		NotificationChannelID: types.StringPointerValue(i.NotificationChannelID),
	}
}

func (r *integrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan integrationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateIntegration(ctx, integrationToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error installing integration", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationFromClient(created))...)
}

func (r *integrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state integrationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	i, err := r.client.GetIntegration(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading integration", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationFromClient(i))...)
}

func (r *integrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan integrationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state integrationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateIntegration(ctx, state.ID.ValueString(), integrationToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating integration", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationFromClient(updated))...)
}

func (r *integrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state integrationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteIntegration(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting integration", err.Error())
	}
}

func (r *integrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
