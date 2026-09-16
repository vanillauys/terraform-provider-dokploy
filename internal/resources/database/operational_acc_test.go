// The operational settings (#51): command, args, the four resource limits,
// replicas, and mongo's replica_sets. Proven once on postgres for the seven
// shared attributes, on the same reasoning as optional_computed_acc_test.go:
// kind.go's schemaAttributes defines them once for every Kind, model.go's
// applyOperational and flatten carry them once, and
// TestKindClient_NetworkMapping_Expand and _Flatten (kind_symmetry_test.go)
// prove every engine's adapter maps all seven in both directions. Only
// replica_sets is engine-specific, and it gets its own mongo test below.
package database_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// TestAccDatabase_operationalAttributes walks the seven shared attributes
// through their lifecycle on dokploy_postgres:
//
//   - a plan-time rejection of the shapes that cannot round-trip ("" on
//     the command, [] on args) or that Dokploy would misread ("512m",
//     "0.5");
//   - a create with all seven set and deploy_on_change = false: the create
//     endpoint accepts none of them, so this proves the follow-up update
//     lands them on the first apply (the same trap dokploy_libsql's test
//     covers). command and args are placeholders that no real deploy runs;
//   - an update that drops command/args/replicas and deploys with the four
//     limits in place, converging to status done;
//   - dropping every one of them: an in-place update, an empty plan after
//     refresh, replicas back at its default of 1, and every string null on
//     the server, not only in state;
//   - an import with a clean verify.
func TestAccDatabase_operationalAttributes(t *testing.T) {
	name := acctest.RandomName("pg-ops")
	base := func(optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_postgres" "test" {
  name              = %q
  environment_id    = dokploy_project.test.production_environment_id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = "postgres:16-alpine"
%s
}`, name+"-proj", name, optionals)
	}
	// Bytes and nano-CPUs: Dokploy hands parseInt(value) to swarm as-is
	// (doc.go, v1.3.0 records), so a "512m" would deploy as 512 bytes.
	limits := `
  cpu_limit          = "500000000"
  cpu_reservation    = "250000000"
  memory_limit       = "536870912"
  memory_reservation = "268435456"`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkPostgresDestroy,
		Steps: []resource.TestStep{
			{
				Config:      base("  command          = \"\"\n  deploy_on_change = false"),
				ExpectError: regexp.MustCompile(`(?s)command.*string length must be at least 1`),
			},
			{
				Config:      base("  args             = []\n  deploy_on_change = false"),
				ExpectError: regexp.MustCompile(`(?s)args.*list must contain at least 1 elements`),
			},
			{
				// The shape Docker users reach for first: Dokploy would
				// deploy it as 512 bytes and fail the 4 MiB minimum.
				Config:      base("  memory_limit     = \"512m\"\n  deploy_on_change = false"),
				ExpectError: regexp.MustCompile(`(?s)memory_limit.*whole number of bytes or nano-CPUs`),
			},
			{
				// A fraction parses as 0 and sets no limit at all.
				Config:      base("  cpu_limit        = \"0.5\"\n  deploy_on_change = false"),
				ExpectError: regexp.MustCompile(`(?s)cpu_limit.*whole number of bytes or nano-CPUs`),
			},
			{
				Config: base(`
  command          = "/bin/probe"
  args             = ["--acceptance", "--only"]
  replicas         = 2
  deploy_on_change = false` + limits),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "command", "/bin/probe"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "args.#", "2"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "args.0", "--acceptance"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "args.1", "--only"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "replicas", "2"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "cpu_limit", "500000000"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "cpu_reservation", "250000000"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "memory_limit", "536870912"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "memory_reservation", "268435456"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.Command == nil || *pg.Command != "/bin/probe" {
							return fmt.Errorf("command not saved on the first apply: %v", pg.Command)
						}
						if len(pg.Args) != 2 || pg.Args[0] != "--acceptance" || pg.Args[1] != "--only" {
							return fmt.Errorf("args not saved on the first apply: %v", pg.Args)
						}
						if pg.Replicas != 2 {
							return fmt.Errorf("replicas = %d, want 2", pg.Replicas)
						}
						if pg.CPULimit == nil || *pg.CPULimit != "500000000" || pg.CPUReservation == nil || *pg.CPUReservation != "250000000" ||
							pg.MemoryLimit == nil || *pg.MemoryLimit != "536870912" || pg.MemoryReservation == nil || *pg.MemoryReservation != "268435456" {
							return fmt.Errorf("limits not saved on the first apply: cpu %v/%v memory %v/%v",
								pg.CPULimit, pg.CPUReservation, pg.MemoryLimit, pg.MemoryReservation)
						}
						return nil
					},
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// The placeholders go; the limits stay and a real deploy
				// runs against them and converges.
				Config: base(limits),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_postgres.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "command"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "args"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "replicas", "1"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "memory_limit", "536870912"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.Command != nil {
							return fmt.Errorf("command not cleared server-side: %q", *pg.Command)
						}
						if len(pg.Args) != 0 {
							return fmt.Errorf("args not cleared server-side: %v", pg.Args)
						}
						if pg.Replicas != 1 {
							return fmt.Errorf("replicas = %d, want the default 1 back on the server", pg.Replicas)
						}
						if pg.MemoryLimit == nil || *pg.MemoryLimit != "536870912" {
							return fmt.Errorf("memory_limit lost across the update: %v", pg.MemoryLimit)
						}
						return nil
					},
				),
			},
			{
				// A changed limit is a deploy trigger: an in-place update
				// that redeploys and converges.
				Config: base(`
  cpu_limit          = "500000000"
  cpu_reservation    = "250000000"
  memory_limit       = "805306368"
  memory_reservation = "268435456"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_postgres.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "memory_limit", "805306368"),
				),
			},
			{
				// Everything dropped: each string reverts to a server-side
				// null, replicas to its default, and the plan is empty.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_postgres.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_postgres.test", "replicas", "1"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "cpu_limit"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "cpu_reservation"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "memory_limit"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "memory_reservation"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						for name, v := range map[string]*string{
							"cpu_limit": pg.CPULimit, "cpu_reservation": pg.CPUReservation,
							"memory_limit": pg.MemoryLimit, "memory_reservation": pg.MemoryReservation,
						} {
							if v != nil && *v != "" {
								return fmt.Errorf("server still stores %s %q; it was removed from config", name, *v)
							}
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "dokploy_postgres.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccMongo_replicaSets covers the mongo-only topology switch (#51):
// a create with replica_sets = true deploys and converges as a replica
// set; the switch back to false is an in-place update (never a replace)
// that redeploys and converges; dropping the attribute leaves the default
// false in place with an empty plan; and import verifies clean. doc.go's
// v1.3.0 records hold the live probe (both directions converge on mongo:7).
func TestAccMongo_replicaSets(t *testing.T) {
	name := acctest.RandomName("mongo-rs")
	base := func(optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_mongo" "test" {
  name              = %q
  environment_id    = dokploy_project.test.production_environment_id
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = "mongo:7"
%s
}`, name+"-proj", name, optionals)
	}
	checkServer := func(want bool) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			mo, err := getAccMongo(s)
			if err != nil {
				return err
			}
			if mo.ReplicaSets != want {
				return fmt.Errorf("server replicaSets = %v, want %v", mo.ReplicaSets, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMongoDestroy,
		Steps: []resource.TestStep{
			{
				Config: base("  replica_sets = true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mongo.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_mongo.test", "replica_sets", "true"),
					checkServer(true),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: base("  replica_sets = false"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_mongo.test", plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mongo.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_mongo.test", "replica_sets", "false"),
					checkServer(false),
				),
			},
			{
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_mongo.test", "replica_sets", "false"),
					checkServer(false),
				),
			},
			{
				ResourceName:      "dokploy_mongo.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccPostgres_upgradeFromV1_2 proves that a state written by v1.2.0,
// which has none of the operational attributes, loads under this schema
// with an empty plan: the refresh fills replicas from the server (1) and
// the six nullable ones as null, so the defaults never show as a diff and
// never start a redeploy on the first apply after the upgrade. The pattern
// is TestAccPostgres_upgradeFromV0_11's.
func TestAccPostgres_upgradeFromV1_2(t *testing.T) {
	acctest.SkipWithoutTerraformRegistry(t)
	name := acctest.RandomName("pg-up12")
	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_postgres" "test" {
  name              = %q
  environment_id    = dokploy_project.test.production_environment_id
  database_name     = "acc"
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = "postgres:16-alpine"
  deploy_on_change  = false
}`, name+"-proj", name)
	resource.Test(t, resource.TestCase{
		PreCheck:     func() { acctest.PreCheck(t) },
		CheckDestroy: checkPostgresDestroy,
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"dokploy": {Source: "vanillauys/dokploy", VersionConstraint: "1.2.0"},
				},
				Config: config,
				Check:  resource.TestCheckResourceAttrSet("dokploy_postgres.test", "id"),
			},
			{
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				Config:                   config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "replicas", "1"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "command"),
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "memory_limit"),
				),
			},
		},
	})
}
