# terraform-provider-dokploy

[![Terraform Registry](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fregistry.terraform.io%2Fv1%2Fproviders%2Fvanillauys%2Fdokploy&query=%24.version&prefix=v&label=registry&color=7B42BC&logo=terraform&logoColor=white)](https://registry.terraform.io/providers/vanillauys/dokploy/latest)
[![Registry downloads](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fregistry.terraform.io%2Fv2%2Fproviders%2Fvanillauys%2Fdokploy&query=%24.data.attributes.downloads&label=downloads&color=7B42BC)](https://registry.terraform.io/providers/vanillauys/dokploy/latest)
[![test](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/test.yml/badge.svg)](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/test.yml)
[![nightly acceptance](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/nightly.yml/badge.svg)](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/nightly.yml)
[![release](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/release.yml/badge.svg)](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/release.yml)
[![CodeQL](https://github.com/vanillauys/terraform-provider-dokploy/actions/workflows/dynamic/github-code-scanning/codeql/badge.svg)](https://github.com/vanillauys/terraform-provider-dokploy/security/code-scanning)
[![Go version](https://img.shields.io/github/go-mod/go-version/vanillauys/terraform-provider-dokploy?logo=go&logoColor=white)](go.mod)
[![Dokploy v0.30.5](https://img.shields.io/badge/Dokploy-v0.30.5-0EA5E9)](https://github.com/Dokploy/dokploy/releases/tag/v0.30.5)
[![Terraform 1.5+](https://img.shields.io/badge/Terraform-1.5%2B-7B42BC?logo=terraform&logoColor=white)](https://developer.hashicorp.com/terraform)
[![OpenTofu 1.12](https://img.shields.io/badge/OpenTofu-1.12-FFDA18?logo=opentofu&logoColor=black)](https://opentofu.org)
[![License: MIT](https://img.shields.io/github/license/vanillauys/terraform-provider-dokploy)](LICENSE)

A Terraform and OpenTofu provider for [Dokploy](https://dokploy.com), the
self-hosted PaaS. It manages projects, environments, applications, compose
stacks, databases, domains, backups, servers, git providers, notifications,
users, and API keys: 46 resources and 19 data sources, each with an
acceptance test against a real Dokploy server.

[Documentation](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs) ·
[Get started](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/guides/getting-started) ·
[Usage examples](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs/guides/usage-examples) ·
[Changelog](CHANGELOG.md) ·
[Contributing](CONTRIBUTING.md) ·
[Security](SECURITY.md)

The registry address is `vanillauys/dokploy`. The provider requires
Terraform 1.5 or later. The write-only companions of the secret attributes
(`<name>_wo`) need Terraform 1.11 or later; a configuration without them
works on 1.5.

## Usage

```hcl
terraform {
  required_providers {
    dokploy = {
      source  = "vanillauys/dokploy"
      version = "~> 1.0"
    }
  }
}

provider "dokploy" {
  endpoint = "https://dokploy.example.com" # or DOKPLOY_ENDPOINT
  # The provider reads api_key from DOKPLOY_API_KEY.
}
```

The [registry documentation](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs)
describes each resource and data source.

## What you can manage

| Area | Resources |
|------|-----------|
| Projects | `dokploy_project`, `dokploy_environment`, `dokploy_environment_variables` |
| Services | `dokploy_application` (GitHub, GitLab, Bitbucket, Gitea, git, or Docker source), `dokploy_compose` (the same sources or an inline file) |
| Databases | `dokploy_postgres`, `dokploy_mysql`, `dokploy_mariadb`, `dokploy_mongo`, `dokploy_redis`, `dokploy_libsql` |
| Routing | `dokploy_domain`, `dokploy_port`, `dokploy_redirect`, `dokploy_security`, `dokploy_certificate` |
| Storage and backups | `dokploy_mount`, `dokploy_destination`, `dokploy_backup`, `dokploy_volume_backup`, `dokploy_network` |
| Automation | `dokploy_schedule`, twelve `dokploy_<channel>_notification` resources (Slack, Discord, Telegram, email, Resend, Gotify, ntfy, Mattermost, Lark, Teams, Pushover, custom webhook) |
| Servers | `dokploy_server`, `dokploy_ssh_key` |
| Integrations | `dokploy_gitlab_provider`, `dokploy_bitbucket_provider`, `dokploy_gitea_provider`, `dokploy_registry`, `dokploy_vault_provider`, `dokploy_ai` |
| Access | `dokploy_organization`, `dokploy_user`, `dokploy_user_permissions`, `dokploy_api_key` |

A data source with the same name looks up most of these records by name or
id, including the GitHub Apps that only the Dokploy UI can create. The
[Usage examples](docs/guides/usage-examples.md) guide shows the common
combinations in a few lines each.

## Compatibility

This provider targets **Dokploy v0.30.5**. The acceptance suite installs
Dokploy with the upstream `install.sh` script. That script installs the latest
Dokploy release, so the suite tests each new version when it ships, on every
pull request and every night. The suite does not test older versions. A server
older than the pinned version can reject a field that a newer Dokploy
introduced, for example the network attachment fields of v0.30.0. If your
server is older, upgrade it to the pinned version or later before you apply.

The acceptance suite runs on Terraform 1.16.1 in CI. On 2026-09-05 the
project and destination packages of the suite also passed on Terraform 1.5.7
and on OpenTofu 1.12.6, with the provider binary from this repository. The
provider is not on the OpenTofu registry yet.

## Stability

The provider follows [semantic versioning](https://semver.org) from v1.0.0:

- A minor release adds resources, data sources, and attributes. A
  configuration and a state from the previous minor release load with an
  empty plan.
- A change that removes or renames an attribute, changes a default, or
  changes what an existing attribute does is a breaking change. It needs a
  major release, and the [upgrade guide](docs/guides/upgrading.md) shows the
  old and the new configuration.
- A deprecated attribute stays for at least one minor release and prints a
  warning at plan time before a major release removes it.
- The Dokploy compatibility pin moves in minor releases, with the census of
  the upstream changes in the changelog.
- The write-only companions need Terraform 1.11 or later. Everything else
  needs Terraform 1.5 or later.

Pin the minor version, `~> 1.0`, to get fixes and additions without a
breaking change.

## Documentation

The full reference is on the
[Terraform Registry](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs).
Start with the guides:

- **[Get started](docs/guides/getting-started.md)**: configure the provider and apply a first project, database, application, and domain.
- **[Usage examples](docs/guides/usage-examples.md)**: short, complete configurations for the common setups.
- **[Adopt an existing Dokploy server](docs/guides/adopting-an-existing-instance.md)**: import a running server without a rebuild.
- **[Deploy semantics](docs/guides/deploy-semantics.md)**: `deploy_on_change`, timeouts, and deploy failures.
- **[Secrets and sensitive values](docs/guides/secrets.md)**: environment variables, database passwords, and backup credentials.
- **[Upgrade guide](docs/guides/upgrading.md)**: what each release needs from your configuration; v0.11.0 had the breaking changes.

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

## Coverage gaps

The provider does not model these Dokploy features yet.

- **Some Dokploy settings have no resource.** Manage DNS providers, the
  Traefik configuration files, preview deployments, rollbacks, tags, and the
  Docker Swarm placement fields of a service in the Dokploy UI.
- **`dokploy_server` stores the record only.** It does not run the setup that
  installs Docker on the machine. Run **Setup Server** in the Dokploy UI
  after the first apply.
- **A GitLab or Gitea connection needs one browser step.** Terraform stores
  the OAuth application; a person authorizes it once in the Dokploy UI. The
  `dokploy_gitlab_provider` and `dokploy_gitea_provider` data sources report
  `is_configured` for that state. A GitHub App has no create endpoint at all;
  only the `dokploy_github_provider` data source exists.
- **`dokploy_api_key` cannot be imported.** Dokploy returns the key once, at
  creation.
- **`dokploy_user` cannot change a password.** Dokploy has no endpoint that
  resets another user's password, so a password change replaces the
  account.
- **`dokploy_vault_provider` models six provider types.** Dokploy v0.30.5
  adds a seventh type, `phase` (Phase.dev). The resource cannot create or
  update a Phase vault provider. Manage one in the Dokploy UI.
- **`dokploy_backup` cannot back up Redis.** Dokploy has no logical dump for
  Redis. Use `dokploy_volume_backup`, which archives the volume and accepts a
  Redis parent. The provider also does not expose a backup of the Dokploy
  server itself (Dokploy's `web-server` backup type). That backup type has no
  parent service and needs its own validation path.
- **`dokploy_redis` has no `database_name`, `database_user`, or
  `database_root_password` attribute.** The Dokploy data model for Redis has
  no credential field other than `database_password`. This is a gap in
  Dokploy, not in the schema.
- **`dokploy_mongo` has no `database_name` or `database_root_password`
  attribute.** MongoDB has no database name at create time and no root
  password separate from `database_user` and `database_password`. Dokploy
  also has a `replicaSets` option for MongoDB, for a replica-set topology.
  The provider does not expose it. Each `dokploy_mongo` instance uses the
  default standalone mode of the server.
- **A `dokploy_libsql` replica needs a `command` override on Dokploy
  v0.30.5.** Dokploy stores `sqld_node = "replica"` and `sqld_primary_url`
  and passes them to the container as `SQLD_NODE` and `SQLD_PRIMARY_URL`.
  It also starts `sqld` with a fixed command that bypasses the image
  entrypoint, and `sqld` reads neither variable, so the container runs as a
  second primary. The resource page shows the `command` that makes it
  replicate; a row written on the primary then reads back from the replica
  (verified 2026-09-05). A replica also cannot have an external port:
  Dokploy rejects each `saveExternalPorts` call while `sqld_node` is
  `replica`.
- **Names are not unique in Dokploy.** Each data source that looks up a
  record by name errors when more than one record matches. This applies to
  project, environment, application, destination, network, ssh_key, server,
  organization, the four git providers, and all six database engines.
  Domain hosts are also not unique, because the same host can attach to more
  than one domain. There is no `dokploy_domain` data source, so nothing
  looks up a domain by host.
- **`dogfood/generate_imports.py` lists a user as a comment.** Dokploy
  never returns a password, so an imported `dokploy_user` has no valid
  configuration until you add `password` or `password_wo` by hand. The
  script imports the permissions of each member.

## Development

[CONTRIBUTING.md](CONTRIBUTING.md) describes the toolchain, the git hooks,
the test layout, and the engineering rules. Quick reference:

- `make test`: run the unit tests.
- `./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc`: run the acceptance tests against a disposable Dokploy server. Never point them at a real server.
- `make docs`: regenerate the registry docs.
- `make hooks`: enable the gitleaks pre-commit scan. `make build` enables it on the first run.

## Contributing

Open an [issue](https://github.com/vanillauys/terraform-provider-dokploy/issues/new/choose) for a bug or a Dokploy
feature the provider does not model. A pull request follows
[CONTRIBUTING.md](CONTRIBUTING.md); the pull request template lists the
files a resource change touches. Every pull request runs the unit tests,
the linter, `govulncheck`, CodeQL, the docs check, and the acceptance suite
against a fresh Dokploy server.

## Security

Report a vulnerability through
[GitHub private vulnerability reporting](https://github.com/vanillauys/terraform-provider-dokploy/security/advisories/new),
not through a public issue. [SECURITY.md](SECURITY.md) states the scope
and the supported versions. Each release is signed with GPG key
`750EE4482941313E`, the key the Terraform registry verifies.

## License

[MIT](LICENSE).

