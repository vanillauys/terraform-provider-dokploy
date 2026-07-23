package tfutil

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestParseTimeout(t *testing.T) {
	d, err := ParseTimeout(types.StringValue("10m"))
	if err != nil || d != 10*time.Minute {
		t.Errorf("d=%v err=%v", d, err)
	}
	// Null falls back to the spec default of 15m.
	d, err = ParseTimeout(types.StringNull())
	if err != nil || d != 15*time.Minute {
		t.Errorf("null: d=%v err=%v", d, err)
	}
	if _, err = ParseTimeout(types.StringValue("banana")); err == nil {
		t.Error("invalid duration accepted")
	}
}

func TestDeployAttributes(t *testing.T) {
	attrs := DeployAttributes()
	for _, name := range []string{"deploy_on_change", "deployment_timeout"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
}
