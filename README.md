# RRQ GitOps Infrastructure

[![Argo CD](https://img.shields.io/badge/managed_by-Argo_CD-blue?logo=argo)](https://argoproj.github.io/cd/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

This repository is the single source of truth for the **Infrastructure as Code (IaC)** and **Declarative GitOps** state of the RRQ (River Rust Queue) payment processing core.

It strictly decouples the platform infrastructure and deployment lifecycle from the application code (which lives in the [`river-rust-queue`](https://github.com/Joel-Ajayi/river-rust-queue) repository), allowing operations and development to scale independently.

---

## Documentation Quick Links

- [GitOps Architecture Specification](docs/ARCHITECTURE.md) — Argo CD App-of-Apps, sync waves, operator/CR separation, and namespace isolation.
- [Cluster Provisioning & Bootstrap Guide](docs/BOOTSTRAP.md) — Step-by-step cluster setup for production and local Kind dev environments.
- [Production Capacity & Sizing Guide](overlays/prod/README.md) — Enterprise-scale 3-node HA capacity specifications and resource allocations.
- [Local Production Capacity & Sizing Guide](overlays/local/README.md) — Workstation right-sized HA capacity specifications (20 thread / 25 GiB RAM target).
- [Infrastructure Operational Runbooks](docs/RUNBOOKS.md) — SRE procedures for database failovers, Kafka partition scaling, and secret rotation.
- [Security & Network Policy Matrix](docs/SECURITY.md) — Multi-layer security model, default-deny ingress/egress, and sealed secret standards.
- [Observability Stack Guide](base/observability/README.md) — OTel agents/gateway, trace, metrics, log pipelines, and architecture diagram.
- [Grafana Dashboards Guide](base/observability/dashboards/README.md) — Persona-driven 5-Tier dashboard taxonomy (RED/USE methodology).
- [Capacity Planning Engine Guide](tools/capacity-engine/README.md) — Queueing theory models (M/M/c & Kingman), Little's Law, and automated manifest generation.
- [Load Testing & Benchmark Suite](tools/load-tests/README.md) — k6 load tests (`smoke`, `full_workload`, `stress`, `spike`), token refresh, and DLQ batch replay.

---

## Repository Structure

```
rrq-gitops/
├── bootstrap/
│   ├── root-app.yaml              # Production ApplicationSet (generates all sync waves)
│   └── root-app-local.yaml        # Local Production ApplicationSet (generates all sync waves)
├── base/                          # Shared base manifests (environment-agnostic)
│   ├── platform/
│   │   ├── operators/             #   All Helm operator charts (CRD installers)
│   │   └── datastores/            #   Postgres clusters, Kafka cluster & topics
│   ├── observability/             #   OTel collectors, dashboards, ServiceMonitors
│   │   ├── README.md              #   Observability stack & architecture diagram
│   │   └── dashboards/
│   │       └── README.md          #   5-Tier Grafana dashboard taxonomy
│   └── workloads/                 #   Microservice deployments, Gateway CRs, migrations
├── overlays/                      # Environment-specific customizations
│   ├── dev/                       #   Local Kind cluster overrides (minimal non-HA)
│   ├── local/                     #   Local Kind cluster overrides (right-sized HA production)
│   └── prod/                      #   Production cluster overrides (enterprise cloud scale)
├── secrets/                       # Plaintext secrets (git-ignored, used by `make seal`)
│   ├── dev/
│   ├── local/
│   └── prod/
├── kind/                          # Kind cluster configurations
│   ├── cluster-dev.yaml
│   └── cluster-local.yaml
├── tools/
│   ├── capacity-engine/           # Go-based analytical capacity sizing engine
│   │   └── README.md              #   Engine inputs, mathematical models & manifest patchers
│   └── load-tests/                # k6 load testing & verification suite
│       └── README.md              #   Scenario guide, metric gathering & DLQ replay
└── Makefile                       # All GitOps operations
```

---

## Key Infrastructure & Performance Benchmarks

All microservices, databases, and message brokers are sized via the **Queueing Theory Capacity Engine** (`tools/capacity-engine/`) and verified with empirical k6 load benchmarks:

| Sizing Dimension / Benchmark | Engine Formulation & Constraint | Measured Live Performance |
| :--- | :--- | :--- |
| **Peak Ingress Capacity** | Kingman $G/G/1$ Heavy Traffic Approximation | **$3,000\text{ RPS}$ sustained burst** |
| **Transactional Outbox Drain** | AIMD Adaptive Window Buffer | **$\approx 1,000\text{ events/sec}$** with $< 8\%$ Kafka buffer fill |
| **PostgreSQL Connection Pool** | Fair-share allocation ($\le 239\text{ max\_conns}$) | **$5\text{ RW conns / pod}$** with zero DB pool exhaustion |
| **Kafka `jobs` Topic** | $3,000\text{ RPS peak} / 300\text{ RPS/part}$ | **$10\text{ partitions}$** (Consumer floor: 6 pods) |
| **Kafka `notify` Topic** | $3,000\text{ RPS peak} / 150\text{ RPS/part}$ | **$20\text{ partitions}$** (Consumer floor: 5 pods) |
| **Kafka `xshard.*` Topics** | $1,500\text{ RPS peak} / 100\text{ RPS/part}$ | **$15\text{ partitions}$** (Consumer floor: 6 pods) |
| **Circuit Breaker Shedding** | Fast-fail protection on DB queue saturation | **$< 0.01\text{ ms}$ response** returning `HTTP 503` |
| **Global DLQ Recovery** | Bounded batch replay via `/v1/admin/dlq/replay` | **$100\%$ recovery** ($43/43$ messages reprocessed) |

---

## Quick Start

### Production Deployment

For production Kubernetes environments (DOKS, EKS, GKE):

```bash
# 1. Configure kubectl to point at your production cluster
doctl kubernetes cluster kubeconfig save rrq-prod

# 2. Bootstrap (installs Argo CD, applies Root App, seals secrets)
make bootstrap ENV=prod
```

Argo CD will automatically reconcile the entire cluster using sync waves:

| Wave | Phase         | What Deploys                                                                                                |
| ---- | ------------- | ----------------------------------------------------------------------------------------------------------- |
| `-2` | Operators     | Sealed Secrets, CNPG, Strimzi, Envoy Gateway, KEDA, cert-manager, ECK, kube-prometheus-stack, OTel Operator |
| `0`  | Datastores    | PostgreSQL clusters, Kafka cluster & topics, Redis                                                          |
| `1`  | Observability | OTel collectors, Grafana dashboards, ServiceMonitors, Portainer, Elasticsearch, Kibana                      |
| `2`  | Workloads     | core-api, ledger-worker, outbox-relay, webhook-worker, fraud-worker, Gateway & HTTPRoutes                   |

### Local Production Deployment (Kind with Argo CD)

Runs the complete, right-sized High-Availability production architecture on a local Kind cluster using **Argo CD App-of-Apps GitOps** (exactly mirroring production sync-waves and deployment flow):

```bash
# 1. Create 4-node local Kind cluster (1 control-plane + 3 workers)
make cluster-local

# 2. Bootstrap Local Production (installs Argo CD, applies Local Root App, seals secrets)
make bootstrap ENV=local
```

### Local Development (Kind - Fast Iteration)

Local dev **bypasses Argo CD entirely** — `kubectl apply -k` is used directly against local files so uncommitted changes are immediately reflected:

```bash
# 1. Create 3-worker local Kind cluster
make cluster-up

# 2. Bootstrap local dev (sequential kubectl apply, no Argo CD)
make bootstrap ENV=dev
```

#### Local Endpoints

Local Envoy Gateway maps NodePorts to host ports `8080` (HTTP) and `8443` (HTTPS):

- **API Core Ingress**: `https://api.127.0.0.1.nip.io:8443/v1/transfers`
- **Portainer UI**: `http://cluster.127.0.0.1.nip.io:8080`
- **Business Transactions Dashboard (Tier 1)**: `http://grafana.127.0.0.1.nip.io:8080/executive`
- **User Journeys Dashboard (Tier 2)**: `http://grafana.127.0.0.1.nip.io:8080/journeys`
- **Service Health RED Dashboard (Tier 3)**: `http://grafana.127.0.0.1.nip.io:8080/services`
- **Middleware USE Dashboard (Tier 4)**: `http://grafana.127.0.0.1.nip.io:8080/middleware`
- **Infrastructure USE Dashboard (Tier 5)**: `http://grafana.127.0.0.1.nip.io:8080/infrastructure`

---

## Makefile Targets

| Target                                  | Description                                                                           |
| --------------------------------------- | ------------------------------------------------------------------------------------- |
| `make tools`                            | Install all GitOps CLIs (kubectl, helm, kind, kubeseal, argocd, skaffold, k6, jq, yq) |
| `make cluster-up`                       | Create local Kind cluster (`kind/cluster-$(ENV).yaml`)                                |
| `make cluster-local`                    | Create 4-node local production Kind cluster (`kind/cluster-local.yaml`)               |
| `make cluster-down`                     | Delete local Kind cluster                                                             |
| `make bootstrap ENV=<dev\|local\|prod>` | Full cluster bootstrap (dispatches to dev, local, or prod)                            |
| `make bootstrap-local`                  | Local production GitOps bootstrap via Argo CD                                         |
| `make bootstrap-prod`                   | Cloud production GitOps bootstrap via Argo CD                                         |
| `make bootstrap-dev`                    | Fast local dev bootstrap (sequential kubectl apply, bypasses Argo CD)                 |
| `make argocd`                           | Install Argo CD via Helm                                                              |
| `make seal ENV=<dev\|local\|prod>`      | Encrypt plaintext secrets into SealedSecrets                                          |
| `make render ENV=<dev\|local\|prod>`    | Dry-run: print fully-rendered Kustomize manifests                                     |
| `make capacity`                         | Regenerate GitOps manifests from capacity models                                      |
| `make bench SCENARIO=<name>`            | Run a k6 load test scenario (`smoke`, `full_workload`, `stress`, `spike`)            |

---

## Core Philosophy

1. **Declarative**: The entire system is described declaratively in Kubernetes YAML and Kustomize overlays.
2. **Versioned**: Every change to the infrastructure is a Git commit. Git is the authoritative control plane.
3. **Automated (Pull)**: Argo CD constantly monitors this repository and pulls changes into the cluster.
4. **Self-Healing**: If cluster state drifts from this repository, Argo CD automatically overwrites it to match Git.
5. **Operator/CR Separation**: All CRD-installing operators deploy in Wave -2. Custom Resources that depend on those CRDs deploy in later waves, guaranteeing zero race conditions.

---

## License

[MIT](LICENSE).
