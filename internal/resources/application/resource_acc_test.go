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

// Docker source: fastest to deploy, exercises the full engine.
func TestAccApplication_dockerLifecycle(t *testing.T) {
	name := acctest.RandomName("app")
	config := func(image string) string {
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

  env                = "WHOAMI_NAME=acceptance"
  deployment_timeout = "10m"
}`, name+"-proj", name, image)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("traefik/whoami:v1.10"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_application.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_application.test", "app_name"),
					resource.TestCheckResourceAttr("dokploy_application.test", "status", "done"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["dokploy_application.test"]
						c, err := acctest.ClientFromEnv()
						if err != nil {
							return err
						}
						app, err := c.GetApplication(context.Background(), rs.Primary.ID)
						if err != nil {
							return err
						}
						if app.SourceType != "docker" {
							return fmt.Errorf("sourceType = %q, want docker", app.SourceType)
						}
						return nil
					},
				),
			},
			{
				// Image change is a deploy trigger.
				Config: config("traefik/whoami:v1.10.2"),
				Check:  resource.TestCheckResourceAttr("dokploy_application.test", "status", "done"),
			},
			{
				ResourceName:            "dokploy_application.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"deploy_on_change", "deployment_timeout"},
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
