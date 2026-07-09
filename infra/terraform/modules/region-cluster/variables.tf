variable "name" {
  description = "Cluster name (e.g. courtside-us)"
  type        = string
}

variable "region" {
  description = "DigitalOcean region slug"
  type        = string
}

variable "vpc_id" {
  description = "VPC to place the cluster in (from the data state)"
  type        = string
}

variable "db_cluster_id" {
  description = "Managed DB cluster id to grant this cluster access to"
  type        = string
}

variable "node_size" {
  description = "Droplet size slug for worker nodes"
  type        = string
  default     = "s-4vcpu-8gb"
}

variable "node_count" {
  description = "Number of worker nodes"
  type        = number
  default     = 3
}
