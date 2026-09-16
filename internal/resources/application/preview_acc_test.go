package application_test

import (
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// TestAccApplication_previewRollbackAndBuildSettings covers the v1.4.0
// (#52) attributes: title, subtitle, the preview_deployments and rollback
// blocks, and the four build settings on application.update. Every step
// asserts the server with a direct read, because application.update is
// dialect B: a field that the resource forgets to send keeps its stored
// value and Terraform state alone would not show it. The registry on the
// rig serves as the rollback and build registry.
func TestAccApplication_previewRollbackAndBuildSettings(t *testing.T) {
	registryURL := acctest.StartRigRegistry(t)
	name := acctest.RandomName("app-preview")
	base := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_registry" "test" {
  name     = %q
  url      = %q
  username = "acceptance"
  password = "acceptance-only"
}
`, name+"-proj", name, registryURL)

	// deploy is false for every step except the last: that one ends in an
	// import step, and import cannot see configuration, so it can only seed
	// deploy_on_change and deployment_timeout with their schema defaults
	// (tfutil.ImportDeployDefaults). The last apply therefore deploys.
	app := func(deploy bool, body string) string {
		deployLine := "  deploy_on_change = false\n"
		if deploy {
			deployLine = ""
		}
		return base + fmt.Sprintf(`
resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
  docker         = { image = "traefik/whoami:v1.10" }
%s%s
}`, name, deployLine, body)
	}
	full := app(false, `
  title    = "Preview app"
  subtitle = "Acceptance"

  preview_deployments = {
    enabled                          = true
    env                              = "PREVIEW=1"
    build_args                       = "ARG=1"
    build_secrets                    = "SECRET=1"
    certificate_type                 = "letsencrypt"
    custom_cert_resolver             = "resolver"
    https                            = true
    labels                           = ["traefik.enable=true"]
    limit                            = 5
    path                             = "/p"
    port                             = 4321
    require_collaborator_permissions = false
    wildcard                         = "*.preview.example.com"
  }

  rollback = {
    enabled     = true
    registry_id = dokploy_registry.test.id
  }

  build_registry_id = dokploy_registry.test.id
  clean_cache       = true
  drop_build_path   = "/drop"
`)
	changed := app(false, `
  title = "Preview app 2"

  preview_deployments = {
    enabled = true
    limit   = 2
    port    = 8080
  }

  rollback = {
    enabled = true
  }

  clean_cache = true
`)
	minimal := app(true, "")

	str := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return *p
	}
	emptyPlan := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				// Create sends the settings in the follow-up update
				// (application.create does not accept them).
				Config: full,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "title", "Preview app"),
					resource.TestCheckResourceAttr("dokploy_application.test", "preview_deployments.enabled", "true"),
					resource.TestCheckResourceAttr("dokploy_application.test", "preview_deployments.limit", "5"),
					resource.TestCheckResourceAttr("dokploy_application.test", "preview_deployments.build_secrets", "SECRET=1"),
					resource.TestCheckResourceAttr("dokploy_application.test", "preview_deployments.labels.#", "1"),
					resource.TestCheckResourceAttr("dokploy_application.test", "rollback.enabled", "true"),
					resource.TestCheckResourceAttrPair("dokploy_application.test", "rollback.registry_id", "dokploy_registry.test", "id"),
					resource.TestCheckResourceAttrPair("dokploy_application.test", "build_registry_id", "dokploy_registry.test", "id"),
					resource.TestCheckResourceAttr("dokploy_application.test", "clean_cache", "true"),
					fetchApplication(func(a *client.Application) error {
						for field, got := range map[string]struct{ have, want any }{
							"title":                                 {str(a.Title), "Preview app"},
							"subtitle":                              {str(a.Subtitle), "Acceptance"},
							"isPreviewDeploymentsActive":            {a.IsPreviewDeploymentsActive, true},
							"previewEnv":                            {str(a.PreviewEnv), "PREVIEW=1"},
							"previewBuildArgs":                      {str(a.PreviewBuildArgs), "ARG=1"},
							"previewBuildSecrets":                   {str(a.PreviewBuildSecrets), "SECRET=1"},
							"previewCertificateType":                {a.PreviewCertificateType, "letsencrypt"},
							"previewCustomCertResolver":             {str(a.PreviewCustomCertResolver), "resolver"},
							"previewHttps":                          {a.PreviewHTTPS, true},
							"previewLabels":                         {fmt.Sprint(a.PreviewLabels), "[traefik.enable=true]"},
							"previewLimit":                          {a.PreviewLimit, int64(5)},
							"previewPath":                           {a.PreviewPath, "/p"},
							"previewPort":                           {a.PreviewPort, int64(4321)},
							"previewRequireCollaboratorPermissions": {a.PreviewRequireCollaboratorPermissions, false},
							"previewWildcard":                       {str(a.PreviewWildcard), "*.preview.example.com"},
							"rollbackActive":                        {a.RollbackActive, true},
							"cleanCache":                            {a.CleanCache, true},
							"dropBuildPath":                         {str(a.DropBuildPath), "/drop"},
						} {
							if got.have != got.want {
								return fmt.Errorf("server %s = %v, want %v", field, got.have, got.want)
							}
						}
						if a.RollbackRegistryID == nil || a.BuildRegistryID == nil || *a.RollbackRegistryID != *a.BuildRegistryID {
							return fmt.Errorf("server rollbackRegistryId = %v, buildRegistryId = %v, want the fixture registry on both", a.RollbackRegistryID, a.BuildRegistryID)
						}
						return nil
					}),
				),
				ConfigPlanChecks: emptyPlan,
			},
			{
				// A partial block: the omitted nested attributes revert to
				// their defaults, the omitted plain ones to null, and the
				// dropped rollback registry and build registry clear.
				Config: changed,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_application.test", "preview_deployments.limit", "2"),
					resource.TestCheckResourceAttr("dokploy_application.test", "preview_deployments.certificate_type", "none"),
					resource.TestCheckNoResourceAttr("dokploy_application.test", "preview_deployments.wildcard"),
					resource.TestCheckNoResourceAttr("dokploy_application.test", "subtitle"),
					resource.TestCheckNoResourceAttr("dokploy_application.test", "rollback.registry_id"),
					fetchApplication(func(a *client.Application) error {
						if a.PreviewLimit != 2 || a.PreviewPort != 8080 || a.PreviewCertificateType != "none" || a.PreviewPath != "/" {
							return fmt.Errorf("server preview = limit %d port %d cert %q path %q", a.PreviewLimit, a.PreviewPort, a.PreviewCertificateType, a.PreviewPath)
						}
						if !a.PreviewRequireCollaboratorPermissions || a.PreviewHTTPS {
							return errors.New("server preview bools did not revert to their defaults")
						}
						for field, v := range map[string]*string{
							"subtitle": a.Subtitle, "previewEnv": a.PreviewEnv, "previewBuildSecrets": a.PreviewBuildSecrets,
							"previewWildcard": a.PreviewWildcard, "rollbackRegistryId": a.RollbackRegistryID,
							"buildRegistryId": a.BuildRegistryID, "dropBuildPath": a.DropBuildPath,
						} {
							if v != nil && *v != "" {
								return fmt.Errorf("server %s = %q, want cleared", field, *v)
							}
						}
						if len(a.PreviewLabels) != 0 || !a.RollbackActive || !a.CleanCache {
							return fmt.Errorf("server labels %v rollbackActive %v cleanCache %v", a.PreviewLabels, a.RollbackActive, a.CleanCache)
						}
						return nil
					}),
				),
				ConfigPlanChecks: emptyPlan,
			},
			{
				// Everything removed: both blocks read back null, and the
				// server holds the defaults of a fresh record.
				Config: minimal,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("dokploy_application.test", "title"),
					resource.TestCheckNoResourceAttr("dokploy_application.test", "preview_deployments"),
					resource.TestCheckNoResourceAttr("dokploy_application.test", "rollback"),
					resource.TestCheckResourceAttr("dokploy_application.test", "clean_cache", "false"),
					fetchApplication(func(a *client.Application) error {
						if a.IsPreviewDeploymentsActive || a.PreviewLimit != 3 || a.PreviewPort != 3000 || a.RollbackActive || a.CleanCache {
							return fmt.Errorf("server did not revert to the defaults: active %v limit %d port %d rollback %v cleanCache %v",
								a.IsPreviewDeploymentsActive, a.PreviewLimit, a.PreviewPort, a.RollbackActive, a.CleanCache)
						}
						return nil
					}),
				),
				ConfigPlanChecks: emptyPlan,
			},
			{
				ResourceName:      "dokploy_application.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// The write-only form of the preview build secret: the server holds the
// value, the state holds null, and a version bump sends a new value.
func TestAccApplication_previewBuildSecretsWriteOnly(t *testing.T) {
	name := acctest.RandomName("app-preview-wo")
	cfg := func(secret string, version int) string {
		return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name             = %q
  environment_id   = dokploy_project.test.environments[0].id
  docker           = { image = "traefik/whoami:v1.10" }
  deploy_on_change = false

  preview_deployments = {
    enabled                  = true
    build_secrets_wo         = %q
    build_secrets_wo_version = %d
  }
}`, name+"-proj", name, secret, version)
	}
	serverSecret := func(want string) resource.TestCheckFunc {
		return fetchApplication(func(a *client.Application) error {
			if a.PreviewBuildSecrets == nil || *a.PreviewBuildSecrets != want {
				return fmt.Errorf("server previewBuildSecrets = %v, want %q", a.PreviewBuildSecrets, want)
			}
			return nil
		})
	}
	stateHidesSecret := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr("dokploy_application.test", "preview_deployments.build_secrets"),
		resource.TestCheckNoResourceAttr("dokploy_application.test", "preview_deployments.build_secrets_wo"),
	)
	emptyPlan := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   acctest.WriteOnlyVersionChecks(),
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkApplicationDestroy,
		Steps: []resource.TestStep{
			{
				// Both forms at once is a configuration error. This step runs
				// first, before any resource exists, so that the test's
				// destroy runs against a valid config.
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name             = %q
  environment_id   = dokploy_project.test.environments[0].id
  docker           = { image = "traefik/whoami:v1.10" }
  deploy_on_change = false

  preview_deployments = {
    build_secrets            = "SECRET=plain"
    build_secrets_wo         = "SECRET=wo"
    build_secrets_wo_version = 3
  }
}`, name+"-proj", name),
				ExpectError: regexp.MustCompile(`cannot be specified when`),
			},
			{
				Config: cfg("SECRET=one", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					stateHidesSecret,
					resource.TestCheckResourceAttr("dokploy_application.test", "preview_deployments.build_secrets_wo_version", "1"),
					serverSecret("SECRET=one"),
				),
				ConfigPlanChecks: emptyPlan,
			},
			{
				// A version bump alone is an update that carries the new value.
				Config: cfg("SECRET=two", 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction("dokploy_application.test", plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(stateHidesSecret, serverSecret("SECRET=two")),
			},
		},
	})
}
