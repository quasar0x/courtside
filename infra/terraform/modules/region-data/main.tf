resource "digitalocean_vpc" "this" {
  name   = "${var.name}-vpc"
  region = var.region
}

resource "digitalocean_database_cluster" "pg" {
  name                 = "${var.name}-pg"
  engine               = "pg"
  version              = "16"
  size                 = var.pg_size
  region               = var.region
  node_count           = var.pg_node_count
  private_network_uuid = digitalocean_vpc.this.id

  lifecycle {
    prevent_destroy = true
  }
}

resource "digitalocean_database_db" "service" {
  for_each   = toset(var.service_databases)
  cluster_id = digitalocean_database_cluster.pg.id
  name       = each.value
}
