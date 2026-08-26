# Production Environment Capacity & Resource Specification

## 1. Overview

This document specifies the enterprise-scale **Production High-Availability (HA)** capacity model for `overlays/prod`. The architecture is designed to handle sustained multi-thousand TPS transaction workloads across 3 availability zones (AZs) / 3 Kubernetes worker nodes with full N+1 / N+2 redundancy, asynchronous replication, and exhaustive distributed observability.

---

## 2. Resource Allocations by Subsystem

### 2.1 Datastores Subsystem (`scale-up.yaml`)

| Component                    | Architecture / Instances                   | CPU Req (Pod / Total)                                                             | CPU Lim (Pod / Total)                                                              | Mem Req (Pod / Total)                                                                | Mem Lim (Pod / Total)                                                                 | Storage (Per Pod / Total)           | HA / Resilience Strategy                             |
| :--------------------------- | :----------------------------------------- | :-------------------------------------------------------------------------------- | :--------------------------------------------------------------------------------- | :----------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------ | :---------------------------------- | :--------------------------------------------------- |
| **Postgres: `merchants-db`** | 1 Primary + 2 Standbys (3 pods)            | 4.0 CPU / 12.0 CPU                                                                | 8.0 CPU / 24.0 CPU                                                                 | 8.0 GiB / 24.0 GiB                                                                   | 16.0 GiB / 48.0 GiB                                                                   | 5 GiB / 15 GiB                      | CNPG synchronous replication, auto-failover          |
| **Postgres: `shard-a`**      | 1 Primary + 2 Standbys (3 pods)            | 4.0 CPU / 12.0 CPU                                                                | 8.0 CPU / 24.0 CPU                                                                 | 8.0 GiB / 24.0 GiB                                                                   | 16.0 GiB / 48.0 GiB                                                                   | 50 GiB / 150 GiB                    | Ledger shard A; 2GB `shared_buffers`, 239 conns      |
| **Postgres: `shard-b`**      | 1 Primary + 2 Standbys (3 pods)            | 4.0 CPU / 12.0 CPU                                                                | 8.0 CPU / 24.0 CPU                                                                 | 8.0 GiB / 24.0 GiB                                                                   | 16.0 GiB / 48.0 GiB                                                                   | 50 GiB / 150 GiB                    | Ledger shard B; 2GB `shared_buffers`, 239 conns      |
| **Kafka (`rrq-kafka`)**      | KRaft mode, 2 brokers (2 pods)             | 2.0 CPU / 4.0 CPU                                                                 | 4.0 CPU / 8.0 CPU                                                                  | 4.0 GiB / 8.0 GiB                                                                    | 8.0 GiB / 16.0 GiB                                                                    | 62 GiB / 124 GiB                    | RF=2, min.isr=2; `required` inter-node anti-affinity |
| **Kafka Entity Operator**    | Topic + User operators (1 pod)             | 200m / 0.2 CPU                                                                    | 500m / 0.5 CPU                                                                     | 384 MiB / 384 MiB                                                                    | 768 MiB / 768 MiB                                                                     | —                                   | Managed CRD synchronization                          |
| **Redis (`fraud-redis`)**    | Master + 2 Replicas + 3 Sentinels (6 pods) | Master: 2.0 CPU<br>Replicas: 2×2.0 CPU<br>Sentinels: 3×100m<br>**Total: 6.3 CPU** | Master: 4.0 CPU<br>Replicas: 2×4.0 CPU<br>Sentinels: 3×300m<br>**Total: 12.9 CPU** | Master: 2.0 GiB<br>Replicas: 2×2.0 GiB<br>Sentinels: 3×128 MiB<br>**Total: 6.4 GiB** | Master: 4.0 GiB<br>Replicas: 2×4.0 GiB<br>Sentinels: 3×256 MiB<br>**Total: 12.8 GiB** | In-memory with Sentinel quorum of 2 | Velocity cache & token bucket rate limiting          |
| **Datastores Subtotal**      | **18 Pods**                                | **~46.5 CPU**                                                                     | **~93.4 CPU**                                                                      | **~86.8 GiB**                                                                        | **~173.6 GiB**                                                                        | **~439 GiB PVC**                    |                                                      |

---

### 2.2 Operators & Platform Subsystem

| Component                                  | Pods        | CPU Request  | CPU Limit    | Memory Request | Memory Limit | Purpose                                    |
| :----------------------------------------- | :---------- | :----------- | :----------- | :------------- | :----------- | :----------------------------------------- |
| **CloudNativePG Operator**                 | 1           | 100m         | 500m         | 256 MiB        | 512 MiB      | PostgreSQL lifecycle controller            |
| **Strimzi Kafka Operator**                 | 1           | 200m         | 500m         | 384 MiB        | 768 MiB      | Kafka KRaft & topic reconciliation         |
| **Envoy Gateway Controller**               | 2           | 200m         | 1.0 CPU      | 256 MiB        | 1.0 GiB      | High-throughput edge proxy & routing       |
| **KEDA (Operator + Metrics Server)**       | 2           | 200m         | 500m         | 256 MiB        | 512 MiB      | Kafka consumer lag autoscaling             |
| **cert-manager (Controller, Webhook, CA)** | 3           | 150m         | 500m         | 384 MiB        | 768 MiB      | Automated ACME Let's Encrypt TLS issuance  |
| **ECK Operator**                           | 1           | 100m         | 500m         | 256 MiB        | 512 MiB      | Elasticsearch cluster management           |
| **Prometheus Operator**                    | 1           | 100m         | 500m         | 256 MiB        | 512 MiB      | Prometheus & ServiceMonitor reconciliation |
| **OpenTelemetry Operator**                 | 1           | 100m         | 500m         | 256 MiB        | 512 MiB      | Workload auto-instrumentation injection    |
| **Sealed Secrets Controller**              | 1           | 50m          | 200m         | 64 MiB         | 256 MiB      | Production secret decryption               |
| **Operators Subtotal**                     | **13 Pods** | **~1.2 CPU** | **~4.7 CPU** | **~2.3 GiB**   | **~5.3 GiB** |                                            |

---

### 2.3 Observability Subsystem (`patches/elastic-storage.yaml` & `prometheus-storage.yaml`)

| Component                  | Pods          | CPU Req (Total) | CPU Lim (Total) | Mem Req (Total)     | Mem Lim (Total)     | Storage            | Notes                                  |
| :------------------------- | :------------ | :-------------- | :-------------- | :------------------ | :------------------ | :----------------- | :------------------------------------- |
| **Prometheus**             | 2             | 500m            | 2.0 CPU         | 1.0 GiB (2×512 MiB) | 4.0 GiB             | 100 GiB (2×50 GiB) | Multi-day metrics retention            |
| **Grafana**                | 1             | 100m            | 500m            | 256 MiB             | 512 MiB             | —                  | Production dashboards                  |
| **Alertmanager**           | 1             | 50m             | 250m            | 128 MiB             | 256 MiB             | —                  | PagerDuty / OpsGenie integration       |
| **OTel Collector Agent**   | 3 (DaemonSet) | 300m (3×100m)   | 1.5 CPU         | 384 MiB (3×128 MiB) | 1.5 GiB             | —                  | Node hostmetrics & pod log parser      |
| **OTel Collector Gateway** | 1             | 100m            | 500m            | 256 MiB             | 1.0 GiB             | —                  | Span metrics generation & log pipeline |
| **Elasticsearch**          | 2             | 500m (2×250m)   | 2.0 CPU (2×1.0) | 1.5 GiB (2×768 MiB) | 3.0 GiB (2×1.5 GiB) | 100 GiB (2×50 GiB) | Scaled clustered log indexing          |
| **Kibana**                 | 1             | 100m            | 500m            | 256 MiB             | 1.0 GiB             | —                  | Central log analytics UI               |
| **Jaeger**                 | 1             | 100m            | 500m            | 256 MiB             | 512 MiB             | —                  | Trace query frontend & storage router  |
| **Portainer**              | 1             | 50m             | 250m            | 128 MiB             | 256 MiB             | —                  | Cluster inspection dashboard           |
| **Observability Subtotal** | **~13 Pods**  | **~1.8 CPU**    | **~8.0 CPU**    | **~4.2 GiB**        | **~12.0 GiB**       | **200 GiB PVC**    |                                        |

---

### 2.4 Application Workloads Subsystem

_Workloads scale dynamically via HPA and KEDA. Baseline allocations represent minimum steady-state capacity:_

| Service                | Min-Max Replicas | Baseline CPU Req   | Peak CPU Lim        | Baseline Mem Req   | Peak Mem Lim       | Anti-Affinity       | Scaling Trigger              |
| :--------------------- | :--------------- | :----------------- | :------------------ | :----------------- | :----------------- | :------------------ | :--------------------------- |
| **`core-api`**         | 2 – 8 (HPA)      | 3.42 CPU (2×1712m) | 13.70 CPU (8×1712m) | 128 MiB (2×64 MiB) | 512 MiB (8×64 MiB) | `required` hostname | CPU (70%), Memory (80%)      |
| **`outbox-relay`**     | 2 – 8 (KEDA)     | 534m (2×267m)      | 2.14 CPU (8×267m)   | 148 MiB (2×74 MiB) | 592 MiB (8×74 MiB) | `required` hostname | Kafka outbox consumer lag    |
| **`ledger-worker`**    | 2 – 8 (KEDA)     | 330m (2×165m)      | 1.32 CPU (8×165m)   | 134 MiB (2×67 MiB) | 536 MiB (8×67 MiB) | `required` hostname | Kafka `jobs` partition lag   |
| **`webhook-worker`**   | 2 – 8 (KEDA)     | 102m (2×51m)       | 8.00 CPU (8×1000m)  | 144 MiB (2×72 MiB) | 576 MiB (8×72 MiB) | `required` hostname | Kafka `notify` partition lag |
| **`fraud-worker`**     | 2 – 8 (KEDA)     | 42m (2×21m)        | 8.00 CPU (8×1000m)  | 130 MiB (2×65 MiB) | 520 MiB (8×65 MiB) | `required` hostname | Kafka `xshard` partition lag |
| **`webhook-echo`**     | 1                | 50m                | 200m                | 64 MiB             | 128 MiB            | —                   | Test harness mock receiver   |
| **Workloads Subtotal** | **11 – 41 Pods** | **~4.48 CPU**      | **~33.36 CPU**      | **~0.75 GiB**      | **~2.86 GiB**      |                     |                              |

---

### 2.5 Argo CD & Control Plane

| Component                                               | Pods         | CPU Req      | CPU Lim      | Mem Req      | Mem Lim      | Purpose                             |
| :------------------------------------------------------ | :----------- | :----------- | :----------- | :----------- | :----------- | :---------------------------------- |
| **Argo CD** (server, repo, controller, redis, dex)      | ~5           | 0.9 CPU      | 2.0 CPU      | 1.2 GiB      | 2.5 GiB      | Automated App-of-Apps sync          |
| **Kubernetes Overhead** (apiserver, etcd, coredns, CNI) | ~14          | 1.5 CPU      | 3.0 CPU      | 3.0 GiB      | 4.5 GiB      | Managed / Self-hosted control plane |
| **Control Plane Subtotal**                              | **~19 Pods** | **~2.4 CPU** | **~5.0 CPU** | **~4.2 GiB** | **~7.0 GiB** |                                     |

---

## 3. Total Cluster Aggregate Capacity

| Category                     | Pod Count (Base → Peak) | CPU Requests (Steady-State) | CPU Limits (Peak Capacity) | Memory Requests | Memory Limits   | PVC Storage Required |
| :--------------------------- | :---------------------- | :-------------------------- | :------------------------- | :-------------- | :-------------- | :------------------- |
| **Datastores**               | 18                      | 46.50 CPU                   | 93.40 CPU                  | 86.80 GiB       | 173.60 GiB      | 439 GiB              |
| **Operators & Platform**     | 13                      | 1.20 CPU                    | 4.70 CPU                   | 2.30 GiB        | 5.30 GiB        | —                    |
| **Observability**            | 13                      | 1.80 CPU                    | 8.00 CPU                   | 4.20 GiB        | 12.00 GiB       | 200 GiB              |
| **Workloads**                | 11 → 41                 | 4.48 CPU                    | 33.36 CPU                  | 0.75 GiB        | 2.86 GiB        | —                    |
| **Argo CD & System**         | 19                      | 2.40 CPU                    | 5.00 CPU                   | 4.20 GiB        | 7.00 GiB        | —                    |
| **TOTAL PRODUCTION CLUSTER** | **~74 → 104 Pods**      | **~56.38 CPU**              | **~144.46 CPU**            | **~98.25 GiB**  | **~200.76 GiB** | **~639 GiB**         |

---

## 4. Minimum Node Capacity (3 Worker Nodes Setup)

To deploy the production environment reliably across a **3-Node Worker Cluster** (with managed control plane like AWS EKS, GCP GKE, or Azure AKS):

### 4.1 Per-Worker Node Resource Distribution

Pods are distributed across the 3 worker nodes using strict topology spread constraints and `requiredDuringSchedulingIgnoredDuringExecution` anti-affinity:

| Metric                 | Baseline Scheduled per Worker | Peak / Autoscaled Workload per Worker | Minimum Schedulable Capacity per Node | Recommended Cloud Instance Spec per Node |
| :--------------------- | :---------------------------- | :------------------------------------ | :------------------------------------ | :--------------------------------------- |
| **vCPU (Cores)**       | ~18.8 CPU requests            | ~48.0 CPU peak limits                 | **24 vCPUs**                          | **32 vCPUs**                             |
| **Memory (RAM)**       | ~32.8 GiB requests            | ~67.0 GiB limits ceiling              | **48 GiB - 64 GiB RAM**               | **64 GiB - 128 GiB RAM**                 |
| **Persistent Storage** | ~213 GiB PVC volume claims    | Log buffers, Docker image cache       | **300 GB SSD (gp3 / NVMe)**           | **500 GB+ SSD / io2**                    |
| **Network Throughput** | ~5 Gbps inter-service         | Burst transaction sync                | **10 Gbps Network**                   | **12.5 - 25 Gbps Network**               |
| **Max Pods**           | ~25 - 35 pods per node        | Peak burst: ~50 pods                  | **60 Pods limit**                     | **110+ Pods limit**                      |

### 4.2 Recommended Cloud Instance Sizing (3-Node Worker Cluster)

```
┌────────────────────────────────────────────────────────────────────────┐
│             PRODUCTION 3-NODE WORKER CLUSTER SIZING                   │
├────────────────────────────────────────────────────────────────────────┤
│ • Worker Node Count    : 3 Dedicated Worker Nodes (Spread across 3 AZs)│
│ • Minimum Spec / Node  : 24 vCPUs | 64 GiB RAM  | 300 GB NVMe Storage  │
│ • Recommended Spec/Node: 32 vCPUs | 128 GiB RAM | 500 GB NVMe Storage  │
│ • Total Cluster Pool   : 72-96 vCPUs | 192-384 GiB RAM | ~1.5 TB NVMe  │
│                                                                        │
│ Cloud Provider Mappings (Recommended Instance Families):               │
│ • AWS EKS   : 3x m6i.8xlarge (32 vCPU, 128 GiB RAM, 12.5 Gbps)        │
│               or 3x c6i.8xlarge (32 vCPU, 64 GiB RAM)                  │
│ • GCP GKE   : 3x n2-standard-32 (32 vCPU, 128 GiB RAM, 16 Gbps)       │
│ • Azure AKS : 3x Standard_D32s_v5 (32 vCPU, 128 GiB RAM)               │
└────────────────────────────────────────────────────────────────────────┘
```
