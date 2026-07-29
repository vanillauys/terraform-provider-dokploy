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
//   - libsql.create is DIALECT A. An empty body 400s naming all eleven
//     fields: name, appName, environmentId, description, databaseUser,
//     databasePassword, sqldNode, sqldPrimaryUrl, serverId, dockerImage,
//     enableNamespaces. Every one must be transmitted on every create.
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
//     provided." Clearing one port at a time works (including the last
//     remaining one, since the other two are then absent rather than null),
//     so a full clear needs three separate calls, not one.
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
package client
