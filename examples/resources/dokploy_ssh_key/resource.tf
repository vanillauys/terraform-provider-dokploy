# Generate the key pair in Terraform and register it in Dokploy. The
# hashicorp/tls provider keeps the private key in the state; the write-only
# companion below keeps it out of the Dokploy provider's part of the state.
resource "tls_private_key" "deploy" {
  algorithm = "ED25519"
}

resource "dokploy_ssh_key" "deploy" {
  name        = "deploy"
  description = "Key that Dokploy uses to reach the worker servers"
  public_key  = tls_private_key.deploy.public_key_openssh

  # Write-only: Terraform 1.11 or later. Change the version to send a new key,
  # which replaces the record because Dokploy cannot update a stored key.
  private_key_wo         = tls_private_key.deploy.private_key_openssh
  private_key_wo_version = 1
}

# Or register a key pair that already exists on disk.
resource "dokploy_ssh_key" "existing" {
  name        = "ci"
  public_key  = file("~/.ssh/ci.pub")
  private_key = file("~/.ssh/ci")
}
