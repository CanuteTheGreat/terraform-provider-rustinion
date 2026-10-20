package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccStatusPageIncidentResource exercises create, read, update,
// import, and (via CheckDestroy) delete against a real running
// rustinion-web instance — see README.md's "Running acceptance tests".
// Unlike `rustinion_status_page`, incidents are not a singleton, so
// this behaves like every other TestAcc* in this package.
//
// This test deliberately never sets `component_id`: `rustinion_status_
// page`'s own `component` block exposes no server-assigned `id`
// attribute at all (see its own model's doc comment — components are
// fully replaced on every `PUT`, so there's no stable id to expose),
// so there is no real Terraform-visible value this test could pass.
// `component_id`'s own tenant-ownership validation is covered directly
// at the Rust layer instead
// (`create_incident_rejects_a_component_from_another_status_page` in
// web/src/routes/customer_api.rs).
func TestAccStatusPageIncidentResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStatusPageIncidentDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: `
resource "rustinion_status_page" "test" {
  slug  = "tf-acc-incident-status"
  title = "tf-acc incident status page"
}

resource "rustinion_status_page_incident" "test" {
  title   = "Elevated latency"
  status  = "investigating"
  message = "We're investigating elevated latency."

  depends_on = [rustinion_status_page.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_status_page_incident.test", "id"),
					resource.TestCheckResourceAttr("rustinion_status_page_incident.test", "title", "Elevated latency"),
					resource.TestCheckResourceAttr("rustinion_status_page_incident.test", "status", "investigating"),
				),
			},
			// Import — `message` is write-only (never read back), so it
			// is excluded from the verify comparison, same convention
			// `resource_alert_rule_test.go` already established for its
			// own non-read-back field.
			{
				ResourceName:            "rustinion_status_page_incident.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"message"},
			},
			// Update in place: rename and change status.
			{
				Config: `
resource "rustinion_status_page" "test" {
  slug  = "tf-acc-incident-status"
  title = "tf-acc incident status page"
}

resource "rustinion_status_page_incident" "test" {
  title   = "Elevated latency (renamed)"
  status  = "monitoring"
  message = "We're investigating elevated latency."

  depends_on = [rustinion_status_page.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_status_page_incident.test", "title", "Elevated latency (renamed)"),
					resource.TestCheckResourceAttr("rustinion_status_page_incident.test", "status", "monitoring"),
				),
			},
		},
	})
}

// There is no declarative way to create a `source = "auto"` incident
// (only `status_pages::run_tick` ever writes one) for this provider's
// own acceptance test to import and exercise against — the rejection
// itself (the single most important correctness/security boundary this
// pass adds) is directly tested at the Rust layer instead, where a test
// can insert an `auto` row directly:
// `update_incident_rejects_an_auto_sourced_incident`,
// `delete_incident_rejects_an_auto_sourced_incident`, and
// `post_incident_update_rejects_an_auto_sourced_incident` in
// web/src/routes/customer_api.rs.

func testAccCheckStatusPageIncidentDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_status_page_incident" {
			continue
		}
		_, err := c.GetStatusPageIncident(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("status page incident %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
