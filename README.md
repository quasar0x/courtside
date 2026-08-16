# Courtside — Cloud-Native Membership Platform

A production-style GitOps platform for a sports-club / membership SaaS, running on **DigitalOcean Kubernetes (DOKS)**.

## Architecture

- **5 Go microservices** (members, clubs, memberships, billing, notifications) — `net/http`, pgx, Kafka, Redis; multi-stage **distroless non-root** images on **Go 1.26**. Both synchronous (HTTP) and asynchronous (Kafka event) flows.
- **DOKS** — managed control plane + 3-node pool, provisioned by **Terraform** (reusable region module; stateful `data` state split from the ephemeral `cluster` state, so compute can be torn down to ~$0 while the database persists).
- **Managed PostgreSQL** per region — private VPC, TLS-only, firewall trusting only the cluster; database-per-service.
- **Kafka + Redis** — event backbone (a membership event fans out to billing + notifications) plus idempotency/cache.
- **Istio** service mesh — STRICT mutual TLS; ingress via a DigitalOcean Load Balancer.
- **Argo CD (GitOps)** — app-of-apps per environment; desired state in Git, self-heal + prune.
- **Kyverno** — admission control **enforcing** that only Cosign-signed images may run in the cluster.
- **Observability** — OpenTelemetry Collector → Tempo (traces) + Loki (logs); Prometheus + Grafana (metrics + Istio golden signals).
- **Multi-region ready** — US live; EU (Frankfurt) and Canada (Toronto) are configuration replicas, with per-region data residency (GDPR / PIPEDA / CCPA).

## DevSecOps pipeline

1. **PR gates** (`pr-checks`) — gofmt/vet, unit tests, **Semgrep** (SAST), **govulncheck** + **Trivy** (dependency scanning). Branch protection blocks failing merges.
2. **On merge to `main`** (`ci`) — build → **Trivy** image scan → **Cosign** keyless sign (GitHub OIDC → Fulcio → Rekor) → **SBOM** attest → push to ghcr → auto-promote to **staging**.
3. **Staging validation** — isolated namespace + databases; an **e2e smoke test** (`e2e-staging`, exercising the real sync + async signup flow) and an **OWASP ZAP** DAST baseline.
4. **Production** (`promote-prod`) — **e2e-gated** and **human-approved** (GitHub Environment gate); Argo CD rolls prod.
5. **Admission control** — Kyverno verifies Cosign signatures at deploy time; **unsigned images are rejected**.
6. **Dependencies** — Dependabot opens weekly update PRs, gated by the same checks.

Supply-chain loop: **sign in CI → verify at admission → unsigned refused.**

## Layout

- `services/` — Go microservices + Dockerfiles
- `infra/terraform/` — DOKS + managed Postgres (modules + environments)
- `gitops/` — Argo CD apps, base Helm chart, per-region/-env config & values
- `tests/e2e/` — end-to-end smoke test (staging validation + prod promotion gate)
- `observability/` — Prometheus/Loki/Tempo/OTel-Collector values + manifests
- `platform/` — Kyverno image-signature policies (enforced); Vault + ESO (roadmap)
- `docs/` — residency & compliance, observability architecture
