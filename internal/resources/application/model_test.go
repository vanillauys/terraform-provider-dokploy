package application

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }

func TestFlattenSourceDockerOnly(t *testing.T) {
	app := &client.Application{
		ApplicationID:     "app1",
		Name:              "web",
		AppName:           "web-a1b2",
		EnvironmentID:     "e1",
		ApplicationStatus: "done",
		SourceType:        "docker",
		DockerImage:       strPtr("traefik/whoami:v1.10"),
		BuildType:         "nixpacks",
		CreatedAt:         "2026-07-23T10:00:00.000Z",
	}
	var m resourceModel
	diags := flatten(context.Background(), app, &m)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if m.Docker.IsNull() {
		t.Error("docker source must be populated")
	}
	if !m.Github.IsNull() || !m.Git.IsNull() {
		t.Error("inactive source blocks must be null")
	}
	var d dockerModel
	m.Docker.As(context.Background(), &d, objectAsOptions)
	if d.Image.ValueString() != "traefik/whoami:v1.10" {
		t.Errorf("docker = %+v", d)
	}
	var b buildModel
	m.Build.As(context.Background(), &b, objectAsOptions)
	if b.Type.ValueString() != "nixpacks" {
		t.Errorf("build = %+v", b)
	}
}

func TestDeployNeededApplication(t *testing.T) {
	ctx := context.Background()
	docker := func(image string) types.Object {
		obj, _ := types.ObjectValueFrom(ctx, dockerAttrTypes, dockerModel{
			Image:       types.StringValue(image),
			Username:    types.StringNull(),
			Password:    types.StringNull(),
			RegistryURL: types.StringNull(),
		})
		return obj
	}
	base := func() resourceModel {
		return resourceModel{
			Name:   types.StringValue("web"),
			Docker: docker("nginx:1"),
			Github: types.ObjectNull(githubAttrTypes),
			Git:    types.ObjectNull(gitAttrTypes),
			Build:  types.ObjectNull(buildAttrTypes),
			Env:    types.StringValue("A=1"),
		}
	}
	state, plan := base(), base()
	if deployNeeded(plan, state) {
		t.Error("identical models must not trigger a deploy")
	}
	plan = base()
	plan.Name = types.StringValue("renamed")
	if deployNeeded(plan, state) {
		t.Error("name is not a deploy trigger")
	}
	plan = base()
	plan.Docker = docker("nginx:2")
	if !deployNeeded(plan, state) {
		t.Error("docker source change must trigger a deploy")
	}
	plan = base()
	plan.Env = types.StringValue("A=2")
	if !deployNeeded(plan, state) {
		t.Error("env change must trigger a deploy")
	}
}
