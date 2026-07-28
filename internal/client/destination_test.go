package client

import (
	"context"
	"net/http"
	"testing"
)

// destinationJSON is the exact shape destination.create/one/all return,
// captured live (v0.29.13, 2026-07-28). Note the absence of serverId: the
// create endpoint accepts it, the read endpoints never report it.
const destinationJSON = `{
	"destinationId": "d1",
	"name": "vnly-io-dokploy",
	"provider": "Cloudflare",
	"accessKey": "AK",
	"secretAccessKey": "SK",
	"bucket": "b",
	"region": "WEUR",
	"endpoint": "https://x.r2.cloudflarestorage.com",
	"additionalFlags": [],
	"organizationId": "org1",
	"createdAt": "2026-07-28T00:00:00.000Z"
}`

func TestCreateGetAndListDestination(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodPost, Path: "/api/destination.create", Status: 200, Body: destinationJSON},
		route{Method: http.MethodGet, Path: "/api/destination.one", Status: 200, Body: destinationJSON},
		route{Method: http.MethodGet, Path: "/api/destination.all", Status: 200, Body: "[" + destinationJSON + "]"},
	)
	defer srv.Close()
	c := testClient(t, srv)

	d, err := c.CreateDestination(context.Background(), CreateDestinationRequest{Name: "vnly-io-dokploy"})
	if err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}
	// Every field asserted: an unasserted field with a typo'd tag decodes
	// silently wrong and stays green.
	if d.DestinationID != "d1" || d.Name != "vnly-io-dokploy" || d.Provider != "Cloudflare" ||
		d.AccessKey != "AK" || d.SecretAccessKey != "SK" || d.Bucket != "b" ||
		d.Region != "WEUR" || d.Endpoint != "https://x.r2.cloudflarestorage.com" ||
		d.CreatedAt != "2026-07-28T00:00:00.000Z" {
		t.Errorf("destination = %+v", d)
	}
	if d.AdditionalFlags == nil || len(d.AdditionalFlags) != 0 {
		t.Errorf("additionalFlags = %#v, want an empty (non-nil) slice: the server "+
			"stores [] rather than null, and the schema default depends on that", d.AdditionalFlags)
	}

	if got, err := c.GetDestination(context.Background(), "d1"); err != nil || got.DestinationID != "d1" {
		t.Errorf("GetDestination = %+v, %v", got, err)
	}
	list, err := c.ListDestinations(context.Background())
	if err != nil || len(list) != 1 || list[0].DestinationID != "d1" {
		t.Errorf("ListDestinations = %+v, %v", list, err)
	}
}
