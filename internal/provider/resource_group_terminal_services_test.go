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

func TestAccGroupTerminalServicesResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupTerminalServicesOnServer(groupID, client.DefaultGroupTerminalServices()),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_terminal_services" "test" {
  group_id = %q
  builtins = ["oops-agent", "ttyd"]
  custom = [{
    name = "Lab terminal"
    exe  = "*/node"
    args = "/srv/term/server.js"
  }]
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_terminal_services.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_terminal_services.test", "builtins.#", "2"),
					resource.TestCheckResourceAttr("rustinion_group_terminal_services.test", "custom.0.exe", "*/node"),
					testAccCheckGroupTerminalServicesOnServer(groupID, client.GroupTerminalServices{
						Builtins: []string{"oops-agent", "ttyd"},
						Custom:   []client.TerminalService{{Name: "Lab terminal", Exe: "*/node", Args: "/srv/term/server.js"}},
					}),
				),
			},
			{
				ResourceName:      "rustinion_group_terminal_services.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_terminal_services" "test" {
  group_id = %q
  builtins = ["oops-agent"]
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_terminal_services.test", "builtins.#", "1"),
					resource.TestCheckNoResourceAttr("rustinion_group_terminal_services.test", "custom"),
					testAccCheckGroupTerminalServicesOnServer(groupID, client.GroupTerminalServices{
						Builtins: []string{"oops-agent"},
						Custom:   []client.TerminalService{},
					}),
				),
			},
		},
	})
}

func testAccCheckGroupTerminalServicesOnServer(groupID string, want client.GroupTerminalServices) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetGroupTerminalServices(context.Background(), groupID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(*p, want) {
			return fmt.Errorf("server has %+v, want %+v", *p, want)
		}
		return nil
	}
}
