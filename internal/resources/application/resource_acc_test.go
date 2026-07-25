// Package application_test (an external test package, deliberately distinct
// from package application) holds the acceptance tests. It must live
// outside package application: acctest imports provider, and provider
// imports application to register dokploy_application — so an internal
// test file (package application) importing acctest here would form an
// import cycle (application -> acctest -> provider -> application), which
// the Go toolchain rejects with "import cycle not allowed in test".
// Keeping this file in the external application_test package sidesteps
// that: it depends on application (indirectly, via provider) without
// itself being part of application. Mirrors
// internal/resources/postgres/resource_acc_test.go.
package application_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func checkApplicationDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_application" {
			continue
		}
		if _, err := c.GetApplication(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("application %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// fetchApplication runs fn against the live API object behind
// dokploy_application.test, so an assertion can distinguish "Terraform
// state says null" from "the server actually stores null".
func fetchApplication(fn func(*client.Application) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources["dokploy_application.test"]
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		app, err := c.GetApplication(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		return fn(app)
	}
}

// Docker source: fastest to deploy, exercises the full engine.
func TestAccApplication_dockerLifecycle(t *testing.T) {
	name := acctest.RandomName("app")
	// optionals lets a step drop previously-set optional attributes
	// entirely, which is what spec §5.6 (clearable back to null) requires.
	//
	// deployment_timeout is deliberately left out so it takes its schema
	// default: the final import step needs it, because import cannot see
	// config and so can only seed the defaults (see tfutil.ImportDeployDefaults).
	config := func(image, optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id

  docker = {
    image = %q
  }
%s
}`, name+"-proj", name, image, optionals)
	}
	withOptionals := `
  description = "managed by the acceptance suite"
  env         = "WHOAMI_NAME=acceptance"
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("traefik/whoami:v1.10", withOptionals),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_application.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_application.test", "app_name"),
					resource.TestCheckResourceAttr("dokploy_application.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_application.test", "description", "managed by the acceptance suite"),
					fetchApplication(func(app *client.Application) error {
						if app.SourceType != "docker" {
							return fmt.Errorf("sourceType = %q, want docker", app.SourceType)
						}
						return nil
					}),
				),
			},
			{
				// Image change is a deploy trigger.
				Config: config("traefik/whoami:v1.10.2", withOptionals),
				Check:  resource.TestCheckResourceAttr("dokploy_application.test", "status", "done"),
			},
			{
				// Spec §5.6: optional attributes must be clearable back to
				// null, not merely settable. Dropping description and env
				// from config must reach the server, not just Terraform
				// state — with `omitempty` on UpdateApplicationRequest.
				// Description the server silently kept the old text, the
				// next Read flattened it back in, and every subsequent plan
				// showed the same diff forever. ExpectEmptyPlan below is
				// what catches exactly that.
				Config: config("traefik/whoami:v1.10.2", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_application.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_application.test", "env"),
					fetchApplication(func(app *client.Application) error {
						if app.Description != nil && *app.Description != "" {
							return fmt.Errorf("server still stores description %q; it was removed from config", *app.Description)
						}
						if app.Env != nil && *app.Env != "" {
							return fmt.Errorf("server still stores env %q; it was removed from config", *app.Env)
						}
						return nil
					}),
				),
			},
			{
				// No ImportStateVerifyIgnore. deploy_on_change and
				// deployment_timeout are provider-only, so ImportState seeds
				// them with their schema defaults; ignoring them here used to
				// paper over an import that could never produce a clean plan.
				ResourceName:      "dokploy_application.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// A deploy moves `status` (idle -> running -> done), so the plan must leave
// it unknown whenever an apply will actually run. This sequence pins that
// down: step 1 skips the deploy and settles at "idle", step 2 changes the
// image and turns deploying on, so the apply necessarily writes "done".
// While `status` carried stringplanmodifier.UseStateForUnknown() the step-2
// plan pinned the known "idle" and Terraform core rejected the apply with
// `Provider produced inconsistent result after apply: .status: was
// cty.StringVal("idle"), but now cty.StringVal("done")` (reproduced against
// the live rig at commit a7bc6d2, 2026-07-25). The final empty-plan check
// guards the other side of the trade: dropping that modifier must not
// reintroduce the perpetual non-empty plan it was added to cure.
func TestAccApplication_deployOnChangeFlip(t *testing.T) {
	name := acctest.RandomName("app-flip")
	config := func(image string, deploy bool) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id

  docker = {
    image = %q
  }

  deploy_on_change   = %t
  deployment_timeout = "10m"
}`, name+"-proj", name, image, deploy)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("traefik/whoami:v1.10", false),
				Check:  resource.TestCheckResourceAttr("dokploy_application.test", "status", "idle"),
			},
			{
				Config: config("traefik/whoami:v1.10.2", true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("dokploy_application.test", "status", "done"),
			},
		},
	})
}

// Git source: configuration only (deploy_on_change = false keeps the test
// fast and independent of build tooling inside the rig).
func TestAccApplication_gitSource(t *testing.T) {
	name := acctest.RandomName("app-git")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id

  git = {
    url    = "https://github.com/dokploy/dokploy.git"
    branch = "canary"
  }

  build = {
    type = "dockerfile"
    dockerfile = "Dockerfile"
  }

  deploy_on_change = false
}`, name+"-proj", name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "git.url", "https://github.com/dokploy/dokploy.git"),
					resource.TestCheckResourceAttr("dokploy_application.test", "build.type", "dockerfile"),
				),
			},
		},
	})
}

// GitHub source needs a GitHub App installed in the instance — a manual,
// browser-bound prerequisite (spec §12). Gated behind an env var.
func TestAccApplication_githubSource(t *testing.T) {
	githubID := os.Getenv("DOKPLOY_ACC_GITHUB_ID")
	if githubID == "" {
		t.Skip("DOKPLOY_ACC_GITHUB_ID not set; skipping github-source test (requires a GitHub App in the instance)")
	}
	name := acctest.RandomName("app-gh")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id

  github = {
    owner      = "vanillauys"
    repository = "vanillauys-app"
    branch     = "master"
    github_id  = %q
  }

  deploy_on_change = false
}`, name+"-proj", name, githubID),
				Check: resource.TestCheckResourceAttr("dokploy_application.test", "github.owner", "vanillauys"),
			},
		},
	})
}
