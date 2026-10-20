package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupRestartPolicyResource{}
	_ resource.ResourceWithConfigure   = &groupRestartPolicyResource{}
	_ resource.ResourceWithImportState = &groupRestartPolicyResource{}
)

// DefaultGroupRestartPolicy is what destroying the resource writes back:
// nothing restarts on its own (the same safe default every group starts
// with).
const DefaultGroupRestartPolicy = `{"auto_restart":"never"}`

func NewGroupRestartPolicyResource() resource.Resource {
	return &groupRestartPolicyResource{}
}

type groupRestartPolicyResource struct {
	client *client.Client
}

// groupRestartPolicyResourceModel: a per-group singleton, id = group id.
type groupRestartPolicyResourceModel struct {
	ID         types.String `tfsdk:"id"`
	GroupID    types.String `tfsdk:"group_id"`
	PolicyJSON types.String `tfsdk:"policy_json"`
	Paused     types.String `tfsdk:"paused"`
}

func (r *groupRestartPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_restart_policy"
}

func (r *groupRestartPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's restart policy: automatic restart (never / in_window / as_soon_as_needed), install and restart windows, rings, max at once, and coordinated (rolling) restarts with drain and verify hooks. " +
			"The policy is a JSON object (use jsonencode) with the keys of PUT /api/v1/customer/groups/{id}/restart-policy, e.g. " +
			"`{auto_restart = \"in_window\", restart_window = [{days = [0,1,2,3,4,5,6], start_minute = 60, end_minute = 300}], max_concurrent = 1, " +
			"drain_hooks = [{kind = \"stop_services\", units = [\"forgejo-runner*.service\"]}], verify_hooks = [{kind = \"mounted\", paths = [\"/nfs-home\"]}], drain_timeout_minutes = 180}`. " +
			"Absent keys use the built-in defaults. See https://rustinion.com/rolling-restarts.html. Destroying the resource sets the policy back to `{\"auto_restart\":\"never\"}`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as group_id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The group this policy belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"policy_json": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The whole restart policy as a JSON object (jsonencode). The server validates it (windows, hook syntax, timeouts) and rejects invalid values.",
			},
			"paused": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Why the group's restarts are paused (a device didn't come back or isn't healthy after a coordinated restart), empty while they run. Resume on the group's Restarts page.",
			},
		},
	}
}

func (r *groupRestartPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// sameJSON reports whether two JSON documents are equal as values (key
// order, whitespace and number formatting don't matter).
func sameJSON(a, b string) bool {
	var va, vb any
	if json.Unmarshal([]byte(a), &va) != nil || json.Unmarshal([]byte(b), &vb) != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

// groupRestartPolicyFromClient keeps the configured text when the server's
// copy means the same thing, so jsonencode's key order never shows a diff.
func groupRestartPolicyFromClient(groupID string, p *client.GroupRestartPolicy, prior string) groupRestartPolicyResourceModel {
	server := string(p.RestartPolicy)
	if server == "" || server == "null" {
		server = "{}"
	}
	text := server
	if prior != "" && sameJSON(prior, server) {
		text = prior
	}
	paused := ""
	if p.Paused != nil {
		paused = *p.Paused
	}
	return groupRestartPolicyResourceModel{
		ID:         types.StringValue(groupID),
		GroupID:    types.StringValue(groupID),
		PolicyJSON: types.StringValue(text),
		Paused:     types.StringValue(paused),
	}
}

func (r *groupRestartPolicyResource) put(ctx context.Context, m groupRestartPolicyResourceModel) (*groupRestartPolicyResourceModel, error) {
	groupID := m.GroupID.ValueString()
	text := m.PolicyJSON.ValueString()
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		return nil, fmt.Errorf("policy_json must be a JSON object: %w", err)
	}
	saved, err := r.client.PutGroupRestartPolicy(ctx, groupID, json.RawMessage(text))
	if err != nil {
		return nil, err
	}
	out := groupRestartPolicyFromClient(groupID, saved, text)
	return &out, nil
}

func (r *groupRestartPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupRestartPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting group restart policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupRestartPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupRestartPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupRestartPolicy(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group restart policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupRestartPolicyFromClient(groupID, p, state.PolicyJSON.ValueString()))...)
}

func (r *groupRestartPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupRestartPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating group restart policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

// Delete writes the safe default back (nothing restarts on its own).
func (r *groupRestartPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupRestartPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupRestartPolicy(ctx, state.GroupID.ValueString(), json.RawMessage(DefaultGroupRestartPolicy))
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting group restart policy", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_restart_policy.ci <group-id>`.
func (r *groupRestartPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
