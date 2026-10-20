package provider

import (
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
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "id", "alert-routing"),
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "email_severities.#", "2"),
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "pagerduty_severities.#", "1"),
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "ntfy_priority_digest", "min"),
					resource.TestCheckResourceAttr("rustinion_alert_routing.test", "digest_hour", "9"),
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
