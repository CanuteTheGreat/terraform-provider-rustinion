package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &logPipelineProcessorResource{}
	_ resource.ResourceWithConfigure   = &logPipelineProcessorResource{}
	_ resource.ResourceWithImportState = &logPipelineProcessorResource{}
)

func NewLogPipelineProcessorResource() resource.Resource {
	return &logPipelineProcessorResource{}
}

type logPipelineProcessorResource struct {
	client *client.Client
}

// logPipelineProcessorResourceModel is tenant-wide, not group-scoped —
// no `group_id` attribute at all, the same as
// rustinion_log_metric_definition.
type logPipelineProcessorResourceModel struct {
	ID            types.String         `tfsdk:"id"`
	Name          types.String         `tfsdk:"name"`
	ProcessorType types.String         `tfsdk:"processor_type"`
	Config        jsontypes.Normalized `tfsdk:"config"`
	SourceFilter  types.String         `tfsdk:"source_filter"`
	Position      types.Int64          `tfsdk:"position"`
	Enabled       types.Bool           `tfsdk:"enabled"`
}

func (r *logPipelineProcessorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_pipeline_processor"
}

func (r *logPipelineProcessorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages one stage of the Rustinion log processing/enrichment pipeline: a parse-and-enrich step run against every ingested log line before it is stored. Tenant-wide (no group_id) — bounded at 20 processors per tenant server-side.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique within the tenant.",
			},
			"processor_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of: \"grok_parse\" (match a Grok pattern against the line's message and keep its named fields as attributes) or \"remap\" (rename an attribute an earlier processor produced).",
			},
			"config": schema.StringAttribute{
				Optional:   true,
				Computed:   true,
				CustomType: jsontypes.NormalizedType{},
				Default:    stringdefault.StaticString("{}"),
				MarkdownDescription: "The processor's config as a JSON string, shape depending on processor_type: " +
					"grok_parse: {\"pattern\": \"%{IPV4:client_ip} - %{HTTPMETHOD:method} %{URIPATH:path} %{INT:status:int}\"}; " +
					"remap: {\"from\": \"status\", \"to\": \"http_status\"}. " +
					"A Grok pattern is a fixed grammar over a closed set of named patterns, never a raw regex — see the Log pipeline admin page for the supported pattern names. " +
					"In an HCL string, write each `%{` as `%%{` (e.g. `jsonencode({ pattern = \"%%{IPV4:client_ip}\" })`); HCL otherwise reads `%{` as a template directive and rejects the config.",
			},
			"source_filter": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Restricts this processor to log lines whose source matches exactly. Omit to run it against every source.",
			},
			"position": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "Processors run in ascending position, so a remap can act on what an earlier grok_parse extracted. Ties are broken by creation time.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
		},
	}
}

func (r *logPipelineProcessorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func logPipelineProcessorToClient(m logPipelineProcessorResourceModel) client.LogPipelineProcessor {
	config := m.Config.ValueString()
	if config == "" {
		config = "{}"
	}
	p := client.LogPipelineProcessor{
		Name:          m.Name.ValueString(),
		ProcessorType: m.ProcessorType.ValueString(),
		Config:        json.RawMessage(config),
		Position:      m.Position.ValueInt64(),
		Enabled:       m.Enabled.ValueBool(),
	}
	if !m.SourceFilter.IsNull() && !m.SourceFilter.IsUnknown() {
		filter := m.SourceFilter.ValueString()
		p.SourceFilter = &filter
	}
	return p
}

func logPipelineProcessorFromClient(p *client.LogPipelineProcessor) logPipelineProcessorResourceModel {
	m := logPipelineProcessorResourceModel{
		ID:            types.StringValue(p.ID),
		Name:          types.StringValue(p.Name),
		ProcessorType: types.StringValue(p.ProcessorType),
		Config:        jsontypes.NewNormalizedValue(string(p.Config)),
		Position:      types.Int64Value(p.Position),
		Enabled:       types.BoolValue(p.Enabled),
	}
	// The server normalises a blank source_filter to SQL NULL, so a
	// config that set "" reads back as unset rather than as an empty
	// string — mapping both to null here is what keeps that from
	// showing up as a permanent diff.
	if p.SourceFilter != nil && *p.SourceFilter != "" {
		m.SourceFilter = types.StringValue(*p.SourceFilter)
	} else {
		m.SourceFilter = types.StringNull()
	}
	return m
}

func (r *logPipelineProcessorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan logPipelineProcessorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateLogPipelineProcessor(ctx, logPipelineProcessorToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating log pipeline processor", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logPipelineProcessorFromClient(created))...)
}

func (r *logPipelineProcessorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state logPipelineProcessorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, err := r.client.GetLogPipelineProcessor(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading log pipeline processor", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logPipelineProcessorFromClient(p))...)
}

func (r *logPipelineProcessorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan logPipelineProcessorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state logPipelineProcessorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateLogPipelineProcessor(ctx, state.ID.ValueString(), logPipelineProcessorToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating log pipeline processor", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logPipelineProcessorFromClient(updated))...)
}

func (r *logPipelineProcessorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state logPipelineProcessorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteLogPipelineProcessor(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting log pipeline processor", err.Error())
	}
}

func (r *logPipelineProcessorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
