package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &remoteAccessDLPResource{}
	_ resource.ResourceWithConfigure   = &remoteAccessDLPResource{}
	_ resource.ResourceWithImportState = &remoteAccessDLPResource{}
)

func NewRemoteAccessDLPResource() resource.Resource {
	return &remoteAccessDLPResource{}
}

type remoteAccessDLPResource struct {
	client *client.Client
}

type remoteAccessDLPResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Scope         types.String `tfsdk:"scope"`
	ScopeID       types.String `tfsdk:"scope_id"`
	Preset        types.String `tfsdk:"preset"`
	PolicyJSON    types.String `tfsdk:"policy_json"`
	EffectiveJSON types.String `tfsdk:"effective_json"`
}

func (r *remoteAccessDLPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_remote_access_dlp"
}

func (r *remoteAccessDLPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the data-loss policy (what each remote-access channel may carry: clipboard formats and sizes, file types and sizes, drives, printing, USB classes, audio, recording, session limits) at one level: the organization, a group or a device. A policy at a level can only narrow what the levels above allow; the effective policy is the intersection of all levels, and a channel that is switched off carries nothing. Give a `preset` (support, locked_down, full, kiosk) or a `policy_json`; destroying the resource clears the level.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
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
			"preset": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "`support`, `locked_down`, `full` or `kiosk`. Give this or `policy_json`.",
				Validators: []validator.String{
					stringvalidator.OneOf("support", "locked_down", "full", "kiosk"),
					stringvalidator.ConflictsWith(path.MatchRoot("policy_json")),
				},
			},
			"policy_json": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The policy as JSON; keys you leave out take the locked-down defaults. Use `jsonencode`.",
			},
			"effective_json": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The effective policy and channel switches for this scope (all levels combined), as JSON.",
			},
		},
	}
}

func (r *remoteAccessDLPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

// jsonSubset reports whether every key/element of cfg is present in server with the same value (arrays
// compare as sets: the server sorts them). The server fills defaults for keys a configuration omits.
func jsonSubset(cfg, server any) bool {
	switch c := cfg.(type) {
	case map[string]any:
		s, ok := server.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range c {
			sv, ok := s[k]
			if !ok || !jsonSubset(v, sv) {
				return false
			}
		}
		return true
	case []any:
		s, ok := server.([]any)
		if !ok || len(c) != len(s) {
			return false
		}
		as, bs := sortedJSON(c), sortedJSON(s)
		return reflect.DeepEqual(as, bs)
	default:
		return reflect.DeepEqual(cfg, server)
	}
}

func sortedJSON(a []any) []string {
	out := make([]string, len(a))
	for i, v := range a {
		b, _ := json.Marshal(v)
		out[i] = string(b)
	}
	sort.Strings(out)
	return out
}

func remoteAccessDLPModel(scope, scopeID string, prior types.String, preset types.String, v *client.RemoteAccessDLP) remoteAccessDLPResourceModel {
	m := remoteAccessDLPResourceModel{
		ID:            types.StringValue(scope + ":" + scopeID),
		Scope:         types.StringValue(scope),
		ScopeID:       types.StringNull(),
		Preset:        types.StringNull(),
		PolicyJSON:    types.StringNull(),
		EffectiveJSON: types.StringValue(string(v.Effective)),
	}
	if scopeID != "" {
		m.ScopeID = types.StringValue(scopeID)
	}
	if v.Preset != "" && !preset.IsNull() {
		m.Preset = types.StringValue(v.Preset)
	}
	hasPolicy := len(v.Policy) > 0 && string(v.Policy) != "null"
	if hasPolicy && (m.Preset.IsNull() && (preset.IsNull() || v.Preset == "")) {
		// Keep the configured text when it still means the same thing; otherwise show the server's.
		keep := false
		if !prior.IsNull() && !prior.IsUnknown() {
			var cfg, srv any
			if json.Unmarshal([]byte(prior.ValueString()), &cfg) == nil && json.Unmarshal(v.Policy, &srv) == nil {
				keep = jsonSubset(cfg, srv)
			}
		}
		if keep {
			m.PolicyJSON = prior
		} else {
			m.PolicyJSON = types.StringValue(string(v.Policy))
		}
	}
	return m
}

func (r *remoteAccessDLPResource) validate(plan remoteAccessDLPResourceModel) error {
	scope := plan.Scope.ValueString()
	if scope != "tenant" && plan.ScopeID.ValueString() == "" {
		return fmt.Errorf("scope %q needs scope_id", scope)
	}
	if plan.Preset.IsNull() && plan.PolicyJSON.IsNull() {
		return fmt.Errorf("give preset or policy_json")
	}
	if !plan.PolicyJSON.IsNull() && !json.Valid([]byte(plan.PolicyJSON.ValueString())) {
		return fmt.Errorf("policy_json is not valid JSON")
	}
	return nil
}

func (r *remoteAccessDLPResource) apply(ctx context.Context, plan remoteAccessDLPResourceModel) (*remoteAccessDLPResourceModel, error) {
	if err := r.validate(plan); err != nil {
		return nil, err
	}
	scope, scopeID := plan.Scope.ValueString(), plan.ScopeID.ValueString()
	var policy json.RawMessage
	if !plan.PolicyJSON.IsNull() {
		policy = json.RawMessage(plan.PolicyJSON.ValueString())
	}
	v, err := r.client.PutRemoteAccessDLP(ctx, scope, scopeID, plan.Preset.ValueString(), policy)
	if err != nil {
		return nil, err
	}
	m := remoteAccessDLPModel(scope, scopeID, plan.PolicyJSON, plan.Preset, v)
	return &m, nil
}

func (r *remoteAccessDLPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan remoteAccessDLPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	m, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error saving remote access data policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *remoteAccessDLPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state remoteAccessDLPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	scope, scopeID := state.Scope.ValueString(), state.ScopeID.ValueString()
	v, err := r.client.GetRemoteAccessDLP(ctx, scope, scopeID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading remote access data policy", err.Error())
		return
	}
	if len(v.Policy) == 0 || string(v.Policy) == "null" {
		// Someone cleared this level outside Terraform.
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, remoteAccessDLPModel(scope, scopeID, state.PolicyJSON, state.Preset, v))...)
}

func (r *remoteAccessDLPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan remoteAccessDLPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	m, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating remote access data policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *remoteAccessDLPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state remoteAccessDLPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutRemoteAccessDLP(ctx, state.Scope.ValueString(), state.ScopeID.ValueString(), "", nil)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error clearing remote access data policy", err.Error())
	}
}

// ImportState takes `tenant`, `group:<id>` or `device:<id>`.
func (r *remoteAccessDLPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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
