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

// TestAccApplication_operationalAttributes covers the application.update
// (dialect B) attributes. Dialect B is the dangerous one: an absent key is
// silently "keep the old value", so a clearing bug shows up not as an error
// but as a plan that never converges. Every assertion therefore reads the
// server directly as well as state.
func TestAccApplication_operationalAttributes(t *testing.T) {
	name := acctest.RandomName("app-ops")
	base := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}
`, name+"-proj")

	withOps := base + fmt.Sprintf(`
resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
  docker         = { image = "traefik/whoami:v1.10" }

  auto_deploy        = false
  replicas           = 2
  cpu_limit          = "0.5"
  memory_limit       = "512m"
  cpu_reservation    = "0.25"
  memory_reservation = "256m"
  command            = "/bin/sh"
  args               = ["-c", "sleep 1"]

  deploy_on_change = false
}`, name)

	withoutOps := base + fmt.Sprintf(`
resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
  docker         = { image = "traefik/whoami:v1.10" }

  deploy_on_change = false
}`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: withOps,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "replicas", "2"),
					resource.TestCheckResourceAttr("dokploy_application.test", "auto_deploy", "false"),
					resource.TestCheckResourceAttr("dokploy_application.test", "memory_limit", "512m"),
					resource.TestCheckResourceAttr("dokploy_application.test", "args.#", "2"),
					checkApplicationServer("dokploy_application.test", func(a *client.Application) error {
						if a.Replicas != 2 {
							return fmt.Errorf("server replicas = %d, want 2", a.Replicas)
						}
						if a.AutoDeploy {
							return errors.New("server auto_deploy = true, want false")
						}
						if a.MemoryLimit == nil || *a.MemoryLimit != "512m" {
							return fmt.Errorf("server memory_limit = %v, want 512m", a.MemoryLimit)
						}
						if len(a.Args) != 2 {
							return fmt.Errorf("server args = %v, want 2 entries", a.Args)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// All removed. The plain-Optional attributes must revert to
				// null server-side; replicas and auto_deploy are
				// Optional+Computed and revert to their defaults instead.
				Config: withoutOps,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "replicas", "1"),
					resource.TestCheckResourceAttr("dokploy_application.test", "auto_deploy", "true"),
					resource.TestCheckNoResourceAttr("dokploy_application.test", "memory_limit"),
					resource.TestCheckNoResourceAttr("dokploy_application.test", "args"),
					checkApplicationServer("dokploy_application.test", func(a *client.Application) error {
						if a.Replicas != 1 {
							return fmt.Errorf("server replicas = %d, want its default 1", a.Replicas)
						}
						if !a.AutoDeploy {
							return errors.New("server auto_deploy = false, want its default true")
						}
						for name, v := range map[string]*string{
							"cpu_limit": a.CPULimit, "memory_limit": a.MemoryLimit,
							"cpu_reservation": a.CPUReservation, "memory_reservation": a.MemoryReservation,
							"command": a.Command,
						} {
							if v != nil && *v != "" {
								return fmt.Errorf("server %s = %q, want cleared", name, *v)
							}
						}
						if len(a.Args) != 0 {
							return fmt.Errorf("server args = %v, want cleared", a.Args)
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

// TestAccApplication_networkAttachment covers network_ids and
// detach_dokploy_network, the v0.30.0 network attachment fields on
// application.update (internal/client/doc.go, v0.30.0 section). Both fields
// must round-trip, and network_ids must clear back to null rather than an
// empty set: an explicit clear reads back as a literal JSON null, not `[]`,
// and the resource's flatten collapses both shapes to a null set
// (tfutil.StringSetOrNull).
func TestAccApplication_networkAttachment(t *testing.T) {
	// resource.Test below only checks TF_ACC once its Steps start, but this
	// test calls acctest.CreateNetwork BEFORE that - a client call of its
	// own - so it needs the same gate up front. Skipping (not failing)
	// matches every other acceptance test in this file and keeps
	// `make test` green with TF_ACC unset.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless env 'TF_ACC' set")
	}
	acctest.PreCheck(t)
	name := acctest.RandomName("app-net")
	netName := acctest.RandomName("net")
	networkID := acctest.CreateNetwork(t, netName)
	t.Cleanup(func() { acctest.DeleteNetwork(t, networkID) })

	base := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}
`, name+"-proj")

	withNetwork := base + fmt.Sprintf(`
resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
  docker         = { image = "traefik/whoami:v1.10" }

  network_ids            = [%q]
  detach_dokploy_network = true

  deploy_on_change = false
}`, name, networkID)

	withoutNetwork := base + fmt.Sprintf(`
resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
  docker         = { image = "traefik/whoami:v1.10" }

  deploy_on_change = false
}`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: withNetwork,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "network_ids.#", "1"),
					resource.TestCheckTypeSetElemAttr("dokploy_application.test", "network_ids.*", networkID),
					resource.TestCheckResourceAttr("dokploy_application.test", "detach_dokploy_network", "true"),
					checkApplicationServer("dokploy_application.test", func(a *client.Application) error {
						if len(a.NetworkIDs) != 1 || a.NetworkIDs[0] != networkID {
							return fmt.Errorf("server network_ids = %v, want [%s]", a.NetworkIDs, networkID)
						}
						if !a.DetachDokployNetwork {
							return errors.New("server detach_dokploy_network = false, want true")
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Removed from config. network_ids must converge to null (not
				// an empty set) and detach_dokploy_network to its default,
				// false - matching what the server actually stores after an
				// explicit clear (doc.go: null, never []).
				Config: withoutNetwork,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_application.test", "network_ids"),
					resource.TestCheckResourceAttr("dokploy_application.test", "detach_dokploy_network", "false"),
					checkApplicationServer("dokploy_application.test", func(a *client.Application) error {
						if len(a.NetworkIDs) != 0 {
							return fmt.Errorf("server network_ids = %v, want cleared", a.NetworkIDs)
						}
						if a.DetachDokployNetwork {
							return errors.New("server detach_dokploy_network = true, want its default false")
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

// TestAccApplication_forgeSources creates the three provider records and
// moves one application across the gitlab, bitbucket and gitea sources
// with deploy_on_change = false: the save* endpoints store the coordinates
// without contacting the forge, and every step asserts the server holds
// them, because these are dialect A calls.
func TestAccApplication_forgeSources(t *testing.T) {
	name := acctest.RandomName("app-forge")
	base := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %[1]q
}

resource "dokploy_gitlab_provider" "gl" {
  name           = %[1]q
  application_id = "oauth-app"
  secret         = "s"
}

resource "dokploy_bitbucket_provider" "bb" {
  name         = %[1]q
  username     = "bbuser"
  app_password = "p"
}

resource "dokploy_gitea_provider" "gt" {
  name          = %[1]q
  gitea_url     = "https://gitea.example.com"
  client_id     = "cid"
  client_secret = "s"
}
`, name)
	app := func(source string) string {
		return base + fmt.Sprintf(`
resource "dokploy_application" "test" {
  name             = %q
  environment_id   = dokploy_project.test.production_environment_id
  deploy_on_change = false
%s
}
`, name, source)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: app(`
  gitlab = {
    gitlab_id      = dokploy_gitlab_provider.gl.id
    owner          = "group"
    repository     = "app"
    branch         = "main"
    project_id     = 42
    path_namespace = "group/app"
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "gitlab.build_path", "/"),
					resource.TestCheckResourceAttr("dokploy_application.test", "gitlab.project_id", "42"),
					checkApplicationServer("dokploy_application.test", func(a *client.Application) error {
						if a.SourceType != "gitlab" || a.GitlabID == nil || a.GitlabProjectID == nil || *a.GitlabProjectID != 42 ||
							a.GitlabPathNamespace == nil || *a.GitlabPathNamespace != "group/app" || a.GitlabBuildPath == nil || *a.GitlabBuildPath != "/" {
							return fmt.Errorf("server gitlab source = type %q, project %v, namespace %v", a.SourceType, a.GitlabProjectID, a.GitlabPathNamespace)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: app(`
  bitbucket = {
    bitbucket_id    = dokploy_bitbucket_provider.bb.id
    owner           = "workspace"
    repository      = "App"
    repository_slug = "app"
    branch          = "main"
    build_path      = "/svc"
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_application.test", "gitlab.owner"),
					checkApplicationServer("dokploy_application.test", func(a *client.Application) error {
						if a.SourceType != "bitbucket" || a.BitbucketRepositorySlug == nil || *a.BitbucketRepositorySlug != "app" ||
							a.BitbucketBuildPath == nil || *a.BitbucketBuildPath != "/svc" {
							return fmt.Errorf("server bitbucket source = type %q, slug %v, path %v", a.SourceType, a.BitbucketRepositorySlug, a.BitbucketBuildPath)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: app(`
  gitea = {
    gitea_id   = dokploy_gitea_provider.gt.id
    owner      = "org"
    repository = "app"
    branch     = "main"
  }`),
				Check: checkApplicationServer("dokploy_application.test", func(a *client.Application) error {
					if a.SourceType != "gitea" || a.GiteaOwner == nil || *a.GiteaOwner != "org" || a.GiteaBuildPath == nil || *a.GiteaBuildPath != "/" {
						return fmt.Errorf("server gitea source = type %q, owner %v", a.SourceType, a.GiteaOwner)
					}
					return nil
				}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// deploy_on_change is provider-only: the import seeds its
				// default (true), and this config turns it off on purpose.
				ResourceName:            "dokploy_application.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"deploy_on_change"},
			},
		},
	})
}

// v1.0.0 promises that a v0.13.0 state loads with an empty plan. Step 1
// creates the application with v0.13.0 from the registry; step 2 plans the
// same configuration with the local build and expects no change.
func TestAccApplication_upgradeFromV0_13(t *testing.T) {
	name := acctest.RandomName("app-up")
	cfg := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name             = %q
  environment_id   = dokploy_project.test.production_environment_id
  description      = "upgrade test"
  env              = "WHOAMI_NAME=upgrade"
  deploy_on_change = false

  docker = {
    image = "traefik/whoami:v1.10"
  }
}
`, name+"-proj", name)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { acctest.PreCheck(t) },
		CheckDestroy: checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"dokploy": {Source: "vanillauys/dokploy", VersionConstraint: "0.13.0"},
				},
				Config: cfg,
				Check:  resource.TestCheckResourceAttr("dokploy_application.test", "description", "upgrade test"),
			},
			{
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				Config:                   cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("dokploy_application.test", "description", "upgrade test"),
			},
		},
	})
}
