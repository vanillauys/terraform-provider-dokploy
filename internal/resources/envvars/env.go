// Package envvars holds the dokploy_environment_variables resource: a map
// that owns the `env` text of an application, a compose, or an environment.
package envvars

import (
	"fmt"
	"sort"
	"strings"
)

// parseEnv reads Dokploy's multiline KEY=value text into a map. Blank lines
// and `#` comments are skipped; a line without `=` is an error, because the
// resource could not write it back.
func parseEnv(text string) (map[string]string, error) {
	vars := map[string]string{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("line %d is not KEY=value: %q", i+1, line)
		}
		vars[strings.TrimSpace(key)] = value
	}
	return vars, nil
}

// renderEnv writes the map as KEY=value lines in key order, so that the
// text is stable across applies.
func renderEnv(vars map[string]string) string {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+"="+vars[k])
	}
	return strings.Join(lines, "\n")
}
