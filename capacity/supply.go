package main

// supply computes infrastructure ceilings from physical inputs.
// All formulas delegate to models.go.

func supply(input *SLOInput) (map[string]PGCeiling, KafkaCeiling, []RedisCeiling) {
	pg := map[string]PGCeiling{}
	for name, inst := range input.Infra.PG.Instances {
		pg[name] = pgCeil(name, inst, input.Infra.PG.Tuning)
	}
	k := kafkaCeil(input.Infra.Kafka)
	var rc []RedisCeiling
	for i := range input.Infra.Redis.Nodes {
		rc = append(rc, redisCeil(i, input))
	}
	return pg, k, rc
}

// pgCeil — each DB instance's connection ceiling.
// models.go: pgMaxConnections, optimalActive
func pgCeil(name string, inst PGInstance, tuning PGTuning) PGCeiling {
	return PGCeiling{
		Instance:      name,
		MaxConns:      pgMaxConnections(inst.RAMBytes, inst.WorkMemMB, tuning.SharedBuffersPct, tuning.OSPct, tuning.MaintenancePct),
		OptimalActive: optimalActive(inst.Cores, inst.EffectiveSpindles),
	}
}

// kafkaCeil — cluster-level partition budget.
// models.go: kafkaClusterCap
func kafkaCeil(k KafkaInfra) KafkaCeiling {
	kc := KafkaCeiling{
		ClusterCap:   kafkaClusterCap(k.Brokers, k.PerBrokerPartitionCap),
		PerBrokerCap: k.PerBrokerPartitionCap,
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
	return RedisCeiling{Node: idx, MaxMemoryBytes: redisMaxMem(r.RAMBytesPerNode, r.ForkHeadroom, r.Fragmentation)}
}
