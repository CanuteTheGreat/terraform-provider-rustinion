package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccLogArchiveDestinationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLogArchiveDestinationDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: `
resource "rustinion_log_archive_destination" "test" {
  bucket            = "tf-acc-archives"
  region            = "us-east-1"
  access_key_id     = "AKIAEXAMPLE123456789"
  secret_access_key = "tf-acc-secret-value"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_log_archive_destination.test", "id"),
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "bucket", "tf-acc-archives"),
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "region", "us-east-1"),
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "access_key_id", "AKIAEXAMPLE123456789"),
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "secret_access_key", "tf-acc-secret-value"),
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "prefix", ""),
				),
			},
			// Zero-diff-after-apply proof: the customer API's own GET
			// response never includes secret_access_key at all, so a
			// naive Read implementation would show a perpetual "wants to
			// change secret_access_key" diff here. `PlanOnly` fails the
			// test if Terraform thinks there's anything left to apply.
			{
				Config: `
resource "rustinion_log_archive_destination" "test" {
  bucket            = "tf-acc-archives"
  region            = "us-east-1"
  access_key_id     = "AKIAEXAMPLE123456789"
  secret_access_key = "tf-acc-secret-value"
}
`,
				PlanOnly: true,
			},
			// Update: bucket, region, prefix, enabled -- none of these
			// touch access_key_id/secret_access_key, which the customer
			// API has no update path for at all.
			{
				Config: `
resource "rustinion_log_archive_destination" "test" {
  bucket            = "tf-acc-archives-renamed"
  region            = "eu-west-1"
  access_key_id     = "AKIAEXAMPLE123456789"
  secret_access_key = "tf-acc-secret-value"
  prefix            = "renamed-prefix"
  enabled           = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "bucket", "tf-acc-archives-renamed"),
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "region", "eu-west-1"),
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "prefix", "renamed-prefix"),
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "enabled", "false"),
					resource.TestCheckResourceAttr("rustinion_log_archive_destination.test", "access_key_id", "AKIAEXAMPLE123456789"),
				),
			},
		},
	})
}

func testAccCheckLogArchiveDestinationDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_log_archive_destination" {
			continue
		}
		_, err := c.GetLogArchiveDestination(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("log archive destination %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
