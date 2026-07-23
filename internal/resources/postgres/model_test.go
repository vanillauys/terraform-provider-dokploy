package postgres

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDeployNeeded(t *testing.T) {
	base := func() resourceModel {
		return resourceModel{
			DockerImage:      types.StringValue("postgres:16-alpine"),
			DatabasePassword: types.StringValue("hunter2"),
			Env:              types.StringValue("A=1"),
			ExternalPort:     types.Int64Value(5432),
			Name:             types.StringValue("db"),
		}
	}

	state := base()
	plan := base()
	if deployNeeded(plan, state) {
		t.Error("identical models must not trigger a deploy")
	}

	plan = base()
	plan.Name = types.StringValue("renamed")
	if deployNeeded(plan, state) {
		t.Error("name is not a deploy trigger")
	}

	for name, mutate := range map[string]func(*resourceModel){
		"docker_image":      func(m *resourceModel) { m.DockerImage = types.StringValue("postgres:17") },
		"database_password": func(m *resourceModel) { m.DatabasePassword = types.StringValue("changed") },
		"env":               func(m *resourceModel) { m.Env = types.StringValue("A=2") },
		"external_port":     func(m *resourceModel) { m.ExternalPort = types.Int64Value(5433) },
	} {
		plan = base()
		mutate(&plan)
		if !deployNeeded(plan, state) {
			t.Errorf("%s change must trigger a deploy", name)
		}
	}
}
