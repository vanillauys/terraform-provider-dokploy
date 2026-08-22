# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.10.0] - 2026-08-22

### Added

- `dokploy_vault_provider` resource: create, update, and destroy a
  secret-vault connection Dokploy can pull runtime secrets from at deploy
  time, one of six provider types - `hashicorp` (also covers OpenBao),
  `infisical`, `aws`, `doppler`, `azure`, `scaleway` - each its own typed,
  mutually exclusive config block. `assignments` links the vault provider
  to projects and, optionally, specific environments within them; an
  empty list is legal. `verify_connection` is opt-in (default `false`)
  and, when set, calls `vaultProvider.testConnection` against the real
  vault before Create or Update writes anything, so a bad credential or
  an unreachable server fails the apply instead of creating a broken
  vault provider.

  Dokploy masks every secret field as the literal string `********` on
  every read - create, read, and update alike - so this provider cannot
  detect a config value changed in the Dokploy UI, and `terraform import`
  cannot recover a config block; the first apply after import re-writes
  it in full from configuration. A server-side defect independent of
  this - `vaultProvider.create`'s duplicate-name rejection is a raw HTTP
  500 that leaks the failed request's secrets in cleartext, observed on
  doppler and hashicorp - is guarded on two sides: a best-effort
  name-uniqueness pre-check runs before any secret reaches the server,
  and every server error text reaching a diagnostic in Create, Update,
  and the `verify_connection` check is scrubbed of every configured
  secret value first - Read and Delete carry no secrets. A scrubbed
  secret in an error message reads as `(redacted)`.

### Notes

- This closes the wave-6 slate. `vaultProvider.listSecretNames` and all
  `dnsProvider` endpoints stay unmodeled by decision: `listSecretNames`
  is read-only UI surface with no Terraform-shaped use, and DNS belongs
  to the official `cloudflare`/`aws` providers, not this one.

## [0.9.0] - 2026-08-20

### Added

- `dokploy_network` resource: create and destroy Docker networks (bridge or
  overlay) with `internal`, `attachable`, `enable_ipv4`/`enable_ipv6`,
  `mtu`, and `ipam` address pools. Networks are immutable - Dokploy has no
  `network.update` endpoint - so every attribute is `RequiresReplace` and
  changing any of them replaces the network. `network.remove` succeeds even
  while a network is still attached to an application; the reference is
  left dangling until that application is next updated or redeployed.

- `dokploy_network` data source: look up a network by id or name, for
  networks created or imported in the Dokploy UI. `ipam` is not exposed -
  a consumer needs only the id to attach a service to the network.

### Notes

- `network.import`, `network.recreate`, and `network.networksToSync` stay
  unmodeled. They are UI conveniences: `network.import` adopts a
  Docker-level network into Dokploy's database, which a Terraform resource
  cannot address until it has a Dokploy id, and Terraform already expresses
  a rebuild through replace or taint without needing `network.recreate`.
  Replace/taint and the `dokploy_network` data source cover both - see the
  resource docs for the import workaround.
- The `dnsProvider` endpoints stay unmodeled by decision: DNS belongs to
  the official `cloudflare`/`aws` providers, not this one. `vaultProvider`
  lands as `dokploy_vault_provider` in v0.10.0 (wave 6c).

## [0.8.0] - 2026-08-19

### Added

- Dokploy v0.30.0 support. The census pin for endpoint fields moves from
  v0.29.13 to v0.30.0. Wave 6a probed the acceptance rig at v0.30.2, the
  installer's current v0.30.x build at the time, and the census snapshot
  reflects that probe. `docs/index.md` now states the v0.30.0 pin. Newer
  releases stay untested until the acceptance suite exercises them.

- `network_ids` and `detach_dokploy_network` on `dokploy_application`,
  the five database engines (`dokploy_postgres`, `dokploy_mysql`,
  `dokploy_mariadb`, `dokploy_mongo`, `dokploy_redis`), and
  `dokploy_libsql`. These attributes attach a service to extra Docker
  networks beyond the default `dokploy-network`, or detach that default
  network. A network attachment change applies on the next deploy, not
  on apply - the same rule `env` and `build_secrets` already follow.
  `network_ids` rejects an empty set at plan time; omit the attribute
  instead.

  Verified live against v0.30.2, 2026-08-19: an explicit `null` request
  clears the field to a stored `null`, never back to the fresh-create
  default of `[]`. The read path maps both shapes to the same Terraform
  value, or every plan after a clear would show a spurious diff.

- `service_networks`, `create_env_file`, and `icon` on `dokploy_compose`.
  `service_networks` is compose's per-service form of `network_ids`:
  each entry names one compose service and the network ids to attach to
  it, with its own `detach_dokploy_network` toggle, and it applies on
  the next deploy too. `create_env_file` writes the environment
  variables to a `.env` file for the compose project; it defaults to
  `true`, the same as the server's fresh-create default. `icon` sets
  the service icon shown in the Dokploy UI, as an icon name or a data
  URI up to 2 MB.

  `create_env_file` on compose does not follow dialect A:
  `compose.saveEnvironment` and `compose.update` keep the stored value
  when the request omits the key. `application.saveEnvironment` returns
  an HTTP 400 on the same omission.

- `enabled` on `dokploy_domain`. `false` removes the domain's route from
  Traefik but keeps its configuration, so a later apply can re-enable
  the domain without new certificates or paths. `enabled` defaults to
  `true`, the server's own default on a bare create.

  `domain.create` cannot express `enabled = false` in one call - the
  field exists only on `domain.update`. The resource creates the domain
  enabled first, then disables it in a second call when the
  configuration sets `enabled = false`.

### Deprecated

- `isolated_deployment` and `isolated_deployments_volume` on
  `dokploy_compose`. Dokploy deprecates Isolated Deployment upstream in
  v0.30.0; use `service_networks` instead. Both attributes still work -
  Dokploy still accepts and stores them - and this provider keeps them
  until Dokploy removes them upstream.

### Notes

- Dokploy v0.30.0 introduces new `network`, `dnsProvider`, and
  `vaultProvider` routers. These are not resources in this release.
  They land as resources in v0.9.0 and v0.10.0 (waves 6b and 6c).

## [0.7.0] - 2026-08-12

### Added

- `dokploy_libsql` resource and data source: a Dokploy LibSQL (`sqld`)
  database service - a distributed SQLite database.

  It sits outside the shared `database.Kind` abstraction the other five
  engines use, even though Dokploy treats it as a database engine in every
  other sense - it has `database_user`/`database_password` and its own
  `saveEnvironment` endpoint. Three server behaviours break the shared
  abstraction: `libsql.create` returns the literal `true` rather than the
  created record, so the resource has to locate it itself the same way
  `dokploy_backup` does; `libsql.saveExternalPorts` carries three ports
  (`external_port`, `external_admin_port`, `external_grpc_port`) where
  `Kind.SaveExternalPort` only models one; and `sqld_node`, `sqld_primary_url`
  and `enable_namespaces` are fields none of the five engines have, one of
  them a bool, which `Kind.CredentialAttrs` cannot express.

  Clearing all three external ports at once takes two `saveExternalPorts`
  calls, not one: the server 400s a single request that nulls all three
  ports together ("Either externalPort, externalGRPCPort or
  externalAdminPort must be provided"), so a full clear is split
  two-then-one.

  Three cross-field rules are enforced at plan time, not left for the server
  to reject at apply: `sqld_node = "replica"` requires `sqld_primary_url`; a
  non-replica - including the default, `"primary"` - must NOT set
  `sqld_primary_url`; and a replica cannot set any of the three external
  ports at all, since Dokploy rejects every `saveExternalPorts` call while
  `sqld_node` is `replica`, regardless of which ports the request carries. A
  transition from `primary` into `replica` clears the external ports before
  flipping `sqld_node`, not after - `libsql.update` accepts the flip while
  ports are still set server-side, and flipping first would leave those
  ports permanently stuck, since a replica rejects the very call that would
  clear them.

  `app_name` is Computed-only, the only attribute in this provider that
  works this way. `libsql.create` requires a non-empty `appName` on every
  call and always appends a random, server-generated suffix to whatever it
  receives, even a caller-supplied literal, so a configuration-supplied
  value could never match what the server actually stores. The resource
  seeds the create call from `name` and reads back the server's suffixed
  value instead.

  Replica mode is modelled and its cross-field rules are enforced, but it is
  not functionally verified: no replica has been stood up against a real
  primary to confirm it actually deploys and replicates.

- Four guides on the registry: getting started, adopting an existing Dokploy
  instance, deploy semantics, and secrets and sensitive values. The
  `-generate-config-out` limitation affecting all five database engines, and
  the `dogfood/` harness that works around it, are now documented where a
  provider user will find them rather than only in `dogfood/README.md`.

### Changed

- The README's known-limitations list is redistributed. Operational warnings
  moved into the guide that covers them; schema and coverage gaps remain in
  the README under **Coverage gaps**. Nothing was dropped.
- The registry landing page (`templates/index.md.tmpl`) still carried the old
  **Known limitations** section, in phrasings the README triage above had
  already superseded, and it linked to none of the four new guides. Replaced
  with a **Guides** list and the same **Before you start** set the README
  gained. The one rule that existed nowhere else in user-facing form - that a
  database engine owns its data mount - was carried into the adopting guide
  first, and now states both halves explicitly: do not import that mount, and
  do not declare one in fresh configuration either.
- The README's guide links are relative repository paths rather than
  `registry.terraform.io/.../latest/docs/guides/...` URLs. `latest` is
  v0.6.0, which predates these guides, so every one of those links would have
  404'd until a release ships.
- `dokploy_project.example.environments[0].id` is replaced with the
  order-independent `[for e in ... : e.id if e.name == "production"][0]`
  filter across the guides and all 8 `examples/` files, and the reference
  docs are regenerated to match. `BuildEnvironments` appends environments in
  the API's response order with no sort, so `[0]` is not pinned to
  `production` and silently misroutes once a project has a second
  environment; the guides now name it as an anti-pattern, so the examples had
  to stop demonstrating it.

## [0.6.0] - 2026-07-29

### Added

- `dokploy_compose`: a Dokploy compose service - a `docker-compose` project or
  a Docker Swarm `stack`.

  Exactly one of the `github`, `git` or `raw` source blocks is required. `raw`
  carries the compose file inline; the other two fetch it from a repository.
  GitLab, Bitbucket and Gitea sources are **not** modelled, matching
  `dokploy_application` and for the same reason: no instance has been available
  to observe their shapes against.

  A `dokploy_domain` can now be attached to a compose service through
  `compose_id` and `service_name`. That pathway has existed since v0.1.0 for a
  resource that did not exist yet.

  Three server behaviours worth knowing, all found by acceptance tests going
  red rather than by reading the API:

  - `command` **replaces** the deploy invocation rather than adding to it.
    Setting it to anything that does not itself deploy the stack makes every
    deploy fail.
  - `compose_path` cannot be cleared: the server rejects an empty string, so
    the attribute is `Optional+Computed` and reverts to `./docker-compose.yml`
    rather than to null.
  - `auto_deploy` and `trigger_type` are genuinely nullable server-side, while
    `enable_submodules`, `randomize`, `isolated_deployment` and
    `isolated_deployments_volume` are not - the latter four accept a null and
    silently store `false`, so they default to `false` instead.

  `compose.deploy`, `compose.import`, `compose.randomizeCompose` and the rest
  of the imperative family are deliberately not exposed, per the standing rule
  that imperative operations are not Terraform resources.

- `dokploy_destination` **data source**: resolves an existing S3-compatible
  backup destination by `name` or by `id`.

  The use case is a shared backup target created once and referenced from
  several projects, so `dokploy_backup.destination_id` and
  `dokploy_volume_backup.destination_id` stop being hardcoded opaque ids.

  `access_key` and `secret_access_key` are deliberately **not** exposed.
  `destination.one` returns both in cleartext, but a data source exists to be
  referenced, and copying a shared target's credentials into every consumer's
  state widens their blast radius for no gain. The resource still carries
  them.

  Dokploy does not enforce name uniqueness on destinations, so an ambiguous
  name is an error naming the match count, never `[0]`.

### Changed

- An unexempted `types.StringPointerValue` in `internal/resources` or
  `internal/datasources` now fails the build. Dokploy returns a literal `""`
  for an optional string cleared through its UI where a field never set
  returns `null`, and `StringPointerValue` preserves the `""`, producing a
  `"" -> null` diff no apply can settle. `acf76ab` fixed this as a manual
  sweep in v0.4.0; nothing enforced it until now. No user-visible behaviour
  change - `dokploy_schedule`, `dokploy_backup` and `dokploy_volume_backup`
  were already correct, and now have tests saying so.

## [0.5.0] - 2026-07-28

### Added

- `dokploy_backup`: a scheduled logical dump of a database to an
  S3-compatible destination.

  It does **not** accept a `redis` parent, and rejects one at plan time with
  a message pointing at `dokploy_volume_backup` — Dokploy has no logical
  dump for Redis. Backing up the Dokploy instance itself (its `web-server`
  backup type) is also not exposed: that has no parent service and needs its
  own validation path.

  `service_type` derives Dokploy's `databaseType` and `backupType`; neither
  is exposed. Setting them independently is what allows a record whose type
  and parent disagree — `backup.update` accepts `databaseType` while
  carrying no parent field at all, so it can flip the discriminator while
  leaving every id column untouched.

  `include_encryption_key` defaults to `true` and is **always transmitted**.
  Dokploy stores `true` for a newly created backup but `false` whenever an
  update omits the key, so a request that left it out would silently turn
  encryption-key inclusion off on a record created with it on.

- `dokploy_volume_backup`: a scheduled archive of a Docker volume to an
  S3-compatible destination.

  It accepts a **redis** parent, which `dokploy_backup` will not: a volume
  snapshot copies the volume as-is and works for any service that has one,
  while a logical dump needs engine support Dokploy does not have for Redis.
  The two routers genuinely disagree about this, and the enum sets prove it.

  `service_id` and `service_type` force replacement — `volumeBackups.update`
  sets the parent column it is given without clearing the others, verified
  live, so a retarget would leave the record owned by two services.

  `enabled` defaults to `true` for the same reason as `dokploy_schedule`.
  `turn_off` is passed through to Dokploy's `turnOff` field and always sent
  concretely, because the server coerces both an absent key and an explicit
  null to `false`.

- `dokploy_schedule`: a cron job Dokploy runs against an application, a
  compose service, a remote server, or the Dokploy host itself.

  `schedule_type = "dokploy-server"` takes no `service_id` — it runs against
  the Dokploy host and has no parent service. Every other type requires one.
  Both halves are enforced at apply time, since the rule depends on another
  attribute's value and the stock config validators cannot express that.

  `enabled` defaults to `true`. Dokploy leaves it null when a schedule is
  created through the API alone, which is neither on nor off; a schedule
  declared in configuration that silently never fires is the worse failure.

  `schedule_type` and `service_id` force replacement. Dokploy's update
  endpoint sets the parent column it is given without clearing the others,
  so a retarget would leave the record owned by two parents at once — the
  same defect `dokploy_mount` documents.

## [0.4.0] - 2026-07-28

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
