resource "google_service_account" "worker" {
  account_id   = "${var.name_prefix}-worker"
  display_name = "Zagforge Worker service account"
}

resource "google_cloud_run_v2_job" "worker" {
  name     = "${var.name_prefix}-worker"
  location = var.region

  template {
    task_count = 1

    template {
      service_account = google_service_account.worker.email
      timeout         = "900s" # 15 minutes
      max_retries     = 0      # Retries handled at Cloud Tasks level

      containers {
        # Placeholder image — GitHub Actions owns the actual image tag
        image = "us-docker.pkg.dev/cloudrun/container/hello"

        resources {
          limits = {
            cpu    = "2"
            memory = "4Gi"
          }
        }

        env {
          name  = "APP_ENV"
          value = var.environment
        }
      }
    }
  }

  lifecycle {
    ignore_changes = [
      template[0].template[0].containers[0].image,
    ]
  }
}

# Allow Cloud Tasks to invoke the worker job
resource "google_cloud_run_v2_job_iam_member" "tasks_invoker" {
  name     = google_cloud_run_v2_job.worker.name
  location = var.region
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.worker.email}"
}
