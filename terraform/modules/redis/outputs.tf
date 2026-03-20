output "host" {
  value = var.redis_provider == "memorystore" ? google_redis_instance.main[0].host : ""
}

output "port" {
  value = var.redis_provider == "memorystore" ? google_redis_instance.main[0].port : 0
}

output "auth_string" {
  value     = var.redis_provider == "memorystore" ? google_redis_instance.main[0].auth_string : ""
  sensitive = true
}
