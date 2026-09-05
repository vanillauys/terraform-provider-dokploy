package envvars

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                     = (*envVarsResource)(nil)
	_ resource.ResourceWithConfigure        = (*envVarsResource)(nil)
	_ resource.ResourceWithImportState      = (*envVarsResource)(nil)
	_ resource.ResourceWithConfigValidators = (*envVarsResource)(nil)
	_ resource.ResourceWithValidateConfig   = (*envVarsResource)(nil)
)

type envVarsResource struct{ client *client.Client }

func NewResource() resource.Resource { return &envVarsResource{} }

type resourceModel struct {
	ID            types.String `tfsdk:"id"`
	ApplicationID types.String `tfsdk:"application_id"`
	ComposeID     types.String `tfsdk:"compose_id"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	Variables     types.Map    `tfsdk:"variables"`
}

func (r *envVarsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment_variables"
}

func (r *envVarsResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("application_id"),
			path.MatchRoot("compose_id"),
			path.MatchRoot("environment_id"),
		),
	}
}

// ValidateConfig rejects a value with a line break: the wire format is one
// variable per line, so such a value could not be read back.
func (r *envVarsResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg resourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Variables.IsNull() || cfg.Variables.IsUnknown() {
		return
	}
	for key, value := range cfg.Variables.Elements() {
		s, ok := value.(types.String)
		if !ok || s.IsUnknown() || s.IsNull() {
			continue
		}
		if strings.ContainsAny(s.ValueString(), "\r\n") {
			resp.Diagnostics.AddAttributeError(path.Root("variables").AtMapKey(key), "Multiline value",
				"Dokploy stores one variable per line, so a value cannot contain a line break. Encode it, for example with base64encode().")
		}
		if strings.ContainsAny(key, "=\r\n ") || key == "" {
			resp.Diagnostics.AddAttributeError(path.Root("variables").AtMapKey(key), "Invalid variable name",
				"A variable name cannot be empty or contain `=`, a space, or a line break.")
		}
	}
}

func targetAttribute(kind, label, note string) schema.Attribute {
	return schema.StringAttribute{
		Optional:      true,
		Description:   "Id of the " + label + " whose variables this resource owns." + note + " Set exactly one of `application_id`, `compose_id`, or `environment_id`. A change replaces the resource.",
		PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
}

func (r *envVarsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The environment variables of one application, compose, or environment, as a map. The resource " +
			"owns the whole `env` text of its target: each apply writes every variable in the map, in key order, and " +
			"removes every other line.\n\n" +
			"~> **The target must not manage `env` itself.** `dokploy_application`, `dokploy_compose`, and " +
			"`dokploy_environment` refresh their `env` attribute from the server, so a target without " +
			"`lifecycle { ignore_changes = [env] }` plans to clear what this resource wrote on its next apply. Add that " +
			"lifecycle block to the target, and do not set `env` on it.\n\n" +
			"~> **A change here does not redeploy the service.** Dokploy applies the variables on the next deploy of " +
			"the application or the compose. An environment's shared variables reach a service on its next deploy too.\n\n" +
			"~> Values are written verbatim: no quotes, no escaping. A value cannot contain a line break; encode " +
			"such a value first. Comment lines that a person wrote in the Dokploy UI do not survive an apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "`application/<id>`, `compose/<id>`, or `environment/<id>`; also the import id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"application_id": targetAttribute("application", "application", ""),
			"compose_id":     targetAttribute("compose", "compose service", ""),
			"environment_id": targetAttribute("environment", "environment", " Every service in the environment shares them."),
			"variables": schema.MapAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "The variables, name to value. Use Terraform sensitive variables for secret values: the map " +
					"itself is not sensitive, and Dokploy stores and returns it in cleartext.",
			},
		},
	}
}

func (r *envVarsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// target names the record the model points at.
type target struct{ kind, id string }

func targetOf(m resourceModel) target {
	switch {
	case !m.ApplicationID.IsNull():
		return target{"application", m.ApplicationID.ValueString()}
	case !m.ComposeID.IsNull():
		return target{"compose", m.ComposeID.ValueString()}
	default:
		return target{"environment", m.EnvironmentID.ValueString()}
	}
}

func (t target) String() string { return t.kind + "/" + t.id }

// readEnv returns the target's env text.
func (r *envVarsResource) readEnv(ctx context.Context, t target) (string, error) {
	switch t.kind {
	case "application":
		app, err := r.client.GetApplication(ctx, t.id)
		if err != nil {
			return "", err
		}
		return deref(app.Env), nil
	case "compose":
		c, err := r.client.GetCompose(ctx, t.id)
		if err != nil {
			return "", err
		}
		return deref(c.Env), nil
	default:
		e, err := r.client.GetEnvironment(ctx, t.id)
		if err != nil {
			return "", err
		}
		return e.Env, nil
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// writeEnv stores the text. Every endpoint carries more than env, and two
// of them wipe an absent sibling field (application.saveEnvironment is
// dialect A), so each write reads the target first and resends the other
// fields as stored.
func (r *envVarsResource) writeEnv(ctx context.Context, t target, text string) error {
	switch t.kind {
	case "application":
		app, err := r.client.GetApplication(ctx, t.id)
		if err != nil {
			return err
		}
		return r.client.SaveApplicationEnvironment(ctx, client.SaveApplicationEnvironmentRequest{
			ApplicationID: t.id,
			Env:           &text,
			BuildArgs:     app.BuildArgs,
			BuildSecrets:  app.BuildSecrets,
			CreateEnvFile: &app.CreateEnvFile,
		})
	case "compose":
		c, err := r.client.GetCompose(ctx, t.id)
		if err != nil {
			return err
		}
		return r.client.SaveComposeEnvironment(ctx, client.SaveComposeEnvironmentRequest{
			ComposeID:     t.id,
			Env:           &text,
			CreateEnvFile: &c.CreateEnvFile,
		})
	default:
		e, err := r.client.GetEnvironment(ctx, t.id)
		if err != nil {
			return err
		}
		return r.client.UpdateEnvironment(ctx, client.UpdateEnvironmentRequest{
			EnvironmentID: t.id,
			Name:          e.Name,
			Description:   e.Description,
			Env:           text,
		})
	}
}

func variablesOf(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	vars := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return vars
	}
	diags.Append(m.ElementsAs(ctx, &vars, false)...)
	return vars
}

func (r *envVarsResource) apply(ctx context.Context, m *resourceModel, diags *diag.Diagnostics) {
	t := targetOf(*m)
	vars := variablesOf(ctx, m.Variables, diags)
	if diags.HasError() {
		return
	}
	if err := r.writeEnv(ctx, t, renderEnv(vars)); err != nil {
		diags.AddError("Writing environment variables", fmt.Sprintf("%s: %s", t, err))
		return
	}
	m.ID = types.StringValue(t.String())
	r.refresh(ctx, m, diags)
}

// refresh reads the target's env back into the map.
func (r *envVarsResource) refresh(ctx context.Context, m *resourceModel, diags *diag.Diagnostics) {
	t := targetOf(*m)
	text, err := r.readEnv(ctx, t)
	if err != nil {
		diags.AddError("Reading environment variables", fmt.Sprintf("%s: %s", t, err))
		return
	}
	vars, err := parseEnv(text)
	if err != nil {
		diags.AddError("Reading environment variables", fmt.Sprintf("%s: the stored env text has a line this resource cannot represent: %s", t, err))
		return
	}
	value, d := types.MapValueFrom(ctx, types.StringType, vars)
	diags.Append(d...)
	m.Variables = value
}

func (r *envVarsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *envVarsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	text, err := r.readEnv(ctx, targetOf(state))
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading environment variables", err.Error())
		return
	}
	vars, err := parseEnv(text)
	if err != nil {
		resp.Diagnostics.AddError("Reading environment variables", err.Error())
		return
	}
	value, d := types.MapValueFrom(ctx, types.StringType, vars)
	resp.Diagnostics.Append(d...)
	state.Variables = value
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *envVarsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete clears the target's env text. A target that is already gone is
// fine.
func (r *envVarsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.writeEnv(ctx, targetOf(state), ""); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Clearing environment variables", err.Error())
	}
}

// ImportState takes `application/<id>`, `compose/<id>`, or
// `environment/<id>`.
func (r *envVarsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	kind, id, ok := strings.Cut(req.ID, "/")
	attr := map[string]string{"application": "application_id", "compose": "compose_id", "environment": "environment_id"}[kind]
	if !ok || attr == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import id",
			fmt.Sprintf("expected application/<id>, compose/<id>, or environment/<id>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(attr), id)...)
}
