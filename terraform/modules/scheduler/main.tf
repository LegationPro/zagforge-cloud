resource "google_cloud_scheduler_job" "watchdog" {
  name     = "${var.name_prefix}-watchdog"
  schedule = "*/5 * * * *"
  region   = var.region

  http_target {
    http_method = "POST"
    uri         = "${var.api_url}/internal/watchdog/timeout"

    oidc_token {
      service_account_email = var.api_service_account
      audience              = var.api_url
    }
  }

  retry_config {
    retry_count = 1
  }
}
