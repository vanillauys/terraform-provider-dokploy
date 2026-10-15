package client

import (
	"context"
	"net/url"
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

// GetPort uses the shared strict resource-absence envelope validation,
// including this endpoint's documented HTTP 400 variant.
func (c *Client) GetPort(ctx context.Context, id string) (*Port, error) {
	var p Port
	if err := c.Get(ctx, "/port.one", url.Values{"portId": {id}}, &p); err != nil {
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
