package client

import (
	"reflect"
	"testing"
)

// dialectARequests are the request structs sent to endpoints where an absent
// key is an HTTP 400 — so every field is transmitted on every call, and any
// field the resource does not model is written blind on every apply.
//
// This is the STRUCTURAL half of the invariant: every such request must be a
// struct, with a complete json-tagged field set, registered in both guard
// tables. It cannot see a hardcoded value — that is the resource-layer half,
// in internal/resources/application/blind_field_test.go.
//
// Why "must be a struct" is a rule and not a style preference:
// SaveApplicationEnvironment built an inline map[string]any until wave 3,
// with buildSecrets pinned to nil and createEnvFile pinned to true. A map is
// invisible to reflection over types, so neither the omitempty guard nor the
// endpoint census could see it, and both fields were silently overwritten on
// every apply for three releases.
var dialectARequests = []any{
	SaveGithubProviderRequest{},
	SaveGitProviderRequest{},
	SaveDockerProviderRequest{},
	SaveBuildTypeRequest{},
	SaveApplicationEnvironmentRequest{},
}

func TestDialectARequestsCarryNoBlindFields(t *testing.T) {
	for _, req := range dialectARequests {
		typ := reflect.TypeOf(req)
		t.Run(typ.Name(), func(t *testing.T) {
			if typ.Kind() != reflect.Struct {
				t.Fatalf("%s is a %s, not a struct: reflection guards cannot see it", typ, typ.Kind())
			}
			for i := 0; i < typ.NumField(); i++ {
				if f := typ.Field(i); jsonName(f) == "" {
					t.Errorf("field %s has no json name: it never reaches the server, "+
						"so it cannot be traced to a schema attribute", f.Name)
				}
			}
			if !inMustAlwaysSend(typ) {
				t.Errorf("%s is not in the mustAlwaysSend table in dialect_test.go — "+
					"an omitempty on any of its fields would go uncaught", typ.Name())
			}
			if !inEndpointStructs(typ) {
				t.Errorf("%s is not in the endpointStructs table in census_test.go — "+
					"a field the server accepts but this struct lacks would go uncaught", typ.Name())
			}
		})
	}
}
