# By id.
data "dokploy_environment" "by_id" {
  id = "Ux7kFq2mNp4RtWvYzAbCd"
}

# By name within a project — the usual way to reach the production
# environment Dokploy creates automatically.
data "dokploy_environment" "production" {
  project_id = dokploy_project.example.id
  name       = "production"
}

resource "dokploy_postgres" "db" {
  name              = "db"
  environment_id    = data.dokploy_environment.production.id
  database_name     = "app"
  database_user     = "app"
  database_password = var.db_password
}
