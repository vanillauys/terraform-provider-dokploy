# Terraform provider for Dokploy

[![Terraform Registry](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fregistry.terraform.io%2Fv1%2Fproviders%2Fvanillauys%2Fdokploy&query=%24.version&prefix=v&label=registry&color=7B42BC&logo=terraform&logoColor=white)](https://registry.terraform.io/providers/vanillauys/dokploy/latest)
[![Registry downloads](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fregistry.terraform.io%2Fv2%2Fproviders%2Fvanillauys%2Fdokploy&query=%24.data.attributes.downloads&label=downloads&color=7B42BC)](https://registry.terraform.io/providers/vanillauys/dokploy/latest)
[![Dokploy v0.30.8](https://img.shields.io/badge/Dokploy-v0.30.8-0EA5E9)](https://github.com/Dokploy/dokploy/releases/tag/v0.30.8)
[![test](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/test.yml/badge.svg)](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/test.yml)
[![nightly acceptance](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/nightly.yml/badge.svg)](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/nightly.yml)
[![Quality gate](https://sonarcloud.io/api/project_badges/measure?project=vanillauys_terraform-provider-dokploy&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=vanillauys_terraform-provider-dokploy)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/vanillauys/terraform-provider-dokploy/badge)](https://scorecard.dev/viewer/?uri=github.com/vanillauys/terraform-provider-dokploy)
[![License: MIT](https://img.shields.io/github/license/vanillauys/terraform-provider-dokploy)](LICENSE)

Manage a self-hosted [Dokploy](https://dokploy.com) server as code, with
Terraform or OpenTofu. Declare your projects, applications, compose stacks,
databases, domains, backups, and integrations. `terraform apply` creates
them, deploys them, and keeps them in sync.

- **48 resources and 45 data sources.** Each one has an acceptance test that
  runs against a real Dokploy server on every pull request and every night.
- **Deploys on change.** A service redeploys when its configuration changes,
  and the apply waits for the deploy to finish.
- **Secrets stay out of state.** Every secret attribute has a write-only
  companion (`<name>_wo`) for Terraform 1.11 or later.
- **Docker Swarm settings** on applications and databases: replicas,
  placement, update and rollback policy, health checks, and more.
- **Connection checks.** `verify_connection = true` tests a registry,
  destination, git provider, AI endpoint, or notification channel during
  the apply.
- **Adoption.** Every resource except `dokploy_api_key` imports, and
  [a script](dogfood/generate_imports.py) writes the import blocks for a
  running server.

[Documentation](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs) ·
[Get started](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/guides/getting-started) ·
[Usage examples](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/guides/usage-examples) ·
[Changelog](CHANGELOG.md) ·
[Contributing](CONTRIBUTING.md) ·
[Security](SECURITY.md)

## Quick start

1. In the Dokploy UI, open **Settings > Profile > API/CLI Keys** and
   generate an API key.

2. Export the server address and the key:

   ```sh
   export DOKPLOY_ENDPOINT="https://dokploy.example.com"
   export DOKPLOY_API_KEY="<your key>"
   ```

3. Save this configuration as `main.tf`. It creates a project, deploys a
   container, and routes a domain with a Let's Encrypt certificate to it:

   ```hcl
   terraform {
     required_providers {
       dokploy = {
         source  = "vanillauys/dokploy"
         version = "~> 1.9"
       }
     }
   }

   # The provider reads DOKPLOY_ENDPOINT and DOKPLOY_API_KEY.
   provider "dokploy" {}

   resource "dokploy_project" "demo" {
     name        = "demo"
     description = "Managed by Terraform"
   }

   resource "dokploy_application" "web" {
     name           = "web"
     environment_id = dokploy_project.demo.production_environment_id

     docker = {
       image = "traefik/whoami:v1.10"
     }
   }

   resource "dokploy_domain" "web" {
     application_id   = dokploy_application.web.id
     host             = "web.example.com"
     port             = 80
     https            = true
     certificate_type = "letsencrypt"
   }
   ```

4. Apply it:

   ```sh
   terraform init
   terraform apply
   ```

The [Get started](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/guides/getting-started)
guide continues with a database, environment variables, and a second
environment.

## Provider configuration

| Argument | Environment variable | Required | Description |
|----------|----------------------|----------|-------------|
| `endpoint` | `DOKPLOY_ENDPOINT` | yes | The URL of the Dokploy server, for example `https://dokploy.example.com`. |
| `api_key` | `DOKPLOY_API_KEY` | yes | An API key from **Settings > Profile > API/CLI Keys**. Sensitive. |
| `insecure` | – | no | Skip the TLS certificate check, for a server with a self-signed certificate. Defaults to `false`. |

Set the key through the environment variable, not in the configuration, so
that it stays out of your repository.

## Compatibility

| Provider | Dokploy pin |
|----------|-------------|
| 1.8.x – 1.9.x | v0.30.8 |
| 1.7.x | v0.30.7 |
| 1.1.1 – 1.6.x | v0.30.6 |
| 1.0.x – 1.1.0 | v0.30.5 |

- **Dokploy:** the acceptance suite installs the latest Dokploy release on
  every pull request and every night, so it tests each new version when it
  ships. It does not test older versions. A server older than the pin can
  reject a field that a newer Dokploy introduced. If your server is older,
  upgrade it before you apply.
- **Terraform:** 1.5 or later. The write-only attributes (`<name>_wo`) need
  1.11 or later. The nightly run tests Terraform 1.5.7 and the latest
  release.
- **OpenTofu:** the nightly run tests OpenTofu 1.12.7 with the Terraform
  Registry address `vanillauys/dokploy`. OpenTofu 1.13 cannot import an ID
  that starts with `-`
  ([opentofu/opentofu#4644](https://github.com/opentofu/opentofu/issues/4644)).
  The provider is not on the OpenTofu registry yet.

## What you can manage

| Area | Resources |
|------|-----------|
| Projects | [`dokploy_project`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/project), [`dokploy_environment`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/environment), [`dokploy_environment_variables`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/environment_variables), [`dokploy_tag`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/tag) |
| Services | [`dokploy_application`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/application) (GitHub, GitLab, Bitbucket, Gitea, git, or Docker image), [`dokploy_compose`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/compose) (the same sources or an inline file), [`dokploy_patch`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/patch) |
| Databases | [`dokploy_postgres`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/postgres), [`dokploy_mysql`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/mysql), [`dokploy_mariadb`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/mariadb), [`dokploy_mongo`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/mongo), [`dokploy_redis`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/redis), [`dokploy_libsql`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/libsql) |
| Routing | [`dokploy_domain`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/domain), [`dokploy_port`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/port), [`dokploy_redirect`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/redirect), [`dokploy_security`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/security), [`dokploy_certificate`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/certificate) |
| Storage and backups | [`dokploy_mount`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/mount), [`dokploy_destination`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/destination), [`dokploy_backup`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/backup), [`dokploy_volume_backup`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/volume_backup), [`dokploy_web_server_backup`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/web_server_backup), [`dokploy_network`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/network) |
| Automation | [`dokploy_schedule`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/schedule) |
| Notifications | `dokploy_<channel>_notification` for [Slack](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/slack_notification), [Discord](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/discord_notification), [Telegram](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/telegram_notification), [email](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/email_notification), [Resend](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/resend_notification), [Gotify](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/gotify_notification), [ntfy](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/ntfy_notification), [Mattermost](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/mattermost_notification), [Lark](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/lark_notification), [Microsoft Teams](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/teams_notification), [Pushover](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/pushover_notification), and a [custom webhook](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/custom_notification) |
| Servers | [`dokploy_server`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/server), [`dokploy_ssh_key`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/ssh_key), [`dokploy_web_server_settings`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/web_server_settings) |
| Integrations | [`dokploy_gitlab_provider`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/gitlab_provider), [`dokploy_bitbucket_provider`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/bitbucket_provider), [`dokploy_gitea_provider`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/gitea_provider), [`dokploy_registry`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/registry), [`dokploy_vault_provider`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/vault_provider), [`dokploy_ai`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/ai) |
| Access | [`dokploy_organization`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/organization), [`dokploy_user`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/user), [`dokploy_user_permissions`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/user_permissions), [`dokploy_api_key`](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/resources/api_key) |

Each resource has a data source with the same name, which looks up a record
by id or by name. The exceptions are `dokploy_environment_variables`,
`dokploy_user_permissions`, `dokploy_api_key`, `dokploy_web_server_backup`,
and `dokploy_web_server_settings`. The `dokploy_github_provider` data source finds
a GitHub App, which only the Dokploy UI can create.

## Before you start

These three problems cause the most failures:

1. **Dokploy rate-limits API keys, and an exhausted key returns `401`, not
   `429`.** A large apply can fail with an authentication error on a key that
   works for single requests. See
   [Get started](docs/guides/getting-started.md#before-your-first-apply-api-key-rate-limits).
2. **`dokploy_application` and `dokploy_compose` own the whole service.** An
   apply of either resource replaces each setting that changed in the Dokploy
   UI. Manage a service in Terraform or in the UI, not in both. See
   [Adopt an existing Dokploy server](docs/guides/adopting-an-existing-instance.md#decide-what-terraform-owns).
3. **The default `docker_image` for MariaDB and MongoDB does not exist on
   Docker Hub.** Set an explicit tag, or each deploy fails. See
   [Deploy semantics](docs/guides/deploy-semantics.md#two-engines-whose-default-image-does-not-exist).

## Guides

- **[Get started](docs/guides/getting-started.md)**: configure the provider and apply a first project, database, application, and domain.
- **[Usage examples](docs/guides/usage-examples.md)**: short, complete configurations for the common setups.
- **[Adopt an existing Dokploy server](docs/guides/adopting-an-existing-instance.md)**: import a running server without a rebuild.
- **[Deploy semantics](docs/guides/deploy-semantics.md)**: `deploy_on_change`, timeouts, and deploy failures.
- **[Secrets and sensitive values](docs/guides/secrets.md)**: environment variables, database passwords, write-only attributes, and connection checks.
- **[Upgrade guide](docs/guides/upgrading.md)**: what each release needs from your configuration.

## Known limitations

The provider cannot do these things. In most cases, Dokploy has no API for
them.

- **Some Dokploy settings have no resource.** Manage DNS providers and the
  Traefik configuration files in the Dokploy UI. `dokploy_compose` has no
  `swarm` block, because Dokploy stores no swarm settings for a compose
  service. Preview deployment records and a rollback are operations of the
  UI; `dokploy_application` holds only their settings.
- **`dokploy_server` stores the record only.** It does not install Docker on
  the machine. Run **Setup Server** in the Dokploy UI after the first apply.
- **A GitLab or Gitea connection needs one browser step.** Terraform stores
  the OAuth application; a person authorizes it once in the Dokploy UI. The
  data sources report `is_configured` for that state. A GitHub App has no
  create endpoint, so only the `dokploy_github_provider` data source exists.
- **`dokploy_api_key` cannot be imported.** Dokploy returns the key once, at
  creation.
- **`dokploy_user` cannot change a password.** Dokploy has no endpoint that
  resets the password of another user, so a password change replaces the
  account.
- **`dokploy_backup` cannot back up Redis.** Dokploy has no logical dump for
  Redis. Use `dokploy_volume_backup`, which archives the volume.
- **`dokploy_redis` has only `database_password`, and `dokploy_mongo` has no
  `database_name` or `database_root_password`.** The Dokploy data model has
  no other credential field for these engines.
- **A `dokploy_libsql` replica needs a `command` override.** Dokploy starts
  `sqld` with a fixed command that ignores `SQLD_NODE` and
  `SQLD_PRIMARY_URL`. Without the override, the container runs as a second
  primary (verified on Dokploy v0.30.5). The resource page shows the
  `command` that makes it replicate. A replica also cannot have an external
  port.
- **Names are not unique in Dokploy.** A data source that looks up a record
  by name fails when more than one record matches. Tag names are the
  exception. The `dokploy_domain` data source takes an `application_id` or
  `compose_id` filter, because the same host can attach to more than one
  service.

## Stability

The provider follows [semantic versioning](https://semver.org) from v1.0.0:

- A minor release adds resources, data sources, and attributes. A
  configuration and a state from the previous minor release load with an
  empty plan.
- A change that removes or renames an attribute, changes a default, or
  changes what an existing attribute does needs a major release. The
  [upgrade guide](docs/guides/upgrading.md) then shows the old and the new
  configuration.
- A deprecated attribute stays for at least one minor release, with a
  warning at plan time, before a major release removes it.
- The Dokploy compatibility pin moves in minor releases, with the census of
  the upstream changes in the [changelog](CHANGELOG.md).

Pin the minor version, for example `~> 1.9`, to get fixes and additions
without a breaking change.

## Contributing

Bug reports, feature requests, and pull requests are welcome.
[CONTRIBUTING.md](CONTRIBUTING.md) walks you from the fork to the merge.
Know these three rules first:

- **Every commit must be signed** and show **Verified** on GitHub. The
  guide shows the SSH and the GPG setup.
- **Open an issue before you add a resource or an attribute**, so that the
  shape is agreed first.
- **Acceptance tests run against a disposable local Dokploy server.** Never
  point them at a real server.

Quick reference:

```sh
make test    # unit tests and the schema snapshot
make lint    # golangci-lint, must print "0 issues"
make docs    # regenerate docs/ from templates/ and examples/
./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc
```

## Security

Report a vulnerability through
[GitHub private vulnerability reporting](https://github.com/vanillauys/terraform-provider-dokploy/security/advisories/new),
not through a public issue. [SECURITY.md](SECURITY.md) states the scope and
the supported versions. Each release is signed with GPG key
`750EE4482941313E`, the key that the Terraform Registry verifies, and
carries a GitHub build provenance attestation.

## Support

This is a community project, not an official Dokploy product. Ask a
question or report a problem in the
[issues](https://github.com/vanillauys/terraform-provider-dokploy/issues).
Help comes on a best-effort basis.

## License

[MIT](LICENSE).
