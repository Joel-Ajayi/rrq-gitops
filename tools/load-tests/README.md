# RRQ Load Testing & Capacity Engine Metric Gathering Guide

This directory contains the enterprise k6 load testing suite for the **RRQ (River Rust Queue)** payment core. It provides scenarios (`smoke`, `full_workload`, `load`, `stress`, `spike`, `soak`, `breakpoint`) and data seeding tools to measure real-world performance, detect operational defects, and produce accurate empirical inputs for `rrq-gitops/tools/capacity-engine/slo-input.yaml`.

---

## Directory Structure

- `run.sh` — Enterprise test runner script.
- `seed-test-data.mjs` — DB seeder (creates merchants & wallets).
- `scenarios/` — k6 scenario scripts:
  - `smoke.ts` — Functional & DB query/span baseline.
  - `full_workload.ts` — Comprehensive scenario exercising **all 8 endpoints** defined in `slo-input.yaml`.
  - `load.ts` — Nominal steady-state production load (transfers, deposits, balances).
  - `stress.ts` — Peak load & backpressure limits.
  - `spike.ts` — Sudden surge & channel buffer depth validation.
  - `soak.ts` — Long-running endurance & resource leak detection.
  - `breakpoint.ts` — Over-saturation & circuit breaker fault injection.
- `lib/` — Helpers, API client (`api.ts`), and custom metrics definitions.
- `config/` — Environment configuration profiles (`dev`, `prod`).

---

## Scenarios & Primary Questions Answered

| Scenario Script | Traffic Pattern | Primary Question Answered | Primary Output / Measurement |
| :--- | :--- | :--- | :--- |
| **`smoke.ts`** | Constant 5 RPS (30s) | *"Is the deployment functional, and what is the baseline DB write count per request?"* | `writes_per_message` |
| **`full_workload.ts`** | Mixed production ratios | *"How does the system perform under a realistic mix of all 8 production API endpoints?"* | `nominal_qps`, `avg_query_time_ms`, `c_a_squared`, `c_s_squared` |
| **`load.ts`** | Ramping to nominal (1000 RPS) | *"Does the system satisfy latency & availability SLOs under standard nominal daily traffic?"* | `nominal_qps`, `avg_query_time_ms`, `partition_consume_rps` |
| **`stress.ts`** | Ramping to 3x peak (3000 RPS) | *"Can horizontal autoscaling (KEDA/HPA) and backpressure mechanisms handle projected peak traffic?"* | `peak_qps`, `producer_throughput_rps`, AIMD fractions |
| **`spike.ts`** | Sudden 10x surge (30s) | *"Do in-memory channel buffers and thread headroom absorb sudden traffic surges while pods scale?"* | Autoscale lag, `http_headroom`, buffer depth |
| **`soak.ts`** | Constant 500 RPS (1h - 24h) | *"Are there slow memory leaks, connection leaks, GC decay, or DB index bloat over extended operation?"* | Connection timeouts, memory stability, Redis fragmentation |
| **`breakpoint.ts`** | Unlimited ramping to crash | *"Where does the cluster collapse, what component fails first, and do circuit breakers trip to isolate DBs?"* | Circuit breaker thresholds (`max_fails`, `min_requests`), recovery time |

---

## Detailed Scenario Guides

### 1. `smoke.ts` — Functional & DB Write Baseline
* **Primary Question Answered:** *"Is the deployment functional, and what is the baseline DB write count per request?"*
* **When to Run:** Immediately after deploying a new release or applying migration changes.
* **Run Command:**
  ```bash
  ./run.sh smoke dev
  ```
* **What it Measures:** Validates 200/202 responses and extracts `writes_per_message` from DB write spans (`traces_span_metrics_calls_total`).

---

### 2. `full_workload.ts` — Comprehensive 8-Endpoint Workload Mix
* **Primary Question Answered:** *"How does the system perform under a realistic mix of all 8 production API endpoints?"*
* **When to Run:** Phase 2 baseline measurement for `slo-input.yaml`.
* **Run Command:**
  ```bash
  ./run.sh full_workload dev
  ```
* **What it Measures:** Exercises `create-transfer` (30%), `get-balance` (10%), `merchant-lookup`/`auth-token` (35%), `get-job` (10%), `create-wallet` (3%), `create-merchant` (1%), and `admin-dlq-replay` (1%). Extracts `nominal_qps`, `avg_query_time_ms`, `c_a_squared`, and `c_s_squared`.

---

### 3. `load.ts` — Nominal Steady-State Performance
* **Primary Question Answered:** *"Does the system satisfy latency & availability SLOs under standard nominal daily traffic?"*
* **When to Run:** Routine daily capacity validation or PR merge validation.
* **Run Command:**
  ```bash
  ./run.sh load dev
  ```
* **What it Measures:** Measures p95/p99 HTTP latency and Kafka topic consumption rates (`partition_consume_rps`) under nominal load.

---

### 4. `stress.ts` — Peak Capacity & Backpressure Limits
* **Primary Question Answered:** *"Can horizontal autoscaling (KEDA/HPA) and backpressure mechanisms handle projected peak traffic?"*
* **When to Run:** Before high-volume business events (e.g. Black Friday) or Phase 3 metric gathering.
* **Run Command:**
  ```bash
  ./run.sh stress dev
  ```
* **What it Measures:** Determines `peak_qps`, outbox event publishing throughput (`producer_throughput_rps`), AIMD backpressure levels, and webhook HTTP concurrency.

---

### 5. `spike.ts` — Sudden Surge & Buffer Depth Validation
* **Primary Question Answered:** *"Do in-memory channel buffers and thread headroom absorb sudden 10x traffic surges while pods scale?"*
* **When to Run:** Validating KEDA/HPA autoscale lag and channel buffer sizing (`consumer_partition_size`).
* **Run Command:**
  ```bash
  ./run.sh spike dev
  ```
* **What it Measures:** Rapidly surges traffic from 100 RPS to 2,500 RPS in 30 seconds to observe pod spin-up latency, request queueing, and buffer overflow rejections.

---

### 6. `soak.ts` — Long-Term Endurance & Resource Leak Detection
* **Primary Question Answered:** *"Are there slow memory leaks, connection leaks, GC decay, or DB index bloat over extended operation?"*
* **When to Run:** Overnight stability runs prior to major version releases.
* **Run Command:**
  ```bash
  ./run.sh soak dev
  ```
* **What it Measures:** Runs sustained moderate load (500 RPS) over 1 to 24 hours to monitor Go heap allocation growth, PostgreSQL connection pool leaks (`max_lifetime_ms`), and Redis fragmentation growth.

---

### 7. `breakpoint.ts` — Over-Saturation & Circuit Breaker Fault Injection
* **Primary Question Answered:** *"Where does the cluster collapse, what component fails first, and do circuit breakers trip to isolate DBs?"*
* **When to Run:** Phase 4 resiliency testing and circuit breaker threshold tuning.
* **Run Command:**
  ```bash
  ./run.sh breakpoint dev
  ```
* **What it Measures:** Ramps traffic indefinitely until 5xx errors cascade. Captures circuit breaker trip points (`min_requests`, `max_fails`, `half_open_probes`) and measures cluster recovery time.

---

## Metric Gathering Blueprint for `slo-input.yaml`

To populate `slo-input.yaml` with measured production data rather than assumed values, execute the following 4-Phase operational workflow in order.

```
 ┌──────────────────────────────────┐
 │ Phase 1: Isolated Micro-Bench    │ ──> Measure: rps_per_core (1 pod, 1 vCPU, HPA off)
 │ (Single-Pod & Single-Worker)     │     Measure: writes_per_message
 └──────────────────────────────────┘
                 │
                 ▼
 ┌──────────────────────────────────┐
 │ Phase 2: Full Workload Mix       │ ──> Measure: nominal_qps, avg_query_time_ms,
 │ (All 8 Endpoints at Steady State)│     c_s_squared, c_a_squared, partition_consume_rps
 └──────────────────────────────────┘
                 │
                 ▼
 ┌──────────────────────────────────┐
 │ Phase 3: Stress & Backpressure   │ ──> Measure: peak_qps, producer_throughput_rps,
 │ (Ramping to Peak Capacity)       │     aimd_*_frac, peak_qps_per_pod
 └──────────────────────────────────┘
                 │
                 ▼
 ┌──────────────────────────────────┐
 │ Phase 4: Faults & Resiliency     │ ──> Measure: Circuit breaker thresholds,
 │ (Breakpoint & Over-saturation)   │     half_open_probes, error recovery limits
 └──────────────────────────────────┘
```

### Pre-requisite: Seed the Environment

Before running tests, populate the database with merchant accounts and pre-funded wallets:
```bash
./run.sh seed
```

---

### Phase 1: Isolated Component Micro-Benchmarking (`rps_per_core` & `writes_per_message`)

`rps_per_core` is the single-core CPU capacity baseline ($\text{podCap} = \text{rps\_per\_core} \times \frac{\text{cpu\_mcores}}{1000}$). It MUST be measured on an isolated single pod (`replicas: 1`, `cpu: 1000m`, HPA disabled) before multi-pod autoscaling tests are executed.

1. **Deploy Isolated Benchmarking Pod:** Pin `core-api` to 1 replica and 1 vCPU (`cpu: 1000m`).
2. **Run Smoke Scenario:**
   ```bash
   ./run.sh smoke dev
   ```
3. **Metrics to Extract for `slo-input.yaml`:**
   * **`rps_per_core`** (under each service in `services[*]`):
     * **Dashboard:** `Tier 5 — Advanced Queueing & Saturation` / Panel: `Compute Saturation Curve`
     * **Metric:** `Total Service RPS / Total Pod Cores` at 100% CPU utilization (`process_cpu_seconds_total`).
   * **`writes_per_message`** (under each endpoint in `services[*].endpoints`):
     * **Dashboard:** `Tier 4 — Database & Middleware` / Panel: `DB Write Spans per Request`
     * **Metric:** `traces_span_metrics_calls_total{span_kind="SPAN_KIND_INTERNAL", db_operation=~"INSERT|UPDATE|DELETE"}`

---

### Phase 2: Steady-State Baseline & Full Workload Mix (`full_workload`)

Simulate standard steady-state production traffic across **all 8 endpoints** (`create-transfer`, `get-balance`, `merchant-lookup`, `auth-token`, `create-merchant`, `create-wallet`, `get-job`, `admin-dlq-replay`).

* **Run Command:**
  ```bash
  ./run.sh full_workload dev
  ```
* **Metrics to Extract for `slo-input.yaml` (in order):**
  1. **`nominal_qps`** (under each endpoint in `services[*].endpoints`):
     * **Dashboard:** `Tier 3 — Application & Services` / Panel: `API / Kafka Throughput`
     * **Metric:** `rate(traces_span_metrics_calls_total[5m])` or `rate(kafka_consumer_offset_sum[5m])`
  2. **`avg_query_time_ms`** (under each endpoint):
     * **Dashboard:** `Tier 4 — Database & Middleware` / Panel: `Database Query Latency`
     * **Metric:** `rate(traces_span_metrics_duration_seconds_sum[5m]) / rate(traces_span_metrics_duration_seconds_count[5m]) * 1000`
  3. **Arrival & Service Variance (`c_a_squared` & `c_s_squared`)**:
     * **Dashboard:** `Tier 5 — Advanced Queueing & Saturation` / Panel: `Latency / Arrival Variance`
     * **`c_a_squared` (Arrival Variance):** `variance(RPS) / mean(RPS)^2`
     * **`c_s_squared` (Service Variance):** `variance(duration) / mean(duration)^2`
  4. **`partition_consume_rps`** (`infrastructure.kafka.partition_consume_rps`):
     * **Dashboard:** `Tier 3 — Application & Services` / Panel: `API / Kafka Throughput`
     * **Metric:** `rate(kafka_consumer_offset_sum{topic=~"jobs|xshard.*|notify"}[5m])`
  5. **Postgres & Redis Workload Baseline:**
     * **`session_busy_ratio`** (`infrastructure.postgres.workload`): `Tier 4` / Panel: `Database Query Latency` (`active_sessions / total_sessions`)
     * **`avg_parallelism`** (`infrastructure.postgres.workload`): `Tier 4` / Panel: `Database Query Latency` (`pg_settings_max_parallel_workers_per_gather`)
     * **`fragmentation_factor`** (`infrastructure.redis`): `Tier 4` / Panel: `Redis Memory Fragmentation` (`redis_memory_fragmentation_ratio`)

---

### Phase 3: Peak Traffic & Backpressure (`stress`)

Push system throughput to peak capacity to observe maximum peak QPS, outbox producer limits, and webhook HTTP concurrency.

* **Run Command:**
  ```bash
  ./run.sh stress dev
  ```
* **Metrics to Extract for `slo-input.yaml` (in order):**
  1. **`peak_qps`** (under each endpoint in `services[*].endpoints`):
     * **Dashboard:** `Tier 3 — Application & Services` / Panel: `API / Kafka Throughput`
     * **Metric:** Peak `rate(traces_span_metrics_calls_total[1m])` reached before SLO degradation.
  2. **`producer_throughput_rps`** (`services[outbox-relay]`):
     * **Dashboard:** `Tier 4 — Database & Middleware` / Panel: `Outbox AIMD Backpressure`
     * **Metric:** `rate(outbox_events_published_total[1m])`
  3. **Outbox AIMD Backpressure Ratios (`buffer_max_throttle_level`, `aimd_*_frac`)**:
     * **Dashboard:** `Tier 4 — Database & Middleware` / Panel: `Outbox AIMD Backpressure`
     * **Metric:** `kafka_producer_buffer_fill` fill fractions during saturation.
  4. **Webhook Outbound HTTP Concurrency:**
     * **`peak_qps_per_pod`**: `Tier 4` / Panel: `Webhook Concurrency` (`bulkhead_inflight_requests` max)
     * **`avg_latency_s`**: `Tier 4` / Panel: `Database Query Latency` (`traces_span_metrics_duration_seconds`)

---

### Phase 4: Over-Saturation & Fault Tolerance (`breakpoint`)

Ramp load past peak capacity to force fault conditions and measure circuit breaker trip thresholds.

* **Run Command:**
  ```bash
  ./run.sh breakpoint dev
  ```
* **Metrics to Extract for `slo-input.yaml`:**
  * **Circuit Breaker Parameters (`circuit_breaker` block under each service):**
    * **`min_requests`, `interval_ms`, `max_fails`, `half_open_probes`**: `Tier 3` / Panel: `Open Circuit Breakers` (`circuit_breaker_state`, `circuit_breaker_half_open_failures_total`).

---

## Mapping Reference Table

| Section in `slo-input.yaml` | Value Source | Test Stage Required | Grafana Dashboard Tier & Panel |
| :--- | :--- | :--- | :--- |
| `infrastructure.postgres.instances` | Hardcoded / Infra Specs | None (Terraform / K8s) | N/A |
| `infrastructure.postgres.workload` | Measured | Phase 2 (`full_workload`) | Tier 4 — Database Query Latency |
| `infrastructure.kafka.partition_consume_rps` | Measured | Phase 2 (`full_workload`) | Tier 3 — API / Kafka Throughput |
| `infrastructure.redis.fragmentation_factor` | Measured | Phase 2 (`full_workload`) | Tier 4 — Redis Memory Fragmentation |
| `endpoints[*].nominal_qps` | Measured | Phase 2 (`full_workload`) | Tier 3 — API / Kafka Throughput |
| `endpoints[*].avg_query_time_ms` | Measured | Phase 2 (`full_workload`) | Tier 4 — Database Query Latency |
| `endpoints[*].c_s_squared` / `c_a_squared` | Measured | Phase 2 (`full_workload`) | Tier 5 — Latency / Arrival Variance |
| `endpoints[*].writes_per_message` | Measured | Phase 1 (`smoke`) | Tier 4 — DB Write Spans per Request |
| `endpoints[*].peak_qps` | Measured | Phase 3 (`stress`) | Tier 3 — API / Kafka Throughput |
| `services[*].rps_per_core` | Measured | Phase 1 (Single-Pod Micro-Bench) | Tier 5 — Compute Saturation Curve |
| `services[*].circuit_breaker.*` | Measured | Phase 4 (`breakpoint`) | Tier 3 — Open Circuit Breakers |

---

## Recalculating Capacity

After updating `slo-input.yaml` with measured values:
```bash
cd ../capacity-engine
make run
```
This regenerates `capacity-output.yaml` and updates K8s deployment manifests/HPA/KEDA scale targets.
