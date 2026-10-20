package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccGroupOfflineAlertsResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupOfflineAlertsReset(groupID),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_offline_alerts" "test" {
  group_id               = %q
  warning_after_minutes  = 15
  critical_after_minutes = 45
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_offline_alerts.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_offline_alerts.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_group_offline_alerts.test", "warning_after_minutes", "15"),
					resource.TestCheckResourceAttr("rustinion_group_offline_alerts.test", "critical_after_minutes", "45"),
					testAccCheckGroupOfflineAlertsOnServer(groupID, client.GroupOfflineAlerts{Enabled: true, WarningAfterMinutes: 15, CriticalAfterMinutes: 45}),
				),
			},
			{
				ResourceName:      "rustinion_group_offline_alerts.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_offline_alerts" "test" {
  group_id = %q
  enabled  = false
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_offline_alerts.test", "enabled", "false"),
					resource.TestCheckResourceAttr("rustinion_group_offline_alerts.test", "warning_after_minutes", "10"),
					resource.TestCheckResourceAttr("rustinion_group_offline_alerts.test", "critical_after_minutes", "30"),
					testAccCheckGroupOfflineAlertsOnServer(groupID, client.GroupOfflineAlerts{Enabled: false, WarningAfterMinutes: 10, CriticalAfterMinutes: 30}),
				),
			},
		},
	})
}

func testAccCheckGroupOfflineAlertsOnServer(groupID string, want client.GroupOfflineAlerts) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetGroupOfflineAlerts(context.Background(), groupID)
		if err != nil {
			return err
		}
		if p.Enabled != want.Enabled || p.WarningAfterMinutes != want.WarningAfterMinutes || p.CriticalAfterMinutes != want.CriticalAfterMinutes {
			return fmt.Errorf("server has %+v, want %+v", *p, want)
		}
		return nil
	}
}

func testAccCheckGroupOfflineAlertsReset(groupID string) resource.TestCheckFunc {
	return testAccCheckGroupOfflineAlertsOnServer(groupID, client.DefaultGroupOfflineAlerts)
}

func TestAccGroupHostHealthResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupHostHealthOnServer(groupID, client.DefaultGroupHostHealth()),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_host_health" "test" {
  group_id                 = %q
  swap_alert_threshold_pct = 0
  ignored_failed_services  = ["apt-daily.service", "user@*"]
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_host_health.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_host_health.test", "swap_alert_enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_group_host_health.test", "swap_alert_threshold_pct", "0"),
					resource.TestCheckResourceAttr("rustinion_group_host_health.test", "ignored_failed_services.#", "2"),
					resource.TestCheckResourceAttr("rustinion_group_host_health.test", "ignored_failed_services.1", "user@*"),
					testAccCheckGroupHostHealthOnServer(groupID, client.GroupHostHealth{
						SwapAlertEnabled: true, SwapAlertThresholdPct: 0,
						IgnoredFailedServices: []string{"apt-daily.service", "user@*"},
					}),
				),
			},
			{
				ResourceName:      "rustinion_group_host_health.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_host_health" "test" {
  group_id           = %q
  swap_alert_enabled = false
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_host_health.test", "swap_alert_enabled", "false"),
					resource.TestCheckResourceAttr("rustinion_group_host_health.test", "swap_alert_threshold_pct", "80"),
					resource.TestCheckResourceAttr("rustinion_group_host_health.test", "ignored_failed_services.#", "0"),
					testAccCheckGroupHostHealthOnServer(groupID, client.GroupHostHealth{
						SwapAlertEnabled: false, SwapAlertThresholdPct: 80, IgnoredFailedServices: []string{},
					}),
				),
			},
		},
	})
}

func testAccCheckGroupHostHealthOnServer(groupID string, want client.GroupHostHealth) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetGroupHostHealth(context.Background(), groupID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(*p, want) {
			return fmt.Errorf("server has %+v, want %+v", *p, want)
		}
		return nil
	}
}

// Omitted attributes mean the defaults, never zero values: a PUT replaces
// every field, so an omitted threshold must not turn into "alert at 0".
func TestGroupOfflineAlertsToClientFillsDefaults(t *testing.T) {
	p := groupOfflineAlertsToClient(groupOfflineAlertsResourceModel{
		GroupID:              types.StringValue("g1"),
		Enabled:              types.BoolNull(),
		WarningAfterMinutes:  types.Int64Value(20),
		CriticalAfterMinutes: types.Int64Unknown(),
	})
	want := client.GroupOfflineAlerts{Enabled: true, WarningAfterMinutes: 20, CriticalAfterMinutes: 30}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
}

func TestGroupHostHealthToClientFillsDefaults(t *testing.T) {
	ctx := context.Background()
	p, diags := groupHostHealthToClient(ctx, groupHostHealthResourceModel{
		GroupID:               types.StringValue("g1"),
		SwapAlertEnabled:      types.BoolUnknown(),
		SwapAlertThresholdPct: types.Int64Null(),
		IgnoredFailedServices: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("cups.service")}),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	want := client.GroupHostHealth{SwapAlertEnabled: true, SwapAlertThresholdPct: 80, IgnoredFailedServices: []string{"cups.service"}}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("got %+v, want %+v", p, want)
	}
}

func TestGroupOfflineAlertsAndHostHealthClientRoundTrip(t *testing.T) {
	var calls []string
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		var body map[string]any
		if r.Method == http.MethodPut {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			bodies = append(bodies, body)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/customer/groups/g1/offline-alerts":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"group_id": "g1", "enabled": true, "warning_after_minutes": 5, "critical_after_minutes": 60, "alerts_enabled": true,
			})
		case "/api/v1/customer/groups/g1/host-health":
			// A server that never had the list set may send null.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"group_id": "g1", "swap_alert_enabled": false, "swap_alert_threshold_pct": 50, "ignored_failed_services": nil,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c := client.New(server.URL, "k")
	ctx := context.Background()
	off, err := c.PutGroupOfflineAlerts(ctx, "g1", client.GroupOfflineAlerts{Enabled: true, WarningAfterMinutes: 5, CriticalAfterMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bodies[0]["alerts_enabled"]; ok {
		t.Fatalf("read-only alerts_enabled must not be sent: %v", bodies[0])
	}
	if bodies[0]["critical_after_minutes"] != float64(60) {
		t.Fatalf("body %v", bodies[0])
	}
	m := groupOfflineAlertsFromClient("g1", off)
	if m.WarningAfterMinutes.ValueInt64() != 5 || m.CriticalAfterMinutes.ValueInt64() != 60 {
		t.Fatalf("model %+v", m)
	}

	hh, err := c.PutGroupHostHealth(ctx, "g1", client.GroupHostHealth{SwapAlertEnabled: false, SwapAlertThresholdPct: 50})
	if err != nil {
		t.Fatal(err)
	}
	if list, ok := bodies[1]["ignored_failed_services"].([]any); !ok || len(list) != 0 {
		t.Fatalf("nil list must be sent as [] (null would keep the old list): %v", bodies[1])
	}
	if hh.IgnoredFailedServices == nil || len(hh.IgnoredFailedServices) != 0 {
		t.Fatalf("null list must read back as empty: %+v", hh)
	}
	if _, err := c.GetGroupHostHealth(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetGroupOfflineAlerts(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PUT /api/v1/customer/groups/g1/offline-alerts",
		"PUT /api/v1/customer/groups/g1/host-health",
		"GET /api/v1/customer/groups/g1/host-health",
		"GET /api/v1/customer/groups/g1/offline-alerts",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %v", calls)
	}
}
