# Memorystore instance — only created when redis_provider = "memorystore"
# When using Upstash (dev), the REDIS_URL is managed externally via Doppler.

resource "google_redis_instance" "main" {
  count          = var.redis_provider == "memorystore" ? 1 : 0
  name           = "${var.name_prefix}-redis"
  memory_size_gb = var.memory_gb
  region         = var.region

  tier                    = "BASIC"
  auth_enabled            = true
  transit_encryption_mode = "SERVER_AUTHENTICATION"

  redis_version = "REDIS_7_0"
}
