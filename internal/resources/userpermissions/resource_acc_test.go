// Package userpermissions_test holds the acceptance tests (external
// package; acctest imports provider, which imports userpermissions).
package userpermissions_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func checkServer(assert func(*client.Member) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources["dokploy_user_permissions.test"]
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		m, err := c.GetMember(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		return assert(m)
	}
}

func config(name, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_project" "fixture" {
  name = %[1]q
}

resource "dokploy_user" "fixture" {
  email    = "%[1]s@example.com"
  password = "acceptance-only-pass"
  role     = "member"
}

resource "dokploy_user_permissions" "test" {
  user_id = dokploy_user.fixture.id
%[2]s
}
`, name, extra)
}

func TestAccUserPermissions_lifecycle(t *testing.T) {
	name := acctest.RandomName("perm")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config(name, `  accessed_projects   = [dokploy_project.fixture.id]
  can_access_api      = true
  can_create_services = true`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_user_permissions.test", "can_access_api", "true"),
					resource.TestCheckResourceAttr("dokploy_user_permissions.test", "can_delete_projects", "false"),
					resource.TestCheckResourceAttr("dokploy_user_permissions.test", "accessed_projects.#", "1"),
					resource.TestCheckResourceAttr("dokploy_user_permissions.test", "accessed_servers.#", "0"),
					resource.TestCheckResourceAttrPair("dokploy_user_permissions.test", "member_id", "dokploy_user.fixture", "member_id"),
					checkServer(func(m *client.Member) error {
						if !m.CanAccessToAPI || !m.CanCreateServices || len(m.AccessedProjects) != 1 {
							return fmt.Errorf("server = %+v", m)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Everything dropped: the defaults (false, empty) come back.
				Config: config(name, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_user_permissions.test", "can_access_api", "false"),
					resource.TestCheckResourceAttr("dokploy_user_permissions.test", "accessed_projects.#", "0"),
					checkServer(func(m *client.Member) error {
						if m.CanAccessToAPI || m.CanCreateServices || len(m.AccessedProjects) != 0 {
							return fmt.Errorf("server = %+v, want the defaults", m)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_user_permissions.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_user_permissions.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
