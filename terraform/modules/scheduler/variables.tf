variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "api_url" {
  description = "Cloud Run API service URL"
  type        = string
}

variable "api_service_account" {
  description = "Service account email for OIDC auth"
  type        = string
}
