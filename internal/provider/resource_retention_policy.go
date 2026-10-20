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
	_ resource.Resource                = &retentionPolicyResource{}
	_ resource.ResourceWithConfigure   = &retentionPolicyResource{}
	_ resource.ResourceWithImportState = &retentionPolicyResource{}
)

func NewRetentionPolicyResource() resource.Resource {
	return &retentionPolicyResource{}
}

type retentionPolicyResource struct {
	client *client.Client
}

// retentionPolicyResourceModel is a tenant singleton, like
// rustinion_status_page: no group_id, and the `id` attribute is a fixed
// placeholder rather than a server-assigned identifier, because the API
// addresses the policy with no id at all.
//
// Every attribute is Optional and *not* Computed: omitting one has to
// mean "clear this back to the platform default", which is exactly what
// the server's full-replace PUT does. Making them Computed would let
// the provider keep a stale value in state for an attribute the config
// no longer sets.
type retentionPolicyResourceModel struct {
	ID                                types.String `tfsdk:"id"`
	CustomMetricRawRetentionDays      types.Int64  `tfsdk:"custom_metric_raw_retention_days"`
	CustomMetricMinutelyRetentionDays types.Int64  `tfsdk:"custom_metric_minutely_retention_days"`
	CustomMetricHourlyRetentionDays   types.Int64  `tfsdk:"custom_metric_hourly_retention_days"`
	DeviceMetricsRawRetentionDays     types.Int64  `tfsdk:"device_metrics_raw_retention_days"`
	DeviceMetricsHourlyRetentionDays  types.Int64  `tfsdk:"device_metrics_hourly_retention_days"`
	DeviceLogRetentionDays            types.Int64  `tfsdk:"device_log_retention_days"`
}

// The `id` this resource always reports. The API addresses the policy
// with no id, so nothing is ever looked up by this value; it exists
// because Terraform requires every resource to have one.
const retentionPolicyID = "retention-policy"

func (r *retentionPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_retention_policy"
}

func optionalDaysAttribute(tier string) schema.Int64Attribute {
	return schema.Int64Attribute{
		Optional: true,
		MarkdownDescription: "How many days of " + tier + " to keep for this tenant, 1-3650. " +
			"Omit to use the platform-wide default for this tier.",
	}
}

func (r *retentionPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages this tenant's data retention policy: how long each storage tier is kept before the retention sweep prunes it. A tenant singleton — there is exactly one, and it always exists, so omitting an attribute clears that tier back to the platform-wide default rather than leaving it unchanged. Rollups are written incrementally as samples arrive, so shortening a raw tier never costs you the coarser ones.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "Always \"" + retentionPolicyID + "\" — the API addresses this singleton with no id.",
			},
			"custom_metric_raw_retention_days":      optionalDaysAttribute("raw custom-metric samples"),
			"custom_metric_minutely_retention_days": optionalDaysAttribute("the minutely custom-metric rollup"),
			"custom_metric_hourly_retention_days":   optionalDaysAttribute("the hourly custom-metric rollup"),
			"device_metrics_raw_retention_days":     optionalDaysAttribute("raw host-metric samples"),
			"device_metrics_hourly_retention_days":  optionalDaysAttribute("the hourly host-metric rollup"),
			"device_log_retention_days":             optionalDaysAttribute("log lines"),
		},
	}
}

func (r *retentionPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// optionalDays maps an unset/unknown Terraform Int64 to a nil pointer,
// which serializes as JSON null — the server's "use the platform
// default" signal.
func optionalDays(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	days := v.ValueInt64()
	return &days
}

func daysOrNull(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}

func retentionPolicyToClient(m retentionPolicyResourceModel) client.RetentionPolicy {
	return client.RetentionPolicy{
		CustomMetricRawRetentionDays:      optionalDays(m.CustomMetricRawRetentionDays),
		CustomMetricMinutelyRetentionDays: optionalDays(m.CustomMetricMinutelyRetentionDays),
		CustomMetricHourlyRetentionDays:   optionalDays(m.CustomMetricHourlyRetentionDays),
		DeviceMetricsRawRetentionDays:     optionalDays(m.DeviceMetricsRawRetentionDays),
		DeviceMetricsHourlyRetentionDays:  optionalDays(m.DeviceMetricsHourlyRetentionDays),
		DeviceLogRetentionDays:            optionalDays(m.DeviceLogRetentionDays),
	}
}

func retentionPolicyFromClient(p *client.RetentionPolicy) retentionPolicyResourceModel {
	return retentionPolicyResourceModel{
		ID:                                types.StringValue(retentionPolicyID),
		CustomMetricRawRetentionDays:      daysOrNull(p.CustomMetricRawRetentionDays),
		CustomMetricMinutelyRetentionDays: daysOrNull(p.CustomMetricMinutelyRetentionDays),
		CustomMetricHourlyRetentionDays:   daysOrNull(p.CustomMetricHourlyRetentionDays),
		DeviceMetricsRawRetentionDays:     daysOrNull(p.DeviceMetricsRawRetentionDays),
		DeviceMetricsHourlyRetentionDays:  daysOrNull(p.DeviceMetricsHourlyRetentionDays),
		DeviceLogRetentionDays:            daysOrNull(p.DeviceLogRetentionDays),
	}
}

// Create is a PUT, not a POST: the policy already exists for every
// tenant, so "creating" it means setting it — the same thing Update
// does. Same shape rustinion_status_page uses.
func (r *retentionPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan retentionPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	saved, err := r.client.PutRetentionPolicy(ctx, retentionPolicyToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error setting retention policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, retentionPolicyFromClient(saved))...)
}

func (r *retentionPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state retentionPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := r.client.GetRetentionPolicy(ctx)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading retention policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, retentionPolicyFromClient(policy))...)
}

func (r *retentionPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan retentionPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	saved, err := r.client.PutRetentionPolicy(ctx, retentionPolicyToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating retention policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, retentionPolicyFromClient(saved))...)
}

// Delete clears every override back to the platform default rather than
// removing anything: the policy row is the tenant itself and cannot be
// deleted, so "no longer managed by Terraform" has to mean "back to the
// defaults it would have had if it had never been managed".
func (r *retentionPolicyResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if _, err := r.client.PutRetentionPolicy(ctx, client.RetentionPolicy{}); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error clearing retention policy", err.Error())
	}
}

// ImportState — same as rustinion_status_page: there is only ever one
// policy per tenant and GetRetentionPolicy takes no id, so the value
// passed to `terraform import` is never used to look anything up. Any
// placeholder works, e.g.
// `terraform import rustinion_retention_policy.example retention-policy`.
func (r *retentionPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
