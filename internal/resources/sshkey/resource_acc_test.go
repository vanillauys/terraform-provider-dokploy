// Package sshkey_test holds the acceptance tests (external package; acctest
// imports provider, which imports sshkey).
package sshkey_test

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

func checkSSHKeyDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_ssh_key" {
			continue
		}
		if _, err := c.GetSSHKey(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("ssh key %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// checkServerKey reads the key back through the API and compares the
// private key. The write-only test asserts the server, not the state: in
// that mode the state holds no secret.
func checkServerKey(privateKey string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_ssh_key.test"]
		if !ok {
			return fmt.Errorf("dokploy_ssh_key.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		k, err := c.GetSSHKey(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if k.PrivateKey != privateKey {
			return fmt.Errorf("server private key has %d bytes, want %d", len(k.PrivateKey), len(privateKey))
		}
		return nil
	}
}

func sshKeyConfig(name, publicKey, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_ssh_key" "test" {
  name       = %q
  public_key = %q
%s
}
`, name, publicKey, extra)
}

func TestAccSSHKey_lifecycle(t *testing.T) {
	name := acctest.RandomName("key")
	pub, priv := acctest.GenerateSSHKey(t)
	plain := fmt.Sprintf("  private_key = %q", priv)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkSSHKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: sshKeyConfig(name, pub, plain+"\n  description = \"deploy key\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_ssh_key.test", "name", name),
					resource.TestCheckResourceAttr("dokploy_ssh_key.test", "description", "deploy key"),
					resource.TestCheckResourceAttr("dokploy_ssh_key.test", "public_key", pub),
					resource.TestCheckResourceAttr("dokploy_ssh_key.test", "private_key", priv),
					resource.TestCheckResourceAttrSet("dokploy_ssh_key.test", "organization_id"),
					resource.TestCheckResourceAttrSet("dokploy_ssh_key.test", "created_at"),
					checkServerKey(priv),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// A rename and a new description are an in-place update.
				Config: sshKeyConfig(name+"-renamed", pub, plain+"\n  description = \"renamed\""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_ssh_key.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("dokploy_ssh_key.test", "description", "renamed"),
			},
			{
				// Dropping the description must clear it on the server, not
				// carry the old value forward.
				Config: sshKeyConfig(name+"-renamed", pub, plain),
				Check:  resource.TestCheckNoResourceAttr("dokploy_ssh_key.test", "description"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_ssh_key.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccSSHKey_writeOnlyPrivateKey pins the companion: the state never holds
// the key, a rename keeps it, and a new version replaces the record with a
// new pair.
func TestAccSSHKey_writeOnlyPrivateKey(t *testing.T) {
	name := acctest.RandomName("key-wo")
	pub1, priv1 := acctest.GenerateSSHKey(t)
	pub2, priv2 := acctest.GenerateSSHKey(t)
	wo := func(priv string, version int) string {
		return fmt.Sprintf("  private_key_wo         = %q\n  private_key_wo_version = %d", priv, version)
	}
	noSecretInState := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr("dokploy_ssh_key.test", "private_key"),
		resource.TestCheckNoResourceAttr("dokploy_ssh_key.test", "private_key_wo"),
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkSSHKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: sshKeyConfig(name, pub1, wo(priv1, 1)),
				Check:  resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerKey(priv1)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: sshKeyConfig(name+"-renamed", pub1, wo(priv1, 1)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_ssh_key.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(noSecretInState, checkServerKey(priv1)),
			},
			{
				// A new pair: both halves change, and the version moves.
				Config: sshKeyConfig(name+"-renamed", pub2, wo(priv2, 2)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_ssh_key.test", plancheck.ResourceActionReplace)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					noSecretInState,
					resource.TestCheckResourceAttr("dokploy_ssh_key.test", "public_key", pub2),
					checkServerKey(priv2),
				),
			},
		},
	})
}
