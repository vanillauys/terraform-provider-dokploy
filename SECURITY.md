# Security policy

## Report a vulnerability

Report a vulnerability through
[GitHub private vulnerability reporting](https://github.com/vanillauys/terraform-provider-dokploy/security/advisories/new).
Do not open a public issue for it. You get a first reply within seven days.

## Scope

The provider sends API keys, database passwords, registry passwords, SSH
private keys, and notification tokens to a Dokploy server over HTTPS. In
scope:

- The provider binary and its HTTP client (`internal/`).
- The release pipeline: the GoReleaser build, the GPG signature, and the
  pinned actions in `.github/workflows/`.
- The generated documentation, if it tells a user to do something unsafe.

Out of scope:

- Dokploy itself. Report a Dokploy problem to
  [Dokploy](https://github.com/Dokploy/dokploy/security).
- The Terraform state. The state holds attribute values in cleartext by
  design. The [Secrets guide](docs/guides/secrets.md) explains the
  write-only companions that keep a secret out of the state.
- Terraform and OpenTofu.

## Supported versions

The latest minor release gets fixes. Move to it before you report.

## What each release carries

GoReleaser builds each release in GitHub Actions and signs the checksum
file with the GPG key `750EE4482941313E`, the key that the Terraform
registry verifies. Every action in the workflows is pinned to a commit SHA.
`govulncheck` runs on every pull request and fails it on a reachable
vulnerability.
