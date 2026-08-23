output "cluster_id" {
  value = digitalocean_kubernetes_cluster.this.id
}

output "cluster_name" {
  value = digitalocean_kubernetes_cluster.this.name
}

output "kubeconfig" {
  value     = digitalocean_kubernetes_cluster.this.kube_config[0].raw_config
  sensitive = true
}

output "kafka_host" {
  value = digitalocean_database_cluster.kafka.private_host
}
output "kafka_port" {
  value = digitalocean_database_cluster.kafka.port
}
output "kafka_user" {
  value = digitalocean_database_cluster.kafka.user
}
output "kafka_password" {
  value     = digitalocean_database_cluster.kafka.password
  sensitive = true
}
output "kafka_ca" {
  value = data.digitalocean_database_ca.kafka.certificate
}
output "valkey_host" {
  value = digitalocean_database_cluster.valkey.private_host
}
output "valkey_port" {
  value = digitalocean_database_cluster.valkey.port
}
output "valkey_user" {
  value = digitalocean_database_cluster.valkey.user
}
output "valkey_password" {
  value     = digitalocean_database_cluster.valkey.password
  sensitive = true
}
output "valkey_ca" {
  value = data.digitalocean_database_ca.valkey.certificate
}
