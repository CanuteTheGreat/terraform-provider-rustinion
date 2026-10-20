package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &maintenanceWindowResource{}
	_ resource.ResourceWithConfigure   = &maintenanceWindowResource{}
	_ resource.ResourceWithImportState = &maintenanceWindowResource{}
)

func NewMaintenanceWindowResource() resource.Resource { return &maintenanceWindowResource{} }

type maintenanceWindowResource struct{ client *client.Client }

// Like the admin UI, a maintenance window has no edit: every attribute
// forces a replacement, and destroying it cancels it server-side
// (`cancelled_at`) so its history stays visible.
type maintenanceWindowResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Scope       types.String `tfsdk:"scope"`
	GroupID     types.String `tfsdk:"group_id"`
	StartsAt    types.String `tfsdk:"starts_at"`
	EndsAt      types.String `tfsdk:"ends_at"`
	Recurrence  types.String `tfsdk:"recurrence"`
	CancelledAt types.String `tfsdk:"cancelled_at"`
}

func (r *maintenanceWindowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_window"
}

func (r *maintenanceWindowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Manages a maintenance window: notifications for one group (or the whole organization) are held during the window, once or repeating daily/weekly. No in-place update; destroying it cancels it.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"scope": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "`group` (needs `group_id`) or `tenant` (the whole organization). Defaults to `group` when `group_id` is set, else `tenant`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"group_id":  schema.StringAttribute{Optional: true, PlanModifiers: replace},
			"starts_at": schema.StringAttribute{Required: true, MarkdownDescription: "RFC3339, e.g. `2030-01-06T02:00:00Z`.", PlanModifiers: replace},
			"ends_at":   schema.StringAttribute{Required: true, MarkdownDescription: "RFC3339, after `starts_at`. A daily window must span under 24 hours, a weekly one under 7 days.", PlanModifiers: replace},
			"recurrence": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("none"),
				MarkdownDescription: "`none` (default), `daily` or `weekly`: repeats the same span from `starts_at` until cancelled.",
				PlanModifiers:       replace,
			},
			"cancelled_at": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *maintenanceWindowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configuredClient(req, resp); c != nil {
		r.client = c
	}
}

func maintenanceWindowFromClient(w *client.MaintenanceWindow) maintenanceWindowResourceModel {
	return maintenanceWindowResourceModel{
		ID:          types.StringValue(w.ID),
		Name:        types.StringValue(w.Name),
		Scope:       types.StringValue(w.Scope),
		GroupID:     types.StringPointerValue(w.GroupID),
		StartsAt:    types.StringValue(w.StartsAt),
		EndsAt:      types.StringValue(w.EndsAt),
		Recurrence:  types.StringValue(w.Recurrence),
		CancelledAt: types.StringPointerValue(w.CancelledAt),
	}
}

func (r *maintenanceWindowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan maintenanceWindowResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := client.MaintenanceWindow{
		Name:       plan.Name.ValueString(),
		GroupID:    plan.GroupID.ValueStringPointer(),
		StartsAt:   plan.StartsAt.ValueString(),
		EndsAt:     plan.EndsAt.ValueString(),
		Recurrence: plan.Recurrence.ValueString(),
	}
	if !plan.Scope.IsNull() && !plan.Scope.IsUnknown() {
		in.Scope = plan.Scope.ValueString()
	}
	created, err := r.client.CreateMaintenanceWindow(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating maintenance window", err.Error())
		return
	}
	state := maintenanceWindowFromClient(created)
	// Keep the configured timestamp spelling (the server returns RFC3339
	// in UTC, which can differ textually from an equivalent offset).
	state.StartsAt, state.EndsAt = plan.StartsAt, plan.EndsAt
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *maintenanceWindowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state maintenanceWindowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.GetMaintenanceWindow(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading maintenance window", err.Error())
		return
	}
	next := maintenanceWindowFromClient(w)
	if sameInstant(state.StartsAt.ValueString(), w.StartsAt) {
		next.StartsAt = state.StartsAt
	}
	if sameInstant(state.EndsAt.ValueString(), w.EndsAt) {
		next.EndsAt = state.EndsAt
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *maintenanceWindowResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("maintenance windows cannot be updated in place", "Every attribute of rustinion_maintenance_window forces a replacement; this method should never run.")
}

func (r *maintenanceWindowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state maintenanceWindowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CancelMaintenanceWindow(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error cancelling maintenance window", err.Error())
	}
}

func (r *maintenanceWindowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
