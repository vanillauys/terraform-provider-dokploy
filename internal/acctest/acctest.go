// Package acctest holds shared helpers for acceptance tests.
// Tests require a disposable Dokploy instance:
//
//	./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc
package acctest

import (
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
