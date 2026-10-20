package provider

import (
	"context"
	"fmt"

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
	_ resource.Resource                = &groupResource{}
	_ resource.ResourceWithConfigure   = &groupResource{}
	_ resource.ResourceWithImportState = &groupResource{}
)

func NewGroupResource() resource.Resource {
	return &groupResource{}
}

type groupResource struct {
	client *client.Client
}

// groupResourceModel. `TemplateKey` is a real exception to how every
// other attribute in this provider works: the server accepts it on
// create and on a (re-)apply, but never returns it from any read — a
// group's manifest doesn't remember which template produced it. So
// `Read`/`Create`/`Update` here deliberately never derive `TemplateKey`
// from a server response; they always carry forward whatever the prior
// plan/state already held. Removing `template_key` from config entirely
// isn't supported once one has been applied — there's no "un-apply a
// template" capability server-side, only "apply a (possibly different)
// one" — attempting it is a real, explicit error, not silently ignored.
type groupResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	TemplateKey     types.String `tfsdk:"template_key"`
	ManifestVersion types.Int64  `tfsdk:"manifest_version"`
}

func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion group. A group has no rename anywhere in this app (admin UI included) — name is fixed at creation. template_key (re-)seeds the group's manifest/OS-update policy/compliance policy from a curated starter and can be changed in place at any time (it is one-directional: there is no way to remove a template's effect once applied, only to apply a different one). Deleting a group only succeeds if it has no devices and no outstanding enrollment tokens, matching the same database constraints that protect it everywhere else in this app — surfaced as a real Terraform error, not silently ignored, if it still has either.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned group ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Fixed at creation — there is no rename capability anywhere in this app.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"template_key": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "One of: linux_web_server, linux_database_server, windows_workstation, " +
					"macos_workstation (see templates.rs in the main repo for what each seeds). Omit for a blank " +
					"group. Never returned by the API on read — this provider preserves whatever value you last " +
					"set rather than fighting you every plan; the source of truth for whether it was ever actually " +
					"applied is the group's own manifest_version, which increments on every real apply.",
				Validators: []validator.String{
					stringvalidator.OneOf("linux_web_server", "linux_database_server", "windows_workstation", "macos_workstation"),
				},
			},
			"manifest_version": schema.Int64Attribute{
				Computed: true,
				MarkdownDescription: "Increments every time a template is (re-)applied. Read-only. " +
					"Deliberately has no UseStateForUnknown plan modifier — unlike id (which never " +
					"changes), this genuinely changes on a template_key update, so predicting it would " +
					"stay at its prior value would be wrong; it shows \"(known after apply)\" whenever " +
					"template_key changes, and its real prior value the rest of the time.",
			},
		},
	}
}

func (r *groupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func groupToClient(m groupResourceModel) client.Group {
	g := client.Group{Name: m.Name.ValueString()}
	if !m.TemplateKey.IsNull() {
		g.TemplateKey = m.TemplateKey.ValueString()
	}
	return g
}

// groupFromClient deliberately does not populate TemplateKey — see this
// file's own doc comment. Every caller must set it explicitly from
// whatever plan/state value is authoritative for that operation.
func groupFromClient(g *client.Group) groupResourceModel {
	return groupResourceModel{
		ID:              types.StringValue(g.ID),
		Name:            types.StringValue(g.Name),
		ManifestVersion: types.Int64Value(g.ManifestVersion),
	}
}

func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateGroup(ctx, groupToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating group", err.Error())
		return
	}
	model := groupFromClient(created)
	model.TemplateKey = plan.TemplateKey
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	g, err := r.client.GetGroup(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group", err.Error())
		return
	}
	model := groupFromClient(g)
	model.TemplateKey = state.TemplateKey
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.TemplateKey.IsNull() || plan.TemplateKey.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Cannot clear template_key",
			"Applying a template is one-directional — there is no server-side way to remove a template's "+
				"effect once applied. Set template_key to a real value instead of removing it from config.",
		)
		return
	}

	updated, err := r.client.ApplyGroupTemplate(ctx, state.ID.ValueString(), plan.TemplateKey.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error applying group template", err.Error())
		return
	}
	model := groupFromClient(updated)
	model.TemplateKey = plan.TemplateKey
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteGroup(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting group", err.Error())
	}
}

func (r *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
