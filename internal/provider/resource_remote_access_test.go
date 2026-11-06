package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func raModel(value string, locked bool) remoteAccessChannelModel {
	return remoteAccessChannelModel{Value: types.StringValue(value), Locked: types.BoolValue(locked)}
}

// Listed channels are set; a channel managed before and dropped from the configuration is cleared.
func TestRemoteAccessPutBodySetsListedAndClearsDropped(t *testing.T) {
	plan := map[string]remoteAccessChannelModel{"desktop": raModel("on", false), "usb": raModel("off", true)}
	prior := map[string]remoteAccessChannelModel{"desktop": raModel("off", false), "clipboard": raModel("on", false)}
	body := remoteAccessPutBody(plan, prior)
	if body["clipboard"] != nil {
		t.Fatalf("clipboard must be cleared (nil), got %+v", body["clipboard"])
	}
	if _, ok := body["clipboard"]; !ok {
		t.Fatal("clipboard key must be present so the server clears it")
	}
	if b := body["desktop"]; b == nil || b.Value != "on" || b.Locked {
		t.Fatalf("desktop %+v", b)
	}
	if b := body["usb"]; b == nil || b.Value != "off" || !b.Locked {
		t.Fatalf("usb %+v", b)
	}
	// Channels never mentioned are not in the body at all (the server leaves them alone).
	if _, ok := body["terminal"]; ok {
		t.Fatal("terminal must be untouched")
	}
}

func raView(scope string, cells map[string]client.RemoteAccessCell) *client.RemoteAccessView {
	v := &client.RemoteAccessView{Scope: scope, ScopeID: "x"}
	for _, ch := range client.RemoteAccessChannels {
		cell, ok := cells[ch]
		if !ok {
			cell = client.RemoteAccessCell{Value: "inherit"}
		}
		v.Channels = append(v.Channels, client.RemoteAccessChannel{
			Channel: ch, Cells: map[string]client.RemoteAccessCell{scope: cell},
			Effective: ch == "terminal", Why: "because " + ch,
		})
	}
	return v
}

func TestRemoteAccessModelFromViewShowsManagedAndExplicitChannelsOnly(t *testing.T) {
	v := raView("group", map[string]client.RemoteAccessCell{
		"desktop": {Value: "on", Locked: true},
		"usb":     {Value: "off"},
	})
	managed := map[string]remoteAccessChannelModel{"clipboard": raModel("inherit", false)}
	m, diags := remoteAccessModelFromView("group", "g1", v, managed)
	if diags.HasError() {
		t.Fatal(diags)
	}
	got := map[string]remoteAccessChannelModel{}
	m.Channels.ElementsAs(context.Background(), &got, false)
	want := map[string]remoteAccessChannelModel{
		"desktop":   raModel("on", true),
		"usb":       raModel("off", false),
		"clipboard": raModel("inherit", false), // managed, so kept even though it is inherit
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("channels %+v want %+v", got, want)
	}
	if m.ID.ValueString() != "group:g1" || m.ScopeID.ValueString() != "g1" {
		t.Fatalf("id %q scope_id %q", m.ID.ValueString(), m.ScopeID.ValueString())
	}
	eff := map[string]bool{}
	m.Effective.ElementsAs(context.Background(), &eff, false)
	if !eff["terminal"] || eff["desktop"] || len(eff) != len(client.RemoteAccessChannels) {
		t.Fatalf("effective %v", eff)
	}
	why := map[string]string{}
	m.Why.ElementsAs(context.Background(), &why, false)
	if why["usb"] != "because usb" {
		t.Fatalf("why %v", why)
	}
}

// The organization's "never enabled" default (locked off, from the legacy column) is not a setting:
// it must not appear in state, or every plan would show a diff against a configuration that omits it.
func TestRemoteAccessLegacyDefaultIsNotASetting(t *testing.T) {
	v := raView("tenant", map[string]client.RemoteAccessCell{
		"desktop":  {Value: "off", Locked: true, DefaultFromLegacy: true},
		"terminal": {Value: "off", Locked: true, DefaultFromLegacy: true},
	})
	m, _ := remoteAccessModelFromView("tenant", "", v, nil)
	if !m.Channels.IsNull() {
		t.Fatalf("channels must be null, got %v", m.Channels)
	}
	if m.ID.ValueString() != "tenant:" || !m.ScopeID.IsNull() {
		t.Fatalf("id %q scope_id %v", m.ID.ValueString(), m.ScopeID)
	}
	// ...but a configuration that manages it sees it as inherit, not a drifting locked off.
	managed := map[string]remoteAccessChannelModel{"desktop": raModel("inherit", false)}
	m, _ = remoteAccessModelFromView("tenant", "", v, managed)
	got := map[string]remoteAccessChannelModel{}
	m.Channels.ElementsAs(context.Background(), &got, false)
	if !reflect.DeepEqual(got, managed) {
		t.Fatalf("got %+v", got)
	}
}

func TestRemoteAccessValidationRejectsNonsense(t *testing.T) {
	r := &remoteAccessPolicyResource{}
	ok := remoteAccessPolicyResourceModel{Scope: types.StringValue("group"), ScopeID: types.StringValue("g")}
	if d := r.validate(ok, map[string]remoteAccessChannelModel{"desktop": raModel("on", true)}); d.HasError() {
		t.Fatal(d)
	}
	cases := []struct {
		name  string
		model remoteAccessPolicyResourceModel
		chans map[string]remoteAccessChannelModel
	}{
		{"group without scope_id", remoteAccessPolicyResourceModel{Scope: types.StringValue("group"), ScopeID: types.StringNull()}, nil},
		{"unknown channel", ok, map[string]remoteAccessChannelModel{"rdp": raModel("on", false)}},
		{"lock on inherit", ok, map[string]remoteAccessChannelModel{"desktop": raModel("inherit", true)}},
		{"lock on a device", remoteAccessPolicyResourceModel{Scope: types.StringValue("device"), ScopeID: types.StringValue("d")}, map[string]remoteAccessChannelModel{"desktop": raModel("off", true)}},
	}
	for _, c := range cases {
		if d := r.validate(c.model, c.chans); !d.HasError() {
			t.Errorf("%s: expected an error", c.name)
		}
	}
	// The remote terminal is its own channel, never folded into the desktop.
	found := 0
	for _, c := range client.RemoteAccessChannels {
		if c == "terminal" || c == "desktop" {
			found++
		}
	}
	if found != 2 {
		t.Fatal("terminal and desktop must both be channels")
	}
}

func TestRemoteAccessImportIDs(t *testing.T) {
	for in, want := range map[string][2]string{
		"tenant":      {"tenant", ""},
		"group:g-1":   {"group", "g-1"},
		"device:d-9":  {"device", "d-9"},
		"group:":      {"", ""},
		"device":      {"", ""},
		"nonsense:xx": {"", ""},
	} {
		s, id := parseRemoteAccessImportID(in)
		if s != want[0] || id != want[1] {
			t.Errorf("%q -> %q %q, want %v", in, s, id, want)
		}
	}
}

func TestRemoteAccessClientRoundTrip(t *testing.T) {
	var calls []string
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			bodies = append(bodies, body)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/customer/groups/g1/remote-access":
			_ = json.NewEncoder(w).Encode(raView("group", map[string]client.RemoteAccessCell{"desktop": {Value: "on"}}))
		case r.URL.Path == "/api/v1/customer/remote-access":
			_ = json.NewEncoder(w).Encode(raView("tenant", nil))
		case r.URL.Path == "/api/v1/customer/remote-access/operators" && r.Method == http.MethodPut:
			_ = json.NewEncoder(w).Encode(client.RemoteAccessOperator{ID: "o1", Scope: "group", ScopeID: "g1", UserID: "u1", Email: "a@b.c", Channel: "desktop", Effect: "allow"})
		case r.URL.Path == "/api/v1/customer/remote-access/operators":
			_ = json.NewEncoder(w).Encode(map[string]any{"operators": []client.RemoteAccessOperator{{ID: "o1", Scope: "group", ScopeID: "g1", UserID: "u1", Email: "a@b.c", Channel: "desktop", Effect: "allow"}}})
		case r.URL.Path == "/api/v1/customer/remote-access/operators/o1" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c := client.New(server.URL, "k")
	ctx := context.Background()
	v, err := c.PutRemoteAccess(ctx, "group", "g1", map[string]*client.RemoteAccessInput{
		"desktop": {Value: "on", Locked: true}, "usb": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	chans := bodies[0]["channels"].(map[string]any)
	if d := chans["desktop"].(map[string]any); d["value"] != "on" || d["locked"] != true {
		t.Fatalf("body %v", chans)
	}
	if u, ok := chans["usb"]; !ok || u != nil {
		t.Fatalf("usb must be an explicit null, got %v (present %v)", u, ok)
	}
	if cell, _ := v.Cell("desktop"); cell.Value != "on" {
		t.Fatalf("cell %+v", cell)
	}
	if _, err := c.GetRemoteAccess(ctx, "tenant", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetRemoteAccess(ctx, "planet", "x"); err == nil {
		t.Fatal("unknown scope must be refused before any request")
	}
	op, err := c.PutRemoteAccessOperator(ctx, "group", "g1", "", "a@b.c", "desktop", "allow")
	if err != nil {
		t.Fatal(err)
	}
	if bodies[1]["email"] != "a@b.c" || bodies[1]["scope_id"] != "g1" || bodies[1]["channel"] != "desktop" {
		t.Fatalf("operator body %v", bodies[1])
	}
	if _, has := bodies[1]["user_id"]; has {
		t.Fatal("user_id must be omitted when only email is given")
	}
	m := remoteAccessOperatorFromClient(op)
	if m.ID.ValueString() != "o1" || m.ScopeID.ValueString() != "g1" || m.Email.ValueString() != "a@b.c" {
		t.Fatalf("model %+v", m)
	}
	if got, err := c.GetRemoteAccessOperator(ctx, "o1"); err != nil || got.UserID != "u1" {
		t.Fatalf("get %+v %v", got, err)
	}
	if _, err := c.GetRemoteAccessOperator(ctx, "nope"); !client.IsNotFound(err) {
		t.Fatalf("missing row must be NotFound, got %v", err)
	}
	if err := c.DeleteRemoteAccessOperator(ctx, "o1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PUT /api/v1/customer/groups/g1/remote-access",
		"GET /api/v1/customer/remote-access",
		"PUT /api/v1/customer/remote-access/operators",
		"GET /api/v1/customer/remote-access/operators",
		"GET /api/v1/customer/remote-access/operators",
		"DELETE /api/v1/customer/remote-access/operators/o1",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %v", calls)
	}
}

var _ attr.Value = types.StringNull()
