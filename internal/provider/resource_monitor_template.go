package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
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

// Monitoring templates (web-alerting `monitor_templates` in the main
// repo): rustinion_monitor_template (an organization's own template:
// rules, macro defaults, parents), rustinion_monitor_template_link
// (apply a template, own or built-in, to a group, a device or the whole
// organization), rustinion_monitor_macro_override (a macro value on a
// device, group or organization) and the rustinion_monitor_template
// data source (look up a built-in pack such as "linux" by key).

func monitorConfigureClient(providerData any, diags interface {
	AddError(string, string)
}) *client.Client {
	if providerData == nil {
		return nil
	}
	c, ok := providerData.(*client.Client)
	if !ok {
		diags.AddError("Unexpected Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", providerData))
		return nil
	}
	return c
}

// ------------------------------------------------------------ template

var (
	_ resource.Resource                = &monitorTemplateResource{}
	_ resource.ResourceWithConfigure   = &monitorTemplateResource{}
	_ resource.ResourceWithImportState = &monitorTemplateResource{}
)

func NewMonitorTemplateResource() resource.Resource { return &monitorTemplateResource{} }

type monitorTemplateResource struct{ client *client.Client }

type monitorTemplateMacroModel struct {
	Macro       types.String `tfsdk:"macro"`
	Context     types.String `tfsdk:"context"`
	Value       types.String `tfsdk:"value"`
	Description types.String `tfsdk:"description"`
}

type monitorTemplateRuleModel struct {
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Severity        types.String `tfsdk:"severity"`
	ItemKey         types.String `tfsdk:"item_key"`
	Func            types.String `tfsdk:"func"`
	WindowMinutes   types.Int64  `tfsdk:"window_minutes"`
	Comparator      types.String `tfsdk:"comparator"`
	Threshold       types.String `tfsdk:"threshold"`
	BreachesToFire  types.Int64  `tfsdk:"breaches_to_fire"`
	ClearsToResolve types.Int64  `tfsdk:"clears_to_resolve"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	DeferToBuiltin  types.String `tfsdk:"defer_to_builtin"`
}

type monitorTemplateResourceModel struct {
	ID          types.String                `tfsdk:"id"`
	Name        types.String                `tfsdk:"name"`
	Description types.String                `tfsdk:"description"`
	ParentIDs   []types.String              `tfsdk:"parent_ids"`
	Macro       []monitorTemplateMacroModel `tfsdk:"macro"`
	Rule        []monitorTemplateRuleModel  `tfsdk:"rule"`
}

func (r *monitorTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_template"
}

func (r *monitorTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a monitoring template: rules (Zabbix-style triggers such as last(vfs.fs.size[*,pused]) > {$VFS.FS.PUSED.MAX.CRIT}) evaluated on every device the template is linked to, one alert state per device, plus macro defaults and parent templates whose rules and macros it inherits. Built-in packs are read-only; reference them with the rustinion_monitor_template data source.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(""),
			},
			"parent_ids": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Templates (own or built-in) this one inherits rules and macro defaults from. Its own macro defaults beat its parents'.",
			},
		},
		Blocks: map[string]schema.Block{
			"macro": schema.ListNestedBlock{
				MarkdownDescription: "A user macro default, e.g. {$DISK_CRIT} = 90. Groups, devices and the organization can override it (rustinion_monitor_macro_override).",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"macro": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Macro name in Zabbix syntax: {$NAME} with A-Z, 0-9, _ and .",
						},
						"context": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Optional context, e.g. \"/data\" for {$VFS.FS.PUSED.MAX.CRIT:\"/data\"}.",
						},
						"value": schema.StringAttribute{Required: true},
						"description": schema.StringAttribute{
							Optional: true,
							Computed: true,
							Default:  stringdefault.StaticString(""),
						},
					},
				},
			},
			"rule": schema.ListNestedBlock{
				MarkdownDescription: "A rule: func(item_key, window) comparator threshold. Rules keep their per-device state across updates as long as their name stays the same.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{Required: true},
						"description": schema.StringAttribute{
							Optional: true,
							Computed: true,
							Default:  stringdefault.StaticString(""),
						},
						"severity": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString("warning"),
							MarkdownDescription: "\"warning\" (summary email) or \"critical\" (sent right away).",
							Validators:          []validator.String{stringvalidator.OneOf("warning", "critical")},
						},
						"item_key": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Item key, Zabbix syntax: system.cpu.util, vm.memory.utilization, vfs.fs.size[<path>|*,pused|pfree], vfs.fs.inode[<path>|*,pfree], system.swap.size[,pused], system.cpu.load[percpu,avg15], system.pressure[memory,full], service.failed[<pattern>], proc.num[<name>], docker.containers[unhealthy|restarting|running|stopped|all], agent.ping, system.uptime, clock.synchronized. Macros are allowed in the parameters.",
						},
						"func": schema.StringAttribute{
							Optional:   true,
							Computed:   true,
							Default:    stringdefault.StaticString("last"),
							Validators: []validator.String{stringvalidator.OneOf("last", "avg", "min", "max", "nodata")},
						},
						"window_minutes": schema.Int64Attribute{
							Optional:   true,
							Computed:   true,
							Default:    int64default.StaticInt64(5),
							Validators: []validator.Int64{int64validator.Between(1, 10080)},
						},
						"comparator": schema.StringAttribute{
							Required:   true,
							Validators: []validator.String{stringvalidator.OneOf("gt", "gte", "lt", "lte", "eq", "ne")},
						},
						"threshold": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "A number or a macro, e.g. \"90\" or \"{$DISK_CRIT}\".",
						},
						"breaches_to_fire": schema.Int64Attribute{
							Optional:   true,
							Computed:   true,
							Default:    int64default.StaticInt64(1),
							Validators: []validator.Int64{int64validator.Between(1, 1440)},
						},
						"clears_to_resolve": schema.Int64Attribute{
							Optional:   true,
							Computed:   true,
							Default:    int64default.StaticInt64(1),
							Validators: []validator.Int64{int64validator.Between(1, 1440)},
						},
						"enabled": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(true),
						},
						"defer_to_builtin": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "A built-in alert this rule duplicates (cpu_high, memory_high, disk_full, mount_full, inodes_low, swap_in_use, service_failed, clock_unsynced, device_unreachable): while that alert is on for a device, the rule doesn't notify a second time.",
						},
					},
				},
			},
		},
	}
}

func (r *monitorTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := monitorConfigureClient(req.ProviderData, &resp.Diagnostics); c != nil {
		r.client = c
	}
}

func optString(v types.String) *string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return nil
	}
	s := v.ValueString()
	return &s
}

func strOrNull(p *string) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// monitorTemplateToInput builds the request; ruleIDs (name -> id) keeps
// existing rules' ids so their per-device state survives the update.
func monitorTemplateToInput(m monitorTemplateResourceModel, ruleIDs map[string]string) client.MonitorTemplateInput {
	in := client.MonitorTemplateInput{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueString(),
		ParentIDs:   []string{},
		Macros:      []client.MonitorTemplateMacro{},
		Rules:       []client.MonitorTemplateRule{},
	}
	for _, p := range m.ParentIDs {
		in.ParentIDs = append(in.ParentIDs, p.ValueString())
	}
	for _, mm := range m.Macro {
		in.Macros = append(in.Macros, client.MonitorTemplateMacro{
			Macro:       mm.Macro.ValueString(),
			Context:     optString(mm.Context),
			Value:       mm.Value.ValueString(),
			Description: mm.Description.ValueString(),
		})
	}
	for _, rr := range m.Rule {
		rule := client.MonitorTemplateRule{
			Name:            rr.Name.ValueString(),
			Description:     rr.Description.ValueString(),
			Severity:        rr.Severity.ValueString(),
			ItemKey:         rr.ItemKey.ValueString(),
			Func:            rr.Func.ValueString(),
			WindowMinutes:   rr.WindowMinutes.ValueInt64(),
			Comparator:      rr.Comparator.ValueString(),
			Threshold:       rr.Threshold.ValueString(),
			BreachesToFire:  rr.BreachesToFire.ValueInt64(),
			ClearsToResolve: rr.ClearsToResolve.ValueInt64(),
			Enabled:         rr.Enabled.ValueBool(),
			DeferToBuiltin:  optString(rr.DeferToBuiltin),
		}
		if id, ok := ruleIDs[rule.Name]; ok {
			idCopy := id
			rule.ID = &idCopy
		}
		in.Rules = append(in.Rules, rule)
	}
	return in
}

func monitorTemplateFromClient(t *client.MonitorTemplate, prior *monitorTemplateResourceModel) monitorTemplateResourceModel {
	m := monitorTemplateResourceModel{
		ID:          types.StringValue(t.ID),
		Name:        types.StringValue(t.Name),
		Description: types.StringValue(t.Description),
	}
	// parent_ids: keep the configured order when the set matches.
	if len(t.Parents) > 0 {
		server := map[string]bool{}
		for _, p := range t.Parents {
			server[p.ID] = true
		}
		same := prior != nil && len(prior.ParentIDs) == len(t.Parents)
		if same {
			for _, p := range prior.ParentIDs {
				if !server[p.ValueString()] {
					same = false
				}
			}
		}
		if same {
			m.ParentIDs = prior.ParentIDs
		} else {
			for _, p := range t.Parents {
				m.ParentIDs = append(m.ParentIDs, types.StringValue(p.ID))
			}
		}
	} else if prior != nil && prior.ParentIDs != nil && len(prior.ParentIDs) == 0 {
		m.ParentIDs = []types.String{}
	}
	for _, mm := range t.Macros {
		m.Macro = append(m.Macro, monitorTemplateMacroModel{
			Macro:       types.StringValue(mm.Macro),
			Context:     strOrNull(mm.Context),
			Value:       types.StringValue(mm.Value),
			Description: types.StringValue(mm.Description),
		})
	}
	for _, rr := range t.Rules {
		m.Rule = append(m.Rule, monitorTemplateRuleModel{
			Name:            types.StringValue(rr.Name),
			Description:     types.StringValue(rr.Description),
			Severity:        types.StringValue(rr.Severity),
			ItemKey:         types.StringValue(rr.ItemKey),
			Func:            types.StringValue(rr.Func),
			WindowMinutes:   types.Int64Value(rr.WindowMinutes),
			Comparator:      types.StringValue(rr.Comparator),
			Threshold:       types.StringValue(rr.Threshold),
			BreachesToFire:  types.Int64Value(rr.BreachesToFire),
			ClearsToResolve: types.Int64Value(rr.ClearsToResolve),
			Enabled:         types.BoolValue(rr.Enabled),
			DeferToBuiltin:  strOrNull(rr.DeferToBuiltin),
		})
	}
	return m
}

func (r *monitorTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateMonitorTemplate(ctx, monitorTemplateToInput(plan, nil))
	if err != nil {
		resp.Diagnostics.AddError("Error creating monitoring template", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, monitorTemplateFromClient(created, &plan))...)
}

func (r *monitorTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.GetMonitorTemplate(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading monitoring template", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, monitorTemplateFromClient(t, &state))...)
}

func (r *monitorTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state monitorTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ruleIDs := map[string]string{}
	if current, err := r.client.GetMonitorTemplate(ctx, state.ID.ValueString()); err == nil {
		for _, rr := range current.Rules {
			if rr.ID != nil {
				ruleIDs[rr.Name] = *rr.ID
			}
		}
	}
	updated, err := r.client.UpdateMonitorTemplate(ctx, state.ID.ValueString(), monitorTemplateToInput(plan, ruleIDs))
	if err != nil {
		resp.Diagnostics.AddError("Error updating monitoring template", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, monitorTemplateFromClient(updated, &plan))...)
}

func (r *monitorTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMonitorTemplate(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting monitoring template", err.Error())
	}
}

func (r *monitorTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------- link

var (
	_ resource.Resource                = &monitorTemplateLinkResource{}
	_ resource.ResourceWithConfigure   = &monitorTemplateLinkResource{}
	_ resource.ResourceWithImportState = &monitorTemplateLinkResource{}
)

func NewMonitorTemplateLinkResource() resource.Resource { return &monitorTemplateLinkResource{} }

type monitorTemplateLinkResource struct{ client *client.Client }

type monitorTemplateLinkModel struct {
	ID         types.String `tfsdk:"id"`
	TemplateID types.String `tfsdk:"template_id"`
	GroupID    types.String `tfsdk:"group_id"`
	DeviceID   types.String `tfsdk:"device_id"`
}

func (r *monitorTemplateLinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_template_link"
}

func (r *monitorTemplateLinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Applies a monitoring template (own or built-in) to a group, a device, or (neither set) every device of the organization. Removing the link removes the rule state of the devices it no longer covers.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"template_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"group_id":    schema.StringAttribute{Optional: true, PlanModifiers: replace},
			"device_id":   schema.StringAttribute{Optional: true, PlanModifiers: replace},
		},
	}
}

func (r *monitorTemplateLinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := monitorConfigureClient(req.ProviderData, &resp.Diagnostics); c != nil {
		r.client = c
	}
}

func linkFromClient(l *client.MonitorTemplateLink) monitorTemplateLinkModel {
	return monitorTemplateLinkModel{
		ID:         types.StringValue(l.ID),
		TemplateID: types.StringValue(l.TemplateID),
		GroupID:    strOrNull(l.GroupID),
		DeviceID:   strOrNull(l.DeviceID),
	}
}

func (r *monitorTemplateLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorTemplateLinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateMonitorTemplateLink(ctx, client.MonitorTemplateLink{
		TemplateID: plan.TemplateID.ValueString(),
		GroupID:    optString(plan.GroupID),
		DeviceID:   optString(plan.DeviceID),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error linking monitoring template", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, linkFromClient(created))...)
}

func (r *monitorTemplateLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorTemplateLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	l, err := r.client.GetMonitorTemplateLink(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading monitoring template link", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, linkFromClient(l))...)
}

// Update is never reached: every attribute requires replacement.
func (r *monitorTemplateLinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan monitorTemplateLinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *monitorTemplateLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorTemplateLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMonitorTemplateLink(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error unlinking monitoring template", err.Error())
	}
}

func (r *monitorTemplateLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ------------------------------------------------------- macro override

var (
	_ resource.Resource                = &monitorMacroOverrideResource{}
	_ resource.ResourceWithConfigure   = &monitorMacroOverrideResource{}
	_ resource.ResourceWithImportState = &monitorMacroOverrideResource{}
)

func NewMonitorMacroOverrideResource() resource.Resource { return &monitorMacroOverrideResource{} }

type monitorMacroOverrideResource struct{ client *client.Client }

type monitorMacroOverrideModel struct {
	ID       types.String `tfsdk:"id"`
	Macro    types.String `tfsdk:"macro"`
	Context  types.String `tfsdk:"context"`
	Value    types.String `tfsdk:"value"`
	GroupID  types.String `tfsdk:"group_id"`
	DeviceID types.String `tfsdk:"device_id"`
}

func (r *monitorMacroOverrideResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_macro_override"
}

func (r *monitorMacroOverrideResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Sets a monitoring-template macro on a device, a group, or (neither set) the whole organization. Device beats group beats organization beats the templates' defaults.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"macro":     schema.StringAttribute{Required: true, PlanModifiers: replace},
			"context":   schema.StringAttribute{Optional: true, PlanModifiers: replace},
			"value":     schema.StringAttribute{Required: true},
			"group_id":  schema.StringAttribute{Optional: true, PlanModifiers: replace},
			"device_id": schema.StringAttribute{Optional: true, PlanModifiers: replace},
		},
	}
}

func (r *monitorMacroOverrideResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := monitorConfigureClient(req.ProviderData, &resp.Diagnostics); c != nil {
		r.client = c
	}
}

func overrideFromClient(o *client.MonitorMacroOverride) monitorMacroOverrideModel {
	return monitorMacroOverrideModel{
		ID:       types.StringValue(o.ID),
		Macro:    types.StringValue(o.Macro),
		Context:  strOrNull(o.Context),
		Value:    types.StringValue(o.Value),
		GroupID:  strOrNull(o.GroupID),
		DeviceID: strOrNull(o.DeviceID),
	}
}

func (r *monitorMacroOverrideResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorMacroOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateMonitorMacroOverride(ctx, client.MonitorMacroOverride{
		Macro:    plan.Macro.ValueString(),
		Context:  optString(plan.Context),
		Value:    plan.Value.ValueString(),
		GroupID:  optString(plan.GroupID),
		DeviceID: optString(plan.DeviceID),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error setting macro override", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, overrideFromClient(created))...)
}

func (r *monitorMacroOverrideResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorMacroOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	o, err := r.client.GetMonitorMacroOverride(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading macro override", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, overrideFromClient(o))...)
}

func (r *monitorMacroOverrideResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state monitorMacroOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	o, err := r.client.UpdateMonitorMacroOverrideValue(ctx, state.ID.ValueString(), plan.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating macro override", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, overrideFromClient(o))...)
}

func (r *monitorMacroOverrideResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorMacroOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMonitorMacroOverride(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting macro override", err.Error())
	}
}

func (r *monitorMacroOverrideResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------- data source

var _ datasource.DataSourceWithConfigure = &monitorTemplateDataSource{}

func NewMonitorTemplateDataSource() datasource.DataSource { return &monitorTemplateDataSource{} }

type monitorTemplateDataSource struct{ client *client.Client }

type monitorTemplateDataModel struct {
	ID          types.String `tfsdk:"id"`
	BuiltinKey  types.String `tfsdk:"builtin_key"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	BuiltIn     types.Bool   `tfsdk:"built_in"`
}

func (d *monitorTemplateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_template"
}

func (d *monitorTemplateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		Description: "Looks up a monitoring template by built-in key (linux, windows, macos, openwrt, docker, postgresql) or by name.",
		Attributes: map[string]dsschema.Attribute{
			"id":          dsschema.StringAttribute{Computed: true},
			"builtin_key": dsschema.StringAttribute{Optional: true, Computed: true},
			"name":        dsschema.StringAttribute{Optional: true, Computed: true},
			"description": dsschema.StringAttribute{Computed: true},
			"built_in":    dsschema.BoolAttribute{Computed: true},
		},
	}
}

func (d *monitorTemplateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c := monitorConfigureClient(req.ProviderData, &resp.Diagnostics); c != nil {
		d.client = c
	}
}

func (d *monitorTemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg monitorTemplateDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key, name := cfg.BuiltinKey.ValueString(), cfg.Name.ValueString()
	if key == "" && name == "" {
		resp.Diagnostics.AddError("Missing lookup", "Set builtin_key or name.")
		return
	}
	list, err := d.client.ListMonitorTemplates(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing monitoring templates", err.Error())
		return
	}
	for _, t := range list {
		matchKey := key != "" && t.BuiltinKey != nil && *t.BuiltinKey == key
		matchName := key == "" && t.Name == name
		if matchKey || matchName {
			resp.Diagnostics.Append(resp.State.Set(ctx, monitorTemplateDataModel{
				ID:          types.StringValue(t.ID),
				BuiltinKey:  strOrNull(t.BuiltinKey),
				Name:        types.StringValue(t.Name),
				Description: types.StringValue(t.Description),
				BuiltIn:     types.BoolValue(t.TenantID == nil),
			})...)
			return
		}
	}
	resp.Diagnostics.AddError("Monitoring template not found", fmt.Sprintf("No template with builtin_key %q / name %q.", key, name))
}
