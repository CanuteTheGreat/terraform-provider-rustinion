package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccGroupVMSnapshotPolicyResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupVMSnapshotPolicyReset(groupID),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_vm_snapshot_policy" "test" {
  group_id = %q
  enabled  = false
  keep     = 4
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_vm_snapshot_policy.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_vm_snapshot_policy.test", "enabled", "false"),
					resource.TestCheckResourceAttr("rustinion_group_vm_snapshot_policy.test", "keep", "4"),
				),
			},
			{
				ResourceName:      "rustinion_group_vm_snapshot_policy.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_vm_snapshot_policy" "test" {
  group_id = %q
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_vm_snapshot_policy.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_group_vm_snapshot_policy.test", "keep", "2"),
				),
			},
		},
	})
}

func testAccCheckGroupVMSnapshotPolicyReset(groupID string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetGroupVMSnapshotPolicy(context.Background(), groupID)
		if err != nil {
			return err
		}
		if *p != client.DefaultGroupVMSnapshotPolicy {
			return fmt.Errorf("group VM snapshot policy not reset on destroy: %+v", *p)
		}
		return nil
	}
}

// Omitted attributes mean the defaults (on, keep 2), never zero values: a
// PUT is a full replace, and keep 0 would be refused.
func TestGroupVMSnapshotPolicyToClientFillsDefaults(t *testing.T) {
	p := groupVMSnapshotPolicyToClient(groupVMSnapshotPolicyResourceModel{
		GroupID: types.StringValue("g1"),
		Enabled: types.BoolUnknown(),
		Keep:    types.Int64Null(),
	})
	if p != client.DefaultGroupVMSnapshotPolicy {
		t.Fatalf("got %+v, want the defaults", p)
	}
	p = groupVMSnapshotPolicyToClient(groupVMSnapshotPolicyResourceModel{
		Enabled: types.BoolValue(false),
		Keep:    types.Int64Value(5),
	})
	if p != (client.GroupVMSnapshotPolicy{Enabled: false, Keep: 5}) {
		t.Fatalf("got %+v", p)
	}
}

func TestGroupVMSnapshotPolicyClientRoundTrip(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if r.Method == http.MethodPut {
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"group_id": "g1", "enabled": false, "keep": 3})
	}))
	defer server.Close()

	c := client.New(server.URL, "k")
	saved, err := c.PutGroupVMSnapshotPolicy(context.Background(), "g1", client.GroupVMSnapshotPolicy{Enabled: false, Keep: 3})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/customer/groups/g1/vm-snapshot-policy" {
		t.Fatalf("got %s %s", gotMethod, gotPath)
	}
	if gotBody["enabled"] != false || gotBody["keep"] != float64(3) {
		t.Fatalf("body %v", gotBody)
	}
	m := groupVMSnapshotPolicyFromClient("g1", saved)
	if m.ID.ValueString() != "g1" || m.Keep.ValueInt64() != 3 || m.Enabled.ValueBool() {
		t.Fatalf("model %+v", m)
	}
	if _, err := c.GetGroupVMSnapshotPolicy(context.Background(), "g1"); err != nil || gotMethod != http.MethodGet {
		t.Fatalf("get: %v %s", err, gotMethod)
	}
}
