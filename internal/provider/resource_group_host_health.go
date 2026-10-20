package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &groupHostHealthResource{}
	_ resource.ResourceWithConfigure   = &groupHostHealthResource{}
	_ resource.ResourceWithImportState = &groupHostHealthResource{}
)

func NewGroupHostHealthResource() resource.Resource {
	return &groupHostHealthResource{}
}

type groupHostHealthResource struct {
	client *client.Client
}

type groupHostHealthResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	GroupID               types.String `tfsdk:"group_id"`
	SwapAlertEnabled      types.Bool   `tfsdk:"swap_alert_enabled"`
	SwapAlertThresholdPct types.Int64  `tfsdk:"swap_alert_threshold_pct"`
	IgnoredFailedServices types.List   `tfsdk:"ignored_failed_services"`
}

func (r *groupHostHealthResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_host_health"
}

func (r *groupHostHealthResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a group's host-health settings: the swap alert (swap_in_use) and the failed system services (systemd units, Windows services, launchd jobs, rc.d services) that never raise service_failed. Full-filesystem levels follow the group's disk alert levels (rustinion_group_alert_tuning). Destroying this resource puts the defaults back (swap alert on at 80%, no ignored services).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as group_id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The group these settings belong to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"swap_alert_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Alert when swap use reaches swap_alert_threshold_pct. Default true.",
			},
			"swap_alert_threshold_pct": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(80),
				MarkdownDescription: "Share of swap in use that raises the alert, 0-100 (0 = any swap use). Default 80.",
				Validators:          []validator.Int64{int64validator.Between(0, 100)},
			},
			"ignored_failed_services": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: "Failed services that never alert: exact names (`apt-daily.service`), or a prefix ending in `*` (`user@*`). Default none.",
			},
		},
	}
}

func (r *groupHostHealthResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func groupHostHealthToClient(ctx context.Context, m groupHostHealthResourceModel) (client.GroupHostHealth, diag.Diagnostics) {
	var diags diag.Diagnostics
	p := client.DefaultGroupHostHealth()
	if !m.SwapAlertEnabled.IsNull() && !m.SwapAlertEnabled.IsUnknown() {
		p.SwapAlertEnabled = m.SwapAlertEnabled.ValueBool()
	}
	if !m.SwapAlertThresholdPct.IsNull() && !m.SwapAlertThresholdPct.IsUnknown() {
		p.SwapAlertThresholdPct = m.SwapAlertThresholdPct.ValueInt64()
	}
	if !m.IgnoredFailedServices.IsNull() && !m.IgnoredFailedServices.IsUnknown() {
		var names []string
		diags.Append(m.IgnoredFailedServices.ElementsAs(ctx, &names, false)...)
		if names == nil {
			names = []string{}
		}
		p.IgnoredFailedServices = names
	}
	return p, diags
}

func groupHostHealthFromClient(ctx context.Context, groupID string, p *client.GroupHostHealth) (groupHostHealthResourceModel, diag.Diagnostics) {
	names, diags := types.ListValueFrom(ctx, types.StringType, p.IgnoredFailedServices)
	return groupHostHealthResourceModel{
		ID:                    types.StringValue(groupID),
		GroupID:               types.StringValue(groupID),
		SwapAlertEnabled:      types.BoolValue(p.SwapAlertEnabled),
		SwapAlertThresholdPct: types.Int64Value(p.SwapAlertThresholdPct),
		IgnoredFailedServices: names,
	}, diags
}

func (r *groupHostHealthResource) put(ctx context.Context, m groupHostHealthResourceModel) (*groupHostHealthResourceModel, diag.Diagnostics) {
	groupID := m.GroupID.ValueString()
	in, diags := groupHostHealthToClient(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	saved, err := r.client.PutGroupHostHealth(ctx, groupID, in)
	if err != nil {
		diags.AddError("Error saving group host health settings", err.Error())
		return nil, diags
	}
	out, d := groupHostHealthFromClient(ctx, groupID, saved)
	diags.Append(d...)
	return &out, diags
}

func (r *groupHostHealthResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupHostHealthResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, diags := r.put(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *groupHostHealthResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupHostHealthResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groupID := state.GroupID.ValueString()
	if groupID == "" {
		groupID = state.ID.ValueString()
	}
	p, err := r.client.GetGroupHostHealth(ctx, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group host health settings", err.Error())
		return
	}
	m, diags := groupHostHealthFromClient(ctx, groupID, p)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *groupHostHealthResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupHostHealthResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, diags := r.put(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

// Delete puts the defaults back: the settings are part of the group.
func (r *groupHostHealthResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupHostHealthResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PutGroupHostHealth(ctx, state.GroupID.ValueString(), client.DefaultGroupHostHealth())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting group host health settings", err.Error())
	}
}

// ImportState takes the group id:
// `terraform import rustinion_group_host_health.servers <group-id>`.
func (r *groupHostHealthResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), req.ID)...)
}
