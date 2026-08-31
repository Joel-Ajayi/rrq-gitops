package main

import "math"

const (
	// RetryBudgetBurstWindowSec is the time window over which the retry budget token bucket allows bursts
	RetryBudgetBurstWindowSec = 2.0
	// RetryBudgetMinTokensFloor is the absolute minimum number of tokens required in a retry budget
	RetryBudgetMinTokensFloor = 2
	// RetryBudgetMaxTokensFloor is the absolute minimum maximum-capacity of a retry budget token bucket
	RetryBudgetMaxTokensFloor = 10
	// RetryBudgetMinTokensRatio is the ratio of MaxTokens used to compute MinTokens
	RetryBudgetMinTokensRatio = 0.10
)

// derive computes all per-service derived values by calling model functions from models.go.
// This file is pure orchestration — no formulas live here.

func derive(input *SLOInput) map[string]Derived {
	out := make(map[string]Derived)
	for _, svc := range input.Services {
		out[svc.Name] = deriveOne(svc, input)
	}
	return out
}

func deriveOne(svc Service, inp *SLOInput) Derived {
	d := Derived{
		Name:       svc.Name,
		Partitions: make(map[string]int),
		LatencyMS:  make(map[string]float64),
	}

	rho := svc.SLO.TargetUtilization
	az := inp.Defaults.AZRedundancy
	gr := inp.Defaults.GrowthHeadroom
	slack := inp.Defaults.SlackPercent

	var totalPeak, totalNominal float64
	perIns := map[string]InstDemand{}
	var maxLatencyMS float64

	// Apply retry amplification to peak load expectations.
	retryMultiplier := 1.0 + inp.Defaults.RetryBudgetFraction

	for _, ep := range svc.Endpoints {
		effectivePeak := ep.PeakQPS * retryMultiplier
		totalPeak += effectivePeak
		totalNominal += ep.NominalQPS
		insts := ep.GetDBInstances()
		k := float64(len(insts))
		if k > 0 {
			for _, inst := range insts {
				id := perIns[inst]
				id.Peak += effectivePeak / k
				id.QPSMS += (effectivePeak / k) * ep.AvgQueryTimeMS / 1000.0
				perIns[inst] = id
			}
		}
		if ep.AvgQueryTimeMS > maxLatencyMS {
			maxLatencyMS = ep.AvgQueryTimeMS
		}
	}

	// models.go: Replica Count — pod capacity from k6 benchmark
	podCap := podCapacity(svc.RPSPerCore, svc.CoresPerPod)
	minFloor := inp.Defaults.MinReplicas
	if svc.MinReplicas > 0 {
		minFloor = svc.MinReplicas
	}
	maxCap := inp.Defaults.MaxReplicas
	if svc.MaxReplicas > 0 {
		maxCap = svc.MaxReplicas
	}
	d.MinReplicas, d.MaxReplicas = replicaCounts(totalNominal, totalPeak, podCap, az, minFloor, maxCap)
	d.MaxReplicasCap = d.MaxReplicas

	// models.go: Weighted Average Service Time
	avgMS, _ := weightedAvgMS(svc)

	// models.go: DB Pool Demand + Per-Pod Pool
	maxDemand := dbPoolDemand(perIns, rho)
	d.PoolSize = perPodPool(maxDemand, d.MinReplicas, inp.Defaults.PoolFloor)

	// models.go: Kingman's Formula — per-endpoint latency at target ρ
	// Compute FIRST so workerConcurrency and ProcessTimeout can use it.
	for _, ep := range svc.Endpoints {
		tauDB := ep.AvgQueryTimeMS / 1000.0
		lat := kingmanLatency(rho, ep.CSquaredA, ep.CSquaredS, tauDB, d.PoolSize)
		if svc.HTTP != nil {
			// Add HTTP network I/O to total residence time.
			// (HTTP wait time is 0 since the HTTP pool is always >= workers)
			lat += svc.HTTP.AvgLatencyS * 1000.0
		}
		d.LatencyMS[ep.Name] = lat
	}

	// models.go: Worker Pool
	// wTime = max(kingman L(ρ)) × workerAmp (kingman includes service time)
	peakPerPod := totalPeak / float64(d.MinReplicas)
	maxKingman := maxLatencyMS
	for _, lat := range d.LatencyMS {
		if lat > maxKingman {
			maxKingman = lat
		}
	}
	if maxKingman < 1 {
		maxKingman = 1
	}
	d.Workers = workerConcurrency(peakPerPod, maxKingman, inp.Defaults.WorkerAmplification, inp.Defaults.WorkerFloor)

	// models.go: Retry Budget — derived from SLO (NOT per-endpoint query time)
	d.BackoffBaseMS = int(math.Ceil(avgMS))
	if d.BackoffBaseMS < 1 {
		d.BackoffBaseMS = 1
	}
	d.MaxRetries = maxRetries(svc.SLO.LatencyMS, slack, avgMS, inp.Defaults.RetryBudgetFraction)
	d.BackoffCapMS = backoffCap(d.BackoffBaseMS, d.MaxRetries, svc.SLO.LatencyMS, slack, inp.Defaults.RetryBudgetFraction)

	// Volume-derived Token Bucket Retry Budget parameters
	// maxTokens is derived from PeakQPSPerPod over a 2-second burst window at retry_budget_fraction
	maxTokens := int(math.Ceil(peakPerPod * RetryBudgetBurstWindowSec * inp.Defaults.RetryBudgetFraction))
	if maxTokens < RetryBudgetMaxTokensFloor {
		maxTokens = RetryBudgetMaxTokensFloor
	}
	minTokens := int(math.Ceil(float64(maxTokens) * RetryBudgetMinTokensRatio))
	if minTokens < RetryBudgetMinTokensFloor {
		minTokens = RetryBudgetMinTokensFloor
	}
	d.RetryBudgetMaxTokens = maxTokens
	d.RetryBudgetMinTokens = minTokens

	// ProcessTimeout — per-message deadline, bounded by SLO budget (ceiling).
	// Must accommodate the tail of the latency distribution, so we use the SLO.
	if svc.SLO.LatencyMS > 0 && len(d.LatencyMS) > 0 {
		kingmanFloor := int(math.Ceil(maxKingman))
		sloBudget := int(float64(svc.SLO.LatencyMS) * (1 - slack))
		processTimeout := sloBudget

		if processTimeout < kingmanFloor {
			processTimeout = kingmanFloor // sanity check floor
		}
		d.ProcessTimeoutMs = processTimeout
	}

	// DLQ retry config — derived from ProcessTimeout
	// The total DLQ time must fit inside the ProcessTimeout budget.
	d.DLQMaxRetries, d.DLQBaseDelayMs, d.DLQCapDelayMs = dlqRetryFromProcess(d.ProcessTimeoutMs, inp.Defaults.DLQMaxRetries)

	// Derive Circuit Breaker timeout. It must match the downstream timeout if
	// HTTP outbound exists, or fall back to the ProcessTimeoutMs.
	if svc.HTTP != nil && svc.HTTP.TimeoutMS > 0 {
		d.CircuitBreakerTimeoutMs = svc.HTTP.TimeoutMS
	} else {
		d.CircuitBreakerTimeoutMs = d.ProcessTimeoutMs
	}

	// models.go: Session & Heartbeat
	// SessionTimeout = ProcessTimeout + DLQ_total + 2×buffer
	dlqTotal := d.DLQMaxRetries * (d.DLQBaseDelayMs + d.DLQCapDelayMs)
	// Outer DLQ write deadline. This is the hard cap on how long the worker pool outside the process.
	d.DLQWriteTimeoutMs = min(dlqTotal, d.ProcessTimeoutMs)
	d.SessionMs, d.HeartbeatMs = sessionTiming(
		d.ProcessTimeoutMs, d.DLQWriteTimeoutMs,
		inp.Defaults.ConsumerSessionBufferMS,
	)

	// models.go: Shutdown Timeout
	d.ShutdownTimeoutMs = shutdownTimeout(d.SessionMs)

	// models.go: Kafka Partition Count
	for _, topic := range svc.Topics {
		perPart := inp.Infra.Kafka.PartitionConsumeRPS[topic]
		d.Partitions[topic] = partitionCount(totalPeak, perPart, d.MinReplicas, gr)
	}

	// models.go: KEDA Lag Threshold
	if svc.SLO.LatencyMS > 0 && svc.Role == "consumer" {
		d.LagThreshold = lagThreshold(svc.SLO.LatencyMS, avgMS, d.Workers)
	}

	// models.go: HTTP Outbound Pool
	if svc.HTTP != nil {
		d.HTTPPool, d.HTTPPerHost = httpPoolSize(
			svc.HTTP.PeakQPSPerPod, svc.HTTP.AvgLatencyS,
			inp.Defaults.HTTPHeadroom,
			d.Workers, svc.HTTP.HostCount,
		)
		d.HTTPPerHostCap = svc.HTTP.PerHostHeadroom
	}

	// models.go: Webhook per-merchant bulkhead (issue 24)
	if svc.Webhook != nil {
		// Workers is the unified pool (Kingman-derived above) that guards
		// all concurrent goroutines: Kafka consumer path + HTTP delivery
		// (fast lane) + retry scheduler. No split into consumer and fast-lane
		// portions — the Kingman wTime already captures the per-message
		// service time at target utilization.
		//
		if svc.HTTP != nil && d.HTTPPool > 0 {
			d.FastLaneWorkerPoolSize = max(1, d.HTTPPool)
		}

		// Ensure the unified pool can cover the fast lane.
		if d.Workers < d.FastLaneWorkerPoolSize {
			d.Workers = d.FastLaneWorkerPoolSize
		}

		if svc.Webhook.MaxConcurrencyPerMerchant <= 0 {
			// Per-merchant bulkhead: distribute fast lane workers using
			// standard 10% pool capacity practice for massive multi-tenancy.
			d.WebhookMaxConcurrency = webhookMaxConcurrency(d.FastLaneWorkerPoolSize)
		}
		// Derive BreakerEvictionTTL when not explicitly set. See issue 26:
		// at least 10× the average delivery period, with a 5min floor.
		if svc.Webhook.BreakerEvictionTTLMS <= 0 {
			d.BreakerEvictionTTLMS = breakerEvictionTTL(svc.Webhook.DeliveryBackoffBaseMS, d.DLQBaseDelayMs)
		}
	}

	// models.go: Relay Derived Values
	if svc.Role == "producer" {
		d.RelayFetchBatch, d.RelayPoolIntervalMS, d.RelayBatchTimeoutMS, d.RelayReplicas =
			relayDerived(totalPeak, avgMS, svc.ProducerThroughputRPS, float64(svc.SLO.LatencyMS),
				inp.Defaults.RelayMaxFetchBatch)
	}

	// models.go: Per-Pod Per-Shard RW & RO Caps
	d.PerShardRW, d.PerShardRO = perShardCaps(svc, inp, &d)

	// Total client-side DB connection pool size across all shards
	totalPool := 0
	for _, c := range d.PerShardRW {
		totalPool += c
	}
	for _, c := range d.PerShardRO {
		totalPool += c
	}
	if totalPool > 0 {
		d.PoolSize = totalPool
	}

	// models.go: CPU & Memory Request
	d.CPURequest = cpuRequest(totalNominal/float64(d.MinReplicas), svc.RPSPerCore)
	if svc.Role == "producer" && svc.Relay != nil {
		d.MemRequest = relayMemRequest(d.PoolSize, svc.Relay.StagingKB, d.RelayFetchBatch, svc.Relay.MaxPayloadKB)
	} else {
		kafkaBufferMB := 0
		if svc.Role == "consumer" {
			for _, p := range d.Partitions {
				podPart := int(math.Ceil(float64(p) / float64(d.MinReplicas)))
				kafkaBufferMB += (podPart * inp.Infra.Kafka.ReaderMaxBytes) / (1024 * 1024)
			}
		}

		// In-flight heap allocation from Little's Law: L = λ_pod × W_max
		inFlightConcurrency := peakPerPod * (maxKingman / 1000.0)
		inFlightHeapMB := int(math.Ceil((inFlightConcurrency * InFlightRequestAllocBytes * GoGCMemoryMultiplier) / float64(BytesPerMiB)))

		d.MemRequest = memRequest(d.PoolSize, d.HTTPPool, inFlightHeapMB) + kafkaBufferMB
	}

	return d
}

// perShardCaps computes per-pod per-shard connection caps for both RW and RO pools.
// Each shard's pool is mathematically constrained by that shard's PostgreSQL hard ceiling
// and floored at pool_floor to eliminate serial bottlenecks.
func perShardCaps(svc Service, inp *SLOInput, d *Derived) (map[string]int, map[string]int) {
	rwCaps := make(map[string]int)
	roCaps := make(map[string]int)

	shards := map[string]bool{}
	for _, ep := range svc.Endpoints {
		for _, inst := range ep.GetDBInstances() {
			shards[inst] = true
		}
	}

	rho := svc.HPATargetCPU
	minR := float64(d.MinReplicas)
	if minR <= 0 {
		minR = 1
	}

	floor := inp.Defaults.PoolFloor
	if floor < 2 {
		floor = 2
	}

	for shardName := range shards {
		inst, ok := inp.Infra.PG.Instances[shardName]
		if !ok {
			continue
		}
		ceiling := pgMaxConnections(inst.RAMBytes, inst.WorkMemMB,
			inp.Infra.PG.Tuning.SharedBuffersPct,
			inp.Infra.PG.Tuning.OSPct,
			inp.Infra.PG.Tuning.MaintenancePct)

		numServicesOnShard := 0
		for _, other := range inp.Services {
			for _, ep := range other.Endpoints {
				for _, otherInst := range ep.GetDBInstances() {
					if otherInst == shardName {
						numServicesOnShard++
						break
					}
				}
			}
		}

		var rwQPSMS, roQPSMS float64
		for _, ep := range svc.Endpoints {
			insts := ep.GetDBInstances()
			k := float64(len(insts))
			if k == 0 {
				continue
			}
			for _, targetInst := range insts {
				if targetInst == shardName {
					qpsms := (ep.PeakQPS / k) * (ep.AvgQueryTimeMS / 1000.0)
					if ep.WritesPerMessage > 0 {
						rwQPSMS += qpsms
					} else {
						roQPSMS += qpsms
					}
				}
			}
		}

		if rwQPSMS > 0 {
			rwPoolDemand := int(math.Ceil((rwQPSMS / rho) / minR))
			if rwPoolDemand < floor {
				rwPoolDemand = floor
			}
			rwCaps[shardName] = perShardRWCap(rwPoolDemand, ceiling, numServicesOnShard, d.MaxReplicasCap, floor)
		}
		if roQPSMS > 0 {
			roPoolDemand := int(math.Ceil((roQPSMS / rho) / minR))
			if roPoolDemand < floor {
				roPoolDemand = floor
			}
			roCaps[shardName] = perShardRWCap(roPoolDemand, ceiling, numServicesOnShard, d.MaxReplicasCap, floor)
		}
	}

	return rwCaps, roCaps
}
