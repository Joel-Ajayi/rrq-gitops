# Observability, Capacity, & Reliability Improvements Plan

Based on the engineering review and your feedback, here is the implementation plan to address the remaining gaps and establish a mature, clean-slate dashboard architecture.

## 1. Dashboard Architecture Overhaul (Clean Slate)

Currently, there are 8 scattered dashboards with hardcoded service names and mixed concerns. I propose deleting the existing dashboards and replacing them with a standardized, variable-driven 4-tier structure typical of mature SRE organizations:

### Tier 1: Executive & Business Operations (`tier1-business-kpis.yaml`)
- **Focus**: "Is the business making money? Are merchants succeeding?"
- **Panels**: Global Transfer Volume (GTV), overall transfer success rate, active merchants, total ledger imbalance (must be 0).
- **Target Audience**: Product, Execs, Business Ops.

### Tier 2: Universal Service Overview (`tier2-service-overview.yaml`)
- **Focus**: "Is my specific microservice healthy?"
- **Structure**: A single, dynamic dashboard with a `$service_name` dropdown variable.
- **Panels**: RED metrics (Rate, Error, Duration p50/p95/p99) powered by the `span_metrics` connector, CPU/Memory utilization per pod, Pod restarts, Open Circuit Breakers.
- **Target Audience**: On-call Engineers, Developers.

### Tier 3: Async & Messaging Pipeline (`tier3-event-pipeline.yaml`)
- **Focus**: "Are background jobs and webhooks flowing without getting stuck?"
- **Structure**: Dropdown for `$topic` and `$consumer_group`.
- **Panels**: Kafka consumer group lag, Outbox relay lag, DLQ ingestion rate, Webhook delivery success vs. backoff retries.
- **Target Audience**: Platform Engineers, Backend Developers.

### Tier 4: Infrastructure & Capacity Bottlenecks (`tier4-capacity-bottlenecks.yaml`)
- **Focus**: "Are we running out of physical resources?"
- **Panels**: 
  - **Postgres**: Client-side connection pool utilization (Active vs. Idle vs. Max per service), query latency distribution.
  - **Redis**: Memory fragmentation, Hit/Miss ratio, eviction rate.
  - **Kafka**: Disk utilization, under-replicated partitions.
- **Target Audience**: SREs, DBA, Capacity Engineers.

---

## 2. PostgreSQL Connection Pool Observability

To fulfill the Tier 4 dashboard requirements and give us exact visibility into worker starvation, we will instrument the `pgxpool` client-side metrics directly in the Go application:
- **Change**: In `services/go-services/internal/platform/postgres.go`, spawn a background goroutine for each created pool.
- **Action**: Every 5 seconds, read `pool.Stat()` and record the values to OTel gauges:
  - `rrq_pg_pool_acquired_conns`
  - `rrq_pg_pool_idle_conns`
  - `rrq_pg_pool_max_conns`
  - `rrq_pg_pool_empty_acquire_count` (counter for when workers wait for a connection)
- **Why**: Client-side metrics immediately expose when a service is bottlenecked waiting for a DB connection, which `postgres-exporter` (server-side) cannot see.

---

## 3. k6 Test Suite Enhancements

### 3.1 Resolving JWT Expiry in Soak Tests
- **How**: Modify `seed-test-data.mjs` to save the `apiKey` alongside the `jwts` into `test-data.json`.
- **Action**: In `k6/lib/api.ts`, implement a `refreshJwtIfNeeded` function. Since soak tests run for hours, the script will periodically call `POST /v1/auth/token` using the API key to fetch a fresh JWT before the 24-hour expiration hits.

### 3.2 Implementing xk6-disruptor for Chaos Testing
- **How it works**: `xk6-disruptor` is a k6 extension that injects faults directly into Kubernetes pods by attaching an ephemeral sidecar container that manipulates `iptables` (for network faults) or the application process. 
- **Action**: In `k6/scenarios/chaos.ts`, we will use the `PodDisruptor` API. During the test, the script will target the `core-api` pods using the label `app.kubernetes.io/name: core-api`.
- **Faults**: It will inject a 500ms network delay and a 10% HTTP 500 error rate for 60 seconds mid-test to observe how the circuit breakers and retries respond.

### 3.3 Aligning Thresholds to SLOs
- **Action**: Update `k6/config/thresholds.json` so that the `p(99)` latency thresholds match the 2000ms and 5000ms bounds defined in `capacity/slo-input.yaml`, ensuring k6 fails if the exact mathematical SLO is breached.
- **Action**: Update the workload mix across all scenarios to ensure `merchant-lookup` (highest QPS read endpoint) and `getWalletBalance` are exercised.

---

## 4. Capacity Engine & Observability Verifications

- **2.3 Capacity Math Bug**: Fix the integer division in `rrq-gitops/capacity/models.go` (`safeCap := float64(ceiling) / float64(numServicesOnShard) / float64(maxReplicas)`).
- **2.8 Consumer Poll Timeout**: Acknowledged. This was manually removed by you in recent commits. No action needed.
- **3.5 & 3.6 Span Metrics Attributes**: Verified. The `span_metrics` connector *automatically* extracts and includes `span.kind` and `service.name` as default dimensions. We do not need to explicitly declare them in `gateway.yaml`.

## Next Steps

Please review the proposed Dashboard structure (Tier 1-4). If the "clean slate" structure looks exactly like what you want, let me know, and I will proceed with the implementation!
