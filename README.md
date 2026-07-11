# Courtside — Cloud-Native Membership Platform

A production-style GitOps platform for a sports-club / membership SaaS, running on **DigitalOcean Kubernetes (DOKS)**.

## Architecture

- **5 Go microservices** (members, clubs, memberships, billing, notifications) — `net/http`, pgx, Kafka, Redis; distroless non-root images.
- **DOKS** — managed control plane + 3-node pool, provisioned by **Terraform** (reusable region module; stateful data state split from the ephemeral cluster state).
- **Managed PostgreSQL** per region — private VPC, TLS-only, firewall trusting only the cluster; database-per-service.
- **Istio** service mesh — STRICT mutual TLS; ingress via a DigitalOcean Load Balancer.
- **Argo CD (GitOps)** — app-of-apps per environment; desired state in Git, self-heal + prune.
- **Observability** — OpenTelemetry Collector → Tempo (traces) + Loki (logs); Prometheus + Grafana (metrics + Istio golden signals).
- **Multi-region ready** — US live; EU (Frankfurt) and Canada (Toronto) are configuration replicas, with per-region data residency (GDPR / PIPEDA / CCPA).

## DevSecOps pipeline

1. **PR gates** — gofmt/vet, unit tests, **Semgrep** (SAST), **govulncheck** + **Trivy** (dependency/image scanning). Branch protection blocks failing merges.
2. **On merge to `main`** — build → **Trivy** image scan → **Cosign** keyless sign → **SBOM** attest → push to ghcr → auto-promote to **staging**.
3. **Staging** — isolated namespace + databases; **OWASP ZAP** DAST baseline.
4. **Production** — **human-approved** promotion (GitHub Environment gate); Argo CD rolls prod.

## Layout

- `services/` — Go microservices + Dockerfiles
- `infra/terraform/` — DOKS + managed Postgres (modules + environments)
- `gitops/` — Argo CD apps, base Helm chart, per-region/-env config & values
- `observability/` — Prometheus/Loki/Tempo/OTel-Collector values + manifests
- `platform/` — Vault, ESO, Kyverno, Cosign
- `docs/` — residency & compliance, observability architecture
