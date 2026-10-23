package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupHardeningResource{}
	_ resource.ResourceWithConfigure   = &groupHardeningResource{}
	_ resource.ResourceWithImportState = &groupHardeningResource{}
)

func NewGroupHardeningResource() resource.Resource {
	return &groupHardeningResource{}
}

type groupHardeningResource struct {
	client *client.Client
}

type groupHardeningResourceModel struct {
	ID            types.String `tfsdk:"id"`
	GroupID       types.String `tfsdk:"group_id"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	IntervalHours types.Int64  `tfsdk:"interval_hours"`
	Policies      types.List   `tfsdk:"policies"`
	MinScore      types.Int64  `tfsdk:"min_score"`
	AcceptedRisks types.List   `tfsdk:"accepted_risks"`
}

func (r *groupHardeningResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_hardening"
}

func (r *groupHardeningResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's hardening benchmarks: CIS-style checks (SSH, accounts, kernel network settings, permissions, firewall, updates, logging) run by the agent on Linux, Windows, macOS and OpenWrt; each device gets a score and a list of failing checks with remediation. Checks only read; nothing is changed on the device. Destroying this resource turns the checks off.",
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
				MarkdownDescription: "Run the checks in the group. Default true (the resource exists to turn them on).",
			},
			"interval_hours": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(24),
				MarkdownDescription: "Hours between scans (1 to 336). Default 24.",
			},
			"policies": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(emptyStringList()),
				MarkdownDescription: "Policy ids to run: shipped (`rustinion_debian_ubuntu_hardening`, `rustinion_rhel_hardening`, `rustinion_windows_hardening`, `rustinion_macos_hardening`, `rustinion_openwrt_hardening`) or imported. Empty = every shipped policy; each applies only to the operating systems it is written for.",
			},
			"min_score": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Raise a finding (summary email) when a device scores below this percentage. Unset = never.",
			},
			"accepted_risks": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(emptyStringList()),
				MarkdownDescription: "Checks you accept, as `policy_id | check_id | reason`; they leave the score. Default none.",
			},
		},
	}
}

func (r *groupHardeningResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func acceptedFromStrings(lines []string, diags *diag.Diagnostics) []client.AcceptedRisk {
	out := []client.AcceptedRisk{}
	for _, l := range lines {
		parts := strings.SplitN(l, "|", 3)
		if len(parts) < 2 {
			diags.AddError("Bad accepted_risks entry", fmt.Sprintf("%q: use `policy_id | check_id | reason`", l))
			continue
		}
		a := client.AcceptedRisk{PolicyID: strings.TrimSpace(parts[0]), CheckID: strings.TrimSpace(parts[1])}
		if len(parts) == 3 {
			a.Reason = strings.TrimSpace(parts[2])
		}
		out = append(out, a)
	}
	return out
}

func groupHardeningToClient(ctx context.Context, m groupHardeningResourceModel) (client.GroupHardening, diag.Diagnostics) {
	var diags diag.Diagnostics
	p := client.DefaultGroupHardening()
	p.Enabled = true
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		p.Enabled = m.Enabled.ValueBool()
	}
	if !m.IntervalHours.IsNull() && !m.IntervalHours.IsUnknown() {
		p.IntervalHours = m.IntervalHours.ValueInt64()
	}
	p.Policies = listToStrings(ctx, m.Policies, &diags)
	p.AcceptedRisks = acceptedFromStrings(listToStrings(ctx, m.AcceptedRisks, &diags), &diags)
	if !m.MinScore.IsNull() && !m.MinScore.IsUnknown() {
		v := m.MinScore.ValueInt64()
		p.MinScore = &v
	}
	return p, diags
}

func groupHardeningFromClient(ctx context.Context, groupID string, p *client.GroupHardening) (groupHardeningResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	pol, d1 := types.ListValueFrom(ctx, types.StringType, p.Policies)
	diags.Append(d1...)
	lines := []string{}
	for _, a := range p.AcceptedRisks {
		lines = append(lines, fmt.Sprintf("%s | %s | %s", a.PolicyID, a.CheckID, a.Reason))
	}
	acc, d2 := types.ListValueFrom(ctx, types.StringType, lines)
	diags.Append(d2...)
	minScore := types.Int64Null()
	if p.MinScore != nil {
		minScore = types.Int64Value(*p.MinScore)
	}
	return groupHardeningResourceModel{
		ID:            types.StringValue(groupID),
		GroupID:       types.StringValue(groupID),
		Enabled:       types.BoolValue(p.Enabled),
		IntervalHours: types.Int64Value(p.IntervalHours),
		Policies:      pol,
		MinScore:      minScore,
		AcceptedRisks: acc,
	}, diags
}

func (r *groupHardeningResource) put(ctx context.Context, m groupHardeningResourceModel) (*groupHardeningResourceModel, diag.Diagnostics) {
	groupID := m.GroupID.ValueString()
	in, diags := groupHardeningToClient(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	saved, err := r.client.PutGroupHardening(ctx, groupID, in)
	if err != nil {
		diags.AddError("Error saving group hardening settings", err.Error())
		return nil, diags
	}
	out, d := groupHardeningFromClient(ctx, groupID, saved)
	diags.Append(d...)
	return &out, diags
}

func (r *groupHardeningResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupHardeningResourceModel
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

func (r *groupHardeningResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupHardeningResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupHardening(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group hardening settings", err.Error())
		return
	}
	m, diags := groupHardeningFromClient(ctx, groupID, p)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *groupHardeningResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupHardeningResourceModel
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

// Delete turns the checks off.
func (r *groupHardeningResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupHardeningResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupHardening(ctx, state.GroupID.ValueString(), client.DefaultGroupHardening())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error turning off group hardening checks", err.Error())
	}
}

// ImportState takes the group id.
func (r *groupHardeningResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
