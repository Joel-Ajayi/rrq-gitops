# RRQ Load Testing & Capacity Engine Metric Gathering Guide

This directory contains the enterprise k6 load testing suite for the **RRQ (River Rust Queue)** payment core. It provides scenarios (`smoke`, `full_workload`, `load`, `stress`, `spike`, `soak`, `breakpoint`), data seeding tools, and token management utilities to measure real-world performance, validate system resiliency, and produce accurate empirical inputs for `rrq-gitops/tools/capacity-engine/slo-input.yaml`.

---

## Directory Structure

- `run.sh` — Enterprise test runner script.
- `seed-test-data.mts` — DB seeder (creates merchants & wallets in PostgreSQL shards).
- `refresh-tokens.mts` — Authenticates all seeded merchants and persists fresh JWT tokens to `test-data.json`.
- `deposit-test-data.mts` — Pre-funds test wallets with initial balances so transfers can execute without `400 Insufficient Funds`.
- `scenarios/` — k6 scenario scripts:
  - `smoke.ts` — Functional & DB write count baseline (5 RPS constant rate).
  - `full_workload.ts` — Comprehensive scenario exercising **all 8 endpoints** at nominal production ratios.
  - `load.ts` — Nominal steady-state production load (transfers, deposits, balances).
  - `stress.ts` — Peak load & backpressure limits (ramping up to 3,000 RPS).
  - `spike.ts` — Sudden 10x traffic surge to validate channel buffer depth and HPA spin-up lag.
  - `soak.ts` — Long-running endurance test (500 RPS over 1h–24h) for memory leaks and DB index bloat.
  - `breakpoint.ts` — Unlimited ramping to saturation to observe circuit breaker trips and component limits.
- `lib/` — Shared API client (`api.ts`), test data selector (`data.ts`), and config loader (`config.ts`).
- `config/` — Environment profiles (`dev.json`, `prod.json`) and SLO assertion thresholds (`thresholds.json`).

---

## Scenarios & Primary Questions Answered

| Scenario Script | Traffic Pattern | Primary Question Answered | Primary Output / Measurement |
| :--- | :--- | :--- | :--- |
| **`smoke.ts`** | Constant 5 RPS (30s) | *"Is the deployment functional, and what is the baseline DB write count per request under zero load?"* | `writes_per_message` |
| **`full_workload.ts`** | Mixed production ratios (ramping 100 $\rightarrow$ 1000 RPS) | *"How does the system perform across all 8 production API endpoints at nominal steady-state load?"* | `avg_query_time_ms`, `c_a_squared`, `c_s_squared`, `rps_per_core` |
| **`load.ts`** | Ramping to nominal (1000 RPS) | *"Does the system satisfy latency & availability SLOs under standard nominal daily traffic?"* | `avg_query_time_ms`, `partition_consume_rps` |
| **`stress.ts`** | Ramping to 3x peak (3000 RPS) | *"Can horizontal autoscaling (KEDA/HPA) and backpressure mechanisms handle projected peak traffic?"* | `peak_qps`, `producer_throughput_rps`, AIMD backpressure fractions |
| **`spike.ts`** | Sudden 10x surge (30s) | *"Do in-memory channel buffers and thread headroom absorb sudden traffic surges while pods scale?"* | Autoscale spin-up lag, buffer overflow rejections |
| **`soak.ts`** | Constant 500 RPS (1h - 24h) | *"Are there slow memory leaks, connection leaks, GC decay, or DB index bloat over extended operation?"* | Go heap growth, connection pool leaks, Redis fragmentation |
| **`breakpoint.ts`** | Unlimited ramping to saturation | *"Where does the cluster collapse, what component fails first, and do circuit breakers trip to isolate DBs?"* | Circuit breaker thresholds (`max_fails`, `min_requests`), recovery time |

---

## Metric Gathering Blueprint for `slo-input.yaml`

To populate `slo-input.yaml` with measured production data rather than assumed values, follow this 4-Phase workflow.

```
 ┌──────────────────────────────────────┐
 │ Step 0: Seed, Refresh & Pre-Fund     │ ──> ./run.sh seed && ./run.sh refresh && ./run.sh deposit
 └──────────────────────────────────────┘
                    │
                    ▼
 ┌──────────────────────────────────────┐
 │ Phase 1: Zero-Load Baseline & Writes │ ──> ./run.sh smoke dev
 │ (Constant 5 RPS on 1-Core Pods)      │     Measure: writes_per_message
 └──────────────────────────────────────┘
                    │
                    ▼
 ┌──────────────────────────────────────┐
 │ Phase 2: Nominal Workload & Capacity │ ──> ./run.sh full_workload dev
 │ (Mixed Endpoints on 1-Core Pods)     │     Measure: rps_per_core, avg_query_time_ms,
 └──────────────────────────────────────┘              c_s_squared, c_a_squared, partition_consume_rps
                    │
                    ▼
 ┌──────────────────────────────────────┐
 │ Phase 3: Stress & Peak Throughput    │ ──> ./run.sh stress dev
 │ (Ramping to Peak Capacity)           │     Measure: peak_qps, producer_throughput_rps,
 └──────────────────────────────────────┘              aimd_*_frac, peak_qps_per_pod
                    │
                    ▼
 ┌──────────────────────────────────────┐
 │ Phase 4: Breakpoint & Resiliency     │ ──> ./run.sh breakpoint dev
 │ (Saturation & Circuit Breaking)      │     Measure: Circuit breaker trip thresholds,
 └──────────────────────────────────────┘              min_requests, max_fails, half_open_probes
```

---

### Step 0: Data Seeding & Account Preparation

Before running any scenario, prepare the cluster data:

```bash
# 1. Create merchants and wallets across PostgreSQL shards
./run.sh seed

# 2. Authenticate all merchants and save fresh JWTs
./run.sh refresh

# 3. Pre-fund all 10,000 test wallets with initial balances
./run.sh deposit
```

---

### Phase 1: Zero-Load Baseline & DB Writes (`smoke.ts`)

In this phase, all services are pinned to **1 replica constrained to 1.0 vCPU** using `base/workloads/services/per-core.yaml`.

* **Execution Command:**
  ```bash
  ./run.sh smoke dev
  ```
* **Traffic Model:** `constant-arrival-rate` at 5 RPS for 30s (150 total requests).
* **Metrics to Extract for `slo-input.yaml`:**
  * **`writes_per_message`** (under each endpoint in `endpoints[*]`):
    * **Dashboard:** `Tier 4 — Database & Middleware` / Panel: `DB Write Spans per Request`
    * **Metric:** DB write spans divided by total requests:
      $$\text{writes\_per\_message} = \frac{\Delta \text{traces\_span\_metrics\_calls\_total}\{\text{span\_kind}="SPAN\_KIND\_INTERNAL", \text{db\_operation}=\sim"INSERT|UPDATE|DELETE"\}}{\text{Total Requests}}$$

---

### Phase 2: Steady-State Performance & Single-Core Capacity (`full_workload.ts`)

Simulate standard production traffic across **all 8 endpoints** (`create-transfer` 30%, `get-balance` 10%, `auth-token` 35%, `get-job` 10%, `create-wallet` 3%, `create-merchant` 1%, `admin-dlq-replay` 1%).

* **Execution Command:**
  ```bash
  ./run.sh full_workload dev
  ```
* **Metrics to Extract for `slo-input.yaml`:**
  1. **`rps_per_core`** (under each service in `services[*]`):
     * **Dashboard:** `Tier 5 — Advanced Queueing & Saturation` / Panel: `Compute Saturation Curve`
     * **PromQL:**
       $$\text{rps\_per\_core} = \frac{\text{rate}(\text{http\_requests\_total}[1m])}{\text{rate}(\text{container\_cpu\_usage\_seconds\_total}[1m])}$$
     * **Interpretation:** The sustainable throughput delivered per 1.0 CPU core at maximum efficient utilization before latency inflection.
  2. **`avg_query_time_ms`** (under each endpoint in `endpoints[*]`):
     * **Dashboard:** `Tier 4 — Database & Middleware` / Panel: `Database Query Latency`
     * **PromQL:** `rate(traces_span_metrics_duration_seconds_sum[5m]) / rate(traces_span_metrics_duration_seconds_count[5m]) * 1000`
  3. **Arrival & Service Time Variance (`c_a_squared` & `c_s_squared`)**:
     * **Dashboard:** `Tier 5 — Advanced Queueing & Saturation` / Panel: `Latency / Arrival Variance`
     * **`c_a_squared` (Squared Coefficient of Variation of Inter-arrival Times):** $\text{Var}(T_a) / E[T_a]^2$
     * **`c_s_squared` (Squared Coefficient of Variation of Service Times):** $\text{Var}(T_s) / E[T_s]^2$
  4. **`partition_consume_rps`** (`infrastructure.kafka.partition_consume_rps`):
     * **Dashboard:** `Tier 3 — Application & Services` / Panel: `API / Kafka Throughput`
     * **PromQL:** `rate(kafka_consumer_group_offset_sum_ratio{topic=~"jobs|xshard.*|notify"}[5m])`
  5. **Postgres & Redis Workload Baseline:**
     * **`session_busy_ratio`** (`infrastructure.postgres.workload`): Active sessions / Total connection limit
     * **`avg_parallelism`** (`infrastructure.postgres.workload`): Parallel workers utilized per query
     * **`fragmentation_factor`** (`infrastructure.redis`): `redis_memory_fragmentation_ratio`

---

### Phase 3: Peak Traffic & Backpressure Limits (`stress.ts`)

Ramp load from 900 to 3,000 RPS to evaluate peak limits, outbox producer limits, and webhook HTTP concurrency.

* **Execution Command:**
  ```bash
  ./run.sh stress dev
  ```
* **Metrics to Extract for `slo-input.yaml`:**
  1. **`peak_qps`** (under each endpoint in `endpoints[*]`):
     * **Dashboard:** `Tier 3 — Application & Services` / Panel: `API / Kafka Throughput`
     * **Metric:** Highest sustained `rate(traces_span_metrics_calls_total[1m])` without violating p99 latency SLO.
  2. **`producer_throughput_rps`** (`services[outbox-relay]`):
     * **Dashboard:** `Tier 4 — Database & Middleware` / Panel: `Outbox AIMD Backpressure`
     * **Metric:** Maximum `rate(outbox_events_published_total[1m])`
  3. **Outbox AIMD Backpressure Ratios (`aimd_*_frac`, `buffer_max_throttle_level`)**:
     * **Dashboard:** `Tier 4 — Database & Middleware` / Panel: `Outbox AIMD Backpressure`
     * **Metric:** Buffer fill fractions at which AIMD throttling kicks in.
  4. **Webhook Outbound HTTP Concurrency:**
     * **`peak_qps_per_pod`**: Max concurrent requests sustained per worker pod.
     * **`avg_latency_s`**: Average HTTP delivery latency to external webhook destinations.

---

### Phase 4: Over-Saturation & Fault Tolerance (`breakpoint.ts`)

Ramp load beyond system capacity to trigger failure modes, observe circuit breaker trips, and measure recovery.

* **Execution Command:**
  ```bash
  ./run.sh breakpoint dev
  ```
* **Metrics to Extract for `slo-input.yaml`:**
  * **Circuit Breaker Parameters (`circuit_breaker` block under each service):**
    * **`min_requests`**: Minimum request volume required in evaluation window to evaluate tripping.
    * **`max_fails`**: Consecutive failures or error threshold before circuit opens (`circuit_breaker_state == 2`).
    * **`half_open_probes`**: Number of probe requests allowed through in half-open state before full recovery.

---

## Mapping Reference Table

| Section in `slo-input.yaml` | Value Type / Source | Target Scenario | Grafana Dashboard & Panel |
| :--- | :--- | :--- | :--- |
| `endpoints[*].nominal_qps` | Target Business SLA (Demand Input) | Configured in `dev.json` | Demand Requirement |
| `endpoints[*].writes_per_message` | Empirical Measurement | Phase 1 (`smoke`) | Tier 4 — DB Write Spans per Request |
| `services[*].rps_per_core` | Empirical Measurement | Phase 2 (`full_workload`) | Tier 5 — Compute Saturation Curve |
| `endpoints[*].avg_query_time_ms` | Empirical Measurement | Phase 2 (`full_workload`) | Tier 4 — Database Query Latency |
| `endpoints[*].c_s_squared` / `c_a_squared` | Empirical Measurement | Phase 2 (`full_workload`) | Tier 5 — Latency / Arrival Variance |
| `infrastructure.kafka.partition_consume_rps` | Empirical Measurement | Phase 2 (`full_workload`) | Tier 3 — API / Kafka Throughput |
| `infrastructure.postgres.workload` | Empirical Measurement | Phase 2 (`full_workload`) | Tier 4 — Database Active Sessions |
| `infrastructure.redis.fragmentation_factor` | Empirical Measurement | Phase 2 (`full_workload`) | Tier 4 — Redis Memory Fragmentation |
| `endpoints[*].peak_qps` | Empirical Measurement | Phase 3 (`stress`) | Tier 3 — API / Kafka Throughput |
| `services[outbox-relay].producer_throughput_rps`| Empirical Measurement | Phase 3 (`stress`) | Tier 4 — Outbox AIMD Backpressure |
| `services[*].circuit_breaker.*` | Empirical Measurement | Phase 4 (`breakpoint`) | Tier 3 — Open Circuit Breakers |

---

## Recalculating Capacity & Sizing

After updating `slo-input.yaml` with the empirical measurements from the test phases:

```bash
cd ../capacity-engine
make run
```

This runs the analytical queuing models, regenerates `capacity-output.yaml`, and automatically renders production Kubernetes deployments, HPA/KEDA autoscalers, and connection pool configurations in `base/workloads/`.
