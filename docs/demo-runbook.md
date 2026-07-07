# Courtside — Live Demo Runbook (PROVE / RUN / SAY)

Rehearse this top to bottom. Each step: what it PROVES, what to RUN, what to SAY.
Target: ~12-15 minutes end to end. Speak the SAY lines; don't just show output.

## Step 0 — Setup (before the call)
Open 4 terminal tabs and start these port-forwards (leave running):
  T1: kubectl -n argocd     port-forward svc/argocd-server 8080:443
  T2: kubectl -n monitoring port-forward svc/monitoring-grafana 3000:80
  T3: kubectl -n istio-system port-forward svc/istio-ingressgateway 8090:80
  (keep a 5th tab free for commands)
Open browser tabs: ArgoCD (https://localhost:8080), Grafana (http://localhost:3000),
  GitHub Actions, GitHub repo.
If the cluster was restarted: unseal Vault ->
  VAULT_ROOT_TOKEN=$(python3 -c "import json;print(json.load(open('/tmp/vault-init.json'))['root_token'])")
  KEY=$(python3 -c "import json;print(json.load(open('/tmp/vault-init.json'))['unseal_keys_b64'][0])")
  kubectl -n vault exec vault-0 -- vault operator unseal "$KEY"
Warm the dashboards: send a little traffic (Step 3's loop) so panels aren't empty.

## Step 1 — The foundation (eBPF cluster)
PROVE: 3-node K8s with a modern eBPF dataplane, not defaults.
RUN:  kubectl get nodes -o wide
      kubectl -n kube-system get pods | grep -E 'cilium|hubble'
SAY:  "1 control-plane, 2 workers. I replaced the default CNI with Cilium — eBPF instead of
       iptables: faster at scale, L3-L7 network policy, and Hubble flow observability. The
       workers double as my two regions later."

## Step 2 — GitOps is the control plane (ArgoCD)
PROVE: everything is declarative; Git is the only way to change prod; it self-heals.
RUN:  (ArgoCD UI) show root app -> children all Synced/Healthy. Click one, show the tree.
      Then drift test:
      kubectl -n courtside scale deploy/members --replicas=3
      watch it revert -> kubectl -n courtside get deploy members -w   (Ctrl-C after it returns to 1)
SAY:  "App-of-Apps: one root manages every service and the infra, from a single reusable Helm
       chart plus per-service values. selfHeal means a manual change is reverted within a
       reconcile cycle — nobody hand-edits production; the only path is a commit."

## Step 3 — The app: sync + async (the system works)
PROVE: real microservices, both request/response and event-driven.
RUN:  curl -s localhost:8090/clubs/1 | jq         # sync: clubs enriches from members
      curl -s -X POST localhost:8090/memberships -d '{"member_id":"1","club_id":"1","plan":"premium"}'
      sleep 4
      curl -s localhost:8090/invoices | jq         # async: billing reacted via Kafka
      curl -s localhost:8090/notifications | jq     # async: notifications reacted too
SAY:  "One entry point through the Istio gateway. clubs calls members synchronously. Creating a
       membership publishes to Kafka; billing and notifications react independently — one event,
       two consumers. The honest caveat: DB write + publish is a dual-write; production uses a
       transactional outbox."

## Step 4 — Zero-trust mTLS (the security floor)
PROVE: unauthenticated traffic is refused; encryption is enforced, not optional.
RUN:  kubectl -n default run mtls-test --image=curlimages/curl:8.10.1 --restart=Never --rm -i \
        --command -- sh -c 'curl -sS -m 5 http://members.courtside:8080/healthz; echo " exit=$?"'
SAY:  "That pod has no sidecar, so no identity. STRICT mTLS makes members' proxy reset the
       connection. In-mesh callers with a valid certificate get through. mTLS everywhere,
       enforced at the network layer, zero app code."

## Step 5 — Observability: metrics, logs, traces, alerting
PROVE: full golden-signal + trace + log visibility, mostly free from the mesh.
RUN:  (Grafana) Istio Mesh dashboard (rate/success/latency per service).
      Explore -> Loki: {namespace="courtside"} | json | level="ERROR"
      Explore -> Tempo: Search service=clubs, span=GET /clubs/{id}; open a trace -> clubs->members waterfall.
      Alerting: mention CourtsideHigh4xxRate (or trip it with a /clubs/999 loop).
SAY:  "Golden signals per service come from the Istio sidecars — no instrumentation. Logs are
       structured JSON, queryable by field. Traces are linked across services because I propagate
       context. Alerts fire on SLO breach before users notice."

## Step 6 — Secrets done right (Vault, SOC2)
PROVE: secrets encrypted + audited + rotatable, no static creds, nothing plaintext in Git.
RUN:  VAULT_ROOT_TOKEN=$(python3 -c "import json;print(json.load(open('/tmp/vault-init.json'))['root_token'])")
      kubectl -n vault exec vault-0 -- sh -c "VAULT_TOKEN=$VAULT_ROOT_TOKEN vault kv get secret/courtside/postgres"
      kubectl -n vault logs vault-0 --tail=3 | grep response   # values are hmac-sha256:...
SAY:  "Vault: sealed, KV v2, Kubernetes auth so pods authenticate by ServiceAccount — no static
       tokens. Every access is audited with values HMAC-hashed, so the log proves who read what
       without leaking it. External Secrets Operator materializes the secret; I rotated a live
       credential end to end with a full audit trail. Roadmap: Vault dynamic DB secrets."

## Step 7 — Supply chain: only signed images run
PROVE: admission control rejects unsigned images; signed ones pass.
RUN:  kubectl -n courtside run bad --image=kind-registry:5000/courtside/unsigned:0.1.0 --restart=Never   # REJECTED
      kubectl -n courtside run good --image=kind-registry:5000/courtside/members:0.4.0 --restart=Never   # ADMITTED
      kubectl -n courtside delete pod good --ignore-not-found
SAY:  "Cosign signs images; syft attaches an SBOM. Kyverno verifies at admission — unsigned is
       blocked, signed is admitted. In CI it's keyless via GitHub OIDC + Rekor. Insecure-registry
       is on only because my local registry is HTTP; against a TLS registry it stays off."

## Step 8 — Multi-region residency + least-privilege RBAC
PROVE: EU data can't be reached from US; RBAC grants exactly what's needed.
RUN:  kubectl -n residency-us run xus --image=curlimages/curl:8.10.1 --restart=Never --rm -i \
        --command -- sh -c 'curl -s -m 5 -o /dev/null -w "%{http_code}\n" http://eu-data.residency-eu.svc || echo BLOCKED'
      SA=system:serviceaccount:courtside:courtside-viewer
      for x in "list pods" "get secrets" "delete pods"; do echo "$x -> $(kubectl auth can-i $x -n courtside --as=$SA)"; done
SAY:  "Workloads are pinned to region nodes; a Cilium NetworkPolicy denies cross-region — a US
       pod cannot reach EU data. RBAC is least-privilege: this viewer can list pods but not read
       secrets or delete anything. On EKS these become per-region clusters, same label-driven policies."

## Step 9 — CI/CD: signed, automated, no manual prod access
PROVE: the delivery pipeline is automated and supply-chain-secure.
RUN:  (GitHub Actions) show the green 'ci' run; open build-sign-attest(members) -> Keyless sign
      step -> point at 'tlog entry created' (Rekor).
SAY:  "Every merge builds, signs keyless (no key to leak), generates an SBOM, and pushes by
       digest. A final job bumps the GitOps repo and ArgoCD deploys — so the only route to prod
       is merge -> CI -> ArgoCD. No human runs kubectl against production."

## Closer (say this)
"Built locally on kind to iterate fast and prove no vendor lock-in; every pattern lifts to EKS
unchanged. I deliberately kept honest gaps — real per-region clusters, dynamic secrets, managing
Istio/Vault via ArgoCD — because knowing the delta from demo to production is the point."
