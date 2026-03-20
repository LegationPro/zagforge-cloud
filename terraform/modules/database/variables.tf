variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "database_provider" {
  description = "neon or cloudsql"
  type        = string
}

variable "cloud_sql_tier" {
  type    = string
  default = "db-custom-2-8192"
}
