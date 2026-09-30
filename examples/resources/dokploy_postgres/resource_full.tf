# The password comes from an ephemeral resource. Terraform does not write it
# to the plan or to the state. Write-only arguments need Terraform 1.11 or later.
ephemeral "random_password" "db" {
  length  = 32
  special = false
}

resource "dokploy_postgres" "ha" {
  name           = "app-db"
  environment_id = dokploy_project.example.production_environment_id
  database_name  = "app"
  database_user  = "app"
  docker_image   = "postgres:16-alpine"

  # Change the version to send a new password to Dokploy.
  database_password_wo         = ephemeral.random_password.db.result
  database_password_wo_version = 1

  # Run the database on a node that has the label tier=db.
  # Set swarm.mode or replicas. The provider rejects a plan that sets both.
  swarm = {
    mode = {
      replicated = {
        replicas = 1
      }
    }

    placement = {
      constraints = ["node.labels.tier == db"]
    }
  }
}
