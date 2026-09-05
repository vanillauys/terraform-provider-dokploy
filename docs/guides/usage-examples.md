---
page_title: "Usage examples"
subcategory: ""
description: |-
  Short, complete configurations for the common Dokploy setups: an application from GitLab with Slack alerts, a remote worker server, private images from a registry, a teammate with limited access, nightly backups with an email alert, and environment variables as a map.
---

# Usage examples

Each example is a complete configuration on top of the provider block from
[Get started](getting-started). Copy one, replace the values, and apply. The
examples use `production_environment_id`, the environment that Dokploy
creates with each project.

## An application from GitLab, with Slack alerts

The GitLab connection holds the OAuth application that you created in
GitLab. After the first apply, open **Git > GitLab** in Dokploy and authorize
it once. The application then deploys from the project on each push to
`main`, and Slack gets a message for each deploy and each failed build.

```hcl
resource "dokploy_project" "shop" {
  name = "shop"
}

resource "dokploy_gitlab_provider" "main" {
  name              = "my-group"
  application_id    = var.gitlab_oauth_application_id
  secret_wo         = var.gitlab_oauth_secret
  secret_wo_version = 1
  group_name        = "my-group"
}

resource "dokploy_application" "api" {
  name           = "api"
  environment_id = dokploy_project.shop.production_environment_id

  gitlab = {
    gitlab_id      = dokploy_gitlab_provider.main.id
    owner          = "my-group"
    repository     = "api"
    branch         = "main"
    project_id     = 12345678
    path_namespace = "my-group/api"
  }

  build = {
    type = "nixpacks"
  }

  env = <<-EOT
    PORT=3000
  EOT
}

resource "dokploy_domain" "api" {
  application_id = dokploy_application.api.id
  host           = "api.example.com"
  port           = 3000
  https          = true
}

resource "dokploy_slack_notification" "deploys" {
  name                   = "deploys"
  channel                = "#deploys"
  webhook_url_wo         = var.slack_webhook_url
  webhook_url_wo_version = 1

  app_deploy      = true
  app_build_error = true
}
```

## A remote worker server with a database on it

The SSH key pair comes from the `hashicorp/tls` provider. Dokploy stores the
server record; after the first apply, open **Settings > Servers** in Dokploy
and run **Setup Server** once, which installs Docker on the machine. The
database then runs on the worker through `server_id`.

```hcl
resource "tls_private_key" "worker" {
  algorithm = "ED25519"
}

resource "dokploy_ssh_key" "worker" {
  name                   = "worker"
  public_key             = tls_private_key.worker.public_key_openssh
  private_key_wo         = tls_private_key.worker.private_key_openssh
  private_key_wo_version = 1
}

resource "dokploy_server" "worker" {
  name       = "worker-1"
  ip_address = "203.0.113.10"
  ssh_key_id = dokploy_ssh_key.worker.id
}

resource "dokploy_project" "data" {
  name = "data"
}

resource "dokploy_postgres" "db" {
  name                         = "db"
  environment_id               = dokploy_project.data.production_environment_id
  server_id                    = dokploy_server.worker.id
  database_name                = "app"
  database_user                = "app"
  database_password_wo         = var.db_password
  database_password_wo_version = 1
}
```

Add the public key to `~root/.ssh/authorized_keys` on the machine before the
setup: Dokploy signs in as `root` with the private key.

## Private images from a registry

The registry login lets Dokploy pull a private image. Dokploy runs
`docker login` on the server when it stores the record, so the token must be
valid at apply time.

```hcl
resource "dokploy_registry" "ghcr" {
  name                = "ghcr"
  url                 = "ghcr.io"
  username            = "my-org-bot"
  password_wo         = var.ghcr_token
  password_wo_version = 1
}

resource "dokploy_project" "internal" {
  name = "internal"
}

resource "dokploy_application" "worker" {
  name           = "worker"
  environment_id = dokploy_project.internal.production_environment_id

  docker = {
    image        = "ghcr.io/my-org/worker:2.1.0"
    registry_url = "ghcr.io"
    username     = "my-org-bot"
    password     = var.ghcr_token
  }
}
```

An application that Dokploy builds can push its image to the same registry:
set `registry_id = dokploy_registry.ghcr.id` on it.

## A teammate with limited access

The user gets an initial password and the `member` role. The permissions
resource then opens one project to them and lets them create services, but
not delete them.

```hcl
resource "dokploy_project" "shop" {
  name = "shop"
}

resource "dokploy_user" "dev" {
  email               = "dev@example.com"
  role                = "member"
  password_wo         = var.dev_initial_password
  password_wo_version = 1
}

resource "dokploy_user_permissions" "dev" {
  user_id             = dokploy_user.dev.id
  accessed_projects   = [dokploy_project.shop.id]
  can_create_services = true
  can_delete_services = false
}
```

For a person who was invited in the Dokploy UI, look the user up by email
with the `dokploy_user` data source instead of the `dokploy_user` resource.

## Nightly backups with an email alert

The destination is an S3-compatible bucket. The backup dumps the database
every night at 03:00, and the email channel reports each backup result.

```hcl
resource "dokploy_destination" "backups" {
  name                         = "backups"
  provider_name                = "Cloudflare"
  endpoint                     = "https://${var.r2_account_id}.r2.cloudflarestorage.com"
  bucket                       = "dokploy-backups"
  region                       = "auto"
  access_key_wo                = var.r2_access_key
  access_key_wo_version        = 1
  secret_access_key_wo         = var.r2_secret_access_key
  secret_access_key_wo_version = 1
}

resource "dokploy_backup" "db" {
  service_id      = dokploy_postgres.db.id
  service_type    = "postgres"
  destination_id  = dokploy_destination.backups.id
  database        = "app"
  prefix          = "shop/"
  cron_expression = "0 3 * * *"
}

resource "dokploy_email_notification" "backups" {
  name                = "backup-mail"
  smtp_server         = "smtp.example.com"
  smtp_port           = 587
  username            = "dokploy@example.com"
  password_wo         = var.smtp_password
  password_wo_version = 1
  from_address        = "dokploy@example.com"
  to_addresses        = ["ops@example.com"]

  database_backup = true
}
```

## Environment variables as a map

`dokploy_environment_variables` owns the whole variable list of one
application, compose, or environment. The target must not manage `env`
itself, so it carries an `ignore_changes` block.

```hcl
resource "dokploy_application" "api" {
  name           = "api"
  environment_id = dokploy_project.shop.production_environment_id

  docker = {
    image = "ghcr.io/my-org/api:1.4.2"
  }

  lifecycle {
    ignore_changes = [env]
  }
}

resource "dokploy_environment_variables" "api" {
  application_id = dokploy_application.api.id

  variables = {
    PORT      = "3000"
    LOG_LEVEL = "info"
    DB_URL    = "postgres://app:${var.db_password}@${dokploy_postgres.db.app_name}:5432/app"
  }
}
```

A change to the map does not redeploy the application. Dokploy applies the
variables on the next deploy.

## Where to go next

- The [Secrets guide](secrets) explains the `_wo` companions that each
  example uses, and what stays in the state.
- The [Deploy semantics guide](deploy-semantics) explains when a change
  deploys and how long the provider waits.
- The [Adopt guide](adopting-an-existing-instance) explains how to bring a
  server that the UI configured under Terraform.
