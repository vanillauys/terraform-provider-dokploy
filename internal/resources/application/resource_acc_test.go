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

// checkApplicationServer reads the application straight from the API and
// runs an assertion against it. Terraform state agreeing with itself proves
// nothing about a dialect A wipe: the wipe happens server-side, and a Read
// that never looked would happily report the value the plan expected.
func checkApplicationServer(resourceName string, assert func(*client.Application) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s not in state", resourceName)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		app, err := c.GetApplication(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading %s back from the server: %w", rs.Primary.ID, err)
		}
		return assert(app)
	}
}

// TestAccApplication_previouslyBlindFields is the acceptance-level guard for
// the wave-3 wipe. Each of these fields was transmitted on every apply
// without being modelled, so the sequence that matters is: set it, then
// apply again with it REMOVED from config, and check the server — not just
// state — for the documented revert behaviour.
//
// The null-vs-default distinction is load-bearing and differs per field:
//
//	watch_paths      Optional            -> reverts to null
//	build_secrets    Optional, Sensitive -> reverts to null
//	create_env_file  Optional+Computed   -> reverts to its default, true
//	enable_submodules Optional+Computed  -> reverts to its default, false
//	is_static_spa    Optional+Computed   -> reverts to its default, false
//	heroku_version   Optional            -> reverts to null
//	railpack_version Optional            -> reverts to null
func TestAccApplication_previouslyBlindFields(t *testing.T) {
	name := acctest.RandomName("app-blind")
	base := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}
`, name+"-proj")

	withFields := base + fmt.Sprintf(`
resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id

  git = {
    url    = "https://github.com/dokploy/dokploy.git"
    branch = "canary"
  }

  build = {
    type             = "dockerfile"
    dockerfile       = "Dockerfile"
    is_static_spa    = true
    heroku_version   = "22"
    railpack_version = "1"
  }

  env               = "A=1"
  build_secrets     = "SECRET=shh"
  create_env_file   = false
  enable_submodules = true
  watch_paths       = ["src/**", "package.json"]

  deploy_on_change = false
}`, name)

	withoutFields := base + fmt.Sprintf(`
resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id

  git = {
    url    = "https://github.com/dokploy/dokploy.git"
    branch = "canary"
  }

  build = {
    type       = "dockerfile"
    dockerfile = "Dockerfile"
  }

  env = "A=1"

  deploy_on_change = false
}`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: withFields,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "build_secrets", "SECRET=shh"),
					resource.TestCheckResourceAttr("dokploy_application.test", "create_env_file", "false"),
					resource.TestCheckResourceAttr("dokploy_application.test", "enable_submodules", "true"),
					resource.TestCheckResourceAttr("dokploy_application.test", "watch_paths.#", "2"),
					resource.TestCheckResourceAttr("dokploy_application.test", "build.is_static_spa", "true"),
					resource.TestCheckResourceAttr("dokploy_application.test", "build.heroku_version", "22"),
					checkApplicationServer("dokploy_application.test", func(a *client.Application) error {
						if a.BuildSecrets == nil || *a.BuildSecrets != "SECRET=shh" {
							return fmt.Errorf("server build_secrets = %v, want SECRET=shh", a.BuildSecrets)
						}
						if a.CreateEnvFile {
							return errors.New("server create_env_file = true, want false")
						}
						if !a.EnableSubmodules {
							return errors.New("server enable_submodules = false, want true")
						}
						if !a.IsStaticSpa {
							return errors.New("server is_static_spa = false, want true")
						}
						if len(a.WatchPaths) != 2 {
							return fmt.Errorf("server watch_paths = %v, want 2 entries", a.WatchPaths)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Every field removed from config. This is the step that would
				// have caught the original bug in reverse: with the fields
				// unmodelled, the server was reset here regardless of config.
				Config: withoutFields,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_application.test", "build_secrets"),
					resource.TestCheckNoResourceAttr("dokploy_application.test", "watch_paths"),
					resource.TestCheckResourceAttr("dokploy_application.test", "create_env_file", "true"),
					resource.TestCheckResourceAttr("dokploy_application.test", "enable_submodules", "false"),
					resource.TestCheckResourceAttr("dokploy_application.test", "build.is_static_spa", "false"),
					checkApplicationServer("dokploy_application.test", func(a *client.Application) error {
						if a.BuildSecrets != nil && *a.BuildSecrets != "" {
							return fmt.Errorf("server build_secrets = %v, want cleared", *a.BuildSecrets)
						}
						if !a.CreateEnvFile {
							return errors.New("server create_env_file = false, want its default true")
						}
						if a.EnableSubmodules {
							return errors.New("server enable_submodules = true, want its default false")
						}
						if a.IsStaticSpa {
							return errors.New("server is_static_spa = true, want its default false")
						}
						if len(a.WatchPaths) != 0 {
							return fmt.Errorf("server watch_paths = %v, want cleared", a.WatchPaths)
						}
						if a.HerokuVersion != nil && *a.HerokuVersion != "" {
							return fmt.Errorf("server heroku_version = %v, want cleared", *a.HerokuVersion)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
