package envvars

import (
	"reflect"
	"testing"
)

// FuzzParseEnv checks two properties of the .env codec on arbitrary text:
// parseEnv never panics, and a text it accepts survives a round trip
// through renderEnv unchanged. Read depends on the second property: it
// compares the rendered text of the state with the text Dokploy stores.
func FuzzParseEnv(f *testing.F) {
	for _, seed := range []string{
		"",
		"A=1",
		"B=2\r\n# comment\n\nA=x=y\nEMPTY=\n",
		" K = v ",
		"A=1\nnot-a-pair",
		"#only\n",
		"=novalue",
		"A=b\r\r",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		vars, err := parseEnv(text)
		if err != nil {
			return
		}
		again, err := parseEnv(renderEnv(vars))
		if err != nil {
			t.Fatalf("renderEnv output does not parse: %v", err)
		}
		if !reflect.DeepEqual(vars, again) {
			t.Fatalf("round trip changed the variables: %v != %v", vars, again)
		}
	})
}
