# Courtside — Observability Architecture

Modern OpenTelemetry-centric stack, entirely GitOps-managed (Argo CD apps in the `monitoring` namespace).

## Signal flow

- **Traces:** service (OTel SDK + otelhttp) -> OTLP -> `otel-collector:4318` -> k8sattributes -> **Tempo**.
- **Logs:** service stdout (slog JSON) -> OTel Collector `filelog` receiver -> k8sattributes -> **Loki** (OTLP).
- **Metrics:** **Prometheus** scrapes Istio sidecars (PodMonitor `envoy-stats`), istiod (ServiceMonitor), and cluster/k8s targets.
- **Visualization:** **Grafana** with Prometheus (default), Tempo, and Loki datasources.

## Components (pinned)

| Component | Chart | Version | Role |
|---|---|---|---|
| kube-prometheus-stack | prometheus-community | 87.12.2 | Prometheus, Alertmanager, Grafana, node-exporter, kube-state-metrics |
| Tempo | grafana | 1.24.4 | Traces backend (OTLP 4317/4318) |
| Loki | grafana | 6.52.0 | Logs backend (single-binary, filesystem) |
| OpenTelemetry Collector | open-telemetry | 0.164.1 | DaemonSet OTLP ingest + log collection, fan-out to Tempo/Loki |

## Access

Open http://localhost:3000 (admin / admin).

- Traces: Explore -> Tempo -> Service Name = `members`.
- Logs: Explore -> Loki -> `{k8s_namespace_name="courtside"}`.
- Metrics: Dashboards -> Kubernetes / Compute Resources.

## Alerts

`CourtsideHigh4xxRate` (PrometheusRule) — fires when Istio 4xx rate > 0.2 req/s for 1m.

## Config in repo

- `observability/values/*.yaml` — Helm values (kube-prometheus, tempo, loki, otel-collector).
- `observability/manifests/*.yaml` — Istio monitors, alert rule, Loki/Tempo datasources.
- `gitops/regions/us/apps/*.yaml` — the Argo CD Application for each component.