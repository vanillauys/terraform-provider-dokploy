// Package domain_test is an external test package — see the note in
// internal/resources/environment/resource_acc_test.go for why.
package domain_test

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

func checkDomainDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "dokploy_domain" {
			continue
		}
		if _, err := c.GetDomain(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("domain %s still exists (err = %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func fetchDomain(s *terraform.State, address string) (*client.Domain, error) {
	rs, ok := s.RootModule().Resources[address]
	if !ok {
		return nil, fmt.Errorf("%s not found in state", address)
	}
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return nil, err
	}
	return c.GetDomain(context.Background(), rs.Primary.ID)
}

// scaffold is a project + application for the domain to attach to.
//
// application has its own ConfigValidators.ExactlyOneOf(github, git, docker)
// (see internal/resources/application/resource.go), so the application needs
// a source block even though this suite never inspects it. docker is the
// simplest one available, and the image matches the one already used in
// examples/resources/dokploy_application/resource.tf.
func scaffold(projectName, appName string) string {
	return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name           = %q
  environment_id = dokploy_project.test.environments[0].id
  deploy_on_change = false

  docker = {
    image = "traefik/whoami:v1.10"
  }
}
`, projectName, appName)
}

func TestAccDomain_lifecycle(t *testing.T) {
	projectName := acctest.RandomName("proj")
	appName := acctest.RandomName("app")
	host := acctest.RandomName("acc") + ".example.com"

	config := func(extra string) string {
		return scaffold(projectName, appName) + fmt.Sprintf(`
resource "dokploy_domain" "test" {
  application_id = dokploy_application.test.id
  host           = %q
%s
}`, host, extra)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDomainDestroy,
		Steps: []resource.TestStep{
			{
				// Defaults only: proves the schema defaults match the server's.
				Config: config(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("dokploy_domain.test", "id"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "host", host),
					resource.TestCheckResourceAttr("dokploy_domain.test", "port", "3000"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "https", "false"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "path", "/"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "certificate_type", "none"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "domain_type", "application"),
					resource.TestCheckResourceAttrSet("dokploy_domain.test", "created_at"),
				),
			},
			{
				// Every optional attribute is set away from its default/null
				// here, not just the four this step used to check. Without
				// forward_auth_enabled, custom_cert_resolver and service_name
				// going non-default somewhere in the suite, the revert step
				// "reverting" them proves nothing — there is nothing to
				// revert from.
				Config: config(`  port                 = 8080
  https                = true
  certificate_type     = "letsencrypt"
  path                 = "/api"
  internal_path        = "/v1"
  strip_path           = true
  custom_entrypoint    = "websecure"
  custom_cert_resolver = "myresolver"
  service_name         = "web"
  forward_auth_enabled = true`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_domain.test", "port", "8080"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "https", "true"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "certificate_type", "letsencrypt"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "custom_entrypoint", "websecure"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "custom_cert_resolver", "myresolver"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "service_name", "web"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "forward_auth_enabled", "true"),
				),
			},
			{
				// Spec §5.6 / dialect B: domain.update KEEPS any key it does
				// not receive, so dropping these from config only converges if
				// Update sends every field on every call. All ten attributes
				// the previous step set non-default are checked here, both
				// through Terraform state and via a direct API read, so a
				// regression that dropped a single field from expandUpdate
				// (e.g. ForwardAuthEnabled) would leave the server reporting
				// its last-sent value forever and be caught — not just the
				// four this step used to check.
				Config: config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					// Plain Optional, no schema default: revert to null.
					resource.TestCheckNoResourceAttr("dokploy_domain.test", "custom_entrypoint"),
					resource.TestCheckNoResourceAttr("dokploy_domain.test", "custom_cert_resolver"),
					resource.TestCheckNoResourceAttr("dokploy_domain.test", "service_name"),
					// Optional+Computed with a schema Default: revert to that
					// default, never to null.
					resource.TestCheckResourceAttr("dokploy_domain.test", "port", "3000"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "https", "false"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "path", "/"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "internal_path", "/"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "strip_path", "false"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "certificate_type", "none"),
					resource.TestCheckResourceAttr("dokploy_domain.test", "forward_auth_enabled", "false"),
					func(s *terraform.State) error {
						d, err := fetchDomain(s, "dokploy_domain.test")
						if err != nil {
							return err
						}
						if d.CustomEntrypoint != nil {
							return fmt.Errorf("server still stores custom_entrypoint %q after it was removed from config", *d.CustomEntrypoint)
						}
						if d.CustomCertResolver != nil {
							return fmt.Errorf("server still stores custom_cert_resolver %q after it was removed from config", *d.CustomCertResolver)
						}
						if d.ServiceName != nil {
							return fmt.Errorf("server still stores service_name %q after it was removed from config", *d.ServiceName)
						}
						if d.Port != 3000 || d.CertificateType != "none" || d.HTTPS || d.Path != "/" || d.InternalPath != "/" || d.StripPath || d.ForwardAuthEnabled {
							return fmt.Errorf(
								"server did not revert to defaults: port=%d https=%t path=%q internalPath=%q stripPath=%t certificateType=%q forwardAuthEnabled=%t",
								d.Port, d.HTTPS, d.Path, d.InternalPath, d.StripPath, d.CertificateType, d.ForwardAuthEnabled)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "dokploy_domain.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// A domain attached to nothing is a state the API allows and the provider
// must not.
func TestAccDomain_requiresExactlyOneAttachment(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "dokploy_domain" "orphan" {
  host = "orphan.example.com"
}`,
				// terraform-plugin-framework-validators v0.19.0's
				// ExactlyOneOf renders "Missing Attribute Configuration" /
				// "Exactly one of these attributes must be configured"
				// (verified against this pinned version) — capitalised,
				// hence (?i) rather than the plain (?s) a case-sensitive
				// match on "exactly" would silently never satisfy.
				ExpectError: regexp.MustCompile(`(?is)Invalid Attribute Combination|exactly one of these attributes`),
			},
		},
	})
}
