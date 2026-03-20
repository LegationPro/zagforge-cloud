variable "environment" {
  description = "Environment name (dev, staging, prod)"
  type        = string
}

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

# GitHub
variable "github_repo" {
  description = "GitHub repository in owner/repo format"
  type        = string
}

variable "github_app_id" {
  description = "GitHub App ID (non-sensitive)"
  type        = string
}

variable "github_app_slug" {
  description = "GitHub App slug for install URL"
  type        = string
}

# API URL (for worker callback — breaks circular dep between api/worker modules)
variable "api_url" {
  description = "API Cloud Run URL for worker callbacks"
  type        = string
  default     = ""
}

# CORS
variable "cors_allowed_origins" {
  description = "Comma-separated allowed origins for CORS"
  type        = string
  default     = ""
}

# Database
variable "database_provider" {
  description = "Database provider: neon or cloudsql"
  type        = string
  default     = "neon"
}

variable "neon_project_id" {
  description = "Neon project ID (when database_provider = neon)"
  type        = string
  default     = ""
}

variable "cloud_sql_tier" {
  description = "Cloud SQL machine tier (when database_provider = cloudsql)"
  type        = string
  default     = "db-custom-2-8192"
}

# Redis
variable "redis_provider" {
  description = "Redis provider: upstash (dev) or memorystore (prod)"
  type        = string
  default     = "upstash"
}

variable "redis_memory_gb" {
  description = "Redis memory size in GB (only used with memorystore)"
  type        = number
  default     = 1
}

# API scaling
variable "api_min_instances" {
  description = "Minimum API Cloud Run instances"
  type        = number
  default     = 0
}

variable "api_max_instances" {
  description = "Maximum API Cloud Run instances"
  type        = number
  default     = 2
}

# Networking
variable "cloud_armor_enabled" {
  description = "Enable Cloud Armor WAF (off in dev, on in prod)"
  type        = bool
  default     = false
}

variable "domain" {
  description = "Custom domain for the API (e.g. api.zagforge.com)"
  type        = string
  default     = ""
}

# Secrets (references, not values — actual values live in Secret Manager)
variable "secret_ids" {
  description = "Map of secret names to Secret Manager secret IDs"
  type        = map(string)
  default     = {}
}
