package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccSilenceResource exercises create, read, import, and (via
// CheckDestroy) delete/cancel against a real running rustinion-web
// instance — see README.md's "Running acceptance tests". No update
// step: every attribute is RequiresReplace, since the API itself has
// no update endpoint for a silence — see resource_silence.go's own doc
// comment.
func TestAccSilenceResource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSilenceDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_silence" "test" {
  name      = "tf-acc known issue"
  group_id  = %[1]q
  starts_at = "2030-01-01T00:00:00Z"
  ends_at   = "2030-01-02T00:00:00Z"
  reason    = "tf-acc test"
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_silence.test", "id"),
					resource.TestCheckResourceAttr("rustinion_silence.test", "name", "tf-acc known issue"),
					resource.TestCheckResourceAttr("rustinion_silence.test", "group_id", groupID),
					resource.TestCheckResourceAttr("rustinion_silence.test", "starts_at", "2030-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("rustinion_silence.test", "ends_at", "2030-01-02T00:00:00Z"),
					resource.TestCheckResourceAttr("rustinion_silence.test", "reason", "tf-acc test"),
					resource.TestCheckNoResourceAttr("rustinion_silence.test", "cancelled_at"),
				),
			},
			{
				ResourceName:      "rustinion_silence.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccSilenceResource_tagScoped proves a tag-scoped silence (no
// group_id/device_id/alert_rule_id at all) is a valid, real match
// criterion on its own.
func TestAccSilenceResource_tagScoped(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSilenceDestroy,
		Steps: []resource.TestStep{
			{
				Config: `
resource "rustinion_silence" "tag" {
  name      = "tf-acc tag silence"
  tag       = "env:staging"
  starts_at = "2030-01-01T00:00:00Z"
  ends_at   = "2030-01-02T00:00:00Z"
}
`,
				Check: resource.TestCheckResourceAttr("rustinion_silence.tag", "tag", "env:staging"),
			},
		},
	})
}

func testAccCheckSilenceDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_silence" {
			continue
		}
		got, err := c.GetSilence(context.Background(), rs.Primary.ID)
		if err != nil {
			if client.IsNotFound(err) {
				continue
			}
			return err
		}
		// Destroying this resource cancels the silence rather than
		// deleting its row (see resource_silence.go's own doc
		// comment) — a real destroy must leave it cancelled, not
		// still active.
		if got.CancelledAt == nil {
			return fmt.Errorf("silence %s still active (not cancelled) after destroy", rs.Primary.ID)
		}
	}
	return nil
}
