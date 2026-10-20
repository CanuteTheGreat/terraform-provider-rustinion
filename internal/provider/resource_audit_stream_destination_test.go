package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccAuditStreamDestinationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAuditStreamDestinationDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: `
resource "rustinion_audit_stream_destination" "test" {
  name        = "tf_acc_siem"
  webhook_url = "https://siem.example.com/ingest"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_audit_stream_destination.test", "id"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.test", "name", "tf_acc_siem"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.test", "webhook_url", "https://siem.example.com/ingest"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("rustinion_audit_stream_destination.test", "signing_secret"),
				),
			},
			// Zero-diff-after-apply proof: the customer API's own GET
			// response never includes signing_secret, so a naive Read
			// implementation would show a perpetual "wants to change
			// signing_secret" diff here. `PlanOnly` fails the test if
			// Terraform thinks there's anything left to apply.
			{
				Config: `
resource "rustinion_audit_stream_destination" "test" {
  name        = "tf_acc_siem"
  webhook_url = "https://siem.example.com/ingest"
}
`,
				PlanOnly: true,
			},
			// Update: name, webhook_url, and enabled.
			{
				Config: `
resource "rustinion_audit_stream_destination" "test" {
  name        = "tf_acc_siem_renamed"
  webhook_url = "https://siem.example.com/v2"
  enabled     = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.test", "name", "tf_acc_siem_renamed"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.test", "webhook_url", "https://siem.example.com/v2"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.test", "enabled", "false"),
					// The secret must survive an update untouched — it's
					// never sent to or returned by the update endpoint.
					resource.TestCheckResourceAttrSet("rustinion_audit_stream_destination.test", "signing_secret"),
				),
			},
		},
	})
}

// A real, expected apply-time failure: an invalid webhook_url is
// rejected server-side (the exact same rejection
// `audit_stream::validate_audit_stream_destination_fields` gives the
// customer API and the admin UI) — proves the provider surfaces a real
// server-side validation error rather than silently accepting it.
func TestAccAuditStreamDestinationResource_rejectsInvalidURL(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "rustinion_audit_stream_destination" "test" {
  name        = "tf_acc_bad_url"
  webhook_url = "not-a-url"
}
`,
				ExpectError: regexp.MustCompile(`valid URL`),
			},
		},
	})
}

// TestAccAuditStreamDestinationResource_splunkHec exercises the second
// real delivery type: destination_type = "splunk_hec". Unlike the
// webhook's own signing_secret, hec_token has no "shown once" property
// — it's freely returned on every real response, so no PlanOnly step is
// needed here to prove Read doesn't corrupt it (there is nothing to
// preserve specially; the field just round-trips like any other).
func TestAccAuditStreamDestinationResource_splunkHec(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAuditStreamDestinationDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: `
resource "rustinion_audit_stream_destination" "hec" {
  name             = "tf_acc_splunk"
  destination_type = "splunk_hec"
  hec_endpoint     = "https://splunk.example.com:8088/services/collector/event"
  hec_token        = "tf-acc-real-hec-token"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_audit_stream_destination.hec", "id"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.hec", "destination_type", "splunk_hec"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.hec", "hec_endpoint", "https://splunk.example.com:8088/services/collector/event"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.hec", "hec_token", "tf-acc-real-hec-token"),
					// webhook_url is Optional (not Computed) and genuinely
					// unset for a splunk_hec destination — null in real
					// Terraform state, not an empty string.
					resource.TestCheckNoResourceAttr("rustinion_audit_stream_destination.hec", "webhook_url"),
					// signing_secret is Computed (server-controlled, never
					// customer-set) — a Computed attribute always resolves
					// to some known value after apply, so for a
					// splunk_hec destination that's a real, legitimate
					// empty string, not an absent attribute.
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.hec", "signing_secret", ""),
				),
			},
			// Zero-diff-after-apply: hec_token round-trips normally
			// (no write-once workaround needed), so a plain refresh must
			// show no drift either.
			{
				Config: `
resource "rustinion_audit_stream_destination" "hec" {
  name             = "tf_acc_splunk"
  destination_type = "splunk_hec"
  hec_endpoint     = "https://splunk.example.com:8088/services/collector/event"
  hec_token        = "tf-acc-real-hec-token"
}
`,
				PlanOnly: true,
			},
			// Update: endpoint, index, sourcetype — hec_token is
			// deliberately omitted from the update body (not updatable
			// via Update) but must still read back its real, unchanged
			// value from the server's own response.
			{
				Config: `
resource "rustinion_audit_stream_destination" "hec" {
  name             = "tf_acc_splunk_renamed"
  destination_type = "splunk_hec"
  hec_endpoint     = "https://splunk.example.com:8088/services/collector/event2"
  hec_token        = "tf-acc-real-hec-token"
  hec_index        = "security"
  hec_sourcetype   = "rustinion:audit"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.hec", "name", "tf_acc_splunk_renamed"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.hec", "hec_endpoint", "https://splunk.example.com:8088/services/collector/event2"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.hec", "hec_index", "security"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.hec", "hec_sourcetype", "rustinion:audit"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.hec", "hec_token", "tf-acc-real-hec-token"),
				),
			},
		},
	})
}

// A splunk_hec destination missing its required hec_token is rejected
// server-side — the exact same rejection
// `audit_stream::validate_destination_shape` gives the customer API and
// the admin UI.
func TestAccAuditStreamDestinationResource_splunkHecRejectsMissingToken(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "rustinion_audit_stream_destination" "hec_no_token" {
  name             = "tf_acc_splunk_no_token"
  destination_type = "splunk_hec"
  hec_endpoint     = "https://splunk.example.com:8088/services/collector/event"
}
`,
				ExpectError: regexp.MustCompile(`hec_token is required`),
			},
		},
	})
}

// TestAccAuditStreamDestinationResource_syslog exercises the third real
// delivery type: destination_type = "syslog". Unlike webhook/splunk_hec,
// syslog has no secret/token attribute at all — nothing to preserve
// specially on Read, so (like splunk_hec) no PlanOnly zero-diff step is
// needed here either.
func TestAccAuditStreamDestinationResource_syslog(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAuditStreamDestinationDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: `
resource "rustinion_audit_stream_destination" "syslog" {
  name              = "tf_acc_syslog"
  destination_type  = "syslog"
  syslog_host       = "siem.example.com"
  syslog_port       = 6514
  syslog_use_tls    = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_audit_stream_destination.syslog", "id"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.syslog", "destination_type", "syslog"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.syslog", "syslog_host", "siem.example.com"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.syslog", "syslog_port", "6514"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.syslog", "syslog_use_tls", "true"),
					// webhook_url/hec_* are all genuinely unset for a
					// syslog destination — null in real Terraform state.
					resource.TestCheckNoResourceAttr("rustinion_audit_stream_destination.syslog", "webhook_url"),
					resource.TestCheckNoResourceAttr("rustinion_audit_stream_destination.syslog", "hec_endpoint"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.syslog", "signing_secret", ""),
				),
			},
			// Update: host, port, and flip use_tls off.
			{
				Config: `
resource "rustinion_audit_stream_destination" "syslog" {
  name              = "tf_acc_syslog_renamed"
  destination_type  = "syslog"
  syslog_host       = "siem2.example.com"
  syslog_port       = 514
  syslog_use_tls    = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.syslog", "name", "tf_acc_syslog_renamed"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.syslog", "syslog_host", "siem2.example.com"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.syslog", "syslog_port", "514"),
					resource.TestCheckResourceAttr("rustinion_audit_stream_destination.syslog", "syslog_use_tls", "false"),
				),
			},
		},
	})
}

// A syslog destination missing its required syslog_host is rejected
// server-side — the exact same rejection
// `audit_stream::validate_destination_shape` gives the customer API and
// the admin UI.
func TestAccAuditStreamDestinationResource_syslogRejectsMissingHost(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "rustinion_audit_stream_destination" "syslog_no_host" {
  name             = "tf_acc_syslog_no_host"
  destination_type = "syslog"
  syslog_port      = 514
}
`,
				ExpectError: regexp.MustCompile(`syslog_host is required`),
			},
		},
	})
}

func testAccCheckAuditStreamDestinationDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_audit_stream_destination" {
			continue
		}
		_, err := c.GetAuditStreamDestination(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("audit stream destination %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
