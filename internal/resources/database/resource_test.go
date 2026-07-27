package database

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestCheckCredentialsCreatable pins the carry item C3 fix: an explicit
// empty string on a Computed credential attribute must be rejected at
// CREATE (it collapses to the same wire request as leaving the attribute
// unset, but Terraform's plan promises "" while the server actually
// generates a real value — "Provider produced inconsistent result after
// apply"), while an Unknown value (the normal "let the server generate
// one" case, and what a Null value also produces) must pass through
// untouched.
func TestCheckCredentialsCreatable(t *testing.T) {
	k := Kind{
		CredentialAttrs: []CredentialAttr{
			{TFName: "database_name", Required: true}, // not Computed: never checked
			{TFName: "database_root_password", Computed: true},
		},
	}

	t.Run("Unknown Computed credential passes (server will generate one)", func(t *testing.T) {
		plan := genericModel{Credentials: map[string]types.String{
			"database_name":          types.StringValue("acc"),
			"database_root_password": types.StringUnknown(),
		}}
		resp := &resource.CreateResponse{}
		if !checkCredentialsCreatable(k, plan, resp) {
			t.Errorf("Unknown value must be creatable; diagnostics = %v", resp.Diagnostics)
		}
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("Null Computed credential passes", func(t *testing.T) {
		plan := genericModel{Credentials: map[string]types.String{
			"database_name":          types.StringValue("acc"),
			"database_root_password": types.StringNull(),
		}}
		resp := &resource.CreateResponse{}
		if !checkCredentialsCreatable(k, plan, resp) {
			t.Errorf("Null value must be creatable; diagnostics = %v", resp.Diagnostics)
		}
	})

	t.Run("known non-empty Computed credential passes", func(t *testing.T) {
		plan := genericModel{Credentials: map[string]types.String{
			"database_name":          types.StringValue("acc"),
			"database_root_password": types.StringValue("myrootpw123"),
		}}
		resp := &resource.CreateResponse{}
		if !checkCredentialsCreatable(k, plan, resp) {
			t.Errorf("a real value must be creatable; diagnostics = %v", resp.Diagnostics)
		}
	})

	t.Run("known empty string Computed credential is rejected", func(t *testing.T) {
		plan := genericModel{Credentials: map[string]types.String{
			"database_name":          types.StringValue("acc"),
			"database_root_password": types.StringValue(""),
		}}
		resp := &resource.CreateResponse{}
		if checkCredentialsCreatable(k, plan, resp) {
			t.Error("an explicit empty string must be rejected, not silently accepted")
		}
		if !resp.Diagnostics.HasError() {
			t.Error("expected an error diagnostic")
		}
	})

	t.Run("a Required (non-Computed) attribute is never checked, even if empty", func(t *testing.T) {
		plan := genericModel{Credentials: map[string]types.String{
			"database_name":          types.StringValue(""),
			"database_root_password": types.StringUnknown(),
		}}
		resp := &resource.CreateResponse{}
		if !checkCredentialsCreatable(k, plan, resp) {
			t.Errorf("only Computed attrs are this function's concern; diagnostics = %v", resp.Diagnostics)
		}
	})
}
