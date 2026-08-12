// Package libsql_test (an external test package, deliberately distinct from
// package libsql) holds the acceptance tests. It must live outside package
// libsql: acctest imports provider, and provider imports libsql to register
// dokploy_libsql - so an internal test file (package libsql) importing
// acctest here would form an import cycle (libsql -> acctest -> provider ->
// libsql), which the Go toolchain rejects with "import cycle not allowed in
// test". Keeping this file in the external libsql_test package sidesteps
// that. Mirrors internal/resources/database/mariadb_acc_test.go in the
// sibling database package.
//
// checkLibsqlDestroy and getLibsql below are this file's own copies of the
// compose_test package's checkComposeDestroy/getCompose shape, not a reuse
// of database_test's checkDestroy/getAccObject generics: those live in
// package database_test, and libsql_test is a different external test
// package, so they are not reachable from here.
package libsql_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// libsqlImage is the server's own bare-create default for dockerImage
// (internal/client/doc.go, "libsql, wave 5c": confirmed live by a create
// that omits dockerImage entirely) - a real, pinned, pullable tag. It is
// pinned explicitly in every deploying test below anyway, so those tests do
// not depend on whatever the server's bare default happens to be, mirroring
// every sibling engine's own acceptance tests.
const libsqlImage = "ghcr.io/tursodatabase/libsql-server:v0.24.32"

// checkLibsqlDestroy and getLibsql are this file's local equivalents of
// compose_test's checkComposeDestroy/getCompose.
func checkLibsqlDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_libsql" {
			continue
		}
		if _, err := c.GetLibsql(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("libsql %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// getLibsql re-reads a resource straight from the API (spec §7: verify
// server-side truth, not just Terraform's view of state). Every assertion
// in this file's new tests goes through this rather than through Terraform
// state.
func getLibsql(s *terraform.State, addr string) (*client.Libsql, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return nil, fmt.Errorf("%s not found in state", addr)
	}
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return nil, err
	}
	return c.GetLibsql(context.Background(), rs.Primary.ID)
}

func TestAccLibsqlReplicaRequiresPrimaryURL(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: `
resource "dokploy_libsql" "r" {
  name              = "replica-no-url"
  environment_id    = "env-placeholder"
  database_user     = "libsql"
  database_password = "pw"
  sqld_node         = "replica"
}`,
			ExpectError: regexp.MustCompile(`sqld_primary_url`),
		}},
	})
}

func TestAccLibsqlReplicaRejectsExternalPorts(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: `
resource "dokploy_libsql" "r" {
  name              = "replica-with-port"
  environment_id    = "env-placeholder"
  database_user     = "libsql"
  database_password = "pw"
  sqld_node         = "replica"
  sqld_primary_url  = "http://primary:8080"
  external_port     = 8080
}`,
			ExpectError: regexp.MustCompile(`(?s)external.*replica|replica.*external`),
		}},
	})
}

// TestAccLibsql_lifecycle is the full create/update/import/destroy pass,
// asserted through client.GetLibsql rather than Terraform state (spec §7).
// database_password is never printed: assertions on it use
// TestCheckResourceAttrSet (presence only) and a direct equality check that
// never formats either the wanted or the got value into an error message.
//
// The create step also sets command and cpu_limit - task 5's review round
// asked for this: libsql.create's request shape (CreateLibsqlRequest) has
// no JSON keys at all for command/cpu_limit/cpu_reservation/memory_limit/
// memory_reservation/replicas, so without a follow-up UpdateLibsql call
// those fields would be silently ignored on the FIRST apply. Task 5's
// Create issues that follow-up call unconditionally; this step is the proof
// it actually lands the fields, not just that a later apply would.
// deploy_on_change is false for that one step: command overrides the
// container's entrypoint, and an arbitrary value is not a working sqld
// invocation - a real deploy would fail the way compose's placeholder
// command does (internal/client/doc.go). The claim under test here is
// persistence via UpdateLibsql, not deploy success.
func TestAccLibsql_lifecycle(t *testing.T) {
	name := acctest.RandomName("libsql")

	base := func(optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = %q
%s
}`, name+"-proj", name, libsqlImage, optionals)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkLibsqlDestroy,
		Steps: []resource.TestStep{
			{
				// Create: also sets command and cpu_limit - see the doc
				// comment above. deploy_on_change = false so the
				// non-working command is never actually dialed.
				Config: base(`
  description        = "managed by the acceptance suite"
  env                = "TZ=UTC"
  command            = "/bin/sqld --help"
  cpu_limit          = "0.5"
  deploy_on_change   = false
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_libsql.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_libsql.test", "app_name"),
					resource.TestCheckResourceAttrSet("dokploy_libsql.test", "database_password"),
					resource.TestCheckResourceAttr("dokploy_libsql.test", "description", "managed by the acceptance suite"),
					func(s *terraform.State) error {
						ls, err := getLibsql(s, "dokploy_libsql.test")
						if err != nil {
							return err
						}
						if ls.Env == nil || *ls.Env != "TZ=UTC" {
							return fmt.Errorf("env not saved: %v", ls.Env)
						}
						if ls.Command == nil || *ls.Command != "/bin/sqld --help" {
							return fmt.Errorf("command not saved: %v", ls.Command)
						}
						if ls.CPULimit == nil || *ls.CPULimit != "0.5" {
							return fmt.Errorf("cpu_limit not saved: %v", ls.CPULimit)
						}
						// database_password: assert equality without ever
						// formatting the value (want or got) into the error.
						if ls.DatabasePassword != "acc-password-1" {
							return errors.New("database_password did not reach the server as configured")
						}
						return nil
					},
				),
			},
			{
				// Update: command/cpu_limit have done their job and are
				// dropped; env changes; deploy_on_change turns on. A real
				// deploy against the pinned, real, pullable default image
				// must succeed and converge to status done.
				Config: base(`
  description   = "managed by the acceptance suite"
  env           = "TZ=UTC\nLIBSQL_DEBUG=1"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_libsql.test", "status", "done"),
					func(s *terraform.State) error {
						ls, err := getLibsql(s, "dokploy_libsql.test")
						if err != nil {
							return err
						}
						if ls.Env == nil || *ls.Env != "TZ=UTC\nLIBSQL_DEBUG=1" {
							return fmt.Errorf("env not saved: %v", ls.Env)
						}
						if ls.Command != nil {
							return fmt.Errorf("command not cleared server-side: %v", *ls.Command)
						}
						if ls.CPULimit != nil {
							return fmt.Errorf("cpu_limit not cleared server-side: %v", *ls.CPULimit)
						}
						return nil
					},
				),
			},
			{
				// external_port is a deploy trigger too; setting it must
				// redeploy, converge, and persist server-side.
				Config: base(`
  description   = "managed by the acceptance suite"
  env           = "TZ=UTC\nLIBSQL_DEBUG=1"
  external_port = 58080
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_libsql.test", "status", "done"),
					resource.TestCheckResourceAttr("dokploy_libsql.test", "external_port", "58080"),
					func(s *terraform.State) error {
						ls, err := getLibsql(s, "dokploy_libsql.test")
						if err != nil {
							return err
						}
						if ls.ExternalPort == nil || *ls.ExternalPort != 58080 {
							return fmt.Errorf("external_port not saved: %v", ls.ExternalPort)
						}
						return nil
					},
				),
			},
			{
				// Spec §5.6: every optional attribute must be clearable
				// back to null, and the clear has to reach the SERVER, not
				// just Terraform state.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_libsql.test", "status", "done"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "external_port"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "env"),
					func(s *terraform.State) error {
						ls, err := getLibsql(s, "dokploy_libsql.test")
						if err != nil {
							return err
						}
						if ls.ExternalPort != nil {
							return fmt.Errorf("external_port not cleared server-side: %v", *ls.ExternalPort)
						}
						if ls.Description != nil && *ls.Description != "" {
							return fmt.Errorf("server still stores description %q; it was removed from config", *ls.Description)
						}
						if ls.Env != nil && *ls.Env != "" {
							return fmt.Errorf("server still stores env %q; it was removed from config", *ls.Env)
						}
						return nil
					},
				),
			},
			{
				// No ImportStateVerifyIgnore. deploy_on_change and
				// deployment_timeout are provider-only, so ImportState
				// seeds them with their schema defaults; every step above
				// left them at those same defaults (true / "15m"), which is
				// what makes an ignore-free import clean.
				ResourceName:      "dokploy_libsql.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccLibsql_portMatrix is the dedicated proof of dialect B on
// libsql.saveExternalPorts (internal/client/libsql.go's SaveLibsqlExternalPorts):
// an omitted key means "keep the stored value", never "clear it". Every
// step asserts through a direct API read. deploy_on_change is false
// throughout: ports are saved through their own endpoint, independent of
// deploy, so the property under test needs no deploy at all.
func TestAccLibsql_portMatrix(t *testing.T) {
	name := acctest.RandomName("libsqlport")

	base := func(optionals string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = %q
  deploy_on_change  = false
%s
}`, name+"-proj", name, libsqlImage, optionals)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkLibsqlDestroy,
		Steps: []resource.TestStep{
			{
				// 1. Set all three ports.
				Config: base(`
  external_port       = 58101
  external_admin_port = 58102
  external_grpc_port  = 58103
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: func(s *terraform.State) error {
					ls, err := getLibsql(s, "dokploy_libsql.test")
					if err != nil {
						return err
					}
					if ls.ExternalPort == nil || *ls.ExternalPort != 58101 {
						return fmt.Errorf("external_port = %v, want 58101", ls.ExternalPort)
					}
					if ls.ExternalAdminPort == nil || *ls.ExternalAdminPort != 58102 {
						return fmt.Errorf("external_admin_port = %v, want 58102", ls.ExternalAdminPort)
					}
					if ls.ExternalGRPCPort == nil || *ls.ExternalGRPCPort != 58103 {
						return fmt.Errorf("external_grpc_port = %v, want 58103", ls.ExternalGRPCPort)
					}
					return nil
				},
			},
			{
				// 2. Change ONLY external_port. This is the dialect-B
				// proof: the regression it catches is an omitted key read
				// as "clear", which would zero the two untouched ports
				// rather than leave them alone.
				Config: base(`
  external_port       = 58201
  external_admin_port = 58102
  external_grpc_port  = 58103
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: func(s *terraform.State) error {
					ls, err := getLibsql(s, "dokploy_libsql.test")
					if err != nil {
						return err
					}
					if ls.ExternalPort == nil || *ls.ExternalPort != 58201 {
						return fmt.Errorf("external_port = %v, want 58201", ls.ExternalPort)
					}
					if ls.ExternalAdminPort == nil || *ls.ExternalAdminPort != 58102 {
						return fmt.Errorf("external_admin_port changed to %v, want unchanged 58102", ls.ExternalAdminPort)
					}
					if ls.ExternalGRPCPort == nil || *ls.ExternalGRPCPort != 58103 {
						return fmt.Errorf("external_grpc_port changed to %v, want unchanged 58103", ls.ExternalGRPCPort)
					}
					return nil
				},
			},
			{
				// 3. Clear ONLY external_admin_port. The other two survive.
				Config: base(`
  external_port      = 58201
  external_grpc_port = 58103
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "external_admin_port"),
					func(s *terraform.State) error {
						ls, err := getLibsql(s, "dokploy_libsql.test")
						if err != nil {
							return err
						}
						if ls.ExternalAdminPort != nil {
							return fmt.Errorf("external_admin_port not cleared server-side: %v", *ls.ExternalAdminPort)
						}
						if ls.ExternalPort == nil || *ls.ExternalPort != 58201 {
							return fmt.Errorf("external_port changed to %v, want unchanged 58201", ls.ExternalPort)
						}
						if ls.ExternalGRPCPort == nil || *ls.ExternalGRPCPort != 58103 {
							return fmt.Errorf("external_grpc_port changed to %v, want unchanged 58103", ls.ExternalGRPCPort)
						}
						return nil
					},
				),
			},
			{
				// 4. Clear all three in one apply.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "external_port"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "external_admin_port"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "external_grpc_port"),
					func(s *terraform.State) error {
						ls, err := getLibsql(s, "dokploy_libsql.test")
						if err != nil {
							return err
						}
						if ls.ExternalPort != nil || ls.ExternalAdminPort != nil || ls.ExternalGRPCPort != nil {
							return fmt.Errorf("ports not fully cleared server-side: port=%v admin=%v grpc=%v",
								ls.ExternalPort, ls.ExternalAdminPort, ls.ExternalGRPCPort)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccLibsql_replicaTransitionClearsPorts is task 5's review round's
// second addition: prove, live, that a primary-with-a-port flipping to
// replica-with-a-primary-url in ONE apply actually converges. Task 5's
// Update clears external ports BEFORE UpdateLibsql flips sqldNode (the
// becomingReplica guard in resource.go) precisely because a replica rejects
// every libsql.saveExternalPorts call outright, regardless of payload
// (internal/client/doc.go, "libsql, wave 5c"): if the flip landed first,
// the follow-up call meant to clear the old port would 400, and the apply
// could never converge - the user would be stuck destroying and
// recreating. This test is that ordering, live, not just read from the
// comment.
//
// deploy_on_change is false for every step: a replica cannot actually
// deploy without a genuine, reachable primary sqld instance, which this
// test does not stand up (per this task's brief: "if the deploy step fails
// for that reason, set deploy_on_change = false"). The property under test
// is the clear-before-flip ordering, provable entirely through direct API
// reads and an empty post-apply plan, with no live replication topology
// required.
//
// A third step, added for task 6's open review finding, then drops
// sqld_node and sqld_primary_url entirely - not by setting sqld_node back
// to "primary" explicitly, which would never exercise the schema's Default
// plan modifier at all. Dropping it is what proves the Default
// (stringdefault.StaticString("primary"), resource.go) actually resets a
// prior NON-default value, closing the one gap task 6's report flagged: no
// test had set sqld_node away from its default and then dropped it.
func TestAccLibsql_replicaTransitionClearsPorts(t *testing.T) {
	name := acctest.RandomName("libsqlrepl")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkLibsqlDestroy,
		Steps: []resource.TestStep{
			{
				// primary with a real external port set.
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = %q
  sqld_node         = "primary"
  external_port     = 58301
  deploy_on_change  = false
}`, name+"-proj", name, libsqlImage),
				Check: func(s *terraform.State) error {
					ls, err := getLibsql(s, "dokploy_libsql.test")
					if err != nil {
						return err
					}
					if ls.SqldNode != "primary" {
						return fmt.Errorf("sqldNode = %q, want primary", ls.SqldNode)
					}
					if ls.ExternalPort == nil || *ls.ExternalPort != 58301 {
						return fmt.Errorf("external_port = %v, want 58301", ls.ExternalPort)
					}
					return nil
				},
			},
			{
				// ONE apply: flip to replica, supply sqld_primary_url, and
				// drop the port - all at once.
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = %q
  sqld_node         = "replica"
  sqld_primary_url  = "http://primary.internal:8080"
  deploy_on_change  = false
}`, name+"-proj", name, libsqlImage),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_libsql.test", "sqld_node", "replica"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "external_port"),
					func(s *terraform.State) error {
						ls, err := getLibsql(s, "dokploy_libsql.test")
						if err != nil {
							return err
						}
						if ls.SqldNode != "replica" {
							return fmt.Errorf("sqldNode = %q, want replica", ls.SqldNode)
						}
						if ls.SqldPrimaryURL == nil || *ls.SqldPrimaryURL != "http://primary.internal:8080" {
							return fmt.Errorf("sqldPrimaryUrl = %v, want http://primary.internal:8080", ls.SqldPrimaryURL)
						}
						if ls.ExternalPort != nil || ls.ExternalAdminPort != nil || ls.ExternalGRPCPort != nil {
							return fmt.Errorf("ports not cleared on the replica flip: port=%v admin=%v grpc=%v",
								ls.ExternalPort, ls.ExternalAdminPort, ls.ExternalGRPCPort)
						}
						return nil
					},
				),
			},
			{
				// Drop sqld_node and sqld_primary_url entirely. This closes
				// the review finding from task 6's report: no earlier test
				// set sqld_node away from its default and then dropped it, so
				// nothing proved the Default plan modifier
				// (stringdefault.StaticString("primary"), resource.go) really
				// resets a NON-default prior value, as opposed to sqld_node
				// simply having never been changed. deploy_on_change stays
				// false throughout this test: a replica reverting to a bare
				// primary would attempt a real deploy otherwise, which is not
				// the property under test here.
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "test" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc"
  database_password = "acc-password-1"
  docker_image      = %q
  deploy_on_change  = false
}`, name+"-proj", name, libsqlImage),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_libsql.test", "sqld_node", "primary"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "sqld_primary_url"),
					func(s *terraform.State) error {
						ls, err := getLibsql(s, "dokploy_libsql.test")
						if err != nil {
							return err
						}
						if ls.SqldNode != "primary" {
							return fmt.Errorf("sqldNode = %q, want primary; the Default plan modifier did not reset it", ls.SqldNode)
						}
						if ls.SqldPrimaryURL != nil {
							return fmt.Errorf("sqldPrimaryUrl = %v, want cleared", *ls.SqldPrimaryURL)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccLibsql_optionalAttributesRevert drops every optional attribute
// from config in one step and asserts plancheck.ExpectEmptyPlan(): a plain
// Optional reverts to null, and an Optional+Computed attribute
// (docker_image, sqld_node, enable_namespaces, replicas) reverts to a
// non-null value, never null. app_name is checked here too, but as a
// stability proof, not a revert - see below for why.
//
// docker_image is one exception worth flagging: unlike sqld_node/
// enable_namespaces/replicas, it carries no schema Default - only
// UseStateForUnknown (internal/resources/libsql/resource.go). A Default is
// re-applied on every plan where config is null, regardless of prior state,
// so sqld_node/enable_namespaces/replicas truly reset to their canonical
// default (primary/false/1) even if a previous step configured something
// else. UseStateForUnknown alone does not: it just keeps whatever value is
// already in state. So docker_image "reverts" only in the weaker sense of
// "never goes null, and the plan stays empty" - dropping it here does NOT
// reset it to the server's bare ghcr.io default if a prior step set
// something else; it stays at that something else. This is a real
// behavioral difference the brief's single phrase "revert to its default"
// does not distinguish, and this test's docker_image assertion is written
// for the actual (state-carried) contract rather than the shorthand phrase.
//
// app_name is a different kind of exception: after the appName blocker fix
// (task-6-report.md, "appName blocker fix"), it is Computed-only, not
// Optional+Computed - libsql.create requires a real, non-empty appName on
// every call, and the server always appends its own random suffix to
// whatever it receives, so a config-supplied literal could never converge.
// This test never sets it in config - it cannot be set at all now - and
// only checks that the value already in state survives the revert step
// unchanged, which UseStateForUnknown guarantees.
//
// sqld_primary_url is deliberately absent from the set of plain-Optional
// attributes this test sets and reverts: verified live (v0.29.13,
// 2026-08-12), libsql.create 400s with "sqldPrimaryUrl should not be
// provided when sqldNode is not 'replica'" the moment a non-null
// sqldPrimaryUrl reaches the server while sqldNode stays at its "primary"
// default - which is exactly the config this step would otherwise write.
// See internal/client/doc.go's "libsql, wave 5c" section for the probe.
// sqld_primary_url's set-then-clear path is covered instead by
// TestAccLibsql_replicaTransitionClearsPorts, on a real replica where the
// server actually accepts it.
//
// deploy_on_change is false throughout: the claim under test is which
// values persist or clear server-side, not deploy behavior, and the
// command/docker_image values used here are not meant to be deployable.
func TestAccLibsql_optionalAttributesRevert(t *testing.T) {
	name := acctest.RandomName("libsqlopt")
	const customImage = "ghcr.io/tursodatabase/libsql-server:v0.24.32-acc-probe"
	var appName string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkLibsqlDestroy,
		Steps: []resource.TestStep{
			{
				// Every optional attribute set to a non-default value.
				// app_name is never set here - it is Computed-only, so
				// config cannot set it at all - matching the house rule
				// (internal/resources/database/
				// optional_computed_acc_test.go's package comment): no
				// engine's acceptance test sets it in config, since the
				// server is free to alter a caller-supplied value.
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "test" {
  name               = %q
  environment_id     = dokploy_project.test.environments[0].id
  database_user      = "acc"
  database_password  = "acc-password-1"
  deploy_on_change    = false

  docker_image        = %q
  description         = "set"
  env                 = "KEY=value"
  command             = "/bin/probe"
  cpu_limit           = "0.5"
  cpu_reservation     = "0.25"
  memory_limit        = "512m"
  memory_reservation  = "256m"
  enable_namespaces   = true
  replicas            = 2
}`, name+"-proj", name, customImage),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_libsql.test", "app_name"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["dokploy_libsql.test"]
						if !ok {
							return fmt.Errorf("dokploy_libsql.test not found in state")
						}
						appName = rs.Primary.Attributes["app_name"]
						if appName == "" {
							return fmt.Errorf("app_name empty in state")
						}
						return nil
					},
				),
			},
			{
				// Drop every optional attribute. The plan after apply and
				// refresh must be empty, which is what proves each one
				// actually reverted server-side, not merely in state.
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "test" {
  name               = %q
  environment_id     = dokploy_project.test.environments[0].id
  database_user      = "acc"
  database_password  = "acc-password-1"
  deploy_on_change    = false
}`, name+"-proj", name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "description"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "env"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "command"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "cpu_limit"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "cpu_reservation"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "memory_limit"),
					resource.TestCheckNoResourceAttr("dokploy_libsql.test", "memory_reservation"),
					// Optional+Computed WITH a schema Default: these three
					// reset to their canonical default on every plan where
					// config is null, regardless of what was configured
					// before.
					resource.TestCheckResourceAttr("dokploy_libsql.test", "sqld_node", "primary"),
					resource.TestCheckResourceAttr("dokploy_libsql.test", "enable_namespaces", "false"),
					resource.TestCheckResourceAttr("dokploy_libsql.test", "replicas", "1"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["dokploy_libsql.test"]
						if !ok {
							return fmt.Errorf("dokploy_libsql.test not found in state")
						}
						if rs.Primary.Attributes["app_name"] != appName {
							return fmt.Errorf("app_name changed from %q to %q; UseStateForUnknown must pin it",
								appName, rs.Primary.Attributes["app_name"])
						}

						ls, err := getLibsql(s, "dokploy_libsql.test")
						if err != nil {
							return err
						}
						if ls.Description != nil && *ls.Description != "" {
							return fmt.Errorf("description = %v, want cleared", *ls.Description)
						}
						if ls.Env != nil && *ls.Env != "" {
							return fmt.Errorf("env = %v, want cleared", *ls.Env)
						}
						if ls.Command != nil {
							return fmt.Errorf("command = %v, want cleared", *ls.Command)
						}
						if ls.CPULimit != nil {
							return fmt.Errorf("cpuLimit = %v, want cleared", *ls.CPULimit)
						}
						if ls.CPUReservation != nil {
							return fmt.Errorf("cpuReservation = %v, want cleared", *ls.CPUReservation)
						}
						if ls.MemoryLimit != nil {
							return fmt.Errorf("memoryLimit = %v, want cleared", *ls.MemoryLimit)
						}
						if ls.MemoryReservation != nil {
							return fmt.Errorf("memoryReservation = %v, want cleared", *ls.MemoryReservation)
						}
						// docker_image: state-carried, not reset - see this
						// test's doc comment.
						if ls.DockerImage != customImage {
							return fmt.Errorf("dockerImage = %q, want it to stay %q (state-carried via UseStateForUnknown, not reset)",
								ls.DockerImage, customImage)
						}
						if ls.SqldNode != "primary" {
							return fmt.Errorf("sqldNode = %q, want the primary default", ls.SqldNode)
						}
						if ls.EnableNamespaces {
							return errors.New("enableNamespaces = true, want it reset to the false default")
						}
						if ls.Replicas != 1 {
							return fmt.Errorf("replicas = %d, want it reset to the 1 default", ls.Replicas)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccLibsql_createAndLocate proves the id Terraform ends up holding is
// the id of the record actually created - the acceptance-level analog of
// TestCreateRedirectLocatesTheNewID (internal/client/appchild_test.go).
// libsql.create returns literal `true`, not the record, so CreateLibsql
// locates the new id by diffing the environment's libsql slice around the
// call (createAndLocate, internal/client/appchild.go), under a lock keyed
// on EnvironmentID.
//
// Both resources below share one environment and have no dependency on
// each other, so Terraform is free to apply them concurrently - which
// means this genuinely exercises the lock and the diff-and-locate path
// against the real rig, not just a single create with nothing to
// disambiguate against. If createAndLocate ever crossed the two, one
// resource's id would resolve (via GetLibsql) to the OTHER resource's
// databaseUser.
func TestAccLibsql_createAndLocate(t *testing.T) {
	projectName := acctest.RandomName("libsqlloc-proj")
	nameA := acctest.RandomName("libsqlloc-a")
	nameB := acctest.RandomName("libsqlloc-b")

	cfg := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_libsql" "a" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc-a"
  database_password = "acc-password-1"
  deploy_on_change  = false
}

resource "dokploy_libsql" "b" {
  name              = %q
  environment_id    = dokploy_project.test.environments[0].id
  database_user     = "acc-b"
  database_password = "acc-password-1"
  deploy_on_change  = false
}`, projectName, nameA, nameB)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkLibsqlDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_libsql.a", "id"),
					resource.TestCheckResourceAttrSet("dokploy_libsql.b", "id"),
					func(s *terraform.State) error {
						rsA, ok := s.RootModule().Resources["dokploy_libsql.a"]
						if !ok {
							return fmt.Errorf("dokploy_libsql.a not found in state")
						}
						rsB, ok := s.RootModule().Resources["dokploy_libsql.b"]
						if !ok {
							return fmt.Errorf("dokploy_libsql.b not found in state")
						}
						if rsA.Primary.ID == rsB.Primary.ID {
							return fmt.Errorf("dokploy_libsql.a and .b located the SAME id %q; createAndLocate crossed them", rsA.Primary.ID)
						}

						lsA, err := getLibsql(s, "dokploy_libsql.a")
						if err != nil {
							return err
						}
						if lsA.DatabaseUser != "acc-a" {
							return fmt.Errorf("dokploy_libsql.a's located id %q resolves to databaseUser %q, want acc-a - createAndLocate picked the wrong record",
								rsA.Primary.ID, lsA.DatabaseUser)
						}

						lsB, err := getLibsql(s, "dokploy_libsql.b")
						if err != nil {
							return err
						}
						if lsB.DatabaseUser != "acc-b" {
							return fmt.Errorf("dokploy_libsql.b's located id %q resolves to databaseUser %q, want acc-b - createAndLocate picked the wrong record",
								rsB.Primary.ID, lsB.DatabaseUser)
						}
						return nil
					},
				),
			},
		},
	})
}
