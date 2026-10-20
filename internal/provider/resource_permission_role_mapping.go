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
	_ resource.Resource                = &permissionRoleMappingResource{}
	_ resource.ResourceWithConfigure   = &permissionRoleMappingResource{}
	_ resource.ResourceWithImportState = &permissionRoleMappingResource{}
)

func NewPermissionRoleMappingResource() resource.Resource {
	return &permissionRoleMappingResource{}
}

type permissionRoleMappingResource struct {
	client *client.Client
}

// permissionRoleMappingResourceModel — one IdP group name pointed at
// one permission role.
//
// Every attribute is RequiresReplace and there is no Update, because a
// mapping has no mutable state whatsoever: it IS its (source,
// external_group, role_id, group_id) tuple, which the server's two
// partial unique indexes enforce. Changing any attribute names a
// different mapping. The API has no PUT endpoint for the same reason.
//
// Destroying a mapping does not immediately revoke the assignments it
// produced — they are removed on each affected user's next sync (a
// SCIM group write, or that user's next OIDC sign-in). That is
// deliberate server-side behavior, not a provider limitation: see
// permissions.rs's delete_idp_mapping for why revoking outside a sync
// would be worse.
type permissionRoleMappingResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Source        types.String `tfsdk:"source"`
	ExternalGroup types.String `tfsdk:"external_group"`
	RoleID        types.String `tfsdk:"role_id"`
	GroupID       types.String `tfsdk:"group_id"`
}

func (r *permissionRoleMappingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_role_mapping"
}

func (r *permissionRoleMappingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Maps an identity-provider group onto a rustinion_permission_role, so directory membership decides who holds the role. SCIM mappings apply when the IdP pushes a group change; OIDC and SAML mappings apply at each sign-in, from the ID token's groups claim or the assertion's group attribute respectively. Assignments made by hand are never touched by a sync.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned mapping ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"source": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of \"scim\", \"oidc\" or \"saml\". Part of the mapping's identity, so changing it forces replacement.",
				Validators: []validator.String{
					stringvalidator.OneOf("scim", "oidc", "saml"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"external_group": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The IdP's own group name — a SCIM Group `displayName`, one entry of an OIDC `groups` claim, or one value of a SAML assertion's group attribute. Matched case-insensitively.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The role membership of that group confers (`rustinion_permission_role.example.id`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Scope the resulting assignment to one group. Omit for tenant-wide.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *permissionRoleMappingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func permissionRoleMappingFromClient(in *client.PermissionRoleMapping) permissionRoleMappingResourceModel {
	groupID := types.StringNull()
	if in.GroupID != nil {
		groupID = types.StringValue(*in.GroupID)
	}
	return permissionRoleMappingResourceModel{
		ID:            types.StringValue(in.ID),
		Source:        types.StringValue(in.Source),
		ExternalGroup: types.StringValue(in.ExternalGroup),
		RoleID:        types.StringValue(in.RoleID),
		GroupID:       groupID,
	}
}

func (r *permissionRoleMappingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan permissionRoleMappingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var groupID *string
	if !plan.GroupID.IsNull() {
		v := plan.GroupID.ValueString()
		groupID = &v
	}
	created, err := r.client.CreatePermissionRoleMapping(ctx, client.PermissionRoleMapping{
		Source:        plan.Source.ValueString(),
		ExternalGroup: plan.ExternalGroup.ValueString(),
		RoleID:        plan.RoleID.ValueString(),
		GroupID:       groupID,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating permission role mapping", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionRoleMappingFromClient(created))...)
}

func (r *permissionRoleMappingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state permissionRoleMappingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mapping, err := r.client.GetPermissionRoleMapping(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading permission role mapping", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionRoleMappingFromClient(mapping))...)
}

// Update is unreachable: every attribute is RequiresReplace, so the
// framework always destroys and recreates instead of routing a diff
// here. It exists only to satisfy resource.Resource.
func (r *permissionRoleMappingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Permission role mappings cannot be updated in place",
		"Every attribute of rustinion_permission_role_mapping forces replacement; this should be unreachable.",
	)
}

func (r *permissionRoleMappingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state permissionRoleMappingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeletePermissionRoleMapping(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting permission role mapping", err.Error())
	}
}

func (r *permissionRoleMappingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
