output "cluster_name" {
  value = digitalocean_kubernetes_cluster.courtside.name
}

output "region" {
  value = digitalocean_kubernetes_cluster.courtside.region
}

output "kubernetes_version" {
  value = digitalocean_kubernetes_cluster.courtside.version
}

output "kubeconfig" {
  description = "Raw kubeconfig for the cluster"
  value       = digitalocean_kubernetes_cluster.courtside.kube_config[0].raw_config
  sensitive   = true
}
