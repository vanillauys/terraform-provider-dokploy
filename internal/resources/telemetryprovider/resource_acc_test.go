// Package telemetryprovider_test holds the acceptance tests (external
// package: acctest imports provider, which imports telemetryprovider).
package telemetryprovider_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

const res = "dokploy_telemetry_provider.test"

func checkDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_telemetry_provider" {
			continue
		}
		if _, err := c.GetTelemetryProvider(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("telemetry provider %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkServer(assert func(*client.TelemetryProvider) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("%s not in state", res)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		p, err := c.GetTelemetryProvider(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading telemetry provider %s: %w", rs.Primary.ID, err)
		}
		return assert(p)
	}
}

func emptyPlan() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
}

func config(name, providerType, signals, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_telemetry_provider" "test" {
  name          = %q
  provider_type = %q
  signals       = %s
  %s
}
`, name, providerType, signals, extra)
}

func TestAccTelemetryProvider_lifecycle(t *testing.T) {
	name := acctest.RandomName("tp")
	loki := `endpoint = "http://loki.invalid:3100/loki/api/v1/push"`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(name, "loki", `["logs"]`, loki+`
  api_key      = "acc-token"
  enabled      = false
  extra_config = { tenantId = "t1", username = "u1" }`),
				ConfigPlanChecks: emptyPlan(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(res, "id"),
					resource.TestCheckResourceAttrSet(res, "organization_id"),
					resource.TestCheckResourceAttrSet(res, "created_at"),
					resource.TestCheckResourceAttr(res, "extra_config.tenantId", "t1"),
					checkServer(func(p *client.TelemetryProvider) error {
						want := map[string]string{"tenantId": "t1", "username": "u1"}
						if p.ProviderType != "loki" || p.Enabled || !reflect.DeepEqual(p.ExtraConfigValues(), want) {
							return fmt.Errorf("server = %+v, extraConfig %v", p, p.ExtraConfigValues())
						}
						return nil
					}),
				),
			},
			{
				// The server merges extraConfig, so a dropped key must reach
				// it as null. enabled reverts to its true default.
				Config: config(name, "loki", `["logs"]`, loki+`
  api_key      = "acc-token"
  extra_config = { username = "u2" }`),
				ConfigPlanChecks: emptyPlan(),
				Check: checkServer(func(p *client.TelemetryProvider) error {
					if !p.Enabled || !reflect.DeepEqual(p.ExtraConfigValues(), map[string]string{"username": "u2"}) {
						return fmt.Errorf("server enabled %v, extraConfig %v", p.Enabled, p.ExtraConfigValues())
					}
					return nil
				}),
			},
			{
				// §5.6: drop every optional attribute that loki allows.
				Config:           config(name, "loki", `["logs"]`, loki),
				ConfigPlanChecks: emptyPlan(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(res, "extra_config"),
					resource.TestCheckNoResourceAttr(res, "api_key"),
					checkServer(func(p *client.TelemetryProvider) error {
						if len(p.ExtraConfigValues()) != 0 {
							return fmt.Errorf("server extraConfig = %v, want cleared", p.ExtraConfigValues())
						}
						return nil
					}),
				),
			},
			{
				// A type change in place: datadog needs an API key and
				// supports both signals, and the loki endpoint goes away.
				Config: config(name, "datadog", `["logs", "metrics"]`, `api_key = "acc-dd-key"
  extra_config = { site = "datadoghq.eu" }`),
				ConfigPlanChecks: emptyPlan(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "signals.#", "2"),
					checkServer(func(p *client.TelemetryProvider) error {
						if p.ProviderType != "datadog" || !reflect.DeepEqual(p.Signals, []string{"logs", "metrics"}) {
							return fmt.Errorf("server type %q, signals %v", p.ProviderType, p.Signals)
						}
						return nil
					}),
				),
			},
			{
				// Dokploy never returns endpoint, api_key or api_secret, so
				// an import cannot fill them.
				ResourceName:            res,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"api_key"},
			},
		},
	})
}

func TestAccTelemetryProvider_writeOnly(t *testing.T) {
	name := acctest.RandomName("tp-wo")
	cw := func(version int) string {
		return config(name, "aws_cloudwatch", `["logs"]`, fmt.Sprintf(`api_key = "AKIAACCEPTANCEONLY"
  api_secret_wo         = "acc-secret-%d"
  api_secret_wo_version = %d
  extra_config = { region = "eu-west-1", logGroup = "dokploy" }`, version, version))
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		TerraformVersionChecks:   acctest.WriteOnlyVersionChecks(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config:           cw(1),
				ConfigPlanChecks: emptyPlan(),
				Check:            resource.TestCheckNoResourceAttr(res, "api_secret"),
			},
			{
				Config: cw(2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(res, plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
