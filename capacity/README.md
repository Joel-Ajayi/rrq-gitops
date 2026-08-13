# Capacity Engine

The capacity engine calculates infrastructure sizing, retry budgets, timeouts, and resource allocations based on SLO targets and infrastructure constraints.

## How to Run

You can run the engine directly using `go run` from this directory:

```bash
go run . slo-input.yaml
```

Alternatively, you can build it into an executable first:

```bash
go build -o capacity .
./capacity slo-input.yaml
```

## Input

- `slo-input.yaml`: The YAML configuration file containing your SLOs, expected peak QPS, average query times, and infrastructure bounds.

## Output

The engine prints a capacity check summary (Supply, Derived limits, and Fit-Check results) to standard output. 

If the capacity check passes, it automatically renders the derived values and patches the Kustomize manifests in your base cluster configuration, typically located in:
- `../rrq/base/config/` (ConfigMaps)
- `../rrq/base/services/` (KEDA ScaledObjects, Deployments)
- `../rrq/base/kafka/` (Topics)
