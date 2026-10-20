package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"git.canutethegreat.com/CanuteTheGreat/rustinion/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupCustomMetricsResource{}
	_ resource.ResourceWithConfigure   = &groupCustomMetricsResource{}
	_ resource.ResourceWithImportState = &groupCustomMetricsResource{}
)

func NewGroupCustomMetricsResource() resource.Resource {
	return &groupCustomMetricsResource{}
}

type groupCustomMetricsResource struct {
	client *client.Client
}

type scrapeTargetModel struct {
	URL          types.String `tfsdk:"url"`
	Metrics      types.List   `tfsdk:"metrics"`
	Namespace    types.String `tfsdk:"namespace"`
	IntervalSecs types.Int64  `tfsdk:"interval_secs"`
	MaxSeries    types.Int64  `tfsdk:"max_series"`
	Tags         types.Map    `tfsdk:"tags"`
}

type groupCustomMetricsModel struct {
	ID                types.String        `tfsdk:"id"`
	GroupID           types.String        `tfsdk:"group_id"`
	Enabled           types.Bool          `tfsdk:"enabled"`
	StatsdEnabled     types.Bool          `tfsdk:"statsd_enabled"`
	StatsdAddress     types.String        `tfsdk:"statsd_address"`
	StatsdSocket      types.String        `tfsdk:"statsd_socket"`
	FlushIntervalSecs types.Int64         `tfsdk:"flush_interval_secs"`
	ScrapeTargets     []scrapeTargetModel `tfsdk:"scrape_targets"`
	MonitoringEnabled types.Bool          `tfsdk:"monitoring_enabled"`
}

func (r *groupCustomMetricsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_custom_metrics"
}

func (r *groupCustomMetricsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's custom-metrics collection by the Rustinion agent: a DogStatsD/StatsD listener (any Datadog or StatsD client library) and Prometheus/OpenMetrics scrape targets, stored as custom metrics (alert rules `custom:<name>`, dashboards, SLOs). Samples are only stored while the group's basic monitoring is on (`monitoring_enabled`, read-only here). A device can override the on/off switch (device page or `PUT /api/v1/customer/devices/{id}/custom-metrics`), and the device's own policy.toml `[custom_metrics] enabled = false` wins over both. Destroying this resource switches collection off and restores the defaults.",
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
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Collect custom metrics on the group's devices. Default false.",
			},
			"statsd_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Run the StatsD/DogStatsD listener. Default true.",
			},
			"statsd_address": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("127.0.0.1:8125"),
				MarkdownDescription: "UDP `ip:port` the listener binds. `127.0.0.1:8125` (default) accepts local applications only; `0.0.0.0:8125` also accepts containers and other hosts.",
			},
			"statsd_socket": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional Unix datagram socket path (like DogStatsD's DD_DOGSTATSD_SOCKET), e.g. `/var/run/rustinion/dsd.socket`. Linux/macOS/BSD.",
			},
			"flush_interval_secs": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(60),
				MarkdownDescription: "How often aggregated StatsD values are sent (10-3600). Counters are the total per interval, gauges the last value. Default 60.",
				Validators:          []validator.Int64{int64validator.Between(10, 3600)},
			},
			"monitoring_enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the group's basic monitoring is on (read-only). Nothing is stored while it is off.",
			},
			"scrape_targets": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Prometheus/OpenMetrics endpoints each device scrapes (at most 20). Counters are stored as the increase since the previous scrape; histogram buckets are skipped.",
				Validators:          []validator.List{listvalidator.SizeAtMost(20)},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"url": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "http:// or https:// URL, fetched by each device (`http://localhost:9100/metrics` = the node exporter on the device).",
							Validators:          []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^(?i)https?://\S+$`), "must be an http:// or https:// URL")},
						},
						"metrics": schema.ListAttribute{
							Required:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Metric names or patterns to keep (`*` and `?` wildcards), matched against the exposition's family or sample name, e.g. `node_load*`.",
							Validators:          []validator.List{listvalidator.SizeBetween(1, 100)},
						},
						"namespace": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Prefix for the stored names (`<namespace>_<name>`): lowercase letters, digits, underscore, up to 30.",
							Validators:          []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9_]{1,30}$`), "lowercase letters, digits and underscore, up to 30 characters")},
						},
						"interval_secs": schema.Int64Attribute{
							Optional:            true,
							Computed:            true,
							Default:             int64default.StaticInt64(60),
							MarkdownDescription: "Scrape interval, 10-3600 seconds. Default 60.",
							Validators:          []validator.Int64{int64validator.Between(10, 3600)},
						},
						"max_series": schema.Int64Attribute{
							Optional:            true,
							Computed:            true,
							Default:             int64default.StaticInt64(200),
							MarkdownDescription: "Series kept per scrape (1-2000); the rest are counted as skipped on the device page. Default 200.",
							Validators:          []validator.Int64{int64validator.Between(1, 2000)},
						},
						"tags": schema.MapAttribute{
							Optional:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Fixed tags added to every series of this target (label tags win), at most 8.",
						},
					},
				},
			},
		},
	}
}

func (r *groupCustomMetricsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func groupCustomMetricsToClient(ctx context.Context, m groupCustomMetricsModel) (client.GroupCustomMetrics, diag.Diagnostics) {
	var diags diag.Diagnostics
	p := client.DefaultGroupCustomMetrics()
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		p.Enabled = m.Enabled.ValueBool()
	}
	if !m.StatsdEnabled.IsNull() && !m.StatsdEnabled.IsUnknown() {
		p.StatsdEnabled = m.StatsdEnabled.ValueBool()
	}
	if !m.StatsdAddress.IsNull() && !m.StatsdAddress.IsUnknown() {
		p.StatsdAddress = m.StatsdAddress.ValueString()
	}
	if !m.StatsdSocket.IsNull() && !m.StatsdSocket.IsUnknown() {
		s := m.StatsdSocket.ValueString()
		p.StatsdSocket = &s
	}
	if !m.FlushIntervalSecs.IsNull() && !m.FlushIntervalSecs.IsUnknown() {
		p.FlushIntervalSecs = m.FlushIntervalSecs.ValueInt64()
	}
	for _, t := range m.ScrapeTargets {
		st := client.ScrapeTarget{URL: t.URL.ValueString(), IntervalSecs: 60, MaxSeries: 200, Tags: map[string]string{}}
		diags.Append(t.Metrics.ElementsAs(ctx, &st.Metrics, false)...)
		if !t.Namespace.IsNull() && !t.Namespace.IsUnknown() {
			ns := t.Namespace.ValueString()
			st.Namespace = &ns
		}
		if !t.IntervalSecs.IsNull() && !t.IntervalSecs.IsUnknown() {
			st.IntervalSecs = t.IntervalSecs.ValueInt64()
		}
		if !t.MaxSeries.IsNull() && !t.MaxSeries.IsUnknown() {
			st.MaxSeries = t.MaxSeries.ValueInt64()
		}
		if !t.Tags.IsNull() && !t.Tags.IsUnknown() {
			diags.Append(t.Tags.ElementsAs(ctx, &st.Tags, false)...)
		}
		p.ScrapeTargets = append(p.ScrapeTargets, st)
	}
	return p, diags
}

func groupCustomMetricsFromClient(ctx context.Context, groupID string, p *client.GroupCustomMetrics) (groupCustomMetricsModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	m := groupCustomMetricsModel{
		ID:                types.StringValue(groupID),
		GroupID:           types.StringValue(groupID),
		Enabled:           types.BoolValue(p.Enabled),
		StatsdEnabled:     types.BoolValue(p.StatsdEnabled),
		StatsdAddress:     types.StringValue(p.StatsdAddress),
		StatsdSocket:      types.StringNull(),
		FlushIntervalSecs: types.Int64Value(p.FlushIntervalSecs),
		MonitoringEnabled: types.BoolValue(p.MonitoringEnabled),
	}
	if p.StatsdSocket != nil && *p.StatsdSocket != "" {
		m.StatsdSocket = types.StringValue(*p.StatsdSocket)
	}
	// Empty list from the server = attribute not set (keeps plans clean when
	// the configuration omits it).
	for _, t := range p.ScrapeTargets {
		metrics, d := types.ListValueFrom(ctx, types.StringType, t.Metrics)
		diags.Append(d...)
		tm := scrapeTargetModel{
			URL:          types.StringValue(t.URL),
			Metrics:      metrics,
			Namespace:    types.StringNull(),
			IntervalSecs: types.Int64Value(t.IntervalSecs),
			MaxSeries:    types.Int64Value(t.MaxSeries),
			Tags:         types.MapNull(types.StringType),
		}
		if t.Namespace != nil && *t.Namespace != "" {
			tm.Namespace = types.StringValue(*t.Namespace)
		}
		if len(t.Tags) > 0 {
			tags, d := types.MapValueFrom(ctx, types.StringType, t.Tags)
			diags.Append(d...)
			tm.Tags = tags
		}
		m.ScrapeTargets = append(m.ScrapeTargets, tm)
	}
	return m, diags
}

func (r *groupCustomMetricsResource) put(ctx context.Context, m groupCustomMetricsModel) (*groupCustomMetricsModel, diag.Diagnostics) {
	groupID := m.GroupID.ValueString()
	in, diags := groupCustomMetricsToClient(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	saved, err := r.client.PutGroupCustomMetrics(ctx, groupID, in)
	if err != nil {
		diags.AddError("Error saving group custom metrics settings", err.Error())
		return nil, diags
	}
	out, d := groupCustomMetricsFromClient(ctx, groupID, saved)
	diags.Append(d...)
	return &out, diags
}

func (r *groupCustomMetricsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupCustomMetricsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, diags := r.put(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupCustomMetricsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupCustomMetricsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupCustomMetrics(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group custom metrics settings", err.Error())
		return
	}
	m, diags := groupCustomMetricsFromClient(ctx, groupID, p)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *groupCustomMetricsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupCustomMetricsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, diags := r.put(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

// Delete switches collection off and puts the defaults back.
func (r *groupCustomMetricsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupCustomMetricsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupCustomMetrics(ctx, state.GroupID.ValueString(), client.DefaultGroupCustomMetrics())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting group custom metrics settings", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_custom_metrics.apps <group-id>`.
func (r *groupCustomMetricsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
