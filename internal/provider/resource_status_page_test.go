package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccStatusPageResource exercises create, read, update, import, and
// (via CheckDestroy) delete against a real running rustinion-web
// instance — see README.md's "Running acceptance tests". Unlike every
// other TestAcc* in this package, there is only ever one status page
// per tenant (see resource_status_page.go's own doc comment on the
// singleton shape), so this test uses a single resource block across
// every step rather than a fresh one per test.
func TestAccStatusPageResource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStatusPageDestroy,
		Steps: []resource.TestStep{
			// Create + Read: no components yet.
			{
				Config: fmt.Sprintf(`
resource "rustinion_synthetic_check" "api" {
  group_id   = %[1]q
  name       = "tf-acc api health"
  check_type = "http"
  target     = "https://example.com/health"
}

resource "rustinion_status_page" "test" {
  slug  = "tf-acc-status"
  title = "tf-acc status page"
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_status_page.test", "id"),
					resource.TestCheckResourceAttr("rustinion_status_page.test", "slug", "tf-acc-status"),
					resource.TestCheckResourceAttr("rustinion_status_page.test", "title", "tf-acc status page"),
					resource.TestCheckResourceAttr("rustinion_status_page.test", "component.#", "0"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_status_page.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update in place: rename, add a component.
			{
				Config: fmt.Sprintf(`
resource "rustinion_synthetic_check" "api" {
  group_id   = %[1]q
  name       = "tf-acc api health"
  check_type = "http"
  target     = "https://example.com/health"
}

resource "rustinion_status_page" "test" {
  slug  = "tf-acc-status"
  title = "tf-acc status page renamed"

  component {
    synthetic_check_id = rustinion_synthetic_check.api.id
    display_name       = "API"
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_status_page.test", "title", "tf-acc status page renamed"),
					resource.TestCheckResourceAttr("rustinion_status_page.test", "component.#", "1"),
					resource.TestCheckResourceAttr("rustinion_status_page.test", "component.0.display_name", "API"),
					resource.TestCheckResourceAttrPair(
						"rustinion_status_page.test", "component.0.synthetic_check_id",
						"rustinion_synthetic_check.api", "id",
					),
				),
			},
		},
	})
}

func testAccCheckStatusPageDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_status_page" {
			continue
		}
		_, err := c.GetStatusPage(context.Background())
		if err == nil {
			return fmt.Errorf("status page %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
