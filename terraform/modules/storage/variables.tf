variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "api_service_account" {
  description = "API service account email (read access)"
  type        = string
}

variable "worker_service_account" {
  description = "Worker service account email (write access)"
  type        = string
}
