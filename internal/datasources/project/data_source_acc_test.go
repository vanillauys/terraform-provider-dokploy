// Package project_test (an external test package, deliberately distinct
// from package project) holds the acceptance test. It must live outside
// package project: acctest imports provider, and provider imports this
// package to register dokploy_project — so an internal test file (package
// project) importing acctest here would form an import cycle
// (project -> acctest -> provider -> project), which the Go toolchain
// rejects with "import cycle not allowed in test". Keeping this file in
// the external project_test package sidesteps that: it depends on project
// (indirectly, via provider) without itself being part of project. Mirrors
// internal/resources/project/resource_acc_test.go.
package project_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func TestAccProjectDataSource(t *testing.T) {
	name := acctest.RandomName("dsproj")
	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

data "dokploy_project" "by_id" {
  id = dokploy_project.test.id
}

data "dokploy_project" "by_name" {
  name = dokploy_project.test.name
}`, name)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.dokploy_project.by_id", "name", name),
					resource.TestCheckResourceAttrPair("data.dokploy_project.by_name", "id", "dokploy_project.test", "id"),
					resource.TestCheckResourceAttrSet("data.dokploy_project.by_name", "environments.0.id"),
				),
			},
		},
	})
}
