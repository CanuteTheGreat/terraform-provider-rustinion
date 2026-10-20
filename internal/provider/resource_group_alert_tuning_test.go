package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// A replace sends every tuning key: the ones in the config with their
// values, all others as null so the server clears them.
func TestGroupAlertTuningReplaceClearsOmittedKeys(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if r.Method == http.MethodPut {
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"group_id":  "g1",
			"settings":  map[string]any{"oom_alert_scope": "host_services", "cpu_alert_enabled": false},
			"effective": map[string]any{},
		})
	}))
	defer server.Close()

	c := client.New(server.URL, "k")
	saved, err := c.ReplaceGroupAlertTuning(context.Background(), "g1",
		map[string]any{"oom_alert_scope": "host_services", "cpu_alert_enabled": false})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/customer/groups/g1/alert-tuning" {
		t.Fatalf("got %s %s", gotMethod, gotPath)
	}
	if gotBody["oom_alert_scope"] != "host_services" || gotBody["cpu_alert_enabled"] != false {
		t.Fatalf("body %v", gotBody)
	}
	if v, ok := gotBody["disk_alert_threshold_pct"]; !ok || v != nil {
		t.Fatalf("omitted keys must be sent as null, body %v", gotBody)
	}
	if len(gotBody) != len(client.AlertTuningKeys) {
		t.Fatalf("expected %d keys, got %d", len(client.AlertTuningKeys), len(gotBody))
	}
	m, err := alertTuningFromClient("g1", saved)
	if err != nil {
		t.Fatal(err)
	}
	// Same text Terraform's jsonencode produces: sorted keys, compact.
	if want := `{"cpu_alert_enabled":false,"oom_alert_scope":"host_services"}`; m.Settings.ValueString() != want {
		t.Fatalf("settings %s, want %s", m.Settings.ValueString(), want)
	}
	if _, err := c.GetGroupAlertTuning(context.Background(), "g1"); err != nil || gotMethod != http.MethodGet {
		t.Fatalf("get: %v %s", err, gotMethod)
	}
}

func TestGroupAlertTuningSettingsMustBeAnObject(t *testing.T) {
	if _, err := parseSettings("[1,2]"); err == nil {
		t.Fatal("a JSON array must be rejected")
	}
	if m, err := parseSettings(`{"disk_alert_threshold_pct": 90}`); err != nil || m["disk_alert_threshold_pct"] != float64(90) {
		t.Fatalf("%v %v", m, err)
	}
}
