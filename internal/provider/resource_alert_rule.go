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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &alertRuleResource{}
	_ resource.ResourceWithConfigure   = &alertRuleResource{}
	_ resource.ResourceWithImportState = &alertRuleResource{}
)

func NewAlertRuleResource() resource.Resource {
	return &alertRuleResource{}
}

type alertRuleResource struct {
	client *client.Client
}

// alertRuleConditionModel is a `condition {}` block. Conditions have no
// `id` attribute exposed here on purpose: `PUT` on the parent alert rule
// is a full replace on the server (conditions are deleted and
// reinserted wholesale — see routes::customer_api's own "config-as-code:
// alert rules" doc comment), so a condition's server-assigned id is not
// stable across updates and would be meaningless to track in Terraform
// state.
type alertRuleConditionModel struct {
	MetricType            types.String  `tfsdk:"metric_type"`
	ConditionKind         types.String  `tfsdk:"condition_kind"`
	Comparator            types.String  `tfsdk:"comparator"`
	Threshold             types.Float64 `tfsdk:"threshold"`
	WindowMinutes         types.Int64   `tfsdk:"window_minutes"`
	AnomalyMethod         types.String  `tfsdk:"anomaly_method"`
	BaselineWindowMinutes types.Int64   `tfsdk:"baseline_window_minutes"`
	ForecastWithinHours   types.Int64   `tfsdk:"forecast_within_hours"`
	Aggregation           types.String  `tfsdk:"aggregation"`
	TagKey                types.String  `tfsdk:"tag_key"`
	TagValue              types.String  `tfsdk:"tag_value"`
	CompositeRuleID       types.String  `tfsdk:"composite_rule_id"`
	CompositeExpectFiring types.Bool    `tfsdk:"composite_expect_firing"`
	MwLongWindowMinutes   types.Int64   `tfsdk:"mw_long_window_minutes"`
	MwLongThreshold       types.Float64 `tfsdk:"mw_long_threshold"`
}

type alertRuleResourceModel struct {
	ID                 types.String              `tfsdk:"id"`
	GroupID            types.String              `tfsdk:"group_id"`
	Name               types.String              `tfsdk:"name"`
	BoolOp             types.String              `tfsdk:"bool_op"`
	Severity           types.String              `tfsdk:"severity"`
	BreachesToFire     types.Int64               `tfsdk:"breaches_to_fire"`
	ClearsToResolve    types.Int64               `tfsdk:"clears_to_resolve"`
	Enabled            types.Bool                `tfsdk:"enabled"`
	EscalationPolicyID types.String              `tfsdk:"escalation_policy_id"`
	Condition          []alertRuleConditionModel `tfsdk:"condition"`
}

func (r *alertRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_rule"
}

func (r *alertRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion alert rule: an AND/OR combination of metric conditions, evaluated on a schedule with hysteresis. See web/src/alerting.rs in the main repo for full evaluation semantics.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned alert rule ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the group this alert rule belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"bool_op": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("and"),
				MarkdownDescription: "Combine conditions with \"and\" (every condition must breach) or \"or\" (any one does). Defaults to \"and\".",
				Validators:          []validator.String{stringvalidator.OneOf("and", "or")},
			},
			"severity": schema.StringAttribute{
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString("critical"),
				Validators: []validator.String{stringvalidator.OneOf("critical", "warning")},
			},
			"breaches_to_fire": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(3),
				MarkdownDescription: "Consecutive breaching evaluations before the rule fires. Defaults to 3.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"clears_to_resolve": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(2),
				MarkdownDescription: "Consecutive healthy evaluations before a firing rule clears. Defaults to 2.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"escalation_policy_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Escalation policy to notify when this rule fires, in the same group. Omit for none.",
			},
		},
		Blocks: map[string]schema.Block{
			"condition": schema.ListNestedBlock{
				MarkdownDescription: "One metric condition. A rule with no conditions never breaches.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"metric_type": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "A builtin metric type (e.g. \"cpu_avg_pct\"), \"custom:<name>\", \"slo_burn_rate:<uuid>\", \"rum:<app_id>:<vital>\" (e.g. \"rum:<uuid>:LCP\"), \"k8s_event_count:<reason>\" (e.g. \"k8s_event_count:OOMKilled\"), \"new_error_groups:<service_name>\", \"error_rate:<service_name>:<operation_name>\", or (computed server-side, sent as any placeholder for a composite condition) \"composite:<uuid>\".",
						},
						"condition_kind": schema.StringAttribute{
							Optional:   true,
							Computed:   true,
							Default:    stringdefault.StaticString("threshold"),
							Validators: []validator.String{stringvalidator.OneOf("threshold", "anomaly", "forecast", "composite", "slo_burn_rate_multi_window")},
						},
						"comparator": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "For a threshold or forecast condition: gt, gte, lt, lte, or eq (eq is not valid for forecast). For an anomaly condition, only gt/lt/eq are valid (above/below/either direction from baseline). For a composite condition this attribute is required by the schema but not meaningful server-side — use \"eq\", matching the admin UI's own convention.",
							Validators:          []validator.String{stringvalidator.OneOf("gt", "gte", "lt", "lte", "eq")},
						},
						"threshold": schema.Float64Attribute{
							Required:            true,
							MarkdownDescription: "The threshold value, or (for an anomaly condition) the sensitivity in standard deviations from the baseline. For a composite condition this attribute is required by the schema but not meaningful server-side — use 0, matching the admin UI's own convention.",
							Validators:          []validator.Float64{float64validator.AtLeast(0)},
						},
						"window_minutes": schema.Int64Attribute{
							Optional: true,
							Computed: true,
							Default:  int64default.StaticInt64(60),
						},
						"anomaly_method": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Required for an anomaly condition: \"rolling_stddev\" or \"seasonal\". Unused for a threshold or forecast condition.",
							Validators:          []validator.String{stringvalidator.OneOf("rolling_stddev", "seasonal")},
						},
						"baseline_window_minutes": schema.Int64Attribute{
							Optional:            true,
							MarkdownDescription: "Required for an anomaly condition: how far back its baseline is computed from, in minutes.",
						},
						"forecast_within_hours": schema.Int64Attribute{
							Optional:            true,
							MarkdownDescription: "Required for a forecast condition: how soon (in hours) the metric's current linear trend must be projected to cross `threshold` for this condition to breach. Datadog calls this shape a \"Forecast Monitor\" — alerts before a threshold is actually crossed, not just after.",
							Validators:          []validator.Int64{int64validator.AtLeast(1)},
						},
						"aggregation": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString("avg"),
							MarkdownDescription: "How to combine this metric's values across every device in the group before comparing it to threshold: \"avg\" (default), \"sum\", \"min\", \"max\", or a percentile (\"p50\", \"p90\", \"p95\", \"p99\"). Only meaningful for a `custom:<name>` metric_type.",
							Validators: []validator.String{
								stringvalidator.OneOf("avg", "sum", "min", "max", "p50", "p90", "p95", "p99"),
							},
						},
						"tag_key": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Narrow this condition to only the samples carrying this exact tag key=value pair (see `tag_value`). Must be given together with `tag_value`, or omitted along with it — the server rejects one without the other. Only meaningful for a `custom:<name>` metric_type; has no effect on a builtin metric or an SLO burn rate condition.",
						},
						"tag_value": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "The tag value to match, paired with `tag_key` — see that attribute's own description.",
						},
						"composite_rule_id": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Required for a composite condition: the id of another `rustinion_alert_rule` in the same tenant (e.g. `rustinion_alert_rule.other.id`) whose current firing state this condition reads instead of a metric. Must not be this rule's own id, and must belong to this same tenant — both enforced server-side.",
						},
						"composite_expect_firing": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "Only valid for a composite condition: `true` (the server's own default when omitted) breaches when the referenced rule IS firing; `false` breaches when it is NOT firing. Deliberately not `Computed`/defaulted here (unlike most other attributes in this schema) — a computed default would mean Terraform always sends *some* value for it on every condition, including non-composite ones, which the server rejects outright (`composite_rule_id/composite_expect_firing are only valid for condition_kind = \"composite\"`); leaving it purely `Optional` means it's only ever sent when the config actually sets it.",
						},
						"mw_long_window_minutes": schema.Int64Attribute{
							Optional:            true,
							MarkdownDescription: "Required for a slo_burn_rate_multi_window condition: the LONG window's own lookback, in minutes (e.g. 360 for 6h). Paired with window_minutes above, which is reused as the SHORT window's own lookback — fires only when BOTH windows independently breach their own threshold (threshold above is the short window's threshold; mw_long_threshold below is the long window's).",
							Validators:          []validator.Int64{int64validator.AtLeast(1)},
						},
						"mw_long_threshold": schema.Float64Attribute{
							Optional:            true,
							MarkdownDescription: "Required for a slo_burn_rate_multi_window condition: the LONG window's own burn-rate threshold (e.g. 6 for a 6h/6x pairing with a 1h/14.4x short window).",
						},
					},
				},
			},
		},
	}
}

func (r *alertRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func alertRuleToClient(m alertRuleResourceModel) client.AlertRule {
	conditions := make([]client.AlertRuleCondition, 0, len(m.Condition))
	for _, c := range m.Condition {
		cond := client.AlertRuleCondition{
			MetricType:    c.MetricType.ValueString(),
			ConditionKind: c.ConditionKind.ValueString(),
			Comparator:    c.Comparator.ValueString(),
			Threshold:     c.Threshold.ValueFloat64(),
			WindowMinutes: c.WindowMinutes.ValueInt64(),
			Aggregation:   c.Aggregation.ValueString(),
		}
		if !c.AnomalyMethod.IsNull() {
			v := c.AnomalyMethod.ValueString()
			cond.AnomalyMethod = &v
		}
		if !c.BaselineWindowMinutes.IsNull() {
			v := c.BaselineWindowMinutes.ValueInt64()
			cond.BaselineWindowMinutes = &v
		}
		if !c.ForecastWithinHours.IsNull() {
			v := c.ForecastWithinHours.ValueInt64()
			cond.ForecastWithinHours = &v
		}
		if !c.TagKey.IsNull() {
			v := c.TagKey.ValueString()
			cond.TagKey = &v
		}
		if !c.TagValue.IsNull() {
			v := c.TagValue.ValueString()
			cond.TagValue = &v
		}
		if !c.CompositeRuleID.IsNull() {
			v := c.CompositeRuleID.ValueString()
			cond.CompositeRuleID = &v
		}
		if !c.CompositeExpectFiring.IsNull() {
			v := c.CompositeExpectFiring.ValueBool()
			cond.CompositeExpectFiring = &v
		}
		if !c.MwLongWindowMinutes.IsNull() {
			v := c.MwLongWindowMinutes.ValueInt64()
			cond.MwLongWindowMinutes = &v
		}
		if !c.MwLongThreshold.IsNull() {
			v := c.MwLongThreshold.ValueFloat64()
			cond.MwLongThreshold = &v
		}
		conditions = append(conditions, cond)
	}
	rule := client.AlertRule{
		GroupID:         m.GroupID.ValueString(),
		Name:            m.Name.ValueString(),
		BoolOp:          m.BoolOp.ValueString(),
		Severity:        m.Severity.ValueString(),
		BreachesToFire:  m.BreachesToFire.ValueInt64(),
		ClearsToResolve: m.ClearsToResolve.ValueInt64(),
		Enabled:         m.Enabled.ValueBool(),
		Conditions:      conditions,
	}
	if !m.EscalationPolicyID.IsNull() {
		v := m.EscalationPolicyID.ValueString()
		rule.EscalationPolicyID = &v
	}
	return rule
}

// preserveCompositeMetricType overwrites `result`'s own `metric_type`
// for every composite condition with the corresponding condition's
// value from `previous` (the plan being applied, or the prior state),
// matched by list index (conditions are order-preserving — the server's
// own full-replace PUT/POST semantics store and return them in exactly
// the order submitted, the same positional correspondence this
// resource's own tests already rely on elsewhere, e.g.
// `condition.0.metric_type`).
//
// `metric_type` is `Required` (not `Computed`) in the schema, since a
// customer really must supply one for a threshold/anomaly/forecast
// condition — but for a composite condition, the server always computes
// and returns its own real `composite:<uuid>` value, ignoring whatever
// placeholder was submitted (see `routes::customer_api::resolve_
// condition`'s own composite branch in the main repo, which never reads
// the input's own `metric_type` at all). Terraform requires every
// non-Computed attribute's post-apply value to equal the plan's exactly
// — echoing the server's own real value back here would otherwise
// surface as "Provider produced inconsistent result after apply" on
// literally every composite condition, confirmed by a real failing
// acceptance-test run before this fix existed. Preserving the
// configured placeholder instead keeps the attribute genuinely
// `Required`-and-plan-consistent while placing zero real meaning on its
// actual string value for this one condition kind.
func preserveCompositeMetricType(result *alertRuleResourceModel, previous []alertRuleConditionModel) {
	for i := range result.Condition {
		if i >= len(previous) {
			break
		}
		if result.Condition[i].ConditionKind.ValueString() == "composite" {
			result.Condition[i].MetricType = previous[i].MetricType
		}
	}
}

func alertRuleFromClient(rule *client.AlertRule) alertRuleResourceModel {
	conditions := make([]alertRuleConditionModel, 0, len(rule.Conditions))
	for _, c := range rule.Conditions {
		conditions = append(conditions, alertRuleConditionModel{
			MetricType:            types.StringValue(c.MetricType),
			ConditionKind:         types.StringValue(c.ConditionKind),
			Comparator:            types.StringValue(c.Comparator),
			Threshold:             types.Float64Value(c.Threshold),
			WindowMinutes:         types.Int64Value(c.WindowMinutes),
			AnomalyMethod:         types.StringPointerValue(c.AnomalyMethod),
			BaselineWindowMinutes: types.Int64PointerValue(c.BaselineWindowMinutes),
			ForecastWithinHours:   types.Int64PointerValue(c.ForecastWithinHours),
			Aggregation:           types.StringValue(c.Aggregation),
			TagKey:                types.StringPointerValue(c.TagKey),
			TagValue:              types.StringPointerValue(c.TagValue),
			CompositeRuleID:       types.StringPointerValue(c.CompositeRuleID),
			CompositeExpectFiring: types.BoolPointerValue(c.CompositeExpectFiring),
			MwLongWindowMinutes:   types.Int64PointerValue(c.MwLongWindowMinutes),
			MwLongThreshold:       types.Float64PointerValue(c.MwLongThreshold),
		})
	}
	return alertRuleResourceModel{
		ID:                 types.StringValue(rule.ID),
		GroupID:            types.StringValue(rule.GroupID),
		Name:               types.StringValue(rule.Name),
		BoolOp:             types.StringValue(rule.BoolOp),
		Severity:           types.StringValue(rule.Severity),
		BreachesToFire:     types.Int64Value(rule.BreachesToFire),
		ClearsToResolve:    types.Int64Value(rule.ClearsToResolve),
		Enabled:            types.BoolValue(rule.Enabled),
		EscalationPolicyID: types.StringPointerValue(rule.EscalationPolicyID),
		Condition:          conditions,
	}
}

func (r *alertRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateAlertRule(ctx, alertRuleToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating alert rule", err.Error())
		return
	}
	result := alertRuleFromClient(created)
	preserveCompositeMetricType(&result, plan.Condition)
	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}

func (r *alertRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.client.GetAlertRule(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading alert rule", err.Error())
		return
	}
	result := alertRuleFromClient(rule)
	preserveCompositeMetricType(&result, state.Condition)
	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}

func (r *alertRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan alertRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state alertRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateAlertRule(ctx, state.ID.ValueString(), alertRuleToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating alert rule", err.Error())
		return
	}
	result := alertRuleFromClient(updated)
	preserveCompositeMetricType(&result, plan.Condition)
	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}

func (r *alertRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteAlertRule(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting alert rule", err.Error())
	}
}

func (r *alertRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
