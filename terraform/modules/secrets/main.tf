locals {
  secrets = [
    "github-app-private-key",
    "github-app-webhook-secret",
    "hmac-signing-key",
    "clerk-secret-key",
    "database-url",
    "watchdog-secret",
  ]

  accounts = ["api", "worker"]

  # Static keys for for_each — avoids unknown values in keys
  secret_account_pairs = {
    for pair in setproduct(local.secrets, local.accounts) :
    "${pair[0]}-${pair[1]}" => {
      secret_id    = pair[0]
      account_name = pair[1]
    }
  }
}

resource "google_secret_manager_secret" "secrets" {
  for_each  = toset(local.secrets)
  secret_id = each.value

  replication {
    auto {}
  }
}

# Grant both service accounts access to all secrets
resource "google_secret_manager_secret_iam_member" "accessor" {
  for_each = local.secret_account_pairs

  secret_id = google_secret_manager_secret.secrets[each.value.secret_id].id
  role      = "roles/secretmanager.secretAccessor"
  member = (
    each.value.account_name == "api"
    ? "serviceAccount:${var.api_service_account}"
    : "serviceAccount:${var.worker_service_account}"
  )
}
