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
