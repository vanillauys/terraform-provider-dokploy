---
page_title: "Deploy semantics"
subcategory: ""
description: |-
  How deploy_on_change and deployment_timeout work, what starts a deploy, and how a deploy fails.
---

# Deploy semantics

Each service resource in this provider carries two attributes that exist only
in Terraform, never on the Dokploy server. The service resources are the
applications, the compose services, and the six database engines.

- `deploy_on_change` (boolean, default `true`): deploy after a create, and
  after a change to an attribute that starts a deploy.
- `deployment_timeout` (string, default `"15m"`): the maximum wait for a
  deploy to reach a terminal status, as a Go duration string.

Because these attributes are provider-only, `terraform import` cannot recover
them. See [Adopt an existing Dokploy server](adopting-an-existing-instance).

## What happens on a deploy

The provider starts the deploy, then polls until the deploy reaches a terminal
status, `deployment_timeout` expires, or Terraform cancels the operation.

- **A failed deploy fails the apply.** The diagnostic shows the Dokploy status
  and the deployment identifier.
- **A timeout also fails the apply**, but the deploy continues on the server.
  Terraform stops the wait. Dokploy does not stop the work.

An apply that appears to hang is almost always a deploy in progress.

## Database deploys fail when the container never starts

Since Dokploy v0.30.5, a database deploy waits until the container of the
service reaches the `running` state. The wait stops after 45 seconds. If the
container exits or restarts in that time, the deploy call fails with a
`did not converge` error. The service status becomes `error`, and the apply
fails. Before v0.30.5, the same deploy reported success as soon as the service
existed. This applies to all six database engines. Application and compose
deploys are not affected.

## Disable the deploy

Set `deploy_on_change = false` to store a configuration without a deploy. The
provider writes the change to Dokploy. The change takes effect on the next
deploy that you start by other means.

This matters most for credentials.

## Credential changes take effect on the next deploy

**Dokploy does not apply a database password on write.** The provider stores
`database_password`, and the `database_root_password` of MySQL and MariaDB,
when you apply. The new password takes effect only when the service deploys
again. With `deploy_on_change` at its default of `true`, this is invisible,
because the change starts the deploy that applies it. With
`deploy_on_change = false`, the provider stores the password change, and the
change waits for a manual deploy.

If you leave the root password of MySQL or MariaDB unset, the server generates
one.

## Two engines whose default image does not exist

The server-side default `docker_image` for MariaDB and MongoDB does not exist
on Docker Hub. The defaults are `mariadb:6` and `mongo:15`, and they are
unchanged as of Dokploy v0.30.5.

If you leave `docker_image` unset on [`dokploy_mariadb`](../resources/mariadb)
or [`dokploy_mongo`](../resources/mongo), each deploy fails with a Docker
manifest-unknown error. An `external_port` change or an explicit deploy starts
such a deploy. Set an explicit tag that exists:

```hcl
resource "dokploy_mariadb" "db" {
  name              = "app-db"
  environment_id    = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]
  database_name     = "app"
  database_user     = "app"
  database_password = var.db_password
  docker_image      = "mariadb:11.4"
}
```

For `dokploy_mongo`, `mongo:7` is a valid equivalent. The other four engines
have usable server defaults. Pin an explicit tag for all six engines.

## Compose: `command` replaces the deploy command

`dokploy_compose.command` does not extend the deploy command of Dokploy. It
**replaces** it. Dokploy runs your command instead of `docker compose up`. If
the command does not deploy the stack itself, each deploy fails.

Leave `command` unset unless you must replace the deploy command.

## What this provider does not expose

Imperative operations are not Terraform resources. `application.deploy`,
`compose.deploy`, `rollback`, `compose.import`, `compose.randomizeCompose`, and
the `manualBackup*` family are available through the Dokploy API and the UI.
The provider does not model them. A Terraform resource describes a desired
state, and a deploy is an event.

The deploy that this provider does start is a consequence of a state change.
That is why `deploy_on_change` controls it, not a resource of its own.

Since Dokploy v0.30.5, `compose.deploy` also accepts a `freshVolumes` flag,
which removes the volumes of the stack before the deploy. That is a
destructive one-shot action, and the provider never sends it.
