// Package server_test holds the acceptance tests (external package; acctest
// imports provider, which imports server).
package server_test

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

func checkServerDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_server" {
			continue
		}
		if _, err := c.GetServer(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("server %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// checkAgainstAPI compares the state with a direct API read, so that a
// flatten bug and a request bug cannot cancel each other out.
func checkAgainstAPI(want client.Server) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["dokploy_server.test"]
		if !ok {
			return fmt.Errorf("dokploy_server.test not found in state")
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		got, err := c.GetServer(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if got.Name != want.Name || got.Description != want.Description || got.IPAddress != want.IPAddress ||
			got.Port != want.Port || got.Username != want.Username || got.ServerType != want.ServerType ||
			got.EnableDockerCleanup != want.EnableDockerCleanup || got.Command != want.Command ||
			(got.SSHKeyID == "") != (want.SSHKeyID == "") {
			return fmt.Errorf("server on the API = %+v, want %+v", got, want)
		}
		return nil
	}
}

// Dokploy never contacts the machine on create or update, so a private
// address that answers nothing is fine here.
func serverConfig(name, pub, priv, extra string) string {
	return fmt.Sprintf(`
resource "dokploy_ssh_key" "fixture" {
  name        = %[1]q
  public_key  = %[2]q
  private_key = %[3]q
}

resource "dokploy_server" "test" {
  name       = %[1]q
  ip_address = "10.255.255.1"
%[4]s
}
`, name, pub, priv, extra)
}

func TestAccServer_lifecycle(t *testing.T) {
	name := acctest.RandomName("srv")
	pub, priv := acctest.GenerateSSHKey(t)
	full := `  description           = "build box"
  port                  = 2222
  username              = "ubuntu"
  ssh_key_id            = dokploy_ssh_key.fixture.id
  server_type           = "build"
  enable_docker_cleanup = false
  command               = "echo setup"`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkServerDestroy,
		Steps: []resource.TestStep{
			{
				Config: serverConfig(name, pub, priv, full),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_server.test", "port", "2222"),
					resource.TestCheckResourceAttr("dokploy_server.test", "server_type", "build"),
					resource.TestCheckResourceAttr("dokploy_server.test", "command", "echo setup"),
					resource.TestCheckResourceAttrPair("dokploy_server.test", "ssh_key_id", "dokploy_ssh_key.fixture", "id"),
					resource.TestCheckResourceAttrSet("dokploy_server.test", "app_name"),
					resource.TestCheckResourceAttrSet("dokploy_server.test", "created_at"),
					checkAgainstAPI(client.Server{Name: name, Description: "build box", IPAddress: "10.255.255.1", Port: 2222,
						Username: "ubuntu", ServerType: "build", Command: "echo setup", SSHKeyID: "set"}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Every optional attribute dropped: the defaults come back and
				// the free-text fields and the key clear on the server.
				Config: serverConfig(name, pub, priv, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_server.test", "port", "22"),
					resource.TestCheckResourceAttr("dokploy_server.test", "username", "root"),
					resource.TestCheckResourceAttr("dokploy_server.test", "server_type", "deploy"),
					resource.TestCheckResourceAttr("dokploy_server.test", "enable_docker_cleanup", "true"),
					resource.TestCheckNoResourceAttr("dokploy_server.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_server.test", "command"),
					resource.TestCheckNoResourceAttr("dokploy_server.test", "ssh_key_id"),
					checkAgainstAPI(client.Server{Name: name, IPAddress: "10.255.255.1", Port: 22, Username: "root",
						ServerType: "deploy", EnableDockerCleanup: true}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_server.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_server.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
