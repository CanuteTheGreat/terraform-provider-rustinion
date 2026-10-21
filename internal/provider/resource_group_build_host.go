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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupBuildHostResource{}
	_ resource.ResourceWithConfigure   = &groupBuildHostResource{}
	_ resource.ResourceWithImportState = &groupBuildHostResource{}
)

func NewGroupBuildHostResource() resource.Resource {
	return &groupBuildHostResource{}
}

type groupBuildHostResource struct {
	client *client.Client
}

type groupBuildHostResourceModel struct {
	ID                types.String `tfsdk:"id"`
	GroupID           types.String `tfsdk:"group_id"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	ContainerPatterns types.List   `tfsdk:"container_patterns"`
}

func (r *groupBuildHostResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_build_host"
}

func (r *groupBuildHostResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's Build/CI host mode. When enabled, suspicious-process findings (a web server starting a shell, and similar) that come from inside an ephemeral CI job container are recorded as info and never alert: runner job containers (Forgejo/Gitea Actions `*-ACTIONS-TASK-*`, GitLab Runner build containers), containers a running CI job started and labelled `rustinion.ci.job=<job container>`, rootless containers of the CI runner's user, and containers whose name matches container_patterns. Events on the host itself and in containers that can't be traced to a CI job still alert; outside enabled groups labels are never consulted. Destroying this resource turns the mode off.",
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
				MarkdownDescription: "The group's devices are build/CI hosts. Default true (the resource exists to turn the mode on).",
			},
			"container_patterns": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: "Container names your CI starts (`*` matches anything, e.g. `rustinion-ci-*`); a pattern that matches every name is refused. Default none.",
			},
		},
	}
}

func (r *groupBuildHostResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func groupBuildHostToClient(ctx context.Context, m groupBuildHostResourceModel) (client.GroupBuildHost, diag.Diagnostics) {
	var diags diag.Diagnostics
	p := client.GroupBuildHost{Enabled: true, ContainerPatterns: []string{}}
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		p.Enabled = m.Enabled.ValueBool()
	}
	if !m.ContainerPatterns.IsNull() && !m.ContainerPatterns.IsUnknown() {
		var pats []string
		diags.Append(m.ContainerPatterns.ElementsAs(ctx, &pats, false)...)
		if pats == nil {
			pats = []string{}
		}
		p.ContainerPatterns = pats
	}
	return p, diags
}

func groupBuildHostFromClient(ctx context.Context, groupID string, p *client.GroupBuildHost) (groupBuildHostResourceModel, diag.Diagnostics) {
	pats, diags := types.ListValueFrom(ctx, types.StringType, p.ContainerPatterns)
	return groupBuildHostResourceModel{
		ID:                types.StringValue(groupID),
		GroupID:           types.StringValue(groupID),
		Enabled:           types.BoolValue(p.Enabled),
		ContainerPatterns: pats,
	}, diags
}

func (r *groupBuildHostResource) put(ctx context.Context, m groupBuildHostResourceModel) (*groupBuildHostResourceModel, diag.Diagnostics) {
	groupID := m.GroupID.ValueString()
	in, diags := groupBuildHostToClient(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	saved, err := r.client.PutGroupBuildHost(ctx, groupID, in)
	if err != nil {
		diags.AddError("Error saving group Build/CI host setting", err.Error())
		return nil, diags
	}
	out, d := groupBuildHostFromClient(ctx, groupID, saved)
	diags.Append(d...)
	return &out, diags
}

func (r *groupBuildHostResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupBuildHostResourceModel
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

func (r *groupBuildHostResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupBuildHostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupBuildHost(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group Build/CI host setting", err.Error())
		return
	}
	m, diags := groupBuildHostFromClient(ctx, groupID, p)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *groupBuildHostResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupBuildHostResourceModel
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

// Delete turns the mode off and clears the patterns.
func (r *groupBuildHostResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupBuildHostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupBuildHost(ctx, state.GroupID.ValueString(), client.DefaultGroupBuildHost())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting group Build/CI host setting", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_build_host.runners <group-id>`.
func (r *groupBuildHostResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
