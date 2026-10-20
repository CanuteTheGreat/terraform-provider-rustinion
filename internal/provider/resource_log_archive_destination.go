package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource              = &logArchiveDestinationResource{}
	_ resource.ResourceWithConfigure = &logArchiveDestinationResource{}
	// Deliberately NOT ResourceWithImportState, mirroring
	// resource_audit_stream_destination.go's own precedent: an imported
	// destination's secret_access_key could never be populated -- the
	// customer API never returns it to any caller, at any point, ever
	// (unlike audit_stream_destination's own signing_secret, which is at
	// least available once, right after Create) -- so an import would
	// always produce a resource with an empty secret in state, worse
	// than not supporting import at all. Create a new destination
	// through Terraform instead of importing an existing one.
)

func NewLogArchiveDestinationResource() resource.Resource {
	return &logArchiveDestinationResource{}
}

type logArchiveDestinationResource struct {
	client *client.Client
}

// logArchiveDestinationResourceModel's `SecretAccessKey` needs no
// signing_secret-style Read-preservation *mechanism* the way
// audit_stream_destination's own Read must actively apply: the server's
// real GET response for this field is always absent (never even sent as
// null), so Read here simply never touches this attribute at all.
// Create/Update instead set it directly from the plan's own submitted
// value -- the only place it's ever known. `AccessKeyID`/
// `SecretAccessKey` are both schema'd `RequiresReplace`: the customer
// API has no update path for either (see the client's own doc comment),
// so a config change to one must destroy-and-recreate this resource
// rather than call Update with a value the server would silently ignore
// (which would otherwise leave Terraform's own state permanently
// diverged from the real remote destination).
type logArchiveDestinationResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Bucket          types.String `tfsdk:"bucket"`
	Region          types.String `tfsdk:"region"`
	AccessKeyID     types.String `tfsdk:"access_key_id"`
	SecretAccessKey types.String `tfsdk:"secret_access_key"`
	Prefix          types.String `tfsdk:"prefix"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

func (r *logArchiveDestinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_archive_destination"
}

func (r *logArchiveDestinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion long-term S3 log/metric archival destination: archives aging device_logs/device_metrics_samples/custom_metric_samples rows to a customer-owned S3 bucket before retention deletes them. One destination per tenant.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket": schema.StringAttribute{
				Required: true,
			},
			"region": schema.StringAttribute{
				Required: true,
			},
			"access_key_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The AWS IAM access key id for a customer-owned bucket the customer has already created and granted access to. Not updatable -- rotating credentials means destroying and recreating this resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"secret_access_key": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "The AWS IAM secret access key, paired with access_key_id. Write-only: the API never returns this value at any point, not even once, so it is not updatable via Update either (a real, stated v1 scope cut) -- rotate it by destroying and recreating this resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"prefix": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "An optional key prefix under the bucket. Empty string (the default) archives directly at the bucket root.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Defaults to true on create.",
			},
			"created_at": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *logArchiveDestinationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// logArchiveDestinationFromClient fills every attribute EXCEPT
// secret_access_key from a real API response -- that one field has no
// counterpart on the client struct at all (the server never returns it),
// so callers set it separately from the plan's own value, per this
// resource's own doc comment.
func logArchiveDestinationFromClient(d *client.LogArchiveDestination) logArchiveDestinationResourceModel {
	return logArchiveDestinationResourceModel{
		ID:          types.StringValue(d.ID),
		Bucket:      types.StringValue(d.Bucket),
		Region:      types.StringValue(d.Region),
		AccessKeyID: types.StringValue(d.AccessKeyID),
		Prefix:      types.StringValue(d.Prefix),
		Enabled:     types.BoolValue(d.Enabled),
		CreatedAt:   types.StringValue(d.CreatedAt),
	}
}

func (r *logArchiveDestinationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan logArchiveDestinationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateLogArchiveDestination(ctx, client.LogArchiveDestinationCreate{
		Bucket:          plan.Bucket.ValueString(),
		Region:          plan.Region.ValueString(),
		AccessKeyID:     plan.AccessKeyID.ValueString(),
		SecretAccessKey: plan.SecretAccessKey.ValueString(),
		Prefix:          plan.Prefix.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating log archive destination", err.Error())
		return
	}

	state := logArchiveDestinationFromClient(created)
	// The server never echoes this back at all -- the plan's own
	// submitted value is the only place it's ever known.
	state.SecretAccessKey = plan.SecretAccessKey
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Read is called on every `terraform plan`/refresh. GetLogArchiveDestination
// never includes secret_access_key in its response at all (see the
// client struct's own doc comment), so this simply never overwrites that
// one attribute -- it stays whatever was already in state.
func (r *logArchiveDestinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state logArchiveDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.GetLogArchiveDestination(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading log archive destination", err.Error())
		return
	}

	newState := logArchiveDestinationFromClient(d)
	newState.SecretAccessKey = state.SecretAccessKey // preserved, see doc comment above
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

// Update only ever runs for a bucket/region/prefix/enabled change --
// access_key_id/secret_access_key are both schema'd RequiresReplace, so
// a change to either destroys and recreates this resource instead of
// reaching this method at all.
func (r *logArchiveDestinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan logArchiveDestinationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state logArchiveDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateLogArchiveDestination(ctx, state.ID.ValueString(), client.LogArchiveDestinationUpdate{
		Bucket:  plan.Bucket.ValueString(),
		Region:  plan.Region.ValueString(),
		Prefix:  plan.Prefix.ValueString(),
		Enabled: plan.Enabled.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating log archive destination", err.Error())
		return
	}

	newState := logArchiveDestinationFromClient(updated)
	newState.SecretAccessKey = state.SecretAccessKey // never updatable, see doc comment above
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *logArchiveDestinationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state logArchiveDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteLogArchiveDestination(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting log archive destination", err.Error())
	}
}
