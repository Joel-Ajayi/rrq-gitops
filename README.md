# RRQ GitOps Infrastructure

[![Argo CD](https://img.shields.io/badge/managed_by-Argo_CD-blue?logo=argo)](https://argoproj.github.io/cd/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

This repository is the single source of truth for the **Infrastructure as Code (IaC)** and **Declarative GitOps** state of the RRQ (River Rust Queue) payment processing core.

It strictly decouples the platform infrastructure and deployment lifecycle from the application code (which lives in the [`river-rust-queue`](https://github.com/Joel-Ajayi/river-rust-queue) repository), allowing operations and development to scale independently.

---

## Documentation Quick Links

- [GitOps Architecture Specification](docs/ARCHITECTURE.md) — Detailed guide on Argo CD "App of Apps", sync waves, namespace isolation, and operators.
- [Cluster Provisioning & Bootstrap Guide](docs/BOOTSTRAP.md) — Step-by-step cluster setup instructions for production DOKS and local Kind dev environments.
- [Infrastructure Operational Runbooks](docs/RUNBOOKS.md) — SRE procedures for database failovers, Kafka partition scaling, and secret rotation.
- [Security & Network Policy Matrix](docs/SECURITY.md) — Multi-layer security model, default-deny ingress/egress, and sealed secret standards.
- [Capacity Planning Engine Guide](capacity/README.md) — Capacity model inputs (`slo-input.yaml`), formulas, and generated outputs.

---

## Deployment & Development

### 1. Production Kubernetes Deployment (DOKS / Production Cluster)

For production Kubernetes environments (e.g. DigitalOcean DOKS, EKS, GKE):

1. **Deploy Sealed Secrets Controller**:
   ```bash
   kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.27.3/controller.yaml
   ```
2. **Apply Root App-of-Apps Manifest**:
   Apply the root Argo CD Application:
   ```bash
   kubectl apply -f apps/root-app.yaml
   ```
3. **Configure DNS & LoadBalancer Hostnames**:
   Retrieve the external IP address of the provisioned Envoy Gateway LoadBalancer:
   ```bash
   kubectl get svc -n envoy-gateway-system
   ```
   Point your domain's wildcard A record (`*.rrq.yotstack.tech`) to the LoadBalancer IP address. Production endpoints will be automatically routed and secured via Let's Encrypt TLS:
   - **API Core Gateway**: `https://api.rrq.yotstack.tech/v1/transfers`
   - **Executive Dashboard**: `https://cluster.rrq.yotstack.tech`
   - **User Journeys Dashboard**: `https://growth.rrq.yotstack.tech`
   - **Service Health RED Dashboard**: `https://metrics.rrq.yotstack.tech`
   - **Middleware USE Dashboard**: `https://logs.rrq.yotstack.tech`
   - **Infrastructure USE Dashboard**: `https://traces.rrq.yotstack.tech`
   - **Prometheus UI**: `https://prometheus.rrq.yotstack.tech`

---

### 2. Local Development Quickstart (Kind)

For local development on a 3-worker Kind cluster:

#### Prerequisites
- **Docker Engine**
- **Kind** (`v0.31.0+`), **kubectl** (`v1.31+`), **Helm** (`v3.17+`), **Kustomize** (`v5.6+`)

#### Step-by-Step Local Setup
```bash
# 1. Create 3-worker local Kind cluster
make cluster-up

# 2. Install Argo CD
make argocd

# 3. Bootstrap local dev infrastructure & operators
make bootstrap-dev
```

#### Local Endpoints & Hostnames
Local Envoy Gateway maps NodePorts to host ports `8080` (HTTP) and `8443` (HTTPS):
- **API Gateway Endpoint**: `http://localhost:8080/v1/transfers`
- **Ops Redirect Routes**: `http://localhost:8080/executive`, `/journeys`, `/services`, `/middleware`, `/infrastructure`
- *(Optional)* Map `127.0.0.1 api.rrq.dev` in `/etc/hosts` for domain resolution testing.

---

## Core Philosophy

We strictly adhere to the GitOps operating model:

1. **Declarative**: The entire system (from databases to Kafka brokers to microservice replicas) is described declaratively in Kubernetes YAML and Kustomize overlays.
2. **Versioned**: Every change to the infrastructure is a Git commit. Git is the authoritative control plane.
3. **Automated (Pull)**: **Argo CD** constantly monitors this repository and pulls changes into the cluster, applying them automatically.
4. **Self-Healing**: If state in the cluster drifts from this repository, Argo CD automatically overwrites the cluster to match Git.

---

## License

[MIT](LICENSE).
