# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-07-27

### Added

- `dokploy_mysql` resource and data source, the first of wave 2's additional
  database engines. Diverges from `dokploy_postgres` in one field:
  `database_root_password` is server-generated when left unset, settable, and
  clearable, but changing it (like `database_password`) only takes effect on
  the next deploy.
- `dokploy_redis` resource and data source. Redis has no `database_name`,
  `database_user` or `database_root_password` field at all — it exposes no
  credential attributes beyond the shared `database_password`; its schema is
  the uniform database attribute set with zero engine-specific additions.
- `dokploy_mariadb` resource and data source. Field-for-field identical to
  `dokploy_mysql`: `database_root_password` is server-generated when left
  unset, settable, and clearable, but changing it only takes effect on the
  next deploy. MariaDB's server-side default `docker_image` (`mariadb:6`)
  does not exist on Docker Hub; set an explicit tag such as `mariadb:11.4`.
- `dokploy_mongo` resource and data source, the last of wave 2's database
  engines. Diverges from every other engine: no `database_name` and no
  `database_root_password` field at all — its only credential attribute is
  `database_user`. Dokploy's MongoDB `replicaSets` option is not exposed as
  a Terraform attribute (every instance is created in standalone mode).
  MongoDB's server-side default `docker_image` (`mongo:15`) does not exist
  on Docker Hub; set an explicit tag such as `mongo:7`.

### Changed

- Internal: `dokploy_postgres` and the four new engines above now share one
  generic, `Kind`-parameterized resource and data-source implementation
  instead of five near-duplicates. No schema or behavior change for
  `dokploy_postgres` — its docs regenerate byte-identical.
- Names are not unique in Dokploy. Every data source that looks up by name
  (project, environment, application, and — as of this wave — all five
  database engines) errors when more than one record matches, rather than
  silently picking one.
- `domain_type` now uses `UseStateForUnknown`. Both of its inputs
  (`application_id`, `compose_id`) require replace, so it is provably
  immutable, and it no longer surfaces a spurious diff on the plan after an
  apply.
- The `env` attribute description, on `dokploy_environment` and every
  database engine, now documents that omitting it and setting it to `""` are
  indistinguishable on read — both come back null; use omission, not `""`,
  to clear it.
- Internal tooling, no user-visible effect: `make hooks` now works from a
  git worktree (it previously checked for a `.git` directory, which a
  worktree does not have); the pre-commit secret scan detects a pre-8.19
  `gitleaks` binary (missing the `git` subcommand the hook uses) and reports
  the version mismatch plainly instead of misreporting it as a found secret;
  and the `application`/`project`/`environment`/`domain`/`postgres`/
  `deployment` client tests now assert HTTP method and path, matching the
  bar every database-engine client test already held itself to.

### Fixed

- `dogfood/dry-run.sh` (this repo's live-server round-trip harness; not part
  of CI) previously could not complete for any stack containing a database
  engine resource: `terraform plan -generate-config-out` refuses to write a
  value for a `Sensitive` schema attribute, and `database_password` is both
  `Required` and `Sensitive` on every engine, so Terraform Core rejected the
  generated config before the provider ever ran. The harness now patches
  those attributes back in from the same live, read-only API it already
  calls, then continues the round-trip — this strengthens the assertion
  rather than weakening it (see `dogfood/README.md` for the full analysis).
  Not a provider bug: `dokploy_postgres` has shipped with this exact schema
  shape since wave 1; the gap was simply never exercised end-to-end against
  a live server until this wave ran the harness against a stack containing a
  database engine for the first time.

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
