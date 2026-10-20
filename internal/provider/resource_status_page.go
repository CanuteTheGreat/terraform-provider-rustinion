package provider

import (
	"context"
	"fmt"
	"regexp"

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
	_ resource.Resource                = &statusPageResource{}
	_ resource.ResourceWithConfigure   = &statusPageResource{}
	_ resource.ResourceWithImportState = &statusPageResource{}
)

func NewStatusPageResource() resource.Resource {
	return &statusPageResource{}
}

type statusPageResource struct {
	client *client.Client
}

// statusPageComponentModel is one `component {}` block. Components have
// no `id` or `position` attribute exposed, same reason a dashboard
// widget doesn't: `PUT` deletes and reinserts every component wholesale
// in list order, so a component's server-assigned id/position aren't
// stable across updates — see `client.StatusPageComponent`'s own doc
// comment.
type statusPageComponentModel struct {
	SyntheticCheckID types.String `tfsdk:"synthetic_check_id"`
	DisplayName      types.String `tfsdk:"display_name"`
}

// statusPageResourceModel. Unlike every other resource in this
// provider, there is exactly one status page per tenant
// (`status_pages.tenant_id` is unique) — `rustinion_status_page` models
// that as a singleton: `group_id` isn't part of this schema at all
// (there's nothing to scope by, unlike every other resource here), and
// `PUT /api/v1/customer/status-page` is a genuine upsert (creates the
// page if none exists yet, otherwise fully replaces it), so this
// resource's Create and Update both just call that same endpoint.
// Applying this resource against a tenant that already has a status
// page configured (e.g. from the admin UI) overwrites it unconditionally,
// matching the API's own upsert semantics rather than failing with a
// conflict.
type statusPageResourceModel struct {
	ID        types.String               `tfsdk:"id"`
	Slug      types.String               `tfsdk:"slug"`
	Title     types.String               `tfsdk:"title"`
	Component []statusPageComponentModel `tfsdk:"component"`
}

func (r *statusPageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
}

func (r *statusPageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the tenant's one public status page (there is exactly one per tenant — see this resource's own doc comment for how that singleton shape is modeled). See status_pages.rs in the main repo for uptime computation and incident lifecycle; this resource only manages the page's own slug/title/components. Manual incidents are managed by the separate rustinion_status_page_incident resource, since a page has many incidents rather than the singleton shape this resource has.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-assigned status page ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"slug": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "3-63 lowercase letters, digits, or hyphens; must be unique across every tenant (it's the public page's own URL segment).",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-z0-9-]{3,63}$`),
						"must be 3-63 lowercase letters, digits, or hyphens",
					),
				},
			},
			"title": schema.StringAttribute{
				Required: true,
			},
		},
		Blocks: map[string]schema.Block{
			"component": schema.ListNestedBlock{
				MarkdownDescription: "One status page component, in display order. Each must reference a synthetic check already belonging to this tenant.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"synthetic_check_id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "ID of the synthetic check this component's uptime is derived from.",
						},
						"display_name": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
		},
	}
}

func (r *statusPageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func statusPageToClient(m statusPageResourceModel) client.StatusPage {
	components := make([]client.StatusPageComponent, 0, len(m.Component))
	for _, c := range m.Component {
		components = append(components, client.StatusPageComponent{
			SyntheticCheckID: c.SyntheticCheckID.ValueString(),
			DisplayName:      c.DisplayName.ValueString(),
		})
	}
	return client.StatusPage{
		Slug:       m.Slug.ValueString(),
		Title:      m.Title.ValueString(),
		Components: components,
	}
}

func statusPageFromClient(page *client.StatusPage) statusPageResourceModel {
	components := make([]statusPageComponentModel, 0, len(page.Components))
	for _, c := range page.Components {
		components = append(components, statusPageComponentModel{
			SyntheticCheckID: types.StringValue(c.SyntheticCheckID),
			DisplayName:      types.StringValue(c.DisplayName),
		})
	}
	return statusPageResourceModel{
		ID:        types.StringValue(page.ID),
		Slug:      types.StringValue(page.Slug),
		Title:     types.StringValue(page.Title),
		Component: components,
	}
}

func (r *statusPageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.PutStatusPage(ctx, statusPageToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating status page", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, statusPageFromClient(created))...)
}

func (r *statusPageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	page, err := r.client.GetStatusPage(ctx)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading status page", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, statusPageFromClient(page))...)
}

func (r *statusPageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.PutStatusPage(ctx, statusPageToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating status page", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, statusPageFromClient(updated))...)
}

func (r *statusPageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.DeleteStatusPage(ctx); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting status page", err.Error())
	}
}

// ImportState — there is only ever one status page per tenant and
// GetStatusPage takes no id, so unlike every other resource's import
// (which passes the server-assigned id through to look the resource
// up), the value passed to `terraform import` here is never used to
// look anything up; Read() always fetches the tenant's one page
// regardless. Any placeholder works, e.g.
// `terraform import rustinion_status_page.example status-page`.
func (r *statusPageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
