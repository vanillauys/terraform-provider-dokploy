---
page_title: "Secrets and sensitive values"
subcategory: ""
description: |-
  How this provider handles environment variables, database passwords, and backup credentials.
---

# Secrets and sensitive values

## `env` is not sensitive, by design

Dokploy stores the environment of a service as one multiline `KEY=value`
string. The provider exposes it as a plain string:

```hcl
resource "dokploy_application" "web" {
  name           = "web"
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]

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
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]

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

`database_password` is `Required` and `Sensitive` on all six engines. See
[`dokploy_postgres`](../resources/postgres#schema) for the attribute in
context. `dokploy_mysql`, `dokploy_mariadb`, `dokploy_mongo`, `dokploy_redis`,
and `dokploy_libsql` declare it in the same way. The server requires a
password from the caller and never generates one.

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

## Backup destination credentials

`dokploy_destination` carries `access_key` and `secret_access_key`, because
the user who creates the record must supply them.

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
It matters more here than for a provider with write-only secrets: the Dokploy
API returns database passwords and destination credentials, so the state holds
the real values, not one-way hashes.

Use a remote state with encryption at rest, and restrict who can read it.

The same applies to `dogfood/dry-run.sh`. It writes a throwaway
`terraform.tfstate` into `dogfood/scratch/` and keeps it on failure for
inspection. That directory is gitignored, but it is real state on disk.
