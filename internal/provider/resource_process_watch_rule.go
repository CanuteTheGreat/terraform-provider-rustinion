package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &processWatchRuleResource{}
	_ resource.ResourceWithConfigure   = &processWatchRuleResource{}
	_ resource.ResourceWithImportState = &processWatchRuleResource{}
)

func NewProcessWatchRuleResource() resource.Resource {
	return &processWatchRuleResource{}
}

type processWatchRuleResource struct {
	client *client.Client
}

// processWatchRuleResourceModel — same group-scoped, no-sub-table shape
// as a log alert rule (see resource_log_alert_rule.go's own doc
// comment), simplified to the one filter field this rule actually has.
// Note this API is a real capability gain over the admin UI itself:
// `routes::process_watch`'s own add/remove-only handlers have no way
// to toggle `enabled` or rename a rule once created — this resource's
// own `Update` can do both.
type processWatchRuleResourceModel struct {
	ID          types.String `tfsdk:"id"`
	GroupID     types.String `tfsdk:"group_id"`
	ProcessName types.String `tfsdk:"process_name"`
	Enabled     types.Bool   `tfsdk:"enabled"`
}

func (r *processWatchRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_process_watch_rule"
}

func (r *processWatchRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion process-watch rule: raises a compliance finding when no device in the group reports a running process whose name contains process_name (case-insensitive substring) within its last 30-minute snapshot. See process_watch.rs in the main repo for evaluation semantics.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned process watch rule ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the group this process watch rule belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"process_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Substring a running process's name must contain to satisfy this rule (case-insensitive, e.g. \"nginx\" matches \"nginx: worker process\").",
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
		},
	}
}

func (r *processWatchRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func processWatchRuleToClient(m processWatchRuleResourceModel) client.ProcessWatchRule {
	return client.ProcessWatchRule{
		GroupID:     m.GroupID.ValueString(),
		ProcessName: m.ProcessName.ValueString(),
		Enabled:     m.Enabled.ValueBool(),
	}
}

func processWatchRuleFromClient(rule *client.ProcessWatchRule) processWatchRuleResourceModel {
	return processWatchRuleResourceModel{
		ID:          types.StringValue(rule.ID),
		GroupID:     types.StringValue(rule.GroupID),
		ProcessName: types.StringValue(rule.ProcessName),
		Enabled:     types.BoolValue(rule.Enabled),
	}
}

func (r *processWatchRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan processWatchRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateProcessWatchRule(ctx, processWatchRuleToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating process watch rule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, processWatchRuleFromClient(created))...)
}

func (r *processWatchRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state processWatchRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.client.GetProcessWatchRule(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading process watch rule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, processWatchRuleFromClient(rule))...)
}

func (r *processWatchRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan processWatchRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state processWatchRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateProcessWatchRule(ctx, state.ID.ValueString(), processWatchRuleToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating process watch rule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, processWatchRuleFromClient(updated))...)
}

func (r *processWatchRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state processWatchRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteProcessWatchRule(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting process watch rule", err.Error())
	}
}

func (r *processWatchRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
