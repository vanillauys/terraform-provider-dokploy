package dsutil

import (
	"strings"
	"testing"
)

func TestFind(t *testing.T) {
	items := []string{"a", "b", "b"}
	if got, err := Find(items, func(s string) bool { return s == "a" }, "letter", "the value a"); err != nil || *got != "a" {
		t.Errorf("one match = %v, %v", got, err)
	}
	_, err := Find(items, func(s string) bool { return s == "b" }, "letter", "the value b")
	if err == nil || !strings.Contains(err.Error(), "more than one letter matches the value b") {
		t.Errorf("many matches error = %v", err)
	}
	_, err = Find(items, func(s string) bool { return s == "z" }, "letter", "the value z")
	if err == nil || !strings.Contains(err.Error(), "no letter matches the value z") {
		t.Errorf("no match error = %v", err)
	}
}

func TestChildDescriptionAndValidators(t *testing.T) {
	child := Child{
		Kind: "security", What: "a basic-auth record", Example: `data "dokploy_security" "x" {}`,
		Note: "A note.", Secret: "the password", SecretAttr: "`password`", Resource: "`dokploy_security`",
	}
	desc := child.Description()
	for _, want := range []string{
		"Looks up a basic-auth record:\n\n```terraform\ndata \"dokploy_security\" \"x\" {}\n```\n\nA note.\n\n",
		"~> **The data source does not expose the password.** `password` exists on the `dokploy_security` resource",
		"If two security records match the lookup attributes, this data source fails instead of a guess.",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("description lacks %q:\n%s", want, desc)
		}
	}
	if plain := (Child{Kind: "x", What: "w", Example: "e"}).Description(); strings.Contains(plain, "does not expose") {
		t.Error("a Child without a Secret must not render the secret note")
	}
	if !strings.HasPrefix(child.IDAttribute("application_id").GetDescription(), "Security id.") {
		t.Error("the id description must name the kind")
	}
	if !LookupString("s").IsOptional() || !LookupString("s").IsComputed() || !LookupInt64("i").IsOptional() || !LookupInt64("i").IsComputed() {
		t.Error("the lookup constructors must set Optional and Computed")
	}
	// exactly one of id and parent, one companion group, one conflict per
	// distinguishing attribute, and at least one of id and the attributes.
	if got := len(ChildValidators("service_id", []string{"service_type"}, "mount_path", "host_path")); got != 5 {
		t.Errorf("ChildValidators returned %d validators, want 5", got)
	}
	if got := len(ChildValidators("application_id", nil, "regex")); got != 3 {
		t.Errorf("ChildValidators without companions returned %d validators, want 3", got)
	}
}
