package client

import (
	"reflect"
	"strings"
	"testing"
)

// mustAlwaysSend lists, per request struct, the JSON field names that have to
// reach the server on every call.
//
// See doc.go for why. In short: for dialect A an absent key is an HTTP 400,
// and for dialect B an absent key silently keeps the stored value. Neither
// ever means "clear this". So `omitempty` on any field listed here makes the
// corresponding Terraform attribute impossible to set back to null, which
// surfaces as a plan that shows the same diff forever.
//
// When you add a request struct for a dialect A or B endpoint, add it here.
var mustAlwaysSend = []struct {
	value  any
	fields []string
}{
	{UpdateProjectRequest{}, []string{"description"}},
	{UpdatePostgresRequest{}, []string{"description"}},
	{UpdateApplicationRequest{}, []string{"description"}},
	{UpdateDomainRequest{}, []string{
		"host", "path", "internalPath", "port", "https", "stripPath",
		"certificateType", "customCertResolver", "customEntrypoint",
		"serviceName", "forwardAuthEnabled", "domainType",
		"applicationId", "composeId",
	}},
}

func TestRequestStructsNeverOmitMustSendFields(t *testing.T) {
	for _, tc := range mustAlwaysSend {
		typ := reflect.TypeOf(tc.value)
		for _, want := range tc.fields {
			field, ok := fieldByJSONName(typ, want)
			if !ok {
				t.Errorf("%s: no field carries json name %q", typ.Name(), want)
				continue
			}
			if hasOmitempty(field) {
				t.Errorf(
					"%s.%s has `omitempty` on json:%q.\n"+
						"See internal/client/doc.go: for this endpoint an absent key is "+
						"either an HTTP 400 or a silent keep-the-old-value, never a clear. "+
						"With omitempty a null Terraform value vanishes from the request "+
						"body, so the attribute can never be cleared and every subsequent "+
						"plan shows the same diff.",
					typ.Name(), field.Name, want)
			}
		}
	}
}

func fieldByJSONName(t reflect.Type, name string) (reflect.StructField, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if strings.Split(f.Tag.Get("json"), ",")[0] == name {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

func hasOmitempty(f reflect.StructField) bool {
	parts := strings.Split(f.Tag.Get("json"), ",")
	for _, p := range parts[1:] {
		if p == "omitempty" {
			return true
		}
	}
	return false
}
