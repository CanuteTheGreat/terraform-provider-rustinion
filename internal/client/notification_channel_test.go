package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNotificationChannelTemplatesRoundTrip pins the null-vs-empty
// distinction the server actually cares about: an unset template must
// travel as JSON null (meaning "use the built-in format"), and a set
// one must survive verbatim including its {{placeholders}}.
func TestNotificationChannelTemplatesRoundTrip(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		subject := "[{{severity}}] {{reason}}"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(NotificationChannel{
			ID:              "chan-1",
			Name:            "templated",
			ChannelType:     "webhook",
			Config:          json.RawMessage(`{"url":"https://example.com/hook"}`),
			SubjectTemplate: &subject,
		})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	subject := "[{{severity}}] {{reason}}"
	created, err := c.CreateNotificationChannel(context.Background(), NotificationChannel{
		Name:            "templated",
		ChannelType:     "webhook",
		Config:          json.RawMessage(`{"url":"https://example.com/hook"}`),
		SubjectTemplate: &subject,
	})
	if err != nil {
		t.Fatalf("CreateNotificationChannel returned an error: %v", err)
	}

	if raw["subject_template"] != "[{{severity}}] {{reason}}" {
		t.Fatalf("subject_template = %#v, want the template verbatim", raw["subject_template"])
	}
	// Unset templates must be explicit nulls, not absent and not "".
	for _, field := range []string{"body_template", "payload_template"} {
		v, present := raw[field]
		if !present {
			t.Fatalf("%s absent from request body; want explicit null", field)
		}
		if v != nil {
			t.Fatalf("%s = %#v, want null", field, v)
		}
	}
	if created.SubjectTemplate == nil || *created.SubjectTemplate != subject {
		t.Fatalf("created.SubjectTemplate = %v", created.SubjectTemplate)
	}
	if created.BodyTemplate != nil {
		t.Fatalf("created.BodyTemplate = %v, want nil", created.BodyTemplate)
	}
}
