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

func TestAccGroupPortablePolicyResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupPortablePolicyReset(groupID),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_portable_policy" "test" {
  group_id            = %q
  portable_devices    = true
  portable_grace_days = 21
  auto_mark_portable  = "auto"
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_portable_policy.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_portable_policy.test", "auto_mark_portable", "auto"),
					resource.TestCheckResourceAttr("rustinion_group_portable_policy.test", "portable_devices", "true"),
					resource.TestCheckResourceAttr("rustinion_group_portable_policy.test", "portable_grace_days", "21"),
					resource.TestCheckResourceAttr("rustinion_group_portable_policy.test", "asleep_grace_hours", "48"),
				),
			},
			{
				ResourceName:      "rustinion_group_portable_policy.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_portable_policy" "test" {
  group_id           = %q
  asleep_grace_hours = 24
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_portable_policy.test", "portable_devices", "false"),
					resource.TestCheckResourceAttr("rustinion_group_portable_policy.test", "portable_grace_days", "14"),
					resource.TestCheckResourceAttr("rustinion_group_portable_policy.test", "asleep_grace_hours", "24"),
					resource.TestCheckResourceAttr("rustinion_group_portable_policy.test", "auto_mark_portable", "suggest"),
				),
			},
		},
	})
}

func testAccCheckGroupPortablePolicyReset(groupID string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetGroupPortablePolicy(context.Background(), groupID)
		if err != nil {
			return err
		}
		if *p != client.DefaultGroupPortablePolicy {
			return fmt.Errorf("group portable policy not reset on destroy: %+v", *p)
		}
		return nil
	}
}

// Omitted attributes mean the defaults, never zero values: a PUT is a full
// replace, so a 0-day grace would silently turn the feature into noise.
func TestGroupPortablePolicyToClientFillsDefaults(t *testing.T) {
	p := groupPortablePolicyToClient(groupPortablePolicyResourceModel{
		GroupID:         types.StringValue("g1"),
		PortableDevices: types.BoolValue(true),
		// grace days unknown (interpolated), asleep grace unset
		PortableGraceDays: types.Int64Unknown(),
		AsleepGraceHours:  types.Int64Null(),
	})
	want := client.GroupPortablePolicy{PortableDevices: true, PortableGraceDays: 14, AsleepGraceHours: 48, AutoMarkPortable: "suggest"}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
}

func TestGroupPortablePolicyClientRoundTrip(t *testing.T) {
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"group_id": "g1", "portable_devices": true, "portable_grace_days": 30, "asleep_grace_hours": 12,
			"auto_mark_portable": "off",
		})
	}))
	defer server.Close()

	c := client.New(server.URL, "k")
	saved, err := c.PutGroupPortablePolicy(context.Background(), "g1",
		client.GroupPortablePolicy{PortableDevices: true, PortableGraceDays: 30, AsleepGraceHours: 12, AutoMarkPortable: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/customer/groups/g1/portable" {
		t.Fatalf("got %s %s", gotMethod, gotPath)
	}
	if gotBody["portable_grace_days"] != float64(30) || gotBody["portable_devices"] != true || gotBody["auto_mark_portable"] != "off" {
		t.Fatalf("body %v", gotBody)
	}
	m := groupPortablePolicyFromClient("g1", saved)
	if m.ID.ValueString() != "g1" || m.AsleepGraceHours.ValueInt64() != 12 || !m.PortableDevices.ValueBool() || m.AutoMarkPortable.ValueString() != "off" {
		t.Fatalf("model %+v", m)
	}
	if _, err := c.GetGroupPortablePolicy(context.Background(), "g1"); err != nil || gotMethod != http.MethodGet {
		t.Fatalf("get: %v %s", err, gotMethod)
	}
}

// A server that predates auto_mark_portable answers without it: the
// model reads it as the default instead of an empty string (no diff).
func TestGroupPortablePolicyFromAnOlderServer(t *testing.T) {
	m := groupPortablePolicyFromClient("g1", &client.GroupPortablePolicy{PortableGraceDays: 14, AsleepGraceHours: 48})
	if m.AutoMarkPortable.ValueString() != "suggest" {
		t.Fatalf("got %q", m.AutoMarkPortable.ValueString())
	}
}
