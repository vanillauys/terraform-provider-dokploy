package client

import (
	"reflect"
	"strings"
	"testing"
)

// mustAlwaysSend lists, per request struct, the JSON field names that have to
// reach the server on every call.
//
// See doc.go for why. In short: for dialect A an absent key is an HTTP 400,
// for dialect B an absent key silently keeps the stored value, and for
// dialect C an absent key ALSO silently keeps the stored value (only ""
// clears). None of the three ever means "clear this" for an absent key. So
// `omitempty` on any field listed here makes the corresponding Terraform
// attribute impossible to set back to null, which surfaces as a plan that
// shows the same diff forever.
//
// When you add a request struct for a dialect A, B, or C endpoint, add it
// here.
var mustAlwaysSend = []struct {
	value  any
	fields []string
}{
	{UpdateProjectRequest{}, []string{"description"}},
	{UpdatePostgresRequest{}, []string{"description", "networkIds", "detachDokployNetwork"}},
	{UpdateMysqlRequest{}, []string{"description", "databaseRootPassword", "networkIds", "detachDokployNetwork"}},
	{UpdateMariadbRequest{}, []string{"description", "databaseRootPassword", "networkIds", "detachDokployNetwork"}},
	{UpdateRedisRequest{}, []string{"description", "networkIds", "detachDokployNetwork"}},
	{UpdateMongoRequest{}, []string{"description", "networkIds", "detachDokployNetwork"}},
	{UpdateApplicationRequest{}, []string{"description", "networkIds", "detachDokployNetwork"}},
	{UpdateDomainRequest{}, []string{
		"host", "path", "internalPath", "port", "https", "stripPath",
		"certificateType", "customCertResolver", "customEntrypoint",
		"serviceName", "forwardAuthEnabled", "domainType",
		"applicationId", "composeId", "enabled",
	}},
	{UpdateEnvironmentRequest{}, []string{"name", "description", "env"}},
	{CreateEnvironmentRequest{}, []string{"description"}},
	// The dialect A application endpoints. Until wave 3 none of them was in
	// this table at all: it held only dialect B Update* structs, so the
	// endpoints where an absent key is a hard 400 were entirely unguarded.
	{SaveGithubProviderRequest{}, []string{"triggerType", "watchPaths", "enableSubmodules"}},
	{SaveGitProviderRequest{}, []string{"customGitSSHKeyId", "watchPaths", "enableSubmodules"}},
	{SaveDockerProviderRequest{}, []string{"username", "password", "registryUrl"}},
	{SaveBuildTypeRequest{}, []string{
		"dockerfile", "dockerContextPath", "dockerBuildStage",
		"publishDirectory", "herokuVersion", "railpackVersion", "isStaticSpa",
	}},
	{SaveApplicationEnvironmentRequest{}, []string{"env", "buildArgs", "buildSecrets", "createEnvFile"}},
	{CreateMountRequest{}, []string{"hostPath", "volumeName", "filePath", "content"}},
	{UpdateMountRequest{}, []string{"hostPath", "volumeName", "filePath", "content"}},
	{CreateScheduleRequest{}, []string{"description", "script", "enabled", "timezone", "serviceName"}},
	{UpdateScheduleRequest{}, []string{"description", "script", "enabled", "timezone", "serviceName"}},
	{CreateVolumeBackupRequest{}, []string{"serviceName", "keepLatestCount", "enabled"}},
	{UpdateVolumeBackupRequest{}, []string{"serviceName", "keepLatestCount", "enabled"}},
	{CreateBackupRequest{}, []string{"serviceName", "keepLatestCount", "enabled"}},
	{UpdateBackupRequest{}, []string{"serviceName", "keepLatestCount", "enabled", "metadata"}},
	// compose.update is dialect B at the endpoint level, but its fields
	// split three ways (doc.go). Every managed field is listed: the dialect
	// C group and the two enums because they must reach the wire as "" to
	// clear, the rest because a nil must marshal to an explicit null.
	{UpdateComposeRequest{}, []string{
		"name", "composePath", "command", "suffix", "composeFile",
		"composeType", "sourceType",
		"description", "repository", "owner", "branch", "githubId",
		"customGitUrl", "customGitBranch", "customGitSSHKeyId",
		"triggerType", "autoDeploy", "enableSubmodules", "randomize",
		"isolatedDeployment", "isolatedDeploymentsVolume", "watchPaths",
		"icon", "serviceNetworks",
	}},
	{SaveComposeEnvironmentRequest{}, []string{"env", "createEnvFile"}},
	// libsql.update is dialect B: an absent key keeps the stored value.
	// This row lists every clearable pointer field on UpdateLibsqlRequest -
	// description, sqldPrimaryUrl, command, cpuLimit, cpuReservation,
	// memoryLimit, memoryReservation - plus the v0.30.0 network pair,
	// networkIds and detachDokployNetwork. Each one needs an explicit
	// null on the wire to clear the stored value, so omitempty on any of
	// them would drop that null and freeze the field at whatever the
	// server already holds.
	//
	// The struct's other fields - name, databaseUser, databasePassword,
	// sqldNode, enableNamespaces, dockerImage, replicas - keep their
	// omitempty tag on purpose: they are plain strings or values with an
	// update-if-present shape, not nullable-clear fields, so this row
	// does not guard them. See libsql.go's UpdateLibsqlRequest doc
	// comment for the field-by-field reasoning.
	{UpdateLibsqlRequest{}, []string{
		"description", "sqldPrimaryUrl", "command", "cpuLimit",
		"cpuReservation", "memoryLimit", "memoryReservation",
		"networkIds", "detachDokployNetwork",
	}},
}

// inMustAlwaysSend reports whether a request struct is registered above. It
// is what stops a new dialect A endpoint from reaching the server with no
// omitempty guard at all — the exact gap the five entries above just closed.
func inMustAlwaysSend(typ reflect.Type) bool {
	for _, tc := range mustAlwaysSend {
		if reflect.TypeOf(tc.value) == typ {
			return true
		}
	}
	return false
}

func TestRequestStructsNeverOmitMustSendFields(t *testing.T) {
	for _, tc := range mustAlwaysSend {
		typ := reflect.TypeOf(tc.value)
		for _, want := range tc.fields {
			field, ok := fieldByJSONName(typ, want)
			if !ok {
				t.Errorf("%s: no field carries json name %q", typ.Name(), want)
				continue
			}
			if hasOmitempty(field) {
				t.Errorf(
					"%s.%s has `omitempty` on json:%q.\n"+
						"See internal/client/doc.go: for this endpoint an absent key is "+
						"either an HTTP 400 or a silent keep-the-old-value, never a clear. "+
						"With omitempty a null Terraform value vanishes from the request "+
						"body, so the attribute can never be cleared and every subsequent "+
						"plan shows the same diff.",
					typ.Name(), field.Name, want)
			}
		}
	}
}

func fieldByJSONName(t reflect.Type, name string) (reflect.StructField, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if jsonName(f) == name {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

// jsonName is the wire name a field marshals to: the json tag's first
// comma-separated element. It is "" for an untagged field and for `json:"-"`,
// both of which mean the field never reaches the server.
func jsonName(f reflect.StructField) string {
	name := strings.Split(f.Tag.Get("json"), ",")[0]
	if name == "-" {
		return ""
	}
	return name
}

func hasOmitempty(f reflect.StructField) bool {
	parts := strings.Split(f.Tag.Get("json"), ",")
	for _, p := range parts[1:] {
		if p == "omitempty" {
			return true
		}
	}
	return false
}
