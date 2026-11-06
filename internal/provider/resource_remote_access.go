package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &remoteAccessPolicyResource{}
	_ resource.ResourceWithConfigure   = &remoteAccessPolicyResource{}
	_ resource.ResourceWithImportState = &remoteAccessPolicyResource{}
	_ resource.Resource                = &remoteAccessOperatorResource{}
	_ resource.ResourceWithConfigure   = &remoteAccessOperatorResource{}
	_ resource.ResourceWithImportState = &remoteAccessOperatorResource{}
)

// ---------- rustinion_remote_access_policy ----------

func NewRemoteAccessPolicyResource() resource.Resource {
	return &remoteAccessPolicyResource{}
}

type remoteAccessPolicyResource struct {
	client *client.Client
}

type remoteAccessChannelModel struct {
	Value  types.String `tfsdk:"value"`
	Locked types.Bool   `tfsdk:"locked"`
}

var remoteAccessChannelAttrTypes = map[string]attr.Type{
	"value":  types.StringType,
	"locked": types.BoolType,
}

// The Terraform id is "<scope>:<scope_id>" (scope_id empty for the organization).
type remoteAccessPolicyResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Scope     types.String `tfsdk:"scope"`
	ScopeID   types.String `tfsdk:"scope_id"`
	Channels  types.Map    `tfsdk:"channels"`
	Effective types.Map    `tfsdk:"effective"`
	Why       types.Map    `tfsdk:"why"`
}

func (r *remoteAccessPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_remote_access_policy"
}

func (r *remoteAccessPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the remote access settings (remote desktop, each remote-access channel, and the remote terminal as its own separate setting) at one level of the hierarchy: the organization, a group or a device. A value is inherit, on or off; a setting applies to everything below it unless a lower level sets its own, except that a locked setting binds everything below. You can only change what your API key's organization owns; a lock set by the platform cannot be changed here. Channels not listed are left alone; removing a channel from the configuration (or destroying the resource) puts it back to inherit. The read-only effective and why maps show the resolved answer and which level decided it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`<scope>:<scope_id>`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"scope": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`tenant` (your organization), `group` or `device`.",
				Validators:          []validator.String{stringvalidator.OneOf("tenant", "group", "device")},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"scope_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The group or device id (omit for `tenant`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"channels": schema.MapNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Channel name -> setting. Channels: " + channelList() + ".",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"value": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "`inherit`, `on` or `off`.",
							Validators:          []validator.String{stringvalidator.OneOf("inherit", "on", "off")},
						},
						"locked": schema.BoolAttribute{
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(false),
							MarkdownDescription: "Lock the value for everything below (needs `on` or `off`; not available on a device).",
						},
					},
				},
			},
			"effective": schema.MapAttribute{
				Computed:            true,
				ElementType:         types.BoolType,
				MarkdownDescription: "Channel -> is it effectively on for this scope (after inheritance, locks and the dependency on the desktop).",
			},
			"why": schema.MapAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Channel -> one sentence saying which level decided.",
			},
		},
	}
}

func channelList() string {
	s := ""
	for i, c := range client.RemoteAccessChannels {
		if i > 0 {
			s += ", "
		}
		s += "`" + c + "`"
	}
	return s
}

func (r *remoteAccessPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

func validRemoteAccessChannel(name string) bool {
	for _, c := range client.RemoteAccessChannels {
		if c == name {
			return true
		}
	}
	return false
}

// remoteAccessChannelsFromMap decodes the plan/state `channels` map.
func remoteAccessChannelsFromMap(ctx context.Context, m types.Map) (map[string]remoteAccessChannelModel, diag.Diagnostics) {
	out := map[string]remoteAccessChannelModel{}
	if m.IsNull() || m.IsUnknown() {
		return out, nil
	}
	diags := m.ElementsAs(ctx, &out, false)
	return out, diags
}

// remoteAccessPutBody is what a plan means: every listed channel is set, every channel that was managed
// before and is no longer listed is cleared.
func remoteAccessPutBody(plan, prior map[string]remoteAccessChannelModel) map[string]*client.RemoteAccessInput {
	body := map[string]*client.RemoteAccessInput{}
	for name := range prior {
		if _, ok := plan[name]; !ok {
			body[name] = nil
		}
	}
	for name, m := range plan {
		locked := !m.Locked.IsNull() && !m.Locked.IsUnknown() && m.Locked.ValueBool()
		body[name] = &client.RemoteAccessInput{Value: m.Value.ValueString(), Locked: locked}
	}
	return body
}

// remoteAccessModelFromView builds the state from the server's answer. A channel appears when the prior
// state/plan manages it, or when the server has an explicit (non-inherit) setting at this scope (import).
// The organization's legacy "not enabled yet" default is not a setting and never appears by itself.
func remoteAccessModelFromView(scope, scopeID string, v *client.RemoteAccessView, managed map[string]remoteAccessChannelModel) (remoteAccessPolicyResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	channels := map[string]attr.Value{}
	names := map[string]bool{}
	for k := range managed {
		names[k] = true
	}
	for _, ch := range v.Channels {
		cell, ok := ch.Cells[scope]
		if !ok {
			continue
		}
		if cell.Value != "inherit" && !(cell.DefaultFromLegacy && cell.Value == "off") {
			names[ch.Channel] = true
		}
	}
	for name := range names {
		value, locked := "inherit", false
		if cell, ok := v.Cell(name); ok {
			value, locked = cell.Value, cell.Locked
			if cell.DefaultFromLegacy && cell.Value == "off" {
				value, locked = "inherit", false
			}
		}
		obj, d := types.ObjectValue(remoteAccessChannelAttrTypes, map[string]attr.Value{
			"value":  types.StringValue(value),
			"locked": types.BoolValue(locked),
		})
		diags.Append(d...)
		channels[name] = obj
	}
	eff := map[string]attr.Value{}
	why := map[string]attr.Value{}
	for _, ch := range v.Channels {
		eff[ch.Channel] = types.BoolValue(ch.Effective)
		why[ch.Channel] = types.StringValue(ch.Why)
	}
	m := remoteAccessPolicyResourceModel{
		ID:      types.StringValue(scope + ":" + scopeID),
		Scope:   types.StringValue(scope),
		ScopeID: types.StringNull(),
	}
	if scopeID != "" {
		m.ScopeID = types.StringValue(scopeID)
	}
	var d diag.Diagnostics
	if len(channels) == 0 {
		m.Channels = types.MapNull(types.ObjectType{AttrTypes: remoteAccessChannelAttrTypes})
	} else {
		m.Channels, d = types.MapValue(types.ObjectType{AttrTypes: remoteAccessChannelAttrTypes}, channels)
		diags.Append(d...)
	}
	m.Effective, d = types.MapValue(types.BoolType, eff)
	diags.Append(d...)
	m.Why, d = types.MapValue(types.StringType, why)
	diags.Append(d...)
	return m, diags
}

func (r *remoteAccessPolicyResource) validate(plan remoteAccessPolicyResourceModel, chans map[string]remoteAccessChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics
	scope := plan.Scope.ValueString()
	if scope != "tenant" && (plan.ScopeID.IsNull() || plan.ScopeID.ValueString() == "") {
		diags.AddError("scope_id required", fmt.Sprintf("scope %q needs scope_id (the %s id).", scope, scope))
	}
	names := make([]string, 0, len(chans))
	for n := range chans {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if !validRemoteAccessChannel(n) {
			diags.AddError("unknown channel", fmt.Sprintf("%q is not a remote access channel (%s).", n, channelList()))
		}
		m := chans[n]
		if !m.Locked.IsNull() && !m.Locked.IsUnknown() && m.Locked.ValueBool() && m.Value.ValueString() == "inherit" {
			diags.AddError("lock needs a value", fmt.Sprintf("channel %q: a lock needs on or off, not inherit.", n))
		}
		if scope == "device" && !m.Locked.IsNull() && !m.Locked.IsUnknown() && m.Locked.ValueBool() {
			diags.AddError("device cannot lock", fmt.Sprintf("channel %q: a device has nothing below it to lock; lock the group instead.", n))
		}
	}
	return diags
}

func (r *remoteAccessPolicyResource) apply(ctx context.Context, plan remoteAccessPolicyResourceModel, prior map[string]remoteAccessChannelModel) (*remoteAccessPolicyResourceModel, diag.Diagnostics) {
	chans, diags := remoteAccessChannelsFromMap(ctx, plan.Channels)
	if diags.HasError() {
		return nil, diags
	}
	diags.Append(r.validate(plan, chans)...)
	if diags.HasError() {
		return nil, diags
	}
	scope, scopeID := plan.Scope.ValueString(), plan.ScopeID.ValueString()
	view, err := r.client.PutRemoteAccess(ctx, scope, scopeID, remoteAccessPutBody(chans, prior))
	if err != nil {
		diags.AddError("Error saving remote access settings", err.Error())
		return nil, diags
	}
	m, d := remoteAccessModelFromView(scope, scopeID, view, chans)
	diags.Append(d...)
	return &m, diags
}

func (r *remoteAccessPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan remoteAccessPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, diags := r.apply(ctx, plan, nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *remoteAccessPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state remoteAccessPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managed, diags := remoteAccessChannelsFromMap(ctx, state.Channels)
	resp.Diagnostics.Append(diags...)
	scope, scopeID := state.Scope.ValueString(), state.ScopeID.ValueString()
	view, err := r.client.GetRemoteAccess(ctx, scope, scopeID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading remote access settings", err.Error())
		return
	}
	m, d := remoteAccessModelFromView(scope, scopeID, view, managed)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *remoteAccessPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state remoteAccessPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	prior, diags := remoteAccessChannelsFromMap(ctx, state.Channels)
	resp.Diagnostics.Append(diags...)
	saved, diags := r.apply(ctx, plan, prior)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *remoteAccessPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state remoteAccessPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managed, diags := remoteAccessChannelsFromMap(ctx, state.Channels)
	resp.Diagnostics.Append(diags...)
	body := map[string]*client.RemoteAccessInput{}
	for name := range managed {
		body[name] = nil
	}
	if len(body) == 0 {
		return
	}
	_, err := r.client.PutRemoteAccess(ctx, state.Scope.ValueString(), state.ScopeID.ValueString(), body)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error clearing remote access settings", err.Error())
	}
}

// ImportState takes `tenant`, `group:<id>` or `device:<id>`.
func (r *remoteAccessPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scope, scopeID := parseRemoteAccessImportID(req.ID)
	if scope == "" {
		resp.Diagnostics.AddError("Invalid import id", "Use `tenant`, `group:<group id>` or `device:<device id>`.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("scope"), scope)...)
	if scopeID != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("scope_id"), scopeID)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), scope+":"+scopeID)...)
}

func parseRemoteAccessImportID(id string) (scope, scopeID string) {
	if id == "tenant" || id == "tenant:" {
		return "tenant", ""
	}
	for _, s := range []string{"group", "device"} {
		prefix := s + ":"
		if len(id) > len(prefix) && id[:len(prefix)] == prefix {
			return s, id[len(prefix):]
		}
	}
	return "", ""
}

// ---------- rustinion_remote_access_operator ----------

func NewRemoteAccessOperatorResource() resource.Resource {
	return &remoteAccessOperatorResource{}
}

type remoteAccessOperatorResource struct {
	client *client.Client
}

type remoteAccessOperatorResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Scope   types.String `tfsdk:"scope"`
	ScopeID types.String `tfsdk:"scope_id"`
	UserID  types.String `tfsdk:"user_id"`
	Email   types.String `tfsdk:"email"`
	Channel types.String `tfsdk:"channel"`
	Effect  types.String `tfsdk:"effect"`
}

func (r *remoteAccessOperatorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_remote_access_operator"
}

func (r *remoteAccessOperatorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages who may start remote sessions: an allow (a technician who may use a channel inside the organization or one group) or a deny (a person who may not; beats every allow). Organization admins may open the terminal and the desktop by default; anyone else needs an allow. You cannot change your own access, grant what you do not hold, or deny someone above you; the server enforces it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned row id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"scope": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`tenant` or `group`.",
				Validators:          []validator.String{stringvalidator.OneOf("tenant", "group")},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"scope_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The group id (omit for `tenant`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"user_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The person's user id (`rustinion_user.x.user_id`). Give this or `email`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"email": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The person's email. Give this or `user_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"channel": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("*"),
				MarkdownDescription: "A channel name or `*` (every channel, the default; needs organization admin rights).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"effect": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`allow` or `deny`.",
				Validators:          []validator.String{stringvalidator.OneOf("allow", "deny")},
			},
		},
	}
}

func (r *remoteAccessOperatorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

func remoteAccessOperatorFromClient(o *client.RemoteAccessOperator) remoteAccessOperatorResourceModel {
	m := remoteAccessOperatorResourceModel{
		ID:      types.StringValue(o.ID),
		Scope:   types.StringValue(o.Scope),
		ScopeID: types.StringNull(),
		UserID:  types.StringValue(o.UserID),
		Email:   types.StringValue(o.Email),
		Channel: types.StringValue(o.Channel),
		Effect:  types.StringValue(o.Effect),
	}
	if o.Scope == "group" {
		m.ScopeID = types.StringValue(o.ScopeID)
	}
	return m
}

func (r *remoteAccessOperatorResource) put(ctx context.Context, plan remoteAccessOperatorResourceModel) (*remoteAccessOperatorResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	scope := plan.Scope.ValueString()
	if scope == "group" && (plan.ScopeID.IsNull() || plan.ScopeID.ValueString() == "") {
		diags.AddError("scope_id required", "scope \"group\" needs scope_id (the group id).")
	}
	if plan.UserID.ValueString() == "" && plan.Email.ValueString() == "" {
		diags.AddError("person required", "Give user_id or email.")
	}
	if diags.HasError() {
		return nil, diags
	}
	o, err := r.client.PutRemoteAccessOperator(ctx, scope, plan.ScopeID.ValueString(), plan.UserID.ValueString(),
		plan.Email.ValueString(), plan.Channel.ValueString(), plan.Effect.ValueString())
	if err != nil {
		diags.AddError("Error saving remote access operator", err.Error())
		return nil, diags
	}
	m := remoteAccessOperatorFromClient(o)
	return &m, diags
}

func (r *remoteAccessOperatorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan remoteAccessOperatorResourceModel
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

func (r *remoteAccessOperatorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state remoteAccessOperatorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	o, err := r.client.GetRemoteAccessOperator(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading remote access operator", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, remoteAccessOperatorFromClient(o))...)
}

func (r *remoteAccessOperatorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan remoteAccessOperatorResourceModel
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

func (r *remoteAccessOperatorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state remoteAccessOperatorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRemoteAccessOperator(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting remote access operator", err.Error())
	}
}

func (r *remoteAccessOperatorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
