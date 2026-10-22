package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupTerminalServicesResource{}
	_ resource.ResourceWithConfigure   = &groupTerminalServicesResource{}
	_ resource.ResourceWithImportState = &groupTerminalServicesResource{}
)

func NewGroupTerminalServicesResource() resource.Resource {
	return &groupTerminalServicesResource{}
}

type groupTerminalServicesResource struct {
	client *client.Client
}

type terminalServiceModel struct {
	Name types.String `tfsdk:"name"`
	Exe  types.String `tfsdk:"exe"`
	Args types.String `tfsdk:"args"`
}

type groupTerminalServicesResourceModel struct {
	ID       types.String           `tfsdk:"id"`
	GroupID  types.String           `tfsdk:"group_id"`
	Builtins types.List             `tfsdk:"builtins"`
	Custom   []terminalServiceModel `tfsdk:"custom"`
}

func (r *groupTerminalServicesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_terminal_services"
}

func (r *groupTerminalServicesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's expected terminal services: products that give people a shell through the browser on purpose (ttyd, GoTTY, WeTTY, code-server, JupyterLab terminals, the OOPS! lab agent). A shell started under a process matching an entry by program path and arguments is recorded as info and never alerts; any other program starting a shell still alerts. Nothing is expected by default. Destroying this resource clears the list.",
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
			"builtins": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: "Built-in suggestions to expect: `ttyd`, `gotty`, `wetty`, `code-server`, `jupyter`, `oops-agent` (the OOPS! browser-terminal agent, `*/node` running `/opt/oops-agent/agent.js`). Default none.",
			},
			"custom": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Your own entries (at most 50 in total). An entry for a general-purpose runtime or shell (node, python, bash...) must name the script in `args`; a program pattern matching everything is refused.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "What the device page calls it.",
						},
						"exe": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Glob over the program's path (`*` matches anything), e.g. `*/node`.",
						},
						"args": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Glob over the program's arguments joined by spaces, e.g. `/srv/term/server.js`; `*` for any.",
						},
					},
				},
			},
		},
	}
}

func (r *groupTerminalServicesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func groupTerminalServicesToClient(ctx context.Context, m groupTerminalServicesResourceModel) (client.GroupTerminalServices, diag.Diagnostics) {
	var diags diag.Diagnostics
	p := client.DefaultGroupTerminalServices()
	if !m.Builtins.IsNull() && !m.Builtins.IsUnknown() {
		var ids []string
		diags.Append(m.Builtins.ElementsAs(ctx, &ids, false)...)
		if ids != nil {
			p.Builtins = ids
		}
	}
	for _, c := range m.Custom {
		p.Custom = append(p.Custom, client.TerminalService{
			Name: c.Name.ValueString(),
			Exe:  c.Exe.ValueString(),
			Args: c.Args.ValueString(),
		})
	}
	return p, diags
}

func groupTerminalServicesFromClient(ctx context.Context, groupID string, p *client.GroupTerminalServices, planCustomNull bool) (groupTerminalServicesResourceModel, diag.Diagnostics) {
	ids, diags := types.ListValueFrom(ctx, types.StringType, p.Builtins)
	var custom []terminalServiceModel
	for _, c := range p.Custom {
		custom = append(custom, terminalServiceModel{
			Name: types.StringValue(c.Name),
			Exe:  types.StringValue(c.Exe),
			Args: types.StringValue(c.Args),
		})
	}
	if len(custom) == 0 && planCustomNull {
		custom = nil
	}
	return groupTerminalServicesResourceModel{
		ID:       types.StringValue(groupID),
		GroupID:  types.StringValue(groupID),
		Builtins: ids,
		Custom:   custom,
	}, diags
}

func (r *groupTerminalServicesResource) put(ctx context.Context, m groupTerminalServicesResourceModel) (*groupTerminalServicesResourceModel, diag.Diagnostics) {
	groupID := m.GroupID.ValueString()
	in, diags := groupTerminalServicesToClient(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	saved, err := r.client.PutGroupTerminalServices(ctx, groupID, in)
	if err != nil {
		diags.AddError("Error saving group terminal services", err.Error())
		return nil, diags
	}
	out, d := groupTerminalServicesFromClient(ctx, groupID, saved, m.Custom == nil)
	diags.Append(d...)
	return &out, diags
}

func (r *groupTerminalServicesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupTerminalServicesResourceModel
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

func (r *groupTerminalServicesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupTerminalServicesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupTerminalServices(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group terminal services", err.Error())
		return
	}
	m, diags := groupTerminalServicesFromClient(ctx, groupID, p, state.Custom == nil)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *groupTerminalServicesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupTerminalServicesResourceModel
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

// Delete clears the list (nothing expected).
func (r *groupTerminalServicesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupTerminalServicesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupTerminalServices(ctx, state.GroupID.ValueString(), client.DefaultGroupTerminalServices())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error clearing group terminal services", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_terminal_services.labs <group-id>`.
func (r *groupTerminalServicesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
