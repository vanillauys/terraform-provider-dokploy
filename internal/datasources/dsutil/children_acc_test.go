package dsutil_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// childFixture creates one of each service child, plus a second record that
// repeats the lookup attribute of the first where the data source must
// refuse a guess. A security record has no such pair: Dokploy rejects a
// second record with the same user name on one application. No service
// deploys.
func childFixture(name string) string {
	return fmt.Sprintf(`
resource "dokploy_project" "test" {
  name = %[1]q
}

resource "dokploy_destination" "test" {
  name              = %[1]q
  provider_name     = "Cloudflare"
  endpoint          = "https://example.r2.cloudflarestorage.com"
  bucket            = "acc"
  region            = "auto"
  access_key        = "AKIAACCEPTANCEONLY"
  secret_access_key = "acceptance-only-not-a-real-secret"
}

resource "dokploy_application" "test" {
  name             = %[1]q
  environment_id   = dokploy_project.test.environments[0].id
  docker           = { image = "traefik/whoami:v1.10" }
  deploy_on_change = false
}

resource "dokploy_postgres" "test" {
  name              = %[1]q
  environment_id    = dokploy_project.test.environments[0].id
  database_name     = "accdb"
  database_user     = "acc"
  database_password = "acc-pass-12345"
  deploy_on_change  = false
}

resource "dokploy_port" "tcp" {
  application_id = dokploy_application.test.id
  published_port = 18080
  target_port    = 80
  protocol       = "tcp"
}

resource "dokploy_port" "udp" {
  application_id = dokploy_application.test.id
  published_port = 18080
  target_port    = 81
  protocol       = "udp"
}

resource "dokploy_port" "solo" {
  application_id = dokploy_application.test.id
  published_port = 18081
  target_port    = 82
}

resource "dokploy_redirect" "solo" {
  application_id = dokploy_application.test.id
  regex          = "^/solo/(.*)"
  replacement    = "/new/$1"
  permanent      = true
}

resource "dokploy_redirect" "first" {
  application_id = dokploy_application.test.id
  regex          = "^/twin/(.*)"
  replacement    = "/a/$1"
}

resource "dokploy_redirect" "second" {
  application_id = dokploy_application.test.id
  regex          = "^/twin/(.*)"
  replacement    = "/b/$1"
  depends_on     = [dokploy_redirect.first]
}

resource "dokploy_security" "solo" {
  application_id = dokploy_application.test.id
  username       = "solo-user"
  password       = "acceptance-only-password"
}

resource "dokploy_mount" "solo" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  type         = "volume"
  volume_name  = "%[1]s-solo"
  mount_path   = "/data/solo"
}

resource "dokploy_mount" "bind" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  type         = "bind"
  host_path    = "/tmp/%[1]s"
  mount_path   = "/data/bind"
}

resource "dokploy_mount" "first" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  type         = "volume"
  volume_name  = "%[1]s-a"
  mount_path   = "/data/twin"
}

resource "dokploy_mount" "second" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  type         = "volume"
  volume_name  = "%[1]s-b"
  mount_path   = "/data/twin"
}

resource "dokploy_schedule" "solo" {
  name            = "%[1]s-solo"
  schedule_type   = "application"
  service_id      = dokploy_application.test.id
  cron_expression = "0 4 * * *"
  command         = "echo solo"
}

resource "dokploy_schedule" "first" {
  name            = "%[1]s-twin"
  schedule_type   = "application"
  service_id      = dokploy_application.test.id
  cron_expression = "0 5 * * *"
  command         = "echo a"
}

resource "dokploy_schedule" "second" {
  name            = "%[1]s-twin"
  schedule_type   = "application"
  service_id      = dokploy_application.test.id
  cron_expression = "0 6 * * *"
  command         = "echo b"
}

resource "dokploy_volume_backup" "solo" {
  name            = "%[1]s-solo"
  service_id      = dokploy_application.test.id
  service_type    = "application"
  volume_name     = "acc-data"
  prefix          = "volumes/solo/"
  cron_expression = "0 4 * * *"
  destination_id  = dokploy_destination.test.id
}

resource "dokploy_volume_backup" "first" {
  name            = "%[1]s-twin"
  service_id      = dokploy_application.test.id
  service_type    = "application"
  volume_name     = "acc-data"
  prefix          = "volumes/a/"
  cron_expression = "0 5 * * *"
  destination_id  = dokploy_destination.test.id
}

resource "dokploy_volume_backup" "second" {
  name            = "%[1]s-twin"
  service_id      = dokploy_application.test.id
  service_type    = "application"
  volume_name     = "acc-data"
  prefix          = "volumes/b/"
  cron_expression = "0 6 * * *"
  destination_id  = dokploy_destination.test.id
}

resource "dokploy_backup" "solo" {
  service_id      = dokploy_postgres.test.id
  service_type    = "postgres"
  database        = "accdb"
  prefix          = "backups/solo/"
  cron_expression = "0 4 * * *"
  destination_id  = dokploy_destination.test.id
}

resource "dokploy_backup" "first" {
  service_id      = dokploy_postgres.test.id
  service_type    = "postgres"
  database        = "accdb"
  prefix          = "backups/twin/"
  cron_expression = "0 5 * * *"
  destination_id  = dokploy_destination.test.id
  depends_on      = [dokploy_backup.solo]
}

resource "dokploy_backup" "second" {
  service_id      = dokploy_postgres.test.id
  service_type    = "postgres"
  database        = "accdb"
  prefix          = "backups/twin/"
  cron_expression = "0 6 * * *"
  destination_id  = dokploy_destination.test.id
  depends_on      = [dokploy_backup.first]
}
`, name)
}

const childLookups = `
data "dokploy_port" "by_id" {
  id = dokploy_port.solo.id
}

data "dokploy_port" "by_port" {
  application_id = dokploy_application.test.id
  published_port = 18081
}

data "dokploy_redirect" "by_id" {
  id = dokploy_redirect.solo.id
}

data "dokploy_redirect" "by_regex" {
  application_id = dokploy_application.test.id
  regex          = "^/solo/(.*)"
}

data "dokploy_security" "by_id" {
  id = dokploy_security.solo.id
}

data "dokploy_security" "by_username" {
  application_id = dokploy_application.test.id
  username       = "solo-user"
}

data "dokploy_mount" "by_id" {
  id = dokploy_mount.solo.id
}

data "dokploy_mount" "by_mount_path" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  mount_path   = "/data/solo"
}

data "dokploy_mount" "by_host_path" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  host_path    = dokploy_mount.bind.host_path
}

data "dokploy_schedule" "by_id" {
  id = dokploy_schedule.solo.id
}

data "dokploy_schedule" "by_name" {
  service_id    = dokploy_application.test.id
  schedule_type = "application"
  name          = dokploy_schedule.solo.name
}

data "dokploy_volume_backup" "by_id" {
  id = dokploy_volume_backup.solo.id
}

data "dokploy_volume_backup" "by_name" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  name         = dokploy_volume_backup.solo.name
}

data "dokploy_backup" "by_id" {
  id = dokploy_backup.solo.id
}

data "dokploy_backup" "by_prefix" {
  service_id   = dokploy_postgres.test.id
  service_type = "postgres"
  prefix       = "backups/solo/"
}
`

// matches asserts that a data source carries the attributes of the resource
// that it looks up. The resource state comes from the create call, and the
// data source state comes from a separate read, so the pair can disagree.
func matches(dataSource, res string, attrs ...string) resource.TestCheckFunc {
	checks := make([]resource.TestCheckFunc, 0, len(attrs))
	for _, attr := range attrs {
		checks = append(checks, resource.TestCheckResourceAttrPair(dataSource, attr, res, attr))
	}
	return resource.ComposeAggregateTestCheckFunc(checks...)
}

func TestAccChildDataSources_lookups(t *testing.T) {
	name := acctest.RandomName("ds-child")
	cfg := childFixture(name)

	var checks []resource.TestCheckFunc
	for _, lookup := range []string{"by_id", "by_port"} {
		checks = append(checks, matches("data.dokploy_port."+lookup, "dokploy_port.solo",
			"id", "application_id", "published_port", "target_port", "protocol", "publish_mode"))
	}
	for _, lookup := range []string{"by_id", "by_regex"} {
		checks = append(checks, matches("data.dokploy_redirect."+lookup, "dokploy_redirect.solo",
			"id", "application_id", "regex", "replacement", "permanent"))
	}
	for _, lookup := range []string{"by_id", "by_username"} {
		checks = append(checks, matches("data.dokploy_security."+lookup, "dokploy_security.solo",
			"id", "application_id", "username"),
			resource.TestCheckNoResourceAttr("data.dokploy_security."+lookup, "password"))
	}
	for _, lookup := range []string{"by_id", "by_mount_path"} {
		checks = append(checks, matches("data.dokploy_mount."+lookup, "dokploy_mount.solo",
			"id", "service_id", "service_type", "type", "mount_path", "volume_name"))
	}
	checks = append(checks, matches("data.dokploy_mount.by_host_path", "dokploy_mount.bind",
		"id", "type", "mount_path", "host_path"))
	for _, lookup := range []string{"by_id", "by_name"} {
		checks = append(checks,
			matches("data.dokploy_schedule."+lookup, "dokploy_schedule.solo",
				"id", "name", "service_id", "schedule_type", "cron_expression", "command", "shell_type", "enabled", "app_name"),
			matches("data.dokploy_volume_backup."+lookup, "dokploy_volume_backup.solo",
				"id", "name", "service_id", "service_type", "volume_name", "prefix", "cron_expression", "destination_id", "enabled", "turn_off"))
	}
	for _, lookup := range []string{"by_id", "by_prefix"} {
		checks = append(checks, matches("data.dokploy_backup."+lookup, "dokploy_backup.solo",
			"id", "service_id", "service_type", "database", "prefix", "cron_expression", "destination_id", "enabled", "include_encryption_key"))
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: cfg},
			{Config: cfg + childLookups, Check: resource.ComposeAggregateTestCheckFunc(checks...)},
		},
	})
}

func TestAccChildDataSources_errors(t *testing.T) {
	name := acctest.RandomName("ds-child-err")
	cfg := childFixture(name)

	const (
		ambiguous = `more than one .* matches`
		missing   = `no .* matches`
		notFound  = `(?i)not found`
	)
	cases := []struct {
		name, data, want string
	}{
		{"port ambiguous", `data "dokploy_port" "x" {
  application_id = dokploy_application.test.id
  published_port = 18080
}`, ambiguous},
		{"port missing", `data "dokploy_port" "x" {
  application_id = dokploy_application.test.id
  published_port = 19999
}`, missing},
		{"port unknown id", `data "dokploy_port" "x" {
  id = "does-not-exist"
}`, notFound},
		{"redirect ambiguous", `data "dokploy_redirect" "x" {
  application_id = dokploy_application.test.id
  regex          = "^/twin/(.*)"
}`, ambiguous},
		{"redirect missing", `data "dokploy_redirect" "x" {
  application_id = dokploy_application.test.id
  regex          = "^/none"
}`, missing},
		{"redirect unknown id", `data "dokploy_redirect" "x" {
  id = "does-not-exist"
}`, notFound},
		{"security missing", `data "dokploy_security" "x" {
  application_id = dokploy_application.test.id
  username       = "nobody"
}`, missing},
		{"security unknown id", `data "dokploy_security" "x" {
  id = "does-not-exist"
}`, notFound},
		{"mount ambiguous", `data "dokploy_mount" "x" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  mount_path   = "/data/twin"
}`, ambiguous},
		{"mount missing", `data "dokploy_mount" "x" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  mount_path   = "/data/none"
}`, missing},
		{"mount unknown id", `data "dokploy_mount" "x" {
  id = "does-not-exist"
}`, notFound},
		{"schedule ambiguous", fmt.Sprintf(`data "dokploy_schedule" "x" {
  service_id    = dokploy_application.test.id
  schedule_type = "application"
  name          = "%s-twin"
}`, name), ambiguous},
		{"schedule missing", `data "dokploy_schedule" "x" {
  service_id    = dokploy_application.test.id
  schedule_type = "application"
  name          = "none"
}`, missing},
		{"schedule unknown id", `data "dokploy_schedule" "x" {
  id = "does-not-exist"
}`, notFound},
		{"volume backup ambiguous", fmt.Sprintf(`data "dokploy_volume_backup" "x" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  name         = "%s-twin"
}`, name), ambiguous},
		{"volume backup missing", `data "dokploy_volume_backup" "x" {
  service_id   = dokploy_application.test.id
  service_type = "application"
  name         = "none"
}`, missing},
		{"volume backup unknown id", `data "dokploy_volume_backup" "x" {
  id = "does-not-exist"
}`, notFound},
		{"backup ambiguous", `data "dokploy_backup" "x" {
  service_id   = dokploy_postgres.test.id
  service_type = "postgres"
  prefix       = "backups/twin/"
}`, ambiguous},
		{"backup missing", `data "dokploy_backup" "x" {
  service_id   = dokploy_postgres.test.id
  service_type = "postgres"
  prefix       = "backups/none/"
}`, missing},
		{"backup unknown id", `data "dokploy_backup" "x" {
  id = "does-not-exist"
}`, notFound},
	}

	steps := []resource.TestStep{{Config: cfg}}
	for _, c := range cases {
		steps = append(steps, resource.TestStep{Config: cfg + c.data, ExpectError: regexp.MustCompile(c.want)})
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps:                    steps,
	})
}
