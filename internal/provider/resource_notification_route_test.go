package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccNotificationRouteResource exercises create, read, import, a
// forced replace (every attribute is RequiresReplace — a route's scope
// is immutable server-side), and (via CheckDestroy) delete against a
// real running rustinion-web instance — see README.md's "Running
// acceptance tests". Covers both the "tenant" and "tag" scopes; "group"/
// "rule" scope tenant-ownership validation is covered directly at the
// Rust layer (`create_notification_route_rejects_a_group_from_another_
// tenant` in web/src/routes/customer_api.rs).
func TestAccNotificationRouteResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationRouteDestroy,
		Steps: []resource.TestStep{
			// Create + Read — "tenant" scope.
			{
				Config: `
resource "rustinion_notification_channel" "test" {
  name         = "tf-acc route channel"
  channel_type = "webhook"
  config       = jsonencode({ url = "https://example.com/webhook" })
}

resource "rustinion_notification_route" "test" {
  channel_id = rustinion_notification_channel.test.id
  scope      = "tenant"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_notification_route.test", "id"),
					resource.TestCheckResourceAttr("rustinion_notification_route.test", "scope", "tenant"),
				),
			},
			// Import — "channel_id/route_id", same compound-id shape
			// resource_on_call_override.go's own test already uses, for
			// the same reason: fetching/deleting a route always needs
			// its parent channel's id too.
			{
				ResourceName: "rustinion_notification_route.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["rustinion_notification_route.test"]
					if !ok {
						return "", fmt.Errorf("resource not found in state")
					}
					return rs.Primary.Attributes["channel_id"] + "/" + rs.Primary.ID, nil
				},
				ImportStateVerify: true,
			},
			// Changing scope to "tag" forces a real replace (every
			// attribute is RequiresReplace) — proves this resource
			// never attempts an in-place update against an endpoint
			// that doesn't support one.
			{
				Config: `
resource "rustinion_notification_channel" "test" {
  name         = "tf-acc route channel"
  channel_type = "webhook"
  config       = jsonencode({ url = "https://example.com/webhook" })
}

resource "rustinion_notification_route" "test" {
  channel_id = rustinion_notification_channel.test.id
  scope      = "tag"
  tag        = "production"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_notification_route.test", "scope", "tag"),
					resource.TestCheckResourceAttr("rustinion_notification_route.test", "tag", "production"),
				),
			},
		},
	})
}

func testAccCheckNotificationRouteDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_notification_route" {
			continue
		}
		channelID := rs.Primary.Attributes["channel_id"]
		_, err := c.GetNotificationRoute(context.Background(), channelID, rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("notification route %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
