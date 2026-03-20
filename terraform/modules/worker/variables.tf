variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "environment" {
  type = string
}

variable "github_app_id" {
  type = string
}

variable "gcs_bucket" {
  type = string
}

variable "api_url" {
  description = "API base URL for worker callbacks"
  type        = string
}
