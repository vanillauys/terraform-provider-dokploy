package envvars

import (
	"strings"
	"testing"
)

func TestParseAndRenderRoundTrip(t *testing.T) {
	vars, err := parseEnv("B=2\r\n# comment\n\nA=x=y\nEMPTY=\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 3 || vars["A"] != "x=y" || vars["B"] != "2" || vars["EMPTY"] != "" {
		t.Errorf("parseEnv() = %v", vars)
	}
	if got := renderEnv(vars); got != "A=x=y\nB=2\nEMPTY=" {
		t.Errorf("renderEnv() = %q", got)
	}
	if got := renderEnv(map[string]string{}); got != "" {
		t.Errorf("renderEnv(empty) = %q", got)
	}
}

func TestParseRejectsALineWithoutEquals(t *testing.T) {
	if _, err := parseEnv("A=1\nnot-a-pair"); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("parseEnv() = %v, want an error naming line 2", err)
	}
}
