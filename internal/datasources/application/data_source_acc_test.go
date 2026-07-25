// Package application_test (an external test package, deliberately distinct
// from package application) holds the acceptance test. It must live outside
// package application: acctest imports provider, and provider imports this
// package to register dokploy_application — so an internal test file
// (package application) importing acctest here would form an import cycle
// (application -> acctest -> provider -> application), which the Go
// toolchain rejects with "import cycle not allowed in test". Keeping this
// file in the external application_test package sidesteps that: it depends
// on application (indirectly, via provider) without itself being part of
// application. Mirrors internal/datasources/project/data_source_acc_test.go.
package application_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func TestAccApplicationDataSource(t *testing.T) {
	name := acctest.RandomName("dsapp")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_application" "test" {
  name             = %q
  environment_id   = dokploy_project.test.environments[0].id
  docker           = { image = "traefik/whoami:v1.10" }
  deploy_on_change = false
}

data "dokploy_application" "test" {
  id = dokploy_application.test.id
}`, name+"-proj", name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.dokploy_application.test", "name", name),
					resource.TestCheckResourceAttr("data.dokploy_application.test", "source_type", "docker"),
				),
			},
		},
	})
}
