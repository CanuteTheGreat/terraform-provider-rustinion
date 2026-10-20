package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccProcessWatchRuleResource exercises create, read, update
// (toggling enabled and renaming process_name — a real capability
// gain over the admin UI itself, which has no update path at all),
// import, and (via CheckDestroy) delete against a real running
// rustinion-web instance — see README.md's "Running acceptance tests".
func TestAccProcessWatchRuleResource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProcessWatchRuleDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: fmt.Sprintf(`
resource "rustinion_process_watch_rule" "test" {
  group_id     = %[1]q
  process_name = "nginx"
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_process_watch_rule.test", "id"),
					resource.TestCheckResourceAttr("rustinion_process_watch_rule.test", "group_id", groupID),
					resource.TestCheckResourceAttr("rustinion_process_watch_rule.test", "process_name", "nginx"),
					resource.TestCheckResourceAttr("rustinion_process_watch_rule.test", "enabled", "true"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_process_watch_rule.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update in place: rename and disable — proving this API can
			// do both, even though routes::process_watch's own admin-UI
			// handlers never could.
			{
				Config: fmt.Sprintf(`
resource "rustinion_process_watch_rule" "test" {
  group_id     = %[1]q
  process_name = "postgres"
  enabled      = false
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_process_watch_rule.test", "process_name", "postgres"),
					resource.TestCheckResourceAttr("rustinion_process_watch_rule.test", "enabled", "false"),
				),
			},
		},
	})
}

func testAccCheckProcessWatchRuleDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_process_watch_rule" {
			continue
		}
		_, err := c.GetProcessWatchRule(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("process watch rule %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
