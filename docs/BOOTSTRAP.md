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

5. **Apply Root Argo CD Application**:
   ```bash
   kubectl apply -f apps/root-app.yaml
   ```
   Argo CD will automatically discover, fetch, and synchronize all operator components and microservices declared in `rrq/overlays/prod/` across sync waves `-2` through `2`.

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

5. **Verify Running Services**:
   ```bash
   kubectl get pods -A
   ```

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
