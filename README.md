# RRQ GitOps Infrastructure

[![Argo CD](https://img.shields.io/badge/managed_by-Argo_CD-blue?logo=argo)](https://argoproj.github.io/cd/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

This repository is the single source of truth for the **Infrastructure as Code (IaC)** and **Declarative GitOps** state of the RRQ (River Rust Queue) payment processing core.

It strictly decouples the platform infrastructure and deployment lifecycle from the application code (which lives in the [`river-rust-queue`](https://github.com/Joel-Ajayi/river-rust-queue) repository), allowing operations and development to scale independently.

---

## Documentation Quick Links

- [GitOps Architecture Specification](docs/ARCHITECTURE.md) — Argo CD App-of-Apps, sync waves, operator/CR separation, and namespace isolation.
- [Cluster Provisioning & Bootstrap Guide](docs/BOOTSTRAP.md) — Step-by-step cluster setup for production and local Kind dev environments.
- [Infrastructure Operational Runbooks](docs/RUNBOOKS.md) — SRE procedures for database failovers, Kafka partition scaling, and secret rotation.
- [Security & Network Policy Matrix](docs/SECURITY.md) — Multi-layer security model, default-deny ingress/egress, and sealed secret standards.
- [Observability Stack Guide](base/observability/README.md) — OTel agents/gateway, trace, metrics, log pipelines, and architecture diagram.
- [Grafana Dashboards Guide](base/observability/dashboards/README.md) — Persona-driven 5-Tier dashboard taxonomy (RED/USE methodology).
- [Capacity Planning Engine Guide](tools/capacity-engine/README.md) — Capacity model inputs, formulas, and generated outputs.

---

## Repository Structure

```
rrq-gitops/
├── apps/                          # Argo CD Application manifests (production)
│   ├── 00-sealed-secrets.yaml     #   Wave -3: Secret decryption operator
│   ├── 00-operators.yaml          #   Wave -2: CRD-installing operators
│   ├── 01-datastores.yaml         #   Wave  0: Stateful data clusters
│   ├── 01-observability.yaml      #   Wave  1: Monitoring & telemetry
│   └── 02-workloads.yaml          #   Wave  2: Application microservices
├── bootstrap/
│   └── root-app.yaml              # Root App-of-Apps (points to apps/)
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
│   ├── dev/                       #   Local Kind cluster overrides
│   └── prod/                      #   Production cluster overrides
├── secrets/                       # Plaintext secrets (git-ignored, used by `make seal`)
│   ├── dev/
│   └── prod/
├── kind/                          # Kind cluster configuration
├── tools/
│   ├── capacity-engine/           # Go-based capacity planning tool
│   │   └── README.md              #   Engine input, formulas & file index
│   └── load-tests/                # k6 load test scenarios
└── Makefile                       # All GitOps operations
```

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

| Wave | Phase | What Deploys |
|------|-------|-------------|
| `-2` | Operators | Sealed Secrets, CNPG, Strimzi, Envoy Gateway, KEDA, cert-manager, ECK, kube-prometheus-stack, OTel Operator |
| `0`  | Datastores | PostgreSQL clusters, Kafka cluster & topics, Redis |
| `1`  | Observability | OTel collectors, Grafana dashboards, ServiceMonitors, Portainer, Elasticsearch, Kibana |
| `2`  | Workloads | core-api, ledger-worker, outbox-relay, webhook-worker, fraud-worker, Gateway & HTTPRoutes |

### Local Development (Kind)

Local dev **bypasses Argo CD entirely** — `kubectl apply -k` is used directly against local files so uncommitted changes are immediately reflected:

```bash
# 1. Create 3-worker local Kind cluster
make cluster-up

# 2. Bootstrap local dev (sequential kubectl apply, no Argo CD)
make bootstrap ENV=dev
```

#### Local Endpoints
Local Envoy Gateway maps NodePorts to host ports `8080` (HTTP) and `8443` (HTTPS):
- **API Gateway**: `http://localhost:8080/v1/transfers`
- **Portainer**: `http://cluster.127.0.0.1.nip.io:8080`
- **Grafana**: `http://metrics.127.0.0.1.nip.io:8080/services`

---

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make tools` | Install all GitOps CLIs (kubectl, helm, kind, kubeseal, argocd, skaffold, k6, jq, yq) |
| `make cluster-up` | Create local Kind cluster |
| `make cluster-down` | Delete local Kind cluster |
| `make bootstrap ENV=<dev\|prod>` | Full cluster bootstrap (dispatches to dev or prod) |
| `make argocd` | Install Argo CD via Helm |
| `make seal ENV=<dev\|prod>` | Encrypt plaintext secrets into SealedSecrets |
| `make render ENV=<dev\|prod>` | Dry-run: print fully-rendered Kustomize manifests |
| `make capacity` | Regenerate GitOps manifests from capacity models |
| `make bench SCENARIO=<name>` | Run a k6 load test scenario |

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
