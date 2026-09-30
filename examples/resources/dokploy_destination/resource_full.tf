# Test the bucket before the provider writes the record. The Dokploy server
# must reach the endpoint. If the test fails, the apply fails and the provider
# writes nothing. Both keys are write-only: Terraform does not write them to
# the plan or to the state.
resource "dokploy_destination" "checked" {
  name          = "app-backups"
  provider_name = "AWS"
  endpoint      = "https://s3.eu-west-1.amazonaws.com"
  bucket        = "app-backups"
  region        = "eu-west-1"

  access_key_wo                = var.s3_access_key
  access_key_wo_version        = 1
  secret_access_key_wo         = var.s3_secret_access_key
  secret_access_key_wo_version = 1

  verify_connection = true
}
