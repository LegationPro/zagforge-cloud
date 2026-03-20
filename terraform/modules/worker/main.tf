resource "google_service_account" "worker" {
  account_id   = "${var.name_prefix}-worker"
  display_name = "Zagforge Worker service account"
}

resource "google_cloud_run_v2_service" "worker" {
  name     = "${var.name_prefix}-worker"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  template {
    service_account = google_service_account.worker.email

    scaling {
      min_instance_count = 0
      max_instance_count = 5
    }

    timeout = "900s"

    containers {
      # Placeholder image — GitHub Actions owns the actual image tag
      image = "us-docker.pkg.dev/cloudrun/container/hello"

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = "2"
          memory = "4Gi"
        }
      }

      # --- Non-sensitive config ---
      env {
        name  = "APP_ENV"
        value = var.environment
      }
      env {
        name  = "WORKER_MODE"
        value = "http"
      }
      env {
        name  = "GITHUB_APP_ID"
        value = var.github_app_id
      }
      env {
        name  = "GCS_BUCKET"
        value = var.gcs_bucket
      }
      env {
        name  = "API_BASE_URL"
        value = var.api_url
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
    }
  }

  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
    ]
  }
}

# Allow Cloud Tasks service account to invoke the worker
resource "google_cloud_run_v2_service_iam_member" "tasks_invoker" {
  name     = google_cloud_run_v2_service.worker.name
  location = var.region
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.worker.email}"
}
