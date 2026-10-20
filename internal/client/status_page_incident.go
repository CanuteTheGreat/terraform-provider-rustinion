package client

import (
	"context"
	"net/http"
)

// StatusPageIncident mirrors `status_pages::IncidentRow` on the wire —
// the shape every read (`Get`/list, and the body `Create`/`Update`
// return) comes back as. Unlike `StatusPage`, an incident is not a
// singleton — a page has many of them — so it gets a real, stable ID
// and its own CRUD path, same shape as every other non-singleton
// resource in this package.
//
// `Source` is never sent on a write (there is no `source` field on
// either input struct below) — it is only ever read back, either "auto"
// (exclusively written by the server's own synthetic-check hysteresis,
// `status_pages::run_tick` in the main repo) or "manual". The server
// rejects any `Update`/`Delete`/`PostStatusPageIncidentUpdate` against a
// `source = "auto"` incident with a 409 (see `IsConflict`) — this
// provider does not special-case that here; it surfaces as a plain
// error, same as any other API error.
type StatusPageIncident struct {
	ID          string  `json:"id"`
	ComponentID *string `json:"component_id"`
	Source      string  `json:"source"`
	Title       string  `json:"title"`
	Status      string  `json:"status"`
}

// StatusPageIncidentCreateInput mirrors `routes::customer_api::
// CreateIncidentInput`. `Message` is the incident's own opening update
// — required, since an incident can't open without one — but is never
// stored on `StatusPageIncident` itself; it lives only in that opening
// `status_page_incident_updates` row.
type StatusPageIncidentCreateInput struct {
	Title       string  `json:"title"`
	ComponentID *string `json:"component_id,omitempty"`
	Status      string  `json:"status"`
	Message     string  `json:"message"`
}

// StatusPageIncidentUpdateInput mirrors `routes::customer_api::
// UpdateIncidentInput` — edits only `title`/`component_id`; the
// server's own `PUT` has no `status` field at all. A status change only
// ever happens through `PostStatusPageIncidentUpdate`.
type StatusPageIncidentUpdateInput struct {
	Title       string  `json:"title"`
	ComponentID *string `json:"component_id,omitempty"`
}

func (c *Client) CreateStatusPageIncident(ctx context.Context, in StatusPageIncidentCreateInput) (*StatusPageIncident, error) {
	var out StatusPageIncident
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/status-page/incidents", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetStatusPageIncident(ctx context.Context, id string) (*StatusPageIncident, error) {
	var out StatusPageIncident
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/status-page/incidents/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateStatusPageIncident(ctx context.Context, id string, in StatusPageIncidentUpdateInput) (*StatusPageIncident, error) {
	var out StatusPageIncident
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/status-page/incidents/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteStatusPageIncident(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/status-page/incidents/"+id, nil, nil)
}

// PostStatusPageIncidentUpdate posts a real, timestamped update to an
// existing incident's own timeline. There is deliberately no
// corresponding Get/Update/Delete for an individual update — see
// resource_status_page_incident.go's own doc comment.
func (c *Client) PostStatusPageIncidentUpdate(ctx context.Context, id, status, message string) error {
	body := map[string]string{"status": status, "message": message}
	return c.do(ctx, http.MethodPost, "/api/v1/customer/status-page/incidents/"+id+"/updates", body, nil)
}
