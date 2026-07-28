// Package mount_test holds the acceptance tests. It must be external:
// acctest imports provider, and provider imports mount to register
// dokploy_mount, so an internal test file importing acctest would form the
// cycle mount -> acctest -> provider -> mount.
package mount_test

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

func checkMountDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_mount" {
			continue
		}
		if _, err := c.GetMount(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("mount %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func checkMountServer(resourceName string, assert func(*client.Mount) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s not in state", resourceName)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		m, err := c.GetMount(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading mount %s: %w", rs.Primary.ID, err)
		}
		return assert(m)
	}
}

// TestAccMount_allSubtypesOnAnApplication covers bind, volume and file on one
// application, plus a mutation and the empty-plan convergence check.
func TestAccMount_allSubtypesOnAnApplication(t *testing.T) {
	name := acctest.RandomName("mount-app")
	base := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
  docker = { image = "traefik/whoami:v1.10" }
  deploy_on_change = false
}
`, name+"-proj", name)

	mounts := func(bindPath string) string {
		return base + fmt.Sprintf(`
resource "dokploy_mount" "bind" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  type         = "bind"
  host_path    = %q
  mount_path   = "/data/bind"
}

resource "dokploy_mount" "volume" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  type         = "volume"
  volume_name  = "%s-vol"
  mount_path   = "/data/volume"
}

resource "dokploy_mount" "file" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  type         = "file"
  file_path    = "settings.json"
  content      = "{\"a\":1}"
  mount_path   = "/data/file"
}
`, bindPath, name)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMountDestroy,
		Steps: []resource.TestStep{
			{
				Config: mounts("/tmp/acc-bind"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mount.bind", "host_path", "/tmp/acc-bind"),
					resource.TestCheckResourceAttr("dokploy_mount.volume", "volume_name", name+"-vol"),
					resource.TestCheckResourceAttr("dokploy_mount.file", "file_path", "settings.json"),
					checkMountServer("dokploy_mount.bind", func(m *client.Mount) error {
						if m.Type != "bind" || m.HostPath == nil || *m.HostPath != "/tmp/acc-bind" {
							return fmt.Errorf("server bind mount = %+v", m)
						}
						if m.ServiceType != "application" {
							return fmt.Errorf("server serviceType = %q, want application", m.ServiceType)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// In-place update through the dialect B endpoint.
				Config: mounts("/tmp/acc-bind-moved"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mount.bind", "host_path", "/tmp/acc-bind-moved"),
					checkMountServer("dokploy_mount.bind", func(m *client.Mount) error {
						if m.HostPath == nil || *m.HostPath != "/tmp/acc-bind-moved" {
							return fmt.Errorf("server host_path = %v, want /tmp/acc-bind-moved", m.HostPath)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "dokploy_mount.volume",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccMount_onADatabaseParent is the case the live vanillauys stack needs:
// its only user mount hangs off a postgres, not an application. A mount
// resource that only ever ran against applications would pass a
// single-parent suite and then fail the migration.
func TestAccMount_onADatabaseParent(t *testing.T) {
	name := acctest.RandomName("mount-pg")
	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_postgres" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-pass-12345"
  deploy_on_change  = false
}

resource "dokploy_mount" "pg" {
  service_id   = dokploy_postgres.test.id
  service_type = "postgres"
  type         = "volume"
  volume_name  = "%s-extra"
  mount_path   = "/var/lib/extra"
}
`, name+"-proj", name, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMountDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mount.pg", "service_type", "postgres"),
					resource.TestCheckResourceAttrPair(
						"dokploy_mount.pg", "service_id", "dokploy_postgres.test", "id"),
					checkMountServer("dokploy_mount.pg", func(m *client.Mount) error {
						if m.ServiceType != "postgres" {
							return fmt.Errorf("server serviceType = %q", m.ServiceType)
						}
						if m.PostgresID == nil {
							return errors.New("server postgresId is null on a postgres-parented mount")
						}
						if m.ApplicationID != nil {
							return fmt.Errorf("server applicationId = %v, want null: a postgres mount "+
								"must not carry an application parent", *m.ApplicationID)
						}
						return nil
					}),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
