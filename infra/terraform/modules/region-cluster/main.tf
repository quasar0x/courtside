data "digitalocean_kubernetes_versions" "current" {}

resource "digitalocean_kubernetes_cluster" "this" {
  name          = var.name
  region        = var.region
  version       = data.digitalocean_kubernetes_versions.current.latest_version
  vpc_uuid      = var.vpc_id
  auto_upgrade  = false
  surge_upgrade = true

  node_pool {
    name       = "${var.name}-workers"
    size       = var.node_size
    auto_scale = true
    min_nodes  = var.min_nodes
    max_nodes  = var.max_nodes
  }
}

resource "digitalocean_database_firewall" "pg" {
  cluster_id = var.db_cluster_id

  rule {
    type  = "k8s"
    value = digitalocean_kubernetes_cluster.this.id
  }
}

# ---------- Managed Kafka (cattle: lifecycle tied to the cluster) ----------
# DO requires node_count = 3 for Kafka. Verify slugs/versions:
#   doctl databases options slugs --engine kafka
#   doctl databases options versions --engine kafka
resource "digitalocean_database_cluster" "kafka" {
  name                 = "${var.name}-kafka"
  engine               = "kafka"
  version              = var.kafka_version
  size                 = var.kafka_size
  region               = var.region
  node_count           = 3
  private_network_uuid = var.vpc_id
}

resource "digitalocean_database_kafka_topic" "membership_created" {
  cluster_id         = digitalocean_database_cluster.kafka.id
  name               = "membership.created"
  partition_count    = 3
  replication_factor = 3
}

resource "digitalocean_database_firewall" "kafka" {
  cluster_id = digitalocean_database_cluster.kafka.id
  rule {
    type  = "k8s"
    value = digitalocean_kubernetes_cluster.this.id
  }
}

data "digitalocean_database_ca" "kafka" {
  cluster_id = digitalocean_database_cluster.kafka.id
}

# ---------- Managed Valkey (cattle) ----------
resource "digitalocean_database_cluster" "valkey" {
  name                 = "${var.name}-valkey"
  engine               = "valkey"
  version              = var.valkey_version
  size                 = var.valkey_size
  region               = var.region
  node_count           = var.valkey_node_count
  private_network_uuid = var.vpc_id
}

resource "digitalocean_database_firewall" "valkey" {
  cluster_id = digitalocean_database_cluster.valkey.id
  rule {
    type  = "k8s"
    value = digitalocean_kubernetes_cluster.this.id
  }
}

data "digitalocean_database_ca" "valkey" {
  cluster_id = digitalocean_database_cluster.valkey.id
}
