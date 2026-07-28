# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `dokploy_github_provider` **data source**, resolving a GitHub App's name to
  the id `dokploy_application.github.github_id` expects — so that id stops
  being an opaque literal pasted into configuration.

  Two things it does not do, both deliberate. It is not a *resource*:
  Dokploy's API has no `github.create` and installing a GitHub App is a
  browser flow, so a resource would lie about being able to converge. And it
  covers GitHub only, though Dokploy also has gitlab, bitbucket and gitea
  providers — none of those has been observed live (the acceptance rig has
  no provider of any type), and inferring three response shapes from this one
  is the assumption the endpoint census exists to prevent.

  Note `id` is the `githubId`, not the `gitProviderId`. Dokploy keeps both,
  an application references the former, and passing the latter is accepted
  by validation and then fails with an HTTP 500 because the foreign key is
  only enforced at the database layer. The data source exposes the generic
  record separately as `git_provider_id`.

- `dokploy_destination` resource: an S3-compatible bucket Dokploy writes
  backups to (Cloudflare R2, AWS, DigitalOcean Spaces, MinIO, ...). The
  attribute is `provider_name`, not `provider`, because `provider` is a
  reserved meta-argument in Terraform configuration. `access_key` and
  `secret_access_key` are marked sensitive, though Dokploy stores and
  returns both in cleartext to anyone with API access.

  `destination.create` also accepts a `server_id`, which this resource does
  not expose: the read endpoints never return it, so a value written there
  could not be confirmed on refresh and every plan would show a diff.
  Exposing it needs a read path Dokploy does not currently offer.

- `dokploy_port`, `dokploy_redirect` and `dokploy_security` resources, for an
  application's published ports, Traefik regex redirects, and HTTP basic-auth
  credentials. All three share one generic implementation parameterised by a
  `Kind` descriptor: probed live, they agree on everything the resource layer
  touches (a single `applicationId` parent, flat records, a dialect A update
  requiring the full field set, and the `.delete` verb). They diverge only in
  their response envelopes, which stays in the client where it is visible.

  Two consequences worth knowing:

  - `redirects.create` and `security.create` return the literal `true`
    rather than the record, and Dokploy has no endpoint to look either up by
    its fields, so the provider identifies a newly created record by diffing
    the application's child list around the call. Creates are serialised per
    application to keep that exact; if something outside the apply creates a
    sibling at the same moment, the provider errors rather than binding to a
    record it cannot prove is its own.
  - `dokploy_security.password` is stored and returned by Dokploy in
    cleartext. The attribute is marked sensitive, but anyone with API access
    to the instance can read it.

- `dokploy_mount` resource: volume, bind and file mounts attached to an
  application, any database engine, a compose service or a libsql instance.
  Two things about it are worth knowing before you use it:

  - **`service_id` and `service_type` force replacement.** Dokploy's
    `mounts.update` sets the parent column you name *without clearing the
    others*, so retargeting through it leaves the record owned by two
    services at once (verified live, v0.29.13, 2026-07-28). The client
    cannot express a retarget at all.
  - **Database services create their own data mount.** A fresh
    `dokploy_postgres` already owns a volume mount for its data directory
    the moment it is created. That mount belongs to the server — do not
    import it or declare it here.

  Per-`type` field rules (`host_path` for `bind`, `volume_name` for
  `volume`, `content` + `file_path` for `file`) are enforced by the
  provider at plan time. They are provider policy, not a server contract:
  Dokploy accepts a `bind` mount with no host path and stores it broken.

- `dokploy_application` gains nine operational attributes on the
  `application.update` path: `auto_deploy`, `replicas`, `cpu_limit`,
  `memory_limit`, `cpu_reservation`, `memory_reservation`, `command`, `args`
  and `registry_id`. The four resource limits are **strings** in Dokploy's
  schema (Docker-style `"0.5"` / `"512m"`), not numbers. `replicas` and
  `auto_deploy` are Optional+Computed with defaults of `1` and `true`;
  Dokploy's schema has no null variant for `replicas`, so it always holds a
  concrete value. `registry_id` takes a literal id — this provider has no
  registry resource yet.

- `dokploy_application` gains eight attributes for fields it previously sent
  to the server without modelling them: top-level `watch_paths`,
  `build_secrets` (Sensitive), `create_env_file` and `enable_submodules`;
  `trigger_type` (`push` or `tag`) inside the `github` block; and
  `is_static_spa`, `heroku_version` and `railpack_version` inside `build`.

### Fixed

- **`dokploy_application` no longer overwrites four settings on every apply.**
  Dokploy's `application.save*` endpoints transmit every key on every call
  (dialect A in `internal/client/doc.go`), so any field the resource did not
  model was written blind. Verified live against v0.29.13 on 2026-07-28, the
  damage was:

  - `watch_paths` — sent as an explicit JSON null on every apply, clearing
    whatever was configured in the Dokploy UI.
  - `build_secrets` — hardcoded `nil`, same effect.
  - `create_env_file` — hardcoded `true`, so a value of `false` set in the UI
    was silently flipped back on the next apply.
  - `trigger_type` — never sent, but Dokploy applies *and writes* its schema
    default, so omitting the key overwrote the stored value rather than
    preserving it.

  Two further fields, `enable_submodules` and `is_static_spa`, were never
  wiped — Dokploy leaves them out of the endpoint's SQL `SET` list when the
  request omits them — but were unmanageable. Both are now attributes.

  The `heroku_buildpacks` / `railpack` builder version was likewise always
  reset to the server default; `heroku_version` and `railpack_version` now
  control it.

- Internal, no user-visible effect: `application.saveEnvironment` was built
  as an inline `map[string]any`, which is invisible to reflection and so hid
  its two hardcoded literals from every guard in the client package. It is
  now a struct, and three tests keep this class of bug from recurring:
  `TestEndpointFieldCensus` diffs each write endpoint's request struct
  against the server's own OpenAPI field list (distilled into
  `internal/client/testdata/endpoint-fields.json`),
  `TestDialectARequestsCarryNoBlindFields` requires every dialect A request
  to be a fully-tagged struct registered in both guard tables, and
  `TestSaveRequestsReadEveryFieldFromTheModel` builds each request from a
  fully-populated model and fails if any field comes out unset.

### Fixed

- **Optional strings that Dokploy stores as `""` no longer diff forever.**
  Dokploy represents an unset optional string two ways in the same record: a
  field never set reads back as JSON null, a field set and then cleared
  through the UI reads back as a literal `""`. The provider preserved the
  `""`, while Terraform configuration that omits the attribute holds null —
  so `description`, `env`, `build_args`, `build.build_stage`, `server_id`,
  and the `dokploy_domain` custom-resolver fields could each produce a
  `"" -> null` diff that no apply could settle.

  `internal/resources/environment` had the rule right for `env` since wave 2;
  the siblings did not. It is now `tfutil.StringOrNull`, used by every
  resource and data source on every optional-string read path.

  Invisible on the acceptance rig, which creates every record through the API
  and therefore only ever sees null. It surfaced the first time wave 3 ran
  the round-trip against a production instance whose project and
  applications had been created through the Dokploy UI: a four-resource diff
  that could not be applied away.

- `dogfood/dry-run.sh` no longer patches an EMPTY live value into generated
  configuration. `--patch-sensitive` backfills sensitive attributes that
  Terraform's config generation leaves as `null # sensitive`; writing `""`
  for a live value that is empty produced a permanent `null -> ""` diff,
  because the provider maps `""` to null on read. Only Optional sensitive
  attributes can be legitimately empty, and for those null is already the
  correct encoding. Found by the same production round-trip: `build_secrets`
  is empty on both live applications.

### Changed

- `port.one` reports a missing record as HTTP **400**, not 404 — the only
  read endpoint of six probed that does. It is now mapped to the provider's
  not-found error, so a port deleted outside Terraform is reconciled as
  drift instead of failing the next apply. Non-not-found 400s from that
  endpoint stay errors.
- `internal/client/doc.go` corrected: Dokploy *does* serve an OpenAPI
  document, at `GET /api/trpc/settings.getOpenApiDocument`. The old claim
  that it "does not serve that document at all" generalised from the
  `/api/openapi.json` routes, which do 404. Its response schemas remain
  empty objects — hence the hand-written client — but its request field
  lists are complete, and the census above consumes them.

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
  instead of five near-duplicates. No behavior or schema-shape change for
  `dokploy_postgres`; the only doc changes are the `env` attribute's clearing
  caveat (below) and its data source's description noting that any
  `Sensitive` credential attribute is exposed but marked `Sensitive` (true of
  every engine's data source; postgres itself has none).
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
