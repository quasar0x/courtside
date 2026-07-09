variable "name" {
  description = "Resource name prefix (e.g. courtside-us)"
  type        = string
}

variable "region" {
  description = "DigitalOcean region slug"
  type        = string
}

variable "pg_size" {
  description = "Managed Postgres node size slug"
  type        = string
  default     = "db-s-1vcpu-1gb"
}

variable "pg_node_count" {
  description = "Managed Postgres nodes (1 = single, 2+ = HA)"
  type        = number
  default     = 1
}

variable "service_databases" {
  description = "Per-service logical databases to create"
  type        = list(string)
  default     = ["members", "clubs", "memberships", "billing", "notifications"]
}
