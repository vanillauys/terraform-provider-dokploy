// Package network_test holds the acceptance tests (external package; acctest
// imports provider, which imports network).
package network_test

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

func checkNetworkDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_network" {
			continue
		}
		if _, err := c.GetNetwork(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("network %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func TestAccNetwork_lifecycle(t *testing.T) {
	name := acctest.RandomName("net")
	basic := fmt.Sprintf(`
resource "dokploy_network" "test" {
  name = %q
}
`, name)
	full := fmt.Sprintf(`
resource "dokploy_network" "test" {
  name        = %q
  driver      = "bridge"
  attachable  = true
  enable_ipv6 = false
  mtu         = 1400
  ipam = {
    driver = "default"
    config = [{
      subnet   = "172.28.0.0/16"
      gateway  = "172.28.0.1"
      ip_range = "172.28.5.0/24"
    }]
  }
}
`, name+"-full")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_network.test", "name", name),
					resource.TestCheckResourceAttr("dokploy_network.test", "driver", "bridge"),
					resource.TestCheckResourceAttrSet("dokploy_network.test", "id"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// The name change forces a replace: every attribute is
				// RequiresReplace, there is no update endpoint.
				Config: full,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_network.test", "mtu", "1400"),
					resource.TestCheckResourceAttr("dokploy_network.test", "ipam.config.0.subnet", "172.28.0.0/16"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("dokploy_network.test", plancheck.ResourceActionReplace),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_network.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccNetwork_attachToApplication covers spec §4.4's "remove-then-recreate
// plans do not break" clause: an application referencing a network's id
// through a Terraform reference must follow the network across a replace,
// and detaching (removing the reference from config) must converge cleanly.
//
// Destroy order: Terraform destroys dokploy_application before
// dokploy_network here because the application depends on the network (the
// network_ids reference), so remove-while-attached (doc.go, wave 6b gate B:
// network.remove succeeds even while referenced, leaving a dangling id)
// never arises in this test - the application is always gone first.
func TestAccNetwork_attachToApplication(t *testing.T) {
	name := acctest.RandomName("app-net")
	netName := acctest.RandomName("net")

	cfg := func(networkName string, attach bool) string {
		project := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}
`, name+"-proj")
		net := fmt.Sprintf(`
resource "dokploy_network" "test" {
  name = %q
}
`, networkName)
		if !attach {
			return project + net + fmt.Sprintf(`
resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
  docker         = { image = "traefik/whoami:v1.10" }

  deploy_on_change = false
}
`, name)
		}
		return project + net + fmt.Sprintf(`
resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
  docker         = { image = "traefik/whoami:v1.10" }

  network_ids = [dokploy_network.test.id]

  deploy_on_change = false
}
`, name)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg(netName, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemAttrPair("dokploy_application.test", "network_ids.*", "dokploy_network.test", "id"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Renaming the network forces a replace (every network
				// attribute is RequiresReplace). The application's
				// network_ids reference must follow the new id.
				Config: cfg(netName+"-renamed", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemAttrPair("dokploy_application.test", "network_ids.*", "dokploy_network.test", "id"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("dokploy_network.test", plancheck.ResourceActionReplace),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Detach: network_ids removed from the application's config.
				// Proves detach converges cleanly.
				Config: cfg(netName+"-renamed", false),
				Check:  resource.TestCheckNoResourceAttr("dokploy_application.test", "network_ids"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccNetwork_overlay confirms the acceptance rig accepts the overlay
// driver (doc.go, wave 6b gate C: the rig's inner dockerd runs swarm).
func TestAccNetwork_overlay(t *testing.T) {
	name := acctest.RandomName("net-overlay")
	cfg := fmt.Sprintf(`
resource "dokploy_network" "test" {
  name       = %q
  driver     = "overlay"
  attachable = true
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_network.test", "driver", "overlay"),
					resource.TestCheckResourceAttr("dokploy_network.test", "attachable", "true"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
