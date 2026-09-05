// Package appchild holds one generic resource implementation shared by
// dokploy_port, dokploy_redirect and dokploy_security.
//
// Why these three and not mounts. Probed live (v0.29.13, 2026-07-28), the
// three agree on everything the resource layer touches:
//
//   - exactly one parent, always `applicationId`
//   - a flat record with no subtypes
//   - dialect A update: the full field set is required, a body of {id} alone
//     is HTTP 400 naming every missing field, and no field is nullable
//   - the .delete verb (mounts uses .remove)
//
// They diverge only in their response envelopes — port.create returns the
// record while redirects.create and security.create return literal `true`,
// and security.update returns `null` — which is a client-layer concern and
// stays visible there, in each router's own file.
//
// dokploy_mount is deliberately NOT part of this: it has seven parent types,
// three subtypes with different required fields, and a create/update
// asymmetry none of these share. Forcing it in is the mistake
// internal/client/doc.go warns about for the database engines.
//
// The engine is parameterised by the model type rather than by a
// map[string]any bag, so each kind keeps an ordinary tfsdk-tagged struct and
// the framework's own Plan.Get/State.Set do the conversion — no casts, no
// reflection over tags, and a compile error rather than a runtime panic when
// a kind and its model disagree.
package appchild

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// Kind describes one application child resource. Per-kind divergence lives
// here, never in `if kind == ...` branches inside the engine.
type Kind[M any] struct {
	// Name is the resource type suffix: dokploy_<Name>.
	Name string

	// Description is the resource-level schema description.
	Description string

	// Attributes are the kind's own attributes. The engine supplies `id`
	// and `application_id`.
	Attributes map[string]schema.Attribute

	// ID reads the record id out of the model; SetID writes it back. The
	// engine needs both and cannot reach into an arbitrary M itself.
	ID    func(*M) string
	SetID func(*M, string)

	// The four client calls. Create and Read populate the model from the
	// server; Update sends it. None of them touches Terraform types beyond
	// the model itself.
	Create func(context.Context, *client.Client, *M) error
	Read   func(context.Context, *client.Client, *M) error
	Update func(context.Context, *client.Client, *M) error
	Delete func(context.Context, *client.Client, string) error

	// Secrets, when set, names the attributes that carry write-only
	// companions (tfutil.WriteOnlyCompanions). The engine then also reads
	// the config and the prior state, calls ResolveSecrets before Create
	// and Update, records in the private state which secrets the config
	// sets through a companion, and calls HideSecret for each of those
	// after every client call, so the state never holds the secret. Read
	// consults the same private-state flags.
	Secrets []string
	// ResolveSecrets writes into plan the value that the next client call
	// must send for each secret, from the plan, the config (the only
	// carrier of a write-only value) and the prior state (nil on create).
	// It returns, per secret, whether the config uses the companion.
	ResolveSecrets func(ctx context.Context, c *client.Client, plan, cfg, prior *M) (map[string]bool, error)
	// HideSecret nulls one secret in the model.
	HideSecret func(m *M, name string)
}
