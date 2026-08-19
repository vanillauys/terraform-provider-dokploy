package client

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// The census closes a gap no amount of reading this package can close.
//
// A request struct that carries a WRONG VALUE is visible in the source. A
// request struct that is MISSING A FIELD ENTIRELY is not: there is nothing
// to read. On a dialect A endpoint every key is transmitted on every call,
// so a field this client does not model is a field the server resets to its
// schema default on every apply — silently, with an HTTP 200. That is how
// watchPaths, buildSecrets, enableSubmodules, isStaticSpa and triggerType
// all came to be overwritten on every `terraform apply` without a single
// test going red.
//
// testdata/endpoint-fields.json is the server's own account of what each
// endpoint accepts, distilled from the OpenAPI document Dokploy serves at
// GET /api/trpc/settings.getOpenApiDocument (the document is real and
// complete for REQUEST bodies; its 200-response schemas are all empty
// objects, which is why this package is still hand-written — see doc.go).
//
// Regenerate it against a rig whenever the pinned Dokploy version moves:
//
//	eval "$(./acceptance/bootstrap.sh)"
//	curl -sS -H "x-api-key: $DOKPLOY_API_KEY" \
//	  "$DOKPLOY_ENDPOINT/api/trpc/settings.getOpenApiDocument" \
//	| python3 -c '
//	import json,sys
//	spec=json.load(sys.stdin)["result"]["data"]["json"]
//	out={}
//	for path,ops in spec["paths"].items():
//	    name=path.strip("/").split("/")[-1]
//	    for op in ops.values():
//	        s=op.get("requestBody",{}).get("content",{}).get("application/json",{}).get("schema",{})
//	        if s.get("properties"):
//	            out[name]={"fields":sorted(s["properties"]),"required":sorted(s.get("required",[]))}
//	json.dump(dict(sorted(out.items())),sys.stdout,indent=1)
//	' > internal/client/testdata/endpoint-fields.json
//
// Snapshot taken against Dokploy v0.30.2 on 2026-08-19.

// endpointStructs maps a Dokploy write endpoint to the request struct this
// package sends to it. Every endpoint whose absent keys are load-bearing —
// all of dialect A, and dialect B wherever a field must be clearable —
// belongs here. Adding an endpoint to the client without adding it here is
// exactly the gap this table exists to close.
var endpointStructs = map[string]any{
	"application.saveGithubProvider": SaveGithubProviderRequest{},
	"application.saveGitProvider":    SaveGitProviderRequest{},
	"application.saveDockerProvider": SaveDockerProviderRequest{},
	"application.saveBuildType":      SaveBuildTypeRequest{},
	"application.saveEnvironment":    SaveApplicationEnvironmentRequest{},
	"mounts.create":                  CreateMountRequest{},
	"mounts.update":                  UpdateMountRequest{},
	"port.create":                    CreatePortRequest{},
	"port.update":                    UpdatePortRequest{},
	"redirects.create":               CreateRedirectRequest{},
	"redirects.update":               UpdateRedirectRequest{},
	"security.create":                CreateSecurityRequest{},
	"security.update":                UpdateSecurityRequest{},
	"destination.create":             CreateDestinationRequest{},
	"destination.update":             UpdateDestinationRequest{},
	"schedule.create":                CreateScheduleRequest{},
	"schedule.update":                UpdateScheduleRequest{},
	"volumeBackups.create":           CreateVolumeBackupRequest{},
	"volumeBackups.update":           UpdateVolumeBackupRequest{},
	"backup.create":                  CreateBackupRequest{},
	"backup.update":                  UpdateBackupRequest{},
	"compose.create":                 CreateComposeRequest{},
	"compose.update":                 UpdateComposeRequest{},
	"compose.saveEnvironment":        SaveComposeEnvironmentRequest{},
	"libsql.create":                  CreateLibsqlRequest{},
	"libsql.update":                  UpdateLibsqlRequest{},
	"libsql.saveExternalPorts":       saveLibsqlExternalPortsShape{},
	"libsql.saveEnvironment":         saveLibsqlEnvironmentShape{},
}

// inEndpointStructs reports whether a request struct is registered above.
// blind_field_test.go uses it to require that every dialect A request is
// censused against the server's own field list.
func inEndpointStructs(typ reflect.Type) bool {
	for _, v := range endpointStructs {
		if reflect.TypeOf(v) == typ {
			return true
		}
	}
	return false
}

// censusExempt lists endpoint fields this client deliberately does not send,
// each with the reason. An entry here is a decision on the record rather
// than a silent omission — which is the whole point of the census. Never add
// one to quiet a failure you have not understood.
//
// Keep this list as short as you can: an exemption is a field the server
// accepts and this provider silently ignores. Wave-3 task 1 parked five
// application fields here as written-down debt and task 3 modelled all five,
// so the only entries left are ones where NOT sending the field is the
// correct behaviour rather than a gap.
var censusExempt = map[string]map[string]string{
	// mounts.update accepts every parent column plus serviceType, and
	// setting one does NOT clear the others: retargeting through this
	// endpoint leaves the record with two parents (see UpdateMountRequest
	// and doc.go for the live transcript). dokploy_mount marks its parent
	// attributes RequiresReplace instead, so the client never needs to
	// express a retarget and deliberately cannot.
	"mounts.update": {
		"serviceType":   "parent is RequiresReplace; mounts.update corrupts on retarget",
		"applicationId": "parent is RequiresReplace; mounts.update corrupts on retarget",
		"composeId":     "parent is RequiresReplace; mounts.update corrupts on retarget",
		"postgresId":    "parent is RequiresReplace; mounts.update corrupts on retarget",
		"mysqlId":       "parent is RequiresReplace; mounts.update corrupts on retarget",
		"mariadbId":     "parent is RequiresReplace; mounts.update corrupts on retarget",
		"mongoId":       "parent is RequiresReplace; mounts.update corrupts on retarget",
		"redisId":       "parent is RequiresReplace; mounts.update corrupts on retarget",
		"libsqlId":      "parent is RequiresReplace; mounts.update corrupts on retarget",
	},
	// destination.create/update accept serverId, but destination.one and
	// destination.all never return it (verified live, v0.29.13,
	// 2026-07-28). A write-only field cannot round-trip: state would hold a
	// value Read can never confirm, so either the attribute lies or every
	// plan shows a diff. Exposing it needs a read path first.
	"destination.create": {"serverId": "destination.one does not return it; a write-only field cannot round-trip"},
	"destination.update": {"serverId": "destination.one does not return it; a write-only field cannot round-trip"},
	// schedule.update accepts the parent columns but sets one without
	// clearing the others, so a retarget leaves two parents on the record.
	// schedule_type and service_id are RequiresReplace instead.
	"schedule.update": {
		"applicationId":  "parent is RequiresReplace; schedule.update corrupts on retarget",
		"composeId":      "parent is RequiresReplace; schedule.update corrupts on retarget",
		"serverId":       "parent is RequiresReplace; schedule.update corrupts on retarget",
		"appName":        "server-generated; not user configuration",
		"createdAt":      "server-generated; not user configuration",
		"organizationId": "implied by the API key's organization",
	},
	"schedule.create": {
		"appName":        "server-generated; not user configuration",
		"createdAt":      "server-generated; not user configuration",
		"organizationId": "implied by the API key's organization",
		"scheduleId":     "server-generated; the create endpoint assigns it",
	},
	// volumeBackups.update accepts serviceType and every parent column, and
	// retargeting through it leaves the record with two parents -- verified
	// live, see UpdateVolumeBackupRequest. RequiresReplace instead.
	"volumeBackups.update": {
		"serviceType":   "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"applicationId": "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"composeId":     "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"postgresId":    "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"mysqlId":       "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"mariadbId":     "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"mongoId":       "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"redisId":       "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"libsqlId":      "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"appName":       "server-generated; not user configuration",
		"createdAt":     "server-generated; not user configuration",
	},
	"volumeBackups.create": {
		"appName":   "server-generated; not user configuration",
		"createdAt": "server-generated; not user configuration",
	},
	// metadata's schema is `anyOf: [{}, null]` -- genuinely untyped -- and it
	// has read back null on every record observed live. There is no shape to
	// model and no value to preserve, so it is sent as an explicit null.
	// Modelling it needs evidence this provider does not have.
	"backup.create": {
		"metadata": "schema is untyped (anyOf [{}, null]); reads back null on every observed record",
		"userId":   "implied by the API key; the server assigns it",
	},
	// compose.update accepts the gitlab, bitbucket and gitea provider
	// columns. None is modelled, for the same reason
	// internal/datasources/gitprovider covers only GitHub: no instance
	// available to develop against has one, so their shapes would be
	// inferred rather than observed. dokploy_application has the identical
	// gap. Note this endpoint is dialect B, not A, so an unmodelled field is
	// merely unmanageable here - it is NOT reset on every apply, verified
	// live (v0.29.13, 2026-07-29) by setting thirteen fields away from their
	// defaults and issuing an update carrying only composeId and name.
	"compose.update": {
		"gitlabId":            "no gitlab provider observed live; shape would be inferred",
		"gitlabProjectId":     "no gitlab provider observed live; shape would be inferred",
		"gitlabRepository":    "no gitlab provider observed live; shape would be inferred",
		"gitlabOwner":         "no gitlab provider observed live; shape would be inferred",
		"gitlabBranch":        "no gitlab provider observed live; shape would be inferred",
		"gitlabPathNamespace": "no gitlab provider observed live; shape would be inferred",

		"bitbucketId":             "no bitbucket provider observed live; shape would be inferred",
		"bitbucketRepository":     "no bitbucket provider observed live; shape would be inferred",
		"bitbucketRepositorySlug": "no bitbucket provider observed live; shape would be inferred",
		"bitbucketOwner":          "no bitbucket provider observed live; shape would be inferred",
		"bitbucketBranch":         "no bitbucket provider observed live; shape would be inferred",

		"giteaId":         "no gitea provider observed live; shape would be inferred",
		"giteaRepository": "no gitea provider observed live; shape would be inferred",
		"giteaOwner":      "no gitea provider observed live; shape would be inferred",
		"giteaBranch":     "no gitea provider observed live; shape would be inferred",

		"appName":       "server-generated; RequiresReplace on the resource, never updated",
		"createdAt":     "server-generated; not user configuration",
		"environmentId": "RequiresReplace on the resource; compose.move is the supported retarget and is not modelled",
		"refreshToken":  "server-generated webhook token; rotating it is an imperative operation",
		"composeStatus": "server-mutable status; a deploy moves it, Terraform must not write it",
		"env":           "set through compose.saveEnvironment, which is the endpoint the Dokploy UI uses",
		"createEnvFile": "set through compose.saveEnvironment, which is the endpoint the Dokploy UI uses - same split as env",
	},
	// compose.create accepts sourceType since v0.30.0, but the resource sets
	// the source through the follow-up compose.update it already issues on
	// every create (Create's doc comment in resources/compose/resource.go).
	// Sending it twice would add a second writer for the same column.
	"compose.create": {
		"sourceType": "source is set by the follow-up compose.update the resource always issues on create",
	},
	// libsql.update is similar to compose.update: the endpoint accepts more
	// fields than this client models. The Swarm fields are not exposed in
	// Terraform (dokploy_application does not expose them either), and
	// externalAdminPort, externalGRPCPort, externalPort are managed through a
	// separate endpoint (libsql.saveExternalPorts, like compose.saveEnvironment
	// handles env).
	"libsql.update": {
		"endpointSpecSwarm":    "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"healthCheckSwarm":     "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"labelsSwarm":          "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"modeSwarm":            "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"networkSwarm":         "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"placementSwarm":       "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"restartPolicySwarm":   "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"rollbackConfigSwarm":  "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"updateConfigSwarm":    "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"stopGracePeriodSwarm": "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"appName":              "server-generated (suffixed for uniqueness on every create); Computed-only on the resource, so no config path exists to change it and libsql.update never needs to send it",
		"applicationStatus":    "server-mutable status; a deploy moves it, Terraform must not write it",
		"createdAt":            "server-generated; not user configuration",
		"env":                  "set through libsql.saveEnvironment, which is the endpoint the Dokploy UI uses",
		"environmentId":        "RequiresReplace on the resource; libsql.move is the supported retarget and is not modelled",
		"externalAdminPort":    "managed through libsql.saveExternalPorts, not in the primary update endpoint",
		"externalGRPCPort":     "managed through libsql.saveExternalPorts, not in the primary update endpoint",
		"externalPort":         "managed through libsql.saveExternalPorts, not in the primary update endpoint",
	},
}

type endpointFields struct {
	Fields   []string `json:"fields"`
	Required []string `json:"required"`
}

func loadEndpointFields(t *testing.T) map[string]endpointFields {
	t.Helper()
	raw, err := os.ReadFile("testdata/endpoint-fields.json")
	if err != nil {
		t.Fatalf("read endpoint-fields snapshot: %v", err)
	}
	var snapshot map[string]endpointFields
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("decode endpoint-fields snapshot: %v", err)
	}
	return snapshot
}

func TestEndpointFieldCensus(t *testing.T) {
	snapshot := loadEndpointFields(t)

	for endpoint, reqStruct := range endpointStructs {
		entry, ok := snapshot[endpoint]
		if !ok {
			t.Errorf("%s: absent from testdata/endpoint-fields.json — regenerate the snapshot", endpoint)
			continue
		}
		typ := reflect.TypeOf(reqStruct)
		have := make(map[string]bool, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			if name := jsonName(typ.Field(i)); name != "" {
				have[name] = true
			}
		}
		for _, field := range entry.Fields {
			if have[field] || censusExempt[endpoint][field] != "" {
				continue
			}
			t.Errorf(
				"%s: the server accepts %q but %s has no such field and no censusExempt entry.\n"+
					"On a dialect A endpoint every key is sent on every call, so an unmodelled "+
					"field is reset to its schema default on every apply, silently, with an HTTP 200. "+
					"Either model it as a schema attribute or record why not in censusExempt.",
				endpoint, field, typ.Name())
		}
	}
}

// TestCensusExemptionsAreLive keeps the exemption list honest: an entry for a
// field the struct has since grown, or for an endpoint/field the server no
// longer accepts, is stale and must go. Without this, exemptions accumulate
// into a second, invisible blind list.
func TestCensusExemptionsAreLive(t *testing.T) {
	snapshot := loadEndpointFields(t)

	for endpoint, fields := range censusExempt {
		entry, ok := snapshot[endpoint]
		if !ok {
			t.Errorf("censusExempt[%q]: endpoint is not in the snapshot", endpoint)
			continue
		}
		accepted := make(map[string]bool, len(entry.Fields))
		for _, f := range entry.Fields {
			accepted[f] = true
		}
		reqStruct, ok := endpointStructs[endpoint]
		if !ok {
			t.Errorf("censusExempt[%q]: endpoint has no entry in endpointStructs", endpoint)
			continue
		}
		typ := reflect.TypeOf(reqStruct)
		for field, reason := range fields {
			if reason == "" {
				t.Errorf("censusExempt[%q][%q]: exemptions must carry a reason", endpoint, field)
			}
			if !accepted[field] {
				t.Errorf("censusExempt[%q][%q]: the server no longer accepts this field — drop the exemption", endpoint, field)
			}
			if _, has := fieldByJSONName(typ, field); has {
				t.Errorf("censusExempt[%q][%q]: %s now carries this field — drop the exemption", endpoint, field, typ.Name())
			}
		}
	}
}
