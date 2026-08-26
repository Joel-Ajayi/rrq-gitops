# Local Production Environment Capacity & Resource Specification

## 1. Overview

This environment (`overlays/local`) mirrors the full **Production High-Availability (HA)** architecture, platform tooling, and observability stack, while right-sizing resource allocations to run reliably on a single developer workstation (e.g., 20 threads / 25 GiB RAM) using a 4-node Kind cluster (1 control-plane + 3 worker nodes).

---

## 2. Resource Allocations by Subsystem

### 2.1 Datastores Subsystem

| Component                    | Architecture / Instances                  | CPU Req (Pod / Total)                                                    | CPU Lim (Pod / Total)                                                    | Mem Req (Pod / Total)                                                             | Mem Lim (Pod / Total)                                                             | Storage (Per Pod / Total)               | HA / Anti-Affinity Strategy                      |
| :--------------------------- | :---------------------------------------- | :----------------------------------------------------------------------- | :----------------------------------------------------------------------- | :-------------------------------------------------------------------------------- | :-------------------------------------------------------------------------------- | :-------------------------------------- | :----------------------------------------------- |
| **Postgres: `merchants-db`** | Primary + 1 Standby (2 pods)              | 500m / 1.0 CPU                                                           | 1500m / 3.0 CPU                                                          | 512 MiB / 1.0 GiB                                                                 | 1.0 GiB / 2.0 GiB                                                                 | 2 GiB / 4 GiB                           | CNPG automated failover; `preferred` host spread |
| **Postgres: `shard-a`**      | Primary + 1 Standby (2 pods)              | 500m / 1.0 CPU                                                           | 1500m / 3.0 CPU                                                          | 512 MiB / 1.0 GiB                                                                 | 1.0 GiB / 2.0 GiB                                                                 | 10 GiB / 20 GiB                         | CNPG automated failover; `preferred` host spread |
| **Postgres: `shard-b`**      | Primary + 1 Standby (2 pods)              | 500m / 1.0 CPU                                                           | 1500m / 3.0 CPU                                                          | 512 MiB / 1.0 GiB                                                                 | 1.0 GiB / 2.0 GiB                                                                 | 10 GiB / 20 GiB                         | CNPG automated failover; `preferred` host spread |
| **Kafka (`rrq-kafka`)**      | KRaft mode, 2 brokers (2 pods)            | 1000m / 2.0 CPU                                                          | 2000m / 4.0 CPU                                                          | 1.0 GiB / 2.0 GiB                                                                 | 2.0 GiB / 4.0 GiB                                                                 | 10 GiB / 20 GiB                         | RF=2, min.isr=2; `preferred` host spread         |
| **Kafka Entity Operator**    | Topic + User operators (1 pod)            | 200m / 0.2 CPU                                                           | 500m / 0.5 CPU                                                           | 384 MiB / 384 MiB                                                                 | 768 MiB / 768 MiB                                                                 | —                                       | Managed CRD synchronization                      |
| **Redis (`fraud-redis`)**    | Master + 1 Replica + 3 Sentinels (5 pods) | Master: 250m<br>Replica: 250m<br>Sentinels: 3×100m<br>**Total: 0.8 CPU** | Master: 500m<br>Replica: 500m<br>Sentinels: 3×200m<br>**Total: 1.6 CPU** | Master: 256 MiB<br>Replica: 256 MiB<br>Sentinels: 3×128 MiB<br>**Total: 0.9 GiB** | Master: 512 MiB<br>Replica: 512 MiB<br>Sentinels: 3×256 MiB<br>**Total: 1.8 GiB** | In-memory (`save ""` / `appendonly no`) | Sentinel quorum of 2; auto-failover              |
| **Datastores Subtotal**      | **14 Pods**                               | **~6.0 CPU**                                                             | **~15.1 CPU**                                                            | **~6.3 GiB**                                                                      | **~12.6 GiB**                                                                     | **~64 GiB PVC**                         |                                                  |

---

### 2.2 Operators & Platform Subsystem

| Component                                  | Pods        | CPU Request  | CPU Limit    | Memory Request | Memory Limit | Purpose                                |
| :----------------------------------------- | :---------- | :----------- | :----------- | :------------- | :----------- | :------------------------------------- |
| **CloudNativePG Operator**                 | 1           | 100m         | 500m         | 256 MiB        | 512 MiB      | PostgreSQL cluster controller          |
| **Strimzi Kafka Operator**                 | 1           | 200m         | 500m         | 384 MiB        | 768 MiB      | Kafka cluster & topic lifecycle        |
| **Envoy Gateway Controller**               | 1           | 100m         | 500m         | 128 MiB        | 512 MiB      | Kubernetes Gateway API implementation  |
| **KEDA (Operator + Metrics)**              | 2           | 200m         | 500m         | 256 MiB        | 512 MiB      | Event-driven autoscaling controller    |
| **cert-manager (Controller, Webhook, CA)** | 3           | 150m         | 500m         | 384 MiB        | 768 MiB      | Automated TLS certificate provisioning |
| **ECK Operator**                           | 1           | 100m         | 500m         | 256 MiB        | 512 MiB      | Elasticsearch cluster operator         |
| **Prometheus Operator**                    | 1           | 100m         | 500m         | 256 MiB        | 512 MiB      | Prometheus/Alertmanager lifecycle      |
| **OpenTelemetry Operator**                 | 1           | 100m         | 500m         | 256 MiB        | 512 MiB      | OTel collector & auto-instrumentation  |
| **Sealed Secrets Controller**              | 1           | 50m          | 200m         | 64 MiB         | 256 MiB      | Asymmetric secret decryption           |
| **Operators Subtotal**                     | **12 Pods** | **~1.1 CPU** | **~4.2 CPU** | **~2.2 GiB**   | **~4.8 GiB** |                                        |

---

### 2.3 Observability Subsystem

| Component                  | Pods          | CPU Req (Total)  | CPU Lim (Total) | Mem Req (Total)     | Mem Lim (Total) | Storage           | Notes                                    |
| :------------------------- | :------------ | :--------------- | :-------------- | :------------------ | :-------------- | :---------------- | :--------------------------------------- |
| **Prometheus**             | 2             | 500m             | 1.0 CPU         | 1.0 GiB (2×512 MiB) | 2.0 GiB         | 20 GiB (2×10 GiB) | HA pair with 24h retention               |
| **Grafana**                | 1             | 200m             | 500m            | 256 MiB             | 512 MiB         | —                 | Pre-loaded operational dashboards        |
| **Alertmanager**           | 1             | 100m             | 250m            | 128 MiB             | 256 MiB         | —                 | Routing & alerting rules                 |
| **OTel Collector Agent**   | 4 (DaemonSet) | 800m (4×200m)    | 2.0 CPU         | 512 MiB (4×128 MiB) | 2.0 GiB         | —                 | Node hostmetrics & pod log parser        |
| **OTel Collector Gateway** | 1             | 200m             | 500m            | 256 MiB             | 1.0 GiB         | —                 | Span metrics generation & log pipeline   |
| **Elasticsearch**          | 2             | 1.0 CPU (2×500m) | 2.0 CPU         | 1.0 GiB (2×512 MiB) | 2.0 GiB         | 20 GiB (2×10 GiB) | 2-node cluster for log aggregation       |
| **Kibana**                 | 1             | 200m             | 500m            | 128 MiB             | 512 MiB         | —                 | UI for logs search & visual analysis     |
| **Jaeger**                 | 1             | 200m             | 500m            | 256 MiB             | 512 MiB         | —                 | Distributed trace backend & query UI     |
| **Portainer**              | 1             | 100m             | 250m            | 128 MiB             | 256 MiB         | —                 | Container & cluster management dashboard |
| **Observability Subtotal** | **~14 Pods**  | **~3.3 CPU**     | **~7.5 CPU**    | **~3.7 GiB**        | **~9.0 GiB**    | **40 GiB PVC**    |                                          |

---

### 2.4 Application Workloads Subsystem

| Service                | Replicas    | CPU Req (Total)   | CPU Lim (Total)    | Mem Req (Total)     | Mem Lim (Total)     | Anti-Affinity        | Scaling Strategy                 |
| :--------------------- | :---------- | :---------------- | :----------------- | :------------------ | :------------------ | :------------------- | :------------------------------- |
| **`core-api`**         | 2           | 2.0 CPU (2×1000m) | 3.42 CPU (2×1712m) | 256 MiB (2×128 MiB) | 512 MiB (2×256 MiB) | `preferred` hostname | Fixed 2 reps (HPA min=2, max=2)  |
| **`outbox-relay`**     | 2           | 400m (2×200m)     | 1.0 CPU (2×500m)   | 256 MiB (2×128 MiB) | 512 MiB (2×256 MiB) | `preferred` hostname | Fixed 2 reps (KEDA min=2, max=2) |
| **`ledger-worker`**    | 2           | 300m (2×150m)     | 1.0 CPU (2×500m)   | 256 MiB (2×128 MiB) | 512 MiB (2×256 MiB) | `preferred` hostname | Fixed 2 reps (KEDA min=2, max=2) |
| **`webhook-worker`**   | 2           | 100m (2×50m)      | 1.0 CPU (2×500m)   | 256 MiB (2×128 MiB) | 512 MiB (2×256 MiB) | `preferred` hostname | Fixed 2 reps (KEDA min=2, max=2) |
| **`fraud-worker`**     | 2           | 100m (2×50m)      | 1.0 CPU (2×500m)   | 256 MiB (2×128 MiB) | 512 MiB (2×256 MiB) | `preferred` hostname | Fixed 2 reps (KEDA min=2, max=2) |
| **`webhook-echo`**     | 1           | 50m               | 200m               | 64 MiB              | 128 MiB             | —                    | Test harness mock receiver       |
| **Workloads Subtotal** | **11 Pods** | **~2.95 CPU**     | **~7.62 CPU**      | **~1.35 GiB**       | **~2.69 GiB**       |                      |                                  |

---

### 2.5 Argo CD & Kubernetes Control Plane

| Component                                                   | Pods         | CPU Req      | CPU Lim      | Mem Req      | Mem Lim      | Purpose                                            |
| :---------------------------------------------------------- | :----------- | :----------- | :----------- | :----------- | :----------- | :------------------------------------------------- |
| **Argo CD** (server, repo, controller, redis, dex)          | ~5           | 0.9 CPU      | 2.0 CPU      | 1.2 GiB      | 2.5 GiB      | Continuous GitOps delivery                         |
| **Kubernetes Overhead** (apiserver, etcd, coredns, kindnet) | ~14          | 1.0 CPU      | 2.0 CPU      | 2.8 GiB      | 3.5 GiB      | System & container runtime overhead across 4 nodes |
| **Control Plane Subtotal**                                  | **~19 Pods** | **~1.9 CPU** | **~4.0 CPU** | **~4.0 GiB** | **~6.0 GiB** |                                                    |

---

## 3. Total Cluster Aggregate Capacity

| Category                       | Pod Count    | CPU Requests   | CPU Limits (Burst Ceiling) | Memory Requests | Memory Limits (OOM Ceiling) | PVC Storage Required |
| :----------------------------- | :----------- | :------------- | :------------------------- | :-------------- | :-------------------------- | :------------------- |
| **Datastores**                 | 14           | 6.00 CPU       | 15.10 CPU                  | 6.30 GiB        | 12.60 GiB                   | 64 GiB               |
| **Operators & Platform**       | 12           | 1.10 CPU       | 4.20 CPU                   | 2.20 GiB        | 4.80 GiB                    | —                    |
| **Observability**              | 14           | 3.30 CPU       | 7.50 CPU                   | 3.70 GiB        | 9.00 GiB                    | 40 GiB               |
| **Workloads**                  | 11           | 2.95 CPU       | 7.62 CPU                   | 1.35 GiB        | 2.69 GiB                    | —                    |
| **Argo CD & K8s System**       | 19           | 1.90 CPU       | 4.00 CPU                   | 4.00 GiB        | 6.00 GiB                    | —                    |
| **TOTAL CLUSTER REQUIREMENTS** | **~70 Pods** | **~15.25 CPU** | **~38.42 CPU**             | **~17.55 GiB**  | **~35.09 GiB**              | **~104 GiB**         |

---

## 4. Minimum Node Capacity (3 Worker Nodes Setup)

To support this local production cluster topology across **3 Worker Nodes** (with 1 Control-Plane Node or dedicated scheduling):

### 4.1 Per-Worker Node Resource Distribution

When pods are scheduled across 3 worker nodes with anti-affinity:

| Metric           | Workload Scheduled per Worker | System / Daemon Overhead             | Minimum Capacity Required per Node  | Recommended Capacity per Node                 |
| :--------------- | :---------------------------- | :----------------------------------- | :---------------------------------- | :-------------------------------------------- |
| **vCPU (Cores)** | ~4.75 CPU requests            | ~0.50 CPU (kubelet/containerd/OTel)  | **6 vCPUs**                         | **8 vCPUs**                                   |
| **Memory (RAM)** | ~4.60 GiB requests            | ~1.00 GiB buffer & OS page cache     | **6 GiB RAM** (Minimum schedulable) | **8 GiB - 12 GiB RAM** (Handles limits burst) |
| **Disk Storage** | ~35 GiB PVC data              | ~15 GiB container images & ephemeral | **50 GiB SSD**                      | **80 GiB - 100 GiB SSD**                      |
| **Max Pods**     | ~20 - 25 pods per node        | System daemons                       | **35 Pods limit**                   | **50+ Pods limit**                            |

### 4.2 Summary: 3-Node Cluster Footprint

```
┌────────────────────────────────────────────────────────────────────────┐
│               LOCAL PRODUCTION CLUSTER MINIMUM PROFILE                │
├────────────────────────────────────────────────────────────────────────┤
│ • Node Count           : 3 Worker Nodes (+ 1 lightweight Control Plane)│
│ • Per-Worker Min Spec  : 6 vCPUs  | 8 GiB RAM  | 60 GB SSD             │
│ • Per-Worker Rec Spec  : 8 vCPUs  | 12 GiB RAM | 100 GB SSD            │
│ • Total Cluster Pool   : 18-24 vCPUs | 24-36 GiB RAM | ~200 GB SSD     │
│ • Host PC Target       : 10 Core / 20 Thread CPU, 25-32 GiB RAM, SSD   │
└────────────────────────────────────────────────────────────────────────┘
```
