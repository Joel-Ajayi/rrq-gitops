package main

import (
	"fmt"
	"math"
)

const (
	// DefaultSessionTimeoutMS is the fallback minimum session timeout (10s)
	DefaultSessionTimeoutMS = 10000
	// DefaultHeartbeatTimeoutMS is the fallback minimum heartbeat timeout (3s)
	DefaultHeartbeatTimeoutMS = 3000
	// WebhookConcurrencyRatio is the percentage of workers allocated to a single merchant's bulkhead
	WebhookConcurrencyRatio = 0.10
	// DefaultBreakerEvictionTTLMS is the minimum circuit breaker eviction TTL (5 minutes)
	DefaultBreakerEvictionTTLMS = 300000
	// ShutdownTimeoutMinMS is the minimum graceful shutdown period (30s)
	ShutdownTimeoutMinMS = 30000
	// ShutdownTimeoutBufferMS is the buffer added to session timeout for graceful shutdown (15s)
	ShutdownTimeoutBufferMS = 15000
	// ConnectionOverheadMB is the baseline memory overhead per PostgreSQL connection
	ConnectionOverheadMB = 8
)

// KINGMAN'S FORMULA — G/G/1 waiting time (J.F.C. Kingman, 1961
// [SOURCED] E[W_q] ≈ (ρ / (1−ρ)) × ((c_a² + c_s²) / 2) × τ
//   ρ     = target_utilization
//   c_a²  = squared CV of inter-arrival times (Poisson ≈ 1.0)
//   c_s²  = squared CV of service times (DB queries ≈ 1.5)
//   τ     = mean service time in seconds
//
// [DERIVED] L(ρ) = τ + E[W_q] — per-endpoint latency at target ρ

// erlangC computes the probability of delay in an M/M/c queue.
func erlangC(c int, rho float64) float64 {
	if c <= 0 {
		return 1.0 // Degenerate case
	}
	A := float64(c) * rho
	B := 1.0
	for i := 1; i <= c; i++ {
		B = (A * B) / (float64(i) + A*B)
	}
	numerator := float64(c) * B
	denominator := float64(c) - A*(1.0-B)
	if denominator <= 0 {
		return 1.0
	}
	prob := numerator / denominator
	if prob > 1.0 {
		return 1.0
	}
	return prob
}

// kingmanLatency computes the Allen-Cunneen approximation for a G/G/c queue.
func kingmanLatency(rho, cA2, cS2, tauSec float64, c int) float64 {
	probDelay := erlangC(c, rho)
	w := (probDelay / float64(c)) * (tauSec / (1.0 - rho)) * ((cA2 + cS2) / 2.0) * 1000.0
	return tauSec*1000.0 + w
}

// DB POOL DEMAND — per-instance, per-endpoint (Little's Law)
// [DERIVED] demand_i = max ((λ_peak_ep × S_ep) / ρ)  over all endpoints on instance i
//
//	λ_peak_ep = endpoint peak QPS
//	S_ep       = avg_query_time_ms / 1000
func dbPoolDemand(perIns map[string]InstDemand, rho float64) float64 {
	var maxDemand float64
	for _, inst := range perIns {
		demand := inst.QPSMS / rho
		if demand > maxDemand {
			maxDemand = demand
		}
	}
	return maxDemand
}

type InstDemand struct {
	Peak  float64
	QPSMS float64
}

// PER-POD POOL — demand spread across replicas (HikariCP/PG wiki)
// [DERIVED] per_pod = max(floor(maxDemand / minReplicas), pool_floor)
//	Floor because a pool of 1 serializes (HikariCP/goldlapel, B3/D5).

func perPodPool(maxDemand float64, minReplicas, poolFloor int) int {
	// Use Ceil to avoid cluster-wide capacity deficits.
	n := int(math.Ceil(maxDemand / float64(minReplicas)))
	if n < poolFloor {
		n = poolFloor
	}
	return n
}

// PER-POD PER-SHARD RW CAP — connection pool per pod, per shard
// [DERIVED] per_pod_per_shard = max(poolFloor, min(poolSize, ceiling / num_services_on_shard / max_replicas))
//
//	ceiling = PG's hard max_connections (RAM-derived, output in capacity-output.yaml)
//	poolSize = the service's per-pod pool (d.PoolSize, the demand-driven value)
//	num_services_on_shard = count of services in the cluster that hit this shard
//	max_replicas = engine-derived MaxReplicasCap for this service
//
//	Why divide by num_services_on_shard × max_replicas: ensures that
//	even at peak (all replicas, all services), the total per-shard
//	connections fit under the PG hard limit.
func perShardRWCap(poolSize, ceiling, numServicesOnShard, maxReplicas, poolFloor int) int {
	if numServicesOnShard == 0 {
		return poolSize
	}
	if maxReplicas <= 0 {
		maxReplicas = 1
	}
	// Fair share of the DB ceiling divided by maximum peak replicas
	safeCap := int(math.Ceil(float64(ceiling) / float64(numServicesOnShard) / float64(maxReplicas)))

	// Clamp the demanded poolSize to the mathematically safe cap
	if poolSize > safeCap {
		poolSize = safeCap
	}
	// Floor at poolFloor to prevent serial bottlenecks (pool of 1)
	if poolSize < poolFloor {
		poolSize = poolFloor
	}
	return poolSize
}

// WORKER CONCURRENCY — bounded by partition count for consumers,
// throughput demand for APIs.
//
//		[DERIVED] wTime = kingmanLatency(tau) × workerAmp
//	 workerConcurrency computes the required worker pool size per pod bounded by [workerFloor, workerCeil].
func workerConcurrency(throughputPerPod, kingmanLatencyMs, workerAmp float64, workerFloor, workerCeil int) int {
	wTime := kingmanLatencyMs * workerAmp / 1000.0
	w := int(math.Ceil(throughputPerPod * wTime))
	if workerFloor > 0 && w < workerFloor {
		w = workerFloor
	}
	if workerCeil > 0 && w > workerCeil {
		w = workerCeil
	}
	return w
}

// REPLICA COUNT — pod throughput (k6 benchmarks)
// [DERIVED] podCap = rps_per_core × (cpu_mcores / 1000)
//
//	minReplicas = max(ceil(λ_nominal / podCap), min_replicas_default)
//	maxReplicas = min(max(ceil(λ_peak / podCap × az_factor), minReplicas), max_replicas_default)
func replicaCounts(nominal, peak, podCap, azFactor float64, minFloor, maxCap int) (int, int) {
	minR := int(math.Ceil(nominal / podCap))
	if minFloor > 0 && minR < minFloor {
		minR = minFloor
	}
	maxR := int(math.Ceil(peak / podCap * azFactor))
	if maxR < minR {
		maxR = minR
	}
	if maxCap > 0 && maxR > maxCap {
		maxR = maxCap
	}
	if maxR < minR {
		maxR = minR
	}
	return minR, maxR
}

// RETRY BUDGET — exponential backoff with log2 derivation
// [DERIVED] retry_budget_ms = slo_per_hop × (1 − slack) × retry_fraction
//
//	max_retries = floor(log2(2 × budget / base + 1))
//	cap = min(base × 2^(max_retries−1), budget)
//	when retries=0: cap = base × 4 (fallback)
func maxRetries(sloLatencyMS int, slackPct, avgMS, retryFrac float64) int {
	budget := float64(sloLatencyMS) * (1 - slackPct) * retryFrac
	base := math.Max(1, math.Ceil(avgMS))
	if budget/base <= 0 {
		return 0
	}
	// Correct algebraic inverse of the geometric sum (without the 2* multiplier).
	mr := int(math.Floor(math.Log2(budget/base + 1.0)))
	if mr < 0 {
		return 0
	}
	return mr
}

func backoffCap(baseMS, nRetries, perHopMS int, slackPct, retryFrac float64) int {
	if nRetries == 0 {
		return baseMS * 4
	}
	budget := int(float64(perHopMS) * (1 - slackPct) * retryFrac)
	expCap := baseMS * int(math.Pow(2, float64(nRetries-1)))
	if expCap < budget {
		return expCap
	}
	return budget
}

// Note: `perHopMS` here is now `sloLatencyMS` (the SLO budget) — see the
// per-call comment in maxRetries. The formula structure is unchanged but
// the input is corrected.

// SESSION & HEARTBEAT — Kafka consumer group protocol (KIP-62)
// [DERIVED] SessionMs = ProcessTimeout + DLQ_total + 2×buffer
// [SOURCED] HeartbeatMs = SessionMs / 3  (heartbeat ≤ session/3)
func sessionTiming(processTO, dlqTotal, bufferMS int) (session, heartbeat int) {
	// Re-couple to KIP-62 reality for segmentio/kafka-go which doesn't support max.poll.interval.ms
	session = processTO + dlqTotal + (2 * bufferMS)
	heartbeat = session / 3
	if session < DefaultSessionTimeoutMS {
		session = DefaultSessionTimeoutMS
	}
	if heartbeat < DefaultHeartbeatTimeoutMS {
		heartbeat = DefaultHeartbeatTimeoutMS
	}
	return
}

// KAFKA PARTITION COUNT
// [SOURCED] partitions = max(ceil(λ_topic / per_partition_consume), maxReplicas)
func partitionCount(topicPeak float64, perPartRPS float64, maxReplicas int) int {
	return int(math.Max(math.Ceil(topicPeak/perPartRPS), float64(maxReplicas)))
}

// KEDA LAG THRESHOLD — acceptable backlog per pod based on Little's Law.
func lagThreshold(sloLatencyMS int, avgMS float64, workers int) int {
	return int(math.Ceil(float64(sloLatencyMS)/avgMS)) * workers
}

// HTTP OUTBOUND POOL — Go net/http Transport
// [DERIVED] httpPool = max(ceil(qps × latency_s × headroom), worker_count)
//
//	perHost = ceil(pool / host_count)   (distribute across merchant hosts)
func httpPoolSize(qps, latencySec, headroom float64, workers, hostCount int) (pool, perHost int) {
	reqP := int(math.Ceil(qps * latencySec * headroom))
	pool = max(reqP, workers)
	perHost = int(math.Ceil(float64(pool) / float64(hostCount)))
	return
}

// WEBHOOK PER-MERCHANT BULKHEAD (issue 24)
// [DERIVED] per_merchant = max(1, ceil(workers × 0.10))
func webhookMaxConcurrency(workers int) int {
	if workers <= 0 {
		return 1
	}
	v := int(math.Ceil(float64(workers) * WebhookConcurrencyRatio))
	if v < 1 {
		v = 1
	}
	if v > workers {
		v = workers
	}
	return v
}

// WEBHOOK BREAKER EVICTION TTL (issue 26)
// [DERIVED] eviction_ttl_ms = max(5min, 10 × max(delivery_backoff_base_ms, dlq_base_delay_ms))
func breakerEvictionTTL(deliveryBackoffBaseMS, dlqBaseDelayMS int) int {
	base := deliveryBackoffBaseMS
	if dlqBaseDelayMS > base {
		base = dlqBaseDelayMS
	}
	ttl := 10 * base
	if ttl < DefaultBreakerEvictionTTLMS {
		ttl = DefaultBreakerEvictionTTLMS
	}
	return ttl
}

// RELAY (OUTBOX) DERIVED VALUES
// [DERIVED] fetch_batch = min(max_fetch_batch, floor(sloLatencyMS / avgMS))
//
// [DERIVED] batch_timeout = fetch_batch × ceil(avgMS)
// [DERIVED] pool_interval = max(1, ceil(1000 × fetch_batch / totalPeak) − 1)
// [DERIVED] relay_replicas = ceil(totalPeak / (producer_tput × drain_window_s))
func relayDerived(totalPeak, avgMS, producerThroughput, sloLatencyMS float64, maxFetchBatch, bufferMaxPollIntervalMS int) (fetchBatch, poolInterval, batchTimeout, replicas int) {
	// Reserve only 40% of SLO for DB fetch to leave room for Kafka produce/network.
	budgetDriven := int(math.Floor((sloLatencyMS * 0.4) / math.Max(avgMS, 1)))
	fetchBatch = budgetDriven
	if maxFetchBatch > 0 && fetchBatch > maxFetchBatch {
		fetchBatch = maxFetchBatch
	}
	if fetchBatch < 1 {
		fetchBatch = 1
	}
	// Replicas must sustain the arrival rate independent of any drain window.
	replicas = int(math.Ceil(totalPeak / producerThroughput))
	if replicas < 1 {
		replicas = 1
	}
	batchTimeout = fetchBatch * int(math.Ceil(avgMS))
	// Pool interval bounded by cluster peak rate, with a 1000ms minimum floor for idle DB stability.
	podSharePeak := math.Max(totalPeak/float64(replicas), 1)
	poolInterval = max(500, int(math.Ceil(1000*float64(fetchBatch)/podSharePeak))-1)
	// Cap at buffer_max_poll_interval_ms to enforce AIMD backoff ceiling from capacity engine.
	if bufferMaxPollIntervalMS > 0 && poolInterval > bufferMaxPollIntervalMS {
		poolInterval = bufferMaxPollIntervalMS
	}
	return
}

// CPU REQUEST — pod resource sizing
// [DERIVED] cpu_mcores = ceil(λ_nominal / rps_per_core × 1000)
func cpuRequest(nominal float64, rpsPerCore float64) int {
	return int(math.Ceil(nominal / rpsPerCore * 1000))
}

// MEMORY REQUEST — pool + TLS + runtime baseline + in-flight concurrency heap (B17)
// [DERIVED] mem_mib = (pool × 50KB + http × 50KB + 64MiB) / BytesPerMiB + inFlightMB
func memRequest(poolSize, httpPool, inFlightMB int) int {
	return int((int64(poolSize*PodPGConnMemBytes)+int64(httpPool*PodHTTPTLSMemBytes)+PodAPPBaselineMemBytes)/
		int64(BytesPerMiB)) + inFlightMB
}

// RELAY MEMORY REQUEST — adds staging buffer + fetch batch buffer to the
// baseline memory calculation for the outbox-relay service.
// [DERIVED] relay_mem_mib = memRequest(pool, 0, 0) + staging_kb/1024 + max(1, fetchBatch × maxPayloadKB / 1024)
func relayMemRequest(poolSize, stagingKB, fetchBatch, maxPayloadKB int) int {
	base := memRequest(poolSize, 0, 0)
	stagingMiB := stagingKB / 1024
	fetchMiB := fetchBatch * maxPayloadKB / 1024
	if fetchMiB < 1 {
		fetchMiB = 1
	}
	totalMiB := base + stagingMiB + fetchMiB
	if totalMiB < base {
		totalMiB = base // overflow guard
	}
	return totalMiB
}

// [SOURCED] optimal_active = (db_cores × 2) + effective_spindles
//
//	cores are PHYSICAL (exclude HT), spindles: 0=cached SSD, 1=SSD
func optimalActive(cores, spindles int) int {
	return cores*2 + spindles
}

// PG MAX CONNECTIONS — derived from RAM, shared_buffers, work_mem (PG wiki)
// [SOURCED] shared_buffers = RAM × shared_buffers_pct (25% standard — PG wiki)
// [SOURCED] OS page cache ≈ RAM × os_buffer_pct (25% standard)
// [SOURCED] maintenance_work_mem ≈ RAM × maintenance_pct (15% — VACUUM/ANALYZE)
// [DERIVED] available = RAM − shared − os − maintenance
// [DERIVED] per_conn_mem = (work_mem_mb × hash_mem_multiplier) + connection_overhead
//   hash_mem_multiplier = 2 (each hash join can use 2× work_mem)
//   connection_overhead = 8 MB baseline per-connection structures
// [DERIVED] max_connections = floor(available / per_conn_mem)

func pgMaxConnections(ramBytes int64, workMemMB int, sharedPct, osPct, maintPct float64) int {
	totalMB := int(ramBytes / (1024 * 1024))
	sharedBuf := int(float64(totalMB) * sharedPct)
	osBuf := int(float64(totalMB) * osPct)
	maintBuf := int(float64(totalMB) * maintPct)
	available := totalMB - sharedBuf - osBuf - maintBuf
	// Fractional work_mem assumes not all connections execute heavy sorts concurrently.
	perConn := int(math.Ceil(float64(workMemMB)*0.25)) + ConnectionOverheadMB
	if available <= 0 {
		return 1
	}
	return available / perConn
}

// KAFKA CLUSTER CAP — partition budget
// [SOURCED] cluster_cap = min(brokers × per_broker_cap, 200_000)
func kafkaClusterCap(brokers, perBrokerCap int) int {
	c := brokers * perBrokerCap
	if c > KafkaClusterMaxParts {
		c = KafkaClusterMaxParts
	}
	return c
}

// REDIS MAX MEMORY — fork + fragmentation budget
// [SOURCED] maxmem = ram_bytes × (1 − fork_headroom) / fragmentation
func redisMaxMem(ramBytes int64, forkHeadroom, fragmentation float64) int64 {
	return int64(math.Floor(float64(ramBytes) * (1 - forkHeadroom) / fragmentation))
}

// SERVER IDLE TIMEOUT — Go HTTP server
// [DERIVED] idle_ms = SessionMs + HeartbeatMs  (stay alive through heartbeat cycle)
func serverIdleTimeout(sessionMs, heartbeatMs int) int {
	return sessionMs + heartbeatMs
}

// SHUTDOWN TIMEOUT — Graceful termination period for Kubernetes SIGTERM
// [DERIVED] shutdown_ms = max(30000, SessionMs + 15000)
func shutdownTimeout(sessionMs int) int {
	s := sessionMs + ShutdownTimeoutBufferMS
	if s < ShutdownTimeoutMinMS {
		return ShutdownTimeoutMinMS
	}
	return s
}

// DLQ RETRY FROM PROCESS TIMEOUT — fits inside the per-message deadline
// [DERIVED] MaxRetries = 2 (fixed; at-least-one-retry per user requirement)
//
//	BaseDelay = floor(ProcessTimeout × 0.5 / 8)   // small initial backoff
//	MaxDelay  = floor(ProcessTimeout × 0.5 / 4)   // cap at quarter DLQ budget
//	Worst case total: 2 × (BaseDelay + MaxDelay) = 3/4 × DLQ_budget ≤ DLQ_budget ✓
//
// The hard upper bound `maxRetries` is operator-tunable via defaults
// (so we never exceed the cap even if ProcessTimeout is large).
func dlqRetryFromProcess(processTimeoutMs, maxRetriesCap int) (maxRetries, baseDelay, maxDelay int) {
	dlqBudget := processTimeoutMs / 2 // 0.5 of the process timeout
	maxRetries = 2                    // max number of retries
	if maxRetriesCap > 0 && maxRetries > maxRetriesCap {
		maxRetries = maxRetriesCap
	}
	baseDelay = dlqBudget / 8
	maxDelay = dlqBudget / 4
	if baseDelay < 1 {
		baseDelay = 1
	}
	if maxDelay < baseDelay {
		maxDelay = baseDelay
	}
	return
}

// CONSUMER TIMING — platform pipeline (B14)
// [DERIVED] drain_timeout = 2 × session_buffer
//
//	channel_refresh = session_buffer
//	fetch_grab = fetch_backoff_min
func consumerDrainTimeout(bufferMS int) int   { return 2 * bufferMS }
func consumerChannelRefresh(bufferMS int) int { return bufferMS }

// WEIGHTED AVERAGE SERVICE TIME — across endpoints
// [DERIVED] avgMS = Σ(S_i × λ_peak_i) / Σ(λ_peak_i)
//
//	Returns the weighted mean and total peak QPS.
func weightedAvgMS(svc Service) (avgMS, totalPeak float64) {
	for _, ep := range svc.Endpoints {
		totalPeak += ep.PeakQPS
		avgMS += ep.AvgQueryTimeMS * ep.PeakQPS
	}
	if totalPeak > 0 {
		avgMS /= totalPeak
	}
	if svc.HTTP != nil {
		avgMS += svc.HTTP.AvgLatencyS * 1000.0
	}
	return
}

// POD CAPACITY — throughput per pod (k6 benchmarks, B17)
// [DERIVED] podCap = rps_per_core × cores_per_pod

func podCapacity(rpsPerCore float64, coresPerPod int) float64 {
	return rpsPerCore * float64(coresPerPod)
}

// KAFKA SEGMENT COUNT — log segment files (supply.go)
// [DERIVED] segments = ceil(retention_seconds / segment_seconds)
//
//	Each segment = one.index + one.log file on disk.
func kafkaSegmentCount(retentionDays, segmentSeconds int) int {
	return int(math.Ceil(float64(retentionDays*24*3600) / float64(segmentSeconds)))
}

// KAFKA FD ESTIMATE — open file handles per broker
// [SOURCED] Jun Rao 2015: each partition owns index + data files per segment.
// [DERIVED] fd = per_broker_cap × (segments + 1) × 2
//
//	+1 for active (unsealed) segment, ×2 for index + data.
func kafkaFDEstimate(perBrokerCap, segCount int) int {
	return perBrokerCap * (segCount + 1) * 2
}

// CONNECTION DEMAND — total connections per DB instance (Fit-Check)
// [DERIVED] demand = pool_size × replicas
func connectionDemand(poolSize, replicas int) int {
	return poolSize * replicas
}

// HPA CAP — max replicas from remaining connection budget (Fit-Check)
// [DERIVED] cap = max(floor(gap / pool), minReplicas)
//
//	gap = usable_conns − Σ_{j≠i}(pool_j × replicas_j)
func hpaCap(gap, pool, minReplicas int) int {
	if pool == 0 {
		return minReplicas
	}
	cap := gap / pool
	if cap < minReplicas {
		return minReplicas
	}
	return cap
}

// STANDARD CPU REQUEST — rounds up raw mcores to standard 100m units (minimum 200m floor)
func standardCPURequest(mcores int) int {
	if mcores <= 200 {
		return 200
	}
	return int(math.Ceil(float64(mcores)/100.0)) * 100
}

// STANDARD MEMORY REQUEST — rounds up raw MiB to standard cloud power-of-2 / tier (64, 128, 256, 512, 1024 MiB)
func standardMemRequest(mib int) int {
	if mib <= 64 {
		return 64
	}
	if mib <= 128 {
		return 128
	}
	if mib <= 256 {
		return 256
	}
	if mib <= 512 {
		return 512
	}
	return int(math.Ceil(float64(mib)/256.0)) * 256
}

// STANDARD CPU LIMIT — resolves pod CPU limit from service or infra config
func standardCPULimit(svc Service, input *SLOInput) string {
	if svc.CoresPerPod > 0 {
		return fmt.Sprintf("%d", svc.CoresPerPod)
	}
	if input.Infra.Pod.CPULimitM > 0 {
		return fmt.Sprintf("%d", int(math.Ceil(float64(input.Infra.Pod.CPULimitM)/1000.0)))
	}
	return "1"
}
func standardMemLimit(svc Service, input *SLOInput) int {
	if svc.MemLimitBytes > 0 {
		return int(svc.MemLimitBytes / (1024 * 1024))
	}
	if input.Infra.Pod.MemLimitBytes > 0 {
		return int(input.Infra.Pod.MemLimitBytes / (1024 * 1024))
	}
	return 256
}
