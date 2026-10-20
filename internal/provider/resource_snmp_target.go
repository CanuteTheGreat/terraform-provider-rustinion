package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// configuredClient is the shared Configure body of the resources in this
// file family (SNMP target, device tag, maintenance window, RUM
// application, report schedule).
func configuredClient(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *client.Client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return nil
	}
	return c
}

var (
	_ resource.Resource                = &snmpTargetResource{}
	_ resource.ResourceWithConfigure   = &snmpTargetResource{}
	_ resource.ResourceWithImportState = &snmpTargetResource{}
)

func NewSNMPTargetResource() resource.Resource { return &snmpTargetResource{} }

type snmpTargetResource struct{ client *client.Client }

// snmpTargetResourceModel. community / v3_auth_key / v3_priv_key are
// write-only on the server (sealed at rest, never returned), so Read keeps
// whatever is in state for them; the has_* attributes show what the
// server actually holds.
type snmpTargetResourceModel struct {
	ID                  types.String `tfsdk:"id"`
	GroupID             types.String `tfsdk:"group_id"`
	Name                types.String `tfsdk:"name"`
	Host                types.String `tfsdk:"host"`
	Port                types.Int64  `tfsdk:"port"`
	Version             types.String `tfsdk:"version"`
	Community           types.String `tfsdk:"community"`
	V3Username          types.String `tfsdk:"v3_username"`
	V3AuthProtocol      types.String `tfsdk:"v3_auth_protocol"`
	V3AuthKey           types.String `tfsdk:"v3_auth_key"`
	V3PrivProtocol      types.String `tfsdk:"v3_priv_protocol"`
	V3PrivKey           types.String `tfsdk:"v3_priv_key"`
	PollIntervalSeconds types.Int64  `tfsdk:"poll_interval_seconds"`
	Enabled             types.Bool   `tfsdk:"enabled"`
	HasCommunity        types.Bool   `tfsdk:"has_community"`
	HasV3AuthKey        types.Bool   `tfsdk:"has_v3_auth_key"`
	HasV3PrivKey        types.Bool   `tfsdk:"has_v3_priv_key"`
}

func (r *snmpTargetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snmp_target"
}

func (r *snmpTargetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, MarkdownDescription: desc}
	}
	secret := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: desc + " Write-only: stored encrypted and never returned by the API, so a change made outside Terraform is not detected (see the matching `has_*` attribute)."}
	}
	resp.Schema = schema.Schema{
		Description: "Manages an SNMP target: a switch, router, printer or UPS that a group's agents poll over SNMP (v2c or v3), the same as Monitoring -> SNMP targets in the admin UI.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"group_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Group whose agents poll the target. Changing it recreates the target.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Required: true},
			"host": schema.StringAttribute{Required: true, MarkdownDescription: "Hostname or IP address the agents poll."},
			"port": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(161)},
			"version": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("v2c"),
				MarkdownDescription: "`v2c` (default) or `v3`.",
			},
			"community":        secret("SNMPv2c community string."),
			"v3_username":      optString("SNMPv3 user name (required for `version = \"v3\"`)."),
			"v3_auth_protocol": optString("SNMPv3 authentication protocol: `sha1` or `sha2`."),
			"v3_auth_key":      secret("SNMPv3 authentication key."),
			"v3_priv_protocol": optString("SNMPv3 privacy protocol: `des`, `aes128`, `aes192` or `aes256`."),
			"v3_priv_key":      secret("SNMPv3 privacy key."),
			"poll_interval_seconds": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(300),
				MarkdownDescription: "Seconds between polls, at least 30. Default 300.",
			},
			"enabled":         schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"has_community":   schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the server holds a community string."},
			"has_v3_auth_key": schema.BoolAttribute{Computed: true},
			"has_v3_priv_key": schema.BoolAttribute{Computed: true},
		},
	}
}

func (r *snmpTargetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configuredClient(req, resp); c != nil {
		r.client = c
	}
}

// secretValue: a configured secret is sent as is; an unset one is sent
// as "" (clear), so removing it from the configuration clears it.
func secretValue(v types.String) *string {
	s := ""
	if !v.IsNull() && !v.IsUnknown() {
		s = v.ValueString()
	}
	return &s
}

func snmpTargetToClient(m snmpTargetResourceModel) client.SNMPTarget {
	return client.SNMPTarget{
		Name:                m.Name.ValueString(),
		Host:                m.Host.ValueString(),
		Port:                m.Port.ValueInt64(),
		Version:             m.Version.ValueString(),
		Community:           secretValue(m.Community),
		V3Username:          m.V3Username.ValueStringPointer(),
		V3AuthProtocol:      m.V3AuthProtocol.ValueStringPointer(),
		V3AuthKey:           secretValue(m.V3AuthKey),
		V3PrivProtocol:      m.V3PrivProtocol.ValueStringPointer(),
		V3PrivKey:           secretValue(m.V3PrivKey),
		PollIntervalSeconds: m.PollIntervalSeconds.ValueInt64(),
		Enabled:             m.Enabled.ValueBool(),
	}
}

// snmpTargetFromClient keeps the write-only secrets from `prior`.
func snmpTargetFromClient(t *client.SNMPTarget, prior snmpTargetResourceModel) snmpTargetResourceModel {
	return snmpTargetResourceModel{
		ID:                  types.StringValue(t.ID),
		GroupID:             types.StringValue(t.GroupID),
		Name:                types.StringValue(t.Name),
		Host:                types.StringValue(t.Host),
		Port:                types.Int64Value(t.Port),
		Version:             types.StringValue(t.Version),
		Community:           prior.Community,
		V3Username:          types.StringPointerValue(t.V3Username),
		V3AuthProtocol:      types.StringPointerValue(t.V3AuthProtocol),
		V3AuthKey:           prior.V3AuthKey,
		V3PrivProtocol:      types.StringPointerValue(t.V3PrivProtocol),
		V3PrivKey:           prior.V3PrivKey,
		PollIntervalSeconds: types.Int64Value(t.PollIntervalSeconds),
		Enabled:             types.BoolValue(t.Enabled),
		HasCommunity:        types.BoolValue(t.HasCommunity),
		HasV3AuthKey:        types.BoolValue(t.HasV3AuthKey),
		HasV3PrivKey:        types.BoolValue(t.HasV3PrivKey),
	}
}

func (r *snmpTargetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan snmpTargetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateSNMPTarget(ctx, plan.GroupID.ValueString(), snmpTargetToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating SNMP target", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, snmpTargetFromClient(created, plan))...)
}

func (r *snmpTargetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state snmpTargetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.GetSNMPTarget(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading SNMP target", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, snmpTargetFromClient(t, state))...)
}

func (r *snmpTargetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state snmpTargetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateSNMPTarget(ctx, state.ID.ValueString(), snmpTargetToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating SNMP target", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, snmpTargetFromClient(updated, plan))...)
}

func (r *snmpTargetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state snmpTargetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSNMPTarget(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting SNMP target", err.Error())
	}
}

// ImportState: the secrets can't be imported (the API never returns
// them); set them in configuration and apply once to store them again.
func (r *snmpTargetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
