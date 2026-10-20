package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCreateAlertRuleRoundTripsEventBasedMetricTypes proves the three
// plain-opaque-string MetricType wire formats (k8s_event_count:<reason>,
// new_error_groups:<service_name>, error_rate:<service_name>:<operation_name>)
// survive a real Create request/response round trip through this client
// with no mangling or truncation — the metric_type field needs no
// client-side code at all for these, but that's exactly the claim this
// test verifies for real rather than by inspection.
func TestCreateAlertRuleRoundTripsEventBasedMetricTypes(t *testing.T) {
	metricTypes := []string{
		"k8s_event_count:OOMKilled",
		"new_error_groups:checkout-api",
		"error_rate:checkout-api:POST /charge",
	}

	var gotBody AlertRule
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := gotBody
		resp.ID = "rule-1"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	var conditions []AlertRuleCondition
	for _, mt := range metricTypes {
		conditions = append(conditions, AlertRuleCondition{
			MetricType:  mt,
			Comparator:  "gt",
			Threshold:   1,
			Aggregation: "avg",
		})
	}
	created, err := c.CreateAlertRule(context.Background(), AlertRule{
		GroupID:    "group-1",
		Name:       "event-based",
		Conditions: conditions,
	})
	if err != nil {
		t.Fatalf("CreateAlertRule returned an error: %v", err)
	}

	if len(gotBody.Conditions) != len(metricTypes) {
		t.Fatalf("server received %d conditions, want %d", len(gotBody.Conditions), len(metricTypes))
	}
	if len(created.Conditions) != len(metricTypes) {
		t.Fatalf("response decoded %d conditions, want %d", len(created.Conditions), len(metricTypes))
	}
	for i, want := range metricTypes {
		if gotBody.Conditions[i].MetricType != want {
			t.Errorf("request body condition[%d].MetricType = %q, want %q", i, gotBody.Conditions[i].MetricType, want)
		}
		if created.Conditions[i].MetricType != want {
			t.Errorf("response condition[%d].MetricType = %q, want %q", i, created.Conditions[i].MetricType, want)
		}
	}
}

// TestCreateAlertRuleRoundTripsMultiWindowBurnRateFields proves
// MwLongWindowMinutes/MwLongThreshold survive a real Create request/
// response round trip with no mangling — the two new fields closing
// the "no native multi-window/multi-burn-rate SLO alerting preset" gap.
func TestCreateAlertRuleRoundTripsMultiWindowBurnRateFields(t *testing.T) {
	longWindow := int64(360)
	longThreshold := 6.0

	var gotBody AlertRule
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := gotBody
		resp.ID = "rule-1"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreateAlertRule(context.Background(), AlertRule{
		GroupID: "group-1",
		Name:    "multi-window burn rate",
		Conditions: []AlertRuleCondition{
			{
				MetricType:          "slo_burn_rate:11111111-1111-1111-1111-111111111111",
				ConditionKind:       "slo_burn_rate_multi_window",
				Comparator:          "gt",
				Threshold:           14.4,
				WindowMinutes:       60,
				Aggregation:         "avg",
				MwLongWindowMinutes: &longWindow,
				MwLongThreshold:     &longThreshold,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateAlertRule returned an error: %v", err)
	}

	if len(gotBody.Conditions) != 1 || len(created.Conditions) != 1 {
		t.Fatalf("expected exactly 1 condition in request and response")
	}
	for name, cond := range map[string]AlertRuleCondition{"request": gotBody.Conditions[0], "response": created.Conditions[0]} {
		if cond.MwLongWindowMinutes == nil || *cond.MwLongWindowMinutes != longWindow {
			t.Errorf("%s: MwLongWindowMinutes = %v, want %d", name, cond.MwLongWindowMinutes, longWindow)
		}
		if cond.MwLongThreshold == nil || *cond.MwLongThreshold != longThreshold {
			t.Errorf("%s: MwLongThreshold = %v, want %v", name, cond.MwLongThreshold, longThreshold)
		}
	}
}
