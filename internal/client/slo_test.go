package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSloSendsBearerAuthAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotMethod, gotPath string
	var gotBody Slo

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := gotBody
		resp.ID = "slo-123"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreateSlo(context.Background(), Slo{
		GroupID:          "group-1",
		Name:             "Fleet uptime",
		MetricType:       "checkin_freshness",
		TargetPercentage: 99.9,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("CreateSlo returned an error: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/customer/slos" {
		t.Errorf("path = %q, want /api/v1/customer/slos", gotPath)
	}
	if gotBody.MetricType != "checkin_freshness" {
		t.Errorf("request body metric_type = %q, want %q", gotBody.MetricType, "checkin_freshness")
	}
	if created.ID != "slo-123" {
		t.Errorf("created.ID = %q, want %q", created.ID, "slo-123")
	}
}

func TestCreateSloEncodesCustomMetricFieldsAsPointers(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "slo-1"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	name := "queue_depth"
	comparator := "lte"
	threshold := 100.0
	_, err := c.CreateSlo(context.Background(), Slo{
		GroupID:          "group-1",
		Name:             "s",
		MetricType:       "custom_metric",
		TargetPercentage: 95,
		CustomMetricName: &name,
		Comparator:       &comparator,
		GoodThreshold:    &threshold,
	})
	if err != nil {
		t.Fatalf("CreateSlo returned an error: %v", err)
	}
	if gotBody["custom_metric_name"] != "queue_depth" {
		t.Errorf("request body custom_metric_name = %v, want %q", gotBody["custom_metric_name"], "queue_depth")
	}
	if gotBody["good_threshold"] != 100.0 {
		t.Errorf("request body good_threshold = %v, want 100", gotBody["good_threshold"])
	}
}

func TestGetSloReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such slo"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetSlo(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}

func TestDeleteSloSucceedsOn204WithNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/slos/slo-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.DeleteSlo(context.Background(), "slo-1"); err != nil {
		t.Fatalf("DeleteSlo returned an error for a 204 response: %v", err)
	}
}
