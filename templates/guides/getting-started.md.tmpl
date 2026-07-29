---
page_title: "Getting started"
subcategory: ""
description: |-
  Configure the provider against a Dokploy server and apply a first project, database, application and domain.
---

# Getting started

This guide takes a running Dokploy server and brings a project, a PostgreSQL
database, an application and a domain under Terraform management.

You need a Dokploy server you can reach over HTTPS, and an API key from
**Settings > API/CLI** in the Dokploy UI.

## Before your first apply: API key rate limits

Read this first. It is the most likely thing to break a first apply, and it
does not look like what it is.

Dokploy rate-limits API keys server-side, in its api-key plugin. When the
limit is hit **the API answers `401 Unauthorized` rather than `429`**, so an
exhausted budget surfaces as an authentication failure. A key that works fine
for a single request can fail partway through a larger apply, which reads as
"my credentials are wrong" when the credentials are fine.

Applying a large configuration can exhaust the budget, and so can a
configuration with long-running deploys, because this provider polls the
server while it waits for a deployment to finish.

If applies fail with an unexpected `401` against a key that works for single
requests, you probably need a key with rate limiting disabled. The acceptance
rig mints exactly that, in `acceptance/bootstrap.sh`. Whether keys minted
through the Dokploy UI carry the same limit has not been verified.

## Configure the provider

```hcl
terraform {
  required_providers {
    dokploy = {
      source  = "vanillauys/dokploy"
      version = "~> 0.6"
    }
  }
}

provider "dokploy" {
  endpoint = "https://dokploy.example.com"
  # api_key sourced from the DOKPLOY_API_KEY environment variable
}
```

`endpoint` falls back to `DOKPLOY_ENDPOINT` and `api_key` to
`DOKPLOY_API_KEY`. Set `insecure = true` only if your server presents a
self-signed certificate.

Because this provider is pre-1.0, pin it exactly (`version = "0.6.0"`) if you
need stability: breaking changes land in minor releases until v1.0.0.

## A first configuration

Dokploy's hierarchy is **project > environment > service**. Every project is
created with a `production` environment, so you rarely need
`dokploy_environment` on day one.

```hcl
resource "dokploy_project" "example" {
  name        = "example"
  description = "Managed by Terraform"
}

resource "dokploy_postgres" "db" {
  name              = "app-db"
  environment_id    = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]
  database_name     = "app"
  database_user     = "app"
  database_password = var.db_password
  docker_image      = "postgres:16-alpine"
}

resource "dokploy_application" "web" {
  name           = "web"
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]

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
```

Full schemas for the four resources used here:
[`dokploy_project`](../resources/project),
[`dokploy_postgres`](../resources/postgres),
[`dokploy_application`](../resources/application) and
[`dokploy_domain`](../resources/domain).

**Select the environment by name, not by index.** `environments` is populated
in the order Dokploy's API returns it, and the provider does not sort it, so
`environments[0]` is not pinned to `production` - it happens to be right only
while the project has exactly one environment, and silently starts resolving to
something else once a second one exists. The
`[for e in ... : e.id if e.name == "production"][0]` filter above is
order-independent, and is the idiom used throughout these docs.

`var.db_password` should be a sensitive variable. See
[Secrets and sensitive values](secrets) for why this provider does not mark
`env` sensitive for you.

## What happens on apply

Creating a service deploys it. The provider triggers the deploy, then polls
until it reaches a terminal status or `deployment_timeout` (default `15m`)
expires. An apply that appears to hang is usually a deploy in progress.

A failed deployment fails the apply, with Dokploy's status in the diagnostic.
A timeout also fails the apply, but leaves the server-side deployment running.

Both behaviours, and how to turn them off, are covered in
[Deploy semantics](deploy-semantics).

## Next steps

- Already have services on this server? See
  [Adopting an existing Dokploy instance](adopting-an-existing-instance).
- Handling secrets: [Secrets and sensitive values](secrets).
- Controlling deploys: [Deploy semantics](deploy-semantics).
