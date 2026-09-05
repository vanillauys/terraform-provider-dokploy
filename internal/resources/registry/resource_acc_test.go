// Package registry_test holds the acceptance tests (external package; acctest
// imports provider, which imports registry).
package registry_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func checkRegistryDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_registry" {
			continue
		}
		if _, err := c.GetRegistry(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("registry %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// checkStoredPassword reads the password through registry.all, the one read
// path that returns it.
func checkStoredPassword(password string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_registry.test"]
		if !ok {
			return fmt.Errorf("dokploy_registry.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		all, err := c.ListRegistries(context.Background())
		if err != nil {
			return err
		}
		for _, r := range all {
			if r.RegistryID == rs.Primary.ID {
				if r.Password != password {
					return fmt.Errorf("stored password = %q, want %q", r.Password, password)
				}
				return nil
			}
		}
		return fmt.Errorf("registry %s is not in registry.all", rs.Primary.ID)
	}
}

// registryConfig points at the rig registry, which accepts any login: the
// Dokploy daemon runs `docker login` against it on every create and update.
func registryConfig(name, url, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_registry" "test" {
  name     = %q
  url      = %q
  username = "acceptance"
%s
}
`, name, url, extra)
}

func TestAccRegistry_lifecycle(t *testing.T) {
	url := acctest.StartRigRegistry(t)
	name := acctest.RandomName("reg")
	plain := "  password = \"acceptance-only\""
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkRegistryDestroy,
		Steps: []resource.TestStep{
			{
				Config: registryConfig(name, url, plain+"\n  image_prefix = \"acme\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_registry.test", "url", url),
					resource.TestCheckResourceAttr("dokploy_registry.test", "image_prefix", "acme"),
					resource.TestCheckResourceAttr("dokploy_registry.test", "registry_type", "cloud"),
					resource.TestCheckResourceAttr("dokploy_registry.test", "password", "acceptance-only"),
					resource.TestCheckResourceAttrSet("dokploy_registry.test", "created_at"),
					resource.TestCheckResourceAttrSet("dokploy_registry.test", "organization_id"),
					checkStoredPassword("acceptance-only"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// image_prefix dropped: it must clear on the server.
				Config: registryConfig(name+"-renamed", url, plain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_registry.test", "image_prefix"),
					checkStoredPassword("acceptance-only"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_registry.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// registry.one omits the password, so an import cannot
				// recover it; the resource description says so.
				ResourceName:            "dokploy_registry.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}

// TestAccRegistry_writeOnlyPassword pins the companion: the state never
// holds the password, a rename resends the stored one through registry.all,
// and a new version sends the new one.
func TestAccRegistry_writeOnlyPassword(t *testing.T) {
	url := acctest.StartRigRegistry(t)
	name := acctest.RandomName("reg-wo")
	wo := func(password string, version int) string {
		return fmt.Sprintf("  password_wo         = %q\n  password_wo_version = %d", password, version)
	}
	noSecretInState := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr("dokploy_registry.test", "password"),
		resource.TestCheckNoResourceAttr("dokploy_registry.test", "password_wo"),
	)
	update := resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_registry.test", plancheck.ResourceActionUpdate)},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkRegistryDestroy,
		Steps: []resource.TestStep{
			{
				Config: registryConfig(name, url, wo("wo-one", 1)),
				Check:  resource.ComposeAggregateTestCheckFunc(noSecretInState, checkStoredPassword("wo-one")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config:           registryConfig(name+"-renamed", url, wo("wo-one", 1)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkStoredPassword("wo-one")),
			},
			{
				Config:           registryConfig(name+"-renamed", url, wo("wo-two", 2)),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, checkStoredPassword("wo-two")),
			},
		},
	})
}
