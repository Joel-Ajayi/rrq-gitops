# Infrastructure Security & Network Policy Specification

This document details the security posture, network isolation boundaries, RBAC policies, and secret encryption standards for the RRQ GitOps control plane.

---

## 1. Multi-Layer Security Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Layer 1: Edge Security (Envoy Gateway)                     │
│  - TLS Termination                                          │
│  - Edge JWT Signature Verification (SecurityPolicy)         │
│  - Rate Limiting                                            │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               v
┌─────────────────────────────────────────────────────────────┐
│  Layer 2: Network Isolation (Kubernetes NetworkPolicies)    │
│  - Default-Deny Ingress across all namespaces               │
│  - Explicit pod-to-pod ingress allow rules                  │
│  - Default-Deny Egress in Production                        │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               v
┌─────────────────────────────────────────────────────────────┐
│  Layer 3: Pod Security Standards (Restricted Profile)       │
│  - runAsNonRoot: true (UID 1000)                            │
│  - readOnlyRootFilesystem: true                             │
│  - allowPrivilegeEscalation: false                          │
│  - capabilities: drop [ALL]                                 │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. Network Policy Matrix

RRQ applies a **Default-Deny Ingress** policy across all namespaces (`rrq`, `observability`, `cnpg-system`). Communication is granted via explicit NetworkPolicy rules:

| Policy Name | Target Selector | Allowed Source | Allowed Ports | Purpose |
|---|---|---|---|---|
| `default-deny-ingress` | `{}` (all pods) | None | None | Blocks all unauthorized ingress by default. |
| `allow-ingress-to-core-api` | `app: core-api` | `namespace: envoy-gateway-system` | `8080` (http), `9090` (metrics) | Permits Envoy Gateway proxy traffic to REST API. |
| `allow-metrics-scrape` | `{}` (all pods) | `namespace: observability` | `9090` (prometheus metrics) | Allows Prometheus server to collect telemetry. |
| `allow-intra-namespace` | `{}` (all pods) | `namespace: rrq` | All internal ports | Permits pod-to-pod communication within `rrq`. |
| `allow-egress-external-webhook` | `app: webhook-worker` | External Internet | `443` (https), `80` (http) | Permits webhook worker to deliver payloads to merchant URLs. |

---

## 3. Secret Management & Encryption at Rest

1. **Sealed Secrets**: Secret manifests are encrypted using the cluster's public asymmetric sealing key via `kubeseal`.
2. **Git Safety**: Encrypted `SealedSecret` manifests are safe to commit to Git. Plaintext secrets (`secret.plain.yaml`) and `.env` files are ignored by `.gitignore`.
3. **In-Cluster Decryption**: The Sealed Secrets controller running in `sealed-secrets` decrypts `SealedSecret` resources into native Kubernetes `Secret` objects inside the destination namespace.

---

## 4. Pod Security Context Standard

All microservices declared in `rrq/base/services/` conform to the Kubernetes **Restricted Pod Security Standard**:

```yaml
securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 1000
  fsGroup: 1000
```
