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
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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

// getAccProject re-reads the resource directly via the API (spec §7: verify
// server-side truth, not just Terraform's view of state).
func getAccProject(s *terraform.State) (*client.Project, error) {
	rs, ok := s.RootModule().Resources["dokploy_project.test"]
	if !ok {
		return nil, fmt.Errorf("dokploy_project.test not found in state")
	}
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return nil, err
	}
	return c.GetProject(context.Background(), rs.Primary.ID)
}

// checkProductionEnvironmentID asserts that production_environment_id equals
// the id of the `environments` entry named production. A fresh project has
// exactly that one environment, so the check also proves that the isDefault
// flag and the name agree on a new project.
func checkProductionEnvironmentID(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not found in state", name)
		}
		attrs := rs.Primary.Attributes
		n, err := strconv.Atoi(attrs["environments.#"])
		if err != nil {
			return fmt.Errorf("%s: environments.# = %q", name, attrs["environments.#"])
		}
		for i := 0; i < n; i++ {
			prefix := fmt.Sprintf("environments.%d.", i)
			if attrs[prefix+"name"] != "production" {
				continue
			}
			if got, want := attrs["production_environment_id"], attrs[prefix+"id"]; got != want {
				return fmt.Errorf("%s: production_environment_id = %q, want %q", name, got, want)
			}
			return nil
		}
		return fmt.Errorf("%s: no environment named production in state", name)
	}
}

func TestAccProject_lifecycle(t *testing.T) {
	name := acctest.RandomName("proj")
	// description is passed through so a step can drop it entirely, which is
	// what spec §5.6 (clearable back to null) requires.
	config := func(description string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
%s
}`, name, description)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(`  description = "made by acceptance"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_project.test", "id"),
					resource.TestCheckResourceAttr("dokploy_project.test", "name", name),
					resource.TestCheckResourceAttrSet("dokploy_project.test", "created_at"),
					resource.TestCheckResourceAttrSet("dokploy_project.test", "environments.0.id"),
					checkProductionEnvironmentID("dokploy_project.test"),
					func(s *terraform.State) error { // verify via direct API read (spec §7)
						_, err := getAccProject(s)
						return err
					},
				),
			},
			{
				Config: config(`  description = "updated"`),
				Check:  resource.TestCheckResourceAttr("dokploy_project.test", "description", "updated"),
			},
			{
				// Spec §5.6: optional attributes must be clearable back to
				// null, not merely settable. Dropping description from config
				// has to reach the server, not just Terraform state — with
				// `omitempty` on UpdateProjectRequest.Description the key
				// vanished from the body, project.update read that as "keep
				// the stored value" (verified live), the next Read flattened
				// the stale text back in, and every subsequent plan showed the
				// same diff forever. ExpectEmptyPlan catches exactly that; the
				// direct API read catches state-only clearing.
				Config: config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_project.test", "description"),
					func(s *terraform.State) error {
						p, err := getAccProject(s)
						if err != nil {
							return err
						}
						if p.Description != nil && *p.Description != "" {
							return fmt.Errorf("server still stores description %q; it was removed from config", *p.Description)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "dokploy_project.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
