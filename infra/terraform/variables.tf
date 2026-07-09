variable "cluster_name" {
  description = "DOKS cluster name"
  type        = string
  default     = "courtside"
}

variable "region" {
  description = "DigitalOcean region slug"
  type        = string
  default     = "nyc3"
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
