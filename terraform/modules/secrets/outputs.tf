output "secret_ids" {
  description = "Map of secret name to Secret Manager secret ID"
  value = {
    for name, secret in google_secret_manager_secret.secrets :
    name => secret.id
  }
}
