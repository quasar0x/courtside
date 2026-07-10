# Courtside — Data Residency & Compliance

_Owner: Platform Engineering · Status: US region live (nyc3); EU (fra1) and CA (tor1) provisioned-ready pending node-capacity increase._

## 1. Purpose & scope

Courtside is a sports-club membership SaaS serving members in the **United States, Canada, and the European Union**. This document describes how the platform keeps personal data inside each member's legal jurisdiction and maps the technical controls to the applicable privacy regimes: GDPR (EU), PIPEDA (Canada), and US state privacy law (CCPA/CPRA). Brazil / LGPD is explicitly **out of scope** — see section 8.

## 2. Architecture at a glance

Courtside runs **one independent Kubernetes cluster per jurisdiction** on DigitalOcean Kubernetes (DOKS), each paired with an **in-region managed PostgreSQL** database. A single Kubernetes cluster cannot span regions, so residency is achieved with separate regional clusters, not one global cluster. Members are routed to their jurisdiction's cluster at the edge; personal data is written only to that region's database and never crosses a regional boundary.

Every region is identical, provisioned from the same reusable Terraform module and the same GitOps base, differing only in region-specific values (region slug, database endpoint, ingress host). The desired state of the entire estate lives in Git, so it is reproducible and auditable.

Members are geo-routed to their region (edge routing is Slice C, roadmap):

    US (nyc3) : DOKS + Istio  ->  managed Postgres (private VPC)
    EU (fra1) : DOKS + Istio  ->  managed Postgres (private VPC)
    CA (tor1) : DOKS + Istio  ->  managed Postgres (private VPC)

## 3. Jurisdiction, region, and regulation

| Jurisdiction | DO region | Primary regulation(s) | Managed database |
|---|---|---|---|
| United States | nyc3 (New York) | CCPA / CPRA + state privacy laws | courtside-us-pg |
| European Union | fra1 (Frankfurt) | GDPR | courtside-eu-pg |
| Canada | tor1 (Toronto) | PIPEDA | courtside-ca-pg |
| Brazil | none (no DO region) | LGPD | out of scope (section 8) |

## 4. How residency is enforced

- **Per-region managed database.** Each cluster's PostgreSQL lives in the same DO region; personal data is physically stored in-jurisdiction.
- **Private networking.** The database is attached to the region's VPC and exposes no public endpoint; the app tier reaches it over the private network only.
- **Least-trust database firewall.** A k8s-type trusted-source rule permits only that region's DOKS cluster to connect — not the internet, not other clusters, not the operator's laptop.
- **No cross-region PII.** Inter-service calls occur only within a region; no service reads another region's database.
- **Edge geo-routing (roadmap, Slice C).** Members are steered to their jurisdiction's cluster by geography.

## 5. Security controls mapped to compliance

| Control | Implementation | GDPR | PIPEDA | CCPA/CPRA |
|---|---|---|---|---|
| Encryption in transit | Istio STRICT mTLS between services; TLS-required (sslmode=require) to managed DB; edge HTTPS (roadmap) | Art. 32 | Principle 7 | 1798.150 |
| Encryption at rest | DO managed PostgreSQL encryption; DO Block Storage volumes | Art. 32 | P7 | 1798.150 |
| Access control | Private DB + firewall trusting only the cluster; least-privilege RBAC; NetworkPolicy default-deny (roadmap) | Art. 32 | P7 | 1798.150 |
| Data residency | Cluster + managed DB per jurisdiction; edge geo-routing (roadmap) | Ch. V | P1 | — |
| Secrets management | No plaintext secrets in Git; DB credentials injected from Terraform state; Vault + ESO (roadmap) | Art. 32 | P7 | 1798.150 |
| Supply-chain integrity | CI build, keyless Cosign signing (OIDC/Fulcio/Rekor), SBOM (SPDX) attestation, push to ghcr; Kyverno verification (roadmap) | Art. 32 | P7 | — |
| Change management & audit | Terraform IaC with stateful/stateless lifecycle split; GitOps via Argo CD; full Git history | Art. 5(2) | P1 | — |
| Breach detection / audit logging | Observability stack (Prometheus/Grafana/Loki/Tempo) — deferred pending capacity | Art. 33 | P7 | — |

## 6. Data subject rights

Because personal data is partitioned by region, access (GDPR Art. 15) and erasure/deletion (GDPR Art. 17, PIPEDA, CCPA) requests execute against exactly one regional database. A member's data exists in a single jurisdiction, which simplifies both fulfilling the request and proving completeness.

## 7. Change management & auditability

- **Terraform** — modules/region-data (VPC + managed Postgres, long-lived) and modules/region-cluster (DOKS + node pool, ephemeral), called from environments/{us,eu,ca}/{data,cluster}. The stateful data layer has its own state so clusters can be rebuilt without risking data.
- **GitOps** — an Argo CD app-of-apps per region; every change is a reviewed, revertible commit; selfHeal + prune keep live state equal to Git.
- **CI** — matrix build across services, keyless sign, SBOM, attest, publish, so every running image traces to a signed commit.

## 8. Known gaps & roadmap

- **Observability** (metrics/logs/traces) deferred pending the node-capacity increase; re-attempt on a cluster with headroom.
- **Secrets** — DB credentials are injected from Terraform state today (not plaintext in Git, but not yet centrally managed). Roadmap: Vault + ESO with rotation.
- **Admission policy** — Kyverno image-signature verification to be enabled on DOKS.
- **NetworkPolicy default-deny** — currently mesh mTLS + DB firewall provide isolation; add explicit per-region default-deny.
- **Edge TLS + geo-routing** — Cloudflare (Slice C) not yet wired; ingress is presently plain HTTP on the LB IP.
- **HA control plane** — DOKS HA add-on is off (cost); enable for production.
- **Brazil / LGPD** — DigitalOcean has no South America region, so in-country residency is not achievable on DO. If required: a multi-cloud Sao Paulo cluster, or LGPD-compliant international-transfer safeguards. Out of scope for now.

## 9. Architecture defense (talking points)

- "Residency is real, not simulated: one cluster and one managed database per jurisdiction, and data never leaves its region."
- "Stateless compute is cattle, stateful data is a pet — I split the Terraform state so I can rebuild clusters freely without touching the database."
- "Managed Postgres mandates TLS — a clean example of a cloud constraint tightening security versus local dev, where the service had sslmode=disable hardcoded."
- "DOKS is managed control plane plus self-managed node pools (GKE-Standard / EKS tier). Karpenter is AWS/Azure-only, so node scaling here is the cluster-autoscaler via pool min/max."
- "Everything is GitOps — the desired state of every cluster is auditable in Git history, which is my accountability control."
