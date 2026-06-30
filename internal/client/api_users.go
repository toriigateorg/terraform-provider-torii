package client

import (
	"context"
	"net/url"
)

// APIUser is a passwordless Service API user: a machine identity that holds one
// bearer token (torii_sat_...) and a set of role assignments. The plaintext
// Token is only populated on create and regenerate; the GET endpoint never
// returns it.
type APIUser struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Prefix      string  `json:"prefix"`
	Disabled    bool    `json:"disabled"`
	CreatedAt   string  `json:"created_at"`
	ExpiresAt   *string `json:"expires_at"`
	LastUsedAt  *string `json:"last_used_at"`
	Token       string  `json:"token"`
}

type APIUserCreate struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	RoleIDs     []string `json:"role_ids,omitempty"`
	ExpiresAt   *string  `json:"expires_at,omitempty"`
}

func (c *Client) CreateAPIUser(ctx context.Context, in APIUserCreate) (*APIUser, error) {
	var out APIUser
	if err := c.do(ctx, "POST", "/api/v1/admin/api_users", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetAPIUser(ctx context.Context, id string) (*APIUser, error) {
	var out APIUser
	if err := c.do(ctx, "GET", "/api/v1/admin/api_users/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteAPIUser(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/v1/admin/api_users/"+url.PathEscape(id), nil, nil)
}
