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

variable "min_nodes" {
  description = "Minimum worker nodes (cluster autoscaler floor)"
  type        = number
  default     = 3
}

variable "max_nodes" {
  description = "Maximum worker nodes (cluster autoscaler ceiling)"
  type        = number
  default     = 3
}

variable "kafka_version" {
  type    = string
  default = "3.9"
}
variable "kafka_size" {
  description = "Managed Kafka node size slug"
  type        = string
  default     = "db-s-2vcpu-4gb"
}
variable "valkey_version" {
  type    = string
  default = "8"
}
variable "valkey_size" {
  type    = string
  default = "db-s-1vcpu-1gb"
}
variable "valkey_node_count" {
  description = "1 = single node w/ auto-failover; 2 = HA standby"
  type        = number
  default     = 1
}
