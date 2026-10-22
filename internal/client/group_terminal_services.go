package client

import (
	"context"
	"fmt"
	"net/http"
)

// TerminalService is one expected terminal service: globs over the
// program path and its arguments.
type TerminalService struct {
	Name string `json:"name"`
	Exe  string `json:"exe"`
	Args string `json:"args"`
}

// GroupTerminalServices mirrors `GET/PUT
// /api/v1/customer/groups/{id}/terminal-services`: shells started under a
// matching process are info and never alert. Builtins are the built-in
// suggestion ids (`oops-agent`, `ttyd`, ...), Custom the group's own.
type GroupTerminalServices struct {
	Builtins []string          `json:"builtins"`
	Custom   []TerminalService `json:"custom"`
}

// DefaultGroupTerminalServices: nothing expected.
func DefaultGroupTerminalServices() GroupTerminalServices {
	return GroupTerminalServices{Builtins: []string{}, Custom: []TerminalService{}}
}

func groupTerminalServicesPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/terminal-services", groupID)
}

func (p *GroupTerminalServices) normalise() {
	if p.Builtins == nil {
		p.Builtins = []string{}
	}
	if p.Custom == nil {
		p.Custom = []TerminalService{}
	}
}

func (c *Client) GetGroupTerminalServices(ctx context.Context, groupID string) (*GroupTerminalServices, error) {
	var out GroupTerminalServices
	if err := c.do(ctx, http.MethodGet, groupTerminalServicesPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	out.normalise()
	return &out, nil
}

func (c *Client) PutGroupTerminalServices(ctx context.Context, groupID string, p GroupTerminalServices) (*GroupTerminalServices, error) {
	p.normalise()
	var out GroupTerminalServices
	if err := c.do(ctx, http.MethodPut, groupTerminalServicesPath(groupID), p, &out); err != nil {
		return nil, err
	}
	out.normalise()
	return &out, nil
}
