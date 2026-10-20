package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &permissionRoleAssignmentResource{}
	_ resource.ResourceWithConfigure   = &permissionRoleAssignmentResource{}
	_ resource.ResourceWithImportState = &permissionRoleAssignmentResource{}
)

func NewPermissionRoleAssignmentResource() resource.Resource {
	return &permissionRoleAssignmentResource{}
}

type permissionRoleAssignmentResource struct {
	client *client.Client
}

// permissionRoleAssignmentResourceModel — one user holding one role
// over one scope.
//
// Every attribute is RequiresReplace and there is no Update, which is
// not timidity: an assignment has no mutable state at all. It IS the
// (role, user, scope) tuple — exactly what the server's two partial
// unique indexes enforce — so changing any of them names a different
// assignment rather than modifying this one. The server's API has no
// update endpoint for the same reason.
//
// Its own resource rather than a nested block on rustinion_permission_
// role so that adding or removing a person never shows up as a diff on
// the role definition, the same split rustinion_notification_route
// already uses under rustinion_notification_channel.
//
// Import id is `role_id/assignment_id`, not a bare id, since an
// assignment is only ever readable through its parent role's own list
// endpoint — the same shape rustinion_on_call_override needs.
type permissionRoleAssignmentResourceModel struct {
	ID      types.String `tfsdk:"id"`
	RoleID  types.String `tfsdk:"role_id"`
	UserID  types.String `tfsdk:"user_id"`
	GroupID types.String `tfsdk:"group_id"`
}

func (r *permissionRoleAssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_role_assignment"
}

func (r *permissionRoleAssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Assigns a rustinion_permission_role to a user, optionally scoped to one group. The user must already hold a role in the tenant — an assignment refines access, it never confers membership. Has no in-place update: an assignment is its (role, user, scope) tuple, so changing any attribute replaces it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned assignment ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"role_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The role being assigned (`rustinion_permission_role.example.id`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The user receiving it (`users.id`). Reference `rustinion_user.example.user_id` — **not** that resource's `id`, which is its `user_roles` row id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Scope the assignment to one group. Omit for a tenant-wide assignment covering every group.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *permissionRoleAssignmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func permissionRoleAssignmentFromClient(roleID string, in *client.RoleAssignment) permissionRoleAssignmentResourceModel {
	groupID := types.StringNull()
	if in.GroupID != nil {
		groupID = types.StringValue(*in.GroupID)
	}
	// The list endpoint returns role_id on each row; the create
	// response does too. Fall back to the one we asked about so state
	// is never left with an empty parent id.
	effectiveRoleID := in.RoleID
	if effectiveRoleID == "" {
		effectiveRoleID = roleID
	}
	return permissionRoleAssignmentResourceModel{
		ID:      types.StringValue(in.ID),
		RoleID:  types.StringValue(effectiveRoleID),
		UserID:  types.StringValue(in.UserID),
		GroupID: groupID,
	}
}

func (r *permissionRoleAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan permissionRoleAssignmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var groupID *string
	if !plan.GroupID.IsNull() {
		v := plan.GroupID.ValueString()
		groupID = &v
	}
	created, err := r.client.CreateRoleAssignment(ctx, plan.RoleID.ValueString(), client.RoleAssignment{
		UserID:  plan.UserID.ValueString(),
		GroupID: groupID,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error assigning permission role", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionRoleAssignmentFromClient(plan.RoleID.ValueString(), created))...)
}

func (r *permissionRoleAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state permissionRoleAssignmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	assignment, err := r.client.GetRoleAssignment(ctx, state.RoleID.ValueString(), state.ID.ValueString())
	if err != nil {
		// A deleted parent role cascades its assignments away, so a
		// 404 on either the role or the assignment means the same
		// thing here: this resource is gone.
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading permission role assignment", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionRoleAssignmentFromClient(state.RoleID.ValueString(), assignment))...)
}

// Update is unreachable: every attribute is RequiresReplace, so the
// framework always destroys and recreates instead of routing a diff
// here. It exists only to satisfy resource.Resource.
func (r *permissionRoleAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Permission role assignments cannot be updated in place",
		"Every attribute of rustinion_permission_role_assignment forces replacement; this should be unreachable.",
	)
}

func (r *permissionRoleAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state permissionRoleAssignmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteRoleAssignment(ctx, state.RoleID.ValueString(), state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error removing permission role assignment", err.Error())
	}
}

func (r *permissionRoleAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	roleID, assignmentID, found := splitImportID(req.ID)
	if !found {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: role_id/assignment_id. Got: %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_id"), roleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), assignmentID)...)
}
