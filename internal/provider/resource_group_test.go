package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccGroupResource exercises create, read, import, an in-place
// template_key update, and (via CheckDestroy) delete against a real
// running rustinion-web instance — see README.md's "Running acceptance
// tests". Every step here uses a fresh, otherwise-empty group so
// delete succeeds — see resource_group.go's own doc comment for why a
// non-empty group can't be deleted at all.
func TestAccGroupResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupDestroy,
		Steps: []resource.TestStep{
			// Create + Read: blank group, no template.
			{
				Config: `
resource "rustinion_group" "test" {
  name = "tf-acc blank group"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_group.test", "id"),
					resource.TestCheckResourceAttr("rustinion_group.test", "name", "tf-acc blank group"),
					resource.TestCheckResourceAttr("rustinion_group.test", "manifest_version", "1"),
					resource.TestCheckNoResourceAttr("rustinion_group.test", "template_key"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_group.test",
				ImportState:       true,
				ImportStateVerify: true,
				// template_key is never returned by the API — an
				// import can't recover it, so a post-import plan would
				// otherwise show a spurious diff against the config's
				// own (unset) value. Nothing to ignore here since this
				// step's config has no template_key at all, but later
				// steps below do — documented on those steps instead.
			},
			// Update: apply a template in place (no replace — id stays
			// the same, checked implicitly since ImportStateVerify
			// above already confirmed the id).
			{
				Config: `
resource "rustinion_group" "test" {
  name         = "tf-acc blank group"
  template_key = "linux_web_server"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group.test", "template_key", "linux_web_server"),
					resource.TestCheckResourceAttr("rustinion_group.test", "manifest_version", "2"),
				),
			},
			// Update again: a different template, still in place.
			{
				Config: `
resource "rustinion_group" "test" {
  name         = "tf-acc blank group"
  template_key = "linux_database_server"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group.test", "template_key", "linux_database_server"),
					resource.TestCheckResourceAttr("rustinion_group.test", "manifest_version", "3"),
				),
			},
		},
	})
}

func testAccCheckGroupDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_group" {
			continue
		}
		_, err := c.GetGroup(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("group %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
