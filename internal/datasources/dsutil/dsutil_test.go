package dsutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestLookupDescriptionAndAttributes(t *testing.T) {
	l := Lookup{
		Kind: "vault provider", Plural: "vault providers",
		What:    "a vault provider",
		Example: `data "dokploy_vault_provider" "prod" {}`,
		Note:    "A note.",
		Secret:  "the connection config", SecretAttr: "The provider-specific blocks", Resource: "`dokploy_vault_provider`",
	}
	desc := l.Description()
	for _, want := range []string{
		"Looks up a vault provider:\n\n```terraform\ndata \"dokploy_vault_provider\" \"prod\" {}\n```\n\nA note.\n\n",
		"~> **The data source does not expose the connection config.** The provider-specific blocks exists on the `dokploy_vault_provider` resource",
		"If two vault providers share a name, this data source fails instead of a guess.",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("description lacks %q:\n%s", want, desc)
		}
	}
	if plain := (Lookup{Kind: "x", Plural: "xs", What: "w", Example: "e"}).Description(); strings.Contains(plain, "does not expose") {
		t.Error("a Lookup without a Secret must not render the secret note")
	}

	attrs := l.Attributes()
	if len(attrs) != 2 || !strings.HasPrefix(attrs["id"].GetDescription(), "Vault provider id.") {
		t.Errorf("Attributes() = %v", attrs)
	}
	svc := (Lookup{Kind: "compose service", Plural: "services"}).ServiceAttributes()
	if len(svc) != 3 || !strings.HasPrefix(svc["id"].GetDescription(), "Compose service id.") || svc["environment_id"] == nil {
		t.Errorf("ServiceAttributes() = %v", svc)
	}
	if len(IDOrName()) != 1 || len(ServiceLookup()) != 2 {
		t.Error("validator sets have the wrong size")
	}
	if !String("s").Computed || !Bool("b").Computed || !Int64("i").Computed || !StringList("l").Computed {
		t.Error("the computed constructors must set Computed")
	}
}

func TestResolveService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/environment.one" || r.URL.Query().Get("environmentId") != "e1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(`{"compose":[{"composeId":"co1","name":"stack"},{"composeId":"co2","name":"twin"},{"composeId":"co3","name":"twin"}]}`))
	}))
	defer srv.Close()
	c, err := client.New(srv.URL, "k", false, "test")
	if err != nil {
		t.Fatal(err)
	}
	pick := func(s *client.EnvironmentServices) []client.ServiceRef { return s.Compose }
	ctx := context.Background()

	id, diags := ResolveService(ctx, c, types.StringValue("given"), types.StringNull(), types.StringNull(), "compose", pick)
	if diags.HasError() || id != "given" {
		t.Errorf("by id = %q, %v", id, diags)
	}
	id, diags = ResolveService(ctx, c, types.StringNull(), types.StringValue("e1"), types.StringValue("stack"), "compose", pick)
	if diags.HasError() || id != "co1" {
		t.Errorf("by name = %q, %v", id, diags)
	}
	_, diags = ResolveService(ctx, c, types.StringNull(), types.StringValue("e1"), types.StringValue("twin"), "compose", pick)
	if !diags.HasError() || !strings.Contains(diags[0].Detail(), `multiple compose services named "twin"`) {
		t.Errorf("ambiguous name diags = %v", diags)
	}
	_, diags = ResolveService(ctx, c, types.StringNull(), types.StringValue("e1"), types.StringValue("none"), "compose", pick)
	if !diags.HasError() || !strings.Contains(diags[0].Detail(), `no compose service named "none"`) {
		t.Errorf("missing name diags = %v", diags)
	}
}
