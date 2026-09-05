// Package envvars_test holds the acceptance tests (external package;
// acctest imports provider, which imports envvars).
package envvars_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// fixtures: one project with an application (docker source, build secrets
// set, env ignored), a raw compose, and a second environment. Nothing
// deploys.
func fixtures(name string) string {
	return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %[1]q
}

resource "dokploy_application" "app" {
  name             = %[1]q
  environment_id   = dokploy_project.test.production_environment_id
  deploy_on_change = false
  build_secrets    = "TOKEN=keep-me"
  docker = {
    image = "traefik/whoami:v1.10"
  }
  lifecycle {
    ignore_changes = [env]
  }
}

resource "dokploy_compose" "stack" {
  name             = %[1]q
  environment_id   = dokploy_project.test.production_environment_id
  deploy_on_change = false
  raw = {
    compose_file = "services:\n  web:\n    image: nginx:alpine\n"
  }
  lifecycle {
    ignore_changes = [env]
  }
}

resource "dokploy_environment" "staging" {
  project_id  = dokploy_project.test.id
  name        = "staging"
  description = "fixture"
  lifecycle {
    ignore_changes = [env]
  }
}
`, name)
}

func vars(target, body string) string {
	return fmt.Sprintf(`
resource "dokploy_environment_variables" "test" {
  %s
  variables = {
%s
  }
}
`, target, body)
}

type targetCase struct {
	kind, fixture, attr, secrets string
}

// serverEnv reads the env text of the fixture target straight from the API
// and, for the application, proves the build secrets survived the write.
func serverEnv(tc targetCase, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[tc.fixture]
		if !ok {
			return fmt.Errorf("%s not found in state", tc.fixture)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		ctx := context.Background()
		var got string
		switch tc.kind {
		case "application":
			app, err := c.GetApplication(ctx, rs.Primary.ID)
			if err != nil {
				return err
			}
			if app.Env != nil {
				got = *app.Env
			}
			if app.BuildSecrets == nil || *app.BuildSecrets != tc.secrets {
				return fmt.Errorf("build secrets = %v, want %q kept across the env write", app.BuildSecrets, tc.secrets)
			}
		case "compose":
			comp, err := c.GetCompose(ctx, rs.Primary.ID)
			if err != nil {
				return err
			}
			if comp.Env != nil {
				got = *comp.Env
			}
		default:
			env, err := c.GetEnvironment(ctx, rs.Primary.ID)
			if err != nil {
				return err
			}
			got = env.Env
		}
		if got != want {
			return fmt.Errorf("server env = %q, want %q", got, want)
		}
		return nil
	}
}

// TestAccEnvironmentVariables_targets runs the same lifecycle against each
// of the three targets: write, rewrite (a key added and one removed), an
// import by the composite id, and a removal that clears the text.
func TestAccEnvironmentVariables_targets(t *testing.T) {
	name := acctest.RandomName("envvars")
	targets := []targetCase{
		{"application", "dokploy_application.app", "application_id = dokploy_application.app.id", "TOKEN=keep-me"},
		{"compose", "dokploy_compose.stack", "compose_id = dokploy_compose.stack.id", ""},
		{"environment", "dokploy_environment.staging", "environment_id = dokploy_environment.staging.id", ""},
	}
	for _, tc := range targets {
		t.Run(tc.kind, func(t *testing.T) {
			addr := "dokploy_environment_variables.test"
			base := fixtures(name + "-" + tc.kind)
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { acctest.PreCheck(t) },
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				Steps: []resource.TestStep{
					{
						Config: base + vars(tc.attr, "    B = \"2\"\n    A = \"one\""),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(addr, "variables.%", "2"),
							resource.TestCheckResourceAttr(addr, "variables.A", "one"),
							resource.TestCheckResourceAttrPair(addr, tc.kind+"_id", tc.fixture, "id"),
							serverEnv(tc, "A=one\nB=2"),
						),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						},
					},
					{
						Config: base + vars(tc.attr, "    A = \"two\"\n    C = \"x=y\""),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(addr, "variables.%", "2"),
							resource.TestCheckNoResourceAttr(addr, "variables.B"),
							serverEnv(tc, "A=two\nC=x=y"),
						),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
							PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						},
					},
					{
						ResourceName:      addr,
						ImportState:       true,
						ImportStateVerify: true,
						ImportStateIdFunc: func(s *terraform.State) (string, error) {
							return s.RootModule().Resources[addr].Primary.ID, nil
						},
					},
					{
						// The resource removed: its text clears, the target
						// stays.
						Config: base,
						Check:  serverEnv(tc, ""),
					},
				},
			})
		})
	}
}
