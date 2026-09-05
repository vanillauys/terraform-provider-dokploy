package client

import (
	"context"
	"fmt"
)

// locateCreated is createAndLocate for a record type whose list endpoint
// returns the full records, so that a collision can be resolved by content.
//
// createAndLocate diffs ids around the create and refuses to guess when more
// than one new id appears. That is the right answer for a child collection
// on one parent, where the lock on the parent id already rules out this
// provider as the second writer. Organization-level records (sshKey,
// notification, ai, the git providers) have no such parent: a second
// Terraform process, the Dokploy UI, or two acceptance packages on one rig
// can create a sibling in the same second, and the diff then holds two ids.
// The fields of the request tell them apart. match reports whether a record
// is the one this call created; when several new records match, the call
// still fails, because two identical creates really are indistinguishable.
func locateCreated[T any](
	ctx context.Context,
	scope, kind string,
	list func(context.Context) ([]T, error),
	create func(context.Context) error,
	id func(T) string,
	match func(T) bool,
) (string, error) {
	unlock := lockParent(scope)
	defer unlock()

	before, err := list(ctx)
	if err != nil {
		return "", fmt.Errorf("listing %s before create: %w", kind, err)
	}
	seen := make(map[string]bool, len(before))
	for _, r := range before {
		seen[id(r)] = true
	}

	if err := create(ctx); err != nil {
		return "", err
	}

	after, err := list(ctx)
	if err != nil {
		return "", fmt.Errorf("locating the created %s: %w", kind, err)
	}
	var fresh, matching []string
	for _, r := range after {
		if seen[id(r)] {
			continue
		}
		fresh = append(fresh, id(r))
		if match(r) {
			matching = append(matching, id(r))
		}
	}
	switch {
	case len(fresh) == 1:
		return fresh[0], nil
	case len(fresh) == 0:
		return "", fmt.Errorf(
			"%s.create reported success but no new %s appeared in %s",
			kind, kind, scope)
	case len(matching) == 1:
		return matching[0], nil
	default:
		return "", fmt.Errorf(
			"%d new %s records appeared in %s while creating one, and %d of them match the request, so the "+
				"created id cannot be identified. %s.create does not return the record it made. "+
				"Something outside this apply created an identical %s at the same time",
			len(fresh), kind, scope, len(matching), kind, kind)
	}
}
