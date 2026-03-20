variable "project_id" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "region" {
  type = string
}

variable "cloud_armor_enabled" {
  type    = bool
  default = false
}

variable "domain" {
  description = "Custom domain for SSL cert (empty = skip LB setup)"
  type        = string
  default     = ""
}

variable "api_cloud_run_name" {
  description = "Cloud Run service name for the NEG backend"
  type        = string
}
