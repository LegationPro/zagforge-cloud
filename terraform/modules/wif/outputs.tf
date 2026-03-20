output "provider_name" {
  description = "WIF provider resource name — use as WIF_PROVIDER GitHub secret"
  value       = google_iam_workload_identity_pool_provider.github.name
}

output "service_account_email" {
  description = "Deploy service account email — use as WIF_SERVICE_ACCOUNT GitHub secret"
  value       = google_service_account.github_actions.email
}
