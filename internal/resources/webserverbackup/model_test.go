package webserverbackup

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFlatten(t *testing.T) {
	on := true
	keep := int64(7)
	var m resourceModel
	flatten(&client.Backup{
		BackupID: "b1", DestinationID: "d1", Schedule: "0 3 * * *", Prefix: "p/",
		Enabled: &on, KeepLatestCount: &keep, IncludeEncryptionKey: true,
	}, &m)
	if m.ID.ValueString() != "b1" || m.DestinationID.ValueString() != "d1" ||
		m.CronExpression.ValueString() != "0 3 * * *" || m.Prefix.ValueString() != "p/" {
		t.Errorf("flatten strings = %+v", m)
	}
	if !m.Enabled.ValueBool() || !m.IncludeEncryptionKey.ValueBool() || m.KeepLatestCount.ValueInt64() != 7 {
		t.Errorf("flatten scalars = %+v", m)
	}

	flatten(&client.Backup{BackupID: "b1"}, &m)
	if m.Enabled.ValueBool() || !m.KeepLatestCount.IsNull() {
		t.Errorf("a null enabled must read false and a null keepLatestCount must read null: %+v", m)
	}
}

func TestRequests(t *testing.T) {
	on := true
	m := resourceModel{
		ID: types.StringValue("b1"), DestinationID: types.StringValue("d1"),
		CronExpression: types.StringValue("0 3 * * *"), Prefix: types.StringValue("p/"),
		Enabled: types.BoolValue(on), IncludeEncryptionKey: types.BoolValue(true),
		KeepLatestCount: types.Int64Null(),
	}
	c := createRequest(m, "u1")
	if c.DatabaseType != "web-server" || c.BackupType != "database" || c.UserID != "u1" ||
		c.Database != "dokploy" || c.KeepLatestCount != nil || c.Enabled == nil || !*c.Enabled {
		t.Errorf("createRequest = %+v", c)
	}
	u := updateRequest(m)
	if u.BackupID != "b1" || u.DatabaseType != "web-server" || u.Database != "dokploy" ||
		u.DestinationID != "d1" || u.Schedule != "0 3 * * *" || u.Prefix != "p/" ||
		u.KeepLatestCount != nil || !u.IncludeEncryptionKey {
		t.Errorf("updateRequest = %+v", u)
	}
}
