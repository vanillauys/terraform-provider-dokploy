// Package gitprovider_test holds the acceptance test (external package;
// acctest imports provider, which imports this package).
package gitprovider_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// A GitHub App cannot be installed from the API — it is a browser flow, and
// the github router has no .create — so the acceptance rig never has one.
// This test is gated on the same env var the application resource's
// github-source test uses, and states the gap plainly rather than asserting
// something weaker against an empty list.
func TestAccGithubProvider_byName(t *testing.T) {
	name := os.Getenv("DOKPLOY_ACC_GITHUB_PROVIDER_NAME")
	if name == "" {
		t.Skip("set DOKPLOY_ACC_GITHUB_PROVIDER_NAME to the name of a GitHub provider on the target instance")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`data "dokploy_github_provider" "test" { name = %q }`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.dokploy_github_provider.test", "name", name),
					resource.TestCheckResourceAttr("data.dokploy_github_provider.test", "provider_type", "github"),
					resource.TestCheckResourceAttrSet("data.dokploy_github_provider.test", "id"),
					resource.TestCheckResourceAttrSet("data.dokploy_github_provider.test", "git_provider_id"),
				),
			},
			{
				// Looking the same record up by the id just discovered must
				// resolve to the same record.
				Config: fmt.Sprintf(`
data "dokploy_github_provider" "by_name" { name = %q }
data "dokploy_github_provider" "by_id"   { id   = data.dokploy_github_provider.by_name.id }
`, name),
				Check: resource.TestCheckResourceAttrPair(
					"data.dokploy_github_provider.by_id", "git_provider_id",
					"data.dokploy_github_provider.by_name", "git_provider_id"),
			},
		},
	})
}
