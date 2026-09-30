# A highly available service. Swarm runs two tasks, starts each new task
# before it stops the old one, and restarts a task that fails the health check.
# Set swarm.mode or replicas. The provider rejects a plan that sets both.
# Durations are in nanoseconds.
resource "dokploy_application" "ha" {
  name           = "api"
  environment_id = dokploy_project.example.production_environment_id

  docker = {
    image = "traefik/whoami:v1.10"
  }

  swarm = {
    mode = {
      replicated = {
        replicas = 2
      }
    }

    update_config = {
      parallelism = 1
      delay       = 10000000000 # 10 seconds
      order       = "start-first"
    }

    health_check = {
      test         = ["CMD", "wget", "-q", "-O", "/dev/null", "http://localhost:80/health"]
      interval     = 10000000000 # 10 seconds
      timeout      = 3000000000  # 3 seconds
      retries      = 3
      start_period = 5000000000 # 5 seconds
    }
  }
}
