package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccAlertRoutingResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRoutingOnServer(client.DefaultAlertRouting()),
		Steps: []resource.TestStep{
			{
				Config: `
resource "rustinion_alert_routing" "test" {
  notify_all_admins      = false
  extra_recipients       = ["ops@example.test"]
  email_severities       = ["warning", "info"]
  webhook_severities     = ["critical"]
  ntfy_priority_critical = "high"
  alert_digest_cadence   = "daily"
  summary_digest_cadence = "daily"
  digest_timezone        = "America/Los_Angeles"

  summary_recipients_separate = true
  summary_recipients          = ["agents@example.test"]
  summary_notify_all_admins   = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "id", "alert-routing"),
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "email_severities.#", "2"),
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "pagerduty_severities.#", "1"),
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "ntfy_priority_digest", "min"),
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "digest_hour", "9"),
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "summary_recipients_separate", "true"),
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "summary_recipients.#", "1"),
					func(_ *terraform.State) error {
						a, err := testAccAPIClient().GetAlertRouting(context.Background())
						if err != nil {
							return err
						}
						if a.NtfyPriorities.Critical != "high" || a.DigestTimezone != "America/Los_Angeles" ||
							!reflect.DeepEqual(a.EmailSeverities, []string{"warning", "info"}) {
							return fmt.Errorf("server has %+v", *a)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "rustinion_alert_routing.test",
				ImportState:       true,
				ImportStateId:     "alert-routing",
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckAlertRoutingOnServer(want client.AlertRouting) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		a, err := testAccAPIClient().GetAlertRouting(context.Background())
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(*a, want) {
			return fmt.Errorf("server has %+v, want %+v", *a, want)
		}
		return nil
	}
}

// Omitted attributes mean the defaults (push destinations critical only),
// never empty lists: a PUT replaces every field.
func TestAlertRoutingToClientFillsDefaults(t *testing.T) {
	ctx := context.Background()
	a, diags := alertRoutingToClient(ctx, alertRoutingResourceModel{
		NotifyEmail:         types.BoolUnknown(),
		ExtraRecipients:     types.ListNull(types.StringType),
		EmailSeverities:     severitySet("info", "warning"),
		WebhookSeverities:   types.SetUnknown(types.StringType),
		PagerdutySeverities: types.SetNull(types.StringType),
		NtfyPriorityWarning: types.StringValue("default"),
		DigestHour:          types.Int64Null(),
		DigestTimezone:      types.StringValue("Europe/Oslo"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	want := client.DefaultAlertRouting()
	// Not mentioned in the configuration = not sent: the server's summary routing stays as it is.
	want.SummaryRecipientsSeparate = nil
	want.EmailSeverities = []string{"info", "warning"}
	want.NtfyPriorities.Warning = "default"
	want.DigestTimezone = "Europe/Oslo"
	if !reflect.DeepEqual(a, want) {
		t.Fatalf("got %+v, want %+v", a, want)
	}
}

func TestAlertRoutingClientRoundTrip(t *testing.T) {
	var put map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/customer/alert-routing" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Method == http.MethodPut {
			if err := json.NewDecoder(r.Body).Decode(&put); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"notify_email": true, "notify_all_admins": false, "extra_recipients": []string{"a@b.test"},
			"notify_webhook": true, "webhook_format": "ntfy",
			"email_severities": []string{"warning", "info"}, "webhook_severities": []string{"critical"},
			"pagerduty_severities": []string{"critical"},
			"ntfy_priorities":      map[string]string{"critical": "urgent", "warning": "low", "info": "min", "digest": "min"},
			"alert_digest_cadence": "daily", "summary_digest_cadence": "daily",
			"digest_hour": 9, "digest_timezone": "America/Los_Angeles",
		})
	}))
	defer server.Close()
	c := client.New(server.URL, "key")
	got, err := c.PutAlertRouting(context.Background(), client.DefaultAlertRouting())
	if err != nil {
		t.Fatal(err)
	}
	if put["ntfy_priorities"].(map[string]any)["critical"] != "urgent" {
		t.Fatalf("PUT body %v", put)
	}
	if got.DigestTimezone != "America/Los_Angeles" || got.NotifyAllAdmins {
		t.Fatalf("decoded %+v", *got)
	}
	m, diags := alertRoutingFromClient(context.Background(), got)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if len(m.EmailSeverities.Elements()) != 2 || m.ID.ValueString() != "alert-routing" {
		t.Fatalf("model %+v", m)
	}
}

// The summary routing (migration 0289) is sent only when it is configured, and what the server says
// comes back into the state, including the explicit "separate, empty list" and "inherit admins" cases.
func TestAlertRoutingSummaryRecipientsSerialization(t *testing.T) {
	ctx := context.Background()
	base := alertRoutingResourceModel{
		SummaryRecipientsSeparate: types.BoolUnknown(),
		SummaryRecipients:         types.ListUnknown(types.StringType),
		SummaryNotifyAllAdmins:    types.BoolNull(),
	}
	a, diags := alertRoutingToClient(ctx, base)
	if diags.HasError() {
		t.Fatal(diags)
	}
	body, _ := json.Marshal(a)
	for _, k := range []string{"summary_recipients_separate", "summary_recipients", "summary_notify_all_admins"} {
		if bytes.Contains(body, []byte(k)) {
			t.Fatalf("%s must not be sent when unset: %s", k, body)
		}
	}

	list, _ := types.ListValueFrom(ctx, types.StringType, []string{"agents@example.test"})
	base.SummaryRecipientsSeparate = types.BoolValue(true)
	base.SummaryRecipients = list
	base.SummaryNotifyAllAdmins = types.BoolValue(false)
	a, diags = alertRoutingToClient(ctx, base)
	if diags.HasError() {
		t.Fatal(diags)
	}
	body, _ = json.Marshal(a)
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if sent["summary_recipients_separate"] != true || sent["summary_notify_all_admins"] != false ||
		!reflect.DeepEqual(sent["summary_recipients"], []any{"agents@example.test"}) {
		t.Fatalf("PUT body %s", body)
	}

	// An empty list is sent as [] (nobody by email), not dropped.
	empty, _ := types.ListValueFrom(ctx, types.StringType, []string{})
	base.SummaryRecipients = empty
	a, _ = alertRoutingToClient(ctx, base)
	body, _ = json.Marshal(a)
	if !bytes.Contains(body, []byte(`"summary_recipients":[]`)) {
		t.Fatalf("empty list must be sent: %s", body)
	}
}

func TestAlertRoutingSummaryRecipientsRoundTrip(t *testing.T) {
	var put map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&put)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"notify_email": true, "notify_all_admins": true, "extra_recipients": []string{"owner@example.test"},
			"email_severities": []string{"critical"}, "webhook_severities": []string{"critical"},
			"pagerduty_severities": []string{"critical"},
			"ntfy_priorities":      map[string]string{"critical": "urgent", "warning": "low", "info": "min", "digest": "min"},
			"alert_digest_cadence": "per_incident", "summary_digest_cadence": "daily",
			"digest_hour": 9, "digest_timezone": "UTC",
			"summary_recipients_separate": true, "summary_recipients": []string{"agents@example.test"},
			"summary_notify_all_admins": false,
		})
	}))
	defer server.Close()
	c := client.New(server.URL, "key")
	got, err := c.GetAlertRouting(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m, diags := alertRoutingFromClient(context.Background(), got)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !m.SummaryRecipientsSeparate.ValueBool() || m.SummaryNotifyAllAdmins.IsNull() || m.SummaryNotifyAllAdmins.ValueBool() ||
		len(m.SummaryRecipients.Elements()) != 1 {
		t.Fatalf("model %+v", m)
	}
	// Writing the state back sends the same values.
	in, _ := alertRoutingToClient(context.Background(), m)
	if _, err := c.PutAlertRouting(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if put["summary_recipients_separate"] != true || put["summary_notify_all_admins"] != false {
		t.Fatalf("PUT body %v", put)
	}

	// Server without the feature set: separate false, empty list, admins inherited (null).
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"notify_email": true, "summary_recipients_separate": false,
			"summary_recipients": nil, "summary_notify_all_admins": nil,
		})
	}))
	defer server2.Close()
	got, err = client.New(server2.URL, "key").GetAlertRouting(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m, _ = alertRoutingFromClient(context.Background(), got)
	if m.SummaryRecipientsSeparate.ValueBool() || !m.SummaryNotifyAllAdmins.IsNull() || len(m.SummaryRecipients.Elements()) != 0 {
		t.Fatalf("model %+v", m)
	}
}

// Destroying the resource puts the scheduled mail back on the main list (an explicit false is sent).
func TestAlertRoutingDeleteSendsSeparateFalse(t *testing.T) {
	body, _ := json.Marshal(client.DefaultAlertRouting())
	if !bytes.Contains(body, []byte(`"summary_recipients_separate":false`)) {
		t.Fatalf("defaults must carry separate=false: %s", body)
	}
}
