// Package lookup holds tiny generic collection-search helpers shared by
// packages that need to resolve a Dokploy resource by name.
//
// It exists specifically so internal/client and internal/datasources/
// database can share one implementation of "find the one item named X,
// erroring on zero or multiple matches" without either importing the
// other. Before this package existed, internal/client's
// FindServiceByName ([]ServiceRef) and internal/datasources/database's
// findByName ([]resourcedb.Object) were character-for-character
// duplicates of the same loop, sentinel, and error strings, differing
// only in element type — a review finding on Task 4 of the wave-2 plan
// (both call sites treat zero/multiple matches as an error and never take
// the first, per the provider's standing "names are not unique" rule).
//
// This package has zero internal-module dependencies by design: no
// *client.Client, no terraform-plugin-framework. That is what makes it a
// valid import for internal/client (which must not depend on
// terraform-plugin-framework) and, separately, for
// internal/datasources/database (which must not depend on internal/client
// beyond what it already gets indirectly through
// internal/resources/database) — both directions stay a leaf dependency,
// so there is no cycle in either direction.
package lookup

import "fmt"

// ByName resolves an exact name to its id within items, using idOf/nameOf
// to read each item's id and name. kind names the resource kind in error
// messages (e.g. "postgres", "application").
//
// It errors on zero AND on multiple matches — never takes the first —
// using a nil-pointer found-sentinel: a plain string compared against ""
// can't distinguish "nothing matched yet" from "matched an item whose id
// happens to be empty". Dokploy does not enforce unique resource names
// within an environment, so a name lookup must refuse an ambiguous match
// rather than silently picking one.
func ByName[T any](items []T, name, kind string, idOf, nameOf func(T) string) (string, error) {
	var found *string
	for i := range items {
		if nameOf(items[i]) != name {
			continue
		}
		if found != nil {
			return "", fmt.Errorf("multiple %s services named %q in this environment; look it up by id instead", kind, name)
		}
		id := idOf(items[i])
		found = &id
	}
	if found == nil {
		return "", fmt.Errorf("no %s service named %q in this environment", kind, name)
	}
	return *found, nil
}
