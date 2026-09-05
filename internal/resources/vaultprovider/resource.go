// Package vaultprovider holds the dokploy_vault_provider resource: a
// secret-vault connection Dokploy can pull runtime secrets from, one of six
// provider types (hashicorp, infisical, aws, doppler, azure, scaleway).
//
// Config is REDACT, not ECHO (internal/client/doc.go, wave 6c gate R):
// every secret field reads back masked as the literal string "********" on
// every read. Read therefore refreshes only name, assignments, and
// computed fields; every config block attribute - secret and non-secret
// alike - is carried forward from state rather than refreshed from the
// server (no mixed refresh within a block). A config value changed in the
// Dokploy UI persists undetected until the next apply that modifies this
// resource; that apply rewrites the whole block. See the schema
// description on each config block.
//
// A second, separate defense guards against a server-side defect
// (internal/client/doc.go, wave 6c "Duplicate names"): vaultProvider.create
// rejects a duplicate name through a raw HTTP 500 whose body leaks the
// failed request's secret fields in cleartext. Create runs a best-effort
// name-uniqueness pre-check with ListVaultProviders before ever sending a
// secret to the server, and redactSecrets (model.go) scrubs every
// configured secret value out of any server error text reaching AddError in
// Create, Update, and the verify_connection path - in case the pre-check
// itself loses a race, or a different server-side path leaks a secret
// later.
//
// Update accepts a full type swap (e.g. hashicorp -> doppler) in place,
// verified live in wave 6c, so no config block carries a RequiresReplace
// plan modifier.
//
// verify_connection is provider-only: when true, Create and Update call
// vaultProvider.testConnection with the expanded config before writing
// anything, and fail the apply on the server's (redacted) message with
// nothing created or updated. It has no server-side value to import from,
// so ImportState seeds it false explicitly - the same problem
// tfutil.ImportDeployDefaults solves for the deploy-engine attributes.
//
// Two pieces of the real API surface are deliberately unmodeled:
// vaultProvider.listSecretNames is a read-only UI helper (a flat array of
// "path:key" strings for one vault provider, confirmed live in wave 6c)
// with no Terraform-shaped use, and `${{vault.name.key}}` env var
// references are opaque text that passes through the existing `env`
// attributes on dokploy_application and friends untouched - this resource
// only manages the vault connection itself.
package vaultprovider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                     = (*vaultProviderResource)(nil)
	_ resource.ResourceWithConfigure        = (*vaultProviderResource)(nil)
	_ resource.ResourceWithImportState      = (*vaultProviderResource)(nil)
	_ resource.ResourceWithConfigValidators = (*vaultProviderResource)(nil)
)

type vaultProviderResource struct{ client *client.Client }

func NewResource() resource.Resource { return &vaultProviderResource{} }

func (r *vaultProviderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault_provider"
}

// ConfigValidators enforces exactly one config block, mirroring
// dokploy_application's and dokploy_compose's exactly-one-of source
// pattern.
func (r *vaultProviderResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("hashicorp"),
			path.MatchRoot("infisical"),
			path.MatchRoot("aws"),
			path.MatchRoot("doppler"),
			path.MatchRoot("azure"),
			path.MatchRoot("scaleway"),
		),
	}
}

func (r *vaultProviderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A secret-vault connection. Dokploy pulls runtime secrets from it at deploy time. Other resources reference a secret in their " +
			"`env` attribute as `${{vault.<name>.<key>}}`, a plain string that this provider does not parse or validate. " +
			"The resource models six provider types: `hashicorp` (also OpenBao, which uses the same wire protocol), " +
			"`infisical`, `aws`, `doppler`, `azure`, and `scaleway`. Dokploy v0.30.5 adds a seventh type, `phase` (Phase.dev), " +
			"which this resource does not model yet.\n\n" +
			"~> **Dokploy masks each secret on each read.** Dokploy returns each secret field in the config blocks of this resource " +
			"as the literal string `********`, on create, read, and update alike. The provider therefore cannot detect a " +
			"config value that changed in the Dokploy UI. Read keeps each config block exactly as Terraform last wrote it, secret " +
			"and non-secret fields alike. Manage the config of a vault provider only through Terraform. An edit in the UI stays " +
			"undetected until the next apply that modifies this resource. That apply writes the full body and overwrites the edit " +
			"with the Terraform config.\n\n" +
			"~> **Each secret field has a write-only companion**, for example `hashicorp.token_wo` with `hashicorp.token_wo_version`. " +
			"Terraform keeps the companion out of the plan and the state. The server does not return the secret, so the provider " +
			"sends the companion's value on every update. Change the version to start an update when only the secret changed.\n\n" +
			"~> **`terraform import` cannot recover a config block.** The import leaves the config blocks null. " +
			"Supply the block that matches the actual provider type in the configuration. The first `terraform apply` then writes it " +
			"as a full-body update, not as a partial patch.\n\n" +
			"~> **Dokploy does not validate vault credentials on create or update**, for any provider type. Only " +
			"`verify_connection = true` reaches the real vault, through `vaultProvider.testConnection`, before the write. " +
			"Without it, a misconfigured vault provider applies successfully and fails only on the next deploy that needs a secret " +
			"from it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Vault provider id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required: true,
				Description: "Display name, 1 to 64 characters, that matches `^[a-zA-Z0-9_-]+$`. Dokploy rejects a duplicate name. " +
					"The provider checks for a duplicate before it sends a create request, so a collision fails with a clear error " +
					"instead of the raw server error.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 64),
					stringvalidator.RegexMatches(regexp.MustCompile(`^[a-zA-Z0-9_-]+$`), "must contain only letters, numbers, underscores, and hyphens"),
				},
			},
			"hashicorp": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "HashiCorp Vault or OpenBao connection. Set exactly one of `hashicorp`, `infisical`, `aws`, `doppler`, `azure`, or `scaleway`.",
				Attributes: map[string]schema.Attribute{
					"url":   schema.StringAttribute{Required: true, Description: "Vault or OpenBao server URL, for example `https://vault.example.com:8200`."},
					"token": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Vault authentication token. Set this attribute or `token_wo`."},
					"namespace": schema.StringAttribute{
						Optional:    true,
						Description: "Vault Enterprise namespace. Omit it for open-source Vault or OpenBao. The server has no default for this field.",
						// An empty string cannot round-trip: flattenHashicorpConfig (model.go) uses tfutil.StringOrNull,
						// which collapses "" back to null on read, so a "" here would apply and then fail with an
						// "inconsistent result" error. Reject it at plan time instead of at apply time.
						Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"mount": schema.StringAttribute{
						Optional: true, Computed: true,
						Default:     stringdefault.StaticString("secret"),
						Description: "KV secrets engine mount path. Defaults to `secret`.",
					},
				},
			},
			"infisical": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Infisical connection. Set exactly one of `hashicorp`, `infisical`, `aws`, `doppler`, `azure`, or `scaleway`.",
				Attributes: map[string]schema.Attribute{
					"site_url": schema.StringAttribute{
						Optional: true, Computed: true,
						Default:     stringdefault.StaticString("https://app.infisical.com"),
						Description: "Infisical instance URL. Defaults to the Infisical Cloud URL.",
					},
					"client_id":     schema.StringAttribute{Required: true, Description: "Infisical machine identity client id."},
					"client_secret": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Infisical machine identity client secret. Set this attribute or `client_secret_wo`."},
					"project_id":    schema.StringAttribute{Required: true, Description: "Infisical project id."},
					"environment_slug": schema.StringAttribute{
						Required:    true,
						Description: "Infisical environment slug, for example `dev` or `prod`.",
					},
					"secret_path": schema.StringAttribute{
						Optional: true, Computed: true,
						Default:     stringdefault.StaticString("/"),
						Description: "Path inside the Infisical project to read secrets from. Defaults to `/`.",
					},
				},
			},
			"aws": schema.SingleNestedAttribute{
				Optional: true,
				Description: "AWS Secrets Manager connection. Set exactly one of `hashicorp`, `infisical`, `aws`, `doppler`, `azure`, or `scaleway`.\n\n" +
					"~> The shape of this block comes from the OpenAPI contract, not from a live probe. " +
					"The acceptance tests of this resource are the first live confirmation of it.",
				Attributes: map[string]schema.Attribute{
					"region":            schema.StringAttribute{Required: true, Description: "AWS region for Secrets Manager, for example `us-east-1`."},
					"access_key_id":     schema.StringAttribute{Optional: true, Sensitive: true, Description: "AWS access key id. Set this attribute or `access_key_id_wo`."},
					"secret_access_key": schema.StringAttribute{Optional: true, Sensitive: true, Description: "AWS secret access key. Set this attribute or `secret_access_key_wo`."},
					"endpoint": schema.StringAttribute{
						Optional:    true,
						Description: "Custom Secrets Manager endpoint, for a compatible service or a VPC endpoint. Omit it to use the default AWS endpoint. The server has no default for this field.",
						// Same reason as hashicorp.namespace above: flattenAWSConfig collapses "" back to null.
						Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
					},
				},
			},
			"doppler": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Doppler connection. Set exactly one of `hashicorp`, `infisical`, `aws`, `doppler`, `azure`, or `scaleway`.",
				Attributes: map[string]schema.Attribute{
					"service_token": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Doppler service token. Set this attribute or `service_token_wo`."},
					"project": schema.StringAttribute{
						Optional:    true,
						Description: "Doppler project slug. Omit it, and Doppler infers it from the service token. The server has no default for this field.",
						// Same reason as hashicorp.namespace above: flattenDopplerConfig collapses "" back to null.
						Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"config": schema.StringAttribute{
						Optional: true,
						Description: "Doppler config name. The wire field is also named `config`. Omit it, and Doppler infers it from the service " +
							"token. The server has no default for this field.",
						// Same reason as hashicorp.namespace above: flattenDopplerConfig collapses "" back to null.
						Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
					},
				},
			},
			"azure": schema.SingleNestedAttribute{
				Optional: true,
				Description: "Azure Key Vault connection. The API requires each field; Azure has no optional field here. Set exactly one of `hashicorp`, `infisical`, `aws`, `doppler`, `azure`, or `scaleway`.\n\n" +
					"~> The shape of this block comes from the OpenAPI contract, not from a live probe.",
				Attributes: map[string]schema.Attribute{
					"vault_uri":     schema.StringAttribute{Required: true, Description: "Azure Key Vault URI, for example `https://myvault.vault.azure.net/`."},
					"tenant_id":     schema.StringAttribute{Required: true, Description: "Azure AD tenant id."},
					"client_id":     schema.StringAttribute{Required: true, Description: "Azure AD application client id."},
					"client_secret": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Azure AD application client secret. Set this attribute or `client_secret_wo`."},
				},
			},
			"scaleway": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Scaleway Secret Manager connection. Set exactly one of `hashicorp`, `infisical`, `aws`, `doppler`, `azure`, or `scaleway`.",
				Attributes: map[string]schema.Attribute{
					"project_id": schema.StringAttribute{Required: true, Description: "Scaleway project id."},
					"secret_key": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Scaleway API secret key. Set this attribute or `secret_key_wo`."},
					"region": schema.StringAttribute{
						Optional: true, Computed: true,
						Default:     stringdefault.StaticString("fr-par"),
						Description: "Scaleway region. Defaults to `fr-par`.",
					},
					"api_url": schema.StringAttribute{
						Optional: true, Computed: true,
						Default:     stringdefault.StaticString("https://api.scaleway.com"),
						Description: "Scaleway Secret Manager API URL. Defaults to `https://api.scaleway.com`.",
					},
				},
			},
			"assignments": schema.ListNestedAttribute{
				Required: true,
				Description: "Projects, and optionally specific environments in them, that can use this vault provider. " +
					"An empty list is valid: the server accepts `assignments = []` and returns it.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"project_id": schema.StringAttribute{Required: true, Description: "Id of the assigned project."},
						"environment_ids": schema.SetAttribute{
							Optional: true, Computed: true,
							ElementType: types.StringType,
							Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
							Description: "Ids of the environments in the project that this assignment covers. Omit it, or set an " +
								"empty list, to cover each environment in the project. The server stores and returns an empty set for " +
								"that case, not null.",
						},
					},
				},
			},
			"verify_connection": schema.BoolAttribute{
				Optional: true, Computed: true,
				Default: booldefault.StaticBool(false),
				Description: "Test the config against the real vault before the write, through `vaultProvider.testConnection`. Defaults to " +
					"`false`. On failure, the apply fails with the server message, and the provider creates or updates nothing. This attribute is " +
					"provider-only, and Dokploy stores no value for it, so `terraform import` always seeds it with `false`.",
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Creation timestamp from the server.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
	// Every secret field gets its write-only pair inside its block. The
	// server masks each secret on read, so the resource sends the configured
	// value on every update; SentOnEveryUpdate words the descriptions so.
	for block, secrets := range map[string][]string{
		"hashicorp": {"token"},
		"infisical": {"client_secret"},
		"aws":       {"access_key_id", "secret_access_key"},
		"doppler":   {"service_token"},
		"azure":     {"client_secret"},
		"scaleway":  {"secret_key"},
	} {
		nested := resp.Schema.Attributes[block].(schema.SingleNestedAttribute)
		for _, secret := range secrets {
			for name, attr := range tfutil.WriteOnlyCompanions(secret, tfutil.WriteOnlyOptions{ExactlyOne: true, Nested: true, SentOnEveryUpdate: true}) {
				nested.Attributes[name] = attr
			}
		}
		resp.Schema.Attributes[block] = nested
	}
}

func (r *vaultProviderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// populatedBlockType returns the wire discriminator for whichever config
// block is set in m, or "" if none is (only possible before the resource's
// ConfigValidators have run, e.g. on freshly imported state).
func populatedBlockType(m resourceModel) string {
	switch {
	case !m.Hashicorp.IsNull():
		return client.VaultProviderTypeHashicorp
	case !m.Infisical.IsNull():
		return client.VaultProviderTypeInfisical
	case !m.AWS.IsNull():
		return client.VaultProviderTypeAWS
	case !m.Doppler.IsNull():
		return client.VaultProviderTypeDoppler
	case !m.Azure.IsNull():
		return client.VaultProviderTypeAzure
	case !m.Scaleway.IsNull():
		return client.VaultProviderTypeScaleway
	default:
		return ""
	}
}

// checkProviderTypeDrift logs - never resp.Diagnostics.Add* - when the
// server's top-level providerType no longer matches the config block
// populated in state: an out-of-band type swap made in the Dokploy UI.
// Read cannot reconcile this: config is REDACT (internal/client/doc.go,
// wave 6c gate R), so there is no way to decode the server's actual config
// into the right block from here. State is left exactly as it was; the
// next apply that modifies this resource runs Update, which rewrites the
// whole record from Terraform's config regardless of what the server
// currently holds. Raising a diagnostic here would only alarm the
// operator over something that apply already fixes on its own.
func checkProviderTypeDrift(ctx context.Context, state resourceModel, serverType string) {
	stateType := populatedBlockType(state)
	if stateType == "" || serverType == "" || stateType == serverType {
		return
	}
	tflog.Warn(ctx, "vault provider type in state no longer matches the server; config block is stale until the next apply", map[string]any{
		"vault_provider_id": state.ID.ValueString(),
		"state_type":        stateType,
		"server_type":       serverType,
	})
}

// refreshComputed copies only name, assignments, and computed fields from v
// into m - never the config blocks (gate R, see the package doc comment).
func refreshComputed(ctx context.Context, v *client.VaultProvider, m *resourceModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(v.VaultProviderID)
	m.Name = types.StringValue(v.Name)
	m.CreatedAt = types.StringValue(v.CreatedAt)
	m.Assignments = flattenAssignments(ctx, v.Assignments, diags)
}

// Create builds the config struct - and its secrets, for redactSecrets -
// before the name pre-check, not after: that pre-check's own
// ListVaultProviders call can fail too, and its error must be scrubbed the
// same as every other server-error path here (see the package doc
// comment). verify_connection, when set, runs before CreateVaultProvider;
// a failed test leaves nothing created.
func (r *vaultProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// The config, not the plan, carries the write-only secrets: the
	// framework nulls them in the plan (tfutil.WriteOnlyCompanions).
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, _ := expandConfig(ctx, plan, config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	secrets := secretsOf(cfg)

	name := plan.Name.ValueString()
	existing, err := r.client.ListVaultProviders(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Checking existing vault providers", redactSecrets(err.Error(), secrets))
		return
	}
	for _, v := range existing {
		if v.Name == name {
			resp.Diagnostics.AddError("Creating vault provider", fmt.Sprintf("a vault provider named %q already exists", name))
			return
		}
	}

	if plan.VerifyConnection.ValueBool() {
		if err := r.client.TestVaultConnection(ctx, client.TestVaultConnectionRequest{Config: cfg}); err != nil {
			resp.Diagnostics.AddError("Verifying vault connection", redactSecrets(err.Error(), secrets))
			return
		}
	}

	assignments := expandAssignments(ctx, plan.Assignments, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateVaultProvider(ctx, client.CreateVaultProviderRequest{
		Name:        name,
		Config:      cfg,
		Assignments: assignments,
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating vault provider", redactSecrets(err.Error(), secrets))
		return
	}

	refreshComputed(ctx, created, &plan, &resp.Diagnostics)
	flattenConfig(ctx, cfg, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vaultProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.GetVaultProvider(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading vault provider", err.Error())
		return
	}

	checkProviderTypeDrift(ctx, state, v.ProviderType)
	refreshComputed(ctx, v, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update resends the whole record - config block and assignments included -
// matching vaultProvider.update's full-body contract (no partial patch).
// verify_connection, when set, runs first; a failed test leaves the prior
// record untouched server-side.
func (r *vaultProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// The config, not the plan, carries the write-only secrets: the
	// framework nulls them in the plan (tfutil.WriteOnlyCompanions).
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, _ := expandConfig(ctx, plan, config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	secrets := secretsOf(cfg)

	if plan.VerifyConnection.ValueBool() {
		if err := r.client.TestVaultConnection(ctx, client.TestVaultConnectionRequest{Config: cfg}); err != nil {
			resp.Diagnostics.AddError("Verifying vault connection", redactSecrets(err.Error(), secrets))
			return
		}
	}

	assignments := expandAssignments(ctx, plan.Assignments, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateVaultProvider(ctx, client.UpdateVaultProviderRequest{
		VaultProviderID: plan.ID.ValueString(),
		Name:            plan.Name.ValueString(),
		Config:          cfg,
		Assignments:     assignments,
	}); err != nil {
		resp.Diagnostics.AddError("Updating vault provider", redactSecrets(err.Error(), secrets))
		return
	}

	v, err := r.client.GetVaultProvider(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading vault provider after update", redactSecrets(err.Error(), secrets))
		return
	}
	refreshComputed(ctx, v, &plan, &resp.Diagnostics)
	flattenConfig(ctx, cfg, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vaultProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteVaultProvider(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting vault provider", err.Error())
	}
}

// ImportState seeds verify_connection false. It is provider-only - Dokploy
// stores no server-side value for it - so passthrough import leaves it
// null, and a config that omits it (the normal case, since it has a schema
// default) would plan `false` against a null prior value forever
// (tfutil.ImportDeployDefaults documents the identical failure mode for the
// deploy-engine attributes). The config blocks are left null by this
// import: gate R (internal/client/doc.go, wave 6c) means Terraform can
// never recover a secret from the server, so the schema description asks
// the operator to re-supply the matching block in configuration - the
// first apply after import is a full-body update, not an empty plan.
func (r *vaultProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("verify_connection"), false)...)
}
