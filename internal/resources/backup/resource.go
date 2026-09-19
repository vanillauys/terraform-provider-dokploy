// Package backup holds dokploy_backup: a scheduled logical dump of a
// database to an S3-compatible destination.
package backup

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                   = (*backupResource)(nil)
	_ resource.ResourceWithConfigure      = (*backupResource)(nil)
	_ resource.ResourceWithImportState    = (*backupResource)(nil)
	_ resource.ResourceWithUpgradeState   = (*backupResource)(nil)
	_ resource.ResourceWithValidateConfig = (*backupResource)(nil)
)

type backupResource struct{ client *client.Client }

func NewResource() resource.Resource { return &backupResource{} }

func (r *backupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup"
}

// serviceTypes are the parents this resource accepts: the database engines
// Dokploy can logically dump, plus compose.
//
// `redis` is deliberately absent — see the validator message. `web-server`
// is deliberately absent too: it backs up Dokploy's own database rather than
// a service, has no parent id, and needs its own validation path.
var serviceTypes = append([]string{"compose"}, client.BackupDatabaseTypes...)

func (r *backupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		// Version 1 (v0.11.0) renamed `schedule` to `cron_expression`.
		// Version 2 added `compose_database_type`. See UpgradeState.
		Version: 2,
		Description: "A scheduled logical dump of a database to an S3-compatible destination.\n\n" +
			"~> **This resource does not support Redis.** Dokploy has no logical dump for Redis. Use " +
			"`dokploy_volume_backup`, which archives the volume and accepts a Redis parent.\n\n" +
			"~> This resource does not expose a backup of the Dokploy server itself (the Dokploy `web-server` backup type). " +
			"That backup type has no parent service and needs a separate validation path.\n\n" +
			"~> Dokploy stores and returns `compose_database_password` and `compose_database_root_password` in cleartext. " +
			"Both attributes are sensitive, so Terraform does not print them, but anyone with API access to the server " +
			"can read them. The `compose_database_password_wo` and `compose_database_root_password_wo` companions keep " +
			"them out of the Terraform state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Backup id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"service_id": schema.StringAttribute{
				Required: true,
				Description: "Id of the database or compose service to dump. A change forces a replacement: " +
					"the Dokploy update endpoint has no parent field, so a retarget is not possible.",
				PlanModifiers: requiresReplace,
			},
			"service_type": schema.StringAttribute{
				Required: true,
				Description: "Kind of service that `service_id` refers to: one of `postgres`, `mysql`, `mariadb`, " +
					"`mongo`, `libsql`, `compose`. A change forces a replacement. The provider derives the Dokploy `backupType` " +
					"field from this attribute, and the `databaseType` field from it too, except when it is `compose` — " +
					"see `compose_database_type` for that case. Independent values would allow a " +
					"record whose type and parent disagree, so the provider does not expose them.",
				PlanModifiers: requiresReplace,
				Validators:    []validator.String{stringvalidator.OneOf(serviceTypes...)},
			},
			"compose_database_type": schema.StringAttribute{
				Optional: true,
				Description: "Real database engine running inside the compose service that `service_id` refers to: " +
					"one of `postgres`, `mysql`, `mariadb`, `mongo`, `libsql`. Required when `service_type` is " +
					"`compose`, and invalid otherwise. Dokploy's `backup.create` never accepts `compose` as a " +
					"`databaseType` — even for a compose-parented backup it must be the actual engine, because that " +
					"is what the server uses to build the dump command. `service_type` alone cannot describe both " +
					"the backup's parent kind and the underlying engine for a compose parent, so this attribute " +
					"carries the engine in that case. A change forces a replacement.",
				PlanModifiers: requiresReplace,
				Validators:    []validator.String{stringvalidator.OneOf(client.BackupDatabaseTypes...)},
			},
			"database": schema.StringAttribute{
				Required:    true,
				Description: "Name of the database to dump.",
			},
			"prefix": schema.StringAttribute{
				Required:    true,
				Description: "Key prefix inside the destination bucket, for example `backups/app/`.",
			},
			"cron_expression": schema.StringAttribute{
				Required:    true,
				Description: "Standard five-field cron expression, for example `0 4 * * *`.",
			},
			"destination_id": schema.StringAttribute{
				Required:    true,
				Description: "Id of the `dokploy_destination` that receives the dumps.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Whether the backup runs. Defaults to `true`. Dokploy leaves this field null for a record " +
					"from the API alone, which is neither on nor off, and a backup in the " +
					"configuration that never runs is the worse failure.",
			},
			"include_encryption_key": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Include the database encryption key in the dump. Defaults to `true`, the value that " +
					"Dokploy stores for a new backup. The provider always sends the field: the Dokploy " +
					"update endpoint stores `false` for an omitted field, so a request without it would " +
					"turn the key off on a record that had it on.",
			},
			"keep_latest_count": schema.Int64Attribute{
				Optional:    true,
				Description: "Number of dumps to keep. Omit it to keep all of them.",
			},
			"service_name": schema.StringAttribute{
				Optional:    true,
				Description: "Name of the specific container, for compose services with more than one.",
			},
			"compose_database_user": schema.StringAttribute{
				Optional: true,
				Description: "User that the dump command authenticates as, for a compose parent whose " +
					"`compose_database_type` is `postgres`, `mariadb`, or `mongo`. Required for those engines, and " +
					"invalid otherwise: a database parent carries its own credentials, and Dokploy reads this value " +
					"for a compose parent only. Without it, Dokploy builds no dump command, and every run of the " +
					"backup fails.",
			},
			"compose_database_password": schema.StringAttribute{
				Optional: true, Sensitive: true,
				Description: "Password of `compose_database_user`, for a compose parent whose `compose_database_type` " +
					"is `mariadb` or `mongo`. Set this attribute or `compose_database_password_wo` for those engines; " +
					"both are invalid otherwise.",
			},
			"compose_database_root_password": schema.StringAttribute{
				Optional: true, Sensitive: true,
				Description: "Root password of the MySQL server, for a compose parent whose `compose_database_type` " +
					"is `mysql`. Set this attribute or `compose_database_root_password_wo` for that engine; both are " +
					"invalid otherwise.",
			},
			"app_name": schema.StringAttribute{
				Computed:      true,
				Description:   "Internal Dokploy app name. The server generates it.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
	for _, name := range secretNames {
		for k, v := range tfutil.WriteOnlyCompanions(name, tfutil.WriteOnlyOptions{}) {
			resp.Schema.Attributes[k] = v
		}
	}
}

// secretNames lists the attributes with write-only companions. Neither one
// is ExactlyOne: an engine that has no use for the secret needs both forms
// unset (validateComposeCredentials).
var secretNames = []string{"compose_database_password", "compose_database_root_password"}

// writeOnlyInUse reports, per secret in secretNames, whether the config
// carries the write-only companion.
func writeOnlyInUse(cfg resourceModel) map[string]bool {
	return map[string]bool{
		"compose_database_password":      !cfg.ComposeDatabasePasswordWo.IsNull(),
		"compose_database_root_password": !cfg.ComposeDatabaseRootPasswordWo.IsNull(),
	}
}

// hideWriteOnly nulls each secret whose companion is in use (inUse is keyed
// by secretNames), after flatten put the server's cleartext value in.
func hideWriteOnly(m *resourceModel, inUse map[string]bool) {
	if inUse["compose_database_password"] {
		m.ComposeDatabasePassword = types.StringNull()
	}
	if inUse["compose_database_root_password"] {
		m.ComposeDatabaseRootPassword = types.StringNull()
	}
}

func (r *backupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// validateServiceType rejects Redis with an explanation and a pointer at the
// resource that does support it, rather than letting the apply fail on a zod
// enum error that says only "invalid option".
//
// The OneOf validator in the schema already rejects it at plan time; this is
// the belt-and-braces path for a value that reaches Create some other way,
// and the place the better message lives.
func validateServiceType(m resourceModel) error {
	if m.ServiceType.ValueString() == "redis" {
		return fmt.Errorf(
			"`service_type` cannot be `redis`: Dokploy has no logical dump for Redis. " +
				"Use `dokploy_volume_backup`, which snapshots the volume and does accept a Redis parent")
	}
	return nil
}

// validateComposeDatabaseType enforces the pairing between service_type and
// compose_database_type: the latter is required exactly when the former is
// "compose", and invalid otherwise. This is the belt-and-braces path for a
// value that reaches Create some other way; ValidateConfig is where a plan
// normally catches it, with an attribute-level diagnostic.
func validateComposeDatabaseType(m resourceModel) error {
	isCompose := m.ServiceType.ValueString() == "compose"
	hasComposeType := !m.ComposeDatabaseType.IsNull() && m.ComposeDatabaseType.ValueString() != ""
	switch {
	case isCompose && !hasComposeType:
		return fmt.Errorf(
			"`compose_database_type` is required when `service_type` is `compose`: Dokploy's `backup.create` " +
				"always needs the real database engine running inside the compose service, because `databaseType` " +
				"never accepts `compose` as a value")
	case !isCompose && hasComposeType:
		return fmt.Errorf(
			"`compose_database_type` is only valid when `service_type` is `compose`; it must be omitted for a %q parent",
			m.ServiceType.ValueString())
	}
	return nil
}

// validateComposeCredentials enforces the pairing between the engine inside
// a compose parent and the three credential attributes: each engine needs
// exactly the attributes of its dump command (client.BackupMetadata), and a
// database parent needs none. cfg carries the write-only companions, which
// the plan nulls; m carries everything else. The returned name is the
// attribute that the diagnostic points at.
//
// The required half is what closes issue #71: before v1.7.0 a compose backup
// applied without credentials, and every run of it failed on the server.
func validateComposeCredentials(m, cfg resourceModel) (string, error) {
	needs := needsFor(composeEngine(m))
	checks := []struct {
		attr      string
		need, has bool
		engines   string
		companion string
	}{
		{"compose_database_user", needs.user, !m.ComposeDatabaseUser.IsNull(),
			"`postgres`, `mariadb`, or `mongo`", ""},
		{"compose_database_password", needs.password,
			!m.ComposeDatabasePassword.IsNull() || !cfg.ComposeDatabasePasswordWo.IsNull(),
			"`mariadb` or `mongo`", " or `compose_database_password_wo`"},
		{"compose_database_root_password", needs.rootPassword,
			!m.ComposeDatabaseRootPassword.IsNull() || !cfg.ComposeDatabaseRootPasswordWo.IsNull(),
			"`mysql`", " or `compose_database_root_password_wo`"},
	}
	for _, c := range checks {
		switch {
		case c.need && !c.has:
			return c.attr, fmt.Errorf(
				"`%s`%s is required when `service_type` is `compose` and `compose_database_type` is %s: "+
					"Dokploy reads the credentials of a compose backup from the backup record, and builds no dump "+
					"command without them, so every run of the backup fails",
				c.attr, c.companion, c.engines)
		case !c.need && c.has:
			return c.attr, fmt.Errorf(
				"`%s`%s is only valid when `service_type` is `compose` and `compose_database_type` is %s: "+
					"a database parent carries its own credentials, and the other engines have no use for the value",
				c.attr, c.companion, c.engines)
		}
	}
	return "", nil
}

// hasUnknownCredential reports whether a credential attribute or companion
// is not yet known at plan time.
func hasUnknownCredential(cfg resourceModel) bool {
	return cfg.ComposeDatabaseUser.IsUnknown() ||
		cfg.ComposeDatabasePassword.IsUnknown() || cfg.ComposeDatabasePasswordWo.IsUnknown() ||
		cfg.ComposeDatabaseRootPassword.IsUnknown() || cfg.ComposeDatabaseRootPasswordWo.IsUnknown()
}

// ValidateConfig catches a service_type/compose_database_type mismatch, and
// a credential set that does not match the engine, at plan time, with an
// attribute-level diagnostic, rather than surfacing it only once Create runs
// the same checks.
//
// Unknown values (e.g. service_type derived from a resource attribute not
// yet known during plan) are skipped: there is nothing to validate against
// yet, and Create's belt-and-braces check still applies once both values are
// known.
func (r *backupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg resourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.ServiceType.IsUnknown() || cfg.ComposeDatabaseType.IsUnknown() {
		return
	}
	if err := validateComposeDatabaseType(cfg); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("compose_database_type"), "Invalid backup configuration", err.Error())
		return
	}
	if hasUnknownCredential(cfg) {
		return
	}
	if attr, err := validateComposeCredentials(cfg, cfg); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root(attr), "Invalid backup configuration", err.Error())
	}
}

func (r *backupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// The config, not the plan, carries the write-only values: the
	// framework nulls them in the plan (tfutil.WriteOnlyCompanions).
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateServiceType(plan); err != nil {
		resp.Diagnostics.AddError("Invalid backup configuration", err.Error())
		return
	}
	if err := validateComposeDatabaseType(plan); err != nil {
		resp.Diagnostics.AddError("Invalid backup configuration", err.Error())
		return
	}
	if _, err := validateComposeCredentials(plan, cfg); err != nil {
		resp.Diagnostics.AddError("Invalid backup configuration", err.Error())
		return
	}
	inUse := writeOnlyInUse(cfg)
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	metadata := metadataFor(plan,
		tfutil.SecretToCreate(plan.ComposeDatabasePassword, cfg.ComposeDatabasePasswordWo),
		tfutil.SecretToCreate(plan.ComposeDatabaseRootPassword, cfg.ComposeDatabaseRootPasswordWo))
	created, err := r.client.CreateBackup(ctx, parentRef(plan), createRequest(plan, metadata))
	if err != nil {
		resp.Diagnostics.AddError("Creating backup", err.Error())
		return
	}
	flatten(created, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	// The flags say which secrets stay out of the state: the API returns
	// them in cleartext, so a refresh would put them back otherwise.
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, secretNames)
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := r.client.GetBackup(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading backup", err.Error())
		return
	}
	flatten(b, &state)
	hideWriteOnly(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *backupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := writeOnlyInUse(cfg)
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	password, sendPassword := tfutil.SecretToUpdate(plan.ComposeDatabasePassword, cfg.ComposeDatabasePasswordWo,
		state.ComposeDatabasePassword, plan.ComposeDatabasePasswordWoVersion, state.ComposeDatabasePasswordWoVersion)
	rootPassword, sendRoot := tfutil.SecretToUpdate(plan.ComposeDatabaseRootPassword, cfg.ComposeDatabaseRootPasswordWo,
		state.ComposeDatabaseRootPassword, plan.ComposeDatabaseRootPasswordWoVersion, state.ComposeDatabaseRootPasswordWoVersion)
	if needs := needsFor(composeEngine(plan)); (needs.password && !sendPassword) || (needs.rootPassword && !sendRoot) {
		// backup.update replaces metadata as a whole (client/backup.go), so
		// a write-only secret with nothing new to send resends the stored
		// one, which the API returns in cleartext.
		current, err := r.client.GetBackup(ctx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading backup before update", err.Error())
			return
		}
		stored := storedCredentials(current)
		if !sendPassword {
			password = stored.password
		}
		if !sendRoot {
			rootPassword = stored.rootPassword
		}
	}
	if err := r.client.UpdateBackup(ctx, updateRequest(plan, metadataFor(plan, password, rootPassword))); err != nil {
		resp.Diagnostics.AddError("Updating backup", err.Error())
		return
	}
	b, err := r.client.GetBackup(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading backup after update", err.Error())
		return
	}
	flatten(b, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteBackup(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting backup", err.Error())
	}
}

func (r *backupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// UpgradeState moves a version 0 or version 1 state to the current schema.
//
// Version 0 named the cron attribute `schedule`. v0.11.0 renamed it to
// `cron_expression`, the name that dokploy_schedule and dokploy_volume_backup
// already use (D1 in the Phase 1 brief). The wire field stays `schedule`.
//
// Version 1 lacked `compose_database_type`, added in this release to let a
// dokploy_backup target a database running inside a dokploy_compose service
// (see databaseTypeFor in model.go).
func (r *backupResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	v0 := schemaV0(ctx)
	v1 := schemaV1(ctx)
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema:   &v0,
			StateUpgrader: upgradeStateV0,
		},
		1: {
			PriorSchema:   &v1,
			StateUpgrader: upgradeStateV1,
		},
	}
}

// currentSchema returns the schema this resource reports today, for the
// version 0 and version 1 derivations below to start from.
func currentSchema(ctx context.Context) schema.Schema {
	var resp resource.SchemaResponse
	(&backupResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	return resp.Schema
}

// schemaV0 derives the version 0 schema from the current one: the cron
// attribute is named `schedule`, and neither `compose_database_type` nor the
// credential attributes exist yet. Deriving rather than duplicating keeps
// every other attribute from drifting out of sync with the current schema.
func schemaV0(ctx context.Context) schema.Schema {
	s := schemaV1(ctx)
	attrs := s.Attributes
	attrs["schedule"] = attrs["cron_expression"]
	delete(attrs, "cron_expression")
	s.Version = 0
	return s
}

// schemaV1 derives the version 1 schema from the current one: identical
// except `compose_database_type` and the credential attributes do not exist
// yet. The upgrader models (resourceModelV0, resourceModelV1) carry none of
// them, and a prior schema with an attribute the model lacks fails the read.
func schemaV1(ctx context.Context) schema.Schema {
	s := currentSchema(ctx)
	delete(s.Attributes, "compose_database_type")
	for _, name := range credentialAttributes {
		delete(s.Attributes, name)
	}
	s.Version = 1
	return s
}

func upgradeStateV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior resourceModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, prior.upgrade())...)
}

func upgradeStateV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior resourceModelV1
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, prior.upgrade())...)
}
