package main

// supply computes infrastructure ceilings from physical inputs.
// All formulas delegate to models.go.

func supply(input *SLOInput) (map[string]PGCeiling, KafkaCeiling, []RedisCeiling) {
	pg := map[string]PGCeiling{}
	for name, inst := range input.Infra.PG.Instances {
		pg[name] = pgCeil(name, inst, input.Infra.PG.Tuning, input)
	}
	k := kafkaCeil(input.Infra.Kafka, input)
	var rc []RedisCeiling
	for i := range input.Infra.Redis.Nodes {
		rc = append(rc, redisCeil(i, input))
	}
	return pg, k, rc
}

// pgCeil — each DB instance's connection ceiling and storage.
// models.go: pgMaxConnections, optimalActive
func pgCeil(name string, inst PGInstance, tuning PGTuning, input *SLOInput) PGCeiling {
	var totalWritesPerSec float64
	for _, svc := range input.Services {
		for _, ep := range svc.Endpoints {
			insts := ep.GetDBInstances()
			k := float64(len(insts))
			if k == 0 {
				continue
			}
			for _, inst := range insts {
				if inst == name {
					totalWritesPerSec += (ep.PeakQPS / k) * float64(ep.WritesPerMessage)
				}
			}
		}
	}
	// Assume 1KB per write, convert to GB/day
	storageGBPerDay := totalWritesPerSec * 1024 * 86400 / (1024 * 1024 * 1024)

	return PGCeiling{
		Instance:        name,
		MaxConns:        pgMaxConnections(inst.RAMBytes, inst.WorkMemMB, tuning.SharedBuffersPct, tuning.OSPct, tuning.MaintenancePct),
		OptimalActive:   optimalActive(inst.Cores, inst.EffectiveSpindles),
		StorageGBPerDay: storageGBPerDay,
	}
}

// kafkaCeil — cluster-level partition budget and storage.
// models.go: kafkaClusterCap
func kafkaCeil(k KafkaInfra, input *SLOInput) KafkaCeiling {
	var totalProducerRPS float64
	for _, svc := range input.Services {
		if svc.Role == "producer" || svc.ProducerThroughputRPS > 0 {
			for _, ep := range svc.Endpoints {
				totalProducerRPS += ep.PeakQPS
			}
		}
	}
	// Assume 1KB per message, replicate, convert to GB/day
	storageGBPerDay := totalProducerRPS * 1024 * 86400 * float64(k.ReplicationFactor) / (1024 * 1024 * 1024)

	kc := KafkaCeiling{
		ClusterCap:      kafkaClusterCap(k.Brokers, k.PerBrokerPartitionCap),
		PerBrokerCap:    k.PerBrokerPartitionCap,
		StorageGBPerDay: storageGBPerDay,
	}
	if k.LatencyCritical && k.Brokers > 0 {
		if lc := KafkaLatencyAdvisoryR * k.Brokers * k.ReplicationFactor; kc.ClusterCap > lc {
			kc.LatencyWarning = true
		}
	}
	// models.go: Kafka FD Estimate
	segs := kafkaSegmentCount(k.RetentionDays, k.SegmentSeconds)
	fd := kafkaFDEstimate(k.PerBrokerPartitionCap, segs)
	if int64(fd) > k.BrokerFDULimit {
		kc.FDWarning = true
	}
	return kc
}

// redisCeil — per-node memory ceiling.
// models.go: redisMaxMem
func redisCeil(idx int, input *SLOInput) RedisCeiling {
	r := input.Infra.Redis
	maxMem := redisMaxMem(r.RAMBytesPerNode, r.ForkHeadroom, r.Fragmentation)

	// AOF persistence is disabled, so no PVC storage is required
	storageGB := 0.0

	return RedisCeiling{
		Node:           idx,
		MaxMemoryBytes: maxMem,
		StorageGB:      storageGB,
	}
}
