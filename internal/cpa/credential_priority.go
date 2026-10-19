package cpa

import (
	"context"
	"net/http"
)

type authFilePriorityPatchRequest struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
}

func (c *Client) UpdateAuthFilePriority(ctx context.Context, name string, priority int) (int, error) {
	statusCode, _, err := c.doManagementJSONRequestWithBody(ctx, http.MethodPatch, cpaManagementAuthFilesFieldsEndpoint,
		authFilePriorityPatchRequest{Name: name, Priority: priority}, nil, "auth file priority")
	return statusCode, err
}
