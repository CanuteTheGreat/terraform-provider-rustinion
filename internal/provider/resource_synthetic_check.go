package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ resource.Resource                = &syntheticCheckResource{}
	_ resource.ResourceWithConfigure   = &syntheticCheckResource{}
	_ resource.ResourceWithImportState = &syntheticCheckResource{}
)

func NewSyntheticCheckResource() resource.Resource {
	return &syntheticCheckResource{}
}

type syntheticCheckResource struct {
	client *client.Client
}

// syntheticCheckStepModel is one `step {}` block, only meaningful when
// the parent check's check_type is "http_multistep". Steps have no `id`
// attribute exposed for the same reason alert rule conditions/dashboard
// widgets don't — a full `PUT` replace deletes and reinserts every step
// wholesale.
type syntheticCheckStepModel struct {
	Name              types.String         `tfsdk:"name"`
	Method            types.String         `tfsdk:"method"`
	URL               types.String         `tfsdk:"url"`
	Headers           jsontypes.Normalized `tfsdk:"headers"`
	Body              types.String         `tfsdk:"body"`
	ExpectedStatusMin types.Int64          `tfsdk:"expected_status_min"`
	ExpectedStatusMax types.Int64          `tfsdk:"expected_status_max"`
	AssertBodyRegex   types.String         `tfsdk:"assert_body_regex"`
	AssertJSONPath    types.String         `tfsdk:"assert_json_path"`
	AssertJSONValue   types.String         `tfsdk:"assert_json_value"`
	CaptureVariable   types.String         `tfsdk:"capture_variable"`
	CaptureSource     types.String         `tfsdk:"capture_source"`
	CapturePath       types.String         `tfsdk:"capture_path"`
}

// syntheticCheckLocationModel is one `location {}` block — no `id`
// attribute exposed, same reasoning `step` blocks already established:
// a full `PUT` replace deletes and reinserts every location wholesale.
type syntheticCheckLocationModel struct {
	DeviceID types.String `tfsdk:"device_id"`
	Label    types.String `tfsdk:"label"`
}

type syntheticCheckResourceModel struct {
	ID                  types.String                  `tfsdk:"id"`
	GroupID             types.String                  `tfsdk:"group_id"`
	Name                types.String                  `tfsdk:"name"`
	CheckType           types.String                  `tfsdk:"check_type"`
	Target              types.String                  `tfsdk:"target"`
	HTTPMethod          types.String                  `tfsdk:"http_method"`
	ExpectedStatusMin   types.Int64                   `tfsdk:"expected_status_min"`
	ExpectedStatusMax   types.Int64                   `tfsdk:"expected_status_max"`
	TimeoutMs           types.Int64                   `tfsdk:"timeout_ms"`
	IntervalSeconds     types.Int64                   `tfsdk:"interval_seconds"`
	RelayMode           types.String                  `tfsdk:"relay_mode"`
	Enabled             types.Bool                    `tfsdk:"enabled"`
	MinFailingLocations types.Int64                   `tfsdk:"min_failing_locations"`
	DNSRecordType       types.String                  `tfsdk:"dns_record_type"`
	DNSServer           types.String                  `tfsdk:"dns_server"`
	ExpectedAnswer      types.String                  `tfsdk:"expected_answer"`
	TLSVerify           types.Bool                    `tfsdk:"tls_verify"`
	MaxResponseTimeMs   types.Int64                   `tfsdk:"max_response_time_ms"`
	CertWarningDays     types.Int64                   `tfsdk:"cert_warning_days"`
	CertCriticalDays    types.Int64                   `tfsdk:"cert_critical_days"`
	FailuresBeforeAlert types.Int64                   `tfsdk:"failures_before_alert"`
	Step                []syntheticCheckStepModel     `tfsdk:"step"`
	Location            []syntheticCheckLocationModel `tfsdk:"location"`
}

func (r *syntheticCheckResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_synthetic_check"
}

func (r *syntheticCheckResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Rustinion synthetic (uptime) check: a single HTTP(S) or TCP request, a TLS certificate check (expiry and trust), a ping, a DNS lookup, or an ordered sequence of HTTP steps (check_type = \"http_multistep\"). See synthetic.rs in the main repo for execution semantics, including the SSRF protections a direct-mode check's target is checked against on every scheduled run (not at creation time — this provider does not pre-validate the target either, matching the server's own admin-UI behavior).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"check_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of: http, tcp, tls (certificate expiry and trust for host[:port]), icmp (ping), dns, http_multistep.",
				Validators:          []validator.String{stringvalidator.OneOf("http", "tcp", "tls", "icmp", "dns", "http_multistep")},
			},
			"target": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Full URL for check_type = \"http\" (e.g. \"https://example.com/health\"), \"host:port\" for \"tcp\", \"host\" or \"host:port\" for \"tls\" (port 443 by default), a host name or IP for \"icmp\", the name to look up for \"dns\". Required unless check_type is \"http_multistep\", which the server always stores as the fixed placeholder \"(multi-step)\" regardless of what's sent here (a multi-step check's real target is its own ordered steps).",
				PlanModifiers: []planmodifier.String{
					syntheticCheckTargetPlanModifier(),
				},
			},
			"http_method": schema.StringAttribute{
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString("GET"),
				Validators: []validator.String{stringvalidator.OneOf("GET", "POST", "HEAD")},
			},
			"expected_status_min": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(200),
			},
			"expected_status_max": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(299),
			},
			"timeout_ms": schema.Int64Attribute{
				Optional:   true,
				Computed:   true,
				Default:    int64default.StaticInt64(10000),
				Validators: []validator.Int64{int64validator.Between(1, 60000)},
			},
			"interval_seconds": schema.Int64Attribute{
				Optional:   true,
				Computed:   true,
				Default:    int64default.StaticInt64(60),
				Validators: []validator.Int64{int64validator.AtLeast(30)},
			},
			"relay_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("direct"),
				MarkdownDescription: "\"direct\" (the server makes the request itself) or \"relay\" (queued to an enrolled agent — required for endpoints only reachable inside the customer's own network). A check_type of \"http_multistep\" must use \"direct\" — relay mode isn't supported for multi-step checks.",
				Validators:          []validator.String{stringvalidator.OneOf("direct", "relay")},
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"min_failing_locations": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
				MarkdownDescription: "How many of this check's own configured `location` blocks must be currently failing before it's considered down. 1 (the default) means any location failing is enough; set it to the full location count to require every location to agree. Meaningless until at least one `location` block exists — a check with none keeps the server's existing dynamic single-relay-device behavior, unaffected by this field.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"dns_record_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "check_type = \"dns\" only: A, AAAA, CNAME, MX, NS or TXT. Unset means A.",
				Validators:          []validator.String{stringvalidator.OneOf("A", "AAAA", "CNAME", "MX", "NS", "TXT")},
			},
			"dns_server": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "check_type = \"dns\" only: the DNS server to ask, an IP address with an optional port (\"10.0.0.53\", \"10.0.0.53:5353\"). Unset means a public resolver (1.1.1.1) for a direct check and the device's own resolver for a relay check. A direct check may only ask public servers and look up public names (single-label and .internal/.local/.lan/.home.arpa names, and answers with private addresses, are refused); use relay_mode = \"relay\" for internal ones.",
			},
			"expected_answer": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "check_type = \"dns\" only: comma-separated values that must all be in the answer (for MX, \"10 mx.example.com\" or just \"mx.example.com\"). Unset means any non-empty answer passes.",
			},
			"tls_verify": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "check_type = \"tls\": fail when the certificate isn't trusted or doesn't match the name. Set false for self-signed internal services (expiry is still checked). Default true.",
			},
			"max_response_time_ms": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Any check: a run slower than this many milliseconds counts as a failure. Unset means no limit.",
				Validators:          []validator.Int64{int64validator.Between(1, 60000)},
			},
			"cert_warning_days": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(30),
				MarkdownDescription: "tls and https checks: raise certificate_expiring (warning, summary email) when the certificate expires within this many days. 0 = only once expired. Default 30.",
				Validators:          []validator.Int64{int64validator.Between(0, 365)},
			},
			"cert_critical_days": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(7),
				MarkdownDescription: "tls and https checks: certificate_expiring becomes critical (sent right away) within this many days. Must not exceed cert_warning_days. Default 7.",
				Validators:          []validator.Int64{int64validator.Between(0, 365)},
			},
			"failures_before_alert": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
				MarkdownDescription: "Alert only after this many failed runs in a row, 1-10. Default 1 (the first failure).",
				Validators:          []validator.Int64{int64validator.Between(1, 10)},
			},
		},
		Blocks: map[string]schema.Block{
			"step": schema.ListNestedBlock{
				MarkdownDescription: "One HTTP step, in order. Only valid when check_type is \"http_multistep\"; the server rejects a non-empty step list on any other check_type.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required: true,
						},
						"method": schema.StringAttribute{
							Optional:   true,
							Computed:   true,
							Default:    stringdefault.StaticString("GET"),
							Validators: []validator.String{stringvalidator.OneOf("GET", "POST", "PUT", "PATCH", "DELETE", "HEAD")},
						},
						"url": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "May contain {{variable}} placeholders substituted from an earlier step's own capture in the same run.",
						},
						"headers": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							CustomType:          jsontypes.NormalizedType{},
							Default:             stringdefault.StaticString("{}"),
							MarkdownDescription: "Request headers as a JSON object string, e.g. {\"Authorization\": \"Bearer {{token}}\"}. Defaults to {}.",
						},
						"body": schema.StringAttribute{
							Optional: true,
						},
						"expected_status_min": schema.Int64Attribute{
							Optional: true,
							Computed: true,
							Default:  int64default.StaticInt64(200),
						},
						"expected_status_max": schema.Int64Attribute{
							Optional: true,
							Computed: true,
							Default:  int64default.StaticInt64(299),
						},
						"assert_body_regex": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Regex the response body must match, in addition to the status range.",
						},
						"assert_json_path": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Dotted path into the parsed JSON response body (e.g. \"data.items.0.status\"). Requires assert_json_value to also be set.",
						},
						"assert_json_value": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Expected value at assert_json_path. Requires assert_json_path to also be set.",
						},
						"capture_variable": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Name to bind this step's captured value to, usable as {{name}} in any later step's url/headers/body. Requires capture_source and capture_path.",
						},
						"capture_source": schema.StringAttribute{
							Optional:   true,
							Validators: []validator.String{stringvalidator.OneOf("header", "json_path")},
						},
						"capture_path": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "The header name or JSON path to capture from, depending on capture_source.",
						},
					},
				},
			},
			"location": schema.ListNestedBlock{
				MarkdownDescription: "One of this check's own group's enrolled devices, relaying it from wherever that device actually is — not a hosted multi-region probe network, your own agents. Only meaningful for relay_mode = \"relay\". See min_failing_locations for the aggregation this activates.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"device_id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "ID of a device in this check's own group_id.",
						},
						"label": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "A human-readable name for this location, e.g. \"us-east-office\".",
						},
					},
				},
			},
		},
	}
}

func (r *syntheticCheckResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func syntheticCheckToClient(m syntheticCheckResourceModel) client.SyntheticCheck {
	steps := make([]client.SyntheticCheckStep, 0, len(m.Step))
	for _, s := range m.Step {
		headers := s.Headers.ValueString()
		if headers == "" {
			headers = "{}"
		}
		step := client.SyntheticCheckStep{
			Name:              s.Name.ValueString(),
			Method:            s.Method.ValueString(),
			URL:               s.URL.ValueString(),
			Headers:           json.RawMessage(headers),
			ExpectedStatusMin: s.ExpectedStatusMin.ValueInt64(),
			ExpectedStatusMax: s.ExpectedStatusMax.ValueInt64(),
		}
		if !s.Body.IsNull() {
			v := s.Body.ValueString()
			step.Body = &v
		}
		if !s.AssertBodyRegex.IsNull() {
			v := s.AssertBodyRegex.ValueString()
			step.AssertBodyRegex = &v
		}
		if !s.AssertJSONPath.IsNull() {
			v := s.AssertJSONPath.ValueString()
			step.AssertJSONPath = &v
		}
		if !s.AssertJSONValue.IsNull() {
			v := s.AssertJSONValue.ValueString()
			step.AssertJSONValue = &v
		}
		if !s.CaptureVariable.IsNull() {
			v := s.CaptureVariable.ValueString()
			step.CaptureVariable = &v
		}
		if !s.CaptureSource.IsNull() {
			v := s.CaptureSource.ValueString()
			step.CaptureSource = &v
		}
		if !s.CapturePath.IsNull() {
			v := s.CapturePath.ValueString()
			step.CapturePath = &v
		}
		steps = append(steps, step)
	}
	locations := make([]client.SyntheticCheckLocation, 0, len(m.Location))
	for _, l := range m.Location {
		loc := client.SyntheticCheckLocation{DeviceID: l.DeviceID.ValueString()}
		if !l.Label.IsNull() {
			v := l.Label.ValueString()
			loc.Label = &v
		}
		locations = append(locations, loc)
	}
	return client.SyntheticCheck{
		GroupID:             m.GroupID.ValueString(),
		Name:                m.Name.ValueString(),
		CheckType:           m.CheckType.ValueString(),
		Target:              m.Target.ValueString(),
		HTTPMethod:          m.HTTPMethod.ValueString(),
		ExpectedStatusMin:   m.ExpectedStatusMin.ValueInt64(),
		ExpectedStatusMax:   m.ExpectedStatusMax.ValueInt64(),
		TimeoutMs:           m.TimeoutMs.ValueInt64(),
		IntervalSeconds:     m.IntervalSeconds.ValueInt64(),
		RelayMode:           m.RelayMode.ValueString(),
		Enabled:             m.Enabled.ValueBool(),
		MinFailingLocations: m.MinFailingLocations.ValueInt64(),
		DNSRecordType:       m.DNSRecordType.ValueStringPointer(),
		DNSServer:           m.DNSServer.ValueStringPointer(),
		ExpectedAnswer:      m.ExpectedAnswer.ValueStringPointer(),
		TLSVerify:           m.TLSVerify.IsNull() || m.TLSVerify.IsUnknown() || m.TLSVerify.ValueBool(),
		MaxResponseTimeMs:   m.MaxResponseTimeMs.ValueInt64Pointer(),
		CertWarningDays:     int64OrDefault(m.CertWarningDays, 30),
		CertCriticalDays:    int64OrDefault(m.CertCriticalDays, 7),
		FailuresBeforeAlert: int64OrDefault(m.FailuresBeforeAlert, 1),
		Steps:               steps,
		Locations:           locations,
	}
}

// int64OrDefault: a null/unknown attribute means the server default, never 0.
func int64OrDefault(v types.Int64, def int64) int64 {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return v.ValueInt64()
}

func syntheticCheckFromClient(check *client.SyntheticCheck) syntheticCheckResourceModel {
	steps := make([]syntheticCheckStepModel, 0, len(check.Steps))
	for _, s := range check.Steps {
		headers := string(s.Headers)
		if headers == "" {
			headers = "{}"
		}
		steps = append(steps, syntheticCheckStepModel{
			Name:              types.StringValue(s.Name),
			Method:            types.StringValue(s.Method),
			URL:               types.StringValue(s.URL),
			Headers:           jsontypes.NewNormalizedValue(headers),
			Body:              types.StringPointerValue(s.Body),
			ExpectedStatusMin: types.Int64Value(s.ExpectedStatusMin),
			ExpectedStatusMax: types.Int64Value(s.ExpectedStatusMax),
			AssertBodyRegex:   types.StringPointerValue(s.AssertBodyRegex),
			AssertJSONPath:    types.StringPointerValue(s.AssertJSONPath),
			AssertJSONValue:   types.StringPointerValue(s.AssertJSONValue),
			CaptureVariable:   types.StringPointerValue(s.CaptureVariable),
			CaptureSource:     types.StringPointerValue(s.CaptureSource),
			CapturePath:       types.StringPointerValue(s.CapturePath),
		})
	}
	locations := make([]syntheticCheckLocationModel, 0, len(check.Locations))
	for _, l := range check.Locations {
		locations = append(locations, syntheticCheckLocationModel{
			DeviceID: types.StringValue(l.DeviceID),
			Label:    types.StringPointerValue(l.Label),
		})
	}
	return syntheticCheckResourceModel{
		ID:                  types.StringValue(check.ID),
		GroupID:             types.StringValue(check.GroupID),
		Name:                types.StringValue(check.Name),
		CheckType:           types.StringValue(check.CheckType),
		Target:              types.StringValue(check.Target),
		HTTPMethod:          types.StringValue(check.HTTPMethod),
		ExpectedStatusMin:   types.Int64Value(check.ExpectedStatusMin),
		ExpectedStatusMax:   types.Int64Value(check.ExpectedStatusMax),
		TimeoutMs:           types.Int64Value(check.TimeoutMs),
		IntervalSeconds:     types.Int64Value(check.IntervalSeconds),
		RelayMode:           types.StringValue(check.RelayMode),
		Enabled:             types.BoolValue(check.Enabled),
		MinFailingLocations: types.Int64Value(check.MinFailingLocations),
		DNSRecordType:       types.StringPointerValue(check.DNSRecordType),
		DNSServer:           types.StringPointerValue(check.DNSServer),
		ExpectedAnswer:      types.StringPointerValue(check.ExpectedAnswer),
		TLSVerify:           types.BoolValue(check.TLSVerify),
		MaxResponseTimeMs:   types.Int64PointerValue(check.MaxResponseTimeMs),
		CertWarningDays:     types.Int64Value(check.CertWarningDays),
		CertCriticalDays:    types.Int64Value(check.CertCriticalDays),
		FailuresBeforeAlert: types.Int64Value(check.FailuresBeforeAlert),
		Step:                steps,
		Location:            locations,
	}
}

func (r *syntheticCheckResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan syntheticCheckResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateSyntheticCheck(ctx, syntheticCheckToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating synthetic check", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, syntheticCheckFromClient(created))...)
}

func (r *syntheticCheckResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state syntheticCheckResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	check, err := r.client.GetSyntheticCheck(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading synthetic check", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, syntheticCheckFromClient(check))...)
}

func (r *syntheticCheckResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan syntheticCheckResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state syntheticCheckResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateSyntheticCheck(ctx, state.ID.ValueString(), syntheticCheckToClient(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating synthetic check", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, syntheticCheckFromClient(updated))...)
}

func (r *syntheticCheckResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state syntheticCheckResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteSyntheticCheck(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting synthetic check", err.Error())
	}
}

func (r *syntheticCheckResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
