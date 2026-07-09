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
