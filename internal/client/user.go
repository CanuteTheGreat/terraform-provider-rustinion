package client

import (
	"context"
	"fmt"
	"net/http"
)

// User mirrors `routes::customer_api::UserInput`/`CustomerUser` — a
// tenant member (one `user_roles` row, this struct's own `ID`) joined
// with its underlying `users` row for `Email`. `UserID` is the
// cross-tenant identity (`users.id`) — useful to reference the invited
// user elsewhere (an on-call schedule member, say) without a second
// lookup.
type User struct {
	ID          string  `json:"id,omitempty"`
	UserID      string  `json:"user_id,omitempty"`
	Email       string  `json:"email"`
	GroupID     *string `json:"group_id,omitempty"`
	AccessLevel string  `json:"access_level,omitempty"`
}

type updateUserAccessLevelRequest struct {
	AccessLevel string `json:"access_level"`
}

func (c *Client) CreateUser(ctx context.Context, u User) (*User, error) {
	var out User
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/users", u, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	var out User
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/users/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateUserAccessLevel is the only update this API offers for a user
// — it never touches `Email`/`GroupID`, both fixed at invite time (see
// `resource_user.go`'s own doc comment).
func (c *Client) UpdateUserAccessLevel(ctx context.Context, id, accessLevel string) (*User, error) {
	var out User
	path := fmt.Sprintf("/api/v1/customer/users/%s", id)
	if err := c.do(ctx, http.MethodPut, path, updateUserAccessLevelRequest{AccessLevel: accessLevel}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/users/"+id, nil, nil)
}
