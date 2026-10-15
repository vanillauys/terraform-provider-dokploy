package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func hardenedReadFixture(t *testing.T, path string) (string, string, map[string]any) {
	t.Helper()
	var dto any
	switch path {
	case "/project.one":
		dto = Project{ProjectID: "expected", Name: "lab", CreatedAt: "2026-10-15T00:00:00Z", Environments: []Environment{{EnvironmentID: "env", Name: "production", ProjectID: "expected", IsDefault: true}}}
	case "/environment.one":
		dto = Environment{EnvironmentID: "expected", Name: "lab", ProjectID: "parent"}
	case "/compose.one":
		dto = Compose{ComposeID: "expected", Name: "lab", AppName: "lab-abcdef", EnvironmentID: "parent", ComposeType: "docker-compose", SourceType: "raw", ComposeFile: "services: {}", ComposePath: "./docker-compose.yml", ComposeStatus: "done", CreatedAt: "2026-10-15T00:00:00Z"}
	case "/domain.one":
		dto = Domain{DomainID: "expected", Host: "lab.invalid", Path: "/", InternalPath: "/", CertificateType: "none", DomainType: "compose", ComposeID: ptr("parent"), CreatedAt: "2026-10-15T00:00:00Z"}
	case "/destination.one":
		dto = Destination{DestinationID: "expected", Name: "lab", Provider: "AWS", CreatedAt: "2026-10-15T00:00:00Z"}
	case "/server.one":
		dto = Server{ServerID: "expected", Name: "lab", IPAddress: "192.0.2.1", Username: "root", AppName: "lab-abcdef", ServerType: "deploy", CreatedAt: "2026-10-15T00:00:00Z"}
	case "/sshKey.one":
		dto = SSHKey{SSHKeyID: "expected", Name: "lab", PublicKey: "FAKE_PUBLIC", PrivateKey: "FAKE_PRIVATE", CreatedAt: "2026-10-15T00:00:00Z"}
	case "/volumeBackups.one":
		dto = VolumeBackup{VolumeBackupID: "expected", Name: "lab", VolumeName: "lab-data", DestinationID: "dest", ServiceType: "compose", ComposeID: ptr("parent"), AppName: "lab-abcdef", CreatedAt: "2026-10-15T00:00:00Z"}
	default:
		t.Fatal("unhandled fixture")
	}
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err = json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	identity := map[string]string{"/project.one": "projectId", "/environment.one": "environmentId", "/compose.one": "composeId", "/domain.one": "domainId", "/destination.one": "destinationId", "/server.one": "serverId", "/sshKey.one": "sshKeyId", "/volumeBackups.one": "volumeBackupId"}[path]
	return identity, "expected", object
}
func ptr(v string) *string { return &v }

func TestHardenedReadRejectsUntrustedSuccess(t *testing.T) {
	paths := []string{"/compose.one", "/project.one", "/environment.one", "/domain.one", "/destination.one", "/server.one", "/sshKey.one", "/volumeBackups.one"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			key, id, object := hardenedReadFixture(t, path)
			valid, _ := json.Marshal(object)
			wrong := map[string]any{}
			for k, v := range object {
				wrong[k] = v
			}
			wrong[key] = "foreign"
			wrongRaw, _ := json.Marshal(wrong)
			sparse, _ := json.Marshal(map[string]string{key: id})
			missing := map[string]any{}
			for k, v := range object {
				missing[k] = v
			}
			delete(missing, "name")
			if path == "/domain.one" {
				delete(missing, "host")
			}
			missingRaw, _ := json.Marshal(missing)
			duplicates := append([]byte(`{"`+key+`":"foreign",`), valid[1:]...)
			folded := append([]byte(`{"`+strings.ToUpper(key)+`":"foreign",`), valid[1:]...)
			for name, body := range map[string][]byte{"empty": {}, "null": []byte(`null`), "object": []byte(`{}`), "array": []byte(`[]`), "wrong_id": wrongRaw, "id_only": sparse, "missing_field": missingRaw, "duplicate_id": duplicates, "case_variant_id": folded, "malformed": []byte(`{"` + key + `":`), "trailing": append(append([]byte{}, valid...), []byte(` {}`)...)} {
				t.Run(name, func(t *testing.T) {
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
					defer srv.Close()
					out := map[string]any{"prior": "state"}
					before := map[string]any{"prior": "state"}
					err := testClient(t, srv).Get(context.Background(), path, url.Values{key: {id}}, &out)
					if err == nil || errors.Is(err, ErrNotFound) {
						t.Fatalf("invalid success err=%v", err)
					}
					if !reflect.DeepEqual(out, before) {
						t.Fatalf("failed read modified caller: %+v", out)
					}
				})
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(valid) }))
			defer srv.Close()
			var out map[string]any
			if err := testClient(t, srv).Get(context.Background(), path, url.Values{key: {id}}, &out); err != nil {
				t.Fatalf("valid complete object: %v", err)
			}
		})
	}
}

func TestHardenedAmbiguous404DoesNotEraseState(t *testing.T) {
	for name, body := range map[string]string{
		"empty": "", "html": "<html>404 Not Found</html>", "malformed": `{"message":`, "unstructured": `{"message":"Project not found","code":"NOT_FOUND"}`,
		"wrong_route":      `{"code":"NOT_FOUND","message":"Project not found","data":{"code":"NOT_FOUND","httpStatus":404,"path":"compose.one"}}`,
		"route_error":      `{"code":"NOT_FOUND","message":"No procedure found on path project.one","data":{"code":"NOT_FOUND","httpStatus":404,"path":"project.one"}}`,
		"duplicate_code":   `{"code":"UNAUTHORIZED","code":"NOT_FOUND","message":"Project not found","data":{"code":"NOT_FOUND","httpStatus":404,"path":"project.one"}}`,
		"duplicate_nested": `{"code":"NOT_FOUND","message":"Project not found","data":{"code":"NOT_FOUND","httpStatus":403,"httpStatus":404,"path":"project.one"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404); _, _ = fmt.Fprint(w, body) }))
			defer srv.Close()
			err := testClient(t, srv).Get(context.Background(), "/project.one", url.Values{"projectId": {"gone"}}, new(Project))
			if err == nil || errors.Is(err, ErrNotFound) {
				t.Fatalf("ambiguous404 err=%v", err)
			}
		})
	}
	for _, status := range []int{401, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, `{"code":"NOT_FOUND","message":"Project not found","data":{"code":"NOT_FOUND","httpStatus":404,"path":"project.one"}}`)
			}))
			defer srv.Close()
			err := testClient(t, srv).Get(context.Background(), "/project.one", url.Values{"projectId": {"gone"}}, new(Project))
			if err == nil || errors.Is(err, ErrNotFound) {
				t.Fatalf("status%d err=%v", status, err)
			}
		})
	}
}

func TestHardenedConfirmedMissingRecord(t *testing.T) {
	for _, path := range []string{"/compose.one", "/project.one", "/environment.one", "/domain.one", "/destination.one", "/server.one", "/sshKey.one", "/volumeBackups.one", "/port.one"} {
		t.Run(path, func(t *testing.T) {
			name := strings.TrimPrefix(strings.Split(path, ".")[0], "/")
			if name == "volumeBackups" {
				name = "volumeBackup"
			}
			key := name + "Id"
			code := "NOT_FOUND"
			status := 404
			if path == "/port.one" {
				code = "BAD_REQUEST"
				status = 400
			}
			raw, _ := json.Marshal(map[string]any{"code": code, "message": name + " not found", "data": map[string]any{"code": code, "httpStatus": status, "path": strings.TrimPrefix(path, "/")}})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); _, _ = w.Write(raw) }))
			defer srv.Close()
			err := testClient(t, srv).Get(context.Background(), path, url.Values{key: {"gone"}}, nil)
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("confirmed missing record: %v", err)
			}
		})
	}
}

func TestHardenedResponseLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, io.LimitReader(strings.NewReader(strings.Repeat("x", maxResponseBytes+1)), maxResponseBytes+1))
	}))
	defer srv.Close()
	err := testClient(t, srv).Get(context.Background(), "/project.all", nil, new([]Project))
	if err == nil {
		t.Fatal("oversized response accepted")
	}
}

// Complete protocol envelope for legacy not-found tests. Bare status codes
// and unstructured messages are covered separately as unsafe/ambiguous.
func missingRecordErrorFixture(path string, status int) string {
	entity := strings.Split(path, ".")[0]
	switch entity {
	case "certificates":
		entity = "certificate"
	case "volumeBackups":
		entity = "volumeBackup"
	}
	code := "NOT_FOUND"
	if status == 400 {
		code = "BAD_REQUEST"
	}
	raw, _ := json.Marshal(map[string]any{"code": code, "message": entity + " not found", "data": map[string]any{"code": code, "httpStatus": status, "path": path}})
	return string(raw)
}

func TestHardenedMutationRejectsHTMLSuccess(t *testing.T) {
	for _, route := range []string{"/compose.delete", "/project.remove", "/compose.update"} {
		t.Run(route, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = fmt.Fprint(w, "<html>unexpected login page</html>")
			}))
			defer srv.Close()
			err := testClient(t, srv).Post(context.Background(), route, map[string]string{"composeId": "owned"}, nil)
			if err == nil || calls.Load() != 1 {
				t.Fatalf("mutation err=%v attempts=%d", err, calls.Load())
			}
		})
	}
}

func TestHardenedManagedCollectionsRejectSparseEntries(t *testing.T) {
	for _, test := range []struct {
		path, field string
		value       any
	}{
		{"/compose.one", "serviceNetworks", []any{map[string]any{"serviceName": "web"}}},
		{"/project.one", "projectTags", []any{map[string]any{"id": "join"}}},
		{"/project.one", "environments", []any{map[string]any{"environmentId": "e", "name": "prod"}}},
	} {
		t.Run(test.field, func(t *testing.T) {
			key, id, object := hardenedReadFixture(t, test.path)
			object[test.field] = test.value
			raw, _ := json.Marshal(object)
			if validateResponse(http.MethodGet, test.path, url.Values{key: {id}}, raw) == nil {
				t.Fatal("sparse collection accepted")
			}
		})
	}
}

func TestHardenedDocumentedNullEnvironmentEnv(t *testing.T) {
	key, id, object := hardenedReadFixture(t, "/environment.one")
	object["env"] = nil
	raw, _ := json.Marshal(object)
	if err := validateResponse(http.MethodGet, "/environment.one", url.Values{key: {id}}, raw); err != nil {
		t.Fatal(err)
	}
}

func TestReadDecodeFailurePreservesOutput(t *testing.T) {
	out := map[string]int{"prior": 7}
	if err := decodeResponse([]byte(`{"prior":"wrong type"}`), &out); err == nil || out["prior"] != 7 {
		t.Fatalf("decode failed to preserve prior output: %+v err=%v", out, err)
	}
}
