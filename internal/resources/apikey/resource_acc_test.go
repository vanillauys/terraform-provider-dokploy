// Package apikey_test holds the acceptance tests (external package; acctest
// imports provider, which imports apikey).
package apikey_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func checkDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	keys, err := c.ListAPIKeys(context.Background())
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_api_key" {
			continue
		}
		for _, k := range keys {
			if k.ID == rs.Primary.ID {
				return fmt.Errorf("api key %s still exists", rs.Primary.ID)
			}
		}
	}
	return nil
}

// checkKeyWorks proves the generated key authenticates: a client built
// from it must read the active organization.
func checkKeyWorks(s *terraform.State) error {
	rs := s.RootModule().Resources["dokploy_api_key.test"]
	key := rs.Primary.Attributes["key"]
	if !strings.HasPrefix(key, "acc") {
		return fmt.Errorf("key %q does not start with the prefix", key[:3])
	}
	c, err := acctest.ClientWithKey(key)
	if err != nil {
		return err
	}
	if _, err := c.GetActiveOrganization(context.Background()); err != nil {
		return fmt.Errorf("the new key cannot authenticate: %w", err)
	}
	return nil
}

func config(name, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_api_key" "test" {
  name   = %q
  prefix = "acc"
%s
}
`, name, extra)
}

func TestAccAPIKey_lifecycle(t *testing.T) {
	name := acctest.RandomName("key")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(name, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_api_key.test", "rate_limit_enabled", "false"),
					resource.TestCheckNoResourceAttr("dokploy_api_key.test", "expires_at"),
					resource.TestCheckResourceAttrSet("dokploy_api_key.test", "created_at"),
					checkKeyWorks,
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// An expiry and a rename: both replace the key.
				Config: config(name+"-renamed", "  expires_in = 86400"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_api_key.test", "expires_at"),
					checkKeyWorks,
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_api_key.test", plancheck.ResourceActionReplace)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
