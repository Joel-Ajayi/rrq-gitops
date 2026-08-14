# Capacity Planning Engine

The capacity planning engine dynamically derives microservice timeouts, token bucket retry budgets, worker concurrency pools, HPA replica bounds, and database connection limits directly from SLO targets and physical infrastructure constraints.

---

## How to Run

Execute the engine directly using `go run`:

```bash
go run . slo-input.yaml
```

Alternatively, build a standalone binary:

```bash
go build -o capacity .
./capacity slo-input.yaml
```

---

## Input

- **`slo-input.yaml`**: Authoritative configuration file declaring target QPS, latency SLOs ($W$), variance parameters ($C_s^2 / C_a^2$), PostgreSQL instance profiles, Kafka broker limits, and pod CPU/memory limits.

---

## Output

1. **Terminal Console Report**: Displays infrastructure supply ceilings, per-service derived parameters (latency, CPU requests, retry budgets, session timeouts), and fit-check pass/fail status.
2. **`capacity-output.yaml`**: Serialized debug output containing snake_case derived properties across `ceilings`, `kafka_cap` (including aggregated `topics`), `redis_cap`, and `services`.
3. **Kustomize ConfigMaps**: Automatically updates Kustomize environment configuration in `../rrq/base/config/` for GitOps deployment.
