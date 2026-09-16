// Package vaultprovider_test holds the acceptance tests (external package;
// acctest imports provider, which imports this package).
package vaultprovider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// fixture uses fake Phase.dev credentials: vaultProvider.create never
// contacts the vault (client.CreateVaultProviderRequest), so no real vault
// is touched. The assignment covers one project, every environment.
func fixture(name string) string {
	return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %[1]q
}

resource "dokploy_vault_provider" "fixture" {
  name = %[1]q

  phase = {
    token  = "pss_service:v1:acceptance-only-fake"
    app_id = "app_acceptance_only"
    env    = "production"
  }

  assignments = [{ project_id = dokploy_project.test.id }]
}
`, name)
}

// checkAgainstAPI asserts the data source's state against a direct API
// read, and that no config block reaches its state.
func checkAgainstAPI(addr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("%s not found in state", addr)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		got, err := c.GetVaultProvider(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading vault provider %s from the API: %w", rs.Primary.ID, err)
		}
		for _, f := range []struct{ attr, want string }{
			{"name", got.Name},
			{"provider_type", got.ProviderType},
			{"created_at", got.CreatedAt},
			{"assignments.#", fmt.Sprint(len(got.Assignments))},
			{"assignments.0.project_id", got.Assignments[0].ProjectID},
			{"assignments.0.environment_ids.#", fmt.Sprint(len(got.Assignments[0].EnvironmentIDs))},
		} {
			if have := rs.Primary.Attributes[f.attr]; have != f.want {
				return fmt.Errorf("%s.%s = %q, API says %q", addr, f.attr, have, f.want)
			}
		}
		for k := range rs.Primary.Attributes {
			if k == "phase" || k == "hashicorp" || len(k) > 6 && k[:6] == "phase." {
				return fmt.Errorf("%s has %s in state; the data source must not model the config block", addr, k)
			}
		}
		return nil
	}
}

func TestAccVaultProviderDataSource_byNameAndByID(t *testing.T) {
	name := acctest.RandomName("vault-ds")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture(name)},
			{
				Config: fixture(name) + `
data "dokploy_vault_provider" "by_name" {
  name = dokploy_vault_provider.fixture.name
}

data "dokploy_vault_provider" "by_id" {
  id = dokploy_vault_provider.fixture.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgainstAPI("data.dokploy_vault_provider.by_name"),
					checkAgainstAPI("data.dokploy_vault_provider.by_id"),
					resource.TestCheckResourceAttrPair("data.dokploy_vault_provider.by_name", "id", "dokploy_vault_provider.fixture", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_vault_provider.by_id", "id", "dokploy_vault_provider.fixture", "id"),
					resource.TestCheckResourceAttr("data.dokploy_vault_provider.by_id", "provider_type", "phase"),
					resource.TestCheckResourceAttrPair("data.dokploy_vault_provider.by_id", "assignments.0.project_id", "dokploy_project.test", "id"),
				),
			},
			{
				Config:      fixture(name) + `data "dokploy_vault_provider" "missing" { name = "no-such-vault-xyzzy" }`,
				ExpectError: regexp.MustCompile(`no vault provider named "no-such-vault-xyzzy"`),
			},
		},
	})
}

// Exactly one of id or name is required. Setting neither must be a
// configuration error rather than a list-everything-and-guess read.
func TestAccVaultProviderDataSource_requiresIDOrName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_vault_provider" "neither" {}`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured: \[id,name\]`),
			},
		},
	})
}
