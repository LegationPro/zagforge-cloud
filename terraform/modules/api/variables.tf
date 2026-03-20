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

variable "min_instances" {
  type    = number
  default = 0
}

variable "max_instances" {
  type    = number
  default = 2
}

variable "github_app_id" {
  type = string
}

variable "github_app_slug" {
  type = string
}

variable "gcs_bucket" {
  type = string
}

variable "cloud_tasks_project" {
  type = string
}

variable "cloud_tasks_location" {
  type = string
}

variable "cloud_tasks_queue" {
  type = string
}

variable "cors_allowed_origins" {
  type    = string
  default = ""
}
