---
page_title: "Secrets and sensitive values"
subcategory: ""
description: |-
  How this provider handles environment variables, database passwords and backup credentials.
---

# Secrets and sensitive values

## `env` is not marked sensitive, on purpose

Dokploy stores a service's environment as one multiline `KEY=value` blob, and
this provider exposes it as a plain string:

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

It is deliberately **not** force-marked sensitive. Most of what lives in `env`
is ordinary configuration, and marking the whole blob sensitive would redact
all of it from every plan, making diffs unreadable for the majority case in
order to protect the minority.

Mark the values that are actually secret, at the point where they enter the
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

Terraform propagates sensitivity through interpolation, so the whole `env`
value is redacted in plan output once a sensitive variable is interpolated
into it.

One behaviour worth knowing: setting `env` on `dokploy_application` also
clears the application's **build secrets** in Dokploy.
[`build_secrets`](../resources/application#schema) is a
separate schema attribute; set it explicitly if you need it.

## Database passwords

`database_password` is `Required` and `Sensitive` on all five engines - see
[`dokploy_postgres`](../resources/postgres#schema) for the attribute in
context; `dokploy_mysql`, `dokploy_mariadb`, `dokploy_mongo` and
`dokploy_redis` declare it identically. The
server genuinely requires a caller-supplied password and never generates one.

Two consequences:

- Changing it takes effect on the **next deploy**, not on write. See
  [Deploy semantics](deploy-semantics).
- `terraform plan -generate-config-out` cannot produce a working config for a
  stack containing any database engine, because Terraform refuses to write a
  value for a sensitive attribute and then rejects the resulting `null` on a
  required one. See
  [Adopting an existing Dokploy instance](adopting-an-existing-instance) for
  the workaround.

MySQL's and MariaDB's `database_root_password` is also sensitive, but is
`Optional` and `Computed`, so it does not hit the same generation failure.

## Backup destination credentials

`dokploy_destination` carries `access_key` and `secret_access_key`, because
whoever creates the record has to supply them.

The [**data source**](../data-sources/destination) deliberately omits both.
Dokploy's `destination.one`
returns them in cleartext, but a data source exists to be referenced, and
copying a shared backup target's credentials into every consumer's state
widens their blast radius for no gain. Consumers need only the id:

```hcl
data "dokploy_destination" "backups" {
  name = "s3-backups"
}

resource "dokploy_backup" "db" {
  destination_id = data.dokploy_destination.backups.id
  service_id     = dokploy_postgres.db.id
  service_type   = "postgres"
  schedule       = "0 3 * * *"
  database       = "app"
  prefix         = "backups/app/"
}
```

All six of those [`dokploy_backup`](../resources/backup#schema) attributes are
required, including `prefix`.

## State contains these values in cleartext

Terraform state stores attribute values, including sensitive ones, in
cleartext by design. That is not specific to this provider, but it matters
more here than for a provider whose secrets are write-only: database
passwords and destination credentials are readable back from Dokploy's API, so
they are genuinely present in state rather than being one-way hashes.

Use remote state with encryption at rest, and restrict who can read it.

The same applies to `dogfood/dry-run.sh`, which writes a throwaway
`terraform.tfstate` into `dogfood/scratch/` and preserves it on failure for
inspection. That directory is gitignored, but it is real state on disk.
