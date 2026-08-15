# RRQ Observability Stack

This directory contains the complete observability platform for the **RRQ (River Rust Queue)** payment processing core: OpenTelemetry collection, Prometheus metrics, Grafana dashboards, centralized logging, and distributed tracing — all deployed declaratively via GitOps (Argo CD, Wave 1).

---

## Architecture

<style>
  .diagram-container svg { min-width: 1000px !important; }
</style>
<div class="diagram-container" style="overflow: auto; max-height: 80vh;">

```mermaid
%%{init: {"theme": "base", "themeVariables": {"fontSize": "14px", "fontFamily": "Inter, sans-serif", "lineColor": "#5b6472", "edgeLabelBackground": "#ffffff"}, "flowchart": {"useMaxWidth": true, "nodeSpacing": 40, "rankSpacing": 50}}}%%
flowchart LR
    classDef app fill:#0d3b66,stroke:#0a2e4d,color:#ffffff,font-weight:bold
    classDef instr fill:#5e548e,stroke:#4b4176,color:#ffffff,font-weight:bold
    classDef agent fill:#006d77,stroke:#00535b,color:#ffffff,font-weight:bold
    classDef gateway fill:#1b6ca8,stroke:#155a8c,color:#ffffff,font-weight:bold
    classDef backend fill:#8f3e00,stroke:#7a3500,color:#ffffff,font-weight:bold
    classDef channel fill:#6d6875,stroke:#5a5662,color:#ffffff,font-weight:bold

    subgraph Apps["RRQ Workloads - namespace: rrq"]
        API["core-api"]
        LDG["ledger-worker"]
        OBR["outbox-relay"]
        WHK["webhook-worker"]
        FRD["fraud-worker"]
    end

    subgraph INSTR["OTel Operator - observability"]
        AUTO["Instrumentation - eBPF Go Auto-Instrumentation"]
    end

    subgraph AGENT["OTel Agent - DaemonSet"]
        OTLP_A["OTLP Receiver - 4317 / 4318"]
        FILELOG["Filelog Receiver - /var/log/pods"]
        HOSTM["Hostmetrics + Kubelet Stats"]
    end

    subgraph GATEWAY["OTel Gateway - Deployment"]
        OTLP_G["OTLP Receiver"]
        KAFKA_M["Kafka Metrics Receiver - rrq-kafka-bootstrap:9092"]
        PROC["Processors - memory_limiter / k8sattributes / batch"]
        SPAN_METRICS["span_metrics Connector"]
    end

    subgraph BACKENDS["Backends"]
        JAEGER["Jaeger - Query UI :16686"]
        ELASTIC["Elasticsearch (ECK) - 8.17.0"]
        KIBANA["Kibana"]
        PROM["Prometheus - kube-prometheus-stack"]
        GRAFANA["Grafana - 5-Tier Dashboards"]
        ALERT["Alertmanager"]
    end

    subgraph CHANNELS["Alerting"]
        SLACK["Slack - critical / warning"]
    end

    class API,LDG,OBR,WHK,FRD app
    class AUTO instr
    class OTLP_A,FILELOG,HOSTM agent
    class OTLP_G,KAFKA_M,PROC,SPAN_METRICS gateway
    class JAEGER,ELASTIC,KIBANA,PROM,GRAFANA,ALERT backend
    class SLACK channel

    API --> OTLP_A
    LDG --> OTLP_A
    OBR --> OTLP_A
    WHK --> OTLP_A
    FRD --> OTLP_A
    AUTO -.->|inject agent| API
    AUTO -.->|inject agent| LDG
    AUTO -.->|inject agent| OBR
    AUTO -.->|inject agent| WHK
    AUTO -.->|inject agent| FRD

    FILELOG -->|logs| OTLP_A
    HOSTM -->|metrics| OTLP_A

    OTLP_A -->|traces / metrics / logs| OTLP_G
    KAFKA_M -->|broker metrics| OTLP_G
    OTLP_G --> PROC --> SPAN_METRICS

    SPAN_METRICS -->|RED metrics| PROM
    PROC -->|traces| JAEGER
    JAEGER -->|trace index| ELASTIC
    PROC -->|logs| ELASTIC
    ELASTIC --> KIBANA
    PROM -->|scrape :8889| SPAN_METRICS
    PROM --> GRAFANA
    GRAFANA -->|dashboards via sidecar| PROM

    PROM -.->|PrometheusRule| ALERT
    ALERT -.->|AlertmanagerConfig| SLACK
```

</div>
### Data Flow Summary

| Signal | Collection | Pipeline | Destination |
|--------|-----------|----------|-------------|
| **Traces** | OTel Agent (`otlp` receiver) receives from auto-instrumented workloads | Agent → Gateway → `otlp/jaeger` | Jaeger (Elasticsearch-backed storage) |
| **Metrics** | Workload spans, `hostmetrics`, `kubelet_stats`, `kafka_metrics` | Agent → Gateway → `span_metrics` connector → `prometheus` exporter (`:8889`) | Prometheus → Grafana |
| **Logs** | `filelog` receiver tails `/var/log/pods/*/*/*.log` | Agent → Gateway → `elasticsearch` exporter | Elasticsearch (`rrq-logs-%{+yyyy.MM.dd}`) → Kibana |
| **Alerts** | `PrometheusRule` (`rrq-critical-alerts`) | Alertmanager groups by `[alertname, severity]` | Slack (`slack-critical`, `slack-warning`) |

---

## Components

| Manifest | Kind | Purpose |
|----------|------|---------|
| `namespace.yaml` | `Namespace` | `observability` namespace isolation |
| `agent.yaml` | `OpenTelemetryCollector` (DaemonSet) | Node-level collection: OTLP, filelog, hostmetrics, kubelet stats → forwards to gateway |
| `agent-rbac.yaml` | `Role` / `ClusterRole` | Permissions for agent collectors (kubelet stats, pod metadata) |
| `agent-rbac.yaml` | `ServiceAccount` | `agent` service account |
| `gateway.yaml` | `OpenTelemetryCollector` (Deployment) | Central pipeline: `span_metrics` connector, `k8sattributes`, fan-out to Jaeger / Prometheus / Elasticsearch |
| `gateway-rbac.yaml` | `ServiceAccount` | `gateway` service account |
| `instrumentation.yaml` | `Instrumentation` | OTel Operator eBPF auto-instrumentation for Go binaries (gRPC `4317`) |
| `servicemonitor.yaml` | `ServiceMonitor` | Prometheus scrapes gateway `:8889` / `/metrics` every 15s |
| `prometheusrule.yaml` | `PrometheusRule` | Critical SLO alerts (CrashLoopBackOff, 5xx rate, etc.) |
| `alertmanagerconfig.yaml` | `AlertmanagerConfig` | Routing: critical → `slack-critical`, warning → `slack-warning`, critical inhibits warnings |
| `jaeger.yaml` | `OpenTelemetryCollector` | Jaeger UI (`:16686`) + OTLP receiver, ES-backed |
| `elastic.yaml` | `Elasticsearch` / `Kibana` (ECK) | Log storage (2 nodes) and Kibana UI |
| `ops-routes.yaml` | `HTTPRoute` | Exposes observability UIs via Envoy Gateway (Grafana, Kibana, Jaeger) |

---

## Dashboards

Grafana dashboards follow a **Persona-Driven 5-Tier Taxonomy** (RED + USE methods) and are shipped as labeled `ConfigMap`s auto-imported by the Grafana sidecar.

- [`dashboards/README.md`](dashboards/README.md) — Full tier breakdown: Business SLOs, User Journeys, Service Health (RED), Middleware & Data (USE), Compute & Infrastructure (USE).

## GitOps

- Deployed as part of **Wave 1** by `apps/01-observability.yaml`.
- Dashboards reference the capacity engine's `slo-input.yaml` baselines 1:1.