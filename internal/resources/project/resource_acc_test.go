// Package project_test (an external test package, deliberately distinct
// from package project) holds the acceptance test. It must live outside
// package project: acctest imports provider, and provider imports project
// to register dokploy_project — so an internal test file (package project)
// importing acctest here would form an import cycle
// (project -> acctest -> provider -> project), which the Go toolchain
// rejects with "import cycle not allowed in test". Keeping this file in
// the external project_test package sidesteps that: it depends on project
// (indirectly, via provider) without itself being part of project.
package project_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func checkProjectDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_project" {
			continue
		}
		if _, err := c.GetProject(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("project %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func TestAccProject_lifecycle(t *testing.T) {
	name := acctest.RandomName("proj")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name        = %q
  description = "made by acceptance"
}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_project.test", "id"),
					resource.TestCheckResourceAttr("dokploy_project.test", "name", name),
					resource.TestCheckResourceAttrSet("dokploy_project.test", "created_at"),
					resource.TestCheckResourceAttrSet("dokploy_project.test", "environments.0.id"),
					func(s *terraform.State) error { // verify via direct API read (spec §7)
						rs := s.RootModule().Resources["dokploy_project.test"]
						c, err := acctest.ClientFromEnv()
						if err != nil {
							return err
						}
						_, err = c.GetProject(context.Background(), rs.Primary.ID)
						return err
					},
				),
			},
			{
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name        = %q
  description = "updated"
}`, name),
				Check: resource.TestCheckResourceAttr("dokploy_project.test", "description", "updated"),
			},
			{
				ResourceName:      "dokploy_project.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
