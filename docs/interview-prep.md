# Courtside Platform — Architecture Defense & Interview Prep

## 0. The 30-second pitch
A cloud-native, GitOps-driven Kubernetes platform for a sports-club/membership SaaS.
Five Go microservices (sync + event-driven) on a 3-node cluster with an eBPF dataplane,
zero-trust mTLS mesh, full observability, Vault-backed secrets with audit + rotation,
signed-image supply chain with admission control, multi-region data-residency isolation,
and a keyless-signing CI/CD pipeline. Built locally on kind, designed to lift to EKS unchanged.

## 1. Architecture, layer by layer (what + WHY + tradeoff)

### Cluster & networking
- **kind, 1 control-plane + 2 workers.** Workers stand in for regions later.
- **Cilium (eBPF) CNI + Hubble**, not the default kindnet. WHY: eBPF is faster than
  iptables at scale, gives L3-L7 NetworkPolicy, kube-proxy replacement capability, and
  Hubble flow visibility. It's also the JD's "eBPF-based networking/observability" nice-to-have.
- TRADEOFF: heavier than kindnet; on 16GB RAM that mattered (I ran lean everywhere).

### Application (5 services)
- members, clubs (synchronous REST); memberships (event producer); billing, notifications
  (event consumers). **Database-per-service** (one Postgres instance, a DB per service) —
  logical isolation without paying for five instances on a laptop.
- **Kafka** for async events (`membership.created` fan-out), **Redis** for consumer idempotency.
- WHY Go: tiny static binaries -> distroless images ~10-27MB, minimal CVE surface.
- Hardening: distroless nonroot, runAsUser 65532, drop ALL caps, readOnlyRootFilesystem,
  seccomp RuntimeDefault (Pod Security "restricted").
- KEY TALKING POINT — the dual-write problem: producer writes DB then publishes to Kafka;
  if the publish fails they diverge. Production fix = transactional outbox. (I hit this live
  when a disk-full event dropped a Kafka publish and an invoice never generated.)

### GitOps (ArgoCD)
- **App-of-Apps**: one root Application manages child Applications (5 services + infra).
- **One reusable Helm chart + per-service values files** — DRY; the differences are pure config.
- `automated: {prune, selfHeal}` — Git is the only way to change the cluster; hand-edits revert.
- WHY ArgoCD over Flux: mature UI for visualizing sync/health, App-of-Apps pattern, strong
  RBAC/SSO story, and it's what I've run in production. Both are valid; I lead with ArgoCD.

### Service mesh (Istio)
- **STRICT mTLS** (PeerAuthentication) — every service-to-service call mutually authenticated
  and encrypted, proven by a non-mesh pod getting connection-reset.
- Ingress gateway + VirtualService (path routing) + DestinationRule (circuit breaker).
- **Datastores excluded from the mesh** (sidecar.istio.io/inject=false). WHY: Kafka's
  advertised-listener protocol and StatefulSet sidecar ordering are fragile; Istio's automatic
  mTLS detects the missing sidecar and uses plaintext to them, so nothing breaks. Defensible.
- Cilium (CNI) + Istio (mesh) chosen deliberately: Cilium for eBPF networking, Istio for
  L7 mTLS/traffic mgmt. Could also do Cilium-only mesh (ambient) — I can compare.

### Observability
- Metrics: kube-prometheus-stack. **Golden signals per service come free from the Istio
  sidecars** (rate/errors/latency) — no app instrumentation needed.
- Logs: Loki + Promtail; structured JSON logs queryable by field.
- Traces: Tempo + OpenTelemetry. `clubs -> members` shows as one linked trace because I
  propagate trace context (otelhttp inject/extract). Async Kafka tracing is the next extension
  (inject context into message headers) — I can describe it.
- Alerting: PrometheusRule -> Alertmanager (watched a 4xx-rate alert go inactive->pending->firing).

### Secrets (Vault)
- Vault: seal/unseal (Shamir), KV v2, **audit device** (values HMAC-hashed, shipped to Loki),
  **Kubernetes auth** (pods authenticate by ServiceAccount, no static tokens), least-privilege
  policy. External Secrets Operator syncs Vault -> a K8s Secret the apps already consume.
- **Live rotation demo**: changed the password in Postgres + Vault, ESO re-synced, rolling
  restart, apps reconnected — zero code change, full audit trail.
- HONEST GAP: Postgres's own bootstrap secret is still static; production = Vault dynamic
  database secrets (ephemeral per-connection creds, no static password anywhere).

### Supply chain
- Cosign signs images (local: key-based; CI: keyless via GitHub OIDC + Fulcio + Rekor).
- Syft SBOM attached as a signed attestation.
- **Kyverno admission control**: verifyImages rejects unsigned images (proven: unsigned pod
  blocked, signed pod admitted); disallow-latest; Pod Security.
- KNOWN nuance: verifyImages is scoped to my registry; third-party images (nginx) bypass it —
  production adds a registry allowlist. Also `--allowInsecureRegistry` is only on because the
  LOCAL registry is HTTP; against a TLS registry it stays false.

### Multi-region & RBAC
- Workers labeled region=eu-west / us-east; workloads pinned via nodeAffinity; **Cilium
  NetworkPolicy enforces residency** (US pod -> EU data = blocked; EU -> EU = 200).
- Least-privilege RBAC: a viewer SA can list pods but NOT read secrets, delete, or reach
  other namespaces (proven with `kubectl auth can-i`).
- On real EKS: separate clusters per region, GitOps-per-region, same label-driven policies.

### CI/CD
- GitHub Actions matrix: build -> push ghcr -> **keyless sign** -> SBOM attest, per service.
- Zero manual prod access: merge -> CI signs -> CI bumps GitOps values -> ArgoCD deploys.

## 2. Recruiter application answers (the six asks)
- **Timezone/overlap:** UTC+1; morning-early-afternoon overlaps Yerevan (UTC+4), late-afternoon-
  evening overlaps Vancouver Island (UTC-8).
- **CNCF certs:** Kubestronaut — CKA, CKS, CKAD, KCNA, KCSA (Credly badges provided).
- **Production platform:** [real] EKS payments platform at Ivory Pay — ArgoCD App-of-Apps,
  OIDC keyless deploys, IRSA, Cosign/SBOM across 12+ services, SOC2/PCI alignment. [demo]
  Courtside shows the same patterns end to end.
- **Multi-region residency:** region-pinned workloads + network-policy isolation + per-region
  GitOps; data-subject rights serviced in-region. (Walk the Courtside demo.)
- **GitOps tooling:** ArgoCD — App-of-Apps, self-heal, strong visualization/RBAC; comfortable
  with Flux, chose ArgoCD for the UI and multi-tenant story.
- **Secrets in SOC2 context:** Vault sealed + audited (hashed values, tamper-evident), K8s-auth
  (no static creds), least-privilege policies, rotation without downtime, ESO to avoid plaintext
  in Git; roadmap to dynamic secrets.

## 3. Mock assessment (scenario Q&A)
Q: Design multi-region cluster topology with data residency.
A: Cluster-per-region (data plane in-region), GitOps repo per region (or overlays), region
   labels + affinity, deny-by-default NetworkPolicies for cross-region, mesh mTLS for any
   permitted cross-region calls, per-region observability + audit stores. Data-subject requests
   routed to the owning region. Avoid a global control plane touching regional data.

Q: How do secrets work end to end for SOC2?
A: Vault as source of truth (encrypted at rest, sealed). Pods auth via K8s ServiceAccount
   (short-lived tokens). Least-privilege policies per workload. ESO (or Vault Agent) materializes
   secrets; nothing plaintext in Git. Audit device logs every access (values hashed) to a
   central store. Rotation via versioned KV + rollout, or dynamic secrets for zero standing creds.

Q: How does a change reach production safely?
A: PR -> review -> merge. CI builds, scans, signs (keyless), generates SBOM, pushes by digest,
   and bumps the GitOps repo. ArgoCD reconciles; self-heal prevents drift; automated rollback on
   failed health. No human has kubectl on prod. Every step is in Git = auditable.

Q: How do you guarantee only trusted images run?
A: Sign in CI (keyless OIDC), verify at admission (Kyverno verifyImages against the workflow
   identity), pin by digest, registry allowlist, Pod Security restricted, SBOM for provenance.

Q: Something's slow in production — how do you find it?
A: Golden-signal dashboards (rate/errors/latency per service from the mesh) to localize;
   distributed traces (Tempo) to find the slow hop; logs (Loki) for the error detail;
   correlate via trace/span IDs. Alerts fire on SLO breach before users notice.

## 4. Live-demo runbook (what to show, in order)
1. `kubectl get nodes` + Hubble — the eBPF cluster.
2. ArgoCD UI — App-of-Apps synced/healthy; change replicas in Git -> self-heal.
3. Ingress gateway `/clubs/1` enriched (sync) + create a membership -> invoice+notification (async).
4. Non-mesh pod -> members = connection reset (STRICT mTLS).
5. Grafana: Istio golden signals; Loki logs; Tempo clubs->members trace; fire the alert.
6. Vault: audit log (hashed) + rotate a credential live.
7. Kyverno: unsigned pod rejected, signed pod admitted.
8. Residency: US pod -> EU data blocked; RBAC `auth can-i` boundaries.
9. GitHub Actions: green pipeline, keyless sign + Rekor entry.

## 5. Honest production deltas (say these; they show judgment)
- Real per-region clusters (not simulated node pools); HA everywhere; Vault auto-unseal via KMS.
- Vault dynamic DB secrets; transactional outbox for events; registry allowlist.
- Manage Istio/observability/Vault via ArgoCD (some were installed imperatively for speed).
- Async trace propagation through Kafka; progressive delivery (canary/argo-rollouts).
