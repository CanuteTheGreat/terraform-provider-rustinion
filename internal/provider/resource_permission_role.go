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
	_ resource.Resource                = &permissionRoleResource{}
	_ resource.ResourceWithConfigure   = &permissionRoleResource{}
	_ resource.ResourceWithImportState = &permissionRoleResource{}
)

func NewPermissionRoleResource() resource.Resource {
	return &permissionRoleResource{}
}

type permissionRoleResource struct {
	client *client.Client
}

// permissionRoleResourceModel — a named, reusable set of permissions
// that users are assigned to, as an alternative to hand-managing a
// rustinion_permission_grant per person per resource type.
//
// `permission` is a nested block list mirroring the API's own nested
// array, the same shape rustinion_alert_rule's `condition` and
// rustinion_dashboard's `widget` blocks already use, and PUT replaces
// the whole set. A role with no `permission` blocks is legal and means
// exactly what it says — it confers nothing on its holders.
//
// Who holds a role is deliberately NOT modelled here: that is
// rustinion_permission_role_assignment, a separate resource, so that
// adding or removing a person never shows up as a diff on the role
// definition itself. Same split rustinion_notification_channel and
// rustinion_notification_route already use.
type permissionRoleResourceModel struct {
	ID          types.String                    `tfsdk:"id"`
	Name        types.String                    `tfsdk:"name"`
	Description types.String                    `tfsdk:"description"`
	Permissions []permissionRolePermissionModel `tfsdk:"permission"`
}

type permissionRolePermissionModel struct {
	ResourceType    types.String `tfsdk:"resource_type"`
	PermissionLevel types.String `tfsdk:"permission_level"`
}

func (r *permissionRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_role"
}

func (r *permissionRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a named, reusable permission role: a set of resource-type permissions you assign to users, instead of granting each person each resource type individually. Assign it with rustinion_permission_role_assignment. See permissions.rs in the main repo for how a role resolves against a user's plain org/department role.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned role ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Role name, unique within the tenant case-insensitively — \"SRE\" and \"sre\" cannot both exist.",
			},
			"description": schema.StringAttribute{
				Optional: true,
			},
		},
		Blocks: map[string]schema.Block{
			"permission": schema.ListNestedBlock{
				MarkdownDescription: "What this role permits. At most one block per `resource_type`. Omitting every block is legal and means the role confers nothing.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"resource_type": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf("alert_rules", "dashboards", "synthetic_checks", "notification_channels"),
							},
						},
						"permission_level": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf("read", "write", "admin"),
							},
						},
					},
				},
			},
		},
	}
}

func (r *permissionRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func permissionRoleToClient(m permissionRoleResourceModel) client.PermissionRole {
	var description *string
	if !m.Description.IsNull() {
		v := m.Description.ValueString()
		description = &v
	}
	// Always a non-nil slice: an empty permission set is a real value
	// the server must be told about, and `null` would serialize
	// differently from `[]`.
	perms := make([]client.RolePermission, 0, len(m.Permissions))
	for _, p := range m.Permissions {
		perms = append(perms, client.RolePermission{
			ResourceType:    p.ResourceType.ValueString(),
			PermissionLevel: p.PermissionLevel.ValueString(),
		})
	}
	return client.PermissionRole{
		Name:        m.Name.ValueString(),
		Description: description,
		Permissions: perms,
	}
}

func permissionRoleFromClient(in *client.PermissionRole) permissionRoleResourceModel {
	description := types.StringNull()
	if in.Description != nil {
		description = types.StringValue(*in.Description)
	}
	var perms []permissionRolePermissionModel
	for _, p := range in.Permissions {
		perms = append(perms, permissionRolePermissionModel{
			ResourceType:    types.StringValue(p.ResourceType),
			PermissionLevel: types.StringValue(p.PermissionLevel),
		})
	}
	return permissionRoleResourceModel{
		ID:          types.StringValue(in.ID),
		Name:        types.StringValue(in.Name),
		Description: description,
		Permissions: perms,
	}
}

func (r *permissionRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan permissionRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreatePermissionRole(ctx, permissionRoleToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating permission role", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionRoleFromClient(created))...)
}

func (r *permissionRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state permissionRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	role, err := r.client.GetPermissionRole(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading permission role", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionRoleFromClient(role))...)
}

func (r *permissionRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan permissionRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state permissionRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdatePermissionRole(ctx, state.ID.ValueString(), permissionRoleToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating permission role", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionRoleFromClient(updated))...)
}

func (r *permissionRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state permissionRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeletePermissionRole(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting permission role", err.Error())
	}
}

func (r *permissionRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
