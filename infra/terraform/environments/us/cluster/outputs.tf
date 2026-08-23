output "kubeconfig" {
  value     = module.cluster.kubeconfig
  sensitive = true
}
output "cluster_id" {
  value = module.cluster.cluster_id
}
output "pg_host" {
  value = data.terraform_remote_state.data.outputs.pg_host
}
output "pg_user" {
  value = data.terraform_remote_state.data.outputs.pg_user
}
output "pg_password" {
  value     = data.terraform_remote_state.data.outputs.pg_password
  sensitive = true
}
output "pg_databases" {
  value = data.terraform_remote_state.data.outputs.pg_databases
}

output "kafka_host" {
  value = module.cluster.kafka_host
}
output "kafka_port" {
  value = module.cluster.kafka_port
}
output "kafka_user" {
  value = module.cluster.kafka_user
}
output "kafka_password" {
  value     = module.cluster.kafka_password
  sensitive = true
}
output "kafka_ca" {
  value = module.cluster.kafka_ca
}
output "valkey_host" {
  value = module.cluster.valkey_host
}
output "valkey_port" {
  value = module.cluster.valkey_port
}
output "valkey_user" {
  value = module.cluster.valkey_user
}
output "valkey_password" {
  value     = module.cluster.valkey_password
  sensitive = true
}
output "valkey_ca" {
  value = module.cluster.valkey_ca
}
