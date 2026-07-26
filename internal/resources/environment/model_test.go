package environment

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// Dialect C has no way to store null. A never-set field reads back as JSON
// null (decoded to "") and a cleared field reads back as "". Both must
// present as a null Terraform value, or a config that omits the attribute can
// never produce an empty plan.
func TestEmptyToNull(t *testing.T) {
	if got := EmptyToNull(""); !got.IsNull() {
		t.Errorf(`EmptyToNull("") = %v, want null`, got)
	}
	if got := EmptyToNull("text"); got.ValueString() != "text" {
		t.Errorf(`EmptyToNull("text") = %v, want "text"`, got)
	}
}

// The write-side inverse: a null config value must travel as "" because
// dialect C rejects an explicit null and treats an absent key as "keep".
func TestNullToEmpty(t *testing.T) {
	if got := NullToEmpty(types.StringNull()); got != "" {
		t.Errorf(`NullToEmpty(null) = %q, want ""`, got)
	}
	if got := NullToEmpty(types.StringUnknown()); got != "" {
		t.Errorf(`NullToEmpty(unknown) = %q, want ""`, got)
	}
	if got := NullToEmpty(types.StringValue("text")); got != "text" {
		t.Errorf(`NullToEmpty("text") = %q, want "text"`, got)
	}
}

func TestFlattenMapsEmptyServerValuesToNull(t *testing.T) {
	var m resourceModel
	flatten(context.Background(), &client.Environment{
		EnvironmentID: "e1",
		Name:          "production",
		ProjectID:     "p1",
		Description:   "",
		Env:           "",
		IsDefault:     true,
	}, &m)

	if m.ID.ValueString() != "e1" {
		t.Errorf("ID = %v, want e1", m.ID)
	}
	if m.ProjectID.ValueString() != "p1" {
		t.Errorf("ProjectID = %v, want p1", m.ProjectID)
	}
	if !m.Description.IsNull() {
		t.Errorf("Description = %v, want null", m.Description)
	}
	if !m.Env.IsNull() {
		t.Errorf("Env = %v, want null", m.Env)
	}
	if !m.IsDefault.ValueBool() {
		t.Error("IsDefault = false, want true")
	}
}

// Delete must refuse a default environment before calling the API. This is a
// unit test rather than an acceptance test because the refusal is permanent:
// resource.Test's own end-of-test destroy would hit it and fail the run.
func TestDeleteBlockedReason(t *testing.T) {
	ordinary := &resourceModel{
		Name:      types.StringValue("staging"),
		IsDefault: types.BoolValue(false),
	}
	if got := deleteBlockedReason(ordinary); got != "" {
		t.Errorf("a non-default environment must be deletable, got %q", got)
	}

	def := &resourceModel{
		Name:      types.StringValue("production"),
		IsDefault: types.BoolValue(true),
	}
	got := deleteBlockedReason(def)
	if got == "" {
		t.Fatal("a default environment must be refused")
	}
	if !strings.Contains(got, "production") {
		t.Errorf("message should name the environment; got %q", got)
	}
	if !strings.Contains(got, "terraform state rm") {
		t.Errorf("message should offer the remedy; got %q", got)
	}
}
