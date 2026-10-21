package provider

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccGroupBuildHostResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupBuildHostOnServer(groupID, client.DefaultGroupBuildHost()),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_build_host" "test" {
  group_id           = %q
  container_patterns = ["rustinion-ci-*", "rce-ci-*"]
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_build_host.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_build_host.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_group_build_host.test", "container_patterns.#", "2"),
					resource.TestCheckResourceAttr("rustinion_group_build_host.test", "container_patterns.1", "rce-ci-*"),
					testAccCheckGroupBuildHostOnServer(groupID, client.GroupBuildHost{
						Enabled: true, ContainerPatterns: []string{"rustinion-ci-*", "rce-ci-*"},
					}),
				),
			},
			{
				ResourceName:      "rustinion_group_build_host.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_build_host" "test" {
  group_id = %q
  enabled  = false
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_build_host.test", "enabled", "false"),
					resource.TestCheckResourceAttr("rustinion_group_build_host.test", "container_patterns.#", "0"),
					testAccCheckGroupBuildHostOnServer(groupID, client.DefaultGroupBuildHost()),
				),
			},
		},
	})
}

func testAccCheckGroupBuildHostOnServer(groupID string, want client.GroupBuildHost) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetGroupBuildHost(context.Background(), groupID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(*p, want) {
			return fmt.Errorf("server has %+v, want %+v", *p, want)
		}
		return nil
	}
}
