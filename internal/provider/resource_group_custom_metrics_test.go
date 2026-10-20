package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"git.canutethegreat.com/CanuteTheGreat/rustinion/terraform-provider-rustinion/internal/client"
)

func TestAccGroupCustomMetricsResource(t *testing.T) {
	groupID := testAccGroupID(t)
	r := "rustinion_group_custom_metrics.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupCustomMetricsEnabled(groupID, false, 0),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_custom_metrics" "test" {
  group_id            = %q
  enabled             = true
  statsd_address      = "0.0.0.0:8125"
  flush_interval_secs = 30
  scrape_targets = [
    {
      url       = "http://localhost:9100/metrics"
      metrics   = ["node_load*", "node_memory_MemAvailable_bytes"]
      namespace = "host"
      tags      = { job = "node" }
    },
  ]
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(r, "id", groupID),
					resource.TestCheckResourceAttr(r, "enabled", "true"),
					resource.TestCheckResourceAttr(r, "statsd_enabled", "true"),
					resource.TestCheckResourceAttr(r, "statsd_address", "0.0.0.0:8125"),
					resource.TestCheckResourceAttr(r, "flush_interval_secs", "30"),
					resource.TestCheckResourceAttr(r, "scrape_targets.#", "1"),
					resource.TestCheckResourceAttr(r, "scrape_targets.0.metrics.#", "2"),
					resource.TestCheckResourceAttr(r, "scrape_targets.0.interval_secs", "60"),
					resource.TestCheckResourceAttr(r, "scrape_targets.0.max_series", "200"),
					resource.TestCheckResourceAttr(r, "scrape_targets.0.tags.job", "node"),
					testAccCheckGroupCustomMetricsEnabled(groupID, true, 1),
				),
			},
			{
				ResourceName:      r,
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_custom_metrics" "test" {
  group_id = %q
  enabled  = false
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(r, "enabled", "false"),
					resource.TestCheckResourceAttr(r, "statsd_address", "127.0.0.1:8125"),
					resource.TestCheckResourceAttr(r, "scrape_targets.#", "0"),
					testAccCheckGroupCustomMetricsEnabled(groupID, false, 0),
				),
			},
		},
	})
}

func testAccCheckGroupCustomMetricsEnabled(groupID string, enabled bool, targets int) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetGroupCustomMetrics(context.Background(), groupID)
		if err != nil {
			return err
		}
		if p.Enabled != enabled || len(p.ScrapeTargets) != targets {
			return fmt.Errorf("server has %+v, want enabled=%v with %d target(s)", *p, enabled, targets)
		}
		return nil
	}
}

func TestGroupCustomMetricsClientRoundTrip(t *testing.T) {
	var calls []string
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			bodies = append(bodies, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"group_id": "g1", "enabled": true, "statsd_enabled": true, "statsd_address": "127.0.0.1:8125",
			"statsd_socket": nil, "flush_interval_secs": 60, "monitoring_enabled": true,
			"scrape_targets": []any{map[string]any{
				"url": "http://localhost:9100/metrics", "metrics": []string{"node_load1"}, "namespace": nil,
				"interval_secs": 60, "max_series": 200, "tags": map[string]string{},
			}},
		})
	}))
	defer server.Close()

	c := client.New(server.URL, "k")
	ctx := context.Background()
	in := client.DefaultGroupCustomMetrics()
	in.Enabled = true
	in.MonitoringEnabled = true
	saved, err := c.PutGroupCustomMetrics(ctx, "g1", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bodies[0]["monitoring_enabled"]; ok {
		t.Fatalf("read-only monitoring_enabled must not be sent: %v", bodies[0])
	}
	if list, ok := bodies[0]["scrape_targets"].([]any); !ok || len(list) != 0 {
		t.Fatalf("no targets must be sent as [] (absent would keep the old list): %v", bodies[0])
	}
	m, diags := groupCustomMetricsFromClient(ctx, "g1", saved)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !m.MonitoringEnabled.ValueBool() || len(m.ScrapeTargets) != 1 {
		t.Fatalf("model %+v", m)
	}
	if !m.ScrapeTargets[0].Tags.IsNull() || !m.ScrapeTargets[0].Namespace.IsNull() || !m.StatsdSocket.IsNull() {
		t.Fatalf("empty server values must be null in state: %+v", m.ScrapeTargets[0])
	}
	back, diags := groupCustomMetricsToClient(ctx, m)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if back.ScrapeTargets[0].URL != "http://localhost:9100/metrics" || back.ScrapeTargets[0].Metrics[0] != "node_load1" {
		t.Fatalf("round trip %+v", back)
	}
	if _, err := c.GetGroupCustomMetrics(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PUT /api/v1/customer/groups/g1/custom-metrics",
		"GET /api/v1/customer/groups/g1/custom-metrics",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %v", calls)
	}
}
