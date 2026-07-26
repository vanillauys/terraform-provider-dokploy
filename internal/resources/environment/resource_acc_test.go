// Package environment_test is an external test package. It must be: acctest
// imports provider, and provider imports this package to register
// dokploy_environment, so an internal test file importing acctest would form
// the import cycle environment -> acctest -> provider -> environment.
package environment_test

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

func checkEnvironmentDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_environment" {
			continue
		}
		if _, err := c.GetEnvironment(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("environment %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func fetchEnvironment(s *terraform.State, address string) (*client.Environment, error) {
	rs, ok := s.RootModule().Resources[address]
	if !ok {
		return nil, fmt.Errorf("%s not found in state", address)
	}
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return nil, err
	}
	return c.GetEnvironment(context.Background(), rs.Primary.ID)
}

func TestAccEnvironment_lifecycle(t *testing.T) {
	projectName := acctest.RandomName("proj")
	envName := acctest.RandomName("env")

	config := func(extra string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_environment" "test" {
  project_id = dokploy_project.test.id
  name       = %q
%s
}`, projectName, envName, extra)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkEnvironmentDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("  description = \"made by acceptance\"\n  env = \"FOO=bar\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_environment.test", "id"),
					resource.TestCheckResourceAttr("dokploy_environment.test", "name", envName),
					resource.TestCheckResourceAttr("dokploy_environment.test", "description", "made by acceptance"),
					// env is discarded by environment.create, so this only
					// passes if Create issues the follow-up update.
					resource.TestCheckResourceAttr("dokploy_environment.test", "env", "FOO=bar"),
					resource.TestCheckResourceAttr("dokploy_environment.test", "is_default", "false"),
					func(s *terraform.State) error {
						e, err := fetchEnvironment(s, "dokploy_environment.test")
						if err != nil {
							return err
						}
						if e.Env != "FOO=bar" {
							return fmt.Errorf("server stores env %q, want FOO=bar — Create did not follow up with an update", e.Env)
						}
						return nil
					},
				),
			},
			{
				Config: config("  description = \"updated\"\n  env = \"FOO=baz\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_environment.test", "description", "updated"),
					resource.TestCheckResourceAttr("dokploy_environment.test", "env", "FOO=baz"),
				),
			},
			{
				// Spec §5.6 / dialect C: both optional attributes must be
				// clearable. The server cannot store null here — it stores ""
				// — so this only converges if Update sends "" AND Read maps ""
				// back to null. Get either half wrong and the plan is never
				// empty.
				Config: config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_environment.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_environment.test", "env"),
					func(s *terraform.State) error {
						e, err := fetchEnvironment(s, "dokploy_environment.test")
						if err != nil {
							return err
						}
						if e.Description != "" || e.Env != "" {
							return fmt.Errorf("server still stores description=%q env=%q after both were removed from config", e.Description, e.Env)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "dokploy_environment.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// The auto-created production environment must import cleanly and report
// is_default = true. That flag is what makes Delete refuse.
//
// There is deliberately NO acceptance test that destroys a default
// environment. The refusal is real and permanent, so resource.Test's own
// end-of-test destroy would hit it and fail the run no matter how the step
// itself is written. The refusal path is covered by
// TestDeleteBlockedReason in model_test.go instead.
func TestAccEnvironment_importDefault(t *testing.T) {
	projectName := acctest.RandomName("proj")
	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}`, projectName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_project.test", "environments.0.name", "production"),
					// Read the auto-created environment straight from the API
					// and confirm the flag Delete keys off is actually set.
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["dokploy_project.test"]
						id := rs.Primary.Attributes["environments.0.id"]
						c, err := acctest.ClientFromEnv()
						if err != nil {
							return err
						}
						e, err := c.GetEnvironment(context.Background(), id)
						if err != nil {
							return err
						}
						if !e.IsDefault {
							return fmt.Errorf("environment %s (%q) has isDefault=false; the Delete guard would not fire", id, e.Name)
						}
						return nil
					},
				),
			},
		},
	})
}

// Dokploy allows two environments in one project to share a name. The
// resource must not assume otherwise.
func TestAccEnvironment_duplicateNamesAllowed(t *testing.T) {
	projectName := acctest.RandomName("proj")
	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_environment" "a" {
  project_id = dokploy_project.test.id
  name       = "shared"
}

resource "dokploy_environment" "b" {
  project_id = dokploy_project.test.id
  name       = "shared"
}`, projectName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkEnvironmentDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_environment.a", "id"),
					resource.TestCheckResourceAttrSet("dokploy_environment.b", "id"),
				),
			},
		},
	})
}
