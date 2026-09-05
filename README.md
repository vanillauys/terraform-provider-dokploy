# terraform-provider-dokploy

A Terraform and OpenTofu provider for [Dokploy](https://dokploy.com).
The registry address is `vanillauys/dokploy`. The provider requires
Terraform 1.5 or later.

## Usage

```hcl
terraform {
  required_providers {
    dokploy = {
      source  = "vanillauys/dokploy"
      version = "~> 0.11"
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

## Compatibility

This provider targets **Dokploy v0.30.5**. The acceptance suite installs
Dokploy with the upstream `install.sh` script. That script installs the latest
Dokploy release, so the suite tests each new version when it ships. The suite
does not test older versions.

## Documentation

The full reference is on the
[Terraform Registry](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs).
Start with the guides:

- **[Get started](docs/guides/getting-started.md)**: configure the provider and apply a first project, database, application, and domain.
- **[Adopt an existing Dokploy server](docs/guides/adopting-an-existing-instance.md)**: import a running server without a rebuild.
- **[Deploy semantics](docs/guides/deploy-semantics.md)**: `deploy_on_change`, timeouts, and deploy failures.
- **[Secrets and sensitive values](docs/guides/secrets.md)**: environment variables, database passwords, and backup credentials.
- **[Upgrade to v0.11](docs/guides/upgrading.md)**: the breaking changes in v0.11.0 and the configuration edit for each.

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

This provider is pre-1.0. Breaking changes can land in minor releases until
v1.0.0. If you need a stable configuration, pin an exact version.

## Coverage gaps

The provider does not model these Dokploy features yet.

- **Some Dokploy features have no resource.** Manage registries, SSH keys,
  certificates, notifications, DNS providers, and remote servers in the
  Dokploy UI. The provider has no resource for them.
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
  project, environment, application, destination, network, github_provider,
  and all six database engines. Domain hosts are also not unique, because the
  same host can attach to more than one domain. There is no `dokploy_domain`
  data source, so nothing looks up a domain by host.
- **`dokploy_compose` supports the GitHub App, plain git, and inline sources
  only.** Dokploy also has GitLab, Bitbucket, and Gitea sources. The provider
  does not model them, because no test server was available to observe their
  shapes. For the same reason, the only git provider data source is
  `dokploy_github_provider`. The same gap applies to `dokploy_application`.

## Development

[CONTRIBUTING.md](CONTRIBUTING.md) describes the toolchain, the git hooks,
the test layout, and the engineering rules. Quick reference:

- `make test`: run the unit tests.
- `./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc`: run the acceptance tests against a disposable Dokploy server. Never point them at a real server.
- `make docs`: regenerate the registry docs.
- `make hooks`: enable the gitleaks pre-commit scan. `make build` enables it on the first run.
