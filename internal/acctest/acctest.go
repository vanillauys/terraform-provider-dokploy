// Package acctest holds shared helpers for acceptance tests.
// Tests require a disposable Dokploy instance:
//
//	./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc
package acctest

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	sdkacctest "github.com/hashicorp/terraform-plugin-testing/helper/acctest"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/provider"
)

func PreCheck(t *testing.T) {
	t.Helper()
	for _, v := range []string{"DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY"} {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for acceptance tests (run acceptance/up.sh, then eval \"$(acceptance/bootstrap.sh)\")", v)
		}
	}
}

func ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"dokploy": providerserver.NewProtocol6WithError(provider.New("acctest")()),
	}
}

// ClientFromEnv builds a direct API client for existence/destroy checks.
func ClientFromEnv() (*client.Client, error) {
	endpoint, apiKey := os.Getenv("DOKPLOY_ENDPOINT"), os.Getenv("DOKPLOY_API_KEY")
	if endpoint == "" || apiKey == "" {
		return nil, fmt.Errorf("DOKPLOY_ENDPOINT and DOKPLOY_API_KEY must be set")
	}
	return client.New(endpoint, apiKey, false, "acctest")
}

// RandomName returns prefix-XXXXXXXX for collision-free test resources.
func RandomName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, sdkacctest.RandStringFromCharSet(8, sdkacctest.CharSetAlphaNum))
}

// CreateNetwork creates a rig network through the real client and returns
// its id. It replaces the raw x-api-key HTTP helpers that four test
// packages copied while the client had no network methods (the wave-6b
// cleanup those copies were annotated for).
//
// Driver and EnableIPv4 are set explicitly to the server's own bare-create
// defaults ("bridge" and true - internal/client/doc.go, "wave 6b network
// probes"). client.CreateNetworkRequest.Driver carries no `omitempty`, so a
// zero-value request marshals an explicit "driver":"" rather than omitting
// the key - and the live rig 400s that ("Invalid option: expected one of
// \"bridge\"|\"overlay\""), confirmed against the acceptance rig on
// 2026-08-20. Only omitting the key lets the server apply its own default,
// which is what the raw HTTP helper this replaces did by sending a bare
// {"name": name}. Setting the two fields here keeps CreateNetwork's
// behavior identical to that helper's.
func CreateNetwork(t *testing.T, name string) string {
	t.Helper()
	c, err := ClientFromEnv()
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	n, err := c.CreateNetwork(context.Background(), client.CreateNetworkRequest{
		Name:       name,
		Driver:     "bridge",
		EnableIPv4: true,
	})
	if err != nil {
		t.Fatalf("creating network %q: %v", name, err)
	}
	return n.NetworkID
}

// DeleteNetwork removes a network created with CreateNetwork.
func DeleteNetwork(t *testing.T, id string) {
	t.Helper()
	c, err := ClientFromEnv()
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	if err := c.DeleteNetwork(context.Background(), id); err != nil {
		t.Fatalf("deleting network %s: %v", id, err)
	}
}
