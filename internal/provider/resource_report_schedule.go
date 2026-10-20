package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &reportScheduleResource{}
	_ resource.ResourceWithConfigure   = &reportScheduleResource{}
	_ resource.ResourceWithImportState = &reportScheduleResource{}
)

func NewReportScheduleResource() resource.Resource { return &reportScheduleResource{} }

type reportScheduleResource struct{ client *client.Client }

type reportScheduleResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	ReportType types.String `tfsdk:"report_type"`
	GroupID    types.String `tfsdk:"group_id"`
	Cadence    types.String `tfsdk:"cadence"`
	Format     types.String `tfsdk:"format"`
	Recipients types.List   `tfsdk:"recipients"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	LastSentAt types.String `tfsdk:"last_sent_at"`
}

func (r *reportScheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_report_schedule"
}

func (r *reportScheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a scheduled report emailed to a list of recipients (Reports in the admin UI): an uptime summary, SLO report or incident digest, daily, weekly or monthly.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{Required: true},
			"report_type": schema.StringAttribute{
				Required: true, MarkdownDescription: "`uptime_summary`, `slo_report` or `incident_digest`.",
			},
			"group_id": schema.StringAttribute{Optional: true, MarkdownDescription: "Limit the report to one group; omit for the whole organization."},
			"cadence":  schema.StringAttribute{Required: true, MarkdownDescription: "`daily`, `weekly` or `monthly`."},
			"format": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("html"),
				MarkdownDescription: "`html` (default, in the email body) or `csv` (attached).",
			},
			"recipients": schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Email addresses, at least one."},
			"enabled":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"last_sent_at": schema.StringAttribute{
				Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "When the report last went out (RFC3339), null before the first send.",
			},
		},
	}
}

func (r *reportScheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configuredClient(req, resp); c != nil {
		r.client = c
	}
}

func (r *reportScheduleResource) toClient(ctx context.Context, m reportScheduleResourceModel) (client.ReportSchedule, bool) {
	var recipients []string
	if diags := m.Recipients.ElementsAs(ctx, &recipients, false); diags.HasError() {
		return client.ReportSchedule{}, false
	}
	return client.ReportSchedule{
		Name:       m.Name.ValueString(),
		ReportType: m.ReportType.ValueString(),
		GroupID:    m.GroupID.ValueStringPointer(),
		Cadence:    m.Cadence.ValueString(),
		Format:     m.Format.ValueString(),
		Recipients: recipients,
		Enabled:    m.Enabled.ValueBool(),
	}, true
}

func reportFromClient(ctx context.Context, s *client.ReportSchedule) (reportScheduleResourceModel, bool) {
	recipients, diags := types.ListValueFrom(ctx, types.StringType, s.Recipients)
	if diags.HasError() {
		return reportScheduleResourceModel{}, false
	}
	return reportScheduleResourceModel{
		ID:         types.StringValue(s.ID),
		Name:       types.StringValue(s.Name),
		ReportType: types.StringValue(s.ReportType),
		GroupID:    types.StringPointerValue(s.GroupID),
		Cadence:    types.StringValue(s.Cadence),
		Format:     types.StringValue(s.Format),
		Recipients: recipients,
		Enabled:    types.BoolValue(s.Enabled),
		LastSentAt: types.StringPointerValue(s.LastSentAt),
	}, true
}

func (r *reportScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan reportScheduleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, ok := r.toClient(ctx, plan)
	if !ok {
		resp.Diagnostics.AddError("Invalid recipients", "recipients must be a list of strings")
		return
	}
	created, err := r.client.CreateReportSchedule(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating report schedule", err.Error())
		return
	}
	state, ok := reportFromClient(ctx, created)
	if !ok {
		resp.Diagnostics.AddError("Unexpected API response", "could not read recipients")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *reportScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state reportScheduleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetReportSchedule(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading report schedule", err.Error())
		return
	}
	next, ok := reportFromClient(ctx, s)
	if !ok {
		resp.Diagnostics.AddError("Unexpected API response", "could not read recipients")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *reportScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state reportScheduleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, ok := r.toClient(ctx, plan)
	if !ok {
		resp.Diagnostics.AddError("Invalid recipients", "recipients must be a list of strings")
		return
	}
	updated, err := r.client.UpdateReportSchedule(ctx, state.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Error updating report schedule", err.Error())
		return
	}
	next, ok := reportFromClient(ctx, updated)
	if !ok {
		resp.Diagnostics.AddError("Unexpected API response", "could not read recipients")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *reportScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state reportScheduleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteReportSchedule(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting report schedule", err.Error())
	}
}

func (r *reportScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
