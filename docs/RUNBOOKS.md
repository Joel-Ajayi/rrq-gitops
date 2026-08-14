# Infrastructure Operational Runbooks

This document provides step-by-step operational runbooks for infrastructure SREs and platform operators managing the RRQ GitOps control plane.

---

## Index of Runbooks

- [RB-INFRA-01: Postgres Primary Failover & CNPG Recovery](#rb-infra-01-postgres-primary-failover--cnpg-recovery)
- [RB-INFRA-02: Scaling Kafka Partitions & Consumer Groups](#rb-infra-02-scaling-kafka-partitions--consumer-groups)
- [RB-INFRA-03: Rotating Sealed Secrets & Platform Keys](#rb-infra-03-rotating-sealed-secrets--platform-keys)
- [RB-INFRA-04: Running Capacity Planning Engine](#rb-infra-04-running-capacity-planning-engine)

---

## RB-INFRA-01: Postgres Primary Failover & CNPG Recovery

### Symptom / Trigger
- Prometheus Alert: `OutboxRelayShardCBOpen` or `LedgerWorkerCBOpen`.
- Primary Postgres node experiencing hardware failure or node network partition.

### Procedure
1. **Inspect CloudNativePG Cluster Status**:
   ```bash
   kubectl cnpg status shard-a -n rrq
   ```

2. **Verify Failover Execution**:
   CloudNativePG automatically promotes one of the two standby replicas (`shard-a-2` or `shard-a-3`) to primary. Confirm that `Primary instance` displays the newly promoted pod name.

3. **Manual Failover (If Required)**:
   If the operator needs to perform a scheduled failover for node maintenance:
   ```bash
   kubectl cnpg promote shard-a shard-a-2 -n rrq
   ```

4. **Verify Application Re-Connection**:
   `pgx/v5` connection pools will automatically reconnect to the newly promoted primary instance within 5 seconds.

---

## RB-INFRA-02: Scaling Kafka Partitions & Consumer Groups

### Symptom / Trigger
- High consumer group lag alert (`KafkaConsumerGroupLagHigh`).
- Target QPS increases beyond current worker replica processing capacity.

### Procedure
1. **Check Partition Calculation**:
   Recall rule: $\text{Workers} \le \text{Partitions}$. If `jobs` topic has 30 partitions, `ledger-worker` can scale up to at most 30 active replica pods.

2. **Increase Topic Partitions in Git**:
   Edit `rrq/base/kafka/topics.yaml`:
   ```yaml
   apiVersion: kafka.strimzi.io/v1beta2
   kind: KafkaTopic
   metadata:
     name: jobs
   spec:
     partitions: 50
   ```

3. **Commit & Push to Git**:
   ```bash
   git commit -am "ops(kafka): increase jobs topic partitions to 50"
   git push origin main
   ```

4. **Verify KEDA Autoscaling**:
   KEDA ScaledObjects will detect consumer lag and automatically scale `ledger-worker` pods up to 50 replicas.

---

## RB-INFRA-03: Rotating Sealed Secrets & Platform Keys

### Procedure
1. **Create Fresh Plain Secret**:
   Create a temporary `secret.plain.yaml` file locally (never commit this file).

2. **Encrypt with `kubeseal`**:
   ```bash
   kubeseal --scope cluster-wide --format yaml < secret.plain.yaml > rrq/secrets/prod/secret.sealed.yaml
   ```

3. **Commit & Push Sealed Secret**:
   ```bash
   git add rrq/secrets/prod/secret.sealed.yaml
   git commit -m "security(secrets): rotate platform secrets"
   git push origin main
   ```

4. **Clean Up Local Plaintext File**:
   ```bash
   rm secret.plain.yaml
   ```

---

## RB-INFRA-04: Running Capacity Planning Engine

### Procedure
1. **Navigate to Capacity Directory**:
   ```bash
   cd capacity
   ```

2. **Edit Model Inputs (`slo-input.yaml`)**:
   Update target QPS, latency SLOs ($W$), or node CPU limits.

3. **Execute Capacity Engine**:
   ```bash
   go run . slo-input.yaml
   ```

4. **Inspect Generated Outputs**:
   - Review console summary & `capacity-output.yaml`.
   - Verify generated Kustomize ConfigMaps in `../rrq/base/config/`.

5. **Commit ConfigMap Updates**:
   ```bash
   git commit -am "capacity: re-derive infrastructure limits from slo-input.yaml"
   git push origin main
   ```
