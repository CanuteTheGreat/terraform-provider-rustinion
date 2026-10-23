package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Needs a device of the test organization (RUSTINION_TEST_DEVICE_ID).
func TestAccAwsSecurityAccountResource(t *testing.T) {
	deviceID := os.Getenv("RUSTINION_TEST_DEVICE_ID")
	if deviceID == "" {
		t.Skip("RUSTINION_TEST_DEVICE_ID is not set")
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			for _, rs := range s.RootModule().Resources {
				if rs.Type != "rustinion_aws_security_account" {
					continue
				}
				if _, err := testAccAPIClient().GetAwsSecurityAccount(context.Background(), rs.Primary.ID); err == nil {
					return fmt.Errorf("account %s still exists", rs.Primary.ID)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_aws_security_account" "test" {
  name       = "Terraform test"
  account_id = "123456789012"
  device_id  = %q
  guardduty  = false
}
`, deviceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_aws_security_account.test", "role_name", "RustinionSecurityRead"),
					resource.TestCheckResourceAttrSet("rustinion_aws_security_account.test", "external_id"),
					resource.TestCheckResourceAttr("rustinion_aws_security_account.test", "role_arn", "arn:aws:iam::123456789012:role/RustinionSecurityRead"),
				),
			},
			{
				ResourceName:      "rustinion_aws_security_account.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
