package main

import "fmt"

// fitcheck verifies that total service demand fits inside infrastructure supply.
// See models.go for all formulas and their sources.

func fitcheck(svcs map[string]Derived, pg map[string]PGCeiling, kc KafkaCeiling, rc []RedisCeiling, input *SLOInput) ([]string, []string) {
	var fails, warns []string

	// models.go: Fit-Check + Connection Demand + HPA Cap — demand vs supply per DB instance
	for instName, ceiling := range pg {
		usable := ceiling.MaxConns
		totalPeakDemand := 0
		totalMinDemand := 0

		// First, aggregate total demand across all services for this instance
		for _, d := range svcs {
			conns := d.PerShardRW[instName] + d.PerShardRO[instName]
			if conns > 0 {
				totalPeakDemand += connectionDemand(conns, d.MaxReplicas)
				totalMinDemand += connectionDemand(conns, d.MinReplicas)
			}
		}

		// Check if peak demand exceeds usable capacity
		if totalPeakDemand > usable {
			fails = append(fails, fmt.Sprintf("pg/%s: total peak demand %d > %d usable", instName, totalPeakDemand, usable))
		}
		// Warn if peak demand exceeds optimal capacity
		if totalPeakDemand > ceiling.OptimalActive {
			warns = append(warns, fmt.Sprintf("pg/%s: total peak demand %d > optimal %d", instName, totalPeakDemand, ceiling.OptimalActive))
		}
		// Hard failure if even the absolute minimum scale of all services exceeds the hard limit
		if totalMinDemand > usable {
			fails = append(fails, fmt.Sprintf("pg/%s: FATAL: minimum scale demand %d > %d usable", instName, totalMinDemand, usable))
		}

		// Calculate clamped HPA caps for each service on this instance
		for _, d := range svcs {
			conns := d.PerShardRW[instName] + d.PerShardRO[instName]
			if conns > 0 {
				gap := usable
				for _, other := range svcs {
					if other.Name != d.Name {
						otherConns := other.PerShardRW[instName] + other.PerShardRO[instName]
						gap -= connectionDemand(otherConns, other.MaxReplicas)
					}
				}
				cap := hpaCap(gap, conns, d.MinReplicas)
				if cap > 0 && cap < svcs[d.Name].MaxReplicasCap {
					updated := svcs[d.Name]
					updated.MaxReplicasCap = cap
					svcs[d.Name] = updated
				}
			}
		}
	}

	// models.go: Kafka — Cluster Partition Budget — per-broker spread check
	if input.Infra.Kafka.SegmentSeconds > input.Infra.Kafka.RetentionDays*24*3600 {
		fails = append(fails, fmt.Sprintf("kafka: segment_seconds (%d) > retention_days (%d) breaks deletion policy", input.Infra.Kafka.SegmentSeconds, input.Infra.Kafka.RetentionDays))
	}
	totalParts := 0
	for _, d := range svcs {
		for _, p := range d.Partitions {
			totalParts += p
		}
	}
	k := input.Infra.Kafka
	if k.Brokers > 0 {
		spread := (totalParts * k.ReplicationFactor) / k.Brokers
		if spread > k.PerBrokerPartitionCap {
			fails = append(fails, fmt.Sprintf("kafka: spread %d > cap %d", spread, k.PerBrokerPartitionCap))
		}
		if totalParts > KafkaClusterMaxParts {
			fails = append(fails, fmt.Sprintf("kafka: %d parts > %d cluster cap", totalParts, KafkaClusterMaxParts))
		}
	}

	// models.go: Redis — Memory Model — velocity keyspace vs maxmemory
	var totalKeyspace int64
	for _, svc := range input.Services {
		if svc.Redis != nil {
			totalKeyspace += svc.Redis.Merchants * int64(svc.Redis.WindowBuckets) * int64(input.Infra.Redis.PerKeyBytes)
		}
	}
	if len(rc) > 0 {
		clusterMemoryBytes := int64(len(rc)) * rc[0].MaxMemoryBytes
		if totalKeyspace > clusterMemoryBytes {
			gapGB := float64(totalKeyspace-clusterMemoryBytes) / 1073741824
			fails = append(fails, fmt.Sprintf("redis: +%.2fGiB over %dGiB cluster capacity", gapGB, clusterMemoryBytes/1073741824))
		}
	}

	// models.go: Memory Request & Limit — pod memory fit-check
	// Uses the engine-derived MemRequest (which includes DB connection pools,
	// HTTP connection pools, Little's Law in-flight heap, Kafka buffers, and relay staging)
	// and compares against the per-service or infra pod memory limit specified in slo-input.yaml.
	for _, d := range svcs {
		svc := findService(input, d.Name)
		limit := input.Infra.Pod.MemLimitBytes
		if svc != nil && svc.MemLimitBytes > 0 {
			limit = svc.MemLimitBytes
		}
		memLimitMiB := int(limit / (1024 * 1024))
		reqMiB := standardMemRequest(d.MemRequest)
		if d.MemRequest > memLimitMiB || reqMiB > memLimitMiB {
			warns = append(warns, fmt.Sprintf("pod/%s: calculated memory requirement %dMiB (request %dMiB) exceeds SLO specified limit %dMiB", d.Name, d.MemRequest, reqMiB, memLimitMiB))
		}
	}

	// SLO Required for consumers — latency budget is the basis for ProcessTimeout
	// and DLQ retry derivation. Without an SLO, those derived values are meaningless.
	for _, svc := range input.Services {
		if svc.Role == "consumer" && svc.SLO.LatencyMS <= 0 {
			fails = append(fails, fmt.Sprintf("service/%s: consumer missing SLO latency_ms", svc.Name))
		}
	}

	// Per-service timing sanity checks
	for _, d := range svcs {
		// ProcessTimeout must be ≥ 1ms
		if d.ProcessTimeoutMs < 1 {
			fails = append(fails, fmt.Sprintf("service/%s: ProcessTimeoutMs < 1", d.Name))
		}
		// DLQWriteTimeoutMs must be ≤ ProcessTimeoutMs — the DLQ write is bounded
		// by the DLQ retry budget (which fits in ProcessTimeout), not the session.
		if d.DLQWriteTimeoutMs > d.ProcessTimeoutMs {
			fails = append(fails, fmt.Sprintf("service/%s: DLQWriteTimeoutMs %d > ProcessTimeoutMs %d", d.Name, d.DLQWriteTimeoutMs, d.ProcessTimeoutMs))
		}
	}

	return fails, warns
}
