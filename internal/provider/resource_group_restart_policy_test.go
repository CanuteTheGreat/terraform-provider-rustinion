package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccGroupRestartPolicyResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupRestartPolicyReset(groupID),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_restart_policy" "test" {
  group_id = %q
  policy_json = jsonencode({
    auto_restart   = "in_window"
    timezone       = "America/Los_Angeles"
    restart_window = [{ days = [0, 1, 2, 3, 4, 5, 6], start_minute = 60, end_minute = 300 }]
    max_concurrent = 1
    drain_hooks    = [{ kind = "stop_services", units = ["forgejo-runner*.service"] }]
    verify_hooks = [
      { kind = "services_active", units = ["forgejo-runner*.service", "docker.service"] },
      { kind = "mounted", paths = ["/nfs-home"], timeout_minutes = 2 },
    ]
    drain_timeout_minutes  = 180
    verify_timeout_minutes = 20
    settle_minutes         = 15
    install_with_restart   = ["docker*", "containerd.io"]
    ignored_services       = ["cups*", "fwupd.service"]
  })
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_restart_policy.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_restart_policy.test", "paused", ""),
					testAccCheckGroupRestartPolicyServer(groupID, func(p map[string]any) error {
						if p["drain_timeout_minutes"] != float64(180) || p["settle_minutes"] != float64(15) {
							return fmt.Errorf("timeouts not stored: %v", p)
						}
						if ig, _ := p["ignored_services"].([]any); len(ig) != 2 || ig[0] != "cups*" {
							return fmt.Errorf("ignored services not stored: %v", p["ignored_services"])
						}
						hooks, _ := p["verify_hooks"].([]any)
						if len(hooks) != 2 {
							return fmt.Errorf("verify hooks not stored: %v", p["verify_hooks"])
						}
						return nil
					}),
				),
			},
			{
				ResourceName:            "rustinion_group_restart_policy.test",
				ImportState:             true,
				ImportStateId:           groupID,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"policy_json"},
			},
			{
				// The server rejects a drain hook in the verify phase.
				Config: fmt.Sprintf(`
resource "rustinion_group_restart_policy" "test" {
  group_id    = %q
  policy_json = jsonencode({ verify_hooks = [{ kind = "stop_services", units = ["x.service"] }] })
}
`, groupID),
				ExpectError: regexp.MustCompile(`can't be a\s+verify hook`),
			},
		},
	})
}

func testAccCheckGroupRestartPolicyServer(groupID string, check func(map[string]any) error) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetGroupRestartPolicy(context.Background(), groupID)
		if err != nil {
			return err
		}
		var m map[string]any
		if err := json.Unmarshal(p.RestartPolicy, &m); err != nil {
			return err
		}
		return check(m)
	}
}

func testAccCheckGroupRestartPolicyReset(groupID string) resource.TestCheckFunc {
	return testAccCheckGroupRestartPolicyServer(groupID, func(m map[string]any) error {
		if len(m) != 1 || m["auto_restart"] != "never" {
			return fmt.Errorf("group restart policy not reset on destroy: %v", m)
		}
		return nil
	})
}

func TestSameJSONIgnoresKeyOrderAndSpacing(t *testing.T) {
	if !sameJSON(`{"a":1,"b":[{"kind":"mounted","paths":["/x"]}]}`, `{ "b": [ {"paths":["/x"], "kind":"mounted"} ], "a": 1.0 }`) {
		t.Fatal("equal documents reported different")
	}
	if sameJSON(`{"a":1}`, `{"a":2}`) || sameJSON(`{"a":1}`, `not json`) {
		t.Fatal("different documents reported equal")
	}
}

func TestGroupRestartPolicyClientRoundTrip(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if r.Method == http.MethodPut {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"group_id":"g1","restart_policy":{"max_concurrent":1,"auto_restart":"in_window"},"paused":"rw-man-03 is not healthy"}`)
	}))
	defer server.Close()

	c := client.New(server.URL, "k")
	saved, err := c.PutGroupRestartPolicy(context.Background(), "g1", json.RawMessage(`{"auto_restart":"in_window","max_concurrent":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/customer/groups/g1/restart-policy" {
		t.Fatalf("got %s %s", gotMethod, gotPath)
	}
	if !sameJSON(gotBody, `{"auto_restart":"in_window","max_concurrent":1}`) {
		t.Fatalf("body %s", gotBody)
	}
	// The configured text survives when the server's copy means the same.
	m := groupRestartPolicyFromClient("g1", saved, `{"auto_restart":"in_window","max_concurrent":1}`)
	if m.PolicyJSON.ValueString() != `{"auto_restart":"in_window","max_concurrent":1}` || m.Paused.ValueString() != "rw-man-03 is not healthy" {
		t.Fatalf("model %+v", m)
	}
	// A real difference (edited outside Terraform) shows the server's copy.
	m = groupRestartPolicyFromClient("g1", saved, `{"auto_restart":"never"}`)
	if !sameJSON(m.PolicyJSON.ValueString(), `{"auto_restart":"in_window","max_concurrent":1}`) {
		t.Fatalf("drift hidden: %s", m.PolicyJSON.ValueString())
	}
	if _, err := c.GetGroupRestartPolicy(context.Background(), "g1"); err != nil || gotMethod != http.MethodGet {
		t.Fatalf("get: %v %s", err, gotMethod)
	}
}
