package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestJSONSubsetIgnoresServerDefaultsAndArrayOrder(t *testing.T) {
	var cfg, srv any
	_ = json.Unmarshal([]byte(`{"clipboard":{"to_host":true,"formats":["text","html"]}}`), &cfg)
	_ = json.Unmarshal([]byte(`{"clipboard":{"to_host":true,"from_host":false,"formats":["html","text"]},"files":{"upload":false}}`), &srv)
	if !jsonSubset(cfg, srv) {
		t.Fatal("a configuration that omits defaults and orders arrays differently must still match")
	}
	_ = json.Unmarshal([]byte(`{"clipboard":{"to_host":false}}`), &cfg)
	if jsonSubset(cfg, srv) {
		t.Fatal("a differing value must not match")
	}
	_ = json.Unmarshal([]byte(`{"clipboard":{"formats":["text"]}}`), &cfg)
	if jsonSubset(cfg, srv) {
		t.Fatal("a different array must not match")
	}
}

func TestRemoteAccessDLPModelKeepsConfiguredTextWhenEquivalent(t *testing.T) {
	v := &client.RemoteAccessDLP{
		Scope: "group", ScopeID: "g1",
		Policy:    json.RawMessage(`{"clipboard":{"to_host":true,"from_host":false},"files":{"upload":false}}`),
		Effective: json.RawMessage(`{"digest":"x"}`),
	}
	prior := types.StringValue(`{"clipboard":{"to_host":true}}`)
	m := remoteAccessDLPModel("group", "g1", prior, types.StringNull(), v)
	if m.PolicyJSON.ValueString() != prior.ValueString() {
		t.Fatalf("policy_json %q", m.PolicyJSON.ValueString())
	}
	if m.ID.ValueString() != "group:g1" || m.EffectiveJSON.ValueString() != `{"digest":"x"}` {
		t.Fatalf("model %+v", m)
	}
	// Drift: the server's policy no longer matches what Terraform wrote -> show the server's text.
	drift := types.StringValue(`{"clipboard":{"to_host":false}}`)
	m = remoteAccessDLPModel("group", "g1", drift, types.StringNull(), v)
	if m.PolicyJSON.ValueString() != string(v.Policy) {
		t.Fatalf("drift not shown: %q", m.PolicyJSON.ValueString())
	}
	// A preset in state stays a preset, with no policy_json in state.
	v.Preset = "support"
	m = remoteAccessDLPModel("group", "g1", types.StringNull(), types.StringValue("support"), v)
	if m.Preset.ValueString() != "support" || !m.PolicyJSON.IsNull() {
		t.Fatalf("preset model %+v", m)
	}
}

func TestRemoteAccessDLPValidation(t *testing.T) {
	r := &remoteAccessDLPResource{}
	if err := r.validate(remoteAccessDLPResourceModel{Scope: types.StringValue("group"), ScopeID: types.StringNull(), Preset: types.StringValue("kiosk"), PolicyJSON: types.StringNull()}); err == nil {
		t.Fatal("group without scope_id")
	}
	if err := r.validate(remoteAccessDLPResourceModel{Scope: types.StringValue("tenant"), Preset: types.StringNull(), PolicyJSON: types.StringNull()}); err == nil {
		t.Fatal("neither preset nor policy")
	}
	if err := r.validate(remoteAccessDLPResourceModel{Scope: types.StringValue("tenant"), Preset: types.StringNull(), PolicyJSON: types.StringValue("{")}); err == nil {
		t.Fatal("bad json")
	}
	if err := r.validate(remoteAccessDLPResourceModel{Scope: types.StringValue("tenant"), Preset: types.StringValue("kiosk"), PolicyJSON: types.StringNull()}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteAccessDLPClientRoundTrip(t *testing.T) {
	var calls []string
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut {
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			bodies = append(bodies, b)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"scope": "group", "scope_id": "g1", "preset": "kiosk", "policy": map[string]any{"input": map[string]any{"control": false}}, "effective": map[string]any{"digest": "d"}})
	}))
	defer server.Close()
	c := client.New(server.URL, "k")
	ctx := context.Background()
	if _, err := c.PutRemoteAccessDLP(ctx, "group", "g1", "kiosk", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PutRemoteAccessDLP(ctx, "tenant", "", "", json.RawMessage(`{"input":{"control":false}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PutRemoteAccessDLP(ctx, "device", "d1", "", nil); err != nil {
		t.Fatal(err)
	}
	if bodies[0]["preset"] != "kiosk" || bodies[1]["policy"] == nil {
		t.Fatalf("bodies %v", bodies)
	}
	if v, ok := bodies[2]["policy"]; !ok || v != nil {
		t.Fatalf("clear must send an explicit null policy, got %v", bodies[2])
	}
	if _, err := c.GetRemoteAccessDLP(ctx, "group", "g1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetRemoteAccessDLP(ctx, "nowhere", ""); err == nil {
		t.Fatal("unknown scope")
	}
	want := []string{
		"PUT /api/v1/customer/groups/g1/remote-access/dlp",
		"PUT /api/v1/customer/remote-access/dlp",
		"PUT /api/v1/customer/devices/d1/remote-access/dlp",
		"GET /api/v1/customer/groups/g1/remote-access/dlp",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %v", calls)
	}
}
