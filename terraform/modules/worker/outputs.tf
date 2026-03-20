output "job_name" {
  value = google_cloud_run_v2_job.worker.name
}

output "service_account_email" {
  value = google_service_account.worker.email
}
