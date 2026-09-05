// Package gitproviders holds the dokploy_gitlab_provider,
// dokploy_bitbucket_provider and dokploy_gitea_provider data sources.
//
// All three read gitProvider.getAll, the one list that shows a provider of
// every type whether or not its OAuth handshake completed. The type-specific
// list endpoints (gitlab.gitlabProviders and friends) hide a provider until
// it holds an access token, which is exactly the state a fresh Terraform
// record is in (probed live, v0.30.5, 2026-09-05). dokploy_github_provider
// keeps its own package and endpoint: a GitHub App has no create path.
package gitproviders

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func exactlyOneOfIDOrName() []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

// find resolves one provider of providerType by its type-specific id or by
// name. It errors on zero AND on multiple matches rather than taking [0]:
// nothing in Dokploy makes provider names unique.
func find(ctx context.Context, c *client.Client, providerType, label string, id, name string, typeID func(client.GitProviderSummary) string) (*client.GitProviderSummary, error) {
	all, err := c.ListGitProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing git providers: %w", err)
	}
	var matches []client.GitProviderSummary
	for _, p := range all {
		if p.ProviderType != providerType {
			continue
		}
		if (id != "" && typeID(p) == id) || (name != "" && p.Name == name) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		if id != "" {
			return nil, fmt.Errorf("no %s provider with id %q", label, id)
		}
		return nil, fmt.Errorf("no %s provider named %q", label, name)
	default:
		return nil, fmt.Errorf(
			"%d %s providers are named %q; names are not unique in Dokploy, so look it up by id instead",
			len(matches), label, name)
	}
}
