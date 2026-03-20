output "service_name" {
  value = google_cloud_run_v2_service.worker.name
}

output "url" {
  value = google_cloud_run_v2_service.worker.uri
}

output "service_account_email" {
  value = google_service_account.worker.email
}
