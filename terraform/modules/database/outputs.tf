output "connection_name" {
  description = "Cloud SQL connection name (project:region:instance)"
  value       = var.database_provider == "cloudsql" ? google_sql_database_instance.main[0].connection_name : ""
}

output "database_name" {
  value = var.database_provider == "cloudsql" ? google_sql_database.main[0].name : ""
}
