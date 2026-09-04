---
page_title: "Upgrade to v0.11"
subcategory: ""
description: |-
  The breaking changes in v0.11.0, the old and the new configuration for each, and what you must do.
---

# Upgrade to v0.11

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
