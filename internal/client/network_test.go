package client

import (
	"context"
	"net/http"
	"testing"
)

// networkJSON is the exact shape network.create/one/all return, captured
// live on the rig (v0.30.2, wave 6b task 1). organizationId is omitted here
// the same way destinationJSON omits it: implied by the API key, and not a
// field this client models.
const networkJSON = `{
	"networkId": "n1",
	"name": "backend-net",
	"driver": "bridge",
	"internal": false,
	"attachable": true,
	"enableIPv4": true,
	"enableIPv6": false,
	"mtu": 1400,
	"ipam": {"config": [{"subnet": "172.28.0.0/16", "gateway": "172.28.0.1", "ipRange": "172.28.5.0/24"}], "driver": "default"},
	"createdAt": "2026-08-20T00:00:00.000Z",
	"serverId": null
}`

func TestCreateGetAndListNetwork(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/network.create", Status: 200, Body: networkJSON},
		route{Method: http.MethodGet, Path: "/api/network.one", Status: 200, Body: networkJSON},
		route{Method: http.MethodGet, Path: "/api/network.all", Status: 200, Body: "[" + networkJSON + "]"},
	)
	defer srv.Close()
	c := testClient(t, srv)

	n, err := c.CreateNetwork(context.Background(), CreateNetworkRequest{Name: "backend-net"})
	if err != nil {
		t.Fatalf("CreateNetwork: %v", err)
	}
	// Every field asserted: an unasserted field with a typo'd tag decodes
	// silently wrong and stays green.
	if n.NetworkID != "n1" || n.Name != "backend-net" || n.Driver != "bridge" ||
		n.Internal || !n.Attachable || !n.EnableIPv4 || n.EnableIPv6 ||
		n.CreatedAt != "2026-08-20T00:00:00.000Z" {
		t.Errorf("network = %+v", n)
	}
	if n.MTU == nil || *n.MTU != 1400 {
		t.Errorf("mtu = %v, want 1400", n.MTU)
	}
	if n.ServerID != nil {
		t.Errorf("serverId = %v, want nil", n.ServerID)
	}
	if n.IPAM == nil || n.IPAM.Driver != "default" || len(n.IPAM.Config) != 1 ||
		n.IPAM.Config[0].Subnet != "172.28.0.0/16" ||
		n.IPAM.Config[0].Gateway != "172.28.0.1" ||
		n.IPAM.Config[0].IPRange != "172.28.5.0/24" {
		t.Errorf("ipam = %+v", n.IPAM)
	}

	if got, err := c.GetNetwork(context.Background(), "n1"); err != nil || got.NetworkID != "n1" {
		t.Errorf("GetNetwork = %+v, %v", got, err)
	}
	list, err := c.ListNetworks(context.Background())
	if err != nil || len(list) != 1 || list[0].NetworkID != "n1" {
		t.Errorf("ListNetworks = %+v, %v", list, err)
	}
}

func TestDeleteNetwork(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/network.remove", Status: 200, Body: `true`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	if err := c.DeleteNetwork(context.Background(), "n1"); err != nil {
		t.Fatalf("DeleteNetwork: %v", err)
	}
}
