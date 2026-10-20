package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateOnCallScheduleSendsBearerAuthAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotMethod, gotPath string
	var gotBody OnCallSchedule

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
		resp.ID = "schedule-123"
		resp.StartedAt = "2026-01-01T00:00:00Z"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreateOnCallSchedule(context.Background(), OnCallSchedule{
		GroupID:      "group-1",
		Name:         "Primary rotation",
		RotationDays: 7,
		Members: []OnCallMember{
			{UserID: "user-1"},
		},
	})
	if err != nil {
		t.Fatalf("CreateOnCallSchedule returned an error: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/customer/on-call-schedules" {
		t.Errorf("path = %q, want /api/v1/customer/on-call-schedules", gotPath)
	}
	if len(gotBody.Members) != 1 || gotBody.Members[0].UserID != "user-1" {
		t.Errorf("request body members not encoded correctly: %+v", gotBody.Members)
	}
	if created.ID != "schedule-123" {
		t.Errorf("created.ID = %q, want %q", created.ID, "schedule-123")
	}
	if created.StartedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("created.StartedAt = %q, want the server-assigned value", created.StartedAt)
	}
}

func TestGetOnCallScheduleReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such on-call schedule"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetOnCallSchedule(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}

func TestDeleteOnCallScheduleSucceedsOn204WithNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.DeleteOnCallSchedule(context.Background(), "schedule-1"); err != nil {
		t.Fatalf("DeleteOnCallSchedule returned an error for a 204 response: %v", err)
	}
}

func TestCreateOnCallOverrideSendsPOSTToTheScheduleOverridesPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/on-call-schedules/schedule-1/overrides" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body OnCallOverride
		_ = json.NewDecoder(r.Body).Decode(&body)
		body.ID = "override-1"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreateOnCallOverride(context.Background(), "schedule-1", OnCallOverride{
		UserID:   "user-1",
		StartsAt: "2026-01-01T00:00:00Z",
		EndsAt:   "2026-01-02T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("CreateOnCallOverride returned an error: %v", err)
	}
	if created.ID != "override-1" {
		t.Errorf("created.ID = %q, want %q", created.ID, "override-1")
	}
}

func TestGetOnCallOverrideFindsItInTheParentSchedulesOwnOverridesList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/customer/on-call-schedules/schedule-1" {
			t.Errorf("path = %q, want the schedule's own GET path", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(OnCallSchedule{
			ID: "schedule-1",
			Overrides: []OnCallOverride{
				{ID: "override-1", UserID: "user-1", StartsAt: "2026-01-01T00:00:00Z", EndsAt: "2026-01-02T00:00:00Z"},
				{ID: "override-2", UserID: "user-2", StartsAt: "2026-02-01T00:00:00Z", EndsAt: "2026-02-02T00:00:00Z"},
			},
		})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	found, err := c.GetOnCallOverride(context.Background(), "schedule-1", "override-2")
	if err != nil {
		t.Fatalf("GetOnCallOverride returned an error: %v", err)
	}
	if found.UserID != "user-2" {
		t.Errorf("found.UserID = %q, want %q", found.UserID, "user-2")
	}
}

func TestGetOnCallOverrideReturnsNotFoundWhenAbsentFromTheSchedule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(OnCallSchedule{ID: "schedule-1", Overrides: []OnCallOverride{}})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetOnCallOverride(context.Background(), "schedule-1", "gone")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for an override absent from its schedule, want true")
	}
}

func TestDeleteOnCallOverrideSucceedsOn204WithNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/on-call-schedules/schedule-1/overrides/override-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.DeleteOnCallOverride(context.Background(), "schedule-1", "override-1"); err != nil {
		t.Fatalf("DeleteOnCallOverride returned an error for a 204 response: %v", err)
	}
}
