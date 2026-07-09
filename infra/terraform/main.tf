data "digitalocean_kubernetes_versions" "current" {}

resource "digitalocean_vpc" "courtside" {
  name   = "${var.cluster_name}-vpc"
  region = var.region
}

resource "digitalocean_kubernetes_cluster" "courtside" {
  name          = var.cluster_name
  region        = var.region
  version       = data.digitalocean_kubernetes_versions.current.latest_version
  vpc_uuid      = digitalocean_vpc.courtside.id
  auto_upgrade  = false
  surge_upgrade = true

  node_pool {
    name       = "${var.cluster_name}-workers"
    size       = var.node_size
    node_count = var.node_count
  }
}
