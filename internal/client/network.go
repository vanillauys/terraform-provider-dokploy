package client

import (
	"context"
	"net/url"
)

// NetworkIPAMConfig is one address pool inside a network's IPAM block. The
// three fields carry omitempty deliberately (a documented exception to the
// no-omitempty rule): in the zod schema each is optional and NOT nullable,
// so an explicit null is a 400 and absence is the only correct "unset".
// They live inside a one-shot create body, so absent-key drift across
// applies - the reason the rule exists - cannot occur.
type NetworkIPAMConfig struct {
	Subnet  string `json:"subnet,omitempty"`
	Gateway string `json:"gateway,omitempty"`
	IPRange string `json:"ipRange,omitempty"`
}

// NetworkIPAM mirrors Docker's IPAM block on network.create. Same omitempty
// reasoning as NetworkIPAMConfig.
type NetworkIPAM struct {
	Driver string              `json:"driver,omitempty"`
	Config []NetworkIPAMConfig `json:"config,omitempty"`
}

// Network is a Docker network Dokploy manages. Docker networks are
// immutable: there is no network.update endpoint, and the resource marks
// every attribute RequiresReplace.
type Network struct {
	NetworkID  string       `json:"networkId"`
	Name       string       `json:"name"`
	Driver     string       `json:"driver"`
	Internal   bool         `json:"internal"`
	Attachable bool         `json:"attachable"`
	EnableIPv4 bool         `json:"enableIPv4"`
	EnableIPv6 bool         `json:"enableIPv6"`
	MTU        *int64       `json:"mtu"`
	IPAM       *NetworkIPAM `json:"ipam"`
	ServerID   *string      `json:"serverId"`
	CreatedAt  string       `json:"createdAt"`
}

// CreateNetworkRequest carries every network.create field. The nullable
// trio (mtu, ipam, serverId) is sent as an explicit JSON null when unset -
// the schema declares all three nullable, verified live in the wave 6b
// probes (doc.go).
//
// The zero value is NOT a valid request. A zero Driver marshals
// `"driver":""`, and the server 400s on it rather than applying its own
// default; a zero EnableIPv4 marshals `"enableIPv4":false`, flipping the
// server's own default of true. Set Driver and EnableIPv4 explicitly, or
// copy acctest.CreateNetwork's shape.
type CreateNetworkRequest struct {
	Name       string       `json:"name"`
	Driver     string       `json:"driver"`
	Internal   bool         `json:"internal"`
	Attachable bool         `json:"attachable"`
	EnableIPv4 bool         `json:"enableIPv4"`
	EnableIPv6 bool         `json:"enableIPv6"`
	MTU        *int64       `json:"mtu"`
	IPAM       *NetworkIPAM `json:"ipam"`
	ServerID   *string      `json:"serverId"`
}

func (c *Client) CreateNetwork(ctx context.Context, req CreateNetworkRequest) (*Network, error) {
	var n Network
	if err := c.Post(ctx, "/network.create", req, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

func (c *Client) GetNetwork(ctx context.Context, id string) (*Network, error) {
	var n Network
	if err := c.Get(ctx, "/network.one", url.Values{"networkId": {id}}, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

func (c *Client) ListNetworks(ctx context.Context) ([]Network, error) {
	var ns []Network
	if err := c.Get(ctx, "/network.all", nil, &ns); err != nil {
		return nil, err
	}
	return ns, nil
}

// DeleteNetwork. Note the verb: network uses .remove, like destination.
func (c *Client) DeleteNetwork(ctx context.Context, id string) error {
	return c.Post(ctx, "/network.remove", map[string]string{"networkId": id}, nil)
}
