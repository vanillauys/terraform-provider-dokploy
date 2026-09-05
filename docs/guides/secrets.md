---
page_title: "Secrets and sensitive values"
subcategory: ""
description: |-
  How this provider handles environment variables, database passwords, and backup credentials, and how the write-only companions keep a secret out of the state.
---

# Secrets and sensitive values

## `env` is not sensitive, by design

Dokploy stores the environment of a service as one multiline `KEY=value`
string. The provider exposes it as a plain string:

```hcl
resource "dokploy_application" "web" {
  name           = "web"
  environment_id = dokploy_project.example.production_environment_id

  docker = {
    image = "traefik/whoami:v1.10"
  }

  env = <<-EOT
    LOG_LEVEL=info
    PORT=80
  EOT
}
```

The provider deliberately does **not** mark `env` as sensitive. Most of the
content of `env` is ordinary configuration. A sensitive `env` would redact the
whole string from each plan, and the diffs would become unreadable for the
common case.

Mark the values that are secret at the point where they enter the
configuration:

```hcl
variable "api_token" {
  type      = string
  sensitive = true
}

resource "dokploy_application" "web" {
  name           = "web"
  environment_id = dokploy_project.example.production_environment_id

  docker = {
    image = "traefik/whoami:v1.10"
  }

  env = <<-EOT
    LOG_LEVEL=info
    API_TOKEN=${var.api_token}
  EOT
}
```

Terraform propagates sensitivity through interpolation. When you interpolate a
sensitive variable into `env`, the plan output redacts the whole `env` value.

One behavior to know: `dokploy_application` writes `env` and
[`build_secrets`](../resources/application#schema) in the same request. A
configuration that sets `env` but omits `build_secrets` clears the build
secrets in Dokploy. If you need build secrets, set `build_secrets` explicitly.

## Database passwords

`database_password` is `Sensitive` on all six engines. See
[`dokploy_postgres`](../resources/postgres#schema) for the attribute in
context. `dokploy_mysql`, `dokploy_mariadb`, `dokploy_mongo`, `dokploy_redis`,
and `dokploy_libsql` declare it in the same way. The server requires a
password from the caller and never generates one. Set `database_password`, or
set its write-only companion `database_password_wo` (see
[Write-only companions](#write-only-companions) below).

Two consequences:

- A password change takes effect on the **next deploy**, not on write. See
  [Deploy semantics](deploy-semantics).
- `terraform plan -generate-config-out` cannot produce a working configuration
  for a stack with a database engine. Terraform refuses to write a value for a
  sensitive attribute, and it then rejects the `null` on a required attribute.
  See [Adopt an existing Dokploy server](adopting-an-existing-instance) for
  the workaround.

The `database_root_password` of MySQL and MariaDB is also sensitive. It is
`Optional` and `Computed`, so it does not cause the same generation failure.
It has a write-only companion too, `database_root_password_wo`.

## Write-only companions

Terraform 1.11 added write-only arguments: values that a provider sends to the
server and that Terraform keeps out of the plan and the state. Every secret
attribute of this provider has one, as a pair next to the plain attribute:

- `<name>_wo`, the write-only form, sensitive.
- `<name>_wo_version`, a number that gates a new value.

The pairs:

| Resource | Plain attribute | Companion |
|----------|-----------------|-----------|
| The six database engines | `database_password` | `database_password_wo` |
| `dokploy_mysql`, `dokploy_mariadb` | `database_root_password` | `database_root_password_wo` |
| `dokploy_destination` | `access_key`, `secret_access_key` | `access_key_wo`, `secret_access_key_wo` |
| `dokploy_security` | `password` | `password_wo` |
| `dokploy_vault_provider` | the secret of each config block, for example `hashicorp.token` | `hashicorp.token_wo`, and so on |

Set the plain attribute or its companion, not both. A validator rejects a
configuration that sets both, and one that sets neither where the server
needs a value.

```hcl
variable "db_password" {
  type      = string
  sensitive = true
}

resource "dokploy_postgres" "db" {
  name                         = "db"
  environment_id               = dokploy_project.example.production_environment_id
  database_name                = "app"
  database_user                = "app"
  database_password_wo         = var.db_password
  database_password_wo_version = 1
}
```

The rules:

- The value reaches the server on create, and when `<name>_wo_version`
  changes. A new value with the same version does not reach the server, and
  the plan is empty. To rotate a secret, change the value and increase the
  version.
- The state holds null for the plain attribute and for the companion. The
  Dokploy API returns most secrets on read, so the provider marks the secret
  in the private state of the resource and keeps the server's value out of
  the state on every refresh. `terraform show` prints no secret.
- The move between the two shapes is an in-place update. To go back to the
  plain attribute, remove the pair and set the plain attribute; the state
  holds the secret again.
- A `Computed` secret (`database_root_password`) can also drop both: the
  server keeps its value, and the state takes it, as before the companions.
- `dokploy_vault_provider` is the exception on the first rule. The Dokploy
  API masks each vault secret on read, so the provider cannot resend a stored
  value; it sends the companion's value on every update. The version then
  only starts an update when nothing else changed.
- `terraform import` has no companion flag. An imported resource holds the
  secret in its state until the first apply with the companion, which sends
  the value and removes it.
- A write-only value needs Terraform 1.11 or later. An older CLI rejects it at
  validation with a message that names the version. A configuration without
  the companions works on Terraform 1.5 as before.

## Backup destination credentials

`dokploy_destination` carries `access_key` and `secret_access_key`, because
the user who creates the record must supply them. Both have a write-only
companion, `access_key_wo` and `secret_access_key_wo`.

The [**data source**](../data-sources/destination) omits both on purpose. The
Dokploy endpoint `destination.one` returns them in cleartext. A data source
exists for references, and a copy of the credentials of a shared backup target
in the state of each consumer widens their exposure with no gain. A consumer
needs only the id:

```hcl
data "dokploy_destination" "backups" {
  name = "s3-backups"
}

resource "dokploy_backup" "db" {
  destination_id  = data.dokploy_destination.backups.id
  service_id      = dokploy_postgres.db.id
  service_type    = "postgres"
  cron_expression = "0 3 * * *"
  database        = "app"
  prefix          = "backups/app/"
}
```

All six [`dokploy_backup`](../resources/backup#schema) attributes in this
example are required, `prefix` included.

## The state holds these values in cleartext

The Terraform state stores attribute values in cleartext, sensitive values
included. That is the design of Terraform, not a property of this provider.
The Dokploy API returns database passwords and destination credentials, so a
plain secret attribute holds the real value in the state, not a one-way hash.
The [write-only companions](#write-only-companions) are the way out: with
them, the state holds null for the secret.

Use a remote state with encryption at rest, and restrict who can read it,
with or without the companions: `env` and the non-secret attributes are still
there.

The same applies to `dogfood/dry-run.sh`. It writes a throwaway
`terraform.tfstate` into `dogfood/scratch/` and keeps it on failure for
inspection. That directory is gitignored, but it is real state on disk.
