output "api_url" {
  description = "Cloud Run API service URL"
  value       = module.api.url
}

output "api_service_account" {
  description = "API service account email"
  value       = module.api.service_account_email
}

output "worker_service_account" {
  description = "Worker service account email"
  value       = module.worker.service_account_email
}

output "registry_url" {
  description = "Artifact Registry URL"
  value       = module.registry.repository_url
}

output "gcs_bucket" {
  description = "GCS snapshots bucket name"
  value       = module.storage.bucket_name
}

output "cloud_tasks_queue" {
  description = "Cloud Tasks queue name"
  value       = module.queue.queue_name
}

output "load_balancer_ip" {
  description = "Load balancer IP address (if networking is enabled)"
  value       = module.networking.load_balancer_ip
}

output "wif_provider" {
  description = "WIF provider name — set as WIF_PROVIDER GitHub secret"
  value       = module.wif.provider_name
}

output "wif_service_account" {
  description = "WIF service account — set as WIF_SERVICE_ACCOUNT GitHub secret"
  value       = module.wif.service_account_email
}
