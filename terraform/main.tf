terraform {
  required_version = ">= 1.5"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

locals {
  name_prefix = "zagforge"
}

# --- Artifact Registry ---
module "registry" {
  source = "./modules/registry"

  project_id  = var.project_id
  region      = var.region
  name_prefix = local.name_prefix
}

# --- Secret Manager ---
module "secrets" {
  source = "./modules/secrets"

  project_id           = var.project_id
  api_service_account  = module.api.service_account_email
  worker_service_account = module.worker.service_account_email
}

# --- GCS Storage ---
module "storage" {
  source = "./modules/storage"

  project_id             = var.project_id
  region                 = var.region
  name_prefix            = local.name_prefix
  api_service_account    = module.api.service_account_email
  worker_service_account = module.worker.service_account_email
}

# --- Database ---
module "database" {
  source = "./modules/database"

  project_id        = var.project_id
  region            = var.region
  name_prefix       = local.name_prefix
  database_provider = var.database_provider
  cloud_sql_tier    = var.cloud_sql_tier
}

# --- Redis ---
module "redis" {
  source = "./modules/redis"

  project_id     = var.project_id
  region         = var.region
  name_prefix    = local.name_prefix
  redis_provider = var.redis_provider
  memory_gb      = var.redis_memory_gb
}

# --- Cloud Tasks Queue ---
module "queue" {
  source = "./modules/queue"

  project_id  = var.project_id
  region      = var.region
  name_prefix = local.name_prefix
}

# --- API (Cloud Run Service) ---
module "api" {
  source = "./modules/api"

  project_id           = var.project_id
  region               = var.region
  name_prefix          = local.name_prefix
  min_instances        = var.api_min_instances
  max_instances        = var.api_max_instances
  environment          = var.environment
  github_app_id        = var.github_app_id
  github_app_slug      = var.github_app_slug
  gcs_bucket           = module.storage.bucket_name
  cloud_tasks_project  = var.project_id
  cloud_tasks_location = var.region
  cloud_tasks_queue      = module.queue.queue_name
  cloud_tasks_worker_url      = module.worker.url
  cloud_tasks_service_account = module.api.service_account_email
  cors_allowed_origins   = var.cors_allowed_origins
}

# --- Worker (Cloud Run Service) ---
module "worker" {
  source = "./modules/worker"

  project_id    = var.project_id
  region        = var.region
  name_prefix   = local.name_prefix
  environment   = var.environment
  github_app_id = var.github_app_id
  gcs_bucket    = module.storage.bucket_name
  api_url       = var.api_url
}

# --- Cloud Scheduler (Watchdog) ---
module "scheduler" {
  source = "./modules/scheduler"

  project_id           = var.project_id
  region               = var.region
  name_prefix          = local.name_prefix
  api_url              = module.api.url
  api_service_account  = module.api.service_account_email
}

# --- Workload Identity Federation (GitHub Actions → GCP) ---
module "wif" {
  source = "./modules/wif"

  project_id  = var.project_id
  github_repo = var.github_repo
}

# --- Networking (LB + Cloud Armor) ---
module "networking" {
  source = "./modules/networking"

  project_id          = var.project_id
  name_prefix         = local.name_prefix
  cloud_armor_enabled = var.cloud_armor_enabled
  domain              = var.domain
  api_cloud_run_name  = module.api.service_name
  region              = var.region
}
