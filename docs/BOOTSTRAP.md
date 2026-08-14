# Cluster Provisioning & Bootstrapping Guide

This document provides step-by-step instructions for provisioning, bootstrapping, and operating RRQ infrastructure across **Production (DigitalOcean DOKS / Production K8s)** and **Local Development (Kind)**.

---

## 1. Production Cluster Provisioning (DOKS / Production K8s)

### Prerequisites
- A managed Kubernetes cluster with at least 3 worker nodes (`s-4vcpu-8gb` minimum).
- `kubectl` configured to point at the cluster.
- All GitOps tools installed (`make tools`).

### Step-by-Step Execution

1. **Provision Managed Kubernetes Cluster**:
   Create a production DOKS cluster via DigitalOcean CLI or Terraform:
   ```bash
   doctl kubernetes cluster create rrq-prod --count 3 --size s-4vcpu-8gb --region fra1
   ```

2. **Configure `kubectl` Context**:
   ```bash
   doctl kubernetes cluster kubeconfig save rrq-prod
   ```

3. **Prepare Plaintext Secrets**:
   Create plaintext secret files in `secrets/prod/` (these are git-ignored):
   ```bash
   # Example: secrets/prod/workloads.plain.yaml
   # Contains database passwords, JWT keys, Redis credentials, etc.
   ```

4. **Bootstrap the Cluster**:
   A single command installs Argo CD, applies the Root App-of-Apps, waits for the Sealed Secrets operator, and encrypts secrets:
   ```bash
   make bootstrap ENV=prod
   ```

   This executes the following sequence:
   1. Installs Argo CD via Helm (`helm upgrade --install argocd`)
   2. Applies `bootstrap/root-app-prod.yaml` (Root Application)
   3. Waits for `sealed-secrets` deployment to become healthy
   4. Runs `make seal ENV=prod` to encrypt all plaintext secrets

   Argo CD then takes over and reconciles the entire cluster through sync waves:
   - **Wave -2**: All operators (Sealed Secrets, CNPG, Strimzi, Envoy Gateway, KEDA, cert-manager, ECK, kube-prometheus-stack, OTel Operator)
   - **Wave 0**: Datastores (PostgreSQL clusters, Kafka, Redis)
   - **Wave 1**: Observability (OTel collectors, dashboards, ServiceMonitors, Portainer)
   - **Wave 2**: Workloads (microservices, Gateway routes, migrations)

5. **Configure Production DNS**:
   Retrieve the external IP of the Envoy Gateway LoadBalancer:
   ```bash
   kubectl get svc -n envoy-gateway-system
   ```
   Point your domain's wildcard A record (`*.<your-domain.com>`) to the LoadBalancer IP. Production endpoints:
   - `https://api.<your-domain.com>/v1/transfers` — Core API Gateway
   - `https://cluster.<your-domain.com>` — Portainer Cluster Management UI
   - `https://metrics.<your-domain.com>` — Grafana Dashboards
   - `https://logs.<your-domain.com>` — Kibana Log Analytics
   - `https://traces.<your-domain.com>` — Jaeger Distributed Tracing
   - `https://prometheus.<your-domain.com>` — Prometheus UI

---

## 2. Local Development Bootstrap (Kind)

### Key Difference from Production
Local dev **bypasses Argo CD entirely**. Instead of GitOps reconciliation from a remote Git repo, `make bootstrap ENV=dev` directly applies Kustomize overlays via sequential `kubectl apply -k` calls. This allows uncommitted local changes to take effect immediately.

### Prerequisites
- **Docker Engine** (running)
- **Kind** (`v0.31.0+`), **kubectl** (`v1.31+`), **Helm** (`v3.17+`)
- Install all tools: `make tools`

### Step-by-Step Execution

1. **Clone the Repository**:
   ```bash
   git clone https://github.com/Joel-Ajayi/rrq-gitops.git
   cd rrq-gitops
   ```

2. **Create Local Kind Cluster**:
   Creates a 3-worker node Kind cluster with port forwarding (host ports 8080/8443):
   ```bash
   make cluster-up
   ```

3. **Bootstrap Dev Infrastructure**:
   Sequentially applies all overlays in strict wave order without Argo CD:
   ```bash
   make bootstrap ENV=dev
   ```

   This executes:
   1. `kubectl apply -k overlays/dev/operators` — Installs all operators
   2. Waits for `sealed-secrets` to become healthy
   3. `make seal ENV=dev` — Encrypts dev secrets
   4. `kubectl apply -k overlays/dev/datastores` — Deploys databases
   5. `kubectl apply -k overlays/dev/observability` — Deploys monitoring
   6. `kubectl apply -k overlays/dev/workloads` — Deploys microservices

4. **Verify Running Services**:
   Local Envoy Gateway exposes traffic on host ports `8080` (HTTP) and `8443` (HTTPS):
   - `http://localhost:8080/v1/transfers` — Core API Endpoint
   - `http://cluster.127.0.0.1.nip.io:8080` — Portainer
   - `http://metrics.127.0.0.1.nip.io:8080/services` — Grafana
   - *(Optional)* Add `127.0.0.1 api.rrq.dev` to `/etc/hosts` for domain resolution.

---

## 3. Secrets Workflow

Secrets are managed via the `make seal` target:

1. **Create plaintext secret files** in `secrets/<env>/`:
   ```
   secrets/dev/workloads.plain.yaml
   secrets/dev/datastores.plain.yaml
   secrets/prod/workloads.plain.yaml
   secrets/prod/datastores.plain.yaml
   ```
   These files are git-ignored (`.gitignore` contains `*.plain.yaml`).

2. **Encrypt with `make seal`**:
   ```bash
   make seal ENV=dev   # or ENV=prod
   ```
   This runs `kubeseal` against each plaintext file and writes the encrypted `SealedSecret` to the corresponding overlay directory (e.g., `overlays/dev/workloads/secrets.yaml`).

3. **Commit the encrypted SealedSecrets** — they are safe to push to Git.

---

## 4. Useful Operational Commands

```bash
# Check Argo CD application sync status
kubectl get applications -n argocd

# Force manual Argo CD sync
argocd app sync operators --prune

# Inspect Postgres cluster status
kubectl cnpg status shard-a -n rrq

# Dry-run: render all manifests without applying
make render ENV=prod

# Tear down local cluster
make cluster-down
```
