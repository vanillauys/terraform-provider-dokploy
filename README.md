# terraform-provider-dokploy

Terraform/OpenTofu provider for [Dokploy](https://dokploy.com).
Registry address: `vanillauys/dokploy`. Requires Terraform >= 1.5.

## Usage

```hcl
terraform {
  required_providers {
    dokploy = {
      source  = "vanillauys/dokploy"
      version = "~> 0.1"
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

Developed and tested against **Dokploy v0.29.13**. The acceptance suite installs
Dokploy with the upstream `install.sh`, which tracks the latest release, so
newer versions are exercised as they ship; older ones are untested.

## Known limitations

- **Dokploy rate-limits API keys.** Keys are rate-limited server-side by
  Dokploy's api-key plugin. When the limit is hit the API answers `401
  Unauthorized` rather than `429`, so it surfaces as an authentication failure
  rather than an obvious throttle. Applying a large configuration — or one with
  long-running deploys, which this provider polls while it waits — can exhaust
  the budget. The acceptance rig works around this by minting a key with rate
  limiting disabled (`acceptance/bootstrap.sh`). If applies fail with an
  unexpected 401 against a key that works for single requests, a key with rate
  limiting disabled is likely required. Whether keys minted through the Dokploy
  UI carry the same limit has not been verified.
- **`dokploy_application` owns the whole application.** Applying it rewrites
  the application's source, build and environment configuration wholesale, so
  anything changed in the Dokploy UI is replaced on the next apply. Manage an
  application either in Terraform or in the UI, not both. As of v0.4.0 the
  resource no longer writes any field it does not model: `watch_paths`,
  `build_secrets`, `create_env_file`, `enable_submodules`, `is_static_spa`,
  `trigger_type`, `heroku_version` and `railpack_version` are all schema
  attributes, and a pair of reflection tests
  (`TestDialectARequestsCarryNoBlindFields`,
  `TestSaveRequestsReadEveryFieldFromTheModel`) fail the build if a future
  field is added to one of these endpoints without one.
- **`terraform import` cannot recover provider-only attributes.** `deploy_on_change`
  and `deployment_timeout` exist only in Terraform, so import seeds them with
  their schema defaults (`true` / `"15m"`). Importing a resource whose config
  sets a non-default value plans one diff to reconcile it.
- **Databases other than PostgreSQL, MySQL, MariaDB, MongoDB and Redis are
  not covered**, nor are compose services or backups. Wave 0 covered
  projects, applications and PostgreSQL; wave 1 added environments and
  domains; wave 2 adds the remaining database engines (MySQL, Redis,
  MariaDB and MongoDB).
- **MySQL's and MariaDB's root password is server-generated when left
  unset**, and, like `database_password`, changing it only takes effect on
  the next deploy. `deploy_on_change` (default `true`) covers the common
  case; setting it to `false` means a `database_root_password` change is
  stored but not applied until a manual deploy.
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
- **MariaDB's and MongoDB's server-side default `docker_image` does not
  exist on Docker Hub** (`mariadb:6` / `mongo:15` as of Dokploy v0.29.13).
  Leaving `docker_image` unset on `dokploy_mariadb`/`dokploy_mongo` and then
  triggering any deploy (`external_port` changes, or an explicit deploy)
  fails with a Docker manifest-unknown error. Set an explicit, real tag
  (e.g. `mariadb:11.4`, `mongo:7`) instead of relying on the server default
  for these two engines.
- **The default `production` environment of a project cannot be deleted**
  through the API, so `terraform destroy` on an imported one fails by design.
  Remove it from state instead.
- **Names are not unique in Dokploy.** Every data source that looks up by
  name (project, environment, application, and all five database engines)
  errors when more than one record matches, rather than silently picking
  one. Domain hosts are not unique either (the same host may be attached to
  more than one domain); there is no `dokploy_domain` data source, so nothing
  looks domains up by host.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full picture (toolchain notes,
git hooks, test layout, engineering contract). Quick reference:

- `make test` — unit tests
- `./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc` — acceptance tests against a disposable Dokploy (never point these at a real instance)
- `make docs` — regenerate registry docs
- `make hooks` — enable the gitleaks pre-commit secret scan (automatic on first `make build`)

Wave-0 scope and the full design live in the project's design spec.
