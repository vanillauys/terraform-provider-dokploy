# An OpenAI-compatible endpoint for Dokploy's AI features (Settings > AI).
resource "dokploy_ai" "openai" {
  name    = "openai"
  api_url = "https://api.openai.com/v1"
  model   = "gpt-4o-mini"

  # Write-only: Terraform 1.11 or later. Change the version to send a new key.
  api_key_wo         = var.openai_api_key
  api_key_wo_version = 1
}

# A self-hosted model, kept but switched off.
resource "dokploy_ai" "local" {
  name       = "ollama"
  api_url    = "http://ollama.internal:11434/v1"
  model      = "llama3"
  api_key    = "ollama"
  is_enabled = false
}
