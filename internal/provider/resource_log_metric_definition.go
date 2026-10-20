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
	_ resource.Resource                = &logMetricDefinitionResource{}
	_ resource.ResourceWithConfigure   = &logMetricDefinitionResource{}
	_ resource.ResourceWithImportState = &logMetricDefinitionResource{}
)

func NewLogMetricDefinitionResource() resource.Resource {
	return &logMetricDefinitionResource{}
}

type logMetricDefinitionResource struct {
	client *client.Client
}

// logMetricDefinitionResourceModel is tenant-wide, not group-scoped —
// there is no `group_id` attribute here at all, unlike most other
// resources this provider manages (dashboards, alert rules, ...).
type logMetricDefinitionResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Pattern    types.String `tfsdk:"pattern"`
	Extraction types.String `tfsdk:"extraction"`
	Enabled    types.Bool   `tfsdk:"enabled"`
}

func (r *logMetricDefinitionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_metric_definition"
}

func (r *logMetricDefinitionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion \"logs to metrics\" definition: a regex pattern evaluated against every ingested log line, turning a match into a custom-metric sample. Tenant-wide (no group_id) — bounded at 20 definitions per tenant server-side.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique within the tenant; also the resulting custom-metric's own name.",
			},
			"pattern": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A regex evaluated against every ingested log line's message. For extraction = \"capture_group\", must have at least one real capturing group.",
			},
			"extraction": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of: \"count\" (every matching line produces a sample of 1) or \"capture_group\" (the pattern's own first capturing group is parsed as the sample's numeric value).",
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
		},
	}
}

func (r *logMetricDefinitionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func logMetricDefinitionToClient(m logMetricDefinitionResourceModel) client.LogMetricDefinition {
	return client.LogMetricDefinition{
		Name:       m.Name.ValueString(),
		Pattern:    m.Pattern.ValueString(),
		Extraction: m.Extraction.ValueString(),
		Enabled:    m.Enabled.ValueBool(),
	}
}

func logMetricDefinitionFromClient(d *client.LogMetricDefinition) logMetricDefinitionResourceModel {
	return logMetricDefinitionResourceModel{
		ID:         types.StringValue(d.ID),
		Name:       types.StringValue(d.Name),
		Pattern:    types.StringValue(d.Pattern),
		Extraction: types.StringValue(d.Extraction),
		Enabled:    types.BoolValue(d.Enabled),
	}
}

func (r *logMetricDefinitionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan logMetricDefinitionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateLogMetricDefinition(ctx, logMetricDefinitionToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating log metric definition", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logMetricDefinitionFromClient(created))...)
}

func (r *logMetricDefinitionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state logMetricDefinitionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.GetLogMetricDefinition(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading log metric definition", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logMetricDefinitionFromClient(d))...)
}

func (r *logMetricDefinitionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan logMetricDefinitionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state logMetricDefinitionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateLogMetricDefinition(ctx, state.ID.ValueString(), logMetricDefinitionToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating log metric definition", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logMetricDefinitionFromClient(updated))...)
}

func (r *logMetricDefinitionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state logMetricDefinitionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteLogMetricDefinition(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting log metric definition", err.Error())
	}
}

func (r *logMetricDefinitionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
