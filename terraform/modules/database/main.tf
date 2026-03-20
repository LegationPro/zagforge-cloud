# Cloud SQL instance — only created when database_provider = "cloudsql"
# When using Neon (dev), the DATABASE_URL is managed externally via Doppler.

resource "google_sql_database_instance" "main" {
  count            = var.database_provider == "cloudsql" ? 1 : 0
  name             = "${var.name_prefix}-postgres"
  database_version = "POSTGRES_16"
  region           = var.region

  settings {
    tier              = var.cloud_sql_tier
    availability_type = "REGIONAL"
    disk_autoresize   = true

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
    }

    ip_configuration {
      ipv4_enabled = false
      # Cloud Run connects via Cloud SQL Auth Proxy / private IP
    }

    database_flags {
      name  = "max_connections"
      value = "100"
    }
  }

  deletion_protection = true
}

resource "google_sql_database" "main" {
  count    = var.database_provider == "cloudsql" ? 1 : 0
  name     = var.name_prefix
  instance = google_sql_database_instance.main[0].name
}

resource "google_sql_user" "main" {
  count    = var.database_provider == "cloudsql" ? 1 : 0
  name     = var.name_prefix
  instance = google_sql_database_instance.main[0].name
  password = random_password.db_password[0].result
}

resource "random_password" "db_password" {
  count   = var.database_provider == "cloudsql" ? 1 : 0
  length  = 32
  special = false
}
