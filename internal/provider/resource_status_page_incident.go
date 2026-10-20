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
	_ resource.Resource                = &statusPageIncidentResource{}
	_ resource.ResourceWithConfigure   = &statusPageIncidentResource{}
	_ resource.ResourceWithImportState = &statusPageIncidentResource{}
)

func NewStatusPageIncidentResource() resource.Resource {
	return &statusPageIncidentResource{}
}

type statusPageIncidentResource struct {
	client *client.Client
}

// statusPageIncidentResourceModel. Unlike `rustinion_status_page`, an
// incident is not a singleton — a page has many of them — so this
// resource has a real, stable `id` like every other non-singleton
// resource in this provider, imported with a plain
// `ImportStatePassthroughID` rather than the placeholder-id shape
// `rustinion_status_page` needs.
//
// There is no `source` attribute at all: every incident this resource
// creates is server-side hardcoded `source = "manual"` — a Terraform
// user can never declare (or import into managing) an `auto`-sourced
// incident, since those are exclusively written by the server's own
// synthetic-check hysteresis and any write against one is rejected with
// a 409. `message` is write-only — it's the incident's own opening
// update, and has no Read counterpart, since the server never returns
// it back on the incident itself; it lives only in that opening
// update's own row. Because of that, Read always echoes back whatever
// value is already in state rather than fetching it; after an import
// (where state starts out empty), the next plan shows an ordinary diff
// for it and Update reconciles it into state without making any real
// API call for that field, rather than forcing a destroy/recreate.
//
// There is deliberately no nested "updates" block, and no separate
// resource for posting a follow-up update: unlike this resource's own
// bounded, editable state (title/component_id/status), a status-
// changing update is fundamentally a timestamped, append-only timeline
// entry with no "desired configuration" a declarative resource could
// converge towards on its own — the same "declarative model has no verb
// for an imperative action" reasoning `rustinion_audit_stream_
// destination` already established for its own test-send action. This
// resource still lets `status` be edited in place (it's real, bounded
// state on the incident itself, unlike a full update's own free-form
// message text): `Update` detects a `status` diff and posts a new
// update via `client.PostStatusPageIncidentUpdate` (using the current
// `message`) before the plain `title`/`component_id` `PUT`, since the
// server's own `PUT` has no `status` field at all — see `Update`'s own
// comment. What this resource does not do is expose a way to post an
// update whose message differs from the incident's own `message`
// attribute, or to see the full updates timeline at all; both remain
// this resource's own real, stated scope cut.
type statusPageIncidentResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Title       types.String `tfsdk:"title"`
	Status      types.String `tfsdk:"status"`
	ComponentID types.String `tfsdk:"component_id"`
	Message     types.String `tfsdk:"message"`
}

func (r *statusPageIncidentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_incident"
}

func (r *statusPageIncidentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages one manually-authored incident on the tenant's own status page. Only source = \"manual\" incidents can be managed this way — incidents the server itself opens from synthetic-check failures (source = \"auto\") are exclusively lifecycle-managed by that monitoring and any write against one is rejected by the API. See status_pages.rs in the main repo for that hysteresis logic.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned incident ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"title": schema.StringAttribute{
				Required: true,
			},
			"status": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of \"investigating\", \"monitoring\", or \"resolved\". Changing this posts a new timeline update (using the current `message`) via the customer API's dedicated updates endpoint, since the plain update endpoint this resource otherwise uses has no `status` field at all — see this resource's own doc comment.",
				Validators: []validator.String{
					stringvalidator.OneOf("investigating", "monitoring", "resolved"),
				},
			},
			"component_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "ID of the status page component this incident is about, if any. Must belong to this tenant's own status page.",
			},
			"message": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The incident's opening update message, posted once at creation. Write-only: the server does not return it back on the incident, so this resource always echoes back whatever is currently in config/state rather than re-reading it — see this resource's own doc comment.",
			},
		},
	}
}

func (r *statusPageIncidentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func statusPageIncidentFromClient(m statusPageIncidentResourceModel, in *client.StatusPageIncident) statusPageIncidentResourceModel {
	componentID := types.StringNull()
	if in.ComponentID != nil {
		componentID = types.StringValue(*in.ComponentID)
	}
	return statusPageIncidentResourceModel{
		ID:          types.StringValue(in.ID),
		Title:       types.StringValue(in.Title),
		Status:      types.StringValue(in.Status),
		ComponentID: componentID,
		// Write-only — the server never returns it, so the plan's own
		// value is kept as-is rather than overwritten with something
		// read back.
		Message: m.Message,
	}
}

func (r *statusPageIncidentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageIncidentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var componentID *string
	if !plan.ComponentID.IsNull() {
		v := plan.ComponentID.ValueString()
		componentID = &v
	}
	created, err := r.client.CreateStatusPageIncident(ctx, client.StatusPageIncidentCreateInput{
		Title:       plan.Title.ValueString(),
		ComponentID: componentID,
		Status:      plan.Status.ValueString(),
		Message:     plan.Message.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating status page incident", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, statusPageIncidentFromClient(plan, created))...)
}

func (r *statusPageIncidentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageIncidentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	incident, err := r.client.GetStatusPageIncident(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading status page incident", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, statusPageIncidentFromClient(state, incident))...)
}

func (r *statusPageIncidentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageIncidentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state statusPageIncidentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The server's own `PUT` has no `status` field at all (see this
	// resource's own doc comment on decision 4) — a status change only
	// ever happens through the dedicated updates endpoint, which always
	// requires an accompanying message. So a `status` diff is applied by
	// posting a new update with the current `message` value before the
	// plain `PUT`, rather than by that `PUT` itself.
	if !plan.Status.Equal(state.Status) {
		err := r.client.PostStatusPageIncidentUpdate(ctx, state.ID.ValueString(), plan.Status.ValueString(), plan.Message.ValueString())
		if err != nil {
			if client.IsConflict(err) {
				resp.Diagnostics.AddError(
					"Cannot modify an auto-managed incident",
					"This incident has source = \"auto\" and is exclusively managed by the status page's own synthetic-check monitoring: "+err.Error(),
				)
				return
			}
			resp.Diagnostics.AddError("Error posting a status page incident update", err.Error())
			return
		}
	}

	var componentID *string
	if !plan.ComponentID.IsNull() {
		v := plan.ComponentID.ValueString()
		componentID = &v
	}
	updated, err := r.client.UpdateStatusPageIncident(ctx, state.ID.ValueString(), client.StatusPageIncidentUpdateInput{
		Title:       plan.Title.ValueString(),
		ComponentID: componentID,
	})
	if err != nil {
		if client.IsConflict(err) {
			resp.Diagnostics.AddError(
				"Cannot modify an auto-managed incident",
				"This incident has source = \"auto\" and is exclusively managed by the status page's own synthetic-check monitoring: "+err.Error(),
			)
			return
		}
		resp.Diagnostics.AddError("Error updating status page incident", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, statusPageIncidentFromClient(plan, updated))...)
}

func (r *statusPageIncidentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageIncidentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteStatusPageIncident(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		if client.IsConflict(err) {
			resp.Diagnostics.AddError(
				"Cannot delete an auto-managed incident",
				"This incident has source = \"auto\" and is exclusively managed by the status page's own synthetic-check monitoring: "+err.Error(),
			)
			return
		}
		resp.Diagnostics.AddError("Error deleting status page incident", err.Error())
	}
}

func (r *statusPageIncidentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
