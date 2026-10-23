package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupActiveResponseResource{}
	_ resource.ResourceWithConfigure   = &groupActiveResponseResource{}
	_ resource.ResourceWithImportState = &groupActiveResponseResource{}
)

func NewGroupActiveResponseResource() resource.Resource {
	return &groupActiveResponseResource{}
}

type groupActiveResponseResource struct {
	client *client.Client
}

type groupActiveResponseModel struct {
	ID             types.String `tfsdk:"id"`
	GroupID        types.String `tfsdk:"group_id"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	DryRun         types.Bool   `tfsdk:"dry_run"`
	MinLevel       types.String `tfsdk:"min_level"`
	BlockIP        types.Bool   `tfsdk:"block_ip"`
	BlockMinutes   types.Int64  `tfsdk:"block_minutes"`
	KillProcess    types.Bool   `tfsdk:"kill_process"`
	DisableUser    types.Bool   `tfsdk:"disable_user"`
	DisableHours   types.Int64  `tfsdk:"disable_hours"`
	Allowlist      types.List   `tfsdk:"allowlist"`
	BlockPrivate   types.Bool   `tfsdk:"block_private"`
	ProtectedUsers types.List   `tfsdk:"protected_users"`
}

func (r *groupActiveResponseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_active_response"
}

func boolAttr(def bool, desc string) schema.BoolAttribute {
	return schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(def), MarkdownDescription: desc}
}

func (r *groupActiveResponseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's active response. A detection rule match at or above `min_level` can block the attacking address on the device's firewall for a while, kill the process the log line names, or lock a newly created account. Starts in dry run (it only records what it would do). Never touched: private ranges (unless allowed), the allowlist, admins' recent addresses, the Rustinion server, root/Administrator and protected accounts. Blocks and locks expire; every applied action can be undone. Destroying this resource turns active response off.",
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
			"enabled":       boolAttr(true, "Turn active response on for the group. Default true (the resource exists to turn it on)."),
			"dry_run":       boolAttr(true, "Only record what would be done. Default true: set false when the dry-run log looks right."),
			"block_ip":      boolAttr(true, "Block the source address on the host firewall. Default true."),
			"kill_process":  boolAttr(false, "Kill the process a matched log line names (pid and name must still match). Default false."),
			"disable_user":  boolAttr(false, "Lock a newly created account a rule flagged. Default false."),
			"block_private": boolAttr(false, "Allow blocking private address ranges (RFC 1918, ULA). Default false."),
			"min_level": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("high"),
				MarkdownDescription: "Act on matches of at least this level (low, medium, high, critical). Default high.",
			},
			"block_minutes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(60),
				MarkdownDescription: "How long a block lasts (1 to 10080). Default 60.",
			},
			"disable_hours": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(24),
				MarkdownDescription: "How long an account lock lasts in hours (0 = until undone, at most 720). Default 24.",
			},
			"allowlist": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(emptyStringList()),
				MarkdownDescription: "Addresses and CIDR ranges that are never blocked (your offices and VPN). Default none.",
			},
			"protected_users": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(emptyStringList()),
				MarkdownDescription: "Accounts that are never locked (root and Administrator always are protected). Default none.",
			},
		},
	}
}

func (r *groupActiveResponseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func boolOr(v types.Bool, def bool) bool {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return v.ValueBool()
}

func intOr(v types.Int64, def int64) int64 {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return v.ValueInt64()
}

func groupActiveResponseToClient(ctx context.Context, m groupActiveResponseModel) (client.GroupActiveResponse, diag.Diagnostics) {
	var diags diag.Diagnostics
	p := client.DefaultGroupActiveResponse()
	p.Enabled = boolOr(m.Enabled, true)
	p.DryRun = boolOr(m.DryRun, true)
	p.BlockIP = boolOr(m.BlockIP, true)
	p.KillProcess = boolOr(m.KillProcess, false)
	p.DisableUser = boolOr(m.DisableUser, false)
	p.BlockPrivate = boolOr(m.BlockPrivate, false)
	if !m.MinLevel.IsNull() && !m.MinLevel.IsUnknown() {
		p.MinLevel = m.MinLevel.ValueString()
	}
	p.BlockMinutes = intOr(m.BlockMinutes, 60)
	p.DisableHours = intOr(m.DisableHours, 24)
	p.Allowlist = listToStrings(ctx, m.Allowlist, &diags)
	p.ProtectedUsers = listToStrings(ctx, m.ProtectedUsers, &diags)
	return p, diags
}

func groupActiveResponseFromClient(ctx context.Context, groupID string, p *client.GroupActiveResponse) (groupActiveResponseModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	allow, d1 := types.ListValueFrom(ctx, types.StringType, p.Allowlist)
	prot, d2 := types.ListValueFrom(ctx, types.StringType, p.ProtectedUsers)
	diags.Append(d1...)
	diags.Append(d2...)
	return groupActiveResponseModel{
		ID:             types.StringValue(groupID),
		GroupID:        types.StringValue(groupID),
		Enabled:        types.BoolValue(p.Enabled),
		DryRun:         types.BoolValue(p.DryRun),
		MinLevel:       types.StringValue(p.MinLevel),
		BlockIP:        types.BoolValue(p.BlockIP),
		BlockMinutes:   types.Int64Value(p.BlockMinutes),
		KillProcess:    types.BoolValue(p.KillProcess),
		DisableUser:    types.BoolValue(p.DisableUser),
		DisableHours:   types.Int64Value(p.DisableHours),
		Allowlist:      allow,
		BlockPrivate:   types.BoolValue(p.BlockPrivate),
		ProtectedUsers: prot,
	}, diags
}

func (r *groupActiveResponseResource) put(ctx context.Context, m groupActiveResponseModel) (*groupActiveResponseModel, diag.Diagnostics) {
	groupID := m.GroupID.ValueString()
	in, diags := groupActiveResponseToClient(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	saved, err := r.client.PutGroupActiveResponse(ctx, groupID, in)
	if err != nil {
		diags.AddError("Error saving group active response settings", err.Error())
		return nil, diags
	}
	out, d := groupActiveResponseFromClient(ctx, groupID, saved)
	diags.Append(d...)
	return &out, diags
}

func (r *groupActiveResponseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupActiveResponseModel
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

func (r *groupActiveResponseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupActiveResponseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupActiveResponse(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group active response settings", err.Error())
		return
	}
	m, diags := groupActiveResponseFromClient(ctx, groupID, p)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *groupActiveResponseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupActiveResponseModel
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

// Delete turns active response off (the action log stays).
func (r *groupActiveResponseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupActiveResponseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupActiveResponse(ctx, state.GroupID.ValueString(), client.DefaultGroupActiveResponse())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error turning off group active response", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_active_response.servers <group-id>`.
func (r *groupActiveResponseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
