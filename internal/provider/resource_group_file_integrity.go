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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupFileIntegrityResource{}
	_ resource.ResourceWithConfigure   = &groupFileIntegrityResource{}
	_ resource.ResourceWithImportState = &groupFileIntegrityResource{}
)

func NewGroupFileIntegrityResource() resource.Resource {
	return &groupFileIntegrityResource{}
}

type groupFileIntegrityResource struct {
	client *client.Client
}

type groupFileIntegrityResourceModel struct {
	ID           types.String `tfsdk:"id"`
	GroupID      types.String `tfsdk:"group_id"`
	Enabled      types.Bool   `tfsdk:"enabled"`
	UseDefaults  types.Bool   `tfsdk:"use_defaults"`
	Paths        types.List   `tfsdk:"paths"`
	Exclusions   types.List   `tfsdk:"exclusions"`
	ScanMinutes  types.Int64  `tfsdk:"scan_minutes"`
	Realtime     types.Bool   `tfsdk:"realtime"`
	CriticalPush types.Bool   `tfsdk:"critical_push"`
}

func (r *groupFileIntegrityResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_file_integrity"
}

func emptyStringList() types.List {
	return types.ListValueMust(types.StringType, []attr.Value{})
}

func (r *groupFileIntegrityResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's file integrity monitoring. Agents hash the watched files (sha256, owner, permissions, size, modification time), compare them with a baseline and report changes; changes made by the package manager (dpkg, rpm, apk, opkg, pkg, Windows Update, macOS installer) are recognised and never alert, anything else raises a finding until accepted. Destroying this resource turns monitoring off.",
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
				MarkdownDescription: "Monitor the group's devices. Default true (the resource exists to turn monitoring on).",
			},
			"use_defaults": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Watch the built-in list for each OS (Linux: /etc, /bin, /sbin, /usr/bin, /usr/sbin, /boot, systemd units, cron, SSH authorized_keys; Windows: System32 drivers and hosts, startup folders, scheduled tasks, core executables; macOS: LaunchDaemons/LaunchAgents, /etc, /usr/bin; FreeBSD/OPNsense/pfSense: /etc, /usr/local/etc, binaries). Default true.",
			},
			"paths": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(emptyStringList()),
				MarkdownDescription: "Extra files or folders (a folder is watched recursively; `~/` = every user's home; `*` wildcards in path segments). Default none.",
			},
			"exclusions": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(emptyStringList()),
				MarkdownDescription: "Patterns never watched (`*` within a folder, `**` any depth). Default none.",
			},
			"scan_minutes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(30),
				MarkdownDescription: "Minutes between scheduled scans (5 to 1440). Default 30.",
			},
			"realtime": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Linux: also rescan within about 30 seconds of a file event (inotify). Default true.",
			},
			"critical_push": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Push an urgent alert when a critical system file (passwd, shadow, sudoers, sshd_config, SSH keys, system binaries, hosts file) changes outside a package update. Default false: other findings only reach the summary email.",
			},
		},
	}
}

func (r *groupFileIntegrityResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func listToStrings(ctx context.Context, l types.List, diags *diag.Diagnostics) []string {
	out := []string{}
	if !l.IsNull() && !l.IsUnknown() {
		diags.Append(l.ElementsAs(ctx, &out, false)...)
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func groupFileIntegrityToClient(ctx context.Context, m groupFileIntegrityResourceModel) (client.GroupFileIntegrity, diag.Diagnostics) {
	var diags diag.Diagnostics
	p := client.DefaultGroupFileIntegrity()
	p.Enabled = true
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		p.Enabled = m.Enabled.ValueBool()
	}
	if !m.UseDefaults.IsNull() && !m.UseDefaults.IsUnknown() {
		p.UseDefaults = m.UseDefaults.ValueBool()
	}
	p.Paths = listToStrings(ctx, m.Paths, &diags)
	p.Exclusions = listToStrings(ctx, m.Exclusions, &diags)
	if !m.ScanMinutes.IsNull() && !m.ScanMinutes.IsUnknown() {
		p.ScanMinutes = m.ScanMinutes.ValueInt64()
	}
	if !m.Realtime.IsNull() && !m.Realtime.IsUnknown() {
		p.Realtime = m.Realtime.ValueBool()
	}
	if !m.CriticalPush.IsNull() && !m.CriticalPush.IsUnknown() {
		p.CriticalPush = m.CriticalPush.ValueBool()
	}
	return p, diags
}

func groupFileIntegrityFromClient(ctx context.Context, groupID string, p *client.GroupFileIntegrity) (groupFileIntegrityResourceModel, diag.Diagnostics) {
	paths, d1 := types.ListValueFrom(ctx, types.StringType, p.Paths)
	excl, d2 := types.ListValueFrom(ctx, types.StringType, p.Exclusions)
	var diags diag.Diagnostics
	diags.Append(d1...)
	diags.Append(d2...)
	return groupFileIntegrityResourceModel{
		ID:           types.StringValue(groupID),
		GroupID:      types.StringValue(groupID),
		Enabled:      types.BoolValue(p.Enabled),
		UseDefaults:  types.BoolValue(p.UseDefaults),
		Paths:        paths,
		Exclusions:   excl,
		ScanMinutes:  types.Int64Value(p.ScanMinutes),
		Realtime:     types.BoolValue(p.Realtime),
		CriticalPush: types.BoolValue(p.CriticalPush),
	}, diags
}

func (r *groupFileIntegrityResource) put(ctx context.Context, m groupFileIntegrityResourceModel) (*groupFileIntegrityResourceModel, diag.Diagnostics) {
	groupID := m.GroupID.ValueString()
	in, diags := groupFileIntegrityToClient(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	saved, err := r.client.PutGroupFileIntegrity(ctx, groupID, in)
	if err != nil {
		diags.AddError("Error saving group file integrity settings", err.Error())
		return nil, diags
	}
	out, d := groupFileIntegrityFromClient(ctx, groupID, saved)
	diags.Append(d...)
	return &out, diags
}

func (r *groupFileIntegrityResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupFileIntegrityResourceModel
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

func (r *groupFileIntegrityResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupFileIntegrityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupFileIntegrity(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group file integrity settings", err.Error())
		return
	}
	m, diags := groupFileIntegrityFromClient(ctx, groupID, p)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *groupFileIntegrityResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupFileIntegrityResourceModel
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

// Delete turns monitoring off (baselines are dropped by the agents).
func (r *groupFileIntegrityResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupFileIntegrityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupFileIntegrity(ctx, state.GroupID.ValueString(), client.DefaultGroupFileIntegrity())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error turning off group file integrity monitoring", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_file_integrity.servers <group-id>`.
func (r *groupFileIntegrityResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
