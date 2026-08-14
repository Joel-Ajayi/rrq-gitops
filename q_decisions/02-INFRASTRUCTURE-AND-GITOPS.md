# Questioning Decisions: Infrastructure & GitOps Architecture

This document explicitly questions every major infrastructure choice made in RRQ's deployment model, detailing **why X was chosen**, **why alternative Y was rejected**, **what trade-offs were accepted**, and **when the decision would be wrong**.

---

## Decision 1: Two-Repository GitOps Split vs Monorepo

### Question:
Why split the system into two repositories (`river-rust-queue` for app code, `rrq-gitops` for infrastructure) instead of keeping everything in a single monorepo?

### Why Two-Repo Split (Chosen):
1. **Access Control & RBAC**: Developers push app logic to `river-rust-queue`. SREs manage production scaling, secrets, and NetworkPolicies in `rrq-gitops`.
2. **CI Pipeline Isolation**: App CI builds images and executes unit tests. GitOps CI validates Kustomize YAML. Updating a Grafana dashboard or HPA config in `rrq-gitops` does NOT trigger application image rebuilds.
3. **Clean Audit Log**: `git log` in `rrq-gitops` reflects pure production state modifications.

### Why Monorepo (Rejected):
Keeping Kubernetes manifests alongside app code in a single repo creates noisy CI cycles (every documentation edit triggers image build steps) and risks accidental modifications to production manifests during feature development.

### Accepted Trade-offs:
- Requires a multi-repository promotion step in CI (`gitops-promote` job using Personal Access Tokens).

### When this Decision is WRONG:
In small early-stage teams with 1-2 engineers where managing multi-repo CI secrets and promotion scripts adds unnecessary overhead.

---

## Decision 2: Kustomize vs Helm for Application Manifest Overlays

### Question:
Why use Kustomize for application environment overlays (`dev`, `prod`) instead of Helm charts?

### Why Kustomize (Chosen):
1. **Pure Declarative Overlays**: Kustomize uses pure YAML patch transformations (`kustomize build`). What you see in the overlay is deterministic.
2. **Native Kubernetes Tooling**: Built directly into `kubectl` (`kubectl apply -k`). No Tiller, no Helm release history ConfigMaps, and no template syntax bugs.

### Why Helm (Rejected):
Helm charts use string-based Go templating (`{{ if .Values.enabled }}`). Complex control logic in Helm charts makes manifests difficult to diff, debug locally, and validate cleanly in CI before deployment.

### Accepted Trade-offs:
- Parameterizing deep values across dozens of microservices requires managing YAML patch files rather than changing a single `values.yaml` variable.

### When this Decision is WRONG:
When packaging software for distribution to third-party external customers who need to install your software in diverse Kubernetes clusters with arbitrary customization requirements.

---

## Decision 3: CloudNativePG Operator vs Patroni or Managed Cloud Postgres

### Question:
Why use CloudNativePG (CNPG) to run PostgreSQL inside Kubernetes instead of Patroni or a managed cloud database like AWS RDS / GCP Cloud SQL?

### Why CloudNativePG (Chosen):
1. **Zero External DCS Dependency**: Patroni requires an independent Etcd or Consul cluster for Distributed Consensus Store (DCS). CNPG uses native Kubernetes API primitives (`Cluster` CRD), eliminating Etcd operational overhead.
2. **Native S3 Barman Backups**: Backup and Point-In-Time Recovery (PITR) are declared directly in the CRD and streamed continuously to S3.
3. **Multi-Cloud Portability**: Identical Postgres setup runs locally in Kind, on DigitalOcean DOKS, or on AWS EKS without vendor lock-in.

### Why Patroni or Managed Cloud Postgres (Rejected):
- **Patroni**: Managing an independent Etcd cluster just for database leader election adds high operational complexity.
- **Managed RDS**: Introduces cloud vendor lock-in, higher cost at scale, and lacks local development parity with Kind.

### Accepted Trade-offs:
- SRE team bears full responsibility for storage volume IOPS monitoring and database operator upgrades.

### When this Decision is WRONG:
If the organization lacks in-house database administration skills and requires a 24/7 cloud vendor SLA guarantee for database engine maintenance.

---

## Decision 4: Strimzi KRaft Kafka vs ZooKeeper-based Kafka

### Question:
Why run Strimzi Kafka in KRaft mode (Kafka Raft Metadata) instead of traditional ZooKeeper mode?

### Why KRaft Mode (Chosen):
1. **Eliminates ZooKeeper Cluster**: Saves 3 stateful nodes, simplifies backup strategies, and reduces cluster memory overhead by ~3GB.
2. **Sub-Second Leader Re-election**: Partition leader re-elections complete in $<100\text{ms}$ during broker restarts, compared to multi-second ZooKeeper metadata sync stalls.

### Why ZooKeeper Mode (Rejected):
ZooKeeper introduces dual-cluster maintenance bugs, complex quorum recovery during network partitions, and is officially deprecated in modern Kafka versions.

### Accepted Trade-offs:
- KRaft mode in older Kafka versions (<3.0) was less battle-tested than ZooKeeper. (RRQ uses Kafka 3.9+).

### When this Decision is WRONG:
Only when maintaining legacy Kafka clusters (<2.8) where KRaft mode is unavailable or marked experimental.

---

## Decision 5: Envoy Gateway (Gateway API) vs Traditional NGINX Ingress Controller

### Question:
Why choose Envoy Gateway implementing the Kubernetes Gateway API standard over NGINX Ingress Controller?

### Why Envoy Gateway (Chosen):
1. **Edge JWT Offloading**: Envoy Gateway handles JWT signature verification at the proxy edge via `SecurityPolicy` CRDs, dropping invalid requests in $<1\text{ms}$ before reaching application pods.
2. **Standardized Role-Oriented CRDs**: Separates `GatewayClass` (platform), `Gateway` (ops), and `HTTPRoute` (app developers).

### Why NGINX Ingress (Rejected):
NGINX Ingress relies on non-standard, annotation-heavy configuration (`nginx.ingress.kubernetes.io/*`), Lua plugin scripts for custom auth, and monolithic proxy reloads that drop active connections under heavy configuration churn.

### Accepted Trade-offs:
- Envoy Gateway CRDs (`gateway.networking.k8s.io`) are newer than legacy Ingress resources.

### When this Decision is WRONG:
Simple legacy Kubernetes clusters where basic host-based HTTP routing is sufficient and Gateway API controllers are not installed.
