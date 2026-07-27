# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `dokploy_mysql` resource and data source, the first of wave 2's additional
  database engines. Diverges from `dokploy_postgres` in one field:
  `database_root_password` is server-generated when left unset, settable, and
  clearable, but changing it (like `database_password`) only takes effect on
  the next deploy.

## [0.2.0] - 2026-07-27

### Added

- `dokploy_environment` resource and data source. Environments sit between a
  project and its services; wave 0 exposed them only as a read-only list on
  `dokploy_project`.
- `dokploy_domain` resource, for attaching hostnames to applications and
  compose services.
- Name-based lookup on the `dokploy_application` and `dokploy_postgres` data
  sources, via `environment_id` + `name`.
- `dogfood/`, a read-only harness that checks the provider can round-trip a
  live stack with an empty plan.

### Notes

- Dokploy refuses to delete a project's default `production` environment.
  Destroying a `dokploy_environment` with `is_default = true` fails with an
  explanatory error; use `terraform state rm`, or destroy the whole project.
- `dokploy_environment` has no `created_at`. Dokploy's read endpoint does not
  return one, though its create and update endpoints do.
- `dokploy_domain.middlewares` is read-only until the provider gains
  middleware resources.
- Environment, application and postgres names are not unique in Dokploy. The
  data sources that look them up by name error on multiple matches rather
  than picking one. Domain hosts are not unique either (the same host may be
  attached to more than one domain), but there is no `dokploy_domain` data
  source, so nothing looks a domain up by host.

## [0.1.0] - 2026-07-25

### Added

- Provider scaffold with `endpoint` / `api_key` / `insecure` configuration and `DOKPLOY_ENDPOINT` / `DOKPLOY_API_KEY` fallbacks.
- Resources: `dokploy_project`, `dokploy_application` (github/git/docker sources), `dokploy_postgres` — all with `terraform import` support.
- Data sources: `dokploy_project` (by id or name), `dokploy_application`, `dokploy_postgres`.
- Deploy-on-change engine: `deploy_on_change` (default `true`) and `deployment_timeout` (default `15m`) on service resources.

### Known limitations

- `dokploy_application` manages an application's source, build and environment
  configuration wholesale. Two Dokploy settings it does not expose — **watch
  paths** and **build secrets** — are sent as empty on every apply, so any value
  configured for them in the Dokploy UI is overwritten. The builder version for
  the `heroku_buildpacks` and `railpack` build types is likewise always reset to
  the server default. Manage an application either in Terraform or in the
  Dokploy UI, not both.
- Dokploy rate-limits API keys server-side and reports the limit as `401
  Unauthorized` rather than `429`. Large configurations may need an API key with
  rate limiting disabled.
- `terraform import` seeds the provider-only `deploy_on_change` and
  `deployment_timeout` with their schema defaults, since nothing server-side
  records them.

### Compatibility

- Developed and tested against Dokploy v0.29.13.
