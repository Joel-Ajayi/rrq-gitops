# RRQ GitOps Infrastructure

[![Argo CD](https://img.shields.io/badge/managed_by-Argo_CD-blue?logo=argo)](https://argoproj.github.io/cd/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

This repository is the single source of truth for the **Infrastructure as Code (IaC)** and **Declarative GitOps** state of the RRQ (River Rust Queue) payment processing core.

It strictly decouples the platform infrastructure and deployment lifecycle from the application code (which lives in the [`river-rust-queue`](https://github.com/Joel-Ajayi/river-rust-queue) repository), allowing operations and development to scale independently.

---

## Documentation Quick Links

- [GitOps Architecture Specification](docs/ARCHITECTURE.md) — Detailed guide on Argo CD "App of Apps", sync waves, namespace isolation, and operators.
- [Cluster Provisioning & Bootstrap Guide](docs/BOOTSTRAP.md) — Step-by-step cluster setup instructions for local Kind dev and production DOKS environments.
- [Infrastructure Operational Runbooks](docs/RUNBOOKS.md) — SRE procedures for database failovers, Kafka partition scaling, and secret rotation.
- [Security & Network Policy Matrix](docs/SECURITY.md) — Multi-layer security model, default-deny ingress/egress, and sealed secret standards.
- [Capacity Planning Engine Guide](capacity/README.md) — Capacity model inputs (`slo-input.yaml`), formulas, and generated outputs.

---

## Core Philosophy

We strictly adhere to the GitOps operating model:

1. **Declarative**: The entire system (from databases to Kafka brokers to microservice replicas) is described declaratively in Kubernetes YAML and Kustomize overlays.
2. **Versioned**: Every change to the infrastructure is a Git commit. Git is the authoritative control plane.
3. **Automated (Pull)**: **Argo CD** constantly monitors this repository and pulls changes into the cluster, applying them automatically. No human or CI pipeline runs `kubectl apply` in production.
4. **Self-Healing**: If state in the cluster drifts from this repository, Argo CD automatically overwrites the cluster to match Git.

---

## Quickstart

### Prerequisites
- Docker Engine
- Kind (`v0.31.0+`)
- kubectl (`v1.31+`), Helm (`v3.17+`), Kustomize (`v5.6+`)

### Bootstrap Local Dev Cluster

```bash
# 1. Create Kind cluster
make cluster-up

# 2. Install Argo CD
make argocd

# 3. Bootstrap infrastructure & operators
make bootstrap-dev
```

---

## License

[MIT](LICENSE).
