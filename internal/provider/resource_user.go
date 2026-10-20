package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &userResource{}
	_ resource.ResourceWithConfigure   = &userResource{}
	_ resource.ResourceWithImportState = &userResource{}
)

func NewUserResource() resource.Resource {
	return &userResource{}
}

type userResource struct {
	client *client.Client
}

// userResourceModel. `email` and `group_id` are both fixed at invite
// time — there is no "rename" or "move to a different group"
// capability anywhere in this app, admin UI included — so both are
// `RequiresReplace`. `access_level` is the one attribute genuinely
// updatable in place (a real `PUT`, matching `routes::admin::
// update_role_access_level`'s own capability, just as a direct "set"
// instead of a toggle — a toggle would be actively dangerous for
// Terraform to call idempotently on every apply).
type userResourceModel struct {
	ID          types.String `tfsdk:"id"`
	UserID      types.String `tfsdk:"user_id"`
	Email       types.String `tfsdk:"email"`
	GroupID     types.String `tfsdk:"group_id"`
	AccessLevel types.String `tfsdk:"access_level"`
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Invites a tenant member — one user_roles row, joined with its underlying users row for email. Sends the same one-time password-set email the admin UI's own invite does (silently skipped if the server has no SMTP configured). email and group_id are fixed at invite time (no rename or move-to-a-different-group capability exists anywhere in this app); access_level (admin or read_only) can be changed in place at any time. Destroying this resource removes the role and revokes the user's sessions if this was their last remaining access anywhere.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned role ID (`user_roles.id`) — this resource's own identity.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The underlying cross-tenant user identity (`users.id`) — reference this elsewhere (e.g. an `rustinion_on_call_schedule` member's own `user_id`), not `id`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Fixed at invite time — there is no rename capability anywhere in this app.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Scopes this member to one group (a department admin/read-only). Omit for org-wide access. Fixed at invite time — there is no capability to move a member to a different group later.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"access_level": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("admin"),
				MarkdownDescription: "\"admin\" (default) or \"read_only\". Updatable in place at any time.",
				Validators:          []validator.String{stringvalidator.OneOf("admin", "read_only")},
			},
		},
	}
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func userToClient(m userResourceModel) client.User {
	u := client.User{
		Email:       m.Email.ValueString(),
		AccessLevel: m.AccessLevel.ValueString(),
	}
	if !m.GroupID.IsNull() {
		v := m.GroupID.ValueString()
		u.GroupID = &v
	}
	return u
}

func userFromClient(u *client.User) userResourceModel {
	return userResourceModel{
		ID:          types.StringValue(u.ID),
		UserID:      types.StringValue(u.UserID),
		Email:       types.StringValue(u.Email),
		GroupID:     types.StringPointerValue(u.GroupID),
		AccessLevel: types.StringValue(u.AccessLevel),
	}
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateUser(ctx, userToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating user", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, userFromClient(created))...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := r.client.GetUser(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, userFromClient(u))...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateUserAccessLevel(ctx, state.ID.ValueString(), plan.AccessLevel.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating user access level", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, userFromClient(updated))...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteUser(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting user", err.Error())
	}
}

func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
