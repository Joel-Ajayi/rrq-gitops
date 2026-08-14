# Cluster Provisioning & Bootstrapping Guide

This document provides step-by-step instructions for provisioning, bootstrapping, and operating RRQ infrastructure across **Production (DigitalOcean DOKS / Production K8s)** and **Local Development (Kind)**.

---

## 1. Production Cluster Provisioning (DOKS / Production K8s)

### Step-by-Step Execution

1. **Provision Managed Kubernetes Cluster**:
   Create a production DOKS cluster via DigitalOcean CLI or Terraform with at least 3 worker nodes (`s-4vcpu-8gb` minimum):
   ```bash
   doctl kubernetes cluster create rrq-prod --count 3 --size s-4vcpu-8gb --region fra1
   ```

2. **Configure `kubectl` Context**:
   ```bash
   doctl kubernetes cluster kubeconfig save rrq-prod
   ```

3. **Deploy Sealed Secrets Controller**:
   ```bash
   kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.27.3/controller.yaml
   ```

4. **Encrypt Production Secrets**:
   Encrypt production credentials using `kubeseal`:
   ```bash
   kubeseal --scope cluster-wide --format yaml < secret.plain.yaml > rrq/secrets/prod/secret.sealed.yaml
   ```

5. **Configure Production Domain in Manifests**:
   Edit `rrq/overlays/prod/services/gateway.yaml` and set the `hostname` attributes to match your production domain (`<your-domain.com>`):
   ```yaml
   listeners:
     - name: api-https
       hostname: "api.<your-domain.com>"
     - name: cluster-https
       hostname: "cluster.<your-domain.com>"
   ```

6. **Apply Root Argo CD Application**:
   ```bash
   kubectl apply -f apps/root-app.yaml
   ```
   Argo CD will automatically discover, fetch, and synchronize all operator components and microservices declared in `rrq/overlays/prod/` across sync waves `-2` through `2`.

7. **Configure Production DNS A Records**:
   Retrieve the external IP address of the provisioned Envoy Gateway LoadBalancer:
   ```bash
   kubectl get svc -n envoy-gateway-system
   ```
   Point your domain's wildcard A record (`*.<your-domain.com>`) to the LoadBalancer IP. The HTTPRoutes and cert-manager ClusterIssuer will automatically terminate SSL/TLS via Let's Encrypt for:
   - `api.<your-domain.com>` $\rightarrow$ Core API Ingress Gateway
   - `cluster.<your-domain.com>` $\rightarrow$ Grafana Executive Dashboard (Tier 1)
   - `growth.<your-domain.com>` $\rightarrow$ Grafana User Journeys Dashboard (Tier 2)
   - `metrics.<your-domain.com>` $\rightarrow$ Grafana Service Health RED Dashboard (Tier 3)
   - `logs.<your-domain.com>` $\rightarrow$ Grafana Middleware USE Dashboard (Tier 4)
   - `traces.<your-domain.com>` $\rightarrow$ Grafana Infrastructure USE Dashboard (Tier 5)
   - `prometheus.<your-domain.com>` $\rightarrow$ Prometheus UI

---

## 2. Local Development Bootstrap (Kind)

### Prerequisites
- **Docker Engine** (running)
- **Kind** (`v0.31.0+`), **kubectl** (`v1.31+`), **Helm** (`v3.17+`), **Kustomize** (`v5.6+`)

### Step-by-Step Execution

1. **Clone the Repository**:
   ```bash
   git clone https://github.com/Joel-Ajayi/rrq-gitops.git
   cd rrq-gitops
   ```

2. **Create Local Kind Cluster**:
   Creates a 3-worker node Kind cluster configured for port forwarding (host port 8080/8443):
   ```bash
   make cluster-up
   ```

3. **Install Argo CD**:
   Deploys Argo CD into the `argocd` namespace:
   ```bash
   make argocd
   ```

4. **Bootstrap Dev Infrastructure**:
   Applies Sealed Secrets, infrastructure operators (CNPG, Strimzi, Redis, Envoy Gateway), and waits for pod readiness:
   ```bash
   make bootstrap-dev
   ```

5. **Verify Running Services & Endpoints**:
   Local Envoy Gateway exposes traffic on host ports `8080` (HTTP) and `8443` (HTTPS):
   - `http://localhost:8080/v1/transfers` — Core API Endpoint
   - `http://localhost:8080/executive` — Grafana Executive Dashboard
   - `http://localhost:8080/services` — Grafana Services RED Dashboard
   - *(Optional)* Add `127.0.0.1 api.rrq.dev` to `/etc/hosts` for domain resolution.

---

## 3. Useful Operational Commands

- **Check Argo CD App Sync Status**:
  ```bash
  kubectl get applications -n argocd
  ```

- **Force Manual Argo CD Sync**:
  ```bash
  argocd app sync rrq-prod --prune
  ```

- **Inspect Postgres Cluster Status**:
  ```bash
  kubectl cnpg status shard-a -n rrq
  ```
