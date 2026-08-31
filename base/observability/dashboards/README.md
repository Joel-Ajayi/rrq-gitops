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

### 1. `tier1-business-transactions.yaml` (`/executive`)
* **Target Audience**: Executive Leadership, Product Owners, Incident Commanders.
* **Methodology**: High-Level Golden Signals & Business Invariants.
* **Key Panels**:
  * **Gross Transaction Value (GTV)** (`business_gtv_total`): Gross settlement value per currency.
  * **Total Processed Transfers** (`business_transfers_total`): Successful, failed, and pending transfers.
  * **Transfer Success Rate**: Percentage of completed transfers vs total requests.
  * **Fraud / Velocity Declines** (`fraud_checks_total`): Transactions blocked by rate-limit and velocity guards.
  * **Merchant Balance Movements & Deposits**: Deposit and transfer velocity per wallet.
  * **Disputes & Refunds**: Chargebacks and refund rates.
  * **Cross-Shard Clearing Sagas**: Initiated vs completed 2-phase clearing sagas.

### 2. `tier2-user-journeys.yaml` (`/journeys`)
* **Target Audience**: Backend Engineers, Systems Architects.
* **Methodology**: Asynchronous Data Flow & State Progression.
* **Key Panels**:
  * **Cross-Shard Saga State**: Pending vs committed vs compensated clearing transactions.
  * **Dead Letter Queue (DLQ) Churn**: Ingested poison pills vs manually replayed events (`admin_dlq_replayed_total`).
  * **Webhook Delivery vs Bulkhead Rejections**: Inflight worker requests vs rate-limiter rejections.
  * **Idempotency Conflicts & Replays**: Upstream duplicate request rates.
  * **Consumer Retry Backoff**: Exponential backoff duration and retry budget consumption.

### 3. `tier3-service-health.yaml` (`/services`)
* **Target Audience**: On-Call Engineers, Application Developers.
* **Methodology**: **RED Method** (**R**ate, **E**rrors, **D**uration) + Service Compute Bounds.
* **Features**: Dynamic `$service_name` dropdown variable to inspect any RRQ microservice.
* **Key Panels**:
  * **API / Kafka Throughput**: Request rate per span and per Kafka topic.
  * **Error Rate**: Failed request rate (`STATUS_CODE_ERROR`).
  * **Latency Distribution**: P50, P90, and P99 latency percentiles.
  * **Circuit Breakers**: State indicator (Closed=0, Half-Open=1, Open=2) and trip rates.
  * **DB Pool Starvation**: Worker wait events for PostgreSQL connection acquisition.
  * **Runtime Health**: Go Goroutine count and Pod CPU/Memory utilization.

### 4. `tier4-middleware-data.yaml` (`/middleware`)
* **Target Audience**: Database Administrators (DBA), Data Platform SREs.
* **Methodology**: **USE Method** (**U**tilization, **S**aturation, **E**rrors) for Stateful Middleware.
* **Key Panels**:
  * **PostgreSQL Shards** (`merchants`, `shard-a`, `shard-b`):
    * *Cache Hit Ratio*: Buffer pool efficiency (`blks_hit / (blks_hit + blks_read)`).
    * *Dead Tuples*: Table bloat monitoring for autovacuum health.
    * *Connection Pool Saturation*: Acquired connections vs max ceiling.
    * *Query Latency & Write Spans*: Traced SQL execution duration and mutation ratios.
  * **Kafka (Strimzi)**:
    * *Under-Replicated Partitions (URP)*: Broker replication health.
    * *Consumer Partition Lag*: Group lag across `jobs`, `notify`, and `xshard.*`.
    * *Outbox Relay AIMD Backpressure*: Producer buffer fill ratio vs publishing rate.
  * **Redis**:
    * *Memory Fragmentation Ratio*: Allocation efficiency indicator.
    * *Evicted Keys Rate*: Volatile key drop monitoring under memory limits.

### 5. `tier5-compute-infrastructure.yaml` (`/infrastructure`)
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

---

## 🔗 Coupling with Capacity Modeling (`slo-input.yaml`)

Every metric query in these dashboards maps 1:1 to the empirical baseline inputs declared in `rrq-gitops/tools/capacity-engine/slo-input.yaml`. When tuning capacity models or updating SLO headroom parameters, operators should source live baseline numbers directly from these panels.
