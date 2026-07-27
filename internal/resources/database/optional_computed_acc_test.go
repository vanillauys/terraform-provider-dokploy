// Wave-2 task 9, carry item C1 (high priority — see the ledger's
// task-9-carry-list.toon C1_NUANCE block). No engine's lifecycle acceptance
// test ever sets app_name or server_id in config; both are only ever
// asserted with TestCheckResourceAttrSet. That leaves the wave's own stated
// constraint ("every optional attribute must provably revert when removed
// from config") proven only for description/env/external_port, never for
// the two Optional+Computed/Optional attributes it exists to protect.
//
// This file fixes that ONCE, against dokploy_postgres, not five times
// against every engine. kind.go's schemaAttributes defines app_name and
// server_id identically for every Kind (Optional+Computed+RequiresReplace+
// UseStateForUnknown, and Optional+RequiresReplace, respectively) — neither
// attribute's schema or plan-modifier wiring is parameterized per engine, so
// proving the property once on any one engine proves it for all five.
// Repeating the same acceptance-test scaffolding four more times would be
// pure duplication with zero additional coverage — exactly what
// acc_helpers_test.go's checkDestroy/getAccObject extraction already exists
// to avoid for this package.
//
// Both attributes carry RequiresReplace, and they revert differently:
//
//   - app_name is Optional+Computed+UseStateForUnknown. Probed live
//     (2026-07-27, v0.29.13) against the acceptance rig: postgres.create
//     with an explicit appName does NOT reject it, but does not store it
//     verbatim either — the server appends a random suffix
//     ("my-custom-app-name" came back as "my-custom-app-name-2czzou").
//     Setting it from Terraform config would therefore make apply fail with
//     "Provider produced inconsistent result after apply" (the planned,
//     config-supplied value can never equal what the server actually
//     stores) — a framework-level failure, not a meaningful assertion.
//     Per the ledger's instruction, this covers only the direction
//     UseStateForUnknown actually governs: a config that never mentions
//     app_name at all must still see it stay non-null and unchanged across
//     an ordinary update, and continue to do so once another optional
//     attribute is dropped from config entirely (proving the framework
//     never re-marks it unknown along the way).
//
//   - server_id is a plain Optional (no Computed) with RequiresReplace.
//     Probed live: postgres.create 401s ("You are not authorized to access
//     this server") on a serverId string that is not a genuine record
//     owned by the caller's organization, so a fabricated id cannot stand
//     in for a real one. server.create/sshKey.create, however, do NOT
//     validate SSH reachability at creation time (only an actual deploy
//     attempt needs a live host) — probed live by creating a server record
//     pointed at 192.0.2.1 (RFC 5737 TEST-NET-1, reserved and always
//     unreachable) and both a postgres.create and postgres.remove against
//     it returned normally. That is what newProbeServer below builds: a
//     real, org-owned server id good enough to satisfy the authorization
//     check, with every step using deploy_on_change = false so the
//     (deliberately unreachable) host is never actually dialed.
//
//     One more asymmetry surfaced building this: unlike every other create
//     endpoint this provider talks to, sshKey.create returns HTTP 200 with
//     a completely empty body (verified live, zero Content-Length) — there
//     is no sshKeyId to decode from its own response. newProbeServer below
//     recovers it with a follow-up sshKey.all, matched by the name it just
//     sent (RandomName's collision-resistant suffix makes that safe here).
package database_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"golang.org/x/crypto/ssh"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// TestAccDatabase_appNamePersistsAcrossUpdatesAndClears proves the
// UseStateForUnknown half of the generic engine's Optional+Computed
// contract for app_name (see this file's package comment for why it is
// never set in config here).
func TestAccDatabase_appNamePersistsAcrossUpdatesAndClears(t *testing.T) {
	name := acctest.RandomName("pg")
	var appName string

	base := func(description string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_postgres" "test" {
  name               = %q
  environment_id     = dokploy_project.test.environments[0].id
  database_name      = "acc"
  database_user      = "acc"
  database_password  = "acc-password-1"
  docker_image       = "postgres:16-alpine"
  deploy_on_change   = false
%s
}`, name+"-proj", name, description)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkPostgresDestroy,
		Steps: []resource.TestStep{
			{
				// Create: app_name is never in config, so the server
				// generates one. Capture it for the following steps.
				Config: base(`  description = "first"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_postgres.test", "app_name"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["dokploy_postgres.test"]
						if !ok {
							return fmt.Errorf("dokploy_postgres.test not found in state")
						}
						appName = rs.Primary.Attributes["app_name"]
						if appName == "" {
							return fmt.Errorf("app_name empty in state")
						}
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.AppName != appName {
							return fmt.Errorf("server app_name %q does not match state %q", pg.AppName, appName)
						}
						return nil
					},
				),
			},
			{
				// description changes; app_name is untouched by config on
				// every step (including this one) but must not be
				// disturbed by an unrelated in-place update. Asserting the
				// plan action is Update (not a replace) directly falsifies
				// the failure mode this attribute's RequiresReplace flag
				// makes possible: a spurious "changed" reading on app_name
				// that would force a replace nobody asked for.
				Config: base(`  description = "second"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("dokploy_postgres.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStateAttrUnchanged("dokploy_postgres.test", "app_name", &appName),
				),
			},
			{
				// description is dropped from config entirely — the actual
				// "optional attribute removed from config" transition.
				// ExpectEmptyPlan is the sharpest test available: if
				// UseStateForUnknown ever failed to pin app_name, the
				// framework would mark it "(known after apply)" on this
				// very refresh and the plan would NOT be empty.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "description"),
					checkStateAttrUnchanged("dokploy_postgres.test", "app_name", &appName),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.AppName != appName {
							return fmt.Errorf("server app_name changed from %q to %q; UseStateForUnknown must pin it", appName, pg.AppName)
						}
						return nil
					},
				),
			},
		},
	})
}

// checkStateAttrUnchanged asserts resourceAddr's attrName in state still
// equals *want (captured by an earlier step's Check). want is a pointer
// because the value itself is server-generated and unknown until the first
// step runs.
func checkStateAttrUnchanged(resourceAddr, attrName string, want *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return fmt.Errorf("%s not found in state", resourceAddr)
		}
		got := rs.Primary.Attributes[attrName]
		if got != *want {
			return fmt.Errorf("%s.%s = %q, want unchanged %q", resourceAddr, attrName, got, *want)
		}
		return nil
	}
}

// probeServer is a throwaway remote-server record — see this file's package
// comment for why it exists and why an unreachable IP is safe to use here.
type probeServer struct {
	serverID string
	sshKeyID string
}

// newProbeServer creates the ssh key + server record and registers their
// teardown via t.Cleanup, which runs after resource.Test's own final
// destroy (t.Cleanup callbacks always run after the test function body
// returns), so by the time these are removed nothing references them.
func newProbeServer(ctx context.Context, t *testing.T, c *client.Client) probeServer {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating throwaway ed25519 keypair: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshaling throwaway private key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("deriving throwaway public key: %v", err)
	}
	privatePEM := string(pem.EncodeToMemory(block))
	publicLine := string(ssh.MarshalAuthorizedKey(sshPub))

	var user struct {
		OrganizationID string `json:"organizationId"`
	}
	if err := c.Get(ctx, "/user.get", nil, &user); err != nil {
		t.Fatalf("user.get: %v", err)
	}

	// sshKey.create returns HTTP 200 with a completely empty body (verified
	// live, 2026-07-27) — there is no sshKeyId to decode here, unlike every
	// other create endpoint this provider talks to. Recover it with a
	// follow-up sshKey.all, matched by the name just sent; RandomName's
	// random suffix makes that match unambiguous.
	sshKeyName := acctest.RandomName("task9-c1-probe-key")
	if err := c.Post(ctx, "/sshKey.create", map[string]any{
		"name":           sshKeyName,
		"privateKey":     privatePEM,
		"publicKey":      publicLine,
		"organizationId": user.OrganizationID,
	}, nil); err != nil {
		t.Fatalf("sshKey.create: %v", err)
	}
	var allKeys []struct {
		SSHKeyID string `json:"sshKeyId"`
		Name     string `json:"name"`
	}
	if err := c.Get(ctx, "/sshKey.all", nil, &allKeys); err != nil {
		t.Fatalf("sshKey.all: %v", err)
	}
	var sshKeyID string
	for _, k := range allKeys {
		if k.Name == sshKeyName {
			sshKeyID = k.SSHKeyID
			break
		}
	}
	if sshKeyID == "" {
		t.Fatalf("sshKey.all: no key named %q found after sshKey.create", sshKeyName)
	}

	var server struct {
		ServerID string `json:"serverId"`
	}
	if err := c.Post(ctx, "/server.create", map[string]any{
		"name":        acctest.RandomName("task9-c1-probe-server"),
		"description": "wave-2 task 9 acceptance probe (C1): server_id revert-to-null coverage",
		// RFC 5737 TEST-NET-1: reserved, never routable. Every step using
		// this server sets deploy_on_change = false, so it is never dialed.
		"ipAddress":  "192.0.2.1",
		"port":       22,
		"username":   "root",
		"sshKeyId":   sshKeyID,
		"serverType": "deploy",
	}, &server); err != nil {
		t.Fatalf("server.create: %v", err)
	}

	p := probeServer{serverID: server.ServerID, sshKeyID: sshKeyID}
	t.Cleanup(func() {
		if err := c.Post(context.Background(), "/server.remove", map[string]string{"serverId": p.serverID}, nil); err != nil {
			t.Errorf("cleanup: server.remove: %v", err)
		}
		if err := c.Post(context.Background(), "/sshKey.remove", map[string]string{"sshKeyId": p.sshKeyID}, nil); err != nil {
			t.Errorf("cleanup: sshKey.remove: %v", err)
		}
	})
	return p
}

// TestAccDatabase_serverIDRevertsToNullOnRemoval proves the plain-Optional
// half of the ledger's C1 item: server_id, once set, reverts to a genuine
// server-side null when dropped from config — not merely from Terraform
// state — and getting there is a replace (RequiresReplace), never an
// in-place update.
func TestAccDatabase_serverIDRevertsToNullOnRemoval(t *testing.T) {
	// The probe server has to exist before Steps run (its id is baked into
	// the first step's Config), so this needs its own TF_ACC gate ahead of
	// resource.Test's own — matching resource.Test's message so an
	// unconfigured run skips exactly like every other acceptance test in
	// this package instead of failing on a missing DOKPLOY_ENDPOINT.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless env 'TF_ACC' set")
	}
	acctest.PreCheck(t)
	c, err := acctest.ClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	probe := newProbeServer(context.Background(), t, c)

	name := acctest.RandomName("pg")
	base := func(serverID string) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_postgres" "test" {
  name               = %q
  environment_id     = dokploy_project.test.environments[0].id
  database_name      = "acc"
  database_user      = "acc"
  database_password  = "acc-password-1"
  docker_image       = "postgres:16-alpine"
  deploy_on_change   = false
%s
}`, name+"-proj", name, serverID)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkPostgresDestroy,
		Steps: []resource.TestStep{
			{
				Config: base(fmt.Sprintf("  server_id = %q", probe.serverID)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_postgres.test", "server_id", probe.serverID),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.ServerID == nil || *pg.ServerID != probe.serverID {
							return fmt.Errorf("server-side server_id = %v, want %q", pg.ServerID, probe.serverID)
						}
						return nil
					},
				),
			},
			{
				// Dropping server_id forces a replace: it has no Computed/
				// Default, so removing it from config plans it back to
				// null, and RequiresReplace means that change can only
				// happen via destroy-then-create.
				Config: base(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("dokploy_postgres.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_postgres.test", "server_id"),
					func(s *terraform.State) error {
						pg, err := getAccPostgres(s)
						if err != nil {
							return err
						}
						if pg.ServerID != nil {
							return fmt.Errorf("server still stores server_id %q; it was removed from config", *pg.ServerID)
						}
						return nil
					},
				),
			},
		},
	})
}
