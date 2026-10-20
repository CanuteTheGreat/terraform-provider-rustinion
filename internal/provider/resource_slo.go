package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &sloResource{}
	_ resource.ResourceWithConfigure   = &sloResource{}
	_ resource.ResourceWithImportState = &sloResource{}
)

func NewSloResource() resource.Resource {
	return &sloResource{}
}

type sloResource struct {
	client *client.Client
}

// sloResourceModel. Every attribute is `RequiresReplace` — the server
// has no update endpoint for an SLO at all (create/read/delete only,
// matching the admin UI's own `routes::slos::create_slo`/`remove_slo`
// — there has never been an "edit an existing SLO" capability anywhere
// in this app), so this resource's own `Update` is unreachable in
// practice and, like `rustinion_on_call_override`'s, implemented as a
// loud failure instead of silently doing the wrong thing.
type sloResourceModel struct {
	ID               types.String  `tfsdk:"id"`
	GroupID          types.String  `tfsdk:"group_id"`
	Name             types.String  `tfsdk:"name"`
	MetricType       types.String  `tfsdk:"metric_type"`
	TargetPercentage types.Float64 `tfsdk:"target_percentage"`
	WindowDays       types.Int64   `tfsdk:"window_days"`
	FreshnessMinutes types.Int64   `tfsdk:"freshness_minutes"`
	CustomMetricName types.String  `tfsdk:"custom_metric_name"`
	Comparator       types.String  `tfsdk:"comparator"`
	GoodThreshold    types.Float64 `tfsdk:"good_threshold"`
	SliAlertRuleID   types.String  `tfsdk:"sli_alert_rule_id"`
	Enabled          types.Bool    `tfsdk:"enabled"`
}

func (r *sloResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_slo"
}

func (r *sloResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion SLO (service-level objective). See slo.rs in the main repo for attainment/error-budget computation. metric_type selects which fields apply: checkin_freshness/compliance_health/snmp_reachability need none of the optional fields below; custom_metric requires custom_metric_name, comparator, and good_threshold; alert_rule requires sli_alert_rule_id. There is no update endpoint for an SLO at all — every attribute here forces a replace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned SLO ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the group this SLO belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"metric_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of: checkin_freshness, compliance_health, snmp_reachability, custom_metric, alert_rule.",
				Validators: []validator.String{
					stringvalidator.OneOf("checkin_freshness", "compliance_health", "snmp_reachability", "custom_metric", "alert_rule"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target_percentage": schema.Float64Attribute{
				Required:            true,
				MarkdownDescription: "Target attainment percentage, greater than 0 and at most 100 (e.g. 99.9).",
				Validators:          []validator.Float64{float64validator.AtMost(100)},
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.RequiresReplace(),
				},
			},
			"window_days": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(30),
				MarkdownDescription: "Rolling attainment window, in days. Defaults to 30.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"freshness_minutes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(60),
				MarkdownDescription: "Only meaningful for metric_type = \"checkin_freshness\" — how recent a checkin must be to count as fresh. Defaults to 60.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"custom_metric_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Required (and only valid) for metric_type = \"custom_metric\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comparator": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Required (and only valid) for metric_type = \"custom_metric\": gt, gte, lt, lte, or eq.",
				Validators:          []validator.String{stringvalidator.OneOf("gt", "gte", "lt", "lte", "eq")},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"good_threshold": schema.Float64Attribute{
				Optional:            true,
				MarkdownDescription: "Required (and only valid) for metric_type = \"custom_metric\".",
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.RequiresReplace(),
				},
			},
			"sli_alert_rule_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Required (and only valid) for metric_type = \"alert_rule\" — must be an existing alert rule in the same group.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *sloResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func sloToClient(m sloResourceModel) client.Slo {
	s := client.Slo{
		GroupID:          m.GroupID.ValueString(),
		Name:             m.Name.ValueString(),
		MetricType:       m.MetricType.ValueString(),
		TargetPercentage: m.TargetPercentage.ValueFloat64(),
		WindowDays:       m.WindowDays.ValueInt64(),
		FreshnessMinutes: m.FreshnessMinutes.ValueInt64(),
		Enabled:          m.Enabled.ValueBool(),
	}
	if !m.CustomMetricName.IsNull() {
		v := m.CustomMetricName.ValueString()
		s.CustomMetricName = &v
	}
	if !m.Comparator.IsNull() {
		v := m.Comparator.ValueString()
		s.Comparator = &v
	}
	if !m.GoodThreshold.IsNull() {
		v := m.GoodThreshold.ValueFloat64()
		s.GoodThreshold = &v
	}
	if !m.SliAlertRuleID.IsNull() {
		v := m.SliAlertRuleID.ValueString()
		s.SliAlertRuleID = &v
	}
	return s
}

func sloFromClient(s *client.Slo) sloResourceModel {
	return sloResourceModel{
		ID:               types.StringValue(s.ID),
		GroupID:          types.StringValue(s.GroupID),
		Name:             types.StringValue(s.Name),
		MetricType:       types.StringValue(s.MetricType),
		TargetPercentage: types.Float64Value(s.TargetPercentage),
		WindowDays:       types.Int64Value(s.WindowDays),
		FreshnessMinutes: types.Int64Value(s.FreshnessMinutes),
		CustomMetricName: types.StringPointerValue(s.CustomMetricName),
		Comparator:       types.StringPointerValue(s.Comparator),
		GoodThreshold:    types.Float64PointerValue(s.GoodThreshold),
		SliAlertRuleID:   types.StringPointerValue(s.SliAlertRuleID),
		Enabled:          types.BoolValue(s.Enabled),
	}
}

func (r *sloResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sloResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateSlo(ctx, sloToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating slo", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, sloFromClient(created))...)
}

func (r *sloResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sloResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := r.client.GetSlo(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading slo", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, sloFromClient(s))...)
}

// Update is unreachable in practice — every attribute above is
// RequiresReplace, so Terraform always plans a destroy+create instead
// of calling this. See `resource_on_call_override.go`'s own Update for
// the same defensive-failure reasoning.
func (r *sloResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"SLOs cannot be updated in place",
		"Every attribute on rustinion_slo is RequiresReplace; this method should never be invoked. If you see this error, the resource's schema and this method have drifted out of sync.",
	)
}

func (r *sloResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sloResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteSlo(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting slo", err.Error())
	}
}

func (r *sloResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
