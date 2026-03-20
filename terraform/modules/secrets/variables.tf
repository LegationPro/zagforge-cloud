variable "project_id" {
  type = string
}

variable "api_service_account" {
  description = "API service account email for IAM bindings"
  type        = string
}

variable "worker_service_account" {
  description = "Worker service account email for IAM bindings"
  type        = string
}
