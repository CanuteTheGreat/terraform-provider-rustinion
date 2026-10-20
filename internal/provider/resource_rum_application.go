package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// sameInstant reports whether two RFC3339 timestamps name the same
// moment, so a configured "+02:00" spelling doesn't diff against the
// server's UTC "Z" one.
func sameInstant(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	return errA == nil && errB == nil && ta.Equal(tb)
}

var (
	_ resource.Resource                = &rumApplicationResource{}
	_ resource.ResourceWithConfigure   = &rumApplicationResource{}
	_ resource.ResourceWithImportState = &rumApplicationResource{}
)

func NewRUMApplicationResource() resource.Resource { return &rumApplicationResource{} }

type rumApplicationResource struct{ client *client.Client }

type rumApplicationResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	ClientToken types.String `tfsdk:"client_token"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (r *rumApplicationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rum_application"
}

func (r *rumApplicationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "Manages a real-user-monitoring (RUM) application. Put `client_token` into the page's rum.js snippet; page views, Web Vitals and JavaScript errors then appear under RUM. Renaming keeps the token.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"name": schema.StringAttribute{Required: true},
			"client_token": schema.StringAttribute{
				Computed: true, PlanModifiers: keep,
				MarkdownDescription: "Server-generated token for the browser snippet (`/v1/rum/<client_token>/events`). Public by design: it is visible in the page source.",
			},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
		},
	}
}

func (r *rumApplicationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configuredClient(req, resp); c != nil {
		r.client = c
	}
}

func rumFromClient(a *client.RUMApplication) rumApplicationResourceModel {
	return rumApplicationResourceModel{
		ID:          types.StringValue(a.ID),
		Name:        types.StringValue(a.Name),
		ClientToken: types.StringValue(a.ClientToken),
		CreatedAt:   types.StringValue(a.CreatedAt),
	}
}

func (r *rumApplicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan rumApplicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateRUMApplication(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating RUM application", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, rumFromClient(created))...)
}

func (r *rumApplicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state rumApplicationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.client.GetRUMApplication(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading RUM application", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, rumFromClient(a))...)
}

func (r *rumApplicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state rumApplicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.client.RenameRUMApplication(ctx, state.ID.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error renaming RUM application", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, rumFromClient(a))...)
}

func (r *rumApplicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state rumApplicationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRUMApplication(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting RUM application", err.Error())
	}
}

func (r *rumApplicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
