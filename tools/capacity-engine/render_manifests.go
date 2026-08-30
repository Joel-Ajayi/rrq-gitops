package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// renderManifests patches the K8s YAML manifests under rootDir
// (the rrq-gitops repo root) from the values produced by the capacity engine.
//
// Files patched:
//   - base/workloads/services/<svc>.yaml : KEDA ScaledObject maxReplicaCount, lagThreshold
//   - base/platform/datastores/kafka/topics.yaml : KafkaTopic partitions
//   - base/platform/datastores/postgres/shards.yaml : PG max_connections
func renderManifests(svcs map[string]Derived, pg map[string]PGCeiling, input *SLOInput, rootDir string) error {
	svcDir := filepath.Join(rootDir, "base", "workloads", "services")
	topicFile := filepath.Join(rootDir, "base", "platform", "datastores", "kafka", "topics.yaml")
	pgFile := filepath.Join(rootDir, "base", "platform", "datastores", "postgres", "shards.yaml")

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

	// 3. Patch PG Cluster spec.postgresql.parameters.max_connections
	//     (server-side; the DB's hard limit, engine-derived from RAM)
	if err := patchPostgresClusters(pgFile, pg); err != nil {
		return err
	}

	// 4. Patch per-service Deployment resources (CPU request/limit, memory request/limit).
	//     Engine-derived CPU and memory replace stale hardcoded values with standard cloud rounding.
	for name, d := range svcs {
		path := filepath.Join(svcDir, name+".yaml")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		var svc Service
		for _, s := range input.Services {
			if s.Name == name {
				svc = s
				break
			}
		}
		if err := patchDeploymentResources(path, d, svc, input); err != nil {
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

// patchDeploymentResources updates CPU requests/limits and memory requests/limits
// in a Deployment's container resources block using standard rounded increments.
func patchDeploymentResources(path string, d Derived, svc Service, input *SLOInput) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	text := string(data)

	reqCPU := standardCPURequest(d.CPURequest)
	reqMem := standardMemRequest(d.MemRequest)
	limCPU := standardCPULimit(svc, input)
	limMem := standardMemLimit(svc, input)

	// Replace the entire resources block with properly sized requests and limits
	resRe := regexp.MustCompile(`(?ms)(resources:\s*\n\s+requests:\s*\n\s+cpu:)\s*[^\n]+(\n\s+memory:)\s*[^\n]+(\n\s+limits:\s*\n\s+cpu:)\s*[^\n]+(\n\s+memory:)\s*[^\n]+`)
	if resRe.MatchString(text) {
		text = resRe.ReplaceAllString(text, fmt.Sprintf(`${1} %dm${2} %dMi${3} "%s"${4} %dMi`, reqCPU, reqMem, limCPU, limMem))
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
