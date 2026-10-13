resource "google_secret_manager_secret" "prowl_github_client_secret" {
  project   = var.project_id
  secret_id = "prowl-github-client-secret"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "purr_ai_gateway_api_key" {
  project   = var.project_id
  secret_id = "purr-ai-gateway-api-key"
  labels = {
    owner = "${var.team}-tooling"
  }

  replication {
    auto {}
  }
}
