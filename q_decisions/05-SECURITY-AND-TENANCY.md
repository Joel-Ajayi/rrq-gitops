# Questioning Decisions: Security & Tenant Isolation Architecture

This document explicitly questions every major security choice made in RRQ, detailing **why X was chosen**, **why alternative Y was rejected**, **what trade-offs were accepted**, and **when the decision would be wrong**.

---

## Decision 1: Argon2id API Key Hashing vs Bcrypt or Fast Hashing (SHA-256)

### Question:
Why store API keys using **Argon2id** (`m=64MB, t=1, p=4`) instead of standard bcrypt or SHA-256?

### Why Argon2id (Chosen):
1. **Memory-Hardness Against GPU Brute-Force**: Argon2id requires 64MB of physical RAM per evaluation. An attacker with a GPU array cannot evaluate millions of hashes per second because GPU memory registers are overwhelmed.
2. **Winner of Password Hashing Competition**: Superior resistance to side-channel timing attacks compared to Argon2i or Argon2d.

### Why Bcrypt or SHA-256 (Rejected):
- **SHA-256**: Dangerously fast ($>10^9\text{ hashes/sec}$ on modern GPUs), rendering leaked hash databases vulnerable to instant rainbow table dictionary attacks.
- **Bcrypt**: Enforces a 72-byte max input limit and lacks configurable memory-hardness parameters.

### Accepted Trade-offs:
- Hashing an API key during initial authentication requires ~64MB memory and ~50ms CPU time on the auth node.

### When this Decision is WRONG:
In resource-constrained microcontrollers or embedded edge devices where 64MB memory allocation per auth request is unavailable.

---

## Decision 2: Asymmetric Ed25519 JWTs vs Symmetric HMAC (HS256)

### Question:
Why sign JWT session tokens using **asymmetric Ed25519** instead of symmetric HMAC SHA-256?

### Why Ed25519 (Chosen):
1. **Private Key Isolation**: The private signing key lives exclusively inside `core-api`. Envoy Gateway verifies tokens using public keys retrieved from `http://core-api:8080/.well-known/jwks.json`.
2. **Zero Proxy Secret Exposure**: If Envoy Gateway or an edge proxy container is compromised, the attacker only acquires the public key and CANNOT forge valid JWT tokens.
3. **High Verification Performance**: Ed25519 signature verification executes in $<20\mu\text{s}$ per token.

### Why Symmetric HMAC (Rejected):
HMAC requires sharing the secret key between `core-api` and Envoy Gateway. Any proxy leak exposes the master signing secret.

### Accepted Trade-offs:
- Ed25519 requires managing quarterly key rotation (`JWT_SIGNING_KEYS`) and exposing a JWKS endpoint.

### When this Decision is WRONG:
When working with legacy API gateway proxies that only support RSA or HMAC JWT algorithms.

---

## Decision 3: HTTP 404 Not Found vs 403 Forbidden for Cross-Tenant Resource Requests

### Question:
When Merchant A attempts to access or transfer funds from a wallet belonging to Merchant B, why return **HTTP 404 Not Found** instead of **HTTP 403 Forbidden**?

### Why HTTP 404 Not Found (Chosen):
1. **Prevents Resource Enumeration**: `HTTP 403` confirms that the resource ID exists but access is denied. `HTTP 404` conceals whether the wallet ID exists at all.
2. **Zero Leakage**: Attackers probing sequential wallet IDs receive identical 404 responses for both non-existent IDs and unowned valid IDs.

### Why HTTP 403 Forbidden (Rejected):
`HTTP 403` leaks metadata to unauthorized callers, allowing bad actors to enumerate active wallet IDs across the platform.

### Accepted Trade-offs:
- Makes internal application debugging slightly harder for authorized developers who misconfigure merchant credentials during manual testing.

### When this Decision is WRONG:
In internal administrative APIs where explicit permission feedback ("Role X lacks Permission Y") is required for RBAC troubleshooting.
