# terraform-provider-dokploy

Terraform/OpenTofu provider for [Dokploy](https://dokploy.com).
Registry address: `vanillauys/dokploy`. Requires Terraform >= 1.5.

## Usage

```hcl
terraform {
  required_providers {
    dokploy = {
      source  = "vanillauys/dokploy"
      version = "~> 0.10"
    }
  }
}

provider "dokploy" {
  endpoint = "https://dokploy.example.com" # or DOKPLOY_ENDPOINT
  # api_key via DOKPLOY_API_KEY
}
```

See the [registry documentation](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs) for resources and data sources.

## Compatibility

Developed and tested against **Dokploy v0.30.5**. The acceptance suite installs
Dokploy with the upstream `install.sh`, which tracks the latest release, so
newer versions are exercised as they ship; older ones are untested.

## Documentation

Full reference documentation is on the
[Terraform Registry](https://registry.terraform.io/providers/vanillauys/dokploy/latest/docs).
Start with the guides:

- **[Getting started](docs/guides/getting-started.md)** - configure the provider and apply a first project, database, application and domain.
- **[Adopting an existing Dokploy instance](docs/guides/adopting-an-existing-instance.md)** - import a running server without recreating anything.
- **[Deploy semantics](docs/guides/deploy-semantics.md)** - `deploy_on_change`, timeouts, and how deploys fail.
- **[Secrets and sensitive values](docs/guides/secrets.md)** - environment variables, database passwords, backup credentials.

## Before you start

Three things bite hardest, in order:

1. **API keys are rate-limited, and an exhausted budget returns `401`, not
   `429`.** A large apply can fail as an authentication error against a key
   that works fine for single requests. See
   [Getting started](docs/guides/getting-started.md#before-your-first-apply-api-key-rate-limits).
2. **`dokploy_application` and `dokploy_compose` own their whole service.**
   Applying either replaces anything changed in the Dokploy UI. Manage a
   service in Terraform or in the UI, not both. See
   [Adopting an existing Dokploy instance](docs/guides/adopting-an-existing-instance.md#decide-what-terraform-owns).
3. **MariaDB's and MongoDB's default `docker_image` does not exist on Docker
   Hub.** Set an explicit tag or every deploy fails. See
   [Deploy semantics](docs/guides/deploy-semantics.md#two-engines-whose-default-image-does-not-exist).

This provider is pre-1.0: breaking changes land in minor releases until
v1.0.0. Pin an exact version if you need stability.

## Coverage gaps

What the provider does not model yet, and why.

- **Not everything Dokploy can do is covered yet.** Databases beyond
  PostgreSQL, MySQL, MariaDB, MongoDB, Redis and LibSQL, registries,
  SSH keys, certificates, notifications, DNS providers and remote servers all
  still have to be managed in the Dokploy UI.
- **`dokploy_vault_provider` models six provider types.** Dokploy v0.30.5
  adds a seventh, `phase` (Phase.dev). The resource cannot create or update a
  Phase vault provider yet; manage one in the Dokploy UI.
- **`dokploy_backup` cannot back up Redis**, because Dokploy has no logical
  dump for it. Use `dokploy_volume_backup`, which snapshots the volume and
  does accept a Redis parent. Backing up the Dokploy instance itself
  (Dokploy's `web-server` backup type) is not exposed either: it has no
  parent service and needs its own validation path.
- **`dokploy_redis` has no `database_name`, `database_user` or
  `database_root_password` attribute.** Unlike every other database engine
  this provider supports, Redis has no per-engine credential fields at all
  beyond the shared `database_password` — this is a genuine gap in Dokploy's
  own data model for this engine, not an omission in the schema.
- **`dokploy_mongo` has no `database_name` or `database_root_password`
  attribute.** MongoDB has no separate database-name concept at create time
  and no root-password credential distinct from `database_user`/
  `database_password`. Dokploy's MongoDB support also has a `replicaSets`
  option (standalone vs. replica-set topology) that this provider does not
  currently expose as a Terraform attribute; every `dokploy_mongo` instance
  is created in the server's default standalone mode.
- **`dokploy_libsql`'s replica mode is accepted but never functionally
  verified.** `sqld_node = "replica"` and `sqld_primary_url` are modelled, and
  Dokploy's own validation rules for them are enforced at plan time, but no
  replica has been stood up against a real primary to confirm it deploys and
  replicates. A replica also cannot have any external port: Dokploy rejects
  every `saveExternalPorts` call while `sqld_node` is `replica`, regardless of
  which ports the request carries.
- **Names are not unique in Dokploy.** Every data source that looks up by
  name (project, environment, application, destination, network,
  github_provider, and all six database engines) errors when more than one
  record matches, rather than silently
  picking one. Domain hosts are not unique either (the same host may be
  attached to more than one domain); there is no `dokploy_domain` data
  source, so nothing looks domains up by host.
- **`dokploy_compose` supports GitHub App, plain git and inline sources only.**
  Dokploy also has GitLab, Bitbucket and Gitea sources, but none is modelled,
  for the same reason there is only a `dokploy_github_provider` data source: no
  instance has been available to observe their shapes against. The same gap
  applies to `dokploy_application`.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full picture (toolchain notes,
git hooks, test layout, engineering contract). Quick reference:

- `make test` — unit tests
- `./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc` — acceptance tests against a disposable Dokploy (never point these at a real instance)
- `make docs` — regenerate registry docs
- `make hooks` — enable the gitleaks pre-commit secret scan (automatic on first `make build`)
