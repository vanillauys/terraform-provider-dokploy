---
page_title: "Usage examples"
subcategory: ""
description: |-
  Short, complete configurations for the common Dokploy setups: an application from GitLab with Slack alerts, a remote worker server, private images from a registry, a teammate with limited access, nightly backups with an email alert, environment variables as a map, a highly available application with Swarm settings, a full stack, a backup of the Dokploy host, connection checks, and lookups.
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

## A highly available application with Swarm settings

The `swarm` attribute models the Docker Swarm service specification. This
example runs two replicas, starts the new task before it stops the old task
on an update, and checks the health of each container. Durations are in
nanoseconds: `10000000000` is 10 seconds.

```hcl
resource "dokploy_project" "ha" {
  name = "ha"
}

resource "dokploy_application" "api" {
  name           = "api"
  environment_id = dokploy_project.ha.production_environment_id

  docker = {
    image = "traefik/whoami:v1.10"
  }

  swarm = {
    mode = {
      replicated = { replicas = 2 }
    }

    update_config = {
      parallelism = 1
      order       = "start-first"
    }

    health_check = {
      test         = ["CMD", "wget", "-q", "-O", "/dev/null", "http://localhost:80"]
      interval     = 10000000000
      timeout      = 5000000000
      retries      = 3
      start_period = 10000000000
    }
  }
}
```

Set the replica count in `swarm.mode.replicated.replicas`. **The top-level
`replicas` attribute and `swarm.mode` cannot both be set.** The provider
rejects the pair at plan time, because Dokploy uses the mode and ignores
`replicas`. Use `replicas` for a simple count, or `swarm.mode` for all other
modes. Swarm cannot change the mode of a service that exists.

A change to `swarm` reaches the service at the next deploy. See
[Deploy semantics](deploy-semantics#swarm-changes-need-a-deploy). The six
database resources take the same block. `dokploy_compose` has no `swarm`
block, and `dokploy_libsql` does not accept `swarm.ulimits`.

## A full stack: database, application, domain, and backup

This stack makes one random password and gives it to the database and to
the application. The application reads the database URL through `env`.

```hcl
terraform {
  required_providers {
    dokploy = {
      source  = "vanillauys/dokploy"
      version = "~> 1.9"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }
}

resource "random_password" "db" {
  length  = 32
  special = false
}

resource "dokploy_project" "shop" {
  name = "shop"
}

resource "dokploy_postgres" "db" {
  name              = "shop-db"
  environment_id    = dokploy_project.shop.production_environment_id
  database_name     = "shop"
  database_user     = "shop"
  database_password = random_password.db.result
  docker_image      = "postgres:16-alpine"
}

resource "dokploy_application" "web" {
  name           = "web"
  environment_id = dokploy_project.shop.production_environment_id

  docker = {
    image = "ghcr.io/my-org/shop:1.0.0"
  }

  env = <<-EOT
    PORT=3000
    DATABASE_URL=postgres://shop:${random_password.db.result}@${dokploy_postgres.db.app_name}:5432/shop
  EOT
}

resource "dokploy_domain" "web" {
  application_id   = dokploy_application.web.id
  host             = "shop.example.com"
  port             = 3000
  https            = true
  certificate_type = "letsencrypt"
}

resource "dokploy_destination" "backups" {
  name                         = "backups"
  provider_name                = "AWS"
  endpoint                     = "https://s3.eu-west-1.amazonaws.com"
  bucket                       = "shop-backups"
  region                       = "eu-west-1"
  access_key_wo                = var.s3_access_key
  access_key_wo_version        = 1
  secret_access_key_wo         = var.s3_secret_access_key
  secret_access_key_wo_version = 1
}

resource "dokploy_backup" "db" {
  service_id      = dokploy_postgres.db.id
  service_type    = "postgres"
  destination_id  = dokploy_destination.backups.id
  database        = "shop"
  prefix          = "shop/"
  cron_expression = "0 3 * * *"
}
```

**The `env` attribute is not write-only, so the password is in the state.**
The `random_password` resource keeps the value in the state, and Dokploy
returns it on read. Use a state backend with encryption at rest, and limit who
can read it. [Secrets and sensitive values](secrets) explains the trade-off.

For a database that no application reads through `env`, keep the password
out of the state with the
[write-only attributes](secrets#write-only-companions).

## A backup of the Dokploy host

`dokploy_web_server_backup` dumps the database and the configuration
directory of Dokploy itself to a destination. It is a separate resource from
`dokploy_backup`, because it has no parent service. The provider does not run
a backup on demand.

```hcl
resource "dokploy_destination" "host" {
  name                         = "host-backups"
  provider_name                = "AWS"
  endpoint                     = "https://s3.eu-west-1.amazonaws.com"
  bucket                       = "dokploy-host-backups"
  region                       = "eu-west-1"
  access_key_wo                = var.s3_access_key
  access_key_wo_version        = 1
  secret_access_key_wo         = var.s3_secret_access_key
  secret_access_key_wo_version = 1
}

resource "dokploy_web_server_backup" "nightly" {
  destination_id  = dokploy_destination.host.id
  cron_expression = "0 2 * * *"
  prefix          = "dokploy/"

  keep_latest_count = 14
}
```

`include_encryption_key` is `true` by default, so the backup holds the key
that decrypts the stored secrets. Store the destination bucket with the same
care as the state.

## Check a connection before the write

`verify_connection = true` tests the credentials before the provider writes
the record. A failed test fails the apply with the message of the server, and
the provider writes nothing.

```hcl
resource "dokploy_registry" "ghcr" {
  name                = "ghcr"
  url                 = "ghcr.io"
  username            = "my-org-bot"
  password_wo         = var.ghcr_token
  password_wo_version = 1

  verify_connection = true
}

resource "dokploy_destination" "backups" {
  name                         = "backups"
  provider_name                = "AWS"
  endpoint                     = "https://s3.eu-west-1.amazonaws.com"
  bucket                       = "my-backups"
  region                       = "eu-west-1"
  access_key_wo                = var.s3_access_key
  access_key_wo_version        = 1
  secret_access_key_wo         = var.s3_secret_access_key
  secret_access_key_wo_version = 1

  verify_connection = true
}
```

The [Secrets guide](secrets#verify-a-connection-before-the-write) lists the
resources that take the attribute and the cases in which the test is not
reliable.

## Look up existing records

The data sources give a second workspace a reference to a record without an
import. A service child needs the id of its parent and one distinguishing
attribute. A notification or an AI record needs an `id` or a `name`. No data
source returns a secret.

```hcl
variable "application_id" {
  type = string
}

data "dokploy_port" "http" {
  application_id = var.application_id
  published_port = 8080
}

data "dokploy_slack_notification" "deploys" {
  name = "deploys"
}

data "dokploy_ai" "main" {
  name = "openai"
}

output "port_id" {
  value = data.dokploy_port.http.id
}

output "deploys_notification_id" {
  value = data.dokploy_slack_notification.deploys.id
}

output "ai_model" {
  value = data.dokploy_ai.main.model
}
```

A lookup that matches no record, or more than one record, fails the plan.

## Where to go next

- The [Secrets guide](secrets) explains the `_wo` companions that each
  example uses, and what stays in the state.
- The [Deploy semantics guide](deploy-semantics) explains when a change
  deploys and how long the provider waits.
- The [Adopt guide](adopting-an-existing-instance) explains how to bring a
  server that the UI configured under Terraform.
