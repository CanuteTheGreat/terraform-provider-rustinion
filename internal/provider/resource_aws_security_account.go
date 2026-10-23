package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &awsSecurityAccountResource{}
	_ resource.ResourceWithConfigure   = &awsSecurityAccountResource{}
	_ resource.ResourceWithImportState = &awsSecurityAccountResource{}
)

func NewAwsSecurityAccountResource() resource.Resource {
	return &awsSecurityAccountResource{}
}

type awsSecurityAccountResource struct {
	client *client.Client
}

type awsSecurityAccountModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	AccountID  types.String `tfsdk:"account_id"`
	RoleName   types.String `tfsdk:"role_name"`
	Regions    types.List   `tfsdk:"regions"`
	DeviceID   types.String `tfsdk:"device_id"`
	CloudTrail types.Bool   `tfsdk:"cloudtrail"`
	GuardDuty  types.Bool   `tfsdk:"guardduty"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	ExternalID types.String `tfsdk:"external_id"`
	RoleARN    types.String `tfsdk:"role_arn"`
}

func (r *awsSecurityAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_security_account"
}

func (r *awsSecurityAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Connects an AWS account so Rustinion reads its CloudTrail management events and GuardDuty findings through a read-only cross-account role. Rustinion makes up an external id (`external_id`); create the role in the account with that external id as the trust condition, for example with an `aws_iam_role` that uses this resource's `external_id`, then the role is assumed every five minutes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The account's id in Rustinion.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A name for the account (up to 80 characters).",
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The 12-digit AWS account id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"role_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("RustinionSecurityRead"),
				MarkdownDescription: "Name of the IAM role Rustinion assumes in the account. Default `RustinionSecurityRead`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"regions": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{types.StringValue("us-east-1")})),
				MarkdownDescription: "Regions to read (up to 8). Default `[\"us-east-1\"]`.",
			},
			"device_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The enrolled device the CloudTrail log lines are stored on (its group's detection rules run over them).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cloudtrail": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Read CloudTrail management events. Default true.",
			},
			"guardduty": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Read GuardDuty findings. Default true.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Poll the account. Default true.",
			},
			"external_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The external id the role's trust policy must require (`sts:ExternalId`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"role_arn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ARN of the role Rustinion assumes.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *awsSecurityAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func awsAccountToClient(ctx context.Context, m awsSecurityAccountModel) (client.AwsSecurityAccount, diag.Diagnostics) {
	var diags diag.Diagnostics
	a := client.AwsSecurityAccount{
		Name:       m.Name.ValueString(),
		AccountID:  m.AccountID.ValueString(),
		RoleName:   m.RoleName.ValueString(),
		DeviceID:   m.DeviceID.ValueString(),
		CloudTrail: true,
		GuardDuty:  true,
		Enabled:    true,
		Regions:    listToStrings(ctx, m.Regions, &diags),
	}
	if !m.CloudTrail.IsNull() && !m.CloudTrail.IsUnknown() {
		a.CloudTrail = m.CloudTrail.ValueBool()
	}
	if !m.GuardDuty.IsNull() && !m.GuardDuty.IsUnknown() {
		a.GuardDuty = m.GuardDuty.ValueBool()
	}
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		a.Enabled = m.Enabled.ValueBool()
	}
	return a, diags
}

func awsAccountFromClient(ctx context.Context, a *client.AwsSecurityAccount) (awsSecurityAccountModel, diag.Diagnostics) {
	regions, d := types.ListValueFrom(ctx, types.StringType, a.Regions)
	return awsSecurityAccountModel{
		ID:         types.StringValue(a.ID),
		Name:       types.StringValue(a.Name),
		AccountID:  types.StringValue(a.AccountID),
		RoleName:   types.StringValue(a.RoleName),
		Regions:    regions,
		DeviceID:   types.StringValue(a.DeviceID),
		CloudTrail: types.BoolValue(a.CloudTrail),
		GuardDuty:  types.BoolValue(a.GuardDuty),
		Enabled:    types.BoolValue(a.Enabled),
		ExternalID: types.StringValue(a.ExternalID),
		RoleARN:    types.StringValue(a.RoleARN),
	}, d
}

func (r *awsSecurityAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan awsSecurityAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, diags := awsAccountToClient(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.client.CreateAwsSecurityAccount(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error connecting the AWS account", err.Error())
		return
	}
	m, d := awsAccountFromClient(ctx, saved)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *awsSecurityAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state awsSecurityAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.client.GetAwsSecurityAccount(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading the AWS account", err.Error())
		return
	}
	m, d := awsAccountFromClient(ctx, a)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *awsSecurityAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state awsSecurityAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, diags := awsAccountToClient(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.client.UpdateAwsSecurityAccount(ctx, state.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Error updating the AWS account", err.Error())
		return
	}
	m, d := awsAccountFromClient(ctx, saved)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *awsSecurityAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state awsSecurityAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAwsSecurityAccount(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error removing the AWS account", err.Error())
	}
}

// ImportState takes the account id in Rustinion.
func (r *awsSecurityAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
