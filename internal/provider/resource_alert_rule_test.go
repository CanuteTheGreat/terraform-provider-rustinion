package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccAlertRuleResource exercises create, read, update, import, and
// (via CheckDestroy) delete against a real running rustinion-web
// instance — see README.md's "Running acceptance tests" for how to
// stand one up. Skipped entirely unless TF_ACC=1 is set (terraform-
// plugin-testing's own convention, since acceptance tests are slow and
// require real infrastructure, unlike this package's plain `go test`
// unit tests).
func TestAccAlertRuleResource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRuleDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "test" {
  group_id = %[1]q
  name     = "tf-acc high cpu"
  severity = "warning"

  condition {
    metric_type = "cpu_avg_pct"
    comparator  = "gt"
    threshold   = 90
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_alert_rule.test", "id"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "group_id", groupID),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "name", "tf-acc high cpu"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "severity", "warning"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "bool_op", "and"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "breaches_to_fire", "3"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "clears_to_resolve", "2"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.#", "1"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.0.metric_type", "cpu_avg_pct"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.0.comparator", "gt"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.0.threshold", "90"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.0.condition_kind", "threshold"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.0.window_minutes", "60"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.0.aggregation", "avg"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_alert_rule.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update in place: rename, change severity, replace the
			// condition wholesale, add a second condition, switch
			// bool_op — exercises the server's own full-replace PUT
			// semantics for the nested condition list.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "test" {
  group_id = %[1]q
  name     = "tf-acc high cpu renamed"
  severity = "critical"
  bool_op  = "or"

  condition {
    metric_type = "mem_avg_pct"
    comparator  = "gt"
    threshold   = 80
  }

  condition {
    metric_type    = "cpu_avg_pct"
    condition_kind = "anomaly"
    comparator     = "gt"
    threshold      = 3
    anomaly_method = "rolling_stddev"
    baseline_window_minutes = 10080
  }

  condition {
    metric_type            = "disk_avg_pct"
    condition_kind         = "forecast"
    comparator             = "gt"
    threshold              = 90
    forecast_within_hours  = 48
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "name", "tf-acc high cpu renamed"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "severity", "critical"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "bool_op", "or"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.#", "3"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.0.metric_type", "mem_avg_pct"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.1.condition_kind", "anomaly"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.1.anomaly_method", "rolling_stddev"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.1.baseline_window_minutes", "10080"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.2.condition_kind", "forecast"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.2.metric_type", "disk_avg_pct"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.test", "condition.2.forecast_within_hours", "48"),
				),
			},
		},
	})
}

// TestAccAlertRuleResource_customMetric proves a "custom:<name>" metric
// reference round-trips correctly through the provider — the on-the-
// wire form alerting::MetricRef::parse expects directly, no separate
// "custom_metric_name" field the way the admin UI's own form has.
func TestAccAlertRuleResource_customMetric(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRuleDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "custom" {
  group_id = %[1]q
  name     = "tf-acc custom metric"

  condition {
    metric_type = "custom:queue_depth"
    comparator  = "gt"
    threshold   = 100
  }
}
`, groupID),
				Check: resource.TestCheckResourceAttr("rustinion_alert_rule.custom", "condition.0.metric_type", "custom:queue_depth"),
			},
		},
	})
}

// TestAccAlertRuleResource_aggregation proves a non-avg aggregation
// (e.g. p95) on a custom-metric condition survives a real create,
// import, and update cycle without drift — this is the regression test
// for the gap docs/roadmap.md flagged: before the "aggregation"
// attribute existed on this schema, applying a plan with a server-side
// non-avg aggregation would silently reset it back to "avg" on the
// very next apply, since the provider never sent the field at all.
// terraform-plugin-testing's own TestStep machinery already fails this
// test if the post-apply refresh+plan isn't empty, so a clean run here
// is itself the "no drift" proof the task asked for.
func TestAccAlertRuleResource_aggregation(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRuleDestroy,
		Steps: []resource.TestStep{
			// Create with a real percentile aggregation.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "agg" {
  group_id = %[1]q
  name     = "tf-acc aggregation"

  condition {
    metric_type = "custom:latency_ms"
    comparator  = "gt"
    threshold   = 100
    aggregation = "p95"
  }
}
`, groupID),
				Check: resource.TestCheckResourceAttr("rustinion_alert_rule.agg", "condition.0.aggregation", "p95"),
			},
			// Import must read back the same non-avg aggregation, not
			// silently coerce it to the schema default.
			{
				ResourceName:      "rustinion_alert_rule.agg",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update to a different non-avg aggregation — proves this
			// isn't just "p95 happens to equal whatever was already
			// there," and that a real change round-trips too.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "agg" {
  group_id = %[1]q
  name     = "tf-acc aggregation"

  condition {
    metric_type = "custom:latency_ms"
    comparator  = "gt"
    threshold   = 100
    aggregation = "p50"
  }
}
`, groupID),
				Check: resource.TestCheckResourceAttr("rustinion_alert_rule.agg", "condition.0.aggregation", "p50"),
			},
			// A second apply of the exact same config must be a no-op —
			// the concrete "does this drift on refresh" check.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "agg" {
  group_id = %[1]q
  name     = "tf-acc aggregation"

  condition {
    metric_type = "custom:latency_ms"
    comparator  = "gt"
    threshold   = 100
    aggregation = "p50"
  }
}
`, groupID),
				Check:    resource.TestCheckResourceAttr("rustinion_alert_rule.agg", "condition.0.aggregation", "p50"),
				PlanOnly: true,
			},
		},
	})
}

// TestAccAlertRuleResource_tagFilter proves a condition's tag_key/
// tag_value survive a real create, import, and update cycle without
// drift — the same "no drift" proof TestAccAlertRuleResource_aggregation
// above already established for the aggregation attribute, now for the
// tag filter that landed alongside "group by tag"/cardinality limiting
// on the backend (see docs/roadmap.md's "Tag group-by, cardinality
// limiting, and alert-condition/dashboard tag filters" entry).
func TestAccAlertRuleResource_tagFilter(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRuleDestroy,
		Steps: []resource.TestStep{
			// Create with a real tag filter.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "tagfilter" {
  group_id = %[1]q
  name     = "tf-acc tag filter"

  condition {
    metric_type = "custom:latency_ms"
    comparator  = "gt"
    threshold   = 100
    tag_key     = "region"
    tag_value   = "us-east-1"
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_alert_rule.tagfilter", "condition.0.tag_key", "region"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.tagfilter", "condition.0.tag_value", "us-east-1"),
				),
			},
			// Import must read back the same tag filter, not silently
			// drop it.
			{
				ResourceName:      "rustinion_alert_rule.tagfilter",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update to a different tag key/value — proves a real
			// change round-trips too, not just idempotence.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "tagfilter" {
  group_id = %[1]q
  name     = "tf-acc tag filter"

  condition {
    metric_type = "custom:latency_ms"
    comparator  = "gt"
    threshold   = 100
    tag_key     = "environment"
    tag_value   = "staging"
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_alert_rule.tagfilter", "condition.0.tag_key", "environment"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.tagfilter", "condition.0.tag_value", "staging"),
				),
			},
			// A second apply of the exact same config must be a no-op.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "tagfilter" {
  group_id = %[1]q
  name     = "tf-acc tag filter"

  condition {
    metric_type = "custom:latency_ms"
    comparator  = "gt"
    threshold   = 100
    tag_key     = "environment"
    tag_value   = "staging"
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_alert_rule.tagfilter", "condition.0.tag_key", "environment"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.tagfilter", "condition.0.tag_value", "staging"),
				),
				PlanOnly: true,
			},
		},
	})
}

// TestAccAlertRuleResource_noTagFilter is a plain regression check: a
// condition with neither tag_key nor tag_value set (the only shape that
// existed before this attribute pair did) still applies cleanly with no
// drift — adding an optional attribute pair must never disturb the
// existing untagged path.
func TestAccAlertRuleResource_noTagFilter(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRuleDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "notagfilter" {
  group_id = %[1]q
  name     = "tf-acc no tag filter"

  condition {
    metric_type = "custom:latency_ms"
    comparator  = "gt"
    threshold   = 100
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("rustinion_alert_rule.notagfilter", "condition.0.tag_key"),
					resource.TestCheckNoResourceAttr("rustinion_alert_rule.notagfilter", "condition.0.tag_value"),
				),
			},
			{
				ResourceName:      "rustinion_alert_rule.notagfilter",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Re-apply of the identical untagged config must be a no-op.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "notagfilter" {
  group_id = %[1]q
  name     = "tf-acc no tag filter"

  condition {
    metric_type = "custom:latency_ms"
    comparator  = "gt"
    threshold   = 100
  }
}
`, groupID),
				PlanOnly: true,
			},
		},
	})
}

// TestAccAlertRuleResource_composite exercises the fourth
// condition_kind, "composite" — a rule's own condition reading another
// real rustinion_alert_rule resource's firing state, proving real
// cross-resource dependency ordering through normal Terraform attribute
// interpolation (composite_rule_id = rustinion_alert_rule.dependency.id),
// not a hardcoded id.
func TestAccAlertRuleResource_composite(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRuleDestroy,
		Steps: []resource.TestStep{
			// Create + Read: a "dependency" rule plus a "composite" rule
			// whose own condition references the dependency rule's real,
			// not-yet-known-at-plan-time id.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "dependency" {
  group_id = %[1]q
  name     = "tf-acc composite dependency"

  condition {
    metric_type = "cpu_avg_pct"
    comparator  = "gt"
    threshold   = 90
  }
}

resource "rustinion_alert_rule" "composite" {
  group_id = %[1]q
  name     = "tf-acc composite rule"

  condition {
    metric_type             = "composite:placeholder"
    condition_kind          = "composite"
    comparator              = "eq"
    threshold               = 0
    composite_rule_id       = rustinion_alert_rule.dependency.id
    composite_expect_firing = false
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_alert_rule.composite", "id"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.composite", "condition.#", "1"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.composite", "condition.0.condition_kind", "composite"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.composite", "condition.0.composite_expect_firing", "false"),
					resource.TestCheckResourceAttrPair(
						"rustinion_alert_rule.composite", "condition.0.composite_rule_id",
						"rustinion_alert_rule.dependency", "id",
					),
				),
			},
			// Import. condition.0.metric_type is real-value-ignored: a
			// fresh import has no prior config/state to preserve the
			// submitted placeholder from (preserveCompositeMetricType's
			// own `previous` list is empty on import), so Read
			// legitimately returns the server's real `composite:<uuid>`
			// value instead — confirmed by an actual failing
			// ImportStateVerify run before this was added. Every other
			// attribute, including the condition's real
			// composite_rule_id/composite_expect_firing, still verifies
			// normally.
			{
				ResourceName:            "rustinion_alert_rule.composite",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"condition.0.metric_type"},
			},
			// Update: flip composite_expect_firing to true.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "dependency" {
  group_id = %[1]q
  name     = "tf-acc composite dependency"

  condition {
    metric_type = "cpu_avg_pct"
    comparator  = "gt"
    threshold   = 90
  }
}

resource "rustinion_alert_rule" "composite" {
  group_id = %[1]q
  name     = "tf-acc composite rule"

  condition {
    metric_type             = "composite:placeholder"
    condition_kind          = "composite"
    comparator              = "eq"
    threshold               = 0
    composite_rule_id       = rustinion_alert_rule.dependency.id
    composite_expect_firing = true
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_alert_rule.composite", "condition.0.composite_expect_firing", "true"),
				),
			},
		},
	})
}

// TestAccAlertRuleResource_compositeUnknownRuleRejected proves a real
// server-side composite-condition rejection surfaces as a real
// Terraform apply-time error. A literal self-reference
// (composite_rule_id = rustinion_alert_rule.selfref.id from within that
// same resource's own config block) is not constructible as an
// acceptance-test config at all — Terraform's own dependency-graph
// cycle detection rejects it at graph-build time, before any request
// ever reaches the server, so it can't exercise the server's own
// "cannot reference its own rule" check. A syntactically valid but
// non-existent composite_rule_id hits the exact same real server code
// path as a cross-tenant reference instead (resolve_condition's tenant
// lookup returns no row either way, so `referenced_tenant_id !=
// Some(tenant_id)` is true in both cases) — the cross-tenant case
// itself is already fully proven at the Rust level
// (create_alert_rule_rejects_a_cross_tenant_composite_rule_id); this
// only needs to prove Terraform parity correctly surfaces that same
// real rejection as a Terraform error, which this does.
func TestAccAlertRuleResource_compositeUnknownRuleRejected(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "unknownref" {
  group_id = %[1]q
  name     = "tf-acc composite unknown rule"

  condition {
    metric_type       = "composite:placeholder"
    condition_kind    = "composite"
    comparator        = "eq"
    threshold         = 0
    composite_rule_id = "00000000-0000-0000-0000-000000000000"
  }
}
`, groupID),
				// `\s+` rather than a literal space between words:
				// Terraform's own diagnostic renderer word-wraps long
				// error text at a fixed column width, which can turn the
				// space between "this" and "tenant" into a newline —
				// confirmed by an earlier real run of this exact test.
				ExpectError: regexp.MustCompile(`(?i)does\s+not\s+belong\s+to\s+this\s+tenant`),
			},
		},
	})
}

// TestAccAlertRuleResource_eventBasedMetricTypes closes the "No
// Terraform provider parity yet" gap docs/roadmap.md stated for
// KubernetesEventCount, NewErrorGroupCount, and ErrorGroupOccurrenceRate.
// Investigation found this was never a schema/code gap — metric_type is
// already a plain, unconstrained string with no validator, exactly like
// the pre-existing rum:/slo_burn_rate: conditions, so all three wire
// formats already round-tripped correctly; this test is what was
// actually missing, real end-to-end acceptance coverage proving it.
func TestAccAlertRuleResource_eventBasedMetricTypes(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRuleDestroy,
		Steps: []resource.TestStep{
			// Create + Read — one condition per new wire format.
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "events" {
  group_id = %[1]q
  name     = "tf-acc event-based metrics"
  bool_op  = "or"

  condition {
    metric_type = "k8s_event_count:OOMKilled"
    comparator  = "gt"
    threshold   = 3
  }

  condition {
    metric_type = "new_error_groups:checkout-api"
    comparator  = "gt"
    threshold   = 0
  }

  condition {
    metric_type = "error_rate:checkout-api:POST /charge"
    comparator  = "gt"
    threshold   = 10
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_alert_rule.events", "condition.#", "3"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.events", "condition.0.metric_type", "k8s_event_count:OOMKilled"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.events", "condition.1.metric_type", "new_error_groups:checkout-api"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.events", "condition.2.metric_type", "error_rate:checkout-api:POST /charge"),
				),
			},
			// Import — every field, including all three metric_type
			// strings, must survive a real refresh-from-server round
			// trip with no drift.
			{
				ResourceName:      "rustinion_alert_rule.events",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccAlertRuleResource_multiWindowBurnRate proves a real
// slo_burn_rate_multi_window condition — the native, first-class
// multi-window/multi-burn-rate SLO alerting preset — round-trips
// through create/read/import against a live server, including its two
// new fields (mw_long_window_minutes/mw_long_threshold) alongside the
// existing window_minutes/threshold, which this condition kind reuses
// as the SHORT window's own parameters.
func TestAccAlertRuleResource_multiWindowBurnRate(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRuleDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_slo" "test" {
  group_id          = %[1]q
  name              = "tf-acc mw burn rate slo"
  metric_type       = "checkin_freshness"
  target_percentage = 99.9
}

resource "rustinion_alert_rule" "mw" {
  group_id = %[1]q
  name     = "tf-acc multi-window burn rate rule"

  condition {
    metric_type             = "slo_burn_rate:${rustinion_slo.test.id}"
    condition_kind           = "slo_burn_rate_multi_window"
    comparator                = "gt"
    threshold                 = 14.4
    window_minutes             = 60
    mw_long_window_minutes = 360
    mw_long_threshold        = 6
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_alert_rule.mw", "id"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.mw", "condition.#", "1"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.mw", "condition.0.condition_kind", "slo_burn_rate_multi_window"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.mw", "condition.0.threshold", "14.4"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.mw", "condition.0.window_minutes", "60"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.mw", "condition.0.mw_long_window_minutes", "360"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.mw", "condition.0.mw_long_threshold", "6"),
				),
			},
			{
				ResourceName:      "rustinion_alert_rule.mw",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckAlertRuleDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_alert_rule" {
			continue
		}
		_, err := c.GetAlertRule(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("alert rule %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
