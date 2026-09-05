package vaultprovider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// blockSecrets names the secret field(s) of each config block.
var blockSecrets = map[string][]string{
	"hashicorp": {"token"},
	"infisical": {"client_secret"},
	"aws":       {"access_key_id", "secret_access_key"},
	"doppler":   {"service_token"},
	"azure":     {"client_secret"},
	"scaleway":  {"secret_key"},
}

// TestSchema_WriteOnlyCompanions pins the D1(a) shape inside the six config
// blocks, runs the framework's own schema checks (which only an acceptance
// run reached before), and pins that each block's attribute-type map in
// model.go agrees with the schema: a mismatch fails flattenConfig at apply.
func TestSchema_WriteOnlyCompanions(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&vaultProviderResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
	attrTypes := map[string]map[string]attr.Type{
		"hashicorp": hashicorpAttrTypes(), "infisical": infisicalAttrTypes(), "aws": awsAttrTypes(),
		"doppler": dopplerAttrTypes(), "azure": azureAttrTypes(), "scaleway": scalewayAttrTypes(),
	}
	for block, secrets := range blockSecrets {
		nested, ok := resp.Schema.Attributes[block].(schema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%s is %T", block, resp.Schema.Attributes[block])
		}
		if want := (types.ObjectType{AttrTypes: attrTypes[block]}); !want.Equal(nested.GetType()) {
			t.Errorf("%s: the model's attribute types %v differ from the schema's %v", block, want, nested.GetType())
		}
		for _, secret := range secrets {
			plain, ok := nested.Attributes[secret].(schema.StringAttribute)
			if !ok || plain.Required || !plain.Optional || !plain.Sensitive {
				t.Errorf("%s.%s must be Optional+Sensitive, got %+v", block, secret, nested.Attributes[secret])
			}
			wo, ok := nested.Attributes[secret+"_wo"].(schema.StringAttribute)
			if !ok || !wo.WriteOnly || !wo.Sensitive || !wo.Optional {
				t.Errorf("%s.%s_wo must be Optional+WriteOnly+Sensitive, got %+v", block, secret, nested.Attributes[secret+"_wo"])
			}
			if _, ok := nested.Attributes[secret+"_wo_version"].(schema.Int64Attribute); !ok {
				t.Errorf("%s.%s_wo_version is %T", block, secret, nested.Attributes[secret+"_wo_version"])
			}
		}
	}
}

// TestExpandConfig_WriteOnlyToken pins that the config block's companion
// supplies the wire secret when the plan's plain field is null, and that
// flattenConfig then leaves the plain field null with the plan's version.
func TestExpandConfig_WriteOnlyToken(t *testing.T) {
	ctx := context.Background()
	plan := resourceModel{
		Hashicorp: types.ObjectValueMust(hashicorpAttrTypes(), map[string]attr.Value{
			"url":              types.StringValue("https://vault.example.com:8200"),
			"token":            types.StringNull(),
			"token_wo":         types.StringNull(),
			"token_wo_version": types.Int64Value(3),
			"namespace":        types.StringNull(),
			"mount":            types.StringValue("secret"),
		}),
		Infisical: types.ObjectNull(infisicalAttrTypes()),
		AWS:       types.ObjectNull(awsAttrTypes()),
		Doppler:   types.ObjectNull(dopplerAttrTypes()),
		Azure:     types.ObjectNull(azureAttrTypes()),
		Scaleway:  types.ObjectNull(scalewayAttrTypes()),
	}
	config := plan
	config.Hashicorp = types.ObjectValueMust(hashicorpAttrTypes(), map[string]attr.Value{
		"url":              types.StringValue("https://vault.example.com:8200"),
		"token":            types.StringNull(),
		"token_wo":         types.StringValue("s.write-only"),
		"token_wo_version": types.Int64Value(3),
		"namespace":        types.StringNull(),
		"mount":            types.StringValue("secret"),
	})
	var diags diag.Diagnostics
	cfg, _ := expandConfig(ctx, plan, config, &diags)
	if diags.HasError() {
		t.Fatalf("expandConfig: %v", diags)
	}
	c, ok := cfg.(*client.VaultHashicorpConfig)
	if !ok || c.Token != "s.write-only" {
		t.Fatalf("expanded = %+v, want the write-only token on the wire", cfg)
	}

	out := plan
	flattenConfig(ctx, cfg, &out, &diags)
	if diags.HasError() {
		t.Fatalf("flattenConfig: %v", diags)
	}
	var got hashicorpModel
	diags.Append(out.Hashicorp.As(ctx, &got, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		t.Fatalf("As: %v", diags)
	}
	if !got.Token.IsNull() || !got.TokenWo.IsNull() || got.TokenWoVersion.ValueInt64() != 3 {
		t.Errorf("flattened = %+v, want token null, token_wo null, version 3", got)
	}
}

// blockWithNullCompanions builds a config block from the plain fields only:
// every write-only companion the block's attribute types declare is filled
// with null, the value it holds in a plan and in the state.
func blockWithNullCompanions(attrTypes map[string]attr.Type, values map[string]attr.Value) types.Object {
	for name, typ := range attrTypes {
		if _, ok := values[name]; ok {
			continue
		}
		switch typ {
		case types.StringType:
			values[name] = types.StringNull()
		case types.Int64Type:
			values[name] = types.Int64Null()
		}
	}
	return types.ObjectValueMust(attrTypes, values)
}

// TestExpandFlattenConfig_RoundTrip covers all six provider types: build a
// resourceModel with exactly one block populated, expand it into the typed
// client struct, check the discriminator, flatten that struct back into a
// fresh model, and confirm the same block round-trips with the other five
// left null.
func TestExpandFlattenConfig_RoundTrip(t *testing.T) {
	ctx := context.Background()

	t.Run("hashicorp", func(t *testing.T) {
		obj := blockWithNullCompanions(hashicorpAttrTypes(), map[string]attr.Value{
			"url":       types.StringValue("https://vault.example.com:8200"),
			"token":     types.StringValue("s.faketoken"),
			"namespace": types.StringValue("admin/"),
			"mount":     types.StringValue("secret"),
		})
		m := resourceModel{
			Hashicorp: obj,
			Infisical: types.ObjectNull(infisicalAttrTypes()),
			AWS:       types.ObjectNull(awsAttrTypes()),
			Doppler:   types.ObjectNull(dopplerAttrTypes()),
			Azure:     types.ObjectNull(azureAttrTypes()),
			Scaleway:  types.ObjectNull(scalewayAttrTypes()),
		}
		var diags diag.Diagnostics
		cfg, providerType := expandConfig(ctx, m, resourceModel{}, &diags)
		if diags.HasError() {
			t.Fatalf("expandConfig diags = %v", diags)
		}
		if providerType != client.VaultProviderTypeHashicorp {
			t.Fatalf("providerType = %q, want %q", providerType, client.VaultProviderTypeHashicorp)
		}
		c, ok := cfg.(*client.VaultHashicorpConfig)
		if !ok {
			t.Fatalf("cfg is %T, want *client.VaultHashicorpConfig", cfg)
		}
		if c.URL != "https://vault.example.com:8200" || c.Token != "s.faketoken" ||
			c.Namespace != "admin/" || c.Mount != "secret" || c.ProviderType != client.VaultProviderTypeHashicorp {
			t.Fatalf("expanded = %+v", c)
		}

		var out resourceModel
		flattenConfig(ctx, cfg, &out, &diags)
		if diags.HasError() {
			t.Fatalf("flattenConfig diags = %v", diags)
		}
		assertOnlyBlockSet(t, out, "hashicorp")

		var got hashicorpModel
		diags.Append(out.Hashicorp.As(ctx, &got, basetypes.ObjectAsOptions{})...)
		if diags.HasError() {
			t.Fatalf("As diags = %v", diags)
		}
		if got.URL.ValueString() != "https://vault.example.com:8200" ||
			got.Token.ValueString() != "s.faketoken" ||
			got.Namespace.ValueString() != "admin/" ||
			got.Mount.ValueString() != "secret" {
			t.Errorf("flattened hashicorp = %+v", got)
		}
	})

	t.Run("hashicorp with namespace omitted flattens to null, not empty string", func(t *testing.T) {
		obj := blockWithNullCompanions(hashicorpAttrTypes(), map[string]attr.Value{
			"url":       types.StringValue("https://vault.example.com:8200"),
			"token":     types.StringValue("s.faketoken"),
			"namespace": types.StringNull(),
			"mount":     types.StringValue("secret"),
		})
		m := resourceModel{Hashicorp: obj, Infisical: types.ObjectNull(infisicalAttrTypes()), AWS: types.ObjectNull(awsAttrTypes()), Doppler: types.ObjectNull(dopplerAttrTypes()), Azure: types.ObjectNull(azureAttrTypes()), Scaleway: types.ObjectNull(scalewayAttrTypes())}
		var diags diag.Diagnostics
		cfg, _ := expandConfig(ctx, m, resourceModel{}, &diags)
		c := cfg.(*client.VaultHashicorpConfig)
		if c.Namespace != "" {
			t.Fatalf("expanded namespace = %q, want empty (omitempty on write)", c.Namespace)
		}

		var out resourceModel
		flattenConfig(ctx, cfg, &out, &diags)
		var got hashicorpModel
		diags.Append(out.Hashicorp.As(ctx, &got, basetypes.ObjectAsOptions{})...)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if !got.Namespace.IsNull() {
			t.Errorf("flattened namespace = %v, want null", got.Namespace)
		}
	})

	t.Run("infisical", func(t *testing.T) {
		obj := blockWithNullCompanions(infisicalAttrTypes(), map[string]attr.Value{
			"site_url":         types.StringValue("https://app.infisical.com"),
			"client_id":        types.StringValue("client-id"),
			"client_secret":    types.StringValue("client-secret"),
			"project_id":       types.StringValue("proj-id"),
			"environment_slug": types.StringValue("dev"),
			"secret_path":      types.StringValue("/"),
		})
		m := resourceModel{Hashicorp: types.ObjectNull(hashicorpAttrTypes()), Infisical: obj, AWS: types.ObjectNull(awsAttrTypes()), Doppler: types.ObjectNull(dopplerAttrTypes()), Azure: types.ObjectNull(azureAttrTypes()), Scaleway: types.ObjectNull(scalewayAttrTypes())}
		var diags diag.Diagnostics
		cfg, providerType := expandConfig(ctx, m, resourceModel{}, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if providerType != client.VaultProviderTypeInfisical {
			t.Fatalf("providerType = %q", providerType)
		}
		c, ok := cfg.(*client.VaultInfisicalConfig)
		if !ok {
			t.Fatalf("cfg is %T", cfg)
		}
		if c.ClientID != "client-id" || c.ClientSecret != "client-secret" || c.ProjectID != "proj-id" ||
			c.EnvironmentSlug != "dev" || c.SiteURL != "https://app.infisical.com" || c.SecretPath != "/" {
			t.Fatalf("expanded = %+v", c)
		}

		var out resourceModel
		flattenConfig(ctx, cfg, &out, &diags)
		assertOnlyBlockSet(t, out, "infisical")
	})

	t.Run("aws", func(t *testing.T) {
		obj := blockWithNullCompanions(awsAttrTypes(), map[string]attr.Value{
			"region":            types.StringValue("us-east-1"),
			"access_key_id":     types.StringValue("AKIAFAKE"),
			"secret_access_key": types.StringValue("fake-secret"),
			"endpoint":          types.StringNull(),
		})
		m := resourceModel{Hashicorp: types.ObjectNull(hashicorpAttrTypes()), Infisical: types.ObjectNull(infisicalAttrTypes()), AWS: obj, Doppler: types.ObjectNull(dopplerAttrTypes()), Azure: types.ObjectNull(azureAttrTypes()), Scaleway: types.ObjectNull(scalewayAttrTypes())}
		var diags diag.Diagnostics
		cfg, providerType := expandConfig(ctx, m, resourceModel{}, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if providerType != client.VaultProviderTypeAWS {
			t.Fatalf("providerType = %q", providerType)
		}
		c, ok := cfg.(*client.VaultAWSConfig)
		if !ok {
			t.Fatalf("cfg is %T", cfg)
		}
		if c.Region != "us-east-1" || c.AccessKeyID != "AKIAFAKE" || c.SecretAccessKey != "fake-secret" || c.Endpoint != "" {
			t.Fatalf("expanded = %+v", c)
		}

		var out resourceModel
		flattenConfig(ctx, cfg, &out, &diags)
		assertOnlyBlockSet(t, out, "aws")
	})

	t.Run("doppler", func(t *testing.T) {
		obj := blockWithNullCompanions(dopplerAttrTypes(), map[string]attr.Value{
			"service_token": types.StringValue("dp.st.fake"),
			"project":       types.StringValue("my-project"),
			"config":        types.StringValue("dev"),
		})
		m := resourceModel{Hashicorp: types.ObjectNull(hashicorpAttrTypes()), Infisical: types.ObjectNull(infisicalAttrTypes()), AWS: types.ObjectNull(awsAttrTypes()), Doppler: obj, Azure: types.ObjectNull(azureAttrTypes()), Scaleway: types.ObjectNull(scalewayAttrTypes())}
		var diags diag.Diagnostics
		cfg, providerType := expandConfig(ctx, m, resourceModel{}, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if providerType != client.VaultProviderTypeDoppler {
			t.Fatalf("providerType = %q", providerType)
		}
		c, ok := cfg.(*client.VaultDopplerConfig)
		if !ok {
			t.Fatalf("cfg is %T", cfg)
		}
		if c.ServiceToken != "dp.st.fake" || c.Project != "my-project" || c.Config != "dev" {
			t.Fatalf("expanded = %+v", c)
		}

		var out resourceModel
		flattenConfig(ctx, cfg, &out, &diags)
		assertOnlyBlockSet(t, out, "doppler")
	})

	t.Run("azure", func(t *testing.T) {
		obj := blockWithNullCompanions(azureAttrTypes(), map[string]attr.Value{
			"vault_uri":     types.StringValue("https://myvault.vault.azure.net/"),
			"tenant_id":     types.StringValue("tenant-id"),
			"client_id":     types.StringValue("client-id"),
			"client_secret": types.StringValue("client-secret"),
		})
		m := resourceModel{Hashicorp: types.ObjectNull(hashicorpAttrTypes()), Infisical: types.ObjectNull(infisicalAttrTypes()), AWS: types.ObjectNull(awsAttrTypes()), Doppler: types.ObjectNull(dopplerAttrTypes()), Azure: obj, Scaleway: types.ObjectNull(scalewayAttrTypes())}
		var diags diag.Diagnostics
		cfg, providerType := expandConfig(ctx, m, resourceModel{}, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if providerType != client.VaultProviderTypeAzure {
			t.Fatalf("providerType = %q", providerType)
		}
		c, ok := cfg.(*client.VaultAzureConfig)
		if !ok {
			t.Fatalf("cfg is %T", cfg)
		}
		if c.VaultURI != "https://myvault.vault.azure.net/" || c.TenantID != "tenant-id" ||
			c.ClientID != "client-id" || c.ClientSecret != "client-secret" {
			t.Fatalf("expanded = %+v", c)
		}

		var out resourceModel
		flattenConfig(ctx, cfg, &out, &diags)
		assertOnlyBlockSet(t, out, "azure")
	})

	t.Run("scaleway", func(t *testing.T) {
		obj := blockWithNullCompanions(scalewayAttrTypes(), map[string]attr.Value{
			"project_id": types.StringValue("proj-id"),
			"secret_key": types.StringValue("fake-key"),
			"region":     types.StringValue("fr-par"),
			"api_url":    types.StringValue("https://api.scaleway.com"),
		})
		m := resourceModel{Hashicorp: types.ObjectNull(hashicorpAttrTypes()), Infisical: types.ObjectNull(infisicalAttrTypes()), AWS: types.ObjectNull(awsAttrTypes()), Doppler: types.ObjectNull(dopplerAttrTypes()), Azure: types.ObjectNull(azureAttrTypes()), Scaleway: obj}
		var diags diag.Diagnostics
		cfg, providerType := expandConfig(ctx, m, resourceModel{}, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if providerType != client.VaultProviderTypeScaleway {
			t.Fatalf("providerType = %q", providerType)
		}
		c, ok := cfg.(*client.VaultScalewayConfig)
		if !ok {
			t.Fatalf("cfg is %T", cfg)
		}
		if c.ProjectID != "proj-id" || c.SecretKey != "fake-key" || c.Region != "fr-par" || c.APIURL != "https://api.scaleway.com" {
			t.Fatalf("expanded = %+v", c)
		}

		var out resourceModel
		flattenConfig(ctx, cfg, &out, &diags)
		assertOnlyBlockSet(t, out, "scaleway")
	})
}

// assertOnlyBlockSet checks that exactly the named block is non-null in m
// and every other one of the six is null - flattenConfig's "nulled sibling
// blocks" guarantee.
func assertOnlyBlockSet(t *testing.T, m resourceModel, want string) {
	t.Helper()
	blocks := map[string]types.Object{
		"hashicorp": m.Hashicorp,
		"infisical": m.Infisical,
		"aws":       m.AWS,
		"doppler":   m.Doppler,
		"azure":     m.Azure,
		"scaleway":  m.Scaleway,
	}
	for name, obj := range blocks {
		if name == want {
			if obj.IsNull() {
				t.Errorf("block %q is null, want set", name)
			}
			continue
		}
		if !obj.IsNull() {
			t.Errorf("block %q is set, want null (sibling of %q)", name, want)
		}
	}
}

func TestRedactSecrets(t *testing.T) {
	t.Run("replaces a single secret", func(t *testing.T) {
		got := redactSecrets(`params: id,name,{"token":"s.superfake"}`, []string{"s.superfake"})
		want := `params: id,name,{"token":"(redacted)"}`
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("replaces every occurrence and every secret in the list", func(t *testing.T) {
		got := redactSecrets("secret-a appears twice: secret-a. secret-b appears once: secret-b.", []string{"secret-a", "secret-b"})
		want := "(redacted) appears twice: (redacted). (redacted) appears once: (redacted)."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("skips empty secrets rather than mangling the message", func(t *testing.T) {
		got := redactSecrets("no secrets configured here", []string{"", ""})
		want := "no secrets configured here"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("leaves a message with no matching secret untouched", func(t *testing.T) {
		msg := "Vault provider not found"
		got := redactSecrets(msg, []string{"some-unrelated-secret"})
		if got != msg {
			t.Errorf("got %q, want %q", got, msg)
		}
	})

	t.Run("secretsOf pulls every sensitive field for each type", func(t *testing.T) {
		cases := []struct {
			name string
			cfg  any
			want []string
		}{
			{"hashicorp", &client.VaultHashicorpConfig{Token: "tok"}, []string{"tok"}},
			{"infisical", &client.VaultInfisicalConfig{ClientSecret: "cs"}, []string{"cs"}},
			{"aws", &client.VaultAWSConfig{AccessKeyID: "ak", SecretAccessKey: "sk"}, []string{"ak", "sk"}},
			{"doppler", &client.VaultDopplerConfig{ServiceToken: "st"}, []string{"st"}},
			{"azure", &client.VaultAzureConfig{ClientSecret: "cs"}, []string{"cs"}},
			{"scaleway", &client.VaultScalewayConfig{SecretKey: "sk"}, []string{"sk"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := secretsOf(tc.cfg)
				if len(got) != len(tc.want) {
					t.Fatalf("secretsOf(%T) = %v, want %v", tc.cfg, got, tc.want)
				}
				for i, s := range tc.want {
					if got[i] != s {
						t.Errorf("secretsOf(%T)[%d] = %q, want %q", tc.cfg, i, got[i], s)
					}
				}
			})
		}
	})
}

func TestAssignmentsExpandFlatten(t *testing.T) {
	ctx := context.Background()

	t.Run("with environment_ids round-trips", func(t *testing.T) {
		envSet, d := types.SetValueFrom(ctx, types.StringType, []string{"env-1", "env-2"})
		if d.HasError() {
			t.Fatalf("building set: %v", d)
		}
		obj := types.ObjectValueMust(assignmentAttrTypes(), map[string]attr.Value{
			"project_id":      types.StringValue("proj-1"),
			"environment_ids": envSet,
		})
		list, d := types.ListValue(types.ObjectType{AttrTypes: assignmentAttrTypes()}, []attr.Value{obj})
		if d.HasError() {
			t.Fatalf("building list: %v", d)
		}

		var diags diag.Diagnostics
		got := expandAssignments(ctx, list, &diags)
		if diags.HasError() {
			t.Fatalf("expandAssignments diags = %v", diags)
		}
		if len(got) != 1 || got[0].ProjectID != "proj-1" {
			t.Fatalf("expanded = %+v", got)
		}
		if len(got[0].EnvironmentIDs) != 2 {
			t.Fatalf("environment ids = %v, want 2 entries", got[0].EnvironmentIDs)
		}

		back := flattenAssignments(ctx, got, &diags)
		if diags.HasError() {
			t.Fatalf("flattenAssignments diags = %v", diags)
		}
		var models []assignmentModel
		diags.Append(back.ElementsAs(ctx, &models, false)...)
		if diags.HasError() {
			t.Fatalf("ElementsAs diags = %v", diags)
		}
		if len(models) != 1 || models[0].ProjectID.ValueString() != "proj-1" {
			t.Fatalf("flattened = %+v", models)
		}
		if models[0].EnvironmentIDs.IsNull() {
			t.Fatalf("environment_ids is null, want a set")
		}
	})

	t.Run("without environment_ids expands to an empty non-nil slice, not omitted", func(t *testing.T) {
		obj := types.ObjectValueMust(assignmentAttrTypes(), map[string]attr.Value{
			"project_id":      types.StringValue("proj-2"),
			"environment_ids": types.SetNull(types.StringType),
		})
		list, d := types.ListValue(types.ObjectType{AttrTypes: assignmentAttrTypes()}, []attr.Value{obj})
		if d.HasError() {
			t.Fatalf("building list: %v", d)
		}

		var diags diag.Diagnostics
		got := expandAssignments(ctx, list, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if len(got) != 1 {
			t.Fatalf("expanded = %+v", got)
		}
		if got[0].EnvironmentIDs == nil {
			t.Fatalf("EnvironmentIDs is nil - client.VaultAssignment has no omitempty, this would marshal as JSON null instead of []")
		}
		if len(got[0].EnvironmentIDs) != 0 {
			t.Fatalf("EnvironmentIDs = %v, want empty", got[0].EnvironmentIDs)
		}
	})

	t.Run("flattenAssignments echoes an empty, non-null set for a zero-length slice", func(t *testing.T) {
		var diags diag.Diagnostics
		list := flattenAssignments(ctx, []client.VaultAssignment{{ProjectID: "proj-3", EnvironmentIDs: nil}}, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		var models []assignmentModel
		diags.Append(list.ElementsAs(ctx, &models, false)...)
		if diags.HasError() {
			t.Fatalf("ElementsAs diags = %v", diags)
		}
		if len(models) != 1 {
			t.Fatalf("models = %+v", models)
		}
		if models[0].EnvironmentIDs.IsNull() {
			t.Errorf("environment_ids is null, want an empty set (matches the schema's empty-set Default)")
		}
		var ids []string
		diags.Append(models[0].EnvironmentIDs.ElementsAs(ctx, &ids, false)...)
		if len(ids) != 0 {
			t.Errorf("ids = %v, want empty", ids)
		}
	})

	t.Run("an empty assignments list round-trips (gate E)", func(t *testing.T) {
		list := types.ListValueMust(types.ObjectType{AttrTypes: assignmentAttrTypes()}, []attr.Value{})
		var diags diag.Diagnostics
		got := expandAssignments(ctx, list, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if len(got) != 0 {
			t.Fatalf("expanded = %+v, want empty", got)
		}

		back := flattenAssignments(ctx, got, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if back.IsNull() {
			t.Errorf("flattened list is null, want an empty (non-null) list")
		}
		if len(back.Elements()) != 0 {
			t.Errorf("flattened list has %d elements, want 0", len(back.Elements()))
		}
	})
}
