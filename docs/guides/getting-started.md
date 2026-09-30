---
page_title: "Get started"
subcategory: ""
description: |-
  Configure the provider against a Dokploy server, then apply a first project, database, application, and domain.
---

# Get started

This guide starts from a running Dokploy server. It brings a project, a
PostgreSQL database, an application, and a domain under Terraform management.

You need:

- A Dokploy server that you can reach over HTTPS.
- An API key from **Settings > API/CLI** in the Dokploy UI.

## Before your first apply: API key rate limits

Read this section first. This problem is the most likely cause of a failed
first apply, and the error does not describe the cause.

Dokploy can rate-limit an API key on the server, in its api-key plugin. When a
key reaches the limit, **the API answers `401 Unauthorized`, not `429`**. An
exhausted key therefore looks like an authentication failure. A key that works
for one request can fail in the middle of a larger apply. The error then reads
as "the credentials are wrong", but the credentials are correct.

Whether a key has a limit depends on how you made it. Both cases were probed
on Dokploy v0.30.5 (2026-09-05):

- A key from the Dokploy UI (**Settings > Profile > API/CLI Keys > Generate
  New Key**) has no limit while the **Enable Rate Limiting** switch stays off,
  which is its default. Such a key answered every request in the probe.
- A key from the `user.createApiKey` API call without the `rateLimitEnabled`
  field gets the plugin default: `rateLimitMax` 10 in a `rateLimitTimeWindow`
  of 24 hours. Such a key answered five `project.all` requests, then `401` on
  every request, and still `401` after a minute. One `terraform plan` of a
  small configuration exceeds that budget.

If an apply fails with an unexpected `401` on a key that works for single
requests, generate a new key in the UI and leave the switch off. A retry does
not help: the window is a day. The acceptance rig mints its key with
`rateLimitEnabled: false` in `acceptance/bootstrap.sh`.

## Set the credentials

Export the endpoint and the API key. The provider reads both variables, so
no secret enters a file.

```bash
export DOKPLOY_ENDPOINT=https://dokploy.example.com
export DOKPLOY_API_KEY=...
```

Set `insecure = true` in the `provider` block only when the server presents a
self-signed certificate. A `provider` block can also set `endpoint` and
`api_key`, but the environment variables keep the key out of the file.

## Write the configuration

Copy this complete file to `main.tf`. It goes from an empty directory to a
running application with a domain. The provider follows semantic versioning
from v1.0.0. The constraint `~> 1.8` accepts each 1.x release from v1.8.0, and
a minor release never changes what an existing attribute does. The
[Upgrade guide](upgrading) lists what each release needs from your
configuration.

The Dokploy hierarchy is **project > environment > service**. Dokploy creates
a `production` environment with each project, so you rarely need
`dokploy_environment` on the first day.

```hcl
terraform {
  required_providers {
    dokploy = {
      source  = "vanillauys/dokploy"
      version = "~> 1.8"
    }
  }
}

provider "dokploy" {}

variable "db_password" {
  type      = string
  sensitive = true
}

resource "dokploy_project" "example" {
  name        = "example"
  description = "Managed by Terraform"
}

resource "dokploy_postgres" "db" {
  name                         = "app-db"
  environment_id               = dokploy_project.example.production_environment_id
  database_name                = "app"
  database_user                = "app"
  database_password_wo         = var.db_password
  database_password_wo_version = 1
  docker_image                 = "postgres:16-alpine"
}

resource "dokploy_application" "web" {
  name           = "web"
  environment_id = dokploy_project.example.production_environment_id

  docker = {
    image = "traefik/whoami:v1.10"
  }

  env = <<-EOT
    PORT=80
  EOT
}

resource "dokploy_domain" "web" {
  application_id   = dokploy_application.web.id
  host             = "app.example.com"
  port             = 80
  https            = true
  certificate_type = "letsencrypt"
}

output "url" {
  value = "https://${dokploy_domain.web.host}"
}
```

The domain `app.example.com` must point at the IP address of the Dokploy
server, or Let's Encrypt cannot issue the certificate. Replace it with your
own host name.

The password uses the write-only attribute `database_password_wo`, which needs
Terraform 1.11 or later. On Terraform 1.5 to 1.10, use `database_password =
var.db_password` and remove the `_wo_version` line.

## Apply

Run these commands in the directory of `main.tf`:

```bash
export TF_VAR_db_password="$(openssl rand -base64 24)"
terraform init
terraform apply
```

The `TF_VAR_db_password` value is not in the file. Keep the value in a
password manager, because the next run of `terraform` needs it again. The
provider sends the password only when `database_password_wo_version`
changes.

The reference has the full schema of each resource:
[`dokploy_project`](../resources/project),
[`dokploy_postgres`](../resources/postgres),
[`dokploy_application`](../resources/application), and
[`dokploy_domain`](../resources/domain).

**Use `production_environment_id` for the default environment.** Dokploy
creates that environment with each project and names it `production`. The
attribute selects it with the server's `isDefault` flag, so a rename does not
change the value. The `environments` list keeps the order of the Dokploy API
response, and the provider does not sort it, so `environments[0]` is not fixed
to `production`. Use the list, or the `dokploy_environment` data source, only
for an environment that is not the default. Do not derive `environment_id`
from the list with a `for` expression: a project update marks the list unknown
in the plan, and an unknown `environment_id` forces a replacement of the
service.

[Secrets and sensitive values](secrets) explains why the provider does not
mark `env` as sensitive.

## What happens on apply

When Terraform creates a service, the provider deploys it. The provider starts
the deploy, then polls the server until the deploy reaches a terminal status
or `deployment_timeout` (default `15m`) expires. An apply that appears to hang
is usually a deploy in progress.

A failed deploy fails the apply, and the diagnostic shows the Dokploy status.
A timeout also fails the apply, but the deploy continues on the server.

[Deploy semantics](deploy-semantics) describes both behaviors and how to
disable the deploy.

## Next steps

- If the server already has services, see
  [Adopt an existing Dokploy server](adopting-an-existing-instance).
- For secrets, see [Secrets and sensitive values](secrets).
- To control deploys, see [Deploy semantics](deploy-semantics).
