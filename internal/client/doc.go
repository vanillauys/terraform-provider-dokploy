// Package client is a hand-written Dokploy API client.
//
// It is hand-written on purpose, but for a narrower reason than this
// comment used to claim.
//
// The server DOES serve an OpenAPI document. It is not at any
// /api/openapi.json-style route — those 404 even with a valid key, which is
// what the earlier claim ("the running server does not serve that document
// at all") was generalising from. It is served at
//
//	GET /api/trpc/settings.getOpenApiDocument
//
// wrapped in {"result":{"data":{"json": ... }}}. Verified against v0.29.13
// on both the acceptance rig and a production instance, 2026-07-28.
//
// What that document is and is not good for:
//
//   - RESPONSE schemas are useless. Every 200 response is an empty object
//     ({"type":"object","properties":{},"additionalProperties":false}), so
//     response models cannot be generated from it. Hence the hand-written
//     structs in this package.
//   - REQUEST schemas are complete and accurate: per endpoint, the full
//     accepted field list plus which fields are required. census_test.go
//     consumes exactly that, distilled into testdata/endpoint-fields.json,
//     to catch request fields this client fails to model at all — a class
//     of bug that is invisible when reading this package's source, because
//     the evidence of a missing field is the absence of code.
//
// The behaviours that actually cost debugging time (the write dialects
// below, fields present on create but absent on read, records whose names
// look unique but are not) are still not expressible in a schema, and the
// acceptance suite remains the arbiter of record: if a live response
// disagrees with a struct here, fix the struct.
//
// # Write dialects
//
// Dokploy has three mutually incompatible conventions for "this optional
// field is absent from my request". All three were verified against a live
// instance (v0.29.13) on 2026-07-26; the postgres endpoints were
// re-verified and mysql/mariadb/mongo/redis newly probed against the same
// version on 2026-07-27 — every new endpoint matches its sibling in the
// same dialect exactly (see "Database engines" below for the field-shape
// differences that do NOT follow that pattern).
//
//	Dialect A — null-required
//	  Endpoints:  postgres.saveEnvironment, postgres.saveExternalPort,
//	              mysql.saveEnvironment, mysql.saveExternalPort,
//	              mariadb.saveEnvironment, mariadb.saveExternalPort,
//	              mongo.saveEnvironment, mongo.saveExternalPort,
//	              redis.saveEnvironment, redis.saveExternalPort,
//	              application.saveGitProvider, application.saveDockerProvider,
//	              application.saveBuildType, application.saveEnvironment
//	  absent key: HTTP 400, "expected nonoptional, received undefined"
//	  JSON null:  clears the stored value
//
//	Dialect B — silent-keep
//	  Endpoints:  project.update, application.update, postgres.update,
//	              mysql.update, mariadb.update, mongo.update, redis.update,
//	              domain.update
//	  absent key: HTTP 200 and THE OLD VALUE IS KEPT — no error at all
//	  JSON null:  clears the stored value
//
//	Dialect C — string-only
//	  Endpoints:  environment.create, environment.update
//	  absent key: HTTP 200 and the old value is kept
//	  JSON null:  HTTP 400, "expected string, received null"
//	  "":         clears the value, and is what the server then stores
//
// Dialect B is the dangerous one: it fails silently, so the symptom is not an
// error but a Terraform plan that shows the same diff forever.
//
// What this means for the structs in this package:
//
//   - A dialect A or B request field is a pointer WITHOUT `omitempty`, so a
//     nil pointer marshals to an explicit JSON null.
//   - A dialect C request field is a plain string WITHOUT `omitempty`, and
//     the caller converts a null Terraform value to "". A pointer would be
//     wrong: null is rejected outright.
//
// TestRequestStructsNeverOmitMustSendFields in dialect_test.go enforces the
// `omitempty` half of this. Add new request structs to its table.
//
// # domain.create: a fourth shape, deliberately not forced into A/B/C
//
// domain.create does not match any of the three dialects above, because it
// is a CREATE endpoint and none of A/B/C's definitions say anything about
// creating a fresh record — all three describe what an absent key means
// when a stored value already exists to keep or clear. Verified live
// (v0.29.13, 2026-07-27) against a scratch application:
//
//   - Only `host` is required — {"host":"x"} alone returns HTTP 200 with
//     every other field at its zod schema default (path "/", port 3000,
//     https false, certificateType "none", domainType "application", ...).
//   - Any other key, if OMITTED, silently takes that same schema default —
//     never a 400, and there is no old value to fall back to.
//
// An explicit JSON null on every other field was probed individually
// (single-field {"host":"x","<field>":null} calls); the result is NOT
// uniform across fields, so no single rule covers all of them:
//
//	stores a literal null:    path, internalPath, port, customCertResolver,
//	                          customEntrypoint, serviceName, domainType,
//	                          applicationId, composeId
//	coerces to its default    https (-> false), stripPath (-> false),
//	(does NOT store null):    forwardAuthEnabled (-> false)
//	rejected outright (400):  certificateType ("Invalid option: expected
//	                          one of \"letsencrypt\"|\"none\"|\"custom\"" —
//	                          it has no nullable variant in the zod schema)
//
// So domain.create's rule is "absent = schema default" for every field, but
// explicit null is three-way per field: stored verbatim for most, silently
// re-defaulted for the three booleans, and outright rejected for the one
// enum. This has no relationship to dialect A's absent-key 400, dialect B
// (no old value exists yet to keep), or dialect C (which 400s on null
// uniformly). CreateDomainRequest sidesteps the whole question by sending
// every field explicitly with a concrete value or a typed nil pointer on
// every call (see its doc comment in domain.go) rather than ever relying
// on omission or null — there is no bug here, only a gap in this file's
// classification that this paragraph closes.
//
// # Database engines: postgres, mysql, mariadb, mongo, redis
//
// All five expose the same six-endpoint shape (.create, .one, .update,
// .saveEnvironment, .saveExternalPort, .remove) and split across the same
// two dialects: .update is dialect B, .saveEnvironment/.saveExternalPort
// are dialect A. postgres was re-verified and mysql/mariadb/mongo/redis
// newly verified, all against v0.29.13 on 2026-07-27, against scratch
// records in one throwaway project+environment created and deleted through
// the API (full transcripts: wave-2 task-2 report). But the five engines
// are NOT one interchangeable shape underneath — the required/optional
// field SET at .create varies per engine, and none of it is guessable from
// postgres alone:
//
//	Engine    .create requires (besides name, environmentId)  Notable difference from postgres
//	postgres  databaseName, databaseUser, databasePassword     (baseline)
//	mysql     databaseName, databaseUser, databasePassword     + databaseRootPassword: optional at
//	                                                            create — server generates a random
//	                                                            value if omitted, never blank
//	mariadb   databaseName, databaseUser, databasePassword     same databaseRootPassword behavior as
//	                                                            mysql
//	mongo     databaseUser, databasePassword                   NO databaseName field exists at all;
//	                                                            + replicaSets (bool, defaults false)
//	redis     databasePassword                                 NO databaseUser, NO databaseName, NO
//	                                                            databaseRootPassword; redis.one's
//	                                                            response also omits the `backups`
//	                                                            array the other four all return (even
//	                                                            if empty)
//
// A "generic engine" abstraction that assumes every engine has
// {databaseName, databaseUser, databasePassword} will compile and then 400
// live on mongo (no databaseName) and redis (no databaseUser, no
// databaseName). The per-engine field set must be modelled explicitly, not
// derived from postgres by analogy.
//
// mysql.update/mariadb.update are dialect B overall (see the list above),
// but databaseRootPassword is a DIALECT C EXCEPTION within that same
// endpoint, not dialect B — verified live on isolated single-field calls
// against both engines: an absent databaseRootPassword key keeps the old
// value (200, matching dialect B), but an explicit JSON null is REJECTED
// ("expected string, received null", HTTP 400 — dialect A/B would accept
// and clear it), and only an explicit "" is accepted and stored, clearing
// the field. A struct that treats every dialect-B endpoint's fields
// uniformly (pointer, no omitempty, null clears) will 400 live the first
// time it tries to clear databaseRootPassword with null instead of "".
//
// The default dockerImage a bare .create picks is not always a real,
// pullable image tag:
//
//	postgres  postgres:18   real tag, pulls fine
//	mysql     mysql:8       real tag, pulls fine
//	mariadb   mariadb:6     DOES NOT EXIST on Docker Hub
//	mongo     mongo:15      DOES NOT EXIST on Docker Hub
//	redis     redis:8       real tag, pulls fine
//
// This is invisible until something makes the server actually attempt a
// (re)deploy. Verified live: mariadb.saveExternalPort and
// mongo.saveExternalPort, called against a record still on its default
// image, both return HTTP 500 "Error on deploy ...Error: Error response
// from daemon: manifest for mariadb:6 (or mongo:15) not found: manifest
// unknown: manifest unknown". The identical call against a record created
// with an explicit valid dockerImage (mariadb:11.4, mongo:7) succeeds for
// both engines, AND the full dialect-A round trip was completed on those
// valid-image records — set externalPort to a concrete value (200, value
// persisted), then set it to null (200, mariadb.one/mongo.one report
// externalPort: null) — so saveExternalPort's dialect-A classification for
// mariadb and mongo is fully proven, not just the absent-key-400 half.
// Plain .create and .update never trigger a deploy and succeed regardless
// of the image; .saveEnvironment against the same broken-image records
// also succeeded (HTTP 200) — in this version only saveExternalPort
// synchronously attempts a redeploy. A real .deploy call was subsequently
// probed too (wave-2 task 10, live, v0.29.13, 2026-07-27, against fresh
// scratch records left on their bare default image): mariadb.deploy and
// mongo.deploy both return the identical HTTP 500 "Error on deploy
// ...Error: Error response from daemon: manifest for mariadb:6 (or
// mongo:15) not found: manifest unknown: manifest unknown" as
// saveExternalPort. Tasks building the mariadb/mongo resources and their
// acceptance tests must not rely on the server's default image: a
// bare-default mariadb or mongo instance will 500 the moment anything
// calls saveExternalPort OR .deploy against it.
//
// # libsql: a sixth engine that does NOT fit the five-engine shape
//
// Probed live against the rig (v0.29.13, 2026-07-29, wave-5a task 3). LibSQL
// is a Dokploy service type alongside the five engines above, and the mount /
// backup / volumeBackups / schedule routers all already carry a libsqlId
// parent column for it. It has databaseUser, databasePassword and
// libsql.saveEnvironment, so it IS a database engine in Dokploy's sense - but
// three findings put it outside the shared Kind abstraction in
// internal/resources/database, and each of them was invisible from the five
// engines above:
//
//	Finding                                       Consequence
//	libsql.create returns literal `true`, not     createAndLocate is required, keyed on the
//	the record (the five engines all return       environment's libsql slice. Same shape as
//	the created record)                           backup.create, which returns literal null.
//	THREE external ports, not one:                Kind models ONE external_port, and
//	libsql.saveExternalPorts (PLURAL) carries     KindClient.SaveExternalPort takes a single
//	externalPort, externalAdminPort and           *int64. Modelling only externalPort would
//	externalGRPCPort                              leave two server-accepted fields silently
//	                                              unmanaged.
//	.create requires sqldNode (string),           Kind.CredentialAttrs is the only per-engine
//	sqldPrimaryUrl (string|null) and              schema hook, and CredentialAttr.
//	enableNamespaces (bool) on EVERY call         schemaAttribute() hardcodes
//	(it is dialect A - see below)                 schema.StringAttribute, so a bool cannot be
//	                                              expressed at all, and the two sqld fields
//	                                              are not credentials.
//
// Dialects, which also differ from the five:
//
//   - libsql.create is DIALECT A for ten of its eleven fields: name, appName,
//     environmentId, description, databaseUser, databasePassword, sqldNode,
//     sqldPrimaryUrl, serverId, enableNamespaces. dockerImage is the
//     exception, and is NOT dialect A - see CreateLibsqlRequest in libsql.go
//     for the third dialect it needs: the key may be omitted (the server
//     then applies its own default), but an explicit null 400s all the
//     same. (Corrected wave 5c task 2; the wave-5a probe that first wrote
//     this section had not yet isolated dockerImage's own behaviour from
//     the other ten fields.)
//   - libsql.update is DIALECT B, like the other five: a partial body is
//     accepted, an omitted key keeps the stored value, an explicit null
//     clears. It also returns literal `true` rather than the record.
//   - libsql.saveExternalPorts is DIALECT B, where every other engine's
//     saveExternalPort is dialect A. An omitted port key KEEPS its stored
//     value (verified: setting all three, then sending only externalPort,
//     left externalAdminPort and externalGRPCPort untouched). An explicit
//     null on one key clears that one.
//   - libsql.saveExternalPorts additionally carries a CROSS-FIELD
//     REFINEMENT: sending all three keys as explicit null 400s with
//     "Either externalPort, externalGRPCPort or externalAdminPort must be
//     provided." Two explicit nulls in one request ARE accepted (corrected
//     wave 5c task 2; the wave-5a probe had tried only one-null-at-a-time
//     and three-nulls-together, and inferred a stricter rule than the
//     server actually enforces). So a full clear needs TWO calls, not
//     three: SaveLibsqlExternalPorts in libsql.go splits it two-then-one.
//
// libsql.one reports not-found as HTTP 404 with "Libsql not found" - the
// ordinary shape, NOT port.one's 400 anomaly.
//
// A working create body, for reference (dockerImage must be a tag that
// actually pulls; the ghcr.io tag below was used for this probe):
//
//	{"name":"x","appName":"x-1","environmentId":"<env>","description":null,
//	 "databaseUser":"libsql","databasePassword":"<pw>","sqldNode":"primary",
//	 "sqldPrimaryUrl":null,"serverId":null,"enableNamespaces":false,
//	 "dockerImage":"ghcr.io/tursodatabase/libsql-server:latest"}
//
// # compose: the findings wave 5b is planned from
//
// Probed live against the rig (v0.29.13, 2026-07-29, wave-5a task 6). No
// compose code exists yet: the wave-5 spec forbids writing the struct before
// this table does, because waves 3 and 4 both found things at this stage
// that would have forced a redesign afterwards.
//
//	#  Question                              Answer                                    Consequence
//	1  Does compose.create return the        YES - the full record, flat, no            No createAndLocate. Unlike
//	   record, or null like backup.create?   envelope.                                  libsql (returns `true`) and
//	                                                                                    backup (returns null).
//	2  composeType / sourceType enums?       composeType: docker-compose | stack.       Both are closed enums and
//	                                         sourceType: git | github | gitlab |        belong in schema validators.
//	                                         bitbucket | gitea | raw.
//	3  What does compose.update require,     Only composeId. A partial body is          DIALECT B, same as
//	   and does it 400 on a partial body?    accepted; an omitted key KEEPS the         application/project/domain
//	                                         stored value, an explicit null clears.     .update. Every managed field
//	                                         It returns the record.                     must be sent on every call.
//	4  Does compose.one 404 or 400 on a      404, "Compose not found".                  Ordinary shape. NOT port.one's
//	   missing id?                                                                      400 anomaly.
//	5  How is composeFile stored for the     As a literal "" - never null, never        composeFile MUST read through
//	   non-raw source types?                 absent. It holds the YAML only when        tfutil.StringOrNull. This is a
//	                                         sourceType is raw.                         "" the server produces on a
//	                                                                                    FRESH API-CREATED record, not
//	                                                                                    only a UI-cleared one.
//	6  Does compose carry application's      NO. There is no replicas, no               Compose is NOT application with
//	   operational attributes?               memoryLimit/memoryReservation, no          a different source block. Do
//	                                         cpuLimit/cpuReservation and no             not reuse application's
//	                                         *Swarm block at all. Its own set is        operational attribute set by
//	                                         autoDeploy, triggerType, watchPaths,       analogy.
//	                                         enableSubmodules, isolatedDeployment,
//	                                         isolatedDeploymentsVolume, randomize,
//	                                         suffix, command, composePath,
//	                                         composeStatus.
//	7  Domain attached to a compose          null. Never "".                            Settles the ComposeID/
//	   service: what is applicationId?                                                  ApplicationID exemption in
//	                                                                                    tfutil's stringornull guard,
//	                                                                                    in both directions.
//
// compose.update's dialect is B at the endpoint level, but the FIELDS split
// three ways, and a single request struct has to honour all three. Probed
// live field by field (v0.29.13, 2026-07-29) by setting every field away
// from its default and then issuing an explicit null per field:
//
//	Group                        Fields                                    Struct shape
//	Dialect C - "" clears, an     command, suffix, composeFile              plain string, no omitempty.
//	explicit null is a 400                                                 Caller maps a Terraform null
//	                                                                       to "", and Read maps both ""
//	                                                                       and null back to null.
//	Min-length 1 - NEITHER null   name, composePath                         plain string, no omitempty,
//	nor "" is accepted; both                                               but the caller must never
//	400 ("Too small: expected                                              produce "". name is Required
//	string to have >=1                                                     on the resource; composePath
//	characters")                                                           is Optional+Computed with a
//	                                                                       default matching the server's
//	                                                                       ./docker-compose.yml, because
//	                                                                       it can never be cleared.
//	Closed enums - null is a      sourceType (git|github|gitlab|            plain string, no omitempty.
//	400 naming the options        bitbucket|gitea|raw),                     Always send a valid option.
//	                              composeType (docker-compose|stack)
//	Nullable - an explicit null   description, env, repository, owner,      pointer, no omitempty. A nil
//	is accepted and CLEARS        branch, githubId, customGitUrl,           marshals to explicit null.
//	                              customGitBranch, customGitSSHKeyId,
//	                              triggerType, autoDeploy, enableSubmodules,
//	                              randomize, isolatedDeployment,
//	                              isolatedDeploymentsVolume, watchPaths
//
// `command` is a deploy-command SUBSTITUTE, not an addition. Dokploy runs it
// in place of `docker compose up`, so a compose service that deploys cleanly
// moves straight to composeStatus "error" the moment command is set to
// anything that is not itself a working deploy command (verified live,
// v0.29.13, 2026-07-29, with command="echo hi" on an otherwise-working
// nginx:alpine stack). Acceptance fixtures that set it must not also deploy.
//
// The min-length group is the trap of the three: "" reads as the obvious way
// to clear a dialect C string, and for command/suffix/composeFile it is - but
// composePath and name reject it. A resource that treated all five alike 400s
// on its very first apply, which is exactly how this row was found.
//
// Two consequences worth stating outright:
//
//   - triggerType, autoDeploy and watchPaths are genuinely NULLABLE columns,
//     not merely defaulted. A bare create gives "push" and true, but an
//     explicit null stores null, and compose.one then reports null. Model
//     them as pointers; a bare bool would read a null record back as false.
//   - enableSubmodules, randomize, isolatedDeployment and
//     isolatedDeploymentsVolume are the opposite: their columns are NOT
//     NULL, and an explicit null is ACCEPTED and then silently COERCED to
//     false. Verified by setting all four true, sending null for each, and
//     reading back false. The accepted-null makes them look nullable from
//     the write side alone - only reading the record back distinguishes the
//     two groups, and getting it wrong produces a `false -> null` diff that
//     no apply can settle.
//   - compose.update has NO write-through-on-absent trap. Verified by
//     setting sourceType, composeType, composePath, triggerType, autoDeploy,
//     randomize, isolatedDeployment, isolatedDeploymentsVolume,
//     enableSubmodules, suffix, command, watchPaths and composeFile all away
//     from their defaults, then issuing an update carrying only composeId
//     and name: EVERY one survived unchanged. This is the opposite of
//     application.saveGithubProvider, whose triggerType is written from its
//     zod default whether or not the request carries it. Compose needs no
//     equivalent always-send guard for that reason - though every field the
//     resource MANAGES must still be sent on every call, or it can never be
//     cleared.
//
// One more asymmetry, and it shapes the resource: compose.create accepts only
// SEVEN fields (appName, composeFile, composeType, description, environmentId,
// name, serverId) of which just name and environmentId are required, while
// compose.update accepts FORTY-FIVE. Everything else - the whole source block,
// autoDeploy, triggerType, watchPaths, composePath, the isolation flags - is
// unreachable at create and must be set by a follow-up update, exactly as
// dokploy_application already does. Server defaults on a bare create: sourceType
// github, composeType docker-compose, composePath "./docker-compose.yml",
// autoDeploy true, triggerType push, command "", suffix "", composeFile "".
//
// # Service child resources: mounts, port, redirects, security
//
// All probed live against the rig (v0.29.13, 2026-07-28, wave-3 task 2)
// against a scratch project holding one application and one postgres. Full
// transcripts in the task report.
//
// ## mounts — dialect B, and a corrupting update path
//
//	mounts.create   requires only type ("bind"|"volume"|"file"), mountPath,
//	                serviceId (+ serviceType). Everything else is optional.
//	mounts.update   dialect B: an absent key keeps the old value (HTTP 200),
//	                an explicit null clears it.
//	mounts.remove   { mountId }
//
// Two traps, neither guessable:
//
// 1. THE SUBTYPE FIELDS ARE NOT SERVER-ENFORCED. A type="bind" mount with
// no hostPath is accepted (200, hostPath null); so is type="volume" with no
// volumeName. The server validates only the enum and mountPath. Any
// per-subtype required-field rule is therefore PROVIDER policy, enforced at
// plan time, not a server contract being mirrored — say so in the schema
// descriptions rather than implying the server would reject it.
//
// 2. mounts.update NEVER CLEARS THE OTHER PARENT COLUMNS, so retargeting a
// mount through it corrupts the record. Verified step by step on one mount
// created against an application:
//
//	after update {serviceId:<pg>, serviceType:"postgres"}
//	  -> serviceType="postgres" BUT applicationId still set, postgresId null
//	after update {postgresId:<pg>}
//	  -> postgresId set AND applicationId STILL SET — two parents at once
//
// The create/update field asymmetry is real (create takes serviceId +
// serviceType; update takes per-type columns applicationId, postgresId,
// mysqlId, mariadbId, mongoId, redisId, libsqlId, composeId), but the fix
// is not to model update's columns: it is to make the parent attributes
// RequiresReplace so update is never asked to retarget anything.
//
// 3. DATABASE ENGINES AUTO-CREATE A MOUNT NOBODY ASKED FOR. A freshly
// created postgres already owns a volume mount
// (volumeName "postgres-<appName>-data", mountPath
// "/var/lib/postgresql/18/docker") the moment .create returns. It is a
// perfectly ordinary mount — mounts.remove deletes it (200) — but nothing
// in Terraform asked for it, so anything that ENUMERATES a service's mounts
// (dogfood/generate_imports.py, an import sweep) will surface a
// server-owned object as if it were user configuration. Handle it there
// deliberately; do not let it be discovered during a migration.
//
// 4. type="file" mounts have filesystem side effects: mounts.create writes
// under /etc/dokploy/applications/<appName>/files, and a second file mount
// whose directory already exists fails with HTTP 400
// "EEXIST: file already exists, mkdir ...". Creation is not idempotent and
// not order-independent.
//
// ## port, redirects, security — dialect A, uniform where it matters
//
//	<router>.create   port: applicationId, publishedPort, targetPort,
//	                        protocol, publishMode
//	                  redirects: applicationId, regex, replacement, permanent
//	                  security: applicationId, username, password
//	<router>.update   DIALECT A — the full field set is required. A body of
//	                  {<id>} alone is HTTP 400 for all three, naming every
//	                  missing field. There is no partial update.
//	<router>.delete   NOTE the verb: .delete, not .remove (mounts uses
//	                  .remove). Takes { portId | redirectId | securityId }.
//
// Uniform: one parent (applicationId only), flat records, dialect A update,
// .delete. That is enough to justify one resource-layer engine.
//
// NOT uniform — the response envelopes, which is a client-layer problem and
// must stay visible per router:
//
//	port.create        returns the created record  { portId, ... }
//	redirects.create   returns literal `true`
//	security.create    returns literal `true`
//	port.update        returns the record
//	redirects.update   returns the record
//	security.update    returns literal `null`
//
// So for redirects and security THE CREATED ID IS NOT IN THE CREATE
// RESPONSE. It has to be recovered from application.one's embedded
// `redirects` / `security` array afterwards. Those arrays are also the only
// list endpoints: there is no redirects.all / security.all.
//
// security.password is returned in CLEARTEXT by both security.one and
// application.one. Mark it Sensitive in the schema and never log it.
//
// ## port.one reports "not found" as HTTP 400, alone in this API
//
// Probed live (v0.29.13, 2026-07-28) with a nonexistent id:
//
//	port.one          400  "Port not found"      <- the odd one out
//	redirects.one     404  "Redirect not found"
//	security.one      404  "Security not found"
//	mounts.one        404  "Mount not found"
//	application.one   404  "Application not found"
//	domain.one        404  "Domain not found"
//
// This matters beyond tidiness: a resource Read only removes itself from
// state on ErrNotFound, which the transport derives from a 404. Unmapped, a
// port deleted through the Dokploy UI would fail the next apply outright
// instead of being reconciled as drift. GetPort translates that single
// endpoint's 400-with-"not found"; the generic transport still treats 400 as
// a real error everywhere else, and GetPort keeps non-not-found 400s (zod
// validation failures) as errors so a bad request can never masquerade as a
// deleted record.
//
// # application.saveGithubProvider: triggerType, and a 500 that is really a 404
//
// Probed live, wave-3 task 2. saveGithubProvider VALIDATES a bogus githubId
// only at the database layer: an unknown value returns HTTP 500 with a
// "Failed query: update \"application\" set ..." body, not a 400 and not a
// 404. Any probe of this endpoint on a rig with no GitHub provider
// configured therefore fails for FK reasons and proves nothing about field
// semantics — use saveGitProvider (no foreign key) to learn the shared
// watchPaths/enableSubmodules behaviour, as this file's findings below did.
//
// The 500 body is still evidence: it echoes the SQL SET list, which names
// exactly the columns the endpoint writes for that request. That is how the
// following was established without a working githubId.
//
//	triggerType   enum "push"|"tag". The OpenAPI document marks it REQUIRED,
//	              but the server accepts a request without it (validation
//	              passes; the failure is the FK 500), and the SET list still
//	              contains triggerType — i.e. a zod default is applied and
//	              WRITTEN. Omitting it does not preserve the stored value; it
//	              overwrites it with the default. Model it explicitly.
//
// # The blind-field findings, corrected by live evidence
//
// Wave 3's spec predicted eight blind fields behaving alike. They do not.
// Probed individually (saveGitProvider for the shared git fields):
//
//	watchPaths        REQUIRED on saveGitProvider — omitting it is HTTP 400,
//	                  not a silent keep. The client sends it as an explicit
//	                  null on every call, which CLEARS it. This is a real,
//	                  reproducible wipe.
//	buildSecrets      saveEnvironment, sent as a hardcoded nil. Confirmed
//	                  wipe: set to "S=secret", then one ordinary apply-shaped
//	                  call returns it to null.
//	createEnvFile     saveEnvironment, sent as a hardcoded true. Confirmed
//	                  overwrite: set to false, one apply-shaped call returns
//	                  it to true.
//	enableSubmodules  optional, and NOT written when omitted — the SET list
//	                  does not contain it. The current client does not wipe
//	                  it; the field is merely unmanageable.
//	isStaticSpa       same: set to true, then a saveBuildType call omitting
//	                  it leaves it true. Unmanageable, not wiped.
//	triggerType       overwritten with the zod default, as above.
//
// So the honest split is three wipes (watchPaths, buildSecrets,
// createEnvFile), one silent default-overwrite (triggerType), and two
// merely-missing attributes (enableSubmodules, isStaticSpa). All six still
// need schema attributes — an attribute the user cannot set is a real gap —
// but only the first four are data loss, and the CHANGELOG should say so
// precisely rather than claiming eight wipes.
// # The backup plane: backup, volumeBackups, schedule
//
// Probed live against the rig (v0.29.13, 2026-07-28, wave-4 task 1).
//
// ## Dialects
//
//	backup.update         DIALECT A. A partial body 400s; every required key
//	                      is transmitted on every call. An explicit null
//	                      CLEARS serviceName, keepLatestCount and enabled.
//	volumeBackups.update  same shape: full field set required.
//	schedule.update       same shape: full field set required.
//
// ## backup.create returns NOTHING
//
// Not the record, not `true` — a literal JSON null with HTTP 200. The
// created id exists only inside the parent's embedded `backups` array
// (postgres.one, mysql.one, ...). There is no backup.all. So creating one
// requires the same locate-by-diff dance as redirects/security (see
// appchild.go's createAndLocate), keyed on the DATABASE parent rather than
// an application.
//
// schedule.create and volumeBackups.create DO return their records; only
// backup needs it.
//
// ## backup.update cannot retarget, but will happily corrupt
//
// backup.update accepts databaseType and carries NO parent field at all.
// Verified: sending {databaseType:"mysql", mysqlId:<id>} against a backup
// created on a postgres returned 200 and left databaseType="mysql",
// mysqlId=null, postgresId STILL SET — a record claiming to be a MySQL
// backup while pointing at a Postgres. Nothing rejects it and nothing
// repairs it.
//
// Hence dokploy_backup derives databaseType from its parent instead of
// exposing it, and marks both RequiresReplace. Same conclusion as
// mounts.update, reached by a different route.
//
// ## includeEncryptionKey: create says true, update says false
//
// The nastiest of the three. On backup.create the field defaults to TRUE.
// On backup.update, omitting the key OR sending an explicit null both store
// FALSE. So any update that does not send it silently turns encryption-key
// inclusion off, on a record that was created with it on.
//
// This is the wave-3 blind-field shape exactly, and the census
// (census_test.go) is what keeps it caught: the field must be modelled and
// always transmitted, never omitted.
//
// ## enabled is optional at create and REQUIRED at update
//
//	create without enabled  -> 200, stored null
//	create enabled=true     -> 200, stored true
//	create enabled=false    -> 200, stored false
//	update without enabled  -> 400 "expected nonoptional, received undefined"
//
// So a backup created through the API alone sits at null, which is neither
// on nor off, and the very next update is forced to pick one. The provider
// gives `enabled` a default of true and always sends it: a backup declared
// in configuration that silently never runs is a worse failure than one that
// runs when you did not ask.
//
// ## Enums, recovered from zod errors
//
//	backup.databaseType         postgres|mariadb|mysql|mongo|web-server|libsql
//	                            NOTE: no redis. Dokploy has no logical dump
//	                            for it.
//	volumeBackups.serviceType   application|postgres|mysql|mariadb|mongo|
//	                            redis|compose|libsql
//	                            NOTE: redis IS here. Volume snapshots work
//	                            where logical dumps do not.
//	schedule.scheduleType       application|compose|server|dokploy-server
//	schedule.shellType          bash|sh
//
// ## Listing needs a parent; there is no global list
//
//	schedule.list       requires id + scheduleType
//	volumeBackups.list  requires id + volumeBackupType
//	backup              has no list endpoint at all
//
// Discovery therefore goes through the parent record's embedded array, the
// same way ports/redirects/security already do.
//
// ## Not-found status
//
// backup.one, schedule.one and volumeBackups.one all return a proper 404
// for a missing id. No repeat of port.one's 400 (see above).
//
// # libsql, wave 5c: sqldNode, the two-call port clear, and deploy timing
//
// internal/client/libsql.go is wave 5c task 2. It builds on the wave-5a
// findings above, corrects the two of them that later probing overturned
// (see the inline corrections in the "libsql: a sixth engine" section), and
// adds the following, all probed live against the rig (v0.29.13, 2026-08-11):
//
//   - sqldNode is a closed enum: primary | replica. A value outside that set
//     400s naming both options.
//   - sqldPrimaryUrl is REQUIRED when sqldNode is "replica" - a replica
//     created with sqldPrimaryUrl null 400s. A primary accepts null there,
//     which is the normal case (a primary has no upstream to point at). The
//     reverse also holds, confirmed live only in wave 5c task 6 (v0.29.13,
//     2026-08-12 - task 5's report had flagged this direction as an
//     unverified claim, not yet probed): a NON-null sqldPrimaryUrl on a
//     sqldNode that is not "replica" 400s too, with "sqldPrimaryUrl should
//     not be provided when sqldNode is not 'replica'". So the field is
//     genuinely tied to sqldNode in both directions, not just the
//     required-for-replica direction. dokploy_libsql's ValidateConfig
//     (resource.go) catches both directions at plan time now, mirroring
//     each other: a null sqld_node counts as the non-replica branch here,
//     since the schema Default turns it into "primary" before Create ever
//     runs.
//   - A replica rejects libsql.saveExternalPorts OUTRIGHT, regardless of
//     payload - even a single-port, otherwise well-formed request 400s. Only
//     a primary's ports are reachable through that endpoint. Task 5's schema
//     must not offer external ports as configurable on a replica.
//   - The three-null rejection on saveExternalPorts (see the correction
//     above) is SYNTACTIC, not stateful: it fires from the request shape
//     alone and does not consult whether the ports are already null. A
//     libsql created bare (all three ports already unset) still 400s if
//     asked to null all three in one call. SaveLibsqlExternalPorts in
//     libsql.go handles this by splitting a full clear into two calls
//     regardless of the ports' prior state, rather than trying to predict
//     the rejection from what is already stored.
//   - The server's working default image, confirmed by a bare create that
//     omits dockerImage entirely: ghcr.io/tursodatabase/libsql-server:v0.24.32.
//     This is the same tag pinned in the wave-5a working-create-body example
//     above, now confirmed to be the actual server default rather than just
//     a tag known to pull.
//   - libsql.update was verified to genuinely mutate databaseUser,
//     databasePassword, enableNamespaces and sqldNode, each checked with a
//     value-set then a libsql.one read-back. This is unlike redis, whose
//     databaseRootPassword libsql.update-style calls silently strip - libsql
//     has no such trap on these four fields.
//   - libsql.update accepts sqldNode: "replica" even while external ports
//     are still set server-side - it does not cross-validate the two. Task
//     5's review round found this the hard way: an early Update called
//     UpdateLibsql (the flip) before syncPorts, and the flip itself always
//     SUCCEEDED; only the follow-up saveExternalPorts call - the one meant
//     to clear the now-stale ports - 400'd, because a replica rejects that
//     endpoint outright (the bullet above). Had libsql.update itself
//     rejected the flip while ports remained set, the bug would have
//     surfaced at that call instead, not at the port-clear call after it.
//     This is why resource.go's Update clears ports BEFORE flipping
//     sqldNode to "replica" (the becomingReplica branch), rather than
//     relying on the server to refuse an inconsistent combination: nothing
//     stops the server from storing sqldNode="replica" with a stale port
//     still on the record if the provider does not clear it first.
//
// ## The appName blocker: required, and always server-suffixed
//
// Wave 5c task 6's acceptance run stopped at the first libsql.create for
// every one of its five new tests, all with the same 400. Isolated and
// probed live against the same rig (v0.29.13, 2026-08-12), against a
// scratch project cleaned up immediately after:
//
//	POST /libsql.create   (appName key OMITTED entirely)
//	-> 400 {"fieldErrors":{"appName":["Invalid input: expected nonoptional,
//	        received undefined"]}}
//
//	POST /libsql.create   (appName: "")
//	-> 400 {"fieldErrors":{"appName":["Too small: expected string to have
//	        >=1 characters"]}}
//
//	POST /libsql.create   (appName: "probe6-fix")
//	-> 200 true
//	GET  /libsql.one?libsqlId=<id>
//	-> {"appName":"probe6-fix-b8aed6", ...}
//
// So appName is genuinely dialect A - a required, non-empty string, exactly
// as this section's opening list already classified it - and
// CreateLibsqlRequest's `omitempty` on that field (internal/client/
// libsql.go) was the bug: app_name is Optional+Computed with no Default, so
// a config that omits it plans an unknown value, ValueString() on an
// unknown value reads "", and omitempty then dropped the key the server
// requires.
//
// The THIRD line above is the reason the fix is not simply "always send
// app_name": the server appends its own random suffix to whatever appName
// it receives, even a caller-supplied literal like "probe6-fix" above -
// the same behavior already documented for postgres's app_name
// (internal/resources/database/optional_computed_acc_test.go) and observed
// again independently during task 6's own diagnosis
// ("probe-libsql-app" stored back as "probe-libsql-app-jaaysi"). A second
// create with the same literal appName was not probed - it did not need to
// be, since the suffix behavior alone already rules out a config-supplied
// value ever converging. A Terraform config that set app_name to a literal
// would plan a value the server can never actually store, which fails
// apply with "Provider produced inconsistent result after apply" instead
// of the create-time 400 above - a different, equally fatal error. So
// app_name is Computed-only in the schema (resource.go), never Optional:
// expandCreate (model.go) seeds the wire value from name, and the server's
// suffix is what makes the result unique; UseStateForUnknown then pins
// whatever the server actually returns.
//
// ## libsql.deploy is synchronous, like postgres.deploy
//
// Task 5 needs fetchStatus to poll applicationStatus with no
// deployment-history gate, the same pattern the five engines already use -
// and that pattern is only safe if the POST does not return before the
// deploy is actually done. Confirmed rather than assumed: against a scratch
// libsql on its bare default image, a timed libsql.deploy call took 1.124s
// wall-clock, and the very next libsql.one read reported
// applicationStatus="done" - not "running", not "idle". No polling loop is
// needed for libsql, matching postgres and unlike a hypothetical async
// engine where fetchStatus would have to guard against reading a stale
// terminal status. Verdict: DONE, the no-gate fetchStatus design holds.
//
// # v0.30.0 fields (probed 2026-08-19)
//
// Wave 6a task 1 probed the acceptance rig at v0.30.2, the installer's
// current v0.30.x build. The probe used one scratch project, its default
// environment, one application, one compose service, one postgres
// instance, one network, and one domain, plus two throwaway
// project+compose pairs and one throwaway project+postgres pair for
// follow-up null-coercion checks. Task 1 created and deleted every record
// through the API. The wave-6a task-1 report holds the full transcripts.
//
// ## env stays plaintext
//
// application.saveEnvironment stored `env: "FOO=bar"`. application.one
// then read back `env: "FOO=bar"`, the exact value, with no encryption
// and no redaction. This result matches the read-only production check
// on server.vnly.io (2026-08-19) and confirms spec §2.3's plaintext
// assumption for v0.30.
//
// ## compose createEnvFile: a new field, and not dialect A
//
// createEnvFile now exists on three compose endpoints: compose.create,
// compose.saveEnvironment, and compose.update. Application already has
// createEnvFile on the same three endpoints, but compose follows a
// different dialect:
//
//	compose.create             default on a bare create: true (matches
//	                           application's default)
//	compose.saveEnvironment    absent key: silent-keep (HTTP 200, the old
//	                           value stays). Explicit null: HTTP 200,
//	                           COERCES to false. The server never rejects
//	                           it.
//	compose.update             same rule as saveEnvironment: an absent key
//	                           keeps the old value, and an explicit null
//	                           coerces to false. Task 1 verified the
//	                           coercion on a fresh compose whose stored
//	                           value was true: true, then null, then
//	                           false - so it is a genuine coercion, not an
//	                           artifact of an already-false value.
//
// application.saveEnvironment is dialect A for createEnvFile (see "Write
// dialects" above): an absent key 400s. compose.saveEnvironment and
// compose.update are not dialect A for this field. Both keep the old
// value on an absent key instead. Model CreateEnvFile on the compose
// request structs as a plain *bool with no omitempty. Give the resource a
// Default of true, to match the server's fresh-create default. Never
// send an unintended null: the server will not reject it, it will turn
// the value off without a warning.
//
// ## networkIds and detachDokployNetwork: new on application and postgres
//
// application.create and postgres.create both now return `networkIds:
// []` and `detachDokployNetwork: false` on a bare create, with neither
// key in the request. This matches the read-only production probe on
// server.vnly.io (2026-08-19). Compose has no top-level networkIds; it
// groups network attachments per service instead (see serviceNetworks
// below).
//
// application.update and postgres.update take the same two fields and
// match each other exactly:
//
//	Request                                        Read-back
//	networkIds:["NET"],                            networkIds ["NET"],
//	  detachDokployNetwork:true                       detachDokployNetwork
//	                                                   true
//	networkIds:null,                               networkIds null (NOT
//	  detachDokployNetwork:false                      []), detachDokployNetwork
//	                                                   false
//	update with only the id and name                networkIds and
//	  (no networkIds or detach key at all)            detachDokployNetwork
//	                                                   SURVIVE unchanged -
//	                                                   dialect B
//
// Two results do not match the brief's prediction:
//
//   - The fresh-create default is `[]`. An explicit `null` clears the
//     field to a literal JSON `null`, never back to `[]`. A Read that
//     maps both shapes to the same Terraform value works. A Read that
//     treats "empty" as always meaning `[]` will show a diff on every
//     plan after the first explicit clear.
//   - detachDokployNetwork:null does not 400. It returns HTTP 200 and
//     coerces the stored value to false, even when the prior value was
//     true. Task 1 verified this on application.update first, then
//     repeated the same true-then-null sequence on postgres.update
//     against a throwaway postgres record: HTTP 200, true, then null,
//     then false. This result holds on both endpoints. The brief
//     predicted a bare-boolean 400 ("schema type is bare boolean"). The
//     server instead treats a null boolean as false, the same coercion
//     domain.update's `enabled` field and compose's `createEnvFile`
//     field both show below. The shipped struct keeps
//     DetachDokployNetwork as a plain bool, not *bool - the same
//     Replicas pattern. A plain bool can never marshal a JSON null.
//     The client can never trigger the server's silent-false coercion
//     by accident. Do not change this field to *bool. A *bool can
//     marshal a JSON null. A stray nil then turns off network
//     detachment with no warning.
//
// ## serviceNetworks and icon on compose.update
//
// Both fields are new, and neither exists on compose.create - a bare
// create returns `serviceNetworks: []` and `icon: null`.
//
//	compose.update
//	  {"serviceNetworks":[{"serviceName":"web","networkIds":["NET"],
//	    "detachDokployNetwork":false}],"icon":"lucide:cloud"}
//	  -> read-back: one entry with all three keys (the server reorders
//	     them, networkIds first, but the data stays the same), and icon
//	     "lucide:cloud" stored exactly as sent.
//	compose.update {"serviceNetworks":null,"icon":null}
//	  -> read-back: serviceNetworks null (NOT []), icon null.
//
// serviceNetworks follows the same null-vs-[] split as networkIds above.
// `[]` is the fresh-create default; `null` is what an explicit clear
// produces; the two shapes are not interchangeable on read. Model
// ServiceNetworks as a pointer to a slice of a small struct (ServiceName,
// NetworkIDs, DetachDokployNetwork), with no omitempty, so a nil pointer
// clears the field to null.
//
// ## domain enabled
//
// A domain.create request naming no `enabled` key still stores `enabled:
// true`, the server's own default. Probed live against v0.30.2
// (2026-08-19). `enabled` is not part of the domain.create request shape
// the "domain.create: a fourth shape" section above already classified -
// this is a new field, not a re-check of an earlier finding.
//
//	domain.update {..., "enabled":false}            -> stores false
//	domain.update, `enabled` omitted entirely        -> silent-keep: stays
//	                                                    false (dialect B,
//	                                                    like every other
//	                                                    domain.update
//	                                                    field)
//	domain.update {..., "enabled":null}              -> HTTP 200, coerces
//	                                                    to false. Task 1
//	                                                    verified this from
//	                                                    a true-stored
//	                                                    record: true, then
//	                                                    null, then false.
//	                                                    The brief's
//	                                                    predicted 400 does
//	                                                    not happen.
//
// `enabled` shows the same null-coerces-to-false behavior as
// detachDokployNetwork and createEnvFile above. It is not a fourth
// dialect on its own. The shipped struct keeps Enabled as a plain
// bool, not *bool - the same Replicas-pattern decision as
// DetachDokployNetwork above. A plain bool can never marshal a JSON
// null. A stray nil can never silently disable a domain. Give the
// resource an Optional attribute with a Default of true, to match
// domain.create's server default.
//
// domain.toggleEnable exists and flips `enabled` in one call
// ({"domainId":"..."} only). A live check confirmed it: a domain at
// enabled:false flipped to enabled:true, and the response carried one
// extra field, requiresRedeploy. toggleEnable is REDUNDANT with
// `enabled` on domain.update. This provider deliberately does not model
// it as a client method or a resource action. Spec §3.5 asked for a
// censusExempt entry for toggleEnable, but census exemptions only cover
// endpoints the census itself walks, and the domain endpoints are not
// censused at all. This paragraph, not a censusExempt entry, is the
// correct record of the decision.
//
// ## A pattern across three fields: a null boolean coerces, it never 400s
//
// Three unrelated fields on three unrelated endpoints showed the same
// behavior, each confirmed live from a true-stored record:
// detachDokployNetwork (application.update, and separately
// postgres.update against its own throwaway record), createEnvFile
// (compose.saveEnvironment, and compose.update), and enabled
// (domain.update). None of the three rejects an explicit JSON null with
// HTTP 400. All three accept the null (HTTP 200) and silently store
// false, even when the prior stored value was true. No bare-boolean
// field probed this wave produced dialect A's "expected nonoptional,
// received undefined" 400. Treat every
// bare-boolean field this wave adds as null-coerces-to-false by default.
// Confirm the opposite live before a later task assumes a 400 exists
// anywhere on this surface.
//
// ## Wave 6b network probes (probed 2026-08-20)
//
// Task 1 probed the acceptance rig at v0.30.2, the same installer build
// wave 6a used. The probe used five scratch networks (probe-bare,
// probe-full, probe-nulls, a rejected duplicate probe-bare, and
// probe-overlay), one scratch project, and one scratch application. Every
// record was created and deleted through the API; the wave-6b task-1
// report holds the full transcripts.
//
// network.create returns the full record. network.one and network.all
// return the identical shape: networkId, name, driver, internal,
// attachable, enableIPv4, enableIPv6, mtu, ipam, createdAt,
// organizationId, serverId. All three read paths agreed on every probe in
// this wave - no create-only or read-only field turned up.
//
// v0.30.3 adds dockerId to that shape on all three read paths alike
// (probed 2026-09-01 against a fresh v0.30.3 rig). The field holds the
// Docker engine's own network id, and it reads as null on an API-created
// network before a deploy attaches it. The server uses it to detect an
// out-of-band delete or recreate of the underlying Docker network. This
// client leaves dockerId unmodeled; the JSON decoder ignores unknown
// keys. v0.30.3 also adds network.resync, a mutation with input
// {networkId}. That is an operational action, not Terraform-shaped
// state, so it stays unmodeled next to network.recreate and
// network.import.
//
// A bare create ({"name":"probe-bare"}) stores driver "bridge", internal
// false, attachable false, enableIPv4 true, enableIPv6 false, mtu null,
// and serverId null. Every one of these matches the plan's predicted
// defaults exactly - no surprises on the bool/driver group this wave.
//
// serverId is present on every read (gate A: yes). network.create,
// network.one, and network.all all return "serverId":null on a bare
// create that sends no serverId key. Task 2 keeps ServerID *string on
// both Network and CreateNetworkRequest; no censusExempt entry is needed
// for this field.
//
// mtu written as 1400 on a full create reads back as 1400 exactly, on
// both the create response and a follow-up network.one. No coercion, no
// rounding.
//
// ## ipam: an omitted key and an explicit null are not the same shape (gate D)
//
// A fully populated ipam
// ({"driver":"default","config":[{"subnet":"172.28.0.0/16",
// "gateway":"172.28.0.1","ipRange":"172.28.5.0/24"}]}) round-trips
// verbatim: same driver string, same config array, all three inner
// fields intact on both the create response and network.one. Only the
// key order changes (config before driver on read, driver before config
// on the request) - JSON object order, not a content difference. Gate D
// passes the way the plan hoped: ipam is safe to model as Optional and
// NOT Computed.
//
// The surprise is what "unset" looks like. An OMITTED ipam key on a bare
// create reads back as an EMPTY OBJECT, {} - not null, and not a
// materialized Docker default (no driver key, no config key at all
// inside it). An EXPLICIT ipam:null on create reads back as a literal
// null, the shape the plan expected. Task 1 confirmed both on separate
// creates (probe-bare omitted the key; probe-nulls sent
// {"mtu":null,"ipam":null,"serverId":null} explicitly, HTTP 200, and
// read back mtu null, ipam null, serverId null - the trio's nullable
// tolerance holds for all three, as predicted).
//
// This does not change Task 2's client: CreateNetworkRequest.IPAM carries
// no omitempty, so a nil pointer always marshals a literal JSON null, the
// probe-nulls shape, never the bare-curl probe-bare shape. The {} shape
// is only reachable by omitting the key entirely from the HTTP body,
// which this Go client never does - encoding/json has no way to omit a
// field except omitempty, and this field does not carry it. Task 3's
// flattenIPAM only needs to handle nil (-> types.ObjectNull) and a
// populated object; the {} shape never reaches it through this client.
//
// ## network.one 404 and network.remove while attached (gate B)
//
// network.one with a bogus id returns HTTP 404, body
// {"message":"Network not found","code":"NOT_FOUND","data":{"code":
// "NOT_FOUND","httpStatus":404,"path":"network.one","zodError":null}} -
// the same tRPC-OpenAPI not-found shape this client already classifies
// as ErrNotFound. Read's RemoveResource path (destination's pattern)
// applies unchanged.
//
// network.remove does NOT refuse to delete a network still referenced by
// an application's networkIds. Task 1 attached probe-full to a scratch
// application (application.update {"applicationId":"...","networkIds":
// ["<probe-full id>"]}, confirmed on the following application.one), then
// called network.remove on that same network id: HTTP 200, and the
// response body is the full deleted record, not a bare true. (This does
// not change Task 2's DeleteNetwork - it discards the response body
// regardless - but an acceptance test asserting the remove response
// should expect the record, not `true`.) The very next network.one on
// that id 404s: the network is genuinely gone. But the application record
// was never touched - a follow-up application.one still reported
// networkIds: ["<the now-deleted id>"], an orphaned reference the server
// does nothing to clean up. Task 3's resource description and Delete docs
// should say so: destroying a dokploy_network still attached to an
// application does not fail and does not detach it; the application
// keeps a dangling id in networkIds until it is next updated or
// redeployed.
//
// ## duplicate names are rejected, not accepted
//
// The plan's open question - whether the data source needs a
// never-take-[0] guard for duplicate names - assumed duplicates might be
// allowed. They are not. A second network.create with the same name as
// an existing network ("probe-bare" again) returned HTTP 400:
// {"message":"(HTTP code 409) unexpected - network with name probe-bare
// already exists ","code":"BAD_REQUEST","data":{"code":"BAD_REQUEST",
// "httpStatus":400,"path":"network.create","zodError":null}} - the
// underlying Docker 409 wrapped into a tRPC 400. Network names are unique
// server-wide, not merely within one project or environment. Task 4's
// data source can trust a name lookup to return at most one match; no
// never-take-[0] guard is needed. A name collision at resource-create
// time surfaces as a normal apply-time error from CreateNetwork.
//
// This probe ran against the host daemon only, on a single-server rig.
// Docker enforces name uniqueness per daemon, not per install: a
// multi-server install runs a separate daemon on each remote server, so
// the same name can exist on two different servers even though this
// single-daemon probe never observed a collision. The claim above ("no
// never-take-[0] guard is needed") holds only within one server; Task 4's
// data source keeps the guard anyway, and its multi-match error tells the
// caller to narrow with server_id or look the network up by id.
//
// ## overlay is accepted (gate C)
//
// {"name":"probe-overlay","driver":"overlay"} returned HTTP 200 with a
// full record: driver "overlay", the same bool/mtu/ipam defaults as a
// bridge bare-create. The rig's inner dockerd runs swarm, as predicted,
// and overlay creation needed no field beyond name and driver. Spec risk
// 6 does not fire this wave: Task 3 should add the overlay acceptance
// test, not skip it and fall back to bridge-only coverage.
//
// ## Wave 6c vault probes (probed 2026-08-22)
//
// Task 1 probed the acceptance rig at v0.30.2, the same installer build
// waves 6a and 6b used. The probe used one scratch project
// (wave6c-probe), one scratch environment, and one dev vault: OpenBao,
// run as a sibling container inside the rig's dind sandbox, joined to
// the dokploy-network overlay. Task 1 created and deleted five vault
// provider records across four provider types (hashicorp, doppler,
// infisical, scaleway). The wave-6c task-1 report holds the full
// transcripts.
//
// ## Gate B: the dev vault answers at its container name, on the first try
//
// The rig did not need the plan's fallback (--network host,
// http://127.0.0.1:8200). `docker run --network dokploy-network
// openbao/openbao:latest` joined the same overlay network Dokploy's own
// containers use. vaultProvider.testConnection reached it straight away
// at http://acc-vault:8200 - HTTP 200, body true. Task 3's StartRigVault
// helper can use this address with no extra network setup.
//
// testConnection's two failure shapes, both HTTP 400 with different
// messages:
//
//	wrong token       {"message":"HashiCorp Vault: token validation
//	                  failed (status 403)","code":"BAD_REQUEST",...}
//	unreachable url   {"message":"fetch failed","code":"BAD_REQUEST",...}
//
// Neither failure comes back as a 5xx or a timeout. Both are ordinary
// tRPC 400s with a message field verify_connection can surface directly.
// The "fetch failed" text names no host and no DNS detail - the resource
// description should say plainly that "fetch failed" usually means a bad
// URL, not a bad credential.
//
// ## Gate V: fake credentials are accepted; create never contacts the vault
//
// vaultProvider.create does not validate config against the target vault
// at all. A doppler create with serviceToken "dp.st.fake" returned HTTP
// 200 with a full record. The same held for infisical and scaleway
// creates with obviously invalid client and project ids. The server only
// calls the vault when verify_connection explicitly asks it to, through
// testConnection (gate B above). Create itself is a metadata write. This
// is the PASS branch: all six types can get full lifecycle acceptance
// coverage with fake credentials, not just stub-server unit tests.
//
// ## Gate R: config is redacted on every read, never echoed in cleartext
//
// Every secret field comes back as the literal string "********", on
// create's response, vaultProvider.one, and vaultProvider.all alike.
// hashicorp.token and doppler.serviceToken were confirmed on the
// original probe run; infisical.clientSecret and scaleway.secretKey were
// confirmed on a follow-up probe against two fresh fake records
// (probe-infisical-2, probe-scaleway-2), each checked on both its create
// response and a vaultProvider.one read-back - all four fields showed
// the identical mask, same as hashicorp and doppler. Non-secret config
// fields - mount, siteUrl, secretPath, region, apiUrl, url, projectId,
// clientId, environmentSlug - come back in cleartext. This is REDACT,
// not ECHO. Task 2's VaultProvider.Config must model only what actually
// comes back; the masked string cannot decode into anything useful.
// Task 3's Read must keep the config blocks from state rather than try
// to refresh them - the plan's gate R REDACT branch already lists the
// consequences (Sensitive attributes, no ImportStateVerify on config, a
// schema note that secret drift is undetectable).
//
// One surprise gate R does not cover: vaultProvider.create and .one both
// also return a providerType field at the TOP level of the record, one
// step above config.providerType. The two always agreed in every probe.
// Task 2's VaultProvider struct should carry ProviderType string at the
// top level too - it costs nothing and confirms the config union decoded
// to the right type.
//
// ## Read shapes: create returns the full record; one 404; one write-through proof
//
// vaultProvider.create returns the full record, not a bare true:
// vaultProviderId, name, providerType, config, assignments,
// organizationId, createdAt. .one, .all, and the bodies of .create and
// .update all agree on this field set. No create-only or read-only field
// turned up.
//
// assignments also echoes a field it was never sent: a create request
// with an entry of only {"projectId":"X"} (no environmentIds key at all)
// reads back as {"projectId":"X","environmentIds":[]}. This is a
// separate fact from gate E below (which asks whether the assignments
// ARRAY itself can be empty) - here a single ASSIGNMENT's environmentIds
// gets a server-stored empty-set default. Together the two facts make
// environment_ids safe to model as Optional+Computed with an empty-set
// Default.
//
// vaultProvider.one with a bogus id returns the ordinary shape: HTTP
// 404, {"message":"Vault provider not found","code":"NOT_FOUND",...} -
// the same tRPC-OpenAPI not-found envelope this client already
// classifies as ErrNotFound. No repeat of port.one's 400 anomaly.
//
// vaultProvider.update returns the full record too, and it genuinely
// mutates the stored secret, even though every read masks it. Task 1
// proved this rather than assumed it: it renamed a hashicorp record and
// changed its token to a value the real dev vault would reject, then
// called testConnection with only vaultProviderId (no config) - this
// reads the STORED config server-side, and it failed with the same
// "token validation failed (status 403)" gate B recorded above. A
// masked-on-read update can look like a no-op from the client's side. It
// is not one.
//
// ## Update accepts a full type swap; no RequiresReplace is needed
//
// Tested pair: hashicorp -> doppler. An update that changed a hashicorp
// record's config block outright to a doppler block - a different
// providerType, an entirely different field set - succeeded with HTTP
// 200. The read-back cleanly dropped every hashicorp-only field: no
// orphaned url, mount, or token, only the doppler fields remained. Only
// this one pair was probed; the other five type-to-type combinations
// were not tried individually, but all six share the same update
// endpoint and the same "config: any" wire shape, so there is no reason
// to expect a different type pair to behave differently. This settles
// Task 3's open question: the six config blocks can update in place.
// The blocks need no RequiresReplace plan modifier.
//
// ## Defaults: mount, siteUrl, secretPath, region, apiUrl all match the plan
//
// Task 1 probed every server-stored default gate V's PASS made
// reachable, by omitting the field on create and reading it back:
//
//	hashicorp.mount        -> "secret"
//	infisical.siteUrl      -> "https://app.infisical.com"
//	infisical.secretPath   -> "/"
//	scaleway.region        -> "fr-par"
//	scaleway.apiUrl        -> "https://api.scaleway.com"
//
// No coercion, no surprise value, no null in place of a default.
// hashicorp.namespace, the one optional field with no documented
// default, does not appear in the response at all when the request
// omits it - not null, simply absent from the JSON object. This matches
// Task 2's plan to model it as a plain string with omitempty on write,
// and to leave it unset on read when absent.
//
// ## Gate E: assignments: [] is accepted
//
// A create request with "assignments":[] returned HTTP 200 and echoed
// assignments: [] on every later read. assignments is
// Required-but-may-be-empty, not effectively min-1. Task 3's schema
// needs no length validator on it.
//
// ## Duplicate names: rejected, but through a 500 that leaks the fake secret
//
// A second create that reused the name "probe-fake" did not get the
// clean 400 wave 6b's network.create duplicate-name probe found. It
// returned HTTP 500, {"code":"INTERNAL_SERVER_ERROR",...}, with a
// message field that carries the raw failed SQL INSERT statement AND
// its bound parameters - including the request's serviceToken, in plain
// text, unmasked. The same probe was repeated on a second, unrelated
// type (hashicorp, name "probe-dup-hashicorp", fake url and token) to
// check whether the leak was a doppler-only artifact: it was not. The
// second type's failed create leaked its fake token in cleartext in the
// identical params-list shape. Observed types: doppler and hashicorp;
// the remaining four (infisical, aws, azure, scaleway) were not probed
// for this specific path, but the leak clearly comes from a
// type-agnostic layer (the SQL insert error handler, which runs after
// config validation has already succeeded for whichever type was sent)
// rather than from any per-type code, so there is no reason to expect
// the other four to behave differently. The server does reject duplicate
// names (a unique constraint on vault_provider.name), but the rejection
// path skips whatever logic redacts config on the success paths above.
//
// This matters past tidiness. Task 3's Create must AddError with the
// server's message verbatim on failure, the same pattern
// TestVaultConnection already uses. On a duplicate-name collision
// specifically, that puts the plaintext secret from the FAILED request
// straight into a Terraform diagnostic - a CI log or a terminal
// scrollback then keeps it. The provider cannot suppress this without
// breaking the general "surface the server's error text" contract every
// other endpoint relies on. This is a server-side defect, not a client
// bug to route around. Task 3's resource package doc should record it
// plainly - this paragraph is the client-layer record of it - so the
// duplicate-name path does not get treated as an ordinary validation
// error when the resource's error handling and acceptance tests get
// written.
//
// ## listSecretNames: a flat array of "path:key" strings
//
// vaultProvider.listSecretNames?vaultProviderId=&projectId= returned
// ["probe/demo:FOO"] for the one secret seeded in the dev vault (bao kv
// put -mount=secret probe/demo FOO=bar). The shape is a flat array of
// colon-joined path:key strings, not a tree and not per-path key lists.
// This confirms the plan's decision to leave the endpoint unmodeled: it
// is read-only UI surface, and one probe is enough to know its shape
// without a client method for it.
//
// ## Gate verdicts
//
// Gate V: PASS. Create accepts fake credentials for every provider type
// tried; it never contacts the real vault.
// Gate R: REDACT. Every secret field reads back masked as "********", on
// create, one, all, and update alike.
// Gate B: PASS. The sibling OpenBao container answers testConnection at
// http://acc-vault:8200 directly, with no network workaround.
// Gate E: PASS. assignments: [] is accepted on create and echoed back as
// [].
//
// # v0.30.5 census (probed 2026-09-04)
//
// The pin moved from v0.30.3 to v0.30.5 against a fresh v0.30.5 rig. The
// regenerated endpoint census differs from the v0.30.3 snapshot in four
// places, and the upstream v0.30.3...v0.30.5 router diff agrees with all
// four. None touches a request struct this client transmits.
//
//   - compose.deploy and compose.redeploy accept an optional boolean
//     freshVolumes. When true, the server runs `docker compose down
//     --volumes` before the deploy, on docker-compose stacks only. This
//     client sends composeId alone, so the server default (false) applies
//     and every deploy keeps its volumes. A destructive one-shot deploy
//     option is not Terraform-shaped state; it stays unmodeled.
//   - application.deployNginxQuickstart is new: the demo deploy behind the
//     cloud onboarding wizard. Not modeled.
//   - dnsProvider.createRecord and dnsProvider.updateRecord accept proxied,
//     and their type enum widens from A/CNAME to nine record types. The
//     dnsProvider surface stays unmodeled by decision (CHANGELOG v0.9.0).
//   - notification.createGotify, updateGotify, createNtfy, and updateNtfy
//     accept serverThreshold. No notification endpoint is modeled.
//
// Two changes do not show in the census because they live below the
// request schema.
//
// ## vaultProvider: a seventh type, phase
//
// vaultProvider.create's config is an untyped object in the OpenAPI
// document, so the census cannot see the discriminated union grow. v0.30.5
// adds providerType "phase" (Phase.dev): required token, appId, and env;
// optional path (default "/") and apiUrl (default
// "https://api.phase.dev"). Probed live: a create with fake credentials
// returned HTTP 200 and the full record; token reads back as "********"
// on the create response and on vaultProvider.one, the same REDACT shape
// as the six existing types, and the top-level providerType echoes
// "phase". testConnection against the stored fake config fails with the
// ordinary HTTP 400 {"message":"fetch failed"} shape. An unknown
// providerType is rejected with HTTP 400 ("No matching discriminator").
// This client models the six original types only. VaultProvider.Config is
// a json.RawMessage, so a phase record decodes without error, but
// dokploy_vault_provider has no phase block: its ExactlyOneOf validator
// accepts only the six existing blocks, so the resource can import a phase
// record (Read finds no populated block and logs nothing) but can never
// create or update one. Modeling phase is a wave item, not a pin bump.
//
// ## Database deploys now wait for swarm convergence
//
// deployPostgres, deployMysql, deployMariadb, deployMongo, deployRedis, and
// deployLibsql call waitForSwarmServiceConvergence after the build: the
// swarm service's tasks must reach running (45s timeout, 2s poll) before
// applicationStatus moves to done. On failure the deploy endpoint itself
// returns HTTP 500 with "Service <appName> did not converge within
// 45000ms: 0/1 tasks running (last state: ...)" and the record's
// applicationStatus reads error. Probed live: postgres.deploy with
// dockerImage alpine:3.20 (an image that exists but exits at once)
// returned the 500 after 56s and left status error; the same call with
// the default postgres:18 returned HTTP 200 after 36s, status done. Before
// v0.30.5 a database deploy reported done as soon as the swarm service
// was created or updated, whatever its tasks then did. For this provider
// the change is visible only as a better failure: the engine's Deploy call
// returns the 500 and deployAndWait surfaces it, where the same
// misconfiguration used to end in a false done.
//
// ## Other v0.30.5 changes, checked and not relevant
//
// compose.one no longer embeds git provider secrets in its nested
// github/gitlab/bitbucket/gitea relations, and github.one strips them for
// callers who are not the provider's owner or an organization owner or
// admin. This client decodes neither nested object. The compose build
// command now pins --project-directory only when the stack has mounts, and
// passes --env-file next to the compose file when createEnvFile is set;
// both are server-side build details with no API shape. schedule.update in
// cloud mode replaces the repeatable job instead of adding one; the
// self-hosted path is unchanged.
package client
