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
- **Databases other than PostgreSQL are not covered**, nor are compose services,
  domains, or backups. Wave 0 covers projects, applications and PostgreSQL.

## Development

- `make test` — unit tests
- `./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc` — acceptance tests against a disposable Dokploy (never point these at a real instance)
- `make docs` — regenerate registry docs

Wave-0 scope and the full design live in the project's design spec.
