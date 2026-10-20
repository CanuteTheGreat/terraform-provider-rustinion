package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// testAccProtoV6ProviderFactories wires this package's own provider.New
// into terraform-plugin-testing's harness — every TestAcc* test in this
// package drives this exact provider code through a real `terraform
// apply`/`plan`/`destroy`, not a mock of the provider or of the HTTP
// client underneath it. The only thing not real is the provider binary
// itself being separately installed — terraform-plugin-testing runs it
// in-process instead, exactly the way `terraform` would talk to a
// locally-built dev override.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"rustinion": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck fails fast with a clear message instead of every test
// failing individually with an opaque HTTP error, when the acceptance
// test fixtures documented in the top-level README ("Running acceptance
// tests") haven't been set up: a real rustinion-web instance
// (RUSTINION_BASE_URL) and a read_write-scoped API key for it
// (RUSTINION_API_KEY), both provisioned by
// scripts/setup_test_tenant.sh against a real running Postgres.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("RUSTINION_BASE_URL") == "" {
		t.Fatal("RUSTINION_BASE_URL must be set for acceptance tests — see README.md's \"Running acceptance tests\" section")
	}
	if os.Getenv("RUSTINION_API_KEY") == "" {
		t.Fatal("RUSTINION_API_KEY must be set for acceptance tests — see README.md's \"Running acceptance tests\" section")
	}
}

// testAccGroupID returns the group every acceptance test creates its
// resources in. This provider has no `rustinion_group` resource (groups
// aren't part of the config-as-code API it wraps — see README's own
// limitations section), so acceptance tests need one provisioned ahead
// of time; scripts/setup_test_tenant.sh does that.
//
// Every TestAcc* function calls this before it calls resource.Test —
// resource.Test only skips the test (when TF_ACC isn't set) *inside*
// its own call, so a plain `go test ./...` with no acceptance
// environment configured would otherwise fail here first, on a Fatal,
// instead of cleanly skipping like every other acceptance test in this
// package. Skipping here too, matching resource.Test's own TF_ACC
// convention, is what makes plain `go test ./...` (no TF_ACC, no real
// rustinion-web instance) pass instead of failing.
func testAccGroupID(t *testing.T) string {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1 is set — see README.md's \"Running acceptance tests\" section")
	}
	groupID := os.Getenv("RUSTINION_TEST_GROUP_ID")
	if groupID == "" {
		t.Fatal("RUSTINION_TEST_GROUP_ID must be set for acceptance tests — see README.md's \"Running acceptance tests\" section")
	}
	return groupID
}

// testAccUserID returns a real tenant member every rustinion_on_call_
// schedule/rustinion_on_call_override acceptance test can reference —
// there is no `rustinion_user` resource (no customer-API surface exists
// for creating one), so this needs to be provisioned ahead of time too;
// scripts/setup_test_tenant.sh does that alongside the group. Same
// TF_ACC-skip-before-Fatal reasoning as testAccGroupID's own doc comment.
func testAccUserID(t *testing.T) string {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1 is set — see README.md's \"Running acceptance tests\" section")
	}
	userID := os.Getenv("RUSTINION_TEST_USER_ID")
	if userID == "" {
		t.Fatal("RUSTINION_TEST_USER_ID must be set for acceptance tests — see README.md's \"Running acceptance tests\" section")
	}
	return userID
}

// testAccDeviceID returns a real enrolled device every
// rustinion_synthetic_check acceptance test's own `location` block can
// reference — same "provisioned ahead of time by scripts/
// setup_test_tenant.sh, no resource in this provider creates one"
// reasoning as testAccUserID's own doc comment (real device enrollment
// needs an agent + enrollment token, not something this provider
// manages).
func testAccDeviceID(t *testing.T) string {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1 is set — see README.md's \"Running acceptance tests\" section")
	}
	deviceID := os.Getenv("RUSTINION_TEST_DEVICE_ID")
	if deviceID == "" {
		t.Fatal("RUSTINION_TEST_DEVICE_ID must be set for acceptance tests — see README.md's \"Running acceptance tests\" section")
	}
	return deviceID
}

// testAccAPIClient builds a plain client.Client (bypassing the provider
// entirely) for a CheckDestroy function to confirm a resource is
// actually gone server-side after `terraform destroy` — the same real
// HTTP path the provider itself uses, just called directly since
// CheckDestroy runs outside of any Terraform config/plan.
func testAccAPIClient() *client.Client {
	return client.New(os.Getenv("RUSTINION_BASE_URL"), os.Getenv("RUSTINION_API_KEY"))
}
