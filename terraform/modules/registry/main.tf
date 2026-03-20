resource "google_artifact_registry_repository" "main" {
  location      = var.region
  repository_id = var.name_prefix
  format        = "DOCKER"
  description   = "Docker images for ${var.name_prefix} services"
}
