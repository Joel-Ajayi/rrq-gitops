# RRQ GitOps Infrastructure

This repository is the single source of truth for the **Infrastructure as Code (IaC)** and **Declarative GitOps** state of the RRQ (River Rust Queue) payment processing core.

It strictly decouples the platform infrastructure and deployment lifecycle from the application code (which lives in the [`river-rust-queue`](../river-rust-queue) repository), allowing operations and development to scale independently.

---

## Philosophy

We strictly adhere to the GitOps operating model:
1. **Declarative**: The entire system (from databases to Kafka brokers to microservice replicas) is described declaratively in Kubernetes YAML and Kustomize overlays.
2. **Versioned**: Every change to the infrastructure is a Git commit. Git is the authoritative control plane.
3. **Automated (Pull)**: **Argo CD** constantly monitors this repository and pulls changes into the cluster, applying them automatically. No human or CI pipeline runs `kubectl apply` in production.
4. **Self-Healing**: If state in the cluster drifts from this repository, Argo CD automatically overwrites the cluster to match Git.

---

## Architecture Overview

This repository uses the **"App of Apps"** pattern for Argo CD and leverages **Kustomize** to manage environment overlays cleanly.

### Directory Structure

| Path | Purpose |
| --- | --- |
| `apps/` | The root Argo CD `Application` manifests that bootstrap the cluster. |
| `rrq/base/` | The pure, un-configured infrastructure building blocks (Postgres, Kafka, Microservices, Observability). |
| `rrq/overlays/dev/` | Configurations specific to local developer environments (reduced replicas, no TLS). |
| `rrq/overlays/prod/` | Configurations specific to production (HA replicas, Sealed Secrets, TLS ingress). |
| `bootstrap/` | (Legacy) Reserved for initial cluster setup configurations. |
| `Makefile` | Tooling to bootstrap the local GitOps environment. |

### Core Technologies

The platform relies on the following Kubernetes operators, configured via public Helm charts and customized in this repository:
- **CloudNativePG**: Manages HA PostgreSQL clusters (`merchants-db`, `shard-a`, `shard-b`).
- **Strimzi**: Manages Kafka brokers using modern KRaft mode.
- **KEDA**: Event-driven autoscaling based on Kafka consumer lag.
- **Kong**: Ingress Edge Router for path stripping and TLS termination.
- **Bitnami Redis**: Ephemeral state for velocity checks.
- **OpenTelemetry**: Auto-instrumentation and trace collection.

---

## Quick Start: Local Development

For the local inner-loop, we run a hybrid model: you use this repository to bootstrap the heavy stateful infrastructure *once*, and then use Skaffold in the application repo to rapidly hot-reload code.

1. **Bootstrap the Platform:**
   ```bash
   cd rrq-gitops
   make bootstrap-dev
   ```
   This spins up a local `kind` cluster, installs Argo CD, and instructs it to apply the `apps/dev/infrastructure.yaml` manifest. Argo CD will reach out to public Helm registries to install all databases and message brokers.

2. **Run the Application:**
   ```bash
   cd ../river-rust-queue
   make dev
   ```
   Skaffold will build your Go images, apply database migrations, and hot-load the microservices into the cluster dynamically.

---

## Quick Start: Production Setup

For production, you do not need to manually run `kubectl apply`. Instead, you use the Makefile which installs Argo CD into your production cluster and points it at this repository.

**Prerequisites:**
- You have provisioned a production Kubernetes cluster (e.g., EKS, GKE, AKS).
- Your active `kubectl` context is pointing to that production cluster.

1. **Bootstrap Production:**
   ```bash
   cd rrq-gitops
   make bootstrap-prod
   ```
   *This command installs Argo CD, and applies the `apps/prod/infrastructure.yaml` and `apps/prod/rrq-app.yaml` manifests. Argo CD will immediately take over and sync all databases, brokers, and application deployments from this repository.*

---

## Production CI/CD Deployment Flow

1. **Application CI**: A developer merges code into the `river-rust-queue` repository.
2. **Image Build**: The CI pipeline builds the Docker image and pushes it to GHCR.
3. **GitOps Trigger**: The CI pipeline modifies `rrq/base/services/kustomization.yaml` in **this** repository to update the image tag, and pushes a commit.
4. **Argo CD Sync**: Argo CD detects the new commit in `rrq-gitops`, pulls the Kustomize manifests, and seamlessly performs a rolling update on the production cluster.

---

## License

[MIT](LICENSE).
