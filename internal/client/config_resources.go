package client

import (
	"context"
	"net/http"
	"net/url"
)

// SNMPTarget mirrors `routes::config_resources::SnmpTargetInput` (request)
// and `CustomerSnmpTarget` (response). Secrets (Community, V3AuthKey,
// V3PrivKey) are write-only: the API never returns them, only the
// Has* flags. On update a nil secret keeps the stored value and "" clears it.
type SNMPTarget struct {
	ID                  string  `json:"id,omitempty"`
	GroupID             string  `json:"group_id,omitempty"`
	Name                string  `json:"name"`
	Host                string  `json:"host"`
	Port                int64   `json:"port"`
	Version             string  `json:"version"`
	Community           *string `json:"community,omitempty"`
	V3Username          *string `json:"v3_username,omitempty"`
	V3AuthProtocol      *string `json:"v3_auth_protocol,omitempty"`
	V3AuthKey           *string `json:"v3_auth_key,omitempty"`
	V3PrivProtocol      *string `json:"v3_priv_protocol,omitempty"`
	V3PrivKey           *string `json:"v3_priv_key,omitempty"`
	PollIntervalSeconds int64   `json:"poll_interval_seconds"`
	Enabled             bool    `json:"enabled"`
	HasCommunity        bool    `json:"has_community,omitempty"`
	HasV3AuthKey        bool    `json:"has_v3_auth_key,omitempty"`
	HasV3PrivKey        bool    `json:"has_v3_priv_key,omitempty"`
}

func (c *Client) CreateSNMPTarget(ctx context.Context, groupID string, t SNMPTarget) (*SNMPTarget, error) {
	var out SNMPTarget
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/groups/"+groupID+"/snmp-targets", t, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetSNMPTarget(ctx context.Context, id string) (*SNMPTarget, error) {
	var out SNMPTarget
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/snmp-targets/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateSNMPTarget(ctx context.Context, id string, t SNMPTarget) (*SNMPTarget, error) {
	var out SNMPTarget
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/snmp-targets/"+id, t, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteSNMPTarget(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/snmp-targets/"+id, nil, nil)
}

// DeviceTags is the `GET /api/v1/customer/devices/:id/tags` response.
type DeviceTags struct {
	DeviceID string   `json:"device_id"`
	Tags     []string `json:"tags"`
}

// DeviceTag is the `PUT .../tags/:tag` response; Tag is the normalized
// (trimmed, lowercased) tag the server stored.
type DeviceTag struct {
	DeviceID string `json:"device_id"`
	Tag      string `json:"tag"`
}

func (c *Client) ListDeviceTags(ctx context.Context, deviceID string) (*DeviceTags, error) {
	var out DeviceTags
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/devices/"+deviceID+"/tags", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) AddDeviceTag(ctx context.Context, deviceID, tag string) (*DeviceTag, error) {
	var out DeviceTag
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/devices/"+deviceID+"/tags/"+url.PathEscape(tag), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RemoveDeviceTag(ctx context.Context, deviceID, tag string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/devices/"+deviceID+"/tags/"+url.PathEscape(tag), nil, nil)
}

// MaintenanceWindow mirrors `MaintenanceWindowInput`/`CustomerMaintenanceWindow`.
// No update endpoint (create/read/cancel, like the admin UI); DELETE cancels.
type MaintenanceWindow struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name"`
	Scope       string  `json:"scope,omitempty"`
	GroupID     *string `json:"group_id,omitempty"`
	StartsAt    string  `json:"starts_at"`
	EndsAt      string  `json:"ends_at"`
	Recurrence  string  `json:"recurrence,omitempty"`
	CancelledAt *string `json:"cancelled_at,omitempty"`
}

func (c *Client) CreateMaintenanceWindow(ctx context.Context, w MaintenanceWindow) (*MaintenanceWindow, error) {
	var out MaintenanceWindow
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/maintenance-windows", w, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetMaintenanceWindow(ctx context.Context, id string) (*MaintenanceWindow, error) {
	var out MaintenanceWindow
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/maintenance-windows/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CancelMaintenanceWindow(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/maintenance-windows/"+id, nil, nil)
}

// RUMApplication mirrors `CustomerRumApplication`. ClientToken is
// server-generated and public by design (it goes into the page snippet).
type RUMApplication struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	ClientToken string `json:"client_token,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

func (c *Client) CreateRUMApplication(ctx context.Context, name string) (*RUMApplication, error) {
	var out RUMApplication
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/rum-applications", RUMApplication{Name: name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetRUMApplication(ctx context.Context, id string) (*RUMApplication, error) {
	var out RUMApplication
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/rum-applications/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RenameRUMApplication(ctx context.Context, id, name string) (*RUMApplication, error) {
	var out RUMApplication
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/rum-applications/"+id, RUMApplication{Name: name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteRUMApplication(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/rum-applications/"+id, nil, nil)
}

// ReportSchedule mirrors `ReportScheduleInput`/`CustomerReportSchedule`.
type ReportSchedule struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	ReportType string   `json:"report_type"`
	GroupID    *string  `json:"group_id"`
	Cadence    string   `json:"cadence"`
	Format     string   `json:"format"`
	Recipients []string `json:"recipients"`
	Enabled    bool     `json:"enabled"`
	LastSentAt *string  `json:"last_sent_at,omitempty"`
}

func (c *Client) CreateReportSchedule(ctx context.Context, r ReportSchedule) (*ReportSchedule, error) {
	var out ReportSchedule
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/report-schedules", r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetReportSchedule(ctx context.Context, id string) (*ReportSchedule, error) {
	var out ReportSchedule
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/report-schedules/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateReportSchedule(ctx context.Context, id string, r ReportSchedule) (*ReportSchedule, error) {
	var out ReportSchedule
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/report-schedules/"+id, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteReportSchedule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/report-schedules/"+id, nil, nil)
}
