// Package registry_test holds the acceptance tests (external package;
// acctest imports provider, which imports this package).
package registry_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// fixture needs a reachable registry: registry.create runs `docker login`
// before it stores the record (client.CreateRegistryRequest).
func fixture(name, url string) string {
	return fmt.Sprintf(`
resource "dokploy_registry" "fixture" {
  name         = %q
  url          = %q
  username     = "acceptance"
  password     = "acceptance-only"
  image_prefix = "acc"
}
`, name, url)
}

// checkAgainstAPI asserts the data source's state against a direct API
// read, and that the password never reaches its state.
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
		got, err := c.GetRegistry(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading registry %s from the API: %w", rs.Primary.ID, err)
		}
		for _, f := range []struct{ attr, want string }{
			{"name", got.RegistryName},
			{"url", got.RegistryURL},
			{"username", got.Username},
			{"image_prefix", got.ImagePrefix},
			{"registry_type", got.RegistryType},
			{"organization_id", got.OrganizationID},
			{"created_at", got.CreatedAt},
		} {
			if have := rs.Primary.Attributes[f.attr]; have != f.want {
				return fmt.Errorf("%s.%s = %q, API says %q", addr, f.attr, have, f.want)
			}
		}
		if v, found := rs.Primary.Attributes["password"]; found {
			return fmt.Errorf("%s has password in state (%d bytes); the data source must not model credentials", addr, len(v))
		}
		return nil
	}
}

func TestAccRegistryDataSource_byNameAndByID(t *testing.T) {
	url := acctest.StartRigRegistry(t)
	name := acctest.RandomName("reg-ds")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture(name, url)},
			{
				Config: fixture(name, url) + `
data "dokploy_registry" "by_name" {
  name = dokploy_registry.fixture.name
}

data "dokploy_registry" "by_id" {
  id = dokploy_registry.fixture.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgainstAPI("data.dokploy_registry.by_name"),
					checkAgainstAPI("data.dokploy_registry.by_id"),
					resource.TestCheckResourceAttrPair("data.dokploy_registry.by_name", "id", "dokploy_registry.fixture", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_registry.by_id", "id", "dokploy_registry.fixture", "id"),
					resource.TestCheckResourceAttr("data.dokploy_registry.by_id", "image_prefix", "acc"),
					resource.TestCheckResourceAttr("data.dokploy_registry.by_id", "registry_type", "cloud"),
				),
			},
			{
				Config:      fixture(name, url) + `data "dokploy_registry" "missing" { name = "no-such-registry-xyzzy" }`,
				ExpectError: regexp.MustCompile(`no registry named "no-such-registry-xyzzy"`),
			},
		},
	})
}

// Exactly one of id or name is required. Setting neither must be a
// configuration error rather than a list-everything-and-guess read.
func TestAccRegistryDataSource_requiresIDOrName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_registry" "neither" {}`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured: \[id,name\]`),
			},
		},
	})
}
