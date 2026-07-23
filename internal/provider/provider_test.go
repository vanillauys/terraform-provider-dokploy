package provider

import (
	"context"
	"testing"

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
