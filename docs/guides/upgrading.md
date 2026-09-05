---
page_title: "Upgrade guide"
subcategory: ""
description: |-
  What each release since v0.11.0 needs from your configuration: the new resources of v0.13, the write-only companions of v0.12, and the breaking changes of v0.11.
---

# Upgrade guide

## Upgrade to v0.13

v0.13.0 has no breaking change. Every attribute from v0.12.0 keeps its shape,
and the existing state loads with an empty plan.

1. Update the version constraint to `~> 0.13`.
2. Run `terraform init -upgrade`.
3. Run `terraform plan`. The plan must be empty.

v0.13.0 adds resources; nothing that exists changes. Two additions touch a
resource you may already use:

- `dokploy_application` and `dokploy_compose` accept three more source
  blocks: `gitlab`, `bitbucket`, and `gitea`. A configuration with the
  `github`, `git`, `docker`, or `raw` block keeps its behavior.
- `dokploy_environment_variables` writes the `env` text of an application, a
  compose, or an environment from a map. If you adopt it for a target that
  already sets `env`, remove `env` from the target and add
  `lifecycle { ignore_changes = [env] }` to it. The [Usage examples](usage-examples#environment-variables-as-a-map)
  guide shows the shape.

Every new secret attribute has a write-only companion, listed in the
[Secrets guide](secrets#write-only-companions).

## Upgrade to v0.12

v0.12.0 has no breaking change. Every attribute from v0.11.0 keeps its shape,
and the existing state loads with an empty plan.

1. Update the version constraint to `~> 0.12`.
2. Run `terraform init -upgrade`.
3. Run `terraform plan`. The plan must be empty.

v0.12.0 adds a write-only companion to each secret attribute. The move to a
companion is optional. It is an in-place update, and it removes the secret
from the state. It needs Terraform 1.11 or later.

Old:

```hcl
resource "dokploy_postgres" "db" {
  name              = "db"
  environment_id    = dokploy_project.example.production_environment_id
  database_name     = "app"
  database_user     = "app"
  database_password = var.db_password
}
```

New:

```hcl
resource "dokploy_postgres" "db" {
  name                         = "db"
  environment_id               = dokploy_project.example.production_environment_id
  database_name                = "app"
  database_user                = "app"
  database_password_wo         = var.db_password
  database_password_wo_version = 1
}
```

The apply sends the value once and redeploys. After it, the state holds null
for `database_password`, and `terraform show` prints no password. To rotate
the password later, change the value and increase the version. See
[Secrets and sensitive values](secrets#write-only-companions) for the full
list of companions and the rules.

## Upgrade to v0.11

v0.11.0 is the last minor release with breaking changes before v1.0.0. It
renames one attribute, removes three, and adds one. The existing state loads
after the upgrade: `dokploy_backup` and `dokploy_compose` carry a state
upgrader. Your configuration needs the edits on this page.

## Before you upgrade

1. Run `terraform plan` with your current provider version. Make sure the
   plan is empty.
2. Update the version constraint:

   ```hcl
   terraform {
     required_providers {
       dokploy = {
         source  = "vanillauys/dokploy"
         version = "~> 0.11"
       }
     }
   }
   ```

3. Run `terraform init -upgrade`.
4. Apply the configuration edits below.
5. Run `terraform plan`. The plan must be empty.

## `dokploy_backup`: `schedule` is now `cron_expression`

The attribute takes the name that `dokploy_schedule` and
`dokploy_volume_backup` already use. The Dokploy API field is unchanged.

Old:

```hcl
resource "dokploy_backup" "db" {
  service_id     = dokploy_postgres.db.id
  service_type   = "postgres"
  database       = "app"
  prefix         = "backups/app/"
  schedule       = "0 3 * * *"
  destination_id = dokploy_destination.backups.id
}
```

New:

```hcl
resource "dokploy_backup" "db" {
  service_id      = dokploy_postgres.db.id
  service_type    = "postgres"
  database        = "app"
  prefix          = "backups/app/"
  cron_expression = "0 3 * * *"
  destination_id  = dokploy_destination.backups.id
}
```

What you must do: rename the attribute in each `dokploy_backup` block. A
configuration that still sets `schedule` fails at plan time with an
unsupported-argument error. The state upgrader moves the stored value to the
new name, so the plan after the rename is empty.

## `dokploy_libsql` data source: `database_password` is removed

The five other engine data sources never exposed the password. One
convention now covers all six. The `dokploy_libsql` resource still has the
attribute.

Old:

```hcl
data "dokploy_libsql" "db" {
  id = var.libsql_id
}

output "db_password" {
  value     = data.dokploy_libsql.db.database_password
  sensitive = true
}
```

New: read the password from the resource that manages the service, or store
it as an input variable. A configuration that references the attribute fails
at plan time with an unsupported-attribute error. A data source holds no
long-lived state, so no state upgrade is involved.

## `dokploy_compose`: `isolated_deployment` and `isolated_deployments_volume` are removed

Dokploy deprecated Isolated Deployment in v0.30.0. `service_networks`
replaces it: attach each compose service to the Docker networks it needs.

Old:

```hcl
resource "dokploy_compose" "stack" {
  name                        = "stack"
  environment_id              = dokploy_project.example.production_environment_id
  isolated_deployment         = true
  isolated_deployments_volume = true

  raw = {
    compose_file = file("${path.module}/docker-compose.yml")
  }
}
```

New:

```hcl
resource "dokploy_compose" "stack" {
  name           = "stack"
  environment_id = dokploy_project.example.production_environment_id

  raw = {
    compose_file = file("${path.module}/docker-compose.yml")
  }

  service_networks = [
    { service_name = "web", network_ids = [dokploy_network.internal.id] },
  ]
}
```

What you must do: remove the two attributes from each `dokploy_compose`
block. The state upgrader drops them from the stored state, so the plan
after the edit is empty. The provider no longer sends the two fields to
Dokploy, and Dokploy keeps the stored values. A stack that had Isolated
Deployment on keeps it on until you turn it off in the Dokploy UI.

## `dokploy_project`: use `production_environment_id`

This change is not breaking, but make it in the same upgrade. The examples
and guides used this expression to find the default environment:

```hcl
environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]
```

That expression forces a replacement of the service on every project update.
`environments` is a computed list, so a change to the project description
marks it unknown in the plan. The `for` expression becomes unknown too, and
an unknown `environment_id` forces a replacement of the service. A live
check on Dokploy v0.30.5 planned a destroy and re-create of an application
after a description change.

New:

```hcl
environment_id = dokploy_project.example.production_environment_id
```

The new attribute selects the environment with the Dokploy `isDefault`
flag, not with the name, and it keeps its value in the plan of a project
update. The value is the same id, so the plan after the edit is empty.
Make this edit before the next change to a `dokploy_project` resource.

## After the upgrade

Run `terraform plan` once more. If the plan shows a replacement, stop and
compare the `environment_id` of the affected service with this page. If the
plan shows an unsupported argument or attribute, one of the edits above is
still missing.
