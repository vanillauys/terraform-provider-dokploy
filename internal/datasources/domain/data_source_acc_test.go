// Package domain_test holds the acceptance tests (external package; acctest
// imports provider, which imports this package).
package domain_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// fixture attaches the SAME host to an application and to a compose
// service, the shape that makes a plain host lookup ambiguous. Neither
// service deploys.
func fixture(name, host string) string {
	return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %[1]q
}

resource "dokploy_application" "test" {
  name             = %[1]q
  environment_id   = dokploy_project.test.environments[0].id
  docker           = { image = "traefik/whoami:v1.10" }
  deploy_on_change = false
}

resource "dokploy_compose" "test" {
  name             = %[1]q
  environment_id   = dokploy_project.test.environments[0].id
  deploy_on_change = false
  raw = {
    compose_file = "services:\n  web:\n    image: nginx:alpine\n"
  }
}

resource "dokploy_domain" "app" {
  host           = %[2]q
  application_id = dokploy_application.test.id
  port           = 8080
  path           = "/api"
}

resource "dokploy_domain" "compose" {
  host         = %[2]q
  compose_id   = dokploy_compose.test.id
  service_name = "web"
  https        = true
}
`, name, host)
}

// checkAgainstAPI asserts the data source's state against a direct API
// read, not against the resource's state: two pieces of provider-produced
// state can agree and both be wrong.
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
		got, err := c.GetDomain(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading domain %s from the API: %w", rs.Primary.ID, err)
		}
		for _, f := range []struct{ attr, want string }{
			{"host", got.Host},
			{"path", got.Path},
			{"internal_path", got.InternalPath},
			{"port", fmt.Sprint(got.Port)},
			{"https", fmt.Sprint(got.HTTPS)},
			{"certificate_type", got.CertificateType},
			{"domain_type", got.DomainType},
			{"enabled", fmt.Sprint(got.Enabled)},
			{"created_at", got.CreatedAt},
		} {
			if have := rs.Primary.Attributes[f.attr]; have != f.want {
				return fmt.Errorf("%s.%s = %q, API says %q", addr, f.attr, have, f.want)
			}
		}
		return nil
	}
}

func TestAccDomainDataSource_byIDAndByHostWithFilter(t *testing.T) {
	name := acctest.RandomName("dom-ds")
	host := name + ".example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture(name, host)},
			{
				Config: fixture(name, host) + `
data "dokploy_domain" "by_id" {
  id = dokploy_domain.app.id
}

data "dokploy_domain" "on_app" {
  host           = dokploy_domain.app.host
  application_id = dokploy_application.test.id
}

data "dokploy_domain" "on_compose" {
  host       = dokploy_domain.compose.host
  compose_id = dokploy_compose.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgainstAPI("data.dokploy_domain.by_id"),
					checkAgainstAPI("data.dokploy_domain.on_app"),
					checkAgainstAPI("data.dokploy_domain.on_compose"),
					resource.TestCheckResourceAttrPair("data.dokploy_domain.by_id", "id", "dokploy_domain.app", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_domain.on_app", "id", "dokploy_domain.app", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_domain.on_compose", "id", "dokploy_domain.compose", "id"),
					resource.TestCheckResourceAttr("data.dokploy_domain.on_app", "port", "8080"),
					resource.TestCheckResourceAttr("data.dokploy_domain.on_app", "path", "/api"),
					resource.TestCheckResourceAttr("data.dokploy_domain.on_app", "domain_type", "application"),
					resource.TestCheckNoResourceAttr("data.dokploy_domain.on_app", "compose_id"),
					resource.TestCheckResourceAttr("data.dokploy_domain.on_compose", "https", "true"),
					resource.TestCheckResourceAttr("data.dokploy_domain.on_compose", "service_name", "web"),
					resource.TestCheckResourceAttr("data.dokploy_domain.on_compose", "domain_type", "compose"),
					resource.TestCheckNoResourceAttr("data.dokploy_domain.on_compose", "application_id"),
					resource.TestCheckResourceAttr("data.dokploy_domain.on_compose", "middlewares.#", "0"),
				),
			},
		},
	})
}

// Without a filter the lookup walks the whole organization, so the host
// that the fixture attaches twice is ambiguous, and the error must say so
// rather than pick one. A host that no service carries must fail with the
// string searched for.
func TestAccDomainDataSource_hostAloneWalksTheOrganization(t *testing.T) {
	name := acctest.RandomName("dom-walk")
	host := name + ".example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture(name, host)},
			{
				Config:      fixture(name, host) + fmt.Sprintf(`data "dokploy_domain" "dup" { host = %q }`, host),
				ExpectError: regexp.MustCompile(fmt.Sprintf(`more than one domain has the host %q`, host)),
			},
			{
				Config: fixture(name, host) + fmt.Sprintf(`
resource "dokploy_domain" "unique" {
  host           = %q
  application_id = dokploy_application.test.id
}

data "dokploy_domain" "walk" {
  host       = dokploy_domain.unique.host
  depends_on = [dokploy_domain.unique]
}
`, "unique-"+host),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgainstAPI("data.dokploy_domain.walk"),
					resource.TestCheckResourceAttrPair("data.dokploy_domain.walk", "id", "dokploy_domain.unique", "id"),
				),
			},
			{
				Config:      fixture(name, host) + `data "dokploy_domain" "missing" { host = "no-such-host-xyzzy.example.com" }`,
				ExpectError: regexp.MustCompile(`no domain has the host "no-such-host-xyzzy.example.com"`),
			},
		},
	})
}

func TestAccDomainDataSource_requiresIDOrHost(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_domain" "neither" {}`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured: \[id,host\]`),
			},
			{
				Config: `
data "dokploy_domain" "both" {
  id             = "x"
  application_id = "y"
}`,
				ExpectError: regexp.MustCompile(`cannot be specified when`),
			},
		},
	})
}
