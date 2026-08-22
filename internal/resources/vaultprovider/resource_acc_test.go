// Package vaultprovider_test holds the acceptance tests (external package;
// acctest imports provider, which imports vaultprovider).
package vaultprovider_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func checkVaultProviderDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_vault_provider" {
			continue
		}
		if _, err := c.GetVaultProvider(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("vault provider %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

// assertNoVaultProviderNamed lists vault providers server-side and fails the
// test if one with the given name exists - used after an ExpectError step,
// where TestStep.Check never runs, to confirm a failed apply created
// nothing.
func assertNoVaultProviderNamed(t *testing.T, name string) {
	t.Helper()
	c, err := acctest.ClientFromEnv()
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	all, err := c.ListVaultProviders(context.Background())
	if err != nil {
		t.Fatalf("listing vault providers: %v", err)
	}
	for _, v := range all {
		if v.Name == name {
			t.Fatalf("vault provider %q exists after a failed apply", name)
		}
	}
}

// TestAccVaultProvider_hashicorpLifecycle runs the full lifecycle against a
// real dev vault (OpenBao, via acctest.StartRigVault): create with
// verify_connection = true, round-trip, rename (an in-place update, no
// replace), import, then the framework's own implicit destroy at the end of
// the test case. Every apply step carries an empty post-refresh plan.
func TestAccVaultProvider_hashicorpLifecycle(t *testing.T) {
	// resource.Test below only checks TF_ACC once its Steps start, but this
	// test calls acctest.StartRigVault BEFORE that - a docker exec of its
	// own against the rig - so it needs the same gate up front. Skipping
	// (not failing) matches the established pattern (see
	// application.TestAccApplication_networkAttachment) and keeps
	// `make test` green, and Docker untouched, with TF_ACC unset.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless env 'TF_ACC' set")
	}
	vaultURL, vaultToken := acctest.StartRigVault(t)
	name := acctest.RandomName("vault-hashi")

	cfg := func(providerName string) string {
		return fmt.Sprintf(`
resource "dokploy_vault_provider" "test" {
  name = %q

  hashicorp = {
    url   = %q
    token = %q
  }

  assignments = []

  verify_connection = true
}
`, providerName, vaultURL, vaultToken)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkVaultProviderDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "name", name),
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "hashicorp.url", vaultURL),
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "hashicorp.token", vaultToken),
					// mount was omitted from config; the server default
					// ("secret") must plan clean via the schema Default.
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "hashicorp.mount", "secret"),
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "assignments.#", "0"),
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "verify_connection", "true"),
					resource.TestCheckResourceAttrSet("dokploy_vault_provider.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_vault_provider.test", "created_at"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Rename only: update accepts this in place, no
				// RequiresReplace on any config block (internal/client/
				// doc.go, wave 6c "Update accepts a full type swap").
				// verify_connection stays true, so Update re-verifies
				// against the same real vault before writing.
				Config: cfg(name + "-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "name", name+"-renamed"),
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "hashicorp.mount", "secret"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("dokploy_vault_provider.test", plancheck.ResourceActionUpdate),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Gate R (REDACT): a masked secret can never be recovered
				// by import, so the config blocks and verify_connection
				// (provider-only, no server value) are excluded from
				// ImportStateVerify - the schema description documents
				// this as the expected shape, not a bug.
				ResourceName:            "dokploy_vault_provider.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"hashicorp.", "verify_connection"},
			},
		},
	})
}

// TestAccVaultProvider_verifyConnectionFails covers both failure shapes
// doc.go's wave 6c probes recorded for vaultProvider.testConnection: a wrong
// credential against the real dev vault (gate B PASS), and an unreachable
// URL, which needs no rig vault at all. Both must fail the apply with the
// server's message and leave no record behind.
func TestAccVaultProvider_verifyConnectionFails(t *testing.T) {
	t.Run("wrong_token", func(t *testing.T) {
		// Same up-front gate as TestAccVaultProvider_hashicorpLifecycle:
		// acctest.StartRigVault runs before resource.Test's own TF_ACC
		// check would fire.
		if os.Getenv("TF_ACC") == "" {
			t.Skip("Acceptance tests skipped unless env 'TF_ACC' set")
		}
		vaultURL, _ := acctest.StartRigVault(t)
		name := acctest.RandomName("vault-fail-token")
		cfg := fmt.Sprintf(`
resource "dokploy_vault_provider" "test" {
  name = %q

  hashicorp = {
    url   = %q
    token = "definitely-the-wrong-token"
  }

  assignments = []

  verify_connection = true
}
`, name, vaultURL)

		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { acctest.PreCheck(t) },
			ProtoV6ProviderFactories: acctest.ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:      cfg,
					ExpectError: regexp.MustCompile(`(?i)token validation failed`),
				},
			},
		})

		assertNoVaultProviderNamed(t, name)
	})

	t.Run("unreachable_url", func(t *testing.T) {
		name := acctest.RandomName("vault-fail-url")
		// .invalid is reserved (RFC 2606) and never resolves, so this case
		// needs no rig vault and is always available.
		cfg := fmt.Sprintf(`
resource "dokploy_vault_provider" "test" {
  name = %q

  hashicorp = {
    url   = "http://acc-vault-nonexistent.invalid:8200"
    token = "irrelevant"
  }

  assignments = []

  verify_connection = true
}
`, name)

		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { acctest.PreCheck(t) },
			ProtoV6ProviderFactories: acctest.ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:      cfg,
					ExpectError: regexp.MustCompile(`(?i)fetch failed`),
				},
			},
		})

		assertNoVaultProviderNamed(t, name)
	})
}

// TestAccVaultProvider_fakeCredLifecycle covers gate V PASS: doppler fake
// credentials with verify_connection = false, plus an assignments change
// (adding an environment id from a scratch dokploy_project /
// dokploy_environment pair) proving the assignments round-trip.
func TestAccVaultProvider_fakeCredLifecycle(t *testing.T) {
	name := acctest.RandomName("vault-doppler")
	projectName := acctest.RandomName("vault-doppler-proj")
	envName := acctest.RandomName("vault-doppler-env")

	cfg := func(includeEnv bool) string {
		envIDs := ""
		if includeEnv {
			envIDs = "\n    environment_ids = [dokploy_environment.extra.id]"
		}
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_environment" "extra" {
  project_id = dokploy_project.test.id
  name       = %q
}

resource "dokploy_vault_provider" "test" {
  name = %q

  doppler = {
    service_token = "dp.st.fake-acceptance-only"
  }

  assignments = [{
    project_id = dokploy_project.test.id%s
  }]

  verify_connection = false
}
`, projectName, envName, name, envIDs)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkVaultProviderDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "doppler.service_token", "dp.st.fake-acceptance-only"),
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "assignments.#", "1"),
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "assignments.0.environment_ids.#", "0"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: cfg(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "assignments.0.environment_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair("dokploy_vault_provider.test", "assignments.0.environment_ids.*", "dokploy_environment.extra", "id"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccVaultProvider_awsFakeCredLifecycle is the first live confirmation
// of the aws config shape - unlike its five siblings, it was never probed
// live in wave 6c (internal/client/vaultprovider.go's VaultAWSConfig doc
// comment). Fake credentials, a create-destroy round trip, asserting the
// non-secret fields.
func TestAccVaultProvider_awsFakeCredLifecycle(t *testing.T) {
	name := acctest.RandomName("vault-aws")
	cfg := fmt.Sprintf(`
resource "dokploy_vault_provider" "test" {
  name = %q

  aws = {
    region            = "us-east-1"
    access_key_id     = "AKIAACCEPTANCEONLY"
    secret_access_key = "acceptance-only-not-a-real-secret"
  }

  assignments = []

  verify_connection = false
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkVaultProviderDestroy,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "aws.region", "us-east-1"),
					resource.TestCheckResourceAttr("dokploy_vault_provider.test", "aws.access_key_id", "AKIAACCEPTANCEONLY"),
					resource.TestCheckNoResourceAttr("dokploy_vault_provider.test", "aws.endpoint"),
					resource.TestCheckResourceAttrSet("dokploy_vault_provider.test", "created_at"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccVaultProvider_duplicateName pins ruling 2 (the secret-leak defense)
// live: a second resource reusing the first's name must fail through the
// clean pre-check, not vaultProvider.create's raw HTTP 500 - which
// internal/client/doc.go's wave 6c "Duplicate names" section records as
// leaking the failed request's secret field in cleartext. The expected
// error text is matched exactly (not a looser pattern like "already
// exists" alone): Create's AddError call for this path is a static
// fmt.Sprintf with no secret in scope
// (internal/resources/vaultprovider/resource.go), so matching this precise
// string - rather than the raw-500 shape, which contains "INTERNAL_SERVER_
// ERROR" and the literal failed SQL insert - rules out the leaky path
// having run instead.
func TestAccVaultProvider_duplicateName(t *testing.T) {
	name := acctest.RandomName("vault-dup")
	first := fmt.Sprintf(`
resource "dokploy_vault_provider" "first" {
  name = %q

  doppler = {
    service_token = "dp.st.fake-dup-first"
  }

  assignments = []
}
`, name)
	both := first + fmt.Sprintf(`
resource "dokploy_vault_provider" "second" {
  name = %q

  doppler = {
    service_token = "dp.st.fake-dup-second-SECRET-MUST-NOT-LEAK"
  }

  assignments = []

  depends_on = [dokploy_vault_provider.first]
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkVaultProviderDestroy,
		Steps: []resource.TestStep{
			{
				Config: first,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: both,
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(fmt.Sprintf(`a vault provider named "%s" already exists`, name)),
				),
			},
		},
	})
}
