package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCreateAndUpdateProcessWatchRuleRoundTrip proves a real HTTP
// create then update round trip through this client for
// ProcessWatchRule — including toggling Enabled, a real capability
// this API grants that the admin UI's own add/remove-only handlers
// never could (see ProcessWatchRule's own doc comment).
func TestCreateAndUpdateProcessWatchRuleRoundTrip(t *testing.T) {
	var gotCreate, gotUpdate ProcessWatchRule
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&gotCreate); err != nil {
				t.Fatalf("decoding create request body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			resp := gotCreate
			resp.ID = "rule-1"
			_ = json.NewEncoder(w).Encode(resp)
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&gotUpdate); err != nil {
				t.Fatalf("decoding update request body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			resp := gotUpdate
			resp.ID = "rule-1"
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	c := New(server.URL, "test-key")

	created, err := c.CreateProcessWatchRule(context.Background(), ProcessWatchRule{
		GroupID:     "group-1",
		ProcessName: "nginx",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("CreateProcessWatchRule returned an error: %v", err)
	}
	if gotCreate.ProcessName != "nginx" || !gotCreate.Enabled {
		t.Fatalf("request body = %+v, want process_name=nginx enabled=true", gotCreate)
	}
	if created.ID != "rule-1" || created.ProcessName != "nginx" || !created.Enabled {
		t.Fatalf("created = %+v, want id=rule-1 process_name=nginx enabled=true", created)
	}

	updated, err := c.UpdateProcessWatchRule(context.Background(), "rule-1", ProcessWatchRule{
		GroupID:     "group-1",
		ProcessName: "postgres",
		Enabled:     false,
	})
	if err != nil {
		t.Fatalf("UpdateProcessWatchRule returned an error: %v", err)
	}
	if gotUpdate.ProcessName != "postgres" || gotUpdate.Enabled {
		t.Fatalf("update request body = %+v, want process_name=postgres enabled=false", gotUpdate)
	}
	if updated.ProcessName != "postgres" || updated.Enabled {
		t.Fatalf("updated = %+v, want process_name=postgres enabled=false", updated)
	}
}

// TestListProcessWatchRulesDecodesEveryRule proves ListProcessWatchRules
// decodes a real multi-item JSON array response — the primitive the
// data source's own client-side group_id filter depends on.
func TestListProcessWatchRulesDecodesEveryRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]ProcessWatchRule{
			{ID: "1", GroupID: "group-a", ProcessName: "nginx", Enabled: true},
			{ID: "2", GroupID: "group-b", ProcessName: "redis-server", Enabled: false},
		})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	rules, err := c.ListProcessWatchRules(context.Background())
	if err != nil {
		t.Fatalf("ListProcessWatchRules returned an error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	if rules[0].GroupID != "group-a" || rules[1].GroupID != "group-b" {
		t.Fatalf("rules = %+v, group_id mismatch", rules)
	}
}
