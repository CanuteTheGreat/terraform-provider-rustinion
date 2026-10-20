package client

import (
	"context"
	"net/http"
)

// NotificationRoute mirrors `routes::customer_api::NotificationRoute`
// on the wire. `scope` is one of "tenant"/"group"/"rule"/"tag";
// `scope_id` is set for "group" (a real `groups.id`) or "rule" (a real
// `alert_rules.id`), and `tag` is set for "tag" — exactly one of
// `scope_id`/`tag` is ever non-nil, matching the server's own check
// constraint. A "tag" route only ever matches device-scoped compliance
// findings (cpu_high, disk_full, stale, etc.) — never an alert-rule/
// log-alert/SNMP-trap/SLO/synthetic-check-driven finding, which has no
// device tags of its own. See notifications.rs's own
// `channels_for_finding` doc comment in the main repo for the full
// reasoning.
type NotificationRoute struct {
	ID        string  `json:"id"`
	ChannelID string  `json:"channel_id"`
	Scope     string  `json:"scope"`
	ScopeID   *string `json:"scope_id"`
	Tag       *string `json:"tag"`
}

// NotificationRouteCreateInput mirrors `routes::customer_api::
// CreateNotificationRouteInput`. There is no update input — a route's
// scope is immutable, delete+recreate to change (same shape the admin
// UI's own add/remove-only routes page already has).
type NotificationRouteCreateInput struct {
	Scope   string  `json:"scope"`
	ScopeID *string `json:"scope_id,omitempty"`
	Tag     *string `json:"tag,omitempty"`
}

func (c *Client) CreateNotificationRoute(ctx context.Context, channelID string, in NotificationRouteCreateInput) (*NotificationRoute, error) {
	var out NotificationRoute
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/notification-channels/"+channelID+"/routes", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListNotificationRoutes(ctx context.Context, channelID string) ([]NotificationRoute, error) {
	var out []NotificationRoute
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/notification-channels/"+channelID+"/routes", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetNotificationRoute has no dedicated server endpoint — a route is
// small enough, and its own list cheap enough, that filtering the
// channel's list client-side is simpler than adding a get-by-id
// endpoint that would exist only for this one lookup.
func (c *Client) GetNotificationRoute(ctx context.Context, channelID, routeID string) (*NotificationRoute, error) {
	routes, err := c.ListNotificationRoutes(ctx, channelID)
	if err != nil {
		return nil, err
	}
	for _, r := range routes {
		if r.ID == routeID {
			return &r, nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Message: "no such notification route"}
}

func (c *Client) DeleteNotificationRoute(ctx context.Context, channelID, routeID string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/notification-channels/"+channelID+"/routes/"+routeID, nil, nil)
}
