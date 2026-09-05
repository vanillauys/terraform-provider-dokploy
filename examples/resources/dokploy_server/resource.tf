# A remote worker that Dokploy manages over SSH. The record needs a key that
# the root user on the machine accepts.
resource "dokploy_ssh_key" "deploy" {
  name        = "deploy"
  public_key  = file("~/.ssh/dokploy.pub")
  private_key = file("~/.ssh/dokploy")
}

resource "dokploy_server" "worker" {
  name        = "worker-1"
  description = "Runs the production services"
  ip_address  = "203.0.113.10"
  ssh_key_id  = dokploy_ssh_key.deploy.id
  # port, username, server_type, and enable_docker_cleanup keep the Dokploy
  # defaults: 22, root, deploy, true.
}

# After the first apply, open Settings > Servers in Dokploy and run
# "Setup Server": that installs Docker on the machine. The provider does not
# run the setup. Services then run on the worker through server_id:
resource "dokploy_postgres" "db" {
  name           = "db"
  environment_id = dokploy_project.app.production_environment_id
  server_id      = dokploy_server.worker.id
}

# A build server only builds images. Set the SSH port and user when they
# differ from the defaults.
resource "dokploy_server" "builder" {
  name        = "builder"
  ip_address  = "203.0.113.11"
  port        = 2222
  username    = "ubuntu"
  server_type = "build"
  ssh_key_id  = dokploy_ssh_key.deploy.id
}
