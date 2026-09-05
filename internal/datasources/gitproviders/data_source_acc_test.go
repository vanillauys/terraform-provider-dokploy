// Package gitproviders_test holds the acceptance tests (external package;
// acctest imports provider, which imports this package).
package gitproviders_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func fixtures(name string) string {
	return fmt.Sprintf(`
resource "dokploy_gitlab_provider" "gl" {
  name           = %[1]q
  application_id = "oauth-app"
  secret         = "s"
}

resource "dokploy_bitbucket_provider" "bb" {
  name         = %[1]q
  username     = "bbuser"
  app_password = "p"
}

resource "dokploy_gitea_provider" "gt" {
  name          = %[1]q
  gitea_url     = "https://gitea.example.com"
  client_id     = "cid"
  client_secret = "s"
}
`, name)
}

// One name across the three types: each data source must filter on its own
// type, so the shared name is not ambiguous.
func TestAccGitProviderDataSources_byNameAndByID(t *testing.T) {
	name := acctest.RandomName("gp-ds")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: fixtures(name)},
			{
				Config: fixtures(name) + `
data "dokploy_gitlab_provider" "by_name" { name = dokploy_gitlab_provider.gl.name }
data "dokploy_gitlab_provider" "by_id" { id = dokploy_gitlab_provider.gl.id }
data "dokploy_bitbucket_provider" "by_name" { name = dokploy_bitbucket_provider.bb.name }
data "dokploy_bitbucket_provider" "by_id" { id = dokploy_bitbucket_provider.bb.id }
data "dokploy_gitea_provider" "by_name" { name = dokploy_gitea_provider.gt.name }
data "dokploy_gitea_provider" "by_id" { id = dokploy_gitea_provider.gt.id }
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.dokploy_gitlab_provider.by_name", "id", "dokploy_gitlab_provider.gl", "id"),
					resource.TestCheckResourceAttrPair("data.dokploy_gitlab_provider.by_id", "git_provider_id", "dokploy_gitlab_provider.gl", "git_provider_id"),
					resource.TestCheckResourceAttr("data.dokploy_gitlab_provider.by_name", "is_configured", "false"),
					resource.TestCheckResourceAttr("data.dokploy_gitlab_provider.by_name", "application_id", "oauth-app"),
					resource.TestCheckResourceAttrPair("data.dokploy_bitbucket_provider.by_name", "id", "dokploy_bitbucket_provider.bb", "id"),
					resource.TestCheckResourceAttr("data.dokploy_bitbucket_provider.by_id", "username", "bbuser"),
					resource.TestCheckResourceAttr("data.dokploy_bitbucket_provider.by_id", "is_deprecated", "true"),
					resource.TestCheckNoResourceAttr("data.dokploy_bitbucket_provider.by_id", "app_password"),
					resource.TestCheckResourceAttrPair("data.dokploy_gitea_provider.by_name", "id", "dokploy_gitea_provider.gt", "id"),
					resource.TestCheckResourceAttr("data.dokploy_gitea_provider.by_id", "gitea_url", "https://gitea.example.com"),
					resource.TestCheckResourceAttr("data.dokploy_gitea_provider.by_id", "is_configured", "false"),
					resource.TestCheckNoResourceAttr("data.dokploy_gitea_provider.by_id", "client_secret"),
				),
			},
		},
	})
}

func TestAccGitProviderDataSources_notFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      `data "dokploy_gitlab_provider" "missing" { name = "no-such-gitlab-xyzzy" }`,
				ExpectError: regexp.MustCompile(`no GitLab provider named "no-such-gitlab-xyzzy"`),
			},
			{
				Config:      `data "dokploy_gitea_provider" "missing" { id = "no-such-id" }`,
				ExpectError: regexp.MustCompile(`no Gitea provider with id "no-such-id"`),
			},
		},
	})
}

func TestAccGitlabProviderDataSource_ambiguousName(t *testing.T) {
	name := acctest.RandomName("gl-dup")
	twins := fmt.Sprintf(`
resource "dokploy_gitlab_provider" "a" {
  name           = %[1]q
  application_id = "a"
  secret         = "s"
}
resource "dokploy_gitlab_provider" "b" {
  name           = %[1]q
  application_id = "b"
  secret         = "s"
}
`, name)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: twins},
			{
				Config:      twins + fmt.Sprintf(`data "dokploy_gitlab_provider" "dup" { name = %q }`, name),
				ExpectError: regexp.MustCompile(fmt.Sprintf(`2 GitLab providers are named %q`, name)),
			},
		},
	})
}
