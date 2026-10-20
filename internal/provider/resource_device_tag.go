package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &deviceTagResource{}
	_ resource.ResourceWithConfigure   = &deviceTagResource{}
	_ resource.ResourceWithImportState = &deviceTagResource{}
)

func NewDeviceTagResource() resource.Resource { return &deviceTagResource{} }

type deviceTagResource struct{ client *client.Client }

// One tag on one device. Tags are trimmed and lowercased by the server
// (as in the admin UI), so "Production" and "production" are the same tag.
type deviceTagResourceModel struct {
	ID       types.String `tfsdk:"id"`
	DeviceID types.String `tfsdk:"device_id"`
	Tag      types.String `tfsdk:"tag"`
}

func normalizeDeviceTag(tag string) string { return strings.ToLower(strings.TrimSpace(tag)) }

func (r *deviceTagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_tag"
}

func (r *deviceTagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Manages one tag on one device. Tags drive device filters, tag-scoped silences and notification routes. The tag is stored trimmed and lowercased.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "`<device_id>/<tag>`, also the import ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"device_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"tag":       schema.StringAttribute{Required: true, MarkdownDescription: "1-100 characters, no `/`.", PlanModifiers: replace},
		},
	}
}

func (r *deviceTagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configuredClient(req, resp); c != nil {
		r.client = c
	}
}

func (r *deviceTagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan deviceTagResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	added, err := r.client.AddDeviceTag(ctx, plan.DeviceID.ValueString(), plan.Tag.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error adding device tag", err.Error())
		return
	}
	plan.ID = types.StringValue(added.DeviceID + "/" + added.Tag)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *deviceTagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state deviceTagResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tags, err := r.client.ListDeviceTags(ctx, state.DeviceID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading device tags", err.Error())
		return
	}
	want := normalizeDeviceTag(state.Tag.ValueString())
	for _, t := range tags.Tags {
		if t == want {
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *deviceTagResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("device tags cannot be updated in place", "Every attribute of rustinion_device_tag forces a replacement; this method should never run.")
}

func (r *deviceTagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state deviceTagResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.RemoveDeviceTag(ctx, state.DeviceID.ValueString(), state.Tag.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error removing device tag", err.Error())
	}
}

// ImportState takes `<device_id>/<tag>`.
func (r *deviceTagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	deviceID, tag, ok := strings.Cut(req.ID, "/")
	if !ok || deviceID == "" || tag == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected <device_id>/<tag>, got "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), deviceID+"/"+normalizeDeviceTag(tag))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("device_id"), deviceID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tag"), normalizeDeviceTag(tag))...)
}
