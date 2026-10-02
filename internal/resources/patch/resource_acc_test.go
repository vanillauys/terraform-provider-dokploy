// Package patch_test holds the acceptance tests (external package: acctest
// imports provider, which imports patch).
package patch_test

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

const res = "dokploy_patch.test"

func checkDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_patch" {
			continue
		}
		if _, err := c.GetPatch(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("patch %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkServer(assert func(*client.Patch) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("%s not in state", res)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		p, err := c.GetPatch(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		return assert(p)
	}
}

func emptyPlan() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
}

// config renders a project with an application and a compose service, and
// one patch. No service deploys.
func config(name, patch string) string {
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

resource "dokploy_patch" "test" {
%[2]s
}

data "dokploy_patch" "by_path" {
  application_id = dokploy_patch.test.application_id
  compose_id     = dokploy_patch.test.compose_id
  file_path      = dokploy_patch.test.file_path
}
`, name, patch)
}

func TestAccPatch_lifecycle(t *testing.T) {
	name := acctest.RandomName("patch")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				// patch.create stores enabled = true, so a disabled patch
				// needs the follow-up update.
				Config: config(name, `  application_id = dokploy_application.test.id
  file_path      = "config/app.yaml"
  content        = "key: value\n"
  type           = "create"
  enabled        = false`),
				ConfigPlanChecks: emptyPlan(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "content", "key: value\n"),
					resource.TestCheckResourceAttrPair("data.dokploy_patch.by_path", "id", res, "id"),
					resource.TestCheckResourceAttr("data.dokploy_patch.by_path", "type", "create"),
					checkServer(func(p *client.Patch) error {
						if p.Enabled || p.Type != "create" || p.FilePath != "config/app.yaml" ||
							p.Content != "key: value\n" {
							return fmt.Errorf("server = %+v", p)
						}
						return nil
					}),
				),
			},
			{
				// An import reads the stored content. Content that ends with
				// a newline, as from file() or a heredoc, imports cleanly.
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// An in-place update: patch.update appends "\n" to the
				// content, and the plan stays empty.
				Config: config(name, `  application_id = dokploy_application.test.id
  file_path      = "config/other.yaml"
  content        = "key: other"`),
				ConfigPlanChecks: emptyPlan(),
				Check: checkServer(func(p *client.Patch) error {
					if !p.Enabled || p.Type != "update" || p.FilePath != "config/other.yaml" || p.Content != "key: other\n" {
						return fmt.Errorf("server = %+v", p)
					}
					return nil
				}),
			},
			{
				// §5.6: content, type and enabled revert to their defaults.
				Config: config(name, `  application_id = dokploy_application.test.id
  file_path      = "config/other.yaml"`),
				ConfigPlanChecks: emptyPlan(),
				Check: checkServer(func(p *client.Patch) error {
					if !p.Enabled || p.Type != "update" || p.Content != "" {
						return fmt.Errorf("server = %+v, want the defaults", p)
					}
					return nil
				}),
			},
			{
				// A new parent replaces the patch.
				Config: config(name, `  compose_id = dokploy_compose.test.id
  file_path  = "docker-compose.override.yml"
  type       = "delete"`),
				ConfigPlanChecks: emptyPlan(),
				Check: checkServer(func(p *client.Patch) error {
					if p.ComposeID == nil || p.ApplicationID != nil || p.Type != "delete" {
						return fmt.Errorf("server = %+v", p)
					}
					return nil
				}),
			},
		},
	})
}
