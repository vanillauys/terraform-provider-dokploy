package telemetryprovider

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestExtraConfigUpdate(t *testing.T) {
	u2 := "u2"
	for name, tc := range map[string]struct {
		plan, prior map[string]string
		want        map[string]*string
	}{
		"null plan clears":       {plan: nil, prior: map[string]string{"a": "1"}, want: nil},
		"dropped key sends null": {plan: map[string]string{"username": "u2"}, prior: map[string]string{"tenantId": "t1", "username": "u1"}, want: map[string]*string{"tenantId": nil, "username": &u2}},
		"no prior":               {plan: map[string]string{"username": "u2"}, want: map[string]*string{"username": &u2}},
	} {
		if got := extraConfigUpdate(tc.plan, tc.prior); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: extraConfigUpdate = %v, want %v", name, got, tc.want)
		}
	}
}

func TestSecretPrefersPlainThenWriteOnly(t *testing.T) {
	if got := secret(types.StringValue("plain"), types.StringValue("wo")); got == nil || *got != "plain" {
		t.Errorf("plain and wo = %v, want plain", got)
	}
	if got := secret(types.StringNull(), types.StringValue("wo")); got == nil || *got != "wo" {
		t.Errorf("wo only = %v, want wo", got)
	}
	if got := secret(types.StringNull(), types.StringNull()); got != nil {
		t.Errorf("neither = %v, want nil, which clears the stored value", *got)
	}
}
