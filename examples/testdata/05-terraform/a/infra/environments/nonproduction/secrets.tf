resource "google_secret_manager_secret" "prowl_github_client_secret" {
  project   = var.project_id
  secret_id = "prowl-github-client-secret"

  replication {
    auto {}
  }
}
