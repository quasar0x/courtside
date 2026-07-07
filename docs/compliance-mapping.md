# Courtside — Compliance & Controls Mapping

Maps the platform's technical controls to the frameworks Courtside operates under:
**GDPR** (EU), **PIPEDA** (Canada), **CCPA** (California), **LGPD** (Brazil),
**POPIA** (South Africa), and **SOC 2 Type II**.

## Control inventory

| Control | Implementation | Purpose | Frameworks |
|---|---|---|---|
| Encryption in transit (mTLS everywhere) | Istio `PeerAuthentication: STRICT` + sidecars | All service-to-service traffic mutually authenticated & encrypted | GDPR Art.32; SOC2 CC6.1/CC6.7; all |
| Secrets management | Vault (KV v2, sealed, Kubernetes auth) + External Secrets Operator | Secrets encrypted at rest; no plaintext in Git; least-privilege access | GDPR Art.32; SOC2 CC6.1 |
| Secrets audit trail | Vault audit device (values HMAC-hashed) → Loki | Every secret access logged, tamper-evident, centralized | SOC2 CC7.2/CC7.3; GDPR Art.30 |
| Credential rotation | Vault versioned KV + ESO re-sync + rolling restart | Rotate credentials with zero downtime | SOC2 CC6.1 |
| Least-privilege RBAC | Namespaced Roles/RoleBindings; scoped ServiceAccounts | Access limited strictly to need | GDPR Art.32; SOC2 CC6.3 |
| Data residency isolation | Region node labels + `nodeAffinity` + Cilium NetworkPolicy | EU personal data confined to EU infra; cross-region access denied | GDPR Ch.V; LGPD; POPIA; PIPEDA; CCPA |
| Network segmentation (zero-trust east-west) | Cilium (eBPF) NetworkPolicies | Deny-by-default pod-to-pod | SOC2 CC6.6; GDPR Art.32 |
| Supply-chain integrity | Cosign image signing + SBOM attestation | Only known-provenance images; tamper-evident contents | SOC2 CC7.1/CC8.1 |
| Admission enforcement | Kyverno: verify signatures, disallow `:latest`, Pod Security | Non-compliant workloads rejected at admission | SOC2 CC7.1/CC8.1 |
| Workload hardening | runAsNonRoot, drop ALL caps, readOnlyRootFilesystem, seccomp RuntimeDefault | Minimize blast radius of a compromise | SOC2 CC6.1 |
| Change management / auditability | GitOps (ArgoCD App-of-Apps); all state in Git | Every change reviewed, versioned, reversible; no manual prod access | SOC2 CC8.1; GDPR Art.32 |
| Observability & incident detection | Prometheus, Grafana, Loki, Tempo + Alertmanager | Detect, investigate, and evidence operational events | SOC2 CC7.2/CC7.3 |

## Data residency architecture

- Regions modeled as node pools (`region=eu-west`, `region=us-east`). In production: **separate EKS clusters per region**, GitOps-managed per region.
- Personal data workloads pinned to their region via `nodeAffinity` + regional namespaces.
- Cross-region access denied by NetworkPolicy — **verified**: a US-namespace pod cannot reach EU data (connection dropped by Cilium).
- Data-subject rights (access/erasure) are serviced within the owning region.

## SOC 2 Type II — Trust Services Criteria (summary)

- **CC6 (Logical & physical access):** RBAC, Vault, mTLS, NetworkPolicies, PSS.
- **CC7 (System operations):** observability stack, alerting, Vault + change audit logs.
- **CC8 (Change management):** GitOps with review/rollback, signed images, admission control.

## Per-framework notes

- **GDPR:** Art.32 (security of processing — encryption, access control, resilience); Ch.V (transfers — residency isolation); Art.30 (records — audit logs).
- **PIPEDA (Canada):** Safeguards principle — encryption, access controls, breach detection.
- **CCPA (California):** Reasonable security; data segregation supports access/deletion requests.
- **LGPD (Brazil):** Art.46 security measures; residency for Brazilian personal data.
- **POPIA (South Africa):** S19 security safeguards; S72 cross-border transfer conditions.

## Honest gaps / production roadmap

- Real per-region clusters (not simulated node pools) with multi-cluster mesh.
- Vault **dynamic** database secrets (ephemeral creds) instead of static KV.
- **Keyless** image signing (OIDC + Rekor transparency log) in CI.
- Registry **allowlist** admission policy to reject unapproved third-party images.
- HA Vault with auto-unseal (cloud KMS); HA across all control-plane components.
