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
	_ resource.Resource                = &permissionGrantResource{}
	_ resource.ResourceWithConfigure   = &permissionGrantResource{}
	_ resource.ResourceWithImportState = &permissionGrantResource{}
)

func NewPermissionGrantResource() resource.Resource {
	return &permissionGrantResource{}
}

type permissionGrantResource struct {
	client *client.Client
}

// permissionGrantResourceModel — one fine-grained permission grant,
// overriding a user's plain role for exactly one resource type in one
// scope. See permissions.rs in the main repo for the resolution order.
//
// Three of the four attributes are RequiresReplace, and that is not
// timidity: a grant's identity genuinely IS its (user_id, group_id,
// resource_type) tuple, enforced server-side by two partial unique
// indexes. Changing any of them does not modify a grant, it names a
// different one.
//
// permission_level, by contrast, updates in place with no
// destroy/recreate, because the server's POST is an upsert that
// returns the same id with the new level. That matters more here than
// it would for an ordinary resource: Terraform's default update
// strategy for a RequiresReplace attribute is destroy-then-create, and
// for a `read` grant that RESTRICTS an admin, the window between the
// two is one where the user silently regains full access. Routing a
// level change through the upsert removes that window entirely.
//
// Deliberately not modelled: there is no `email` convenience
// attribute. A grant is keyed on users.id, and resolving an email to
// an id would mean either a second lookup this provider has no
// endpoint for or a silent "create the user if missing" — reference
// `rustinion_user.example.user_id` instead (note: `user_id`, not `id`,
// which is that resource's own user_roles row id).
type permissionGrantResourceModel struct {
	ID              types.String `tfsdk:"id"`
	UserID          types.String `tfsdk:"user_id"`
	GroupID         types.String `tfsdk:"group_id"`
	ResourceType    types.String `tfsdk:"resource_type"`
	PermissionLevel types.String `tfsdk:"permission_level"`
}

func (r *permissionGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_grant"
}

func (r *permissionGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages one fine-grained permission grant: overrides a user's org/department role for a single kind of resource, in a single scope. A grant only applies to a user who already holds a role in this tenant (it never confers membership on its own) and never overrides a global admin. See permissions.rs in the main repo for the full resolution order.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned grant ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The user this grant applies to (`users.id`). Reference `rustinion_user.example.user_id` — **not** that resource's `id`, which is its `user_roles` row id. Changing this names a different grant, so it forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Scope this grant to one group. Omit for a tenant-wide grant covering every group, the same convention a user's own org-level role uses. Changing this names a different grant, so it forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of \"alert_rules\", \"dashboards\", \"synthetic_checks\", or \"notification_channels\". Changing this names a different grant, so it forces replacement.",
				Validators: []validator.String{
					stringvalidator.OneOf("alert_rules", "dashboards", "synthetic_checks", "notification_channels"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"permission_level": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of \"read\", \"write\", or \"admin\". Updates in place — the API upserts on the grant's own scope, so changing a level never destroys and recreates the grant (which would briefly restore the user's unrestricted role-level access).",
				Validators: []validator.String{
					stringvalidator.OneOf("read", "write", "admin"),
				},
			},
		},
	}
}

func (r *permissionGrantResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func permissionGrantFromClient(in *client.PermissionGrant) permissionGrantResourceModel {
	groupID := types.StringNull()
	if in.GroupID != nil {
		groupID = types.StringValue(*in.GroupID)
	}
	return permissionGrantResourceModel{
		ID:              types.StringValue(in.ID),
		UserID:          types.StringValue(in.UserID),
		GroupID:         groupID,
		ResourceType:    types.StringValue(in.ResourceType),
		PermissionLevel: types.StringValue(in.PermissionLevel),
	}
}

func permissionGrantToClient(m permissionGrantResourceModel) client.PermissionGrant {
	var groupID *string
	if !m.GroupID.IsNull() {
		v := m.GroupID.ValueString()
		groupID = &v
	}
	return client.PermissionGrant{
		UserID:          m.UserID.ValueString(),
		GroupID:         groupID,
		ResourceType:    m.ResourceType.ValueString(),
		PermissionLevel: m.PermissionLevel.ValueString(),
	}
}

func (r *permissionGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan permissionGrantResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreatePermissionGrant(ctx, permissionGrantToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating permission grant", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionGrantFromClient(created))...)
}

func (r *permissionGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state permissionGrantResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	grant, err := r.client.GetPermissionGrant(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading permission grant", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionGrantFromClient(grant))...)
}

// Update only ever handles a permission_level change — every other
// attribute is RequiresReplace, so the framework never routes their
// diffs here. There is no PUT endpoint: the level change goes through
// the same POST as Create, whose server side upserts on the grant's
// own (user_id, group_id, resource_type) scope and returns the SAME
// id. The assertion below is real belt-and-braces: if that upsert ever
// stopped being scope-keyed, silently writing a new id into state
// would strand the old grant in the database, still in force, with
// nothing tracking it.
func (r *permissionGrantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan permissionGrantResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state permissionGrantResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.CreatePermissionGrant(ctx, permissionGrantToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating permission grant", err.Error())
		return
	}
	if updated.ID != state.ID.ValueString() {
		resp.Diagnostics.AddError(
			"Permission grant was recreated rather than updated",
			fmt.Sprintf(
				"Updating the level returned a new grant id (%s, was %s). The API is expected to upsert on the grant's own scope; the previous grant may still exist and remain in force.",
				updated.ID, state.ID.ValueString(),
			),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionGrantFromClient(updated))...)
}

func (r *permissionGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state permissionGrantResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeletePermissionGrant(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting permission grant", err.Error())
	}
}

func (r *permissionGrantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
