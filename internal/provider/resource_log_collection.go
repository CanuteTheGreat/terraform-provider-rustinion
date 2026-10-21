package provider

import (
	"context"
	"encoding/json"
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
	_ resource.Resource                = &logCollectionResource{}
	_ resource.ResourceWithConfigure   = &logCollectionResource{}
	_ resource.ResourceWithImportState = &logCollectionResource{}
)

// logCollectionResource backs both rustinion_group_log_collection
// (singleton per group) and rustinion_device_log_collection (per-device
// override).
type logCollectionResource struct {
	client *client.Client
	device bool
}

func NewGroupLogCollectionResource() resource.Resource  { return &logCollectionResource{} }
func NewDeviceLogCollectionResource() resource.Resource { return &logCollectionResource{device: true} }

type logCollectionModel struct {
	ID           types.String `tfsdk:"id"`
	TargetID     types.String `tfsdk:"target_id"`
	SettingsJSON types.String `tfsdk:"settings_json"`
	Effective    types.String `tfsdk:"effective_json"`
}

func (r *logCollectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	if r.device {
		resp.TypeName = req.ProviderTypeName + "_device_log_collection"
	} else {
		resp.TypeName = req.ProviderTypeName + "_group_log_collection"
	}
}

func (r *logCollectionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	what := "group"
	desc := "Manages a group's device log collection: on/off (`enabled`, default true), `min_severity` (default warning), `kernel_min_severity`, " +
		"`windows_channels`, `windows_always` (event IDs collected at any level), `files` ([{path, service, multiline_start, min_severity}]), " +
		"`containers` ({enabled, include, exclude, min_severity}), `syslog` ({enabled, udp_port, tcp_port, tls_port, tls_cert_path, tls_key_path}), " +
		"`max_lines_per_minute`, `buffer_max_mb`. Absent keys use the built-in defaults. Destroying the resource restores the defaults (collection on)."
	if r.device {
		what = "device"
		desc = "Manages one device's log collection override (same keys as rustinion_group_log_collection; absent = inherit from the group; " +
			"`files` are added to the group's). Destroying the resource removes the override."
	}
	resp.Schema = schema.Schema{
		Description: desc,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as target_id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"target_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The " + what + " id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"settings_json": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The settings as a JSON object (jsonencode). Validated by the server.",
			},
			"effective_json": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "What the " + what + "'s devices actually run with (defaults and inheritance applied).",
			},
		},
	}
}

func (r *logCollectionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// sameLogSettings: equal as JSON values, ignoring a server-reported
// `"enabled": true` (the default) the configuration didn't set.
func sameLogSettings(prior, server string) bool {
	var p, s map[string]any
	if json.Unmarshal([]byte(prior), &p) != nil || json.Unmarshal([]byte(server), &s) != nil {
		return false
	}
	if _, ok := p["enabled"]; !ok && s["enabled"] == true {
		delete(s, "enabled")
	}
	a, _ := json.Marshal(p)
	b, _ := json.Marshal(s)
	return sameJSON(string(a), string(b))
}

func (r *logCollectionResource) fromServer(id string, settings, effective json.RawMessage, prior string) logCollectionModel {
	server := string(settings)
	if server == "" || server == "null" {
		server = "{}"
	}
	text := server
	if prior != "" && sameLogSettings(prior, server) {
		text = prior
	}
	return logCollectionModel{
		ID:           types.StringValue(id),
		TargetID:     types.StringValue(id),
		SettingsJSON: types.StringValue(text),
		Effective:    types.StringValue(string(effective)),
	}
}

func (r *logCollectionResource) put(ctx context.Context, id, text string) (*logCollectionModel, error) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		return nil, fmt.Errorf("settings_json must be a JSON object: %w", err)
	}
	if r.device {
		saved, err := r.client.PutDeviceLogCollection(ctx, id, json.RawMessage(text))
		if err != nil {
			return nil, err
		}
		m := r.fromServer(id, saved.Override, saved.Effective, text)
		return &m, nil
	}
	saved, err := r.client.PutGroupLogCollection(ctx, id, json.RawMessage(text))
	if err != nil {
		return nil, err
	}
	m := r.fromServer(id, saved.Settings, saved.Effective, text)
	return &m, nil
}

func (r *logCollectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan logCollectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan.TargetID.ValueString(), plan.SettingsJSON.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error setting log collection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

func (r *logCollectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state logCollectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.TargetID.ValueString()
	if id == "" {
		id = state.ID.ValueString()
	}
	var m logCollectionModel
	if r.device {
		d, err := r.client.GetDeviceLogCollection(ctx, id)
		if err != nil {
			if client.IsNotFound(err) {
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.AddError("Error reading device log collection", err.Error())
			return
		}
		m = r.fromServer(id, d.Override, d.Effective, state.SettingsJSON.ValueString())
	} else {
		g, err := r.client.GetGroupLogCollection(ctx, id)
		if err != nil {
			if client.IsNotFound(err) {
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.AddError("Error reading group log collection", err.Error())
			return
		}
		m = r.fromServer(id, g.Settings, g.Effective, state.SettingsJSON.ValueString())
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *logCollectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan logCollectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.put(ctx, plan.TargetID.ValueString(), plan.SettingsJSON.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating log collection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, saved)...)
}

// Delete restores the defaults (group: collection on, built-in settings;
// device: no override).
func (r *logCollectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state logCollectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var err error
	if r.device {
		_, err = r.client.PutDeviceLogCollection(ctx, state.TargetID.ValueString(), json.RawMessage(`null`))
	} else {
		_, err = r.client.PutGroupLogCollection(ctx, state.TargetID.ValueString(), json.RawMessage(`{"enabled":true}`))
	}
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting log collection", err.Error())
	}
}

// ImportState takes the group or device id.
func (r *logCollectionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("target_id"), req.ID)...)
}
