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

import (
	"errors"
	"fmt"
)

// ErrNoMatch and ErrMultipleMatches are Find's two failure sentinels.
// Callers wrap them with errors.Is (never string-matching) to fill in
// their own wording — see ByName below for the common case, and
// internal/datasources/environment's FindByName for a call site that needs
// a different return shape and message entirely (wave-2 task 9 carry item
// C2: before Find existed, that lookup was a third character-for-character
// copy of this same zero/multiple-match loop, one ByName's own string
// return type could not absorb).
var (
	ErrNoMatch         = errors.New("no match")
	ErrMultipleMatches = errors.New("multiple matches")
)

// Find returns the single item in items for which match reports true. It
// returns ErrNoMatch if none do, and ErrMultipleMatches if more than one
// does — it never silently takes the first, using a nil-pointer found-
// sentinel: with a plain T there is no zero value guaranteed to be
// unreachable, so a *T is what actually distinguishes "nothing matched
// yet" from "matched a zero-value item". Dokploy does not enforce unique
// resource names within an environment (or a project, for environments
// themselves), so a name lookup must refuse an ambiguous match rather than
// picking one.
func Find[T any](items []T, match func(T) bool) (T, error) {
	var zero T
	var found *T
	for i := range items {
		if !match(items[i]) {
			continue
		}
		if found != nil {
			return zero, ErrMultipleMatches
		}
		found = &items[i]
	}
	if found == nil {
		return zero, ErrNoMatch
	}
	return *found, nil
}

// ByName resolves an exact name to its id within items, using idOf/nameOf
// to read each item's id and name. kind names the resource kind in error
// messages (e.g. "postgres", "application").
func ByName[T any](items []T, name, kind string, idOf, nameOf func(T) string) (string, error) {
	item, err := Find(items, func(t T) bool { return nameOf(t) == name })
	switch {
	case errors.Is(err, ErrMultipleMatches):
		return "", fmt.Errorf("multiple %s services named %q in this environment; look it up by id instead", kind, name)
	case errors.Is(err, ErrNoMatch):
		return "", fmt.Errorf("no %s service named %q in this environment", kind, name)
	case err != nil:
		return "", err
	}
	return idOf(item), nil
}
