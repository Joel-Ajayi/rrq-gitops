# Questioning Decisions: Resilience Engineering & Capacity Modeling

This document explicitly questions every resilience mechanism and capacity sizing formula used in RRQ, detailing **why X was chosen**, **why alternative Y was rejected**, **what trade-offs were accepted**, and **when the decision would be wrong**.

---

## Decision 1: Full Jitter Exponential Backoff vs Fixed or Pure Exponential Backoff

### Question:
During a backend database or network outage, why use **Full Jitter Exponential Backoff** instead of fixed retries or pure exponential backoff?

### Why Full Jitter (Chosen):
$$
t_{\text{sleep}}(n) = \text{UniformRandom}\left(0, \; \min\left(T_{\text{max}}, T_{\text{base}} \cdot 2^{n-1}\right)\right)
$$
1. **Prevents Thundering Herd Retry Storms**: If 500 clients fail simultaneously, pure exponential backoff causes all 500 clients to retry at the exact same millisecond. Full Jitter spreads retry attempts uniformly over time $[0, t_{\text{exponential}}]$.
2. **Zero Collision Spike Probability**: Desynchronizes worker retries, allowing a recovering PostgreSQL database pool to process retries smoothly.

### Why Fixed / Pure Exponential Backoff (Rejected):
- **Fixed Backoff** (e.g. sleep 1s): Creates massive repeating spike waves that re-crush the database every 1 second.
- **Pure Exponential Backoff** (e.g. 1s, 2s, 4s): Spreads out spike waves over time, but every single retry point is still a synchronized thundering herd wave.

### Accepted Trade-offs:
- Random sleep delays mean some retries wait longer than the theoretical minimum exponential delay.

### When this Decision is WRONG:
In interactive, synchronous UI progress indicators where the user interface requires a deterministic minimum retry interval (e.g. polling a status progress bar every exact 2.0 seconds).

---

## Decision 2: Token Bucket Retry Budget vs Fixed Retry Count (e.g. `MaxRetries = 3`)

### Question:
Why use a **Token Bucket Retry Budget** (`platform.RetryBudget`) to limit retries system-wide instead of a static per-request retry count like `MaxRetries = 3`?

### Why Token Bucket Retry Budget (Chosen):
1. **Sheds Retries During Severe Outages**: When error rates spike (e.g. database down), retry tokens drain rapidly. Once tokens hit `0`, `TryAcquire()` returns `false`, failing retries fast to the Dead Letter Queue (DLQ) without sending requests over the network.
2. **Prevents Traffic Amplification**: Under a total outage, static `MaxRetries = 3` multiplies inbound traffic by **$4\times$** ($1 \text{ original} + 3 \text{ retries}$), guaranteeing that a struggling database will never recover. The retry budget caps total retries to a fixed fraction (e.g. 10%) of total successful volume.

### Why Static Retry Count (Rejected):
Static retry counts unconditionally retry $N$ times regardless of whether the system is experiencing a transient glitch or a massive multi-hour outage.

### Accepted Trade-offs:
- During a major outage, transient requests that might have succeeded on a late retry are shed to DLQ early once the token bucket is exhausted.

### When this Decision is WRONG:
In offline batch processing jobs where a job MUST retry indefinitely until manual intervention, regardless of how long the outage lasts.

---

## Decision 3: Kingman's Queueing Formula vs M/M/1 Poisson Approximations in Capacity Planning

### Question:
Why use **Kingman's Formula** in the capacity planning engine to derive queueing latency $W_q$ instead of standard M/M/1 queueing models?

### Why Kingman's Formula (Chosen):
$$
W_q \approx \left( \frac{\rho}{1 - \rho} \right) \cdot \left( \frac{C_a^2 + C_s^2}{2} \right) \cdot \tau
$$
1. **Accounts for Arrival & Service Variance**: M/M/1 assumes Poisson arrivals ($C_a^2 = 1$) and exponential service times ($C_s^2 = 1$). Real payment traffic exhibits bursty arrival variance ($C_a^2 > 1$) and multi-modal database query variance ($C_s^2 > 1$).
2. **Accurate Queue Delay Calculation**: Accurately predicts latency degradation as CPU utilization $\rho \to 0.70$, preventing under-provisioning.

### Why M/M/1 Poisson Model (Rejected):
M/M/1 severely underestimates queueing latency when traffic is bursty, causing the capacity engine to recommend dangerously small pod counts that crash during real-world QPS spikes.

### Accepted Trade-offs:
- Requires measuring and supplying variance parameters ($C_a^2, C_s^2$) in `slo-input.yaml`.

### When this Decision is WRONG:
For perfectly periodic cron-triggered arrival patterns where $C_a^2 = 0$ (zero arrival variance).

---

## Decision 4: Bulkhead Semaphore Limits vs Unbounded In-Flight Request Queuing

### Question:
Why limit in-flight HTTP requests using a counting **Bulkhead Semaphore** (`BulkheadLimit = 500`) instead of allowing unbounded request queuing?

### Why Bulkhead Semaphore (Chosen):
1. **Memory Bound Guarantee**: Little's Law ($L = \lambda W$) proves that memory allocation scales directly with concurrent in-flight requests. Bounding $L \le 500$ guarantees that memory consumption stays strictly under container limits (`512Mi`), preventing Pod `OOMKilled` crashes.
2. **Fast Shedding**: The 501st concurrent request is rejected in $<1\text{ms}$ with `HTTP 429`, allowing upstream load balancers to route traffic to alternative replicas.

### Why Unbounded Request Queuing (Rejected):
Unbounded queues accumulate memory during slow database queries until the Go runtime runs out of heap space, causing `OOMKilled` container restarts that drop ALL in-flight requests.

### Accepted Trade-offs:
- Requests exceeding the bulkhead limit are shed immediately during sudden extreme QPS surges.

### When this Decision is WRONG:
In low-throughput asynchronous background workers where requests can be buffered on disk indefinitely without memory bounds.
