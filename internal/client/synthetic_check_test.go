package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCreateSyntheticCheckRoundTripsLocations proves MinFailingLocations
// and Locations (device_id + optional label) survive a real Create
// request/response round trip through this client with no mangling —
// the multi-location fields this pass adds to the existing
// SyntheticCheck shape.
func TestCreateSyntheticCheckRoundTripsLocations(t *testing.T) {
	labelA := "us-east"
	locations := []SyntheticCheckLocation{
		{DeviceID: "device-a", Label: &labelA},
		{DeviceID: "device-b"},
	}

	var gotBody SyntheticCheck
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := gotBody
		resp.ID = "check-1"
		resp.Locations[0].ID = "location-a"
		resp.Locations[1].ID = "location-b"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreateSyntheticCheck(context.Background(), SyntheticCheck{
		GroupID:             "group-1",
		Name:                "multi-region",
		CheckType:           "tcp",
		Target:              "10.0.0.5:443",
		RelayMode:           "relay",
		MinFailingLocations: 2,
		Locations:           locations,
	})
	if err != nil {
		t.Fatalf("CreateSyntheticCheck returned an error: %v", err)
	}

	if gotBody.MinFailingLocations != 2 {
		t.Errorf("request body MinFailingLocations = %d, want 2", gotBody.MinFailingLocations)
	}
	if len(gotBody.Locations) != 2 {
		t.Fatalf("request body has %d locations, want 2", len(gotBody.Locations))
	}
	if gotBody.Locations[0].DeviceID != "device-a" || gotBody.Locations[0].Label == nil || *gotBody.Locations[0].Label != "us-east" {
		t.Errorf("request body location[0] = %+v, want device-a with label us-east", gotBody.Locations[0])
	}
	if gotBody.Locations[1].DeviceID != "device-b" || gotBody.Locations[1].Label != nil {
		t.Errorf("request body location[1] = %+v, want device-b with no label", gotBody.Locations[1])
	}

	if created.MinFailingLocations != 2 {
		t.Errorf("response MinFailingLocations = %d, want 2", created.MinFailingLocations)
	}
	if len(created.Locations) != 2 || created.Locations[0].ID != "location-a" || created.Locations[1].ID != "location-b" {
		t.Errorf("response Locations = %+v, want server-assigned ids location-a/location-b", created.Locations)
	}
}
