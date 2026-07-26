// Package client is a hand-written Dokploy API client.
//
// It is hand-written on purpose. Dokploy publishes an OpenAPI document in
// which every 200-response schema is an empty object
// ({"type":"object","properties":{},"additionalProperties":false}), and the
// running server does not serve that document at all — every
// /api/openapi.json-style route 404s even with a valid key. So response
// models cannot be generated, and the behaviours that actually cost
// debugging time (the write dialects below, fields present on create but
// absent on read, records whose names look unique but are not) are not
// expressible in a schema anyway. The acceptance suite is the arbiter of
// record: if a live response disagrees with a struct here, fix the struct.
//
// # Write dialects
//
// Dokploy has three mutually incompatible conventions for "this optional
// field is absent from my request". All three were verified against a live
// instance (v0.29.13) on 2026-07-26.
//
//	Dialect A — null-required
//	  Endpoints:  postgres.saveEnvironment, postgres.saveExternalPort,
//	              application.saveGitProvider, application.saveDockerProvider,
//	              application.saveBuildType, application.saveEnvironment
//	  absent key: HTTP 400, "expected nonoptional, received undefined"
//	  JSON null:  clears the stored value
//
//	Dialect B — silent-keep
//	  Endpoints:  project.update, application.update, postgres.update,
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
package client
