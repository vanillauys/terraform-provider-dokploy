---
page_title: "Dokploy Provider"
description: |-
  Manage Dokploy projects, environments, applications, compose stacks, databases, domains, backups, schedules, networks, remote servers, SSH keys, registries, certificates, git providers, notifications, vault providers, users, and API keys with Terraform.
---

# Dokploy Provider

Manage [Dokploy](https://dokploy.com) resources with Terraform:

- **Projects and environments**, with shared environment variables
- **Applications** from a GitHub App, a GitLab project, a Bitbucket repository, a Gitea repository, a plain git repository, or a Docker image
- **Compose services**: `docker-compose` projects and Docker Swarm stacks, from the same sources or an inline compose file
- **Databases**: PostgreSQL, MySQL, MariaDB, MongoDB, Redis, and LibSQL
- **Routing**: domains, published ports, Traefik redirects, HTTP basic auth, and TLS certificates
- **Storage**: bind, volume, and file mounts on any service
- **Backups**: S3-compatible destinations, scheduled database dumps, and Docker volume archives
- **Schedules**: cron jobs on a service, a remote server, or the Dokploy host
- **Networks**: Docker bridge and overlay networks, attached to any service
- **Servers**: remote worker and build servers, and the SSH keys that reach them
- **Git providers**: GitLab, Bitbucket, and Gitea connections, and lookups of GitHub Apps
- **Registries**: container registry logins for private images and built images
- **Notifications**: Slack, Discord, Telegram, email, Resend, Gotify, ntfy, Mattermost, Lark, Microsoft Teams, Pushover, and custom webhooks
- **Vault providers**: HashiCorp Vault or OpenBao, Infisical, AWS Secrets Manager, Doppler, Azure Key Vault, and Scaleway connections for runtime secrets
- **Organization and access**: the organization record, users with an initial password, per-user permissions, and API keys
- **AI settings**: OpenAI-compatible endpoints for Dokploy's AI features

Each service resource deploys on change. Each resource supports
`terraform import`, except `dokploy_api_key`, whose key Dokploy returns only
once.

The provider works with Terraform 1.5 or later and with OpenTofu. The
write-only companions of the secret attributes (`<name>_wo`) need Terraform
1.11 or later; a configuration without them works on 1.5. The provider targets
**Dokploy v0.30.5**. The acceptance suite tests the latest Dokploy release on
every pull request and every night. The suite does not test older releases. A
server older than the pinned version can reject a field that a newer Dokploy
introduced. If your server is older, upgrade it to the pinned version or later
before you apply.

## Stability

The provider follows [semantic versioning](https://semver.org) from v1.0.0:

- A minor release adds resources, data sources, and attributes. A
  configuration and a state from the previous minor release load with an
  empty plan.
- A change that removes or renames an attribute, changes a default, or
  changes what an existing attribute does is a breaking change. It needs a
  major release, and the [upgrade guide](guides/upgrading) shows the old and
  the new configuration.
- A deprecated attribute stays for at least one minor release and prints a
  warning at plan time before a major release removes it.
- The Dokploy compatibility pin moves in minor releases.

Pin the minor version, `~> 1.0`, to get fixes and additions without a
breaking change.

## Guides

- [Get started](guides/getting-started): configure the provider and apply a
  first project, database, application, and domain.
- [Usage examples](guides/usage-examples): short, complete configurations
  for the common setups: an app from GitLab with Slack alerts, a worker
  server, private images, a teammate with limited access, nightly backups,
  and variables as a map.
- [Adopt an existing Dokploy server](guides/adopting-an-existing-instance):
  import a running server into the state without a rebuild.
- [Deploy semantics](guides/deploy-semantics): `deploy_on_change`,
  `deployment_timeout`, and deploy failures.
- [Secrets and sensitive values](guides/secrets): environment variables,
  database passwords, backup credentials, and the write-only companions
  that keep a secret out of the state.
- [Upgrade guide](guides/upgrading): what each release needs from your
  configuration. v1.0 has no change; v0.13 adds the coverage of this page;
  v0.12 adds the write-only companions; v0.11 had the breaking changes.

## Before you start

These three problems cause the most failures:

1. **Dokploy rate-limits API keys, and an exhausted key returns `401`, not
   `429`.** A large apply can fail with an authentication error on a key that
   works for single requests. See
   [Get started](guides/getting-started#before-your-first-apply-api-key-rate-limits).
2. **`dokploy_application` and `dokploy_compose` own the whole service.** An
   apply of either resource replaces each setting that changed in the Dokploy
   UI. Manage a service in Terraform or in the UI, not in both. See
   [Adopt an existing Dokploy server](guides/adopting-an-existing-instance#decide-what-terraform-owns).
3. **The default `docker_image` for MariaDB and MongoDB does not exist on
   Docker Hub.** Set an explicit tag, or each deploy fails. See
   [Deploy semantics](guides/deploy-semantics#two-engines-whose-default-image-does-not-exist).

## Example Usage

```terraform
provider "dokploy" {
  endpoint = "https://dokploy.example.com"
  # The provider reads api_key from the DOKPLOY_API_KEY environment variable.
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `api_key` (String, Sensitive) Dokploy API key. The provider sends it as the `x-api-key` header. If unset, the provider reads the `DOKPLOY_API_KEY` environment variable.
- `endpoint` (String) Base URL of the Dokploy server, for example `https://dokploy.example.com`. If unset, the provider reads the `DOKPLOY_ENDPOINT` environment variable.
- `insecure` (Boolean) Skip the TLS certificate verification, for self-signed endpoints. Defaults to `false`.


