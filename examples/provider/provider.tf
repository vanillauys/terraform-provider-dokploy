provider "dokploy" {
  # If endpoint is unset, the provider reads the DOKPLOY_ENDPOINT environment
  # variable. If api_key is unset, it reads DOKPLOY_API_KEY.
  endpoint = "https://dokploy.example.com"

  # Set insecure = true only for a server with a self-signed certificate.
}
