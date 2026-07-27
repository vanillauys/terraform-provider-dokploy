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
package client
