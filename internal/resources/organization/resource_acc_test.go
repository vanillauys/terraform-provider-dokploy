// Package organization_test holds the acceptance tests (external package;
// acctest imports provider, which imports organization).
package organization_test

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

func checkDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_organization" {
			continue
		}
		if _, err := c.GetOrganization(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("organization %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkServer(assert func(*client.Organization) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_organization.test"]
		if !ok {
			return fmt.Errorf("dokploy_organization.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		o, err := c.GetOrganization(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		return assert(o)
	}
}

func config(name, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_organization" "test" {
  name = %q
%s
}
`, name, extra)
}

func TestAccOrganization_lifecycle(t *testing.T) {
	name := acctest.RandomName("org")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(name, "  logo = \"https://example.com/logo.png\"\n  default_role = \"admin\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_organization.test", "logo", "https://example.com/logo.png"),
					resource.TestCheckResourceAttr("dokploy_organization.test", "default_role", "admin"),
					resource.TestCheckResourceAttrSet("dokploy_organization.test", "slug"),
					resource.TestCheckResourceAttrSet("dokploy_organization.test", "owner_id"),
					checkServer(func(o *client.Organization) error {
						if o.Logo != "https://example.com/logo.png" || o.DefaultRole != "admin" {
							return fmt.Errorf("server = %+v", o)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// The logo dropped clears on the server; the default role has
				// no clear path and stays, in the state as on the server.
				Config: config(name+"-renamed", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_organization.test", "logo"),
					resource.TestCheckResourceAttr("dokploy_organization.test", "default_role", "admin"),
					checkServer(func(o *client.Organization) error {
						if o.Name != name+"-renamed" || o.Logo != "" {
							return fmt.Errorf("server = %+v", o)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_organization.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_organization.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
