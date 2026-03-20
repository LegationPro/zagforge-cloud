variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "memory_gb" {
  type    = number
  default = 1
}

variable "redis_provider" {
  description = "Redis provider: upstash (dev) or memorystore (prod)"
  type        = string
  default     = "upstash"
}
