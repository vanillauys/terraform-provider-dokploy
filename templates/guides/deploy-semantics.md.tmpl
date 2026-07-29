---
page_title: "Deploy semantics"
subcategory: ""
description: |-
  How deploy_on_change and deployment_timeout work, what triggers a redeploy, and how deploys fail.
---

# Deploy semantics

Every service resource in this provider (applications, compose services and
all five database engines) carries two attributes that exist only in
Terraform, never on the Dokploy server:

- `deploy_on_change` (boolean, default `true`) - deploy after create, and
  after a change to any deploy-triggering attribute.
- `deployment_timeout` (string, default `"15m"`) - how long to wait for a
  triggered deployment to reach a terminal status, as a Go duration string.

Because they are provider-only, `terraform import` cannot recover them. See
[Adopting an existing Dokploy instance](adopting-an-existing-instance).

## What happens on a deploy

The provider triggers the deploy, then polls until the deployment reaches a
terminal status, `deployment_timeout` expires, or the context is cancelled.

- **A failed deployment fails the apply**, with Dokploy's status and the
  deployment identifier in the diagnostic.
- **A timeout also fails the apply**, but the server-side deployment keeps
  running. Terraform has stopped waiting; Dokploy has not stopped working.

An apply that looks hung is almost always a deploy in progress.

## Turning it off

Set `deploy_on_change = false` to store configuration without deploying it.
The change is written to Dokploy, and takes effect on the next deploy you
trigger by some other means.

This matters most for credentials.

## Credential changes take effect on the next deploy

**Database passwords are not applied on write.** `database_password`, and
MySQL's and MariaDB's `database_root_password`, are stored when you apply but
only take effect when the service next deploys. With `deploy_on_change` at its
default of `true` this is invisible, because the change triggers the deploy
that applies it. With `deploy_on_change = false`, a password change is stored
and not applied until a manual deploy.

MySQL's and MariaDB's root password is server-generated when left unset.

## Two engines whose default image does not exist

MariaDB's and MongoDB's server-side default `docker_image` does not exist on
Docker Hub: `mariadb:6` and `mongo:15` as of Dokploy v0.29.13.

Leaving `docker_image` unset on `dokploy_mariadb` or `dokploy_mongo` and then
triggering any deploy - an `external_port` change, or an explicit deploy -
fails with a Docker manifest-unknown error. Set an explicit, real tag:

```hcl
resource "dokploy_mariadb" "db" {
  name              = "app-db"
  environment_id    = dokploy_project.example.environments[0].id
  database_name     = "app"
  database_user     = "app"
  database_password = var.db_password
  docker_image      = "mariadb:11.4"
}
```

`mongo:7` is the equivalent for `dokploy_mongo`. The other three engines have
usable server defaults, but pinning an explicit tag is good practice for all
five.

## Compose: `command` replaces the deploy invocation

`dokploy_compose.command` does not add to Dokploy's deploy command, it
**replaces** it. Dokploy runs your command *instead of* `docker compose up`,
so setting it to anything that does not itself deploy the stack makes every
deploy fail.

Leave it unset unless you specifically need to replace the invocation.

## What this provider deliberately does not expose

Imperative operations are not Terraform resources. `application.deploy`,
`compose.deploy`, `rollback`, `compose.import`, `compose.randomizeCompose` and
the `manualBackup*` family are all reachable through Dokploy's API and the UI,
and none of them is modelled here. A Terraform resource describes desired
state; a deploy is an event.

The deploy this provider does perform is a consequence of a state change, which
is why it is controlled by `deploy_on_change` rather than by a resource of its
own.
