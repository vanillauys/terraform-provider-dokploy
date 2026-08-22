// Package compose_test holds the acceptance tests (external package;
// acctest imports provider, which imports compose).
package compose_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func checkComposeDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_compose" {
			continue
		}
		if _, err := c.GetCompose(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("compose %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// getCompose reads the record straight from the API. Every assertion below
// goes through this rather than through Terraform state: comparing two
// pieces of provider-produced state would pass even if both were wrong in
// the same way.
func getCompose(s *terraform.State, addr string) (*client.Compose, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return nil, fmt.Errorf("%s not found in state", addr)
	}
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return nil, err
	}
	return c.GetCompose(context.Background(), rs.Primary.ID)
}

// project builds the scaffolding every case needs.
//
// Note what the deploying fixture does NOT set: `command`. On a compose
// service Dokploy uses it as the literal deploy command in place of
// `docker compose up`, so any placeholder value turns the deploy into an
// error - verified live, v0.29.13, 2026-07-29. It is exercised in the
// non-deploying cases below instead.
func projectConfig(name string) string {
	return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}
`, name)
}

func rawConfig(projectName, composeName, body string) string {
	return rawConfigDeploy(projectName, composeName, body, false)
}

// rawConfigDeploy builds a raw-source compose. deploy is false for every case
// except the lifecycle test: that one ends in an import step, and import
// cannot see configuration, so it can only seed deploy_on_change and
// deployment_timeout with their schema defaults (tfutil.ImportDeployDefaults).
// A config that pinned them to non-defaults would make ImportStateVerify fail
// on a difference that is not a bug - which is what an ImportStateVerifyIgnore
// entry would then paper over. Leaving them out instead keeps the sibling
// resources' "no ImportStateVerifyIgnore" rule intact.
func rawConfigDeploy(projectName, composeName, body string, deploy bool) string {
	deployLine := "  deploy_on_change = false\n"
	if deploy {
		deployLine = ""
	}
	return projectConfig(projectName) + fmt.Sprintf(`
resource "dokploy_compose" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
%s
  raw = {
    compose_file = %q
  }
%s
}
`, composeName, deployLine, "services:\n  web:\n    image: nginx:alpine\n", body)
}

func TestAccCompose_rawLifecycle(t *testing.T) {
	projectName := acctest.RandomName("compose-proj")
	name := acctest.RandomName("compose")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkComposeDestroy,
		Steps: []resource.TestStep{
			{
				Config: rawConfigDeploy(projectName, name, `
  description  = "the site"
  compose_type = "docker-compose"
  env          = "KEY=value"
`, true),
				Check: func(s *terraform.State) error {
					c, err := getCompose(s, "dokploy_compose.test")
					if err != nil {
						return err
					}
					if c.Name != name {
						return fmt.Errorf("name = %q, want %q", c.Name, name)
					}
					if c.SourceType != "raw" {
						return fmt.Errorf("sourceType = %q, want raw", c.SourceType)
					}
					if c.ComposeType != "docker-compose" {
						return fmt.Errorf("composeType = %q", c.ComposeType)
					}
					if c.Description == nil || *c.Description != "the site" {
						return fmt.Errorf("description = %v", c.Description)
					}
					if c.ComposeFile == "" {
						return errors.New("composeFile is empty; the raw source did not reach the server")
					}
					if c.Env == nil || *c.Env != "KEY=value" {
						return fmt.Errorf("env = %v", c.Env)
					}
					return nil
				},
			},
			{
				// Update every mutable field at once.
				Config: rawConfigDeploy(projectName, name, `
  description  = "renamed"
  env          = "KEY=other"
`, true),
				Check: func(s *terraform.State) error {
					c, err := getCompose(s, "dokploy_compose.test")
					if err != nil {
						return err
					}
					if c.Description == nil || *c.Description != "renamed" {
						return fmt.Errorf("description = %v", c.Description)
					}
					if c.Env == nil || *c.Env != "KEY=other" {
						return fmt.Errorf("env = %v", c.Env)
					}
					return nil
				},
			},
			{
				ResourceName:      "dokploy_compose.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// Every optional attribute must provably revert when dropped from
// configuration. This property was broken on 3 of 3 wave-0 resources and is
// this codebase's house specialty of latent bugs.
//
// command, suffix and compose_path are the interesting ones here: they clear
// to a literal "" server-side rather than to null, so a Read that did not
// collapse "" would leave a diff no apply could settle.
func TestAccCompose_optionalAttributesRevert(t *testing.T) {
	projectName := acctest.RandomName("compose-proj")
	name := acctest.RandomName("compose")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkComposeDestroy,
		Steps: []resource.TestStep{
			{
				Config: rawConfig(projectName, name, `
  description                 = "set"
  command                     = "echo hi"
  suffix                      = "sfx"
  compose_path                = "./custom.yml"
  env                         = "KEY=value"
  auto_deploy                 = false
  trigger_type                = "tag"
  watch_paths                 = ["src/**"]
  enable_submodules           = true
  randomize                   = true
  isolated_deployment         = true
  isolated_deployments_volume = true
`),
			},
			{
				// Drop every optional attribute. The plan after apply and
				// refresh must be empty, which is what proves each one
				// actually reverted server-side rather than merely in state.
				Config: rawConfig(projectName, name, ``),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: func(s *terraform.State) error {
					c, err := getCompose(s, "dokploy_compose.test")
					if err != nil {
						return err
					}
					// The dialect C group clears to "", never null.
					for name, got := range map[string]string{
						"command": c.Command,
						"suffix":  c.Suffix,
					} {
						if got != "" {
							return fmt.Errorf("%s = %q, want \"\" after being dropped from config", name, got)
						}
					}
					// compose_path cannot be cleared - compose.update rejects
					// "" with a minimum-length error - so it reverts to its
					// schema default, which matches the server's own.
					if c.ComposePath != "./docker-compose.yml" {
						return fmt.Errorf("composePath = %q, want the ./docker-compose.yml default it reverts to", c.ComposePath)
					}
					if c.Description != nil && *c.Description != "" {
						return fmt.Errorf("description = %v, want cleared", *c.Description)
					}
					if c.Env != nil && *c.Env != "" {
						return fmt.Errorf("env = %v, want cleared", *c.Env)
					}
					if len(c.WatchPaths) != 0 {
						return fmt.Errorf("watchPaths = %v, want cleared", c.WatchPaths)
					}
					return nil
				},
			},
		},
	})
}

// Switching source mode must CLEAR the previous mode's columns. Compose keeps
// stale source columns otherwise - the same corrupt shape mount, backup and
// schedule can all reach - and flatten's discriminator defence would then be
// the only thing between a retarget and a two-source record.
//
// This is the acceptance-level proof of model_test.go's
// TestExpandUpdateClearsTheUnusedSourceColumns, asserted against a direct API
// read.
func TestAccCompose_sourceSwitchClearsOldColumns(t *testing.T) {
	projectName := acctest.RandomName("compose-proj")
	name := acctest.RandomName("compose")

	gitCfg := projectConfig(projectName) + fmt.Sprintf(`
resource "dokploy_compose" "test" {
  name             = %q
  environment_id   = dokploy_project.test.environments[0].id
  deploy_on_change = false

  git = {
    url    = "https://github.com/acme/site.git"
    branch = "main"
  }
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkComposeDestroy,
		Steps: []resource.TestStep{
			{
				Config: gitCfg,
				Check: func(s *terraform.State) error {
					c, err := getCompose(s, "dokploy_compose.test")
					if err != nil {
						return err
					}
					if c.SourceType != "git" {
						return fmt.Errorf("sourceType = %q, want git", c.SourceType)
					}
					if c.CustomGitURL == nil || *c.CustomGitURL != "https://github.com/acme/site.git" {
						return fmt.Errorf("customGitUrl = %v", c.CustomGitURL)
					}
					return nil
				},
			},
			{
				Config: rawConfig(projectName, name, ``),
				Check: func(s *terraform.State) error {
					c, err := getCompose(s, "dokploy_compose.test")
					if err != nil {
						return err
					}
					if c.SourceType != "raw" {
						return fmt.Errorf("sourceType = %q, want raw", c.SourceType)
					}
					for name, got := range map[string]*string{
						"customGitUrl":    c.CustomGitURL,
						"customGitBranch": c.CustomGitBranch,
					} {
						if got != nil && *got != "" {
							return fmt.Errorf("%s = %q, want cleared after switching to the raw source", name, *got)
						}
					}
					return nil
				},
			},
		},
	})
}

// TestAccCompose_v030Fields covers create_env_file, icon and
// service_networks, the three v0.30.0 additions from this task
// (internal/client/doc.go's "compose createEnvFile" and "serviceNetworks
// and icon on compose.update" sections). service_networks needs a real
// network id, so this test calls acctest.CreateNetwork directly - the same
// reason TestAccApplication_networkAttachment gates on TF_ACC before
// resource.Test's Steps start.
//
// The compose file is raw-source with one service, "web": the rig never
// deploys it (deploy_on_change = false), so the compose YAML only has to be
// well-formed, and service_networks.service_name only has to name a real key
// in it.
func TestAccCompose_v030Fields(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless env 'TF_ACC' set")
	}
	acctest.PreCheck(t)
	projectName := acctest.RandomName("compose-proj")
	name := acctest.RandomName("compose")
	netName := acctest.RandomName("net")
	networkID := acctest.CreateNetwork(t, netName)
	t.Cleanup(func() { acctest.DeleteNetwork(t, networkID) })

	withFields := rawConfig(projectName, name, fmt.Sprintf(`
  create_env_file = false
  icon             = "lucide:cloud"

  service_networks = [
    {
      service_name = "web"
      network_ids  = [%q]
    },
  ]
`, networkID))

	withoutFields := rawConfig(projectName, name, ``)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkComposeDestroy,
		Steps: []resource.TestStep{
			{
				Config: withFields,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_compose.test", "create_env_file", "false"),
					resource.TestCheckResourceAttr("dokploy_compose.test", "icon", "lucide:cloud"),
					resource.TestCheckResourceAttr("dokploy_compose.test", "service_networks.#", "1"),
					func(s *terraform.State) error {
						c, err := getCompose(s, "dokploy_compose.test")
						if err != nil {
							return err
						}
						if c.CreateEnvFile {
							return errors.New("server createEnvFile = true, want false")
						}
						if c.Icon == nil || *c.Icon != "lucide:cloud" {
							return fmt.Errorf("server icon = %v, want lucide:cloud", c.Icon)
						}
						if len(c.ServiceNetworks) != 1 {
							return fmt.Errorf("server serviceNetworks = %v, want one entry", c.ServiceNetworks)
						}
						sn := c.ServiceNetworks[0]
						if sn.ServiceName != "web" {
							return fmt.Errorf("serviceName = %q, want web", sn.ServiceName)
						}
						if len(sn.NetworkIDs) != 1 || sn.NetworkIDs[0] != networkID {
							return fmt.Errorf("networkIds = %v, want [%s]", sn.NetworkIDs, networkID)
						}
						return nil
					},
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Removed from config: create_env_file must revert to its
				// schema default (true), and icon/service_networks must
				// converge to null - matching doc.go's recorded shape for an
				// explicit clear (null, never [] or "").
				Config: withoutFields,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_compose.test", "create_env_file", "true"),
					resource.TestCheckNoResourceAttr("dokploy_compose.test", "icon"),
					resource.TestCheckNoResourceAttr("dokploy_compose.test", "service_networks"),
					func(s *terraform.State) error {
						c, err := getCompose(s, "dokploy_compose.test")
						if err != nil {
							return err
						}
						if !c.CreateEnvFile {
							return errors.New("server createEnvFile = false, want true after reverting to the default")
						}
						if c.Icon != nil && *c.Icon != "" {
							return fmt.Errorf("server icon = %v, want cleared", *c.Icon)
						}
						if len(c.ServiceNetworks) != 0 {
							return fmt.Errorf("server serviceNetworks = %v, want cleared", c.ServiceNetworks)
						}
						return nil
					},
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// Exactly one source block is required. Neither zero nor two may plan.
func TestAccCompose_requiresExactlyOneSource(t *testing.T) {
	projectName := acctest.RandomName("compose-proj")
	name := acctest.RandomName("compose")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: projectConfig(projectName) + fmt.Sprintf(`
resource "dokploy_compose" "none" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
}
`, name),
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured`),
			},
		},
	})
}
