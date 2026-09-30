package dsutil

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/lookup"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

// Find returns the one item that match accepts. It fails on zero matches and
// on many matches, and it never takes the first: Dokploy does not enforce
// uniqueness for names or for the attributes of a service child. kind names
// the record and criteria names the lookup in the error text.
func Find[T any](items []T, match func(T) bool, kind, criteria string) (*T, error) {
	found, err := lookup.Find(items, match)
	switch {
	case errors.Is(err, lookup.ErrMultipleMatches):
		return nil, fmt.Errorf("more than one %s matches %s; Dokploy does not enforce uniqueness, so look it up by id", kind, criteria)
	case errors.Is(err, lookup.ErrNoMatch):
		return nil, fmt.Errorf("no %s matches %s", kind, criteria)
	case err != nil:
		return nil, err
	}
	return &found, nil
}

// Child describes a data source for a record that belongs to a service. The
// lookup takes the id of the parent and one or more distinguishing
// attributes.
type Child struct {
	// Kind is the record in the singular and in lowercase: "mount".
	Kind string
	// What completes "Looks up ": "a mount of an application or a database".
	What string
	// Example is the Terraform example, without the code fence.
	Example string
	// Note is an optional sentence after the example.
	Note string
	// Secret and SecretAttr name a secret that stays out of the data
	// source, in the form of Lookup. An empty Secret leaves the note out.
	Secret, SecretAttr, Resource string
}

// Description renders the schema description of the lookup.
func (c Child) Description() string {
	var b strings.Builder
	b.WriteString("Looks up " + c.What + ":\n\n```terraform\n" + c.Example + "\n```\n\n")
	if c.Note != "" {
		b.WriteString(c.Note + "\n\n")
	}
	if c.Secret != "" {
		b.WriteString("~> **The data source does not expose " + c.Secret + ".** " + c.SecretAttr + " exists on the " +
			c.Resource + " resource, but not here, by design.\n\n")
	}
	b.WriteString("~> Dokploy does not enforce uniqueness for this record. If two " + c.Kind +
		" records match the lookup attributes, this data source fails instead of a guess. Look the record up by `id` in that case.")
	return b.String()
}

// IDAttribute returns the id attribute of the lookup.
func (c Child) IDAttribute(parent string) schema.Attribute {
	return schema.StringAttribute{
		Optional: true, Computed: true,
		Description: capitalize(c.Kind) + " id. Set it for a lookup by id, or leave it unset and set `" + parent + "` with the distinguishing attributes.",
	}
}

// LookupString builds a string attribute that a lookup can set and that the
// read path always fills.
func LookupString(description string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Computed: true, Description: description}
}

// LookupInt64 builds an integer attribute that a lookup can set and that the
// read path always fills.
func LookupInt64(description string) schema.Int64Attribute {
	return schema.Int64Attribute{Optional: true, Computed: true, Description: description}
}

// ChildValidators returns the validators of a service child lookup: exactly
// one of `id` and the parent attribute, the companion attributes that go
// with the parent, and at least one distinguishing attribute when there is
// no id. A distinguishing attribute never goes with an id.
func ChildValidators(parent string, with []string, by ...string) []datasource.ConfigValidator {
	validators := []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot(parent)),
	}
	if len(with) > 0 {
		expressions := []path.Expression{path.MatchRoot(parent)}
		for _, w := range with {
			expressions = append(expressions, path.MatchRoot(w))
		}
		validators = append(validators, datasourcevalidator.RequiredTogether(expressions...))
	}
	expressions := []path.Expression{path.MatchRoot("id")}
	for _, b := range by {
		expressions = append(expressions, path.MatchRoot(b))
		validators = append(validators, datasourcevalidator.Conflicting(path.MatchRoot("id"), path.MatchRoot(b)))
	}
	return append(validators, datasourcevalidator.AtLeastOneOf(expressions...))
}

// Record describes one lookup data source. M is the model of the data
// source and R is the record that the client returns. The engine reads the
// config, finds the record by id or by the lookup attributes, and writes the
// model back. Per-record divergence lives in the fields, never in a branch
// of the engine.
type Record[M, R any] struct {
	// Name is the type suffix: dokploy_<Name>.
	Name        string
	Description string
	Attributes  map[string]schema.Attribute
	Validators  []datasource.ConfigValidator
	// ID reads the id from the config; it is null when the config has none.
	ID func(*M) types.String
	// Get reads a record by id. Find resolves the lookup attributes of the
	// config to one record.
	Get  func(context.Context, *client.Client, string) (*R, error)
	Find func(context.Context, *client.Client, *M) (*R, error)
	// Flatten writes the record into the model.
	Flatten func(*R, *M)
}

type recordDataSource[M, R any] struct {
	record Record[M, R]
	client *client.Client
}

// NewDataSource returns the data source constructor for a record.
func NewDataSource[M, R any](record Record[M, R]) func() datasource.DataSource {
	return func() datasource.DataSource { return &recordDataSource[M, R]{record: record} }
}

var (
	_ datasource.DataSource                     = (*recordDataSource[struct{}, struct{}])(nil)
	_ datasource.DataSourceWithConfigure        = (*recordDataSource[struct{}, struct{}])(nil)
	_ datasource.DataSourceWithConfigValidators = (*recordDataSource[struct{}, struct{}])(nil)
)

func (d *recordDataSource[M, R]) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.record.Name
}

func (d *recordDataSource[M, R]) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return d.record.Validators
}

func (d *recordDataSource[M, R]) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: d.record.Description, Attributes: d.record.Attributes}
}

func (d *recordDataSource[M, R]) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *recordDataSource[M, R]) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model M
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *R
	var err error
	if id := d.record.ID(&model); !id.IsNull() && id.ValueString() != "" {
		found, err = d.record.Get(ctx, d.client, id.ValueString())
	} else {
		found, err = d.record.Find(ctx, d.client, &model)
	}
	if err != nil {
		resp.Diagnostics.AddError("Looking up the "+d.record.Name, err.Error())
		return
	}
	d.record.Flatten(found, &model)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
