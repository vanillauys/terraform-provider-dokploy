package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Port is a published port on an application (Docker swarm port mapping).
//
// port.one also returns an expanded `application` object, which is not
// modelled — see the same note on Mount.
type Port struct {
	PortID        string `json:"portId"`
	ApplicationID string `json:"applicationId"`
	PublishedPort int64  `json:"publishedPort"`
	TargetPort    int64  `json:"targetPort"`
	Protocol      string `json:"protocol"`    // tcp | udp
	PublishMode   string `json:"publishMode"` // host | ingress
}

// PortProtocols and PortPublishModes are the enums port.create validates
// against, recovered from its zod error body.
var (
	PortProtocols   = []string{"tcp", "udp"}
	PortPublishMode = []string{"host", "ingress"}
)

// CreatePortRequest. Unlike redirects and security, port.create DOES return
// the record it created, so no createAndLocate dance is needed here.
type CreatePortRequest struct {
	ApplicationID string `json:"applicationId"`
	PublishedPort int64  `json:"publishedPort"`
	TargetPort    int64  `json:"targetPort"`
	Protocol      string `json:"protocol"`
	PublishMode   string `json:"publishMode"`
}

// UpdatePortRequest. port.update is dialect A in its strictest form: the
// full field set is required and a body of {portId} alone is HTTP 400
// naming every missing field. There is no partial update, and no field is
// nullable, so nothing here is a pointer.
type UpdatePortRequest struct {
	PortID        string `json:"portId"`
	PublishedPort int64  `json:"publishedPort"`
	TargetPort    int64  `json:"targetPort"`
	Protocol      string `json:"protocol"`
	PublishMode   string `json:"publishMode"`
}

func (c *Client) CreatePort(ctx context.Context, req CreatePortRequest) (*Port, error) {
	var p Port
	if err := c.Post(ctx, "/port.create", req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPort. port.one is the ONE read endpoint in this API that reports a
// missing record as HTTP 400 rather than 404. Probed live (v0.29.13,
// 2026-07-28) with a nonexistent id against six read endpoints:
//
//	port.one          400  "Port not found"      <- the odd one out
//	redirects.one     404  "Redirect not found"
//	security.one      404  "Security not found"
//	mounts.one        404  "Mount not found"
//	application.one   404  "Application not found"
//	domain.one        404  "Domain not found"
//
// Left unmapped, a port deleted outside Terraform surfaces as a hard apply
// error instead of drift the provider can reconcile, because Read only
// removes the resource from state on ErrNotFound. So this one endpoint's
// 400-with-"not found" is translated here rather than in the generic
// transport, which must keep treating 400 as a real error for every other
// route.
func (c *Client) GetPort(ctx context.Context, id string) (*Port, error) {
	var p Port
	if err := c.Get(ctx, "/port.one", url.Values{"portId": {id}}, &p); err != nil {
		var de *DokployError
		if errors.As(err, &de) && de.HTTPStatus == http.StatusBadRequest &&
			strings.Contains(strings.ToLower(de.Message), "not found") {
			return nil, fmt.Errorf("GET /port.one: %w", ErrNotFound)
		}
		return nil, err
	}
	return &p, nil
}

func (c *Client) UpdatePort(ctx context.Context, req UpdatePortRequest) error {
	return c.Post(ctx, "/port.update", req, nil)
}

// DeletePort. Note the verb: port/redirects/security use .delete, while
// mounts uses .remove.
func (c *Client) DeletePort(ctx context.Context, id string) error {
	return c.Post(ctx, "/port.delete", map[string]string{"portId": id}, nil)
}
