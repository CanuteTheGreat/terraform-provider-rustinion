package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccGroupNotificationsResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupNotificationsOnServer(groupID, client.DefaultGroupNotificationsMode),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_notifications" "test" {
  group_id = %q
  mode     = "digest_only"
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_notifications.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_notifications.test", "mode", "digest_only"),
					testAccCheckGroupNotificationsOnServer(groupID, "digest_only"),
				),
			},
			{
				ResourceName:      "rustinion_group_notifications.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_notifications" "test" {
  group_id = %q
  mode     = "silent"
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_notifications.test", "mode", "silent"),
					testAccCheckGroupNotificationsOnServer(groupID, "silent"),
				),
			},
		},
	})
}

func testAccCheckGroupNotificationsOnServer(groupID string, want string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetGroupNotifications(context.Background(), groupID)
		if err != nil {
			return err
		}
		if p.Mode != want {
			return fmt.Errorf("server has mode %q, want %q", p.Mode, want)
		}
		return nil
	}
}
