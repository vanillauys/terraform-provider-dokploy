// Package dsutil holds the schema and lookup helpers that the by-name data
// sources share: the id-or-name attribute pair with its validators, the
// description text that every lookup repeats, the computed attribute
// constructors, and the name-to-id resolution of a service within an
// environment.
//
// It exists so that a new lookup data source is a record type, a
// description, and a Read body, not a copy of its sibling. SonarQube's
// duplication gate flagged the first five copies; this package is the fix.
package dsutil

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// Lookup describes a data source that finds one organization-level record
// by id or by name.
type Lookup struct {
	// Kind is the record in the singular and in lowercase: "certificate".
	Kind string
	// Plural is the record in the plural, with any scope: "certificates",
	// "compose services in the environment".
	Plural string
	// What completes "Looks up ": "a TLS certificate that already exists in
	// Dokploy (Settings > Certificates), so that a domain can reference it".
	What string
	// Example is the Terraform example, without the code fence.
	Example string
	// Note is an optional sentence after the example.
	Note string
	// Secret, SecretAttr and Resource name a secret that stays out of the
	// data source: "the private key", "`private_key`", "`dokploy_certificate`".
	// An empty Secret leaves the note out.
	Secret, SecretAttr, Resource string
}

// Description renders the schema description of the lookup.
func (l Lookup) Description() string {
	var b strings.Builder
	b.WriteString("Looks up " + l.What + ":\n\n```terraform\n" + l.Example + "\n```\n\n")
	if l.Note != "" {
		b.WriteString(l.Note + "\n\n")
	}
	if l.Secret != "" {
		b.WriteString("~> **The data source does not expose " + l.Secret + ".** " + l.SecretAttr + " exists on the " +
			l.Resource + " resource, but not here, by design. A consumer needs only the id.\n\n")
	}
	b.WriteString("~> Dokploy does not enforce name uniqueness. If two " + l.Plural +
		" share a name, this data source fails instead of a guess. Look the record up by `id` in that case.")
	return b.String()
}

// Attributes returns the id and name attributes of the lookup. The caller
// adds the computed attributes of the record to the map.
func (l Lookup) Attributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Optional: true, Computed: true,
			Description: capitalize(l.Kind) + " id. Set it for a lookup by id, or leave it unset and set `name`.",
		},
		"name": schema.StringAttribute{
			Optional: true, Computed: true,
			Description: "Display name as shown in Dokploy. Set exactly one of `id` or `name`.",
		},
	}
}

// ServiceAttributes returns the id, name and environment_id attributes of a
// lookup for a service within an environment.
func (l Lookup) ServiceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Optional: true, Computed: true,
			Description: capitalize(l.Kind) + " id. Set this attribute, or set both `environment_id` and `name`.",
		},
		"name": schema.StringAttribute{
			Optional: true, Computed: true,
			Description: "Exact display name. The lookup searches within `environment_id` and errors when zero or many " + l.Plural + " match.",
		},
		"environment_id": schema.StringAttribute{
			Optional: true, Computed: true,
			Description: "Id of the environment to search. Required with `name`.",
		},
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// IDOrName is the validator set of Lookup.Attributes.
func IDOrName() []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

// ServiceLookup is the validator set of Lookup.ServiceAttributes.
func ServiceLookup() []datasource.ConfigValidator {
	return append(IDOrName(),
		datasourcevalidator.RequiredTogether(path.MatchRoot("environment_id"), path.MatchRoot("name")),
	)
}

// String, Bool and Int64 build a computed attribute with a description.
func String(description string) schema.StringAttribute {
	return schema.StringAttribute{Computed: true, Description: description}
}

func Bool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{Computed: true, Description: description}
}

func Int64(description string) schema.Int64Attribute {
	return schema.Int64Attribute{Computed: true, Description: description}
}

// StringList builds a computed list of strings with a description.
func StringList(description string) schema.ListAttribute {
	return schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: description}
}

// ResolveService returns the id of a service from a config: the id when it
// is set, else the id of the service named name within environmentID. pick
// selects the service kind's list from environment.one, and kind names it
// in the errors.
func ResolveService(ctx context.Context, c *client.Client, id, environmentID, name types.String, kind string,
	pick func(*client.EnvironmentServices) []client.ServiceRef) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !id.IsNull() {
		return id.ValueString(), diags
	}
	services, err := c.EnvironmentServices(ctx, environmentID.ValueString())
	if err != nil {
		diags.AddError("Listing "+kind+" services", err.Error())
		return "", diags
	}
	found, err := client.FindServiceByName(pick(services), name.ValueString(), kind)
	if err != nil {
		diags.AddError("Looking up "+kind+" service by name", err.Error())
		return "", diags
	}
	return found, diags
}
