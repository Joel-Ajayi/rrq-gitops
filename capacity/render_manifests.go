package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// renderManifests patches the K8s YAML manifests in rrq-gitops/rrq/base
// from the values produced by the capacity engine. This addresses
// issues 14, 15, 16, 30: engine outputs are correct but no render step
// wires them into the deploy manifests.
//
// Files patched:
//   - base/services/<svc>.yaml    : KEDA ScaledObject maxReplicaCount, lagThreshold
//   - base/kafka/topics.yaml      : KafkaTopic partitions
//   - base/config/<svc>-configmap.yaml : derived env vars
//   - base/postgres/shards.yaml   : PG Cluster spec.postgresql.parameters.max_connections
func renderManifests(svcs map[string]Derived, pg map[string]PGCeiling, input *SLOInput, rootDir string) error {
	svcDir := filepath.Join(rootDir, "rrq", "base", "services")
	topicFile := filepath.Join(rootDir, "rrq", "base", "kafka", "topics.yaml")
	configDir := filepath.Join(rootDir, "rrq", "base", "config")
	pgFile := filepath.Join(rootDir, "rrq", "base", "postgres", "shards.yaml")

	// 1. Patch KEDA ScaledObject maxReplicaCount and lagThreshold
	for name, d := range svcs {
		path := filepath.Join(svcDir, name+".yaml")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		patched, err := patchScaledObject(string(data), d)
		if err != nil {
			return fmt.Errorf("patch %s: %w", path, err)
		}
		if patched != string(data) {
			if err := os.WriteFile(path, []byte(patched), 0644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
		}
	}

	// 2. Patch Kafka topic partitions
	partitions := map[string]int{}
	for _, d := range svcs {
		for t, p := range d.Partitions {
			if cur, ok := partitions[t]; !ok || p > cur {
				partitions[t] = p
			}
		}
	}
	if len(partitions) > 0 {
		if err := patchKafkaTopics(topicFile, partitions); err != nil {
			return err
		}
	}

	// 3. Patch webhook-worker KEDA lagThreshold env (in case HPA+KEDA read it)
	// Already covered by the ScaledObject patch above; nothing else to do.

	// 4. Patch webhook-worker KEDA KEDA_LAG_THRESHOLD env in configmap
	if d, ok := svcs["webhook-worker"]; ok && d.LagThreshold > 0 {
		path := filepath.Join(configDir, "webhook-worker-configmap.yaml")
		if err := patchConfigEnv(path, "WEBHOOK_WORKER_KEDA_LAG_THRESHOLD", strconv.Itoa(d.LagThreshold)); err != nil {
			return err
		}
	}

	// 5. Patch PG Cluster spec.postgresql.parameters.max_connections
	//     (server-side; the DB's hard limit, engine-derived from RAM)
	if err := patchPostgresClusters(pgFile, pg); err != nil {
		return err
	}

	// 6. Patch per-service Deployment resources (CPU request, memory limit).
	//     Engine-derived CPU and memory replace stale hardcoded values.
	for name, d := range svcs {
		path := filepath.Join(svcDir, name+".yaml")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := patchDeploymentResources(path, d); err != nil {
			return fmt.Errorf("patch resources %s: %w", path, err)
		}
	}

	return nil
}

// patchScaledObject updates maxReplicaCount and lagThreshold in a single
// ScaledObject block within the given yaml text. The engine's MaxReplicasCap
// is the value the operator should NEVER exceed (DB connection budget cap),
// so we patch maxReplicaCount with that value, not the unrestrained MaxReplicas.
// Also patches minReplicaCount from the engine's MinReplicas.
func patchScaledObject(yaml string, d Derived) (string, error) {
	cap := d.MaxReplicasCap
	if cap <= 0 {
		cap = d.MaxReplicas
	}
	if cap < 1 {
		cap = 1
	}

	// Patch minReplicaCount from the engine's MinReplicas
	if d.MinReplicas > 0 {
		minRe := regexp.MustCompile(`(?m)^(\s+)minReplicaCount:\s*\d+(\s*)$`)
		if minRe.MatchString(yaml) {
			yaml = minRe.ReplaceAllString(yaml, fmt.Sprintf("${1}minReplicaCount: %d${2}", d.MinReplicas))
		}
	}

	// Cap maxReplicaCount
	maxRe := regexp.MustCompile(`(?m)^(\s+)maxReplicaCount:\s*\d+(\s*)$`)
	if maxRe.MatchString(yaml) {
		yaml = maxRe.ReplaceAllString(yaml, fmt.Sprintf("${1}maxReplicaCount: %d${2}", cap))
	}

	// Cap lagThreshold (if present) — for ScaledObjects with Kafka triggers
	if d.LagThreshold > 0 {
		lag := regexp.MustCompile(`(?m)^(\s+)lagThreshold:\s*"?\d+"?(\s*)$`)
		if lag.MatchString(yaml) {
			yaml = lag.ReplaceAllString(yaml, fmt.Sprintf(`${1}lagThreshold: "%d"${2}`, d.LagThreshold))
		}
	}
	return yaml, nil
}

// patchKafkaTopics updates the partitions field for each topic in topics.yaml.
// The file is split by `---` and each KafkaTopic block is patched independently
// based on its `metadata.name`.
func patchKafkaTopics(path string, partitions map[string]int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	blocks := strings.Split(string(data), "\n---\n")
	for i, block := range blocks {
		// Find the first `name:` under metadata
		m := regexp.MustCompile(`(?ms)metadata:\s*\n(?:[ \t]+[^\n]*\n)*?[ \t]+name:\s*(\S+)\s*\n`).FindStringSubmatch(block)
		if len(m) < 2 {
			continue
		}
		topicName := m[1]
		newPart, ok := partitions[topicName]
		if !ok {
			continue
		}
		partRe := regexp.MustCompile(`(?m)^(\s+)partitions:\s*\d+(\s*)$`)
		if partRe.MatchString(block) {
			blocks[i] = partRe.ReplaceAllString(block, fmt.Sprintf("${1}partitions: %d${2}", newPart))
		}
	}
	return os.WriteFile(path, []byte(strings.Join(blocks, "\n---\n")), 0644)
}

// patchConfigEnv updates a single env var value in a ConfigMap's data block.
func patchConfigEnv(path, envName, newValue string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	text := string(data)
	re := regexp.MustCompile(`(?m)^(\s+)` + regexp.QuoteMeta(envName) + `:\s*"?[^"\n]+"?(\s*)$`)
	if re.MatchString(text) {
		text = re.ReplaceAllString(text, fmt.Sprintf(`${1}%s: "%s"${2}`, envName, newValue))
		return os.WriteFile(path, []byte(text), 0644)
	}
	return nil
}

// patchDeploymentResources updates CPU request and memory limit in a
// Deployment's container resources block from engine-derived values.
func patchDeploymentResources(path string, d Derived) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	text := string(data)

	// CPU request: <value>m
	if d.CPURequest > 0 {
		cpuRe := regexp.MustCompile(`(?m)^(\s*-?\s*)cpu:\s*\d+m(\s*)$`)
		if cpuRe.MatchString(text) {
			text = cpuRe.ReplaceAllString(text, fmt.Sprintf("${1}cpu: %dm${2}", d.CPURequest))
		}
	}

	// Memory limit: <value>Mi or <value>Gi
	// We patch the memory limit from the engine-derived MemRequest (MiB).
	if d.MemRequest > 0 {
		memRe := regexp.MustCompile(`(?m)^(\s*-?\s*)memory:\s*\d+(Mi|Gi)(\s*)$`)
		if memRe.MatchString(text) {
			text = memRe.ReplaceAllString(text, fmt.Sprintf("${1}memory: %d${2}", d.MemRequest))
		}
	}

	if text != string(data) {
		return os.WriteFile(path, []byte(text), 0644)
	}
	return nil
}

// patchPostgresClusters updates `spec.postgresql.parameters.max_connections`
// in each PG Cluster (postgresql.cnpg.io/v1) block based on the engine's
// PG ceiling (RAM-derived hard limit per instance).
//
// The file is split by `---`; each block is independently matched against
// its `metadata.name`.
func patchPostgresClusters(path string, ceilings map[string]PGCeiling) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	blocks := strings.Split(string(data), "\n---\n")
	anyPatched := false
	for i, block := range blocks {
		// Match `metadata.name: <name>` under the metadata block.
		m := regexp.MustCompile(`(?ms)metadata:\s*\n(?:[ \t]+[^\n]*\n)*?[ \t]+name:\s*(\S+)\s*\n`).FindStringSubmatch(block)
		if len(m) < 2 {
			continue
		}
		clusterName := m[1]
		// Map CNPG cluster name to engine instance name. The engine uses
		// `merchants`/`shard-a`/`shard-b`; CNPG cluster objects append `-db`.
		ceiling, ok := ceilings[clusterName]
		if !ok && strings.HasSuffix(clusterName, "-db") {
			ceiling, ok = ceilings[strings.TrimSuffix(clusterName, "-db")]
		}
		if !ok {
			continue
		}
		maxRe := regexp.MustCompile(`(?m)^(\s+)max_connections:\s*"?\d+"?(\s*)$`)
		if maxRe.MatchString(block) {
			newBlock := maxRe.ReplaceAllString(block, fmt.Sprintf(`${1}max_connections: "%d"${2}`, ceiling.MaxConns))
			if newBlock != block {
				blocks[i] = newBlock
				anyPatched = true
			}
		}
	}
	if anyPatched {
		return os.WriteFile(path, []byte(strings.Join(blocks, "\n---\n")), 0644)
	}
	return nil
}
