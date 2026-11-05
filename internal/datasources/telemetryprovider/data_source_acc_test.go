// Package telemetryprovider_test holds the acceptance tests (external
// package: acctest imports provider, which imports telemetryprovider).
package telemetryprovider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func TestAccTelemetryProviderDataSource_byNameAndID(t *testing.T) {
	name := acctest.RandomName("tp-ds")
	cfg := fmt.Sprintf(`
resource "dokploy_telemetry_provider" "test" {
  name          = %q
  provider_type = "loki"
  signals       = ["logs"]
  endpoint      = "http://loki.invalid:3100/loki/api/v1/push"
  extra_config  = { tenantId = "t1" }
}

data "dokploy_telemetry_provider" "by_name" {
  name = dokploy_telemetry_provider.test.name
}

data "dokploy_telemetry_provider" "by_id" {
  id = dokploy_telemetry_provider.test.id
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrPair("data.dokploy_telemetry_provider.by_name", "id", "dokploy_telemetry_provider.test", "id"),
				resource.TestCheckResourceAttrPair("data.dokploy_telemetry_provider.by_id", "name", "dokploy_telemetry_provider.test", "name"),
				resource.TestCheckResourceAttr("data.dokploy_telemetry_provider.by_name", "provider_type", "loki"),
				resource.TestCheckResourceAttr("data.dokploy_telemetry_provider.by_name", "signals.0", "logs"),
				resource.TestCheckResourceAttr("data.dokploy_telemetry_provider.by_name", "extra_config.tenantId", "t1"),
				resource.TestCheckResourceAttr("data.dokploy_telemetry_provider.by_id", "enabled", "true"),
			),
		}},
	})
}
