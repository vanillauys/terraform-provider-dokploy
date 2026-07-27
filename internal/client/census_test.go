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
// Snapshot taken against Dokploy v0.29.13 on 2026-07-28.

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
	// application.saveEnvironment is deliberately absent: it has no request
	// struct at all yet, only an inline map[string]any built in
	// SaveApplicationEnvironment. That is precisely how its buildSecrets and
	// createEnvFile literals hid from every reflection guard in this package.
	// Wave-3 task 3 introduces the struct and registers it here.
}

// censusExempt lists endpoint fields this client deliberately does not send,
// each with the reason. An entry here is a decision on the record rather
// than a silent omission — which is the whole point of the census. Never add
// one to quiet a failure you have not understood.
var censusExempt = map[string]map[string]string{
	"application.saveGithubProvider": {
		"watchPaths":       "wave-3 task 3 closes this",
		"enableSubmodules": "wave-3 task 3 closes this",
		"triggerType":      "wave-3 task 3 closes this",
	},
	"application.saveGitProvider": {
		"enableSubmodules": "wave-3 task 3 closes this",
	},
	"application.saveBuildType": {
		"isStaticSpa": "wave-3 task 3 closes this",
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
