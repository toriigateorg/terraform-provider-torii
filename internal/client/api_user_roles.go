package client

import (
	"context"
	"net/url"
)

func (c *Client) ListAPIUserRoles(ctx context.Context, apiUserID string) ([]Role, error) {
	var out userRolesResp
	if err := c.do(ctx, "GET", "/api/v1/admin/api_users/"+url.PathEscape(apiUserID)+"/roles", nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *Client) AssignAPIUserRole(ctx context.Context, apiUserID, roleID string) error {
	return c.do(ctx, "POST", "/api/v1/admin/api_users/"+url.PathEscape(apiUserID)+"/roles",
		userRoleAssignReq{RoleID: roleID}, nil)
}

func (c *Client) RevokeAPIUserRole(ctx context.Context, apiUserID, roleID string) error {
	return c.do(ctx, "DELETE",
		"/api/v1/admin/api_users/"+url.PathEscape(apiUserID)+"/roles/"+url.PathEscape(roleID),
		nil, nil)
}
