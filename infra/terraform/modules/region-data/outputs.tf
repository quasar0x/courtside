output "vpc_id" {
  value = digitalocean_vpc.this.id
}

output "db_cluster_id" {
  value = digitalocean_database_cluster.pg.id
}

output "pg_host" {
  value = digitalocean_database_cluster.pg.private_host
}

output "pg_port" {
  value = digitalocean_database_cluster.pg.port
}

output "pg_user" {
  value = digitalocean_database_cluster.pg.user
}

output "pg_password" {
  value     = digitalocean_database_cluster.pg.password
  sensitive = true
}

output "pg_databases" {
  value = [for db in digitalocean_database_db.service : db.name]
}
