// Package environment_test is an external test package — see the note in
// internal/resources/environment/resource_acc_test.go for why.
package environment_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

func TestAccEnvironmentDataSource_byIDAndByName(t *testing.T) {
	projectName := acctest.RandomName("proj")
	envName := acctest.RandomName("env")

	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_environment" "test" {
  project_id  = dokploy_project.test.id
  name        = %q
  description = "looked up by the data source"
}

data "dokploy_environment" "by_id" {
  id = dokploy_environment.test.id
}

data "dokploy_environment" "by_name" {
  project_id = dokploy_project.test.id
  name       = dokploy_environment.test.name
}

# The production environment Dokploy creates with every project.
data "dokploy_environment" "default" {
  project_id = dokploy_project.test.id
  name       = "production"
}`, projectName, envName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.dokploy_environment.by_id", "name", envName),
					resource.TestCheckResourceAttr("data.dokploy_environment.by_id", "description", "looked up by the data source"),
					resource.TestCheckResourceAttr("data.dokploy_environment.by_id", "is_default", "false"),
					resource.TestCheckResourceAttrPair(
						"data.dokploy_environment.by_name", "id",
						"dokploy_environment.test", "id"),
					// project_id is absent from environment.byProjectId rows,
					// so this only passes if Read finishes with a
					// GetEnvironment call.
					resource.TestCheckResourceAttrPair(
						"data.dokploy_environment.by_name", "project_id",
						"dokploy_project.test", "id"),
					resource.TestCheckResourceAttr("data.dokploy_environment.default", "is_default", "true"),
				),
			},
		},
	})
}

func TestAccEnvironmentDataSource_ambiguousNameErrors(t *testing.T) {
	projectName := acctest.RandomName("proj")
	config := fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %q
}

resource "dokploy_environment" "a" {
  project_id = dokploy_project.test.id
  name       = "shared"
}

resource "dokploy_environment" "b" {
  project_id = dokploy_project.test.id
  name       = "shared"
}

data "dokploy_environment" "ambiguous" {
  project_id = dokploy_project.test.id
  name       = "shared"
  depends_on = [dokploy_environment.a, dokploy_environment.b]
}`, projectName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`multiple environments named "shared"`),
			},
		},
	})
}
