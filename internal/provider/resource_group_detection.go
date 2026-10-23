package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupDetectionResource{}
	_ resource.ResourceWithConfigure   = &groupDetectionResource{}
	_ resource.ResourceWithImportState = &groupDetectionResource{}
)

func NewGroupDetectionResource() resource.Resource {
	return &groupDetectionResource{}
}

type groupDetectionResource struct {
	client *client.Client
}

type groupDetectionResourceModel struct {
	ID             types.String `tfsdk:"id"`
	GroupID        types.String `tfsdk:"group_id"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	CriticalPush   types.Bool   `tfsdk:"critical_push"`
	MinLevel       types.String `tfsdk:"min_level"`
	DisabledRules  types.List   `tfsdk:"disabled_rules"`
	RuleLevels     types.Map    `tfsdk:"rule_levels"`
	RuleThresholds types.Map    `tfsdk:"rule_thresholds"`
}

func (r *groupDetectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_detection"
}

func (r *groupDetectionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's detection rules. Sigma-compatible rules (a curated starter pack plus the organization's imported rules) run over the group's collected logs; matches are shown on the device with the matched log lines and raise findings. Turning it on also makes the agents collect the sign-in, sudo and account lines and Windows security events the rules read. Destroying this resource turns detection off.",
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
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Run detection rules for the group's devices. Default true (the resource exists to turn detection on).",
			},
			"critical_push": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Push an urgent alert when a critical-level rule matches (Windows event log cleared, a new account with user id 0, ...). Default false: findings only reach the summary email.",
			},
			"min_level": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("medium"),
				MarkdownDescription: "Matches below this level (informational, low, medium, high, critical) are kept and shown but are not findings. Default medium.",
			},
			"disabled_rules": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(emptyStringList()),
				MarkdownDescription: "Ids of rules switched off for this group (for example `rustinion-web-not-found`). Default none.",
			},
			"rule_levels": schema.MapAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             mapdefault.StaticValue(types.MapValueMust(types.StringType, map[string]attr.Value{})),
				MarkdownDescription: "Rule id to the level this group gives the rule (informational, low, medium, high, critical). Default: each rule's own level.",
			},
			"rule_thresholds": schema.MapAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.Int64Type,
				Default:             mapdefault.StaticValue(types.MapValueMust(types.Int64Type, map[string]attr.Value{})),
				MarkdownDescription: "Rule id to the number of matches (or distinct values) a counting rule needs inside its time window. Default: each rule's own threshold.",
			},
		},
	}
}

func (r *groupDetectionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func groupDetectionToClient(ctx context.Context, m groupDetectionResourceModel) (client.GroupDetection, diag.Diagnostics) {
	var diags diag.Diagnostics
	p := client.DefaultGroupDetection()
	p.Enabled = true
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		p.Enabled = m.Enabled.ValueBool()
	}
	if !m.CriticalPush.IsNull() && !m.CriticalPush.IsUnknown() {
		p.CriticalPush = m.CriticalPush.ValueBool()
	}
	if !m.MinLevel.IsNull() && !m.MinLevel.IsUnknown() {
		p.MinLevel = m.MinLevel.ValueString()
	}
	for _, id := range listToStrings(ctx, m.DisabledRules, &diags) {
		off := false
		t := p.Rules[id]
		t.Enabled = &off
		p.Rules[id] = t
	}
	levels := map[string]string{}
	if !m.RuleLevels.IsNull() && !m.RuleLevels.IsUnknown() {
		diags.Append(m.RuleLevels.ElementsAs(ctx, &levels, false)...)
	}
	for id, lvl := range levels {
		l := lvl
		t := p.Rules[id]
		t.Level = &l
		p.Rules[id] = t
	}
	thresholds := map[string]int64{}
	if !m.RuleThresholds.IsNull() && !m.RuleThresholds.IsUnknown() {
		diags.Append(m.RuleThresholds.ElementsAs(ctx, &thresholds, false)...)
	}
	for id, n := range thresholds {
		v := n
		t := p.Rules[id]
		t.Threshold = &v
		p.Rules[id] = t
	}
	return p, diags
}

func groupDetectionFromClient(ctx context.Context, groupID string, p *client.GroupDetection) (groupDetectionResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	disabled, d1 := types.ListValueFrom(ctx, types.StringType, p.DisabledRules())
	diags.Append(d1...)
	levels := map[string]string{}
	thresholds := map[string]int64{}
	for id, t := range p.Rules {
		if t.Level != nil {
			levels[id] = *t.Level
		}
		if t.Threshold != nil {
			thresholds[id] = *t.Threshold
		}
	}
	lv, d2 := types.MapValueFrom(ctx, types.StringType, levels)
	th, d3 := types.MapValueFrom(ctx, types.Int64Type, thresholds)
	diags.Append(d2...)
	diags.Append(d3...)
	return groupDetectionResourceModel{
		ID:             types.StringValue(groupID),
		GroupID:        types.StringValue(groupID),
		Enabled:        types.BoolValue(p.Enabled),
		CriticalPush:   types.BoolValue(p.CriticalPush),
		MinLevel:       types.StringValue(p.MinLevel),
		DisabledRules:  disabled,
		RuleLevels:     lv,
		RuleThresholds: th,
	}, diags
}

func (r *groupDetectionResource) put(ctx context.Context, m groupDetectionResourceModel) (*groupDetectionResourceModel, diag.Diagnostics) {
	groupID := m.GroupID.ValueString()
	in, diags := groupDetectionToClient(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	saved, err := r.client.PutGroupDetection(ctx, groupID, in)
	if err != nil {
		diags.AddError("Error saving group detection settings", err.Error())
		return nil, diags
	}
	out, d := groupDetectionFromClient(ctx, groupID, saved)
	diags.Append(d...)
	return &out, diags
}

func (r *groupDetectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupDetectionResourceModel
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

func (r *groupDetectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupDetectionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupDetection(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group detection settings", err.Error())
		return
	}
	m, diags := groupDetectionFromClient(ctx, groupID, p)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *groupDetectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupDetectionResourceModel
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

// Delete turns detection off (the tuning is dropped).
func (r *groupDetectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupDetectionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupDetection(ctx, state.GroupID.ValueString(), client.DefaultGroupDetection())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error turning off group detection rules", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_detection.servers <group-id>`.
func (r *groupDetectionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
