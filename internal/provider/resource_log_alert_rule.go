package provider

import (
	"context"
	"fmt"

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
	_ resource.Resource                = &logAlertRuleResource{}
	_ resource.ResourceWithConfigure   = &logAlertRuleResource{}
	_ resource.ResourceWithImportState = &logAlertRuleResource{}
)

func NewLogAlertRuleResource() resource.Resource {
	return &logAlertRuleResource{}
}

type logAlertRuleResource struct {
	client *client.Client
}

// logAlertRuleResourceModel — same group-scoped shape as an alert rule,
// but with no nested conditions sub-table: every filter field lives
// directly on the row (see `client.LogAlertRule`'s own doc comment).
type logAlertRuleResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	GroupID            types.String `tfsdk:"group_id"`
	Name               types.String `tfsdk:"name"`
	MessagePattern     types.String `tfsdk:"message_pattern"`
	LogSeverity        types.String `tfsdk:"log_severity"`
	LogSource          types.String `tfsdk:"log_source"`
	UnitPattern        types.String `tfsdk:"unit_pattern"`
	ThresholdCount     types.Int64  `tfsdk:"threshold_count"`
	WindowMinutes      types.Int64  `tfsdk:"window_minutes"`
	Severity           types.String `tfsdk:"severity"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	EscalationPolicyID types.String `tfsdk:"escalation_policy_id"`
}

func (r *logAlertRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_alert_rule"
}

func (r *logAlertRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion log-based alert rule: fires when at least threshold_count matching log lines land within window_minutes. See log_alerts.rs in the main repo for evaluation semantics; a blank message_pattern/log_severity/log_source is treated as no filter on that field, matching the admin UI's own form.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned log alert rule ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the group this log alert rule belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"message_pattern": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Substring/pattern a log line's message must contain to match. Omit for no message filter.",
			},
			"log_severity": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Match log lines at this severity or worse: emergency, alert, critical, error, warning, notice, info, debug. Omit for no severity filter.",
				Validators:          []validator.String{stringvalidator.OneOf("emergency", "alert", "critical", "error", "warning", "notice", "info", "debug")},
			},
			"log_source": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Restrict matching to one source: journal, kernel, windows_event, macos_unified, syslog, file, container, otlp, datadog, datadog-service-check, datadog-event, panic. Omit for no source filter.",
				Validators:          []validator.String{stringvalidator.OneOf("journal", "kernel", "windows_event", "macos_unified", "syslog", "file", "container", "otlp", "datadog", "datadog-service-check", "datadog-event", "panic")},
			},
			"unit_pattern": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Restrict matching to log lines from a service/unit whose name contains this. Omit for no unit filter.",
			},
			"threshold_count": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
				MarkdownDescription: "Matching log lines required within window_minutes before the rule fires. Defaults to 1.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"window_minutes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(5),
				MarkdownDescription: "Trailing window, in minutes, threshold_count is evaluated over. Defaults to 5.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"severity": schema.StringAttribute{
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString("critical"),
				Validators: []validator.String{stringvalidator.OneOf("critical", "warning")},
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
	}
}

func (r *logAlertRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func logAlertRuleToClient(m logAlertRuleResourceModel) client.LogAlertRule {
	rule := client.LogAlertRule{
		GroupID:        m.GroupID.ValueString(),
		Name:           m.Name.ValueString(),
		ThresholdCount: m.ThresholdCount.ValueInt64(),
		WindowMinutes:  m.WindowMinutes.ValueInt64(),
		Severity:       m.Severity.ValueString(),
		Enabled:        m.Enabled.ValueBool(),
	}
	if !m.MessagePattern.IsNull() {
		v := m.MessagePattern.ValueString()
		rule.MessagePattern = &v
	}
	if !m.LogSeverity.IsNull() {
		v := m.LogSeverity.ValueString()
		rule.LogSeverity = &v
	}
	if !m.LogSource.IsNull() {
		v := m.LogSource.ValueString()
		rule.LogSource = &v
	}
	if !m.UnitPattern.IsNull() {
		v := m.UnitPattern.ValueString()
		rule.UnitPattern = &v
	}
	if !m.EscalationPolicyID.IsNull() {
		v := m.EscalationPolicyID.ValueString()
		rule.EscalationPolicyID = &v
	}
	return rule
}

func logAlertRuleFromClient(rule *client.LogAlertRule) logAlertRuleResourceModel {
	return logAlertRuleResourceModel{
		ID:                 types.StringValue(rule.ID),
		GroupID:            types.StringValue(rule.GroupID),
		Name:               types.StringValue(rule.Name),
		MessagePattern:     types.StringPointerValue(rule.MessagePattern),
		LogSeverity:        types.StringPointerValue(rule.LogSeverity),
		LogSource:          types.StringPointerValue(rule.LogSource),
		UnitPattern:        types.StringPointerValue(rule.UnitPattern),
		ThresholdCount:     types.Int64Value(rule.ThresholdCount),
		WindowMinutes:      types.Int64Value(rule.WindowMinutes),
		Severity:           types.StringValue(rule.Severity),
		Enabled:            types.BoolValue(rule.Enabled),
		EscalationPolicyID: types.StringPointerValue(rule.EscalationPolicyID),
	}
}

func (r *logAlertRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan logAlertRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateLogAlertRule(ctx, logAlertRuleToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating log alert rule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logAlertRuleFromClient(created))...)
}

func (r *logAlertRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state logAlertRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.client.GetLogAlertRule(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading log alert rule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logAlertRuleFromClient(rule))...)
}

func (r *logAlertRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan logAlertRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state logAlertRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateLogAlertRule(ctx, state.ID.ValueString(), logAlertRuleToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating log alert rule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logAlertRuleFromClient(updated))...)
}

func (r *logAlertRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state logAlertRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteLogAlertRule(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting log alert rule", err.Error())
	}
}

func (r *logAlertRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
