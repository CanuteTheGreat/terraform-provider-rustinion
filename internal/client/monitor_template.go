package client

import (
	"context"
	"net/http"
	"net/url"
)

// MonitorTemplateMacro mirrors `monitor_templates::MacroRow`.
type MonitorTemplateMacro struct {
	Macro       string  `json:"macro"`
	Context     *string `json:"context,omitempty"`
	Value       string  `json:"value"`
	Description string  `json:"description"`
}

// MonitorTemplateRule mirrors `monitor_templates::RuleRow`. Sending a
// rule back with its ID keeps its per-device state across an update.
type MonitorTemplateRule struct {
	ID              *string `json:"id,omitempty"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Severity        string  `json:"severity"`
	ItemKey         string  `json:"item_key"`
	Func            string  `json:"func"`
	WindowMinutes   int64   `json:"window_minutes"`
	Comparator      string  `json:"comparator"`
	Threshold       string  `json:"threshold"`
	BreachesToFire  int64   `json:"breaches_to_fire"`
	ClearsToResolve int64   `json:"clears_to_resolve"`
	Enabled         bool    `json:"enabled"`
	DeferToBuiltin  *string `json:"defer_to_builtin,omitempty"`
}

type MonitorTemplateParent struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	BuiltinKey *string `json:"builtin_key"`
}

// MonitorTemplate is the response shape (`monitor_templates::TemplateDoc`).
type MonitorTemplate struct {
	ID          string                  `json:"id"`
	TenantID    *string                 `json:"tenant_id"`
	BuiltinKey  *string                 `json:"builtin_key"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Source      string                  `json:"source"`
	Parents     []MonitorTemplateParent `json:"parents"`
	Macros      []MonitorTemplateMacro  `json:"macros"`
	Rules       []MonitorTemplateRule   `json:"rules"`
}

// MonitorTemplateInput is the create/replace body
// (`monitor_templates::TemplateInput`).
type MonitorTemplateInput struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ParentIDs   []string               `json:"parent_ids"`
	Macros      []MonitorTemplateMacro `json:"macros"`
	Rules       []MonitorTemplateRule  `json:"rules"`
}

type MonitorTemplateSummary struct {
	ID          string  `json:"id"`
	TenantID    *string `json:"tenant_id"`
	BuiltinKey  *string `json:"builtin_key"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Source      string  `json:"source"`
}

func (c *Client) ListMonitorTemplates(ctx context.Context) ([]MonitorTemplateSummary, error) {
	var out struct {
		Templates []MonitorTemplateSummary `json:"templates"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/monitor-templates", nil, &out); err != nil {
		return nil, err
	}
	return out.Templates, nil
}

func (c *Client) CreateMonitorTemplate(ctx context.Context, in MonitorTemplateInput) (*MonitorTemplate, error) {
	var out MonitorTemplate
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/monitor-templates", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetMonitorTemplate(ctx context.Context, id string) (*MonitorTemplate, error) {
	var out MonitorTemplate
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/monitor-templates/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateMonitorTemplate(ctx context.Context, id string, in MonitorTemplateInput) (*MonitorTemplate, error) {
	var out MonitorTemplate
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/monitor-templates/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteMonitorTemplate(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/monitor-templates/"+id, nil, nil)
}

// MonitorTemplateLink: GroupID, DeviceID, or neither (whole organization).
type MonitorTemplateLink struct {
	ID           string  `json:"id,omitempty"`
	TemplateID   string  `json:"template_id"`
	TemplateName string  `json:"template_name,omitempty"`
	GroupID      *string `json:"group_id,omitempty"`
	DeviceID     *string `json:"device_id,omitempty"`
}

func (c *Client) CreateMonitorTemplateLink(ctx context.Context, l MonitorTemplateLink) (*MonitorTemplateLink, error) {
	var out MonitorTemplateLink
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/monitor-template-links", l, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetMonitorTemplateLink(ctx context.Context, id string) (*MonitorTemplateLink, error) {
	var out MonitorTemplateLink
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/monitor-template-links/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteMonitorTemplateLink(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/monitor-template-links/"+id, nil, nil)
}

// MonitorMacroOverride: one macro value on a device, a group or (both
// empty) the whole organization.
type MonitorMacroOverride struct {
	ID       string  `json:"id,omitempty"`
	Macro    string  `json:"macro"`
	Context  *string `json:"context,omitempty"`
	Value    string  `json:"value"`
	GroupID  *string `json:"group_id,omitempty"`
	DeviceID *string `json:"device_id,omitempty"`
}

func (c *Client) CreateMonitorMacroOverride(ctx context.Context, o MonitorMacroOverride) (*MonitorMacroOverride, error) {
	var out MonitorMacroOverride
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/monitor-macro-overrides", o, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetMonitorMacroOverride(ctx context.Context, id string) (*MonitorMacroOverride, error) {
	var out MonitorMacroOverride
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/monitor-macro-overrides/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateMonitorMacroOverrideValue(ctx context.Context, id, value string) (*MonitorMacroOverride, error) {
	var out MonitorMacroOverride
	body := map[string]string{"value": value}
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/monitor-macro-overrides/"+url.PathEscape(id), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteMonitorMacroOverride(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/monitor-macro-overrides/"+url.PathEscape(id), nil, nil)
}
