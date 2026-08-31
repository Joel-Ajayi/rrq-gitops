# Capacity Planning Engine

A Go-based infrastructure capacity planning tool that derives microservice resource configurations directly from SLO targets and physical infrastructure constraints. It takes a single `slo-input.yaml` file as input, applies a set of **mathematical models** (queueing theory, Little's Law, Kafka/Database/Redis capacity formulas), validates that total demand fits within infrastructure supply, and renders the results directly into Kubernetes manifests.

---

## Pipeline Overview

```
slo-input.yaml
     │
     ▼
┌─────────┐     ┌─────────┐     ┌──────────┐     ┌────────────────┐
│ Supply   │ ──▶ │ Demand  │ ──▶ │ Fit-Check│ ──▶ │ Render         │
│ Ceilings │     │ Derive  │     │ Validate │     │ Manifests + Out│
└─────────┘     └─────────┘     └──────────┘     └────────────────┘
```

| Phase         | Source File                        | Description                                                                                                                |
| ------------- | ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| **Supply**    | `supply.go`                        | Computes infrastructure ceilings from physical inputs (Postgres max connections, Kafka partition budget, Redis maxmemory). |
| **Demand**    | `demand.go`                        | Derives per-service parameters from SLO targets via the models below.                                                      |
| **Fit-Check** | `fitcheck.go`                      | Validates total demand ≤ supply. Emits `FAIL` (blocks deployment) or `WARN` (advisory).                                    |
| **Render**    | `render.go`, `render_manifests.go` | Renders per-service + platform ConfigMaps into `base/workloads/config/`, writes `capacity-output.yaml`, and patches Kubernetes manifests.                                                            |

All formulas live in `models.go`. `demand.go` is pure orchestration — no formulas.

---

## Mathematical Models

### 1. Queueing Theory — Latency & Worker Sizing

#### Kingman's Formula (G/G/1) — _Kingman, 1961_

The expected waiting time in a single-server queue, the foundation of the entire engine:

```
E[W_q] ≈ ( ρ / (1 − ρ) ) × ( (c_a² + c_s²) / 2 ) × τ
```

| Variable | Meaning                           | Typical Value           |
| -------- | --------------------------------- | ----------------------- |
| `ρ`      | Target utilization (from SLO)     | 0.60–0.75               |
| `c_a²`   | Squared CV of inter-arrival times | Poisson arrival ≈ `1.0` |
| `c_s²`   | Squared CV of service times       | DB queries ≈ `1.5`      |
| `τ`      | Mean service time (seconds)       | `AvgQueryTimeMS / 1000` |

#### Erlang C (M/M/c) — Probability of Delay

For `c` parallel workers, offered load `A = c·ρ`:

```
B₀ = 1        Bᵢ = (A·Bᵢ₋₁) / (i + A·Bᵢ₋₁)   (i = 1..c)
P(delay) = (c·B) / (c − A·(1 − B))
```

#### Allen–Cunneen Approximation (G/G/c) — **Per-Endpoint Residence Time**

The engine's latency model, combining Erlang C with Kingman's variance term:

```
L(ρ) = τ + ( P(delay) / c ) × ( τ / (1 − ρ) ) × ( (c_a² + c_s²) / 2 )

workerConcurrency(): workers = ceil( throughput_per_pod × L(ρ) × worker_amp )
```

#### Weighted Average Service Time

Blends per-endpoint query times by their peak traffic share (plus HTTP network I/O if present):

```
avgMS = Σ(Sᵢ × λ_peakᵢ) / Σ(λ_peakᵢ)   (+ HTTP.AvgLatencyS × 1000)
```

---

### 2. Little's Law Applications — Pools & Backlogs

`L = λ × W` applied directly:

#### DB Pool Demand (per instance)

```
demand = max over endpoints ( λ_peak_ep × S_ep ) / ρ
per_pod_pool = max( ceil( maxDemand / minReplicas ), pool_floor )
```

_Pool of 1 serializes — floor exists so pools never collapse to 1 (HikariCP/PG wiki)._

#### KEDA Lag Threshold (acceptable backlog per pod)

```
lag_threshold = ceil( SLO_latency_ms / avg_ms ) × workers
```

#### HTTP Outbound Pool (Go net/http Transport)

```
http_pool = max( ceil( qps × latency_s × headroom ), worker_count )
per_host   = ceil( http_pool / host_count )
```

---

### 3. PostgreSQL Capacity — Supply Side

#### Max Connections (PG wiki RAM budget)

```
shared_buffers   = RAM × shared_buffers_pct    (25% standard)
OS page cache    = RAM × os_buffer_pct         (25% standard)
maintenance_work = RAM × maintenance_pct       (15% — VACUUM/ANALYZE)

available  = RAM − shared − os − maintenance
per_conn   = ceil( work_mem × 0.25 ) + 8MB     (0.25 fractional-work_mem + conn overhead)
max_conns  = floor( available / per_conn )
```

#### Optimal Active Connections

```
optimal_active = (db_cores × 2) + effective_spindles
```

_Cores are PHYSICAL (exclude HT); spindles: `0` = cached SSD, `1` = SSD._

#### Storage Growth

```
storage_GB/day = Σ( λ_peak / shards × writes_per_msg ) × 1KB × 86400 / 2³⁰
```

#### Per-Shard RW/RO Connection Caps

```
per_shard = max( 2, min( pool_demand, ceil( ceiling / services_on_shard / max_replicas ) ) )
```

Guarantees total connections at peak (all replicas × all services) fit under the PG hard limit. RW+RO pools are then scaled proportionally down if their sum exceeds `PoolSize`.

---

### 4. Replica Scaling & Pod Capacity

```
pod_cap    = rps_per_core × cores_per_pod
min_repl   = max( ceil( λ_nominal / pod_cap ), min_replicas_default )
max_repl   = min( max( ceil( λ_peak / pod_cap × az_factor ), min_repl ), max_replicas_default )
```

`az_factor` = N+1 AZ redundancy; `λ_peak` carries a `(1 + retry_budget_fraction)` amplification.

---

### 5. Retry Budgets — Exponential Backoff & Token Bucket

```
budget      = SLO_latency_ms × (1 − slack) × retry_fraction
max_retries = floor( log₂( budget / base + 1 ) )
backoff_cap = min( base × 2^(retries−1), budget )      (retries=0 → base × 4)
base        = max(1, ceil( weighted_avg_ms ))
```

Token bucket derived from per-pod peak volume over a 2s burst window:

```
max_tokens = max( ceil( peak_qps_per_pod × 2.0s × retry_fraction ), 10 )
min_tokens = max( ceil( max_tokens × 0.10 ), 2 )
```

---

### 6. Consumer Timing — Kafka Consumer Group Protocol (KIP-62)

```
process_timeout = max( SLO × (1 − slack), ceil( Kingman L(ρ) ) )   # per-message deadline
session         = process_timeout + DLQ_total + 2 × buffer         # segmentio/kafka-go
heartbeat       = session / 3                                      # heartbeat ≤ session/3
shutdown        = max( 30s, session + 15s )
idle_timeout    = session + heartbeat                              # Go HTTP server
```

#### DLQ Retry Derivation & Storage (Global DLQ)

**Note:** All DLQ messages are now written directly to the Global DLQ (`merchants` database). Storage capacity for DLQ is modeled explicitly by the `admin-dlq-replay` endpoint on the `core-api` service, rather than attributing DLQ write overhead to individual worker nodes.

```
dlq_budget = process_timeout / 2
retries    = 2 (operator-capped)
base_delay = dlq_budget / 8
max_delay  = dlq_budget / 4
worst case = 2 × (base + max) = ¾ × budget ≤ budget ✓
```

---

### 7. Kafka Models

#### Partition Count

```
partitions = max( ceil( λ_topic_peak / per_partition_consume ), max_replicas )
```

* `λ_topic_peak`: Isolated peak message rate for the specific topic (`jobs`, `notify`, `xshard.*`).
* `max_replicas`: Enforces consumer pod scaling concurrency as a hard floor so scaled pods are never partition-starved.
* Automatically patched into `base/platform/datastores/kafka/topics.yaml`.

#### Cluster Capacity & File Descriptors

```
cluster_cap = min( brokers × per_broker_cap, 200,000 )        (Confluent 2023)
segments    = ceil( retention_seconds / segment_seconds )
fd_estimate = per_broker_cap × (segments + 1) × 2             (Jun Rao 2015)
latency_advisory: cluster_cap ≤ 100 × brokers × replication_factor
```

#### Storage Growth

```
storage_GB/day = Σ(producer λ_peak) × 1KB × 86400 × replication_factor / 2³⁰
```

---

### 8. Redis — Max Memory & Keyspace

```
maxmem = RAM_per_node × (1 − fork_headroom) / fragmentation

keyspace (velocity windows) = Σ( merchants × window_buckets × per_key_bytes )
fit-check: keyspace ≤ nodes × maxmem
```

---

### 9. Webhook Fault Tolerance

#### Per-Merchant Bulkhead (multi-tenant isolation)

```
per_merchant = max( 1, ceil( fast_lane_workers × 0.10 ) )   # 10% pool per merchant
```

#### Breaker Eviction TTL

```
eviction_ttl = max( 5 min, 10 × max( delivery_backoff_base, dlq_base_delay ) )
```

---

### 10. Relay (Outbox) Derivation

```
fetch_batch   = min( max_fetch_batch, floor( SLO_latency_ms × 0.4 / avg_ms ) )   # 40% SLO reserved for DB
replicas      = ceil( total_peak / producer_throughput )
batch_timeout = fetch_batch × ceil( avg_ms )
pool_interval = max( 1, ceil( 1000 × fetch_batch / (total_peak / replicas) ) − 1 )
```

---

### 11. Resource Sizing — CPU & Memory

```
cpu_mcores = ceil( (λ_nominal / min_replicas) / rps_per_core × 1000 )

mem_mib = (pool × 50KB) + (http × 50KB) + 64MiB baseline + kafka_reader_buffer
relay   = base + staging_KB/1024 + max(1, fetch_batch × max_payload_KB/1024)
```

_Memory constants (`models.go` → `constants.go`): 50 KB per PG conn (pgx), 50 KB per TLS conn (net/http Transport), 64 MiB Go runtime baseline._

---

### 12. Fit-Check Models — Demand vs Supply

#### Connection Demand & HPA Cap

```
connection_demand = pool_size × replicas
gap               = usable_conns − Σ(j≠i)(pool_j × replicas_j)
hpa_cap           = max( floor( gap / pool_i ), min_replicas )
```

Peak demand > max_conns ⇒ **FAIL**; peak > optimal_active ⇒ **WARN**; minimum-scale demand > max_conns ⇒ **FATAL**. Each service's HPA max is clamped to its fair share of the gap.

#### Kafka Spread

```
spread = (total_partitions × replication_factor) / brokers
```

`spread > per_broker_cap` or `parts > 200,000` ⇒ **FAIL**; `segment_seconds > retention` breaks deletion policy ⇒ **FAIL**.

---

## Physical Constants (`constants.go`)

| Constant                 | Value   | Source                      |
| ------------------------ | ------- | --------------------------- |
| `KafkaClusterMaxParts`   | 200,000 | Confluent 2023, KIP-578     |
| `KafkaLatencyAdvisoryR`  | 100     | Confluent 2023              |
| `PodPGConnMemBytes`      | 50 KB   | go pgx memory profile       |
| `PodHTTPTLSMemBytes`     | 50 KB   | net/http Transport          |
| `PodAPPBaselineMemBytes` | 64 MiB  | Go runtime + GC + libraries |

---

## How to Run

```bash
# From the repository root:
make capacity

# Or directly:
cd tools/capacity-engine
go run . -input slo-input.yaml -output capacity-output.yaml -render ../..
```

Flags:
- `-input` — SLO input YAML (default `slo-input.yaml`)
- `-output` — capacity report YAML (default `capacity-output.yaml`)
- `-render` — rrq-gitops repo root; manifests are patched relative to it (default `.`)

Exits `0` on `CAPACITY CHECK PASSED` or `1` on `CAPACITY CHECK FAILED`.

---

## Input: `slo-input.yaml`

The sole input file. Every value is either measured from Prometheus/Grafana or defined by a business requirement. No defaults, no fallbacks — the YAML is the single source of truth.

| Annotation Pattern                                    | Meaning                                            |
| ----------------------------------------------------- | -------------------------------------------------- |
| `Tier 3 \| Panel: ... \| Metric: ...`                 | Measured from Grafana Service Health RED dashboard |
| `Tier 4 \| Panel: ... \| Metric: ...`                 | Measured from Grafana Middleware USE dashboard     |
| `Tier 5 \| Panel: ... \| Metric: ...`                 | Measured from Grafana Infrastructure USE dashboard |
| `Product SLO`                                         | Business-defined service level objective           |
| `Capacity Team` / `Security Team` / `Risk/Trust Team` | Policy from the respective team                    |
| `k8s ...` / `Terraform ...`                           | Cluster or IaC provisioned value                   |

## Output

1. **Terminal report** — `SUPPLY` ceilings, `DERIVED` per-service values, `FIT-CHECK` verdict.
2. **`capacity-output.yaml`** — serialized debug output (do not edit manually).
3. **Kubernetes manifests** — patches `base/workloads/services/` KEDA ScaledObject thresholds, `base/platform/datastores/kafka/topics.yaml` partition counts, `base/platform/datastores/postgres/shards.yaml` max_connections, and per-service Deployment resources, making output immediately deployable via GitOps.
