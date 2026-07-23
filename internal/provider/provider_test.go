package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
)

func TestProviderSchema(t *testing.T) {
	p := New("test")()

	metaResp := &fwprovider.MetadataResponse{}
	p.Metadata(context.Background(), fwprovider.MetadataRequest{}, metaResp)
	if metaResp.TypeName != "dokploy" {
		t.Fatalf("type name = %q, want dokploy", metaResp.TypeName)
	}

	schemaResp := &fwprovider.SchemaResponse{}
	p.Schema(context.Background(), fwprovider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	for _, name := range []string{"endpoint", "api_key", "insecure"} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}
	if !schemaResp.Schema.Attributes["api_key"].IsSensitive() {
		t.Error("api_key must be sensitive")
	}
}

func TestResolveConfig(t *testing.T) {
	env := map[string]string{
		"DOKPLOY_ENDPOINT": "https://env.example.com",
		"DOKPLOY_API_KEY":  "env-key",
	}
	getenv := func(k string) string { return env[k] }

	t.Run("explicit config wins over env", func(t *testing.T) {
		m := DokployProviderModel{
			Endpoint: types.StringValue("https://cfg.example.com"),
			ApiKey:   types.StringValue("cfg-key"),
			Insecure: types.BoolValue(true),
		}
		rc, missing := resolveConfig(m, getenv)
		if len(missing) != 0 {
			t.Fatalf("missing = %v", missing)
		}
		if rc.endpoint != "https://cfg.example.com" || rc.apiKey != "cfg-key" || !rc.insecure {
			t.Errorf("resolved = %+v", rc)
		}
	})

	t.Run("env fallback", func(t *testing.T) {
		rc, missing := resolveConfig(DokployProviderModel{}, getenv)
		if len(missing) != 0 {
			t.Fatalf("missing = %v", missing)
		}
		if rc.endpoint != "https://env.example.com" || rc.apiKey != "env-key" || rc.insecure {
			t.Errorf("resolved = %+v", rc)
		}
	})

	t.Run("missing everything reported", func(t *testing.T) {
		_, missing := resolveConfig(DokployProviderModel{}, func(string) string { return "" })
		if len(missing) != 2 {
			t.Fatalf("missing = %v, want 2 entries", missing)
		}
	})
}
