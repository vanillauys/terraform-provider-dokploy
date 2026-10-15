package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
)

const maxResponseBytes = 16 << 20

// strictJSON rejects ambiguous duplicate properties and trailing documents,
// including duplicates in nested objects. Errors never contain response data.
func strictJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("JSON nesting limit exceeded")
		}
		token, err := d.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON")
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return fmt.Errorf("invalid JSON object")
				}
				name, ok := key.(string)
				// encoding/json matches struct fields case-insensitively. Reject
				// case variants too, so validation and decoding cannot disagree.
				if !ok || seen[strings.ToLower(name)] {
					return fmt.Errorf("ambiguous JSON object")
				}
				seen[strings.ToLower(name)] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
			close, err := d.Token()
			if err != nil || close != json.Delim('}') {
				return fmt.Errorf("invalid JSON object")
			}
		case json.Delim('['):
			for d.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
			close, err := d.Token()
			if err != nil || close != json.Delim(']') {
				return fmt.Errorf("invalid JSON array")
			}
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}

// Decode transactionally: a type error must not partially overwrite the caller.
func decodeResponse(raw []byte, out any) error {
	dst := reflect.ValueOf(out)
	if dst.Kind() != reflect.Pointer || dst.IsNil() {
		return fmt.Errorf("response destination must be a pointer")
	}
	next := reflect.New(dst.Elem().Type())
	if err := json.Unmarshal(raw, next.Interface()); err != nil {
		return fmt.Errorf("invalid response type")
	}
	dst.Elem().Set(next.Elem())
	return nil
}

// A status code alone cannot distinguish a missing record from a missing
// route, reverse proxy failure, or authentication layer hiding a resource.
// Only the expected resource's structured tRPC error may erase state.
func confirmedNotFound(method, path string, query url.Values, payload []byte, status int, raw []byte) bool {
	if strictJSON(raw) != nil {
		return false
	}
	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Code       string `json:"code"`
			HTTPStatus int    `json:"httpStatus"`
			Path       string `json:"path"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return false
	}
	expected := "NOT_FOUND"
	if path == "/port.one" && status == http.StatusBadRequest {
		expected = "BAD_REQUEST"
	} else if status != http.StatusNotFound {
		return false
	}
	if envelope.Code != expected || envelope.Data.Code != expected || envelope.Data.HTTPStatus != status || envelope.Data.Path != strings.TrimPrefix(path, "/") {
		return false
	}
	lookup := method == http.MethodGet && strings.HasSuffix(path, ".one")
	deletion := method == http.MethodPost && (strings.HasSuffix(path, ".delete") || strings.HasSuffix(path, ".remove"))
	if !lookup && !deletion {
		return false
	}
	var id string
	if lookup {
		_, id = lookupIdentity(path, query)
	} else {
		var body map[string]json.RawMessage
		if json.Unmarshal(payload, &body) != nil {
			return false
		}
		for key, value := range body {
			if strings.HasSuffix(key, "Id") {
				var v string
				if json.Unmarshal(value, &v) == nil && v != "" {
					if id != "" {
						return false
					}
					id = v
				}
			}
		}
	}
	if id == "" {
		return false
	}
	entity := strings.TrimPrefix(strings.SplitN(path, ".", 2)[0], "/")
	switch entity {
	case "volumeBackups":
		entity = "volumeBackup"
	case "redirects":
		entity = "redirect"
	case "mounts":
		entity = "mount"
	case "certificates":
		entity = "certificate"
	}
	normalize := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), "")) }
	return normalize(envelope.Message) == normalize(entity+" not found")
}

func lookupIdentity(path string, query url.Values) (string, string) {
	var field, id string
	for key, values := range query {
		if strings.HasSuffix(key, "Id") && len(values) == 1 && values[0] != "" {
			if id != "" {
				return "", ""
			}
			field, id = key, values[0]
		}
	}
	if path == "/organization.one" {
		field = "id"
	}
	return field, id
}

// The eight resource kinds used by this infrastructure need complete managed
// fields, not merely an object carrying an id. JSON null remains valid where
// the API explicitly models a nullable field. Unknown extra fields are allowed.
func managedReadFields(path string) (any, []string) {
	switch path {
	case "/project.one":
		return Project{}, nil
	case "/environment.one":
		return Environment{}, nil
	case "/destination.one":
		return Destination{}, nil
	case "/domain.one":
		return Domain{}, nil
	case "/server.one":
		return Server{}, nil
	case "/sshKey.one":
		return SSHKey{}, []string{"organizationId", "lastUsedAt"}
	case "/volumeBackups.one":
		return VolumeBackup{}, []string{"applicationId", "composeId", "postgresId", "mysqlId", "mariadbId", "mongoId", "redisId", "libsqlId"}
	case "/compose.one":
		return Compose{}, []string{"domains", "mounts", "isolatedDeployment", "isolatedDeploymentsVolume", "repository", "owner", "branch", "githubId", "customGitUrl", "customGitBranch", "customGitSSHKeyId", "gitlabId", "gitlabProjectId", "gitlabRepository", "gitlabOwner", "gitlabBranch", "gitlabPathNamespace", "bitbucketId", "bitbucketRepository", "bitbucketOwner", "bitbucketBranch", "bitbucketRepositorySlug", "giteaId", "giteaRepository", "giteaOwner", "giteaBranch"}
	}
	return nil, nil
}
func stringField(object map[string]json.RawMessage, key string) string {
	var v string
	_ = json.Unmarshal(object[key], &v)
	return v
}
func responseProblem(method, path, reason string) error {
	return fmt.Errorf("%s %s: invalid API response (%s); prior state must be retained", method, path, reason)
}
func validateResponse(method, path string, query url.Values, raw []byte) error {
	if strictJSON(raw) != nil {
		return responseProblem(method, path, "malformed or ambiguous JSON")
	}
	if method != http.MethodGet || !strings.HasSuffix(path, ".one") {
		return nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return responseProblem(method, path, "expected resource object")
	}
	field, id := lookupIdentity(path, query)
	if field == "" || id == "" || stringField(object, field) != id {
		return responseProblem(method, path, "resource identity mismatch")
	}
	dto, ignore := managedReadFields(path)
	if dto == nil {
		return nil
	}
	skipped := map[string]bool{}
	for _, key := range ignore {
		skipped[key] = true
	}
	typ := reflect.TypeOf(dto)
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		key := strings.Split(f.Tag.Get("json"), ",")[0]
		if key == "" || key == "-" || skipped[key] {
			continue
		}
		value, ok := object[key]
		if !ok {
			return responseProblem(method, path, "missing managed field "+key)
		}
		// Environment.description and env are documented nullable columns
		// represented by plain strings; null intentionally decodes as empty.
		nullable := f.Type.Kind() == reflect.Pointer || f.Type.Kind() == reflect.Slice || key == "description" || (path == "/environment.one" && key == "env") || (path == "/server.one" && key == "sshKeyId")
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) && !nullable {
			return responseProblem(method, path, "null managed field "+key)
		}
	}
	required := []string{}
	switch path {
	case "/project.one":
		required = []string{"name", "createdAt"}
	case "/environment.one":
		required = []string{"name", "projectId"}
	case "/destination.one":
		required = []string{"name", "provider", "createdAt"}
	case "/domain.one":
		required = []string{"host", "path", "internalPath", "certificateType", "domainType", "createdAt"}
	case "/server.one":
		required = []string{"name", "ipAddress", "username", "appName", "serverType", "createdAt"}
	case "/sshKey.one":
		required = []string{"name", "publicKey", "privateKey", "createdAt"}
	case "/compose.one":
		required = []string{"name", "appName", "environmentId", "composeType", "composePath", "sourceType", "composeStatus", "createdAt"}
		switch stringField(object, "composeType") {
		case "docker-compose", "stack":
		default:
			return responseProblem(method, path, "unknown compose type")
		}
		switch stringField(object, "sourceType") {
		case "raw":
		case "github":
			required = append(required, "repository", "owner", "branch", "githubId")
		case "git":
			required = append(required, "customGitUrl", "customGitBranch")
			if _, ok := object["customGitSSHKeyId"]; !ok {
				return responseProblem(method, path, "missing git key field")
			}
		case "gitlab":
			required = append(required, "gitlabId", "gitlabOwner", "gitlabRepository", "gitlabBranch", "gitlabPathNamespace")
			if _, ok := object["gitlabProjectId"]; !ok {
				return responseProblem(method, path, "missing gitlab project")
			}
		case "bitbucket":
			required = append(required, "bitbucketId", "bitbucketOwner", "bitbucketRepository", "bitbucketBranch", "bitbucketRepositorySlug")
		case "gitea":
			required = append(required, "giteaId", "giteaOwner", "giteaRepository", "giteaBranch")
		default:
			return responseProblem(method, path, "unknown source type")
		}
	case "/volumeBackups.one":
		required = []string{"name", "volumeName", "destinationId", "serviceType", "appName", "createdAt"}
		kind := stringField(object, "serviceType")
		ok := false
		for _, k := range VolumeBackupServiceTypes {
			if kind == k {
				ok = true
			}
		}
		if !ok {
			return responseProblem(method, path, "unknown backup parent type")
		}
		required = append(required, kind+"Id")
	}
	for _, key := range required {
		if strings.TrimSpace(stringField(object, key)) == "" {
			return responseProblem(method, path, "missing or empty field "+key)
		}
	}
	if path == "/domain.one" {
		kind := stringField(object, "domainType")
		if kind != "application" && kind != "compose" {
			return responseProblem(method, path, "unknown domain parent type")
		}
		if stringField(object, kind+"Id") == "" {
			return responseProblem(method, path, "missing domain parent")
		}
	}
	if path == "/project.one" {
		var tags []map[string]json.RawMessage
		if json.Unmarshal(object["projectTags"], &tags) != nil {
			return responseProblem(method, path, "invalid project tags")
		}
		for _, tag := range tags {
			if stringField(tag, "tagId") == "" {
				return responseProblem(method, path, "missing tag identity")
			}
			if parent := stringField(tag, "projectId"); parent != "" && parent != id {
				return responseProblem(method, path, "tag parent mismatch")
			}
		}
		var environments []map[string]json.RawMessage
		if json.Unmarshal(object["environments"], &environments) != nil || len(environments) == 0 {
			return responseProblem(method, path, "missing project environments")
		}
		defaults := 0
		for _, environment := range environments {
			if stringField(environment, "environmentId") == "" || stringField(environment, "name") == "" {
				return responseProblem(method, path, "invalid environment identity")
			}
			if parent := stringField(environment, "projectId"); parent != "" && parent != id {
				return responseProblem(method, path, "environment parent mismatch")
			}
			var isDefault bool
			if v, ok := environment["isDefault"]; !ok || bytes.Equal(v, []byte("null")) || json.Unmarshal(v, &isDefault) != nil {
				return responseProblem(method, path, "missing default environment flag")
			}
			if isDefault {
				defaults++
			}
		}
		if defaults != 1 {
			return responseProblem(method, path, "ambiguous default environment")
		}
	}
	if path == "/compose.one" {
		var networks []map[string]json.RawMessage
		if json.Unmarshal(object["serviceNetworks"], &networks) != nil {
			return responseProblem(method, path, "invalid service networks")
		}
		for _, network := range networks {
			var ids []string
			var detach bool
			if stringField(network, "serviceName") == "" || len(network["networkIds"]) == 0 || json.Unmarshal(network["networkIds"], &ids) != nil || len(network["detachDokployNetwork"]) == 0 || bytes.Equal(bytes.TrimSpace(network["detachDokployNetwork"]), []byte("null")) || json.Unmarshal(network["detachDokployNetwork"], &detach) != nil {
				return responseProblem(method, path, "incomplete service network")
			}
			for _, networkID := range ids {
				if networkID == "" {
					return responseProblem(method, path, "missing network identity")
				}
			}
		}
	}
	return nil
}
