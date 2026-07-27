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
- **`dokploy_application` owns the whole application.** Applying it rewrites the
  application's source, build and environment configuration wholesale. Two
  Dokploy settings the resource does not expose — **watch paths** and **build
  secrets** — are reset to empty on every apply, so values set for them in the
  Dokploy UI are lost. The builder version for the `heroku_buildpacks` and
  `railpack` build types is likewise always reset to the server default. Manage
  an application either in Terraform or in the UI, not both.
- **`terraform import` cannot recover provider-only attributes.** `deploy_on_change`
  and `deployment_timeout` exist only in Terraform, so import seeds them with
  their schema defaults (`true` / `"15m"`). Importing a resource whose config
  sets a non-default value plans one diff to reconcile it.
- **Databases other than PostgreSQL and MySQL are not covered**, nor are
  compose services or backups. Wave 0 covered projects, applications and
  PostgreSQL; wave 1 added environments and domains; wave 2 is adding the
  remaining database engines (MySQL shipped, MariaDB/MongoDB/Redis to follow).
- **MySQL's root password is server-generated when left unset**, and, like
  `database_password`, changing it only takes effect on the next deploy.
  `deploy_on_change` (default `true`) covers the common case; setting it to
  `false` means a `database_root_password` change is stored but not applied
  until a manual deploy.
- **The default `production` environment of a project cannot be deleted**
  through the API, so `terraform destroy` on an imported one fails by design.
  Remove it from state instead.
- **Environment, application and postgres names are not unique in Dokploy.**
  The data sources that look them up by name error when more than one record
  matches. Domain hosts are not unique either (the same host may be attached
  to more than one domain); there is no `dokploy_domain` data source, so
  nothing looks domains up by host.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full picture (toolchain notes,
git hooks, test layout, engineering contract). Quick reference:

- `make test` — unit tests
- `./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc` — acceptance tests against a disposable Dokploy (never point these at a real instance)
- `make docs` — regenerate registry docs
- `make hooks` — enable the gitleaks pre-commit secret scan (automatic on first `make build`)

Wave-0 scope and the full design live in the project's design spec.
