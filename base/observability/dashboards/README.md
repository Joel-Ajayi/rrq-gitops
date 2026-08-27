# RRQ Observability Dashboards

This directory contains the production Grafana dashboard definitions for the **River Rust Queue (RRQ)** infrastructure, packaged as Kubernetes `ConfigMap` resources for automated GitOps deployment via ArgoCD.

---

## 🏛️ Architecture & Taxonomy

Rather than relying on tool-centric or monolithic dashboards, RRQ observability follows a **Persona-Driven 5-Tier Taxonomy** built on standard SRE methodologies (**RED Method** for microservices, **USE Method** for infrastructure and data stores).

```
                      ┌─────────────────────────────────────────┐
                      │  Tier 1: Business & SLOs                │
                      │  (Executive & Product)                  │
                      └────────────────────┬────────────────────┘
                                           │
                      ┌────────────────────▼────────────────────┐
                      │  Tier 2: User Journeys & Flows          │
                      │  (Architects & Backend Lead)            │
                      └────────────────────┬────────────────────┘
                                           │
                      ┌────────────────────▼────────────────────┐
                      │  Tier 3: Service Health (RED)           │
                      │  (On-Call Engineers)                    │
                      └──────────┬───────────────────┬──────────┘
                                 │                   │
      ┌──────────────────────────▼───┐           ┌───▼──────────────────────────┐
      │ Tier 4: Middleware & Data    │           │ Tier 5: Compute & Infra      │
      │ (DBA & Platform SRE)         │           │ (System Admin & K8s SRE)     │
      └──────────────────────────────┘           └──────────────────────────────┘
```

---

## 📋 Dashboard Directory Breakdown

### 1. `tier1-business-slos.yaml`
* **Target Audience**: Executive Leadership, Product Owners, Incident Commanders.
* **Methodology**: High-Level Golden Signals & Business Invariants.
* **Key Panels**:
  * **Total Processed Transfers** (`business_transfers_total`): Total transfers breakdown by status (success/failed/pending) emitted directly by `ledger-worker`.
  * **Gross Transaction Value (GTV)** (`business_gtv_total`): Actively recorded business throughput metric from `ledger-worker`.
  * **Fraud / Velocity Rejections**: Rate of transfers blocked by rate-limit and velocity guards.
  * **Transfer Success Rate**: Ratio of successful transfers vs total HTTP ingress requests.
  * **Active Merchants**: Operational lookup traffic proxy.

### 2. `tier2-user-journeys.yaml`
* **Target Audience**: Backend Engineers, Systems Architects.
* **Methodology**: Asynchronous Data Flow & State Progression.
* **Key Panels**:
  * **Saga Unresolved Count** (`saga_unresolved_count`): Pending cross-shard clearing sagas.
  * **Dead Letter Queue (DLQ) Churn**: Ingested poison pills vs manually replayed events.
  * **Webhook Delivery vs Rejections**: Inflight worker requests vs rate-limiter rejections.
  * **Idempotency Conflicts**: Upstream duplicate request rates.

### 3. `tier3-service-health.yaml`
* **Target Audience**: On-Call Engineers, Application Developers.
* **Methodology**: **RED Method** (**R**ate, **E**rrors, **D**uration) + Service Compute Bounds.
* **Features**: Dynamic `$service_name` dropdown variable to inspect any RRQ microservice.
* **Key Panels**:
  * **API / Kafka Throughput**: Request rate per span and per Kafka topic.
  * **Error Rate**: Failed request rate (`STATUS_CODE_ERROR`).
  * **Latency Distribution**: P50 and P99 latency percentiles.
  * **Circuit Breakers**: State indicator (Closed=0, Half-Open=1, Open=2).
  * **DB Pool Starvation**: Worker wait events for PostgreSQL connection acquisition.
  * **Runtime Health**: Go Goroutine count and Pod CPU/Memory utilization.

### 4. `tier4-middleware-data.yaml`
* **Target Audience**: Database Administrators (DBA), Data Platform SREs.
* **Methodology**: **USE Method** (**U**tilization, **S**aturation, **E**rrors) for Stateful Middleware.
* **Key Panels**:
  * **PostgreSQL**:
    * *Cache Hit Ratio*: Buffer pool efficiency (`blks_hit / (blks_hit + blks_read)`).
    * *Dead Tuples*: Table bloat monitoring for autovacuum health.
    * *TXID Wraparound Age*: Maximum transaction age (`pg_database_age`).
    * *Query Latency & Write Spans*: Traced SQL execution duration and mutation ratios.
  * **Kafka (Strimzi)**:
    * *Under-Replicated Partitions (URP)*: Broker replication health.
    * *Active Controllers*: Split-brain detection metric.
    * *Outbox AIMD Backpressure*: Producer buffer fill ratio vs publishing rate.
  * **Redis**:
    * *Memory Fragmentation Ratio*: Allocation efficiency indicator.
    * *Evicted Keys Rate*: Volatile key drop monitoring under memory limits.

### 5. `tier5-compute-infrastructure.yaml`
* **Target Audience**: Kubernetes SREs, Systems Administrators.
* **Methodology**: **USE Method** for Compute & Storage Infrastructure.
* **Key Panels**:
  * **Pod Restarts**: Container CrashLoopBackOff detection.
  * **PVC Saturation**: Storage volume capacity utilization.
  * **Node Pressure**: Node-level CPU, Memory, and Disk pressure conditions.
  * **Network Transmit Errors**: Interface drop rates.
  * **Capacity Modeling Controls**:
    * *Compute Saturation Curve*: CPU cores vs overall RPS.
    * *Variance Parameters ($C_s^2 / C_a^2$)*: Service time variance over mean duration squared.

---

## ⚙️ GitOps & Deployment

Dashboards are automatically imported into Grafana via the **Grafana Sidecar** container shipped with `kube-prometheus-stack`.

### Label Discovery
Every `ConfigMap` in this directory is labeled with:
```yaml
metadata:
  labels:
    grafana_dashboard: "1"
```
The Grafana sidecar watches for ConfigMaps with `grafana_dashboard: "1"` in the `observability` namespace and automatically mounts them into Grafana.

### Formatting Requirements
To ensure Grafana parses the embedded JSON properly inside Kubernetes ConfigMaps, the JSON must be formatted using YAML block scalar style (`|-`):
```yaml
data:
  rrq-tier1-business-slos.json: |-
    {
      "title": "Tier 1: Business & SLOs",
      ...
    }
```
*Note: Utility scripts in `rrq-gitops/capacity/reformat_dashboards.py` ensure all dashboard YAML files maintain this formatting.*

---

## 🔗 Coupling with Capacity Modeling (`slo-input.yaml`)

Every metric query in these dashboards maps 1:1 to the empirical baseline inputs declared in `rrq-gitops/capacity/slo-input.yaml`. When tuning capacity models or updating SLO headroom parameters, operators should source live baseline numbers directly from these panels.
