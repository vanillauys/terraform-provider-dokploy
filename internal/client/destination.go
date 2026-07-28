package client

import (
	"context"
	"net/url"
)

// Destination is an S3-compatible bucket Dokploy writes backups to.
//
// Both credentials come back in CLEARTEXT from destination.one and
// destination.all. The corresponding attributes are Sensitive, and nothing
// in this repo may log them.
type Destination struct {
	DestinationID   string   `json:"destinationId"`
	Name            string   `json:"name"`
	Provider        string   `json:"provider"`
	Endpoint        string   `json:"endpoint"`
	Bucket          string   `json:"bucket"`
	Region          string   `json:"region"`
	AccessKey       string   `json:"accessKey"`
	SecretAccessKey string   `json:"secretAccessKey"`
	AdditionalFlags []string `json:"additionalFlags"`
	CreatedAt       string   `json:"createdAt"`
}

// CreateDestinationRequest.
//
// serverId is deliberately absent. destination.create accepts it, but
// destination.one and destination.all do not return it (verified live,
// v0.29.13, 2026-07-28: the response carries exactly destinationId, name,
// provider, accessKey, secretAccessKey, bucket, region, endpoint,
// additionalFlags, organizationId, createdAt). A field that can be written
// but never read back cannot round-trip: state would hold the configured
// value, Read could not confirm it, and either the attribute lies or every
// plan shows a diff. Recorded in censusExempt with this reasoning.
type CreateDestinationRequest struct {
	Name            string   `json:"name"`
	Provider        string   `json:"provider"`
	Endpoint        string   `json:"endpoint"`
	Bucket          string   `json:"bucket"`
	Region          string   `json:"region"`
	AccessKey       string   `json:"accessKey"`
	SecretAccessKey string   `json:"secretAccessKey"`
	AdditionalFlags []string `json:"additionalFlags"`
}

// UpdateDestinationRequest carries the full field set. destination.update is
// not one of the classified dialects — it is a create-shaped body plus an id
// — and the resource always sends every field from the model, so absent-key
// semantics never come into play.
type UpdateDestinationRequest struct {
	DestinationID   string   `json:"destinationId"`
	Name            string   `json:"name"`
	Provider        string   `json:"provider"`
	Endpoint        string   `json:"endpoint"`
	Bucket          string   `json:"bucket"`
	Region          string   `json:"region"`
	AccessKey       string   `json:"accessKey"`
	SecretAccessKey string   `json:"secretAccessKey"`
	AdditionalFlags []string `json:"additionalFlags"`
}

func (c *Client) CreateDestination(ctx context.Context, req CreateDestinationRequest) (*Destination, error) {
	var d Destination
	if err := c.Post(ctx, "/destination.create", req, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) GetDestination(ctx context.Context, id string) (*Destination, error) {
	var d Destination
	if err := c.Get(ctx, "/destination.one", url.Values{"destinationId": {id}}, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) ListDestinations(ctx context.Context) ([]Destination, error) {
	var ds []Destination
	if err := c.Get(ctx, "/destination.all", nil, &ds); err != nil {
		return nil, err
	}
	return ds, nil
}

func (c *Client) UpdateDestination(ctx context.Context, req UpdateDestinationRequest) error {
	return c.Post(ctx, "/destination.update", req, nil)
}

// DeleteDestination. Note the verb: destination uses .remove.
func (c *Client) DeleteDestination(ctx context.Context, id string) error {
	return c.Post(ctx, "/destination.remove", map[string]string{"destinationId": id}, nil)
}
