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

func TestAccLogMetricDefinitionResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLogMetricDefinitionDestroy,
		Steps: []resource.TestStep{
			// Create + Read: a count-mode definition.
			{
				Config: `
resource "rustinion_log_metric_definition" "test" {
  name       = "tf_acc_oom_events"
  pattern    = "oom-killer"
  extraction = "count"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_log_metric_definition.test", "id"),
					resource.TestCheckResourceAttr("rustinion_log_metric_definition.test", "name", "tf_acc_oom_events"),
					resource.TestCheckResourceAttr("rustinion_log_metric_definition.test", "pattern", "oom-killer"),
					resource.TestCheckResourceAttr("rustinion_log_metric_definition.test", "extraction", "count"),
					resource.TestCheckResourceAttr("rustinion_log_metric_definition.test", "enabled", "true"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_log_metric_definition.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: switch to capture_group extraction, add a real
			// capturing group to the pattern (required server-side —
			// exercises that a real validation error would surface as a
			// real apply failure, not silently succeed), and disable it.
			{
				Config: `
resource "rustinion_log_metric_definition" "test" {
  name       = "tf_acc_oom_events"
  pattern    = "duration_ms=(\\d+)"
  extraction = "capture_group"
  enabled    = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_log_metric_definition.test", "pattern", "duration_ms=(\\d+)"),
					resource.TestCheckResourceAttr("rustinion_log_metric_definition.test", "extraction", "capture_group"),
					resource.TestCheckResourceAttr("rustinion_log_metric_definition.test", "enabled", "false"),
				),
			},
		},
	})
}

// A real, expected apply-time failure: `capture_group` extraction
// against a pattern with no capturing group is rejected server-side
// (the exact same rejection `custom_metrics::validate_log_metric_pattern`
// gives the customer API and the admin UI) — proves the provider surfaces
// a real server-side validation error rather than silently accepting it.
func TestAccLogMetricDefinitionResource_rejectsCaptureGroupWithNoGroup(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "rustinion_log_metric_definition" "test" {
  name       = "tf_acc_bad_capture"
  pattern    = "no group here"
  extraction = "capture_group"
}
`,
				ExpectError: regexp.MustCompile(`capturing group`),
			},
		},
	})
}

func testAccCheckLogMetricDefinitionDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_log_metric_definition" {
			continue
		}
		_, err := c.GetLogMetricDefinition(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("log metric definition %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
