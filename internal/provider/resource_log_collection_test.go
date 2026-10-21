package provider

import "testing"

func TestSameLogSettingsIgnoresDefaultEnabledAndKeyOrder(t *testing.T) {
	prior := `{"min_severity":"error","files":[{"path":"/var/log/a.log"}]}`
	server := `{"files":[{"path":"/var/log/a.log"}],"enabled":true,"min_severity":"error"}`
	if !sameLogSettings(prior, server) {
		t.Fatal("server echo with default enabled must not show a diff")
	}
	if sameLogSettings(prior, `{"enabled":false,"min_severity":"error","files":[{"path":"/var/log/a.log"}]}`) {
		t.Fatal("collection turned off on the server must show a diff")
	}
	if sameLogSettings(`{"enabled":true}`, `{"enabled":false}`) {
		t.Fatal("explicit enabled must be compared")
	}
}
