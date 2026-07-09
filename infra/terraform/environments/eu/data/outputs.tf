output "vpc_id" {
  value = module.data.vpc_id
}
output "db_cluster_id" {
  value = module.data.db_cluster_id
}
output "pg_host" {
  value = module.data.pg_host
}
output "pg_port" {
  value = module.data.pg_port
}
output "pg_user" {
  value = module.data.pg_user
}
output "pg_password" {
  value     = module.data.pg_password
  sensitive = true
}
output "pg_databases" {
  value = module.data.pg_databases
}
