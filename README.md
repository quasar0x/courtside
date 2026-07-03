# Courtside — Cloud-Native Membership Platform (practice build)

Greenfield GitOps Kubernetes platform for a sports-club / membership SaaS.

Stack: Go microservices on kind, Cilium (eBPF CNI) + Istio (mTLS mesh),
ArgoCD (GitOps), Vault (secrets + audit), Kafka + Redis, Prometheus/Grafana/
Loki/Tempo (observability), Cosign/SBOM/Kyverno (supply chain),
multi-region data residency. Terraform + EKS migration path.