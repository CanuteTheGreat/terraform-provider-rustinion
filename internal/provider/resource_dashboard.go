package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &dashboardResource{}
	_ resource.ResourceWithConfigure   = &dashboardResource{}
	_ resource.ResourceWithImportState = &dashboardResource{}
)

func NewDashboardResource() resource.Resource {
	return &dashboardResource{}
}

type dashboardResource struct {
	client *client.Client
}

// dashboardWidgetModel is one `widget {}` block. Widgets have no `id`
// attribute exposed for the same reason alert rule conditions don't —
// `PUT` on the parent dashboard deletes and reinserts every widget
// wholesale (see routes::customer_api's own "config-as-code: dashboards"
// doc comment), so a widget's server-assigned id isn't stable across
// updates. Widget order in this list is significant: the server assigns
// each widget's display position from its index here.
type dashboardWidgetModel struct {
	Title      types.String         `tfsdk:"title"`
	WidgetType types.String         `tfsdk:"widget_type"`
	Config     jsontypes.Normalized `tfsdk:"config"`
	Width      types.String         `tfsdk:"width"`
}

type dashboardResourceModel struct {
	ID                   types.String           `tfsdk:"id"`
	GroupID              types.String           `tfsdk:"group_id"`
	Name                 types.String           `tfsdk:"name"`
	DefaultTimeRangeDays types.Int64            `tfsdk:"default_time_range_days"`
	Widget               []dashboardWidgetModel `tfsdk:"widget"`
}

func (r *dashboardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard"
}

func (r *dashboardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion custom dashboard: an ordered list of widgets (metric_chart, kpi_tile, alert_rule_status, or log_search). See dashboards.rs in the main repo for exactly what each widget_type's config expects.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"default_time_range_days": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "The dashboard-wide time range, in days: one of 1, 7, 14, 30 or 90. " +
					"Applies only to widgets whose config sets \"use_dashboard_time_range\": true; every other " +
					"widget keeps its own \"days\". Omit for no shared range.",
			},
		},
		Blocks: map[string]schema.Block{
			"widget": schema.ListNestedBlock{
				MarkdownDescription: "One widget, in display order. Reordering this list and re-applying reorders the dashboard.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"title": schema.StringAttribute{
							Required: true,
						},
						"widget_type": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "One of: metric_chart, kpi_tile, alert_rule_status, log_search.",
						},
						"width": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString("full"),
							MarkdownDescription: "How much of a row this widget occupies: full (the default), half or third.",
						},
						"config": schema.StringAttribute{
							Optional:   true,
							Computed:   true,
							CustomType: jsontypes.NormalizedType{},
							Default:    stringdefault.StaticString("{}"),
							MarkdownDescription: "The widget's config as a JSON string, shape depending on widget_type: " +
								"metric_chart: {\"metric_type\": \"...\", \"days\": 7, \"aggregation\": \"avg\", \"tag_key\": \"...\", \"tag_value\": \"...\"} (aggregation/tag_key/tag_value all optional, tag_key+tag_value both-or-neither, only meaningful for a custom:<name> metric_type); " +
								"kpi_tile: {\"metric_type\": \"...\", \"window_minutes\": 60, \"aggregation\": \"avg\", \"tag_key\": \"...\", \"tag_value\": \"...\"} (same optional fields as metric_chart); " +
								"alert_rule_status: {} (no config); " +
								"log_search: {\"message_pattern\": \"...\", \"severity\": \"error\"|\"warning\", \"days\": 7} (all fields optional).",
						},
					},
				},
			},
		},
	}
}

func (r *dashboardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func dashboardToClient(m dashboardResourceModel) client.Dashboard {
	widgets := make([]client.DashboardWidget, 0, len(m.Widget))
	for _, w := range m.Widget {
		config := w.Config.ValueString()
		if config == "" {
			config = "{}"
		}
		widgets = append(widgets, client.DashboardWidget{
			Title:      w.Title.ValueString(),
			WidgetType: w.WidgetType.ValueString(),
			Config:     json.RawMessage(config),
			Width:      w.Width.ValueString(),
		})
	}
	d := client.Dashboard{
		GroupID: m.GroupID.ValueString(),
		Name:    m.Name.ValueString(),
		Widgets: widgets,
	}
	if !m.DefaultTimeRangeDays.IsNull() && !m.DefaultTimeRangeDays.IsUnknown() {
		days := m.DefaultTimeRangeDays.ValueInt64()
		d.DefaultTimeRangeDays = &days
	}
	return d
}

func dashboardFromClient(d *client.Dashboard) dashboardResourceModel {
	widgets := make([]dashboardWidgetModel, 0, len(d.Widgets))
	for _, w := range d.Widgets {
		// An older server that doesn't know about widths returns none;
		// treat that exactly as the schema default so the value never
		// comes back unknown/empty and produce a spurious diff.
		width := w.Width
		if width == "" {
			width = "full"
		}
		widgets = append(widgets, dashboardWidgetModel{
			Title:      types.StringValue(w.Title),
			WidgetType: types.StringValue(w.WidgetType),
			Config:     jsontypes.NewNormalizedValue(string(w.Config)),
			Width:      types.StringValue(width),
		})
	}
	m := dashboardResourceModel{
		ID:      types.StringValue(d.ID),
		GroupID: types.StringValue(d.GroupID),
		Name:    types.StringValue(d.Name),
		Widget:  widgets,
	}
	if d.DefaultTimeRangeDays != nil {
		m.DefaultTimeRangeDays = types.Int64Value(*d.DefaultTimeRangeDays)
	} else {
		m.DefaultTimeRangeDays = types.Int64Null()
	}
	return m
}

func (r *dashboardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dashboardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateDashboard(ctx, dashboardToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating dashboard", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, dashboardFromClient(created))...)
}

func (r *dashboardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dashboardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.GetDashboard(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading dashboard", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, dashboardFromClient(d))...)
}

func (r *dashboardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dashboardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state dashboardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateDashboard(ctx, state.ID.ValueString(), dashboardToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating dashboard", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, dashboardFromClient(updated))...)
}

func (r *dashboardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dashboardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteDashboard(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting dashboard", err.Error())
	}
}

func (r *dashboardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
