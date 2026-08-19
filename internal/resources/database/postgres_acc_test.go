// Package database_test (an external test package, deliberately distinct
// from package database) holds the acceptance test. It must live outside
// package database: acctest imports provider, and provider imports
// database to register dokploy_postgres — so an internal test file
// (package database) importing acctest here would form an import cycle
// (database -> acctest -> provider -> database), which the Go toolchain
// rejects with "import cycle not allowed in test". Keeping this file in
// the external database_test package sidesteps that: it depends on
// database (indirectly, via provider) without itself being part of
// database. Mirrors internal/resources/project/resource_acc_test.go.
package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// checkPostgresDestroy and getAccPostgres are one-line calls into the
// shared checkDestroy/getAccObject helpers (acc_helpers_test.go) —
// extracted there in review round 1 on wave-2 task 5's mysql round, once
// mysql's own checkMysqlDestroy/getAccMysql made this a two-way (soon
// five-way) character-for-character copy.
var checkPostgresDestroy = checkDestroy("dokploy_postgres", func(ctx context.Context, c *client.Client, id string) error {
	_, err := c.GetPostgres(ctx, id)
	return err
})

func getAccPostgres(s *terraform.State) (*client.Postgres, error) {
	return getAccObject(s, "dokploy_postgres.test", func(ctx context.Context, c *client.Client, id string) (*client.Postgres, error) {
		return c.GetPostgres(ctx, id)
	})
}

func TestAccPostgres_lifecycle(t *testing.T) {
	name := acctest.RandomName("pg")
	// optionals is spliced in verbatim so a step can drop previously-set
	// optional attributes ENTIRELY (not set them to ""), which is what spec
	// §5.6 (clearable back to null) requires.
	//
	// deployment_timeout is deliberately left out of this config so it takes
	// its schema default. That is the case the final import step needs: since
	// import cannot see config, ImportState can only seed the defaults, so
	// ImportStateVerify only matches strictly when the config used the
	// defaults too — which is also the overwhelmingly common real-world
	// config. (A config that sets a non-default value still plans one diff
	// after import; that is inherent to a provider-only attribute, and it is
	// no worse than the null state it replaced.)
	base := func(optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_postgres" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = "postgres:16-alpine"
%s
}`, name+"-proj", name, optionals)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkPostgresDestroy,
		Steps: []resource.TestStep{
			{
				// Create deploys (deploy_on_change defaults to true) and
				// must end with the service in status done.
				Config: base("  env         = \"TZ=UTC\"\n  description = \"managed by the acceptance suite\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_postgres.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_postgres.test", "app_name"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "description", "managed by the acceptance suite"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.Env == nil || *pg.Env != "TZ=UTC" {
							return fmt.Errorf("env not saved: %v", pg.Env)
						}
						return nil
					},
				),
			},
			{
				// env is a deploy trigger; update must redeploy and converge.
				Config: base("  env         = \"TZ=UTC\\nPGDATA_DEBUG=1\"\n  description = \"managed by the acceptance suite\""),
				Check:  resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
			},
			{
				// external_port is a deploy trigger too; setting it must
				// redeploy, converge, and persist server-side.
				Config: base("  env           = \"TZ=UTC\\nPGDATA_DEBUG=1\"\n  description   = \"managed by the acceptance suite\"\n  external_port = 55432"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "external_port", "55432"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.ExternalPort == nil || *pg.ExternalPort != 55432 {
							return fmt.Errorf("external_port not saved: %v", pg.ExternalPort)
						}
						return nil
					},
				),
			},
			{
				// Spec §5.6: every optional attribute must be clearable back
				// to null, not merely settable — and the clear has to reach
				// the SERVER, not just Terraform state. Each of these three
				// failed in a different way before this round of fixes:
				//   - external_port: fixed earlier (nil -> explicit JSON null).
				//   - description: `omitempty` dropped the key, and
				//     postgres.update reads an absent key as "keep the stored
				//     value" (verified live), so the old text came straight
				//     back on the next Read.
				//   - env: SavePostgresEnvironment took a `string`, so a null
				//     config value was sent as "" and stored verbatim; Read
				//     then reported "" against a null state forever.
				// ExpectEmptyPlan is what catches all three: each one leaves a
				// permanent non-empty plan.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "external_port"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "env"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.ExternalPort != nil {
							return fmt.Errorf("external_port not cleared server-side: %v", *pg.ExternalPort)
						}
						if pg.Description != nil && *pg.Description != "" {
							return fmt.Errorf("server still stores description %q; it was removed from config", *pg.Description)
						}
						if pg.Env != nil && *pg.Env != "" {
							return fmt.Errorf("server still stores env %q; it was removed from config", *pg.Env)
						}
						return nil
					},
				),
			},
			{
				// No ImportStateVerifyIgnore. deploy_on_change and
				// deployment_timeout are provider-only, so ImportState seeds
				// them with their schema defaults; ignoring them here used to
				// paper over an import that could never produce a clean plan.
				ResourceName:      "dokploy_postgres.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccPostgres_networkAttachment covers network_ids and
// detach_dokploy_network, the v0.30.0 network attachment fields shared by
// every database engine (kind.go's schemaAttributes, wired in
// resource.go/model.go by this task). Postgres stands in for all five
// engines here: the fields are engine-neutral (Object/UpdateSpec in kind.go),
// and every adapter maps them identically (postgres.go/mysql.go/mariadb.go/
// mongo.go/redis.go) - so this one acceptance run, plus the shared
// TestKindCredentialDescriptors-style unit coverage, is what the brief calls
// "engine symmetry... covered by the shared engine + adapters and the
// census." Mirrors TestAccApplication_networkAttachment
// (internal/resources/application/resource_acc_test.go) field for field:
// both fields must round-trip, and network_ids must clear back to null
// rather than an empty set - an explicit clear reads back as a literal JSON
// null, not `[]`, and flatten collapses both shapes to a null set
// (tfutil.StringSetOrNull).
func TestAccPostgres_networkAttachment(t *testing.T) {
	// resource.Test below only checks TF_ACC once its Steps start, but this
	// test calls createNetwork BEFORE that - a raw HTTP call of its own - so
	// it needs the same gate up front. Skipping (not failing) matches every
	// other acceptance test in this package and keeps `make test` green with
	// TF_ACC unset.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless env 'TF_ACC' set")
	}
	acctest.PreCheck(t)
	name := acctest.RandomName("pg-net")
	netName := acctest.RandomName("net")
	networkID := createNetwork(t, netName)
	t.Cleanup(func() { deleteNetwork(t, networkID) })

	base := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}
`, name+"-proj")

	withNetwork := base + fmt.Sprintf(`
resource "dokploy_postgres" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = "postgres:16-alpine"

  network_ids            = [%q]
  detach_dokploy_network = true

  deploy_on_change = false
}`, name, networkID)

	withoutNetwork := base + fmt.Sprintf(`
resource "dokploy_postgres" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = "postgres:16-alpine"

  deploy_on_change = false
}`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkPostgresDestroy,
		Steps: []resource.TestStep{
			{
				Config: withNetwork,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "network_ids.#", "1"),
					resource.TestCheckTypeSetElemAttr("dokploy_postgres.test", "network_ids.*", networkID),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "detach_dokploy_network", "true"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if len(pg.NetworkIDs) != 1 || pg.NetworkIDs[0] != networkID {
							return fmt.Errorf("server network_ids = %v, want [%s]", pg.NetworkIDs, networkID)
						}
						if !pg.DetachDokployNetwork {
							return fmt.Errorf("server detach_dokploy_network = false, want true")
						}
						return nil
					},
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Removed from config. network_ids must converge to null (not
				// an empty set) and detach_dokploy_network to its default,
				// false - matching what the server actually stores after an
				// explicit clear (doc.go: null, never []).
				Config: withoutNetwork,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "network_ids"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "detach_dokploy_network", "false"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if len(pg.NetworkIDs) != 0 {
							return fmt.Errorf("server network_ids = %v, want cleared", pg.NetworkIDs)
						}
						if pg.DetachDokployNetwork {
							return fmt.Errorf("server detach_dokploy_network = true, want its default false")
						}
						return nil
					},
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
