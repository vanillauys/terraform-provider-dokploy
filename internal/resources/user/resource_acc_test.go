// Package user_test holds the acceptance tests (external package; acctest
// imports provider, which imports user).
package user_test

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
		if rs.Type != "dokploy_user" {
			continue
		}
		if _, err := c.GetMember(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("user %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkRole(role string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources["dokploy_user.test"]
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		m, err := c.GetMember(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if m.Role != role {
			return fmt.Errorf("server role = %q, want %q", m.Role, role)
		}
		return nil
	}
}

func config(email, role, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_user" "test" {
  email = %q
  role  = %q
%s
}
`, email, role, extra)
}

func TestAccUser_lifecycle(t *testing.T) {
	email := acctest.RandomName("user") + "@example.com"
	plain := "  password = \"acceptance-only-pass\""
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(email, "member", plain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_user.test", "email", email),
					resource.TestCheckResourceAttr("dokploy_user.test", "role", "member"),
					resource.TestCheckResourceAttrSet("dokploy_user.test", "member_id"),
					resource.TestCheckResourceAttrSet("dokploy_user.test", "created_at"),
					checkRole("member"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// A role change is an in-place update through updateMemberRole.
				Config: config(email, "admin", plain),
				Check:  checkRole("admin"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_user.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// A new password replaces the account: Dokploy has no reset
				// path for another user.
				Config: config(email, "admin", "  password_wo         = \"acceptance-only-pass-2\"\n  password_wo_version = 2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_user.test", plancheck.ResourceActionReplace)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_user.test", "password"),
					checkRole("admin"),
				),
			},
			{
				// The server never returns the password; an import cannot
				// recover it, and the resource description says so.
				ResourceName:            "dokploy_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}
