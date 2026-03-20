resource "google_service_account" "api" {
  account_id   = "${var.name_prefix}-api"
  display_name = "Zagforge API service account"
}

resource "google_cloud_run_v2_service" "api" {
  name     = "${var.name_prefix}-api"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.api.email

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    containers {
      # Placeholder image — GitHub Actions owns the actual image tag
      image = "us-docker.pkg.dev/cloudrun/container/hello"

      ports {
        container_port = 8080
      }

      # --- Non-sensitive config ---
      env {
        name  = "APP_ENV"
        value = var.environment
      }
      env {
        name  = "GITHUB_APP_ID"
        value = var.github_app_id
      }
      env {
        name  = "GITHUB_APP_SLUG"
        value = var.github_app_slug
      }
      env {
        name  = "GCS_BUCKET"
        value = var.gcs_bucket
      }
      env {
        name  = "CLOUD_TASKS_PROJECT"
        value = var.cloud_tasks_project
      }
      env {
        name  = "CLOUD_TASKS_LOCATION"
        value = var.cloud_tasks_location
      }
      env {
        name  = "CLOUD_TASKS_QUEUE"
        value = var.cloud_tasks_queue
      }
      env {
        name  = "CORS_ALLOWED_ORIGINS"
        value = var.cors_allowed_origins
      }

      # --- Secrets from Secret Manager ---
      env {
        name = "DATABASE_URL"
        value_source {
          secret_key_ref {
            secret  = "database-url"
            version = "latest"
          }
        }
      }
      env {
        name = "GITHUB_APP_PRIVATE_KEY"
        value_source {
          secret_key_ref {
            secret  = "github-app-private-key"
            version = "latest"
          }
        }
      }
      env {
        name = "GITHUB_APP_WEBHOOK_SECRET"
        value_source {
          secret_key_ref {
            secret  = "github-app-webhook-secret"
            version = "latest"
          }
        }
      }
      env {
        name = "HMAC_SIGNING_KEY"
        value_source {
          secret_key_ref {
            secret  = "hmac-signing-key"
            version = "latest"
          }
        }
      }
      env {
        name = "CLERK_SECRET_KEY"
        value_source {
          secret_key_ref {
            secret  = "clerk-secret-key"
            version = "latest"
          }
        }
      }
      env {
        name = "WATCHDOG_SECRET"
        value_source {
          secret_key_ref {
            secret  = "watchdog-secret"
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }

      startup_probe {
        http_get {
          path = "/readyz"
        }
        initial_delay_seconds = 5
        period_seconds        = 3
        failure_threshold     = 10
      }

      liveness_probe {
        http_get {
          path = "/healthz"
        }
        period_seconds = 30
      }
    }
  }

  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
    ]
  }
}

# Allow unauthenticated access (public API)
resource "google_cloud_run_v2_service_iam_member" "public" {
  name     = google_cloud_run_v2_service.api.name
  location = var.region
  role     = "roles/run.invoker"
  member   = "allUsers"
}
