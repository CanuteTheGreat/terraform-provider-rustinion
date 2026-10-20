package client

import (
	"context"
	"fmt"
	"net/http"
)

// OnCallMember mirrors `routes::customer_api::OnCallMemberInput`/
// `CustomerOnCallMember`. `position` is deliberately not tracked here —
// same "order is the position" reasoning as `EscalationStep`'s own doc
// comment — the server assigns it from this member's index in the
// parent schedule's own array on every write.
type OnCallMember struct {
	ID     string `json:"id,omitempty"`
	UserID string `json:"user_id"`
}

// OnCallOverride mirrors `routes::customer_api::OnCallOverrideInput`/
// `CustomerOnCallOverride`. Unlike every other nested type in this
// package, an override is never part of its parent schedule's own
// `PUT` — it has its own create/delete endpoints (see
// `CreateOnCallOverride`/`DeleteOnCallOverride`), since it's an
// independent, time-bounded event rather than part of the schedule's
// static config — see `resource_on_call_override.go`'s own doc comment.
type OnCallOverride struct {
	ID       string `json:"id,omitempty"`
	UserID   string `json:"user_id"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
}

// OnCallSchedule mirrors `routes::customer_api::OnCallScheduleInput`/
// `CustomerOnCallSchedule`. `StartedAt` is server-assigned (`now()` at
// creation) and never accepted on the way in; `Overrides` is populated
// on read but never sent on a `PUT` — the server ignores it there, see
// `update_on_call_schedule`'s own doc comment for why.
type OnCallSchedule struct {
	ID           string           `json:"id,omitempty"`
	GroupID      string           `json:"group_id"`
	Name         string           `json:"name"`
	RotationDays int64            `json:"rotation_days"`
	StartedAt    string           `json:"started_at,omitempty"`
	Members      []OnCallMember   `json:"members"`
	Overrides    []OnCallOverride `json:"overrides,omitempty"`
}

func (c *Client) CreateOnCallSchedule(ctx context.Context, s OnCallSchedule) (*OnCallSchedule, error) {
	var out OnCallSchedule
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/on-call-schedules", s, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetOnCallSchedule(ctx context.Context, id string) (*OnCallSchedule, error) {
	var out OnCallSchedule
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/on-call-schedules/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateOnCallSchedule(ctx context.Context, id string, s OnCallSchedule) (*OnCallSchedule, error) {
	var out OnCallSchedule
	path := fmt.Sprintf("/api/v1/customer/on-call-schedules/%s", id)
	if err := c.do(ctx, http.MethodPut, path, s, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteOnCallSchedule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/on-call-schedules/"+id, nil, nil)
}

func (c *Client) CreateOnCallOverride(ctx context.Context, scheduleID string, o OnCallOverride) (*OnCallOverride, error) {
	var out OnCallOverride
	path := fmt.Sprintf("/api/v1/customer/on-call-schedules/%s/overrides", scheduleID)
	if err := c.do(ctx, http.MethodPost, path, o, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOnCallOverride has no dedicated server endpoint — an override's
// only server-side read path is as part of its parent schedule's own
// `GET` (`OnCallSchedule.Overrides`). This fetches that schedule and
// finds the matching override client-side, giving
// `resource_on_call_override.go`'s own `Read` a real way to detect the
// override having been deleted out of band (with `client.IsNotFound`
// reported the same way a per-resource `GET :id` would, both when the
// parent schedule itself is gone and when it exists but the override
// no longer does).
func (c *Client) GetOnCallOverride(ctx context.Context, scheduleID, overrideID string) (*OnCallOverride, error) {
	schedule, err := c.GetOnCallSchedule(ctx, scheduleID)
	if err != nil {
		return nil, err
	}
	for _, o := range schedule.Overrides {
		if o.ID == overrideID {
			return &o, nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Message: "no such on-call override"}
}

func (c *Client) DeleteOnCallOverride(ctx context.Context, scheduleID, overrideID string) error {
	path := fmt.Sprintf("/api/v1/customer/on-call-schedules/%s/overrides/%s", scheduleID, overrideID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
