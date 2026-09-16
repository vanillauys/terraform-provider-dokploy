// Package compose_test holds the acceptance tests (external package; acctest
// imports provider, which imports this package).
package compose_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func fixture(name string) string {
	return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %[1]q
}

resource "dokploy_compose" "test" {
  name             = %[1]q
  environment_id   = dokploy_project.test.environments[0].id
  description      = "acceptance fixture"
  compose_type     = "stack"
  env              = "A=1"
  deploy_on_change = false
  raw = {
    compose_file = "services:\n  web:\n    image: nginx:alpine\n"
  }
}
`, name)
}

// checkAgainstAPI asserts the data source's state against a direct API
// read, not against the resource's state.
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
		got, err := c.GetCompose(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading compose %s from the API: %w", rs.Primary.ID, err)
		}
		for _, f := range []struct{ attr, want string }{
			{"name", got.Name},
			{"app_name", got.AppName},
			{"environment_id", got.EnvironmentID},
			{"compose_type", got.ComposeType},
			{"source_type", got.SourceType},
			{"compose_path", got.ComposePath},
			{"status", got.ComposeStatus},
			{"created_at", got.CreatedAt},
		} {
			if have := rs.Primary.Attributes[f.attr]; have != f.want {
				return fmt.Errorf("%s.%s = %q, API says %q", addr, f.attr, have, f.want)
			}
		}
		return nil
	}
}

func TestAccComposeDataSource_byIDAndByName(t *testing.T) {
	name := acctest.RandomName("co-ds")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixture(name)},
			{
				Config: fixture(name) + `
data "dokploy_compose" "by_id" {
  id = dokploy_compose.test.id
}

data "dokploy_compose" "by_name" {
  name           = dokploy_compose.test.name
  environment_id = dokploy_project.test.environments[0].id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgainstAPI("data.dokploy_compose.by_id"),
					checkAgainstAPI("data.dokploy_compose.by_name"),
					resource.TestCheckResourceAttrPair("data.dokploy_compose.by_name", "id", "dokploy_compose.test", "id"),
					resource.TestCheckResourceAttr("data.dokploy_compose.by_id", "compose_type", "stack"),
					resource.TestCheckResourceAttr("data.dokploy_compose.by_id", "source_type", "raw"),
					resource.TestCheckResourceAttr("data.dokploy_compose.by_id", "description", "acceptance fixture"),
					resource.TestCheckResourceAttr("data.dokploy_compose.by_id", "env", "A=1"),
					resource.TestCheckResourceAttr("data.dokploy_compose.by_id", "create_env_file", "true"),
					resource.TestCheckResourceAttr("data.dokploy_compose.by_id", "randomize", "false"),
					resource.TestCheckNoResourceAttr("data.dokploy_compose.by_id", "server_id"),
					resource.TestCheckNoResourceAttr("data.dokploy_compose.by_id", "command"),
				),
			},
			{
				Config: fixture(name) + `
data "dokploy_compose" "missing" {
  name           = "no-such-compose-xyzzy"
  environment_id = dokploy_project.test.environments[0].id
}`,
				ExpectError: regexp.MustCompile(`no compose service named "no-such-compose-xyzzy"`),
			},
		},
	})
}

func TestAccComposeDataSource_requiresIDOrName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_compose" "neither" {}`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured: \[id,name\]`),
			},
			{
				Config:      `data "dokploy_compose" "no_env" { name = "x" }`,
				ExpectError: regexp.MustCompile(`These attributes must be configured together: \[environment_id,name\]`),
			},
		},
	})
}
