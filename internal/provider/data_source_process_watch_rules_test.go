package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccProcessWatchRulesDataSource creates two rules via
// rustinion_process_watch_rule in the same group, then reads them back
// via rustinion_process_watch_rules (this provider's first-ever data
// source) and confirms both appear, proving the client-side group_id
// filter (see data_source_process_watch_rules.go's own doc comment)
// works against a real server, not just a mocked list.
func TestAccProcessWatchRulesDataSource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_process_watch_rule" "a" {
  group_id     = %[1]q
  process_name = "nginx"
}

resource "rustinion_process_watch_rule" "b" {
  group_id     = %[1]q
  process_name = "redis-server"
}

data "rustinion_process_watch_rules" "all" {
  group_id = %[1]q
  depends_on = [
    rustinion_process_watch_rule.a,
    rustinion_process_watch_rule.b,
  ]
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.rustinion_process_watch_rules.all", "rules.#", "2"),
				),
			},
		},
	})
}
