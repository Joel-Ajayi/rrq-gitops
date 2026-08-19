package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func render(svcs map[string]Derived, pg map[string]PGCeiling, kc KafkaCeiling, rc []RedisCeiling, fails []string, warns []string, input *SLOInput, outputPath string, rootDir string) error {
	// Per-service + platform ConfigMaps consumed via envFrom by the
	// deployments (base/workloads/config, wired in base/workloads).
	dir := filepath.Join(rootDir, "base", "workloads", "config")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := renderPlatform(dir, pg, rc, input, svcs); err != nil {
		return err
	}
	for name, d := range svcs {
		svc := findService(input, name)
		if err := renderService(dir, name, d, svc, input); err != nil {
			return err
		}
	}
	if err := renderKustomization(dir, svcs); err != nil {
		return err
	}
	return renderReport(outputPath, svcs, pg, kc, rc, fails, warns)
}

func renderKustomization(dir string, svcs map[string]Derived) error {
	resources := []string{"platform-configmap.yaml"}
	for name := range svcs {
		resources = append(resources, name+"-configmap.yaml")
	}
	kust := map[string]interface{}{
		"apiVersion": "kustomize.config.k8s.io/v1beta1",
		"kind":       "Kustomization",
		"resources":  resources,
	}
	return writeYAML(filepath.Join(dir, "kustomization.yaml"), kust)
}

// renderPlatform writes the shared platform ConfigMap
// with infra-level settings every service inherits via envFrom.
func renderPlatform(dir string, pg map[string]PGCeiling, rc []RedisCeiling, input *SLOInput, svcs map[string]Derived) error {
	d := input.Defaults
	k := input.Infra.Kafka
	r := input.Infra.Redis

	m := map[string]string{
		"KAFKA_READER_MIN_BYTES":            fmt.Sprintf("%d", k.ReaderMinBytes),
		"KAFKA_READER_MAX_BYTES":            fmt.Sprintf("%d", k.ReaderMaxBytes),
		"KAFKA_READER_MAX_WAIT_MS":          fmt.Sprintf("%d", k.ReaderMaxWaitMs),
		"KAFKA_WRITER_MAX_ATTEMPTS":         fmt.Sprintf("%d", k.WriterMaxAttempts),
		"CONSUMER_MAX_PENDING_BYTES":        fmt.Sprintf("%d", d.ConsumerMaxPendingMB*1024*1024),
		"CONSUMER_CHANNEL_REFRESH_MS":       fmt.Sprintf("%d", consumerChannelRefresh(d.ConsumerSessionBufferMS)),
		"CONSUMER_DRAIN_TIMEOUT_MS":         fmt.Sprintf("%d", consumerDrainTimeout(d.ConsumerSessionBufferMS)),
		"CONSUMER_COMMIT_FLUSH_INTERVAL_MS": fmt.Sprintf("%d", d.ConsumerCommitFlushMS),
		"CONSUMER_COMMIT_FLUSH_TIMEOUT_MS":  fmt.Sprintf("%d", d.ConsumerCommitFlushMS),
		"CONSUMER_COMMIT_BATCH_CAPACITY":    fmt.Sprintf("%d", d.ConsumerPartitionSize),
		"CONSUMER_PARTITION_CHANNEL_SIZE":   fmt.Sprintf("%d", d.ConsumerPartitionSize),
		"CONSUMER_MIN_COMMIT_CAP_FRAC":      fmt.Sprintf("%g", d.ConsumerMinCommitCapFrac),
		"KETAMA_VNODES":                     fmt.Sprintf("%d", r.KetamaVnodes),
		"PG_CONN_MAX_IDLE_TIME_MS":          fmt.Sprintf("%d", input.Infra.PG.Connection.MaxIdleMS),
		"PG_CONN_MAX_LIFETIME_MS":           fmt.Sprintf("%d", input.Infra.PG.Connection.MaxLifetimeMS),
		"RETRY_BUDGET_MIN_TOKENS":           fmt.Sprintf("%d", d.RetryBudgetMinTokens),
		"RETRY_BUDGET_MAX_TOKENS":           fmt.Sprintf("%d", d.RetryBudgetMaxTokens),
		"RETRY_BUDGET_FRACTION":             fmt.Sprintf("%g", d.RetryBudgetFraction),
	}

	if len(rc) > 0 {
		m["REDIS_MAXMEMORY_MIB"] = fmt.Sprintf("%d", rc[0].MaxMemoryBytes/(1024*1024))
	}

	return writeYAML(dir+"/platform-configmap.yaml", configMap("platform-config", m))
}

func instTotalDemand(instName string, svcs map[string]Derived, input *SLOInput) int {
	total := 0
	for name, d := range svcs {
		if serviceHitsInstance(input, name, instName) {
			total += connectionDemand(d.PoolSize, d.MaxReplicas)
		}
	}
	return total
}

func instRODemand(instName string, svcs map[string]Derived, input *SLOInput) int {
	total := 0
	for name, d := range svcs {
		for _, svc := range input.Services {
			if svc.Name != name {
				continue
			}
			allRO := true
			for _, ep := range svc.Endpoints {
				if ep.DBInstance == instName && ep.WritesPerMessage > 0 {
					allRO = false
					break
				}
			}
			if allRO && serviceHitsInstance(input, name, instName) {
				total += connectionDemand(d.PoolSize, d.MaxReplicas)
			}
		}
	}
	return total
}

func serviceHitsInstance(input *SLOInput, svcName, instName string) bool {
	for _, svc := range input.Services {
		if svc.Name == svcName {
			for _, ep := range svc.Endpoints {
				for _, inst := range ep.GetDBInstances() {
					if inst == instName {
						return true
					}
				}
			}
		}
	}
	return false
}

func serviceOnlyRO(input *SLOInput, svcName string) bool {
	for _, svc := range input.Services {
		if svc.Name == svcName {
			for _, ep := range svc.Endpoints {
				if ep.WritesPerMessage > 0 {
					return false
				}
			}
			return true
		}
	}
	return false
}

func renderService(dir, name string, d Derived, svc *Service, input *SLOInput) error {
	if svc == nil {
		return nil
	}
	prefix := envPrefix(name)

	m := map[string]string{
		"DB_POOL_SIZE":     fmt.Sprintf("%d", d.PoolSize),
		"WORKER_POOL_SIZE": fmt.Sprintf("%d", d.Workers),
		// ProcessTimeout (per-message deadline, SLO-derived via Kingman)
		"REQUEST_TIMEOUT_MS":      fmt.Sprintf("%d", d.ProcessTimeoutMs),
		"SERVER_TIMEOUT_MS":       fmt.Sprintf("%d", d.SessionMs),
		"SHUTDOWN_TIMEOUT_MS":     fmt.Sprintf("%d", d.ShutdownTimeoutMs),
		"SERVER_IDLE_TIMEOUT_MS":  fmt.Sprintf("%d", serverIdleTimeout(d.SessionMs, d.HeartbeatMs)),
		"MAX_RETRIES":             fmt.Sprintf("%d", d.MaxRetries),
		"BACKOFF_BASE_MS":         fmt.Sprintf("%d", d.BackoffBaseMS),
		"BACKOFF_CAP_MS":          fmt.Sprintf("%d", d.BackoffCapMS),
		"DLQ_MAX_RETRIES":         fmt.Sprintf("%d", d.DLQMaxRetries),
		"DLQ_BASE_DELAY_MS":       fmt.Sprintf("%d", d.DLQBaseDelayMs),
		"DLQ_CAP_DELAY_MS":        fmt.Sprintf("%d", d.DLQCapDelayMs),
		"DLQ_WRITE_TIMEOUT_MS":    fmt.Sprintf("%d", d.DLQWriteTimeoutMs),
		"POD_MEM_REQUEST_MIB":     fmt.Sprintf("%d", d.MemRequest),
		"RETRY_BUDGET_MIN_TOKENS": fmt.Sprintf("%d", d.RetryBudgetMinTokens),
		"RETRY_BUDGET_MAX_TOKENS": fmt.Sprintf("%d", d.RetryBudgetMaxTokens),
		"RETRY_BUDGET_FRACTION":   fmt.Sprintf("%g", input.Defaults.RetryBudgetFraction),
	}

	// Per-pod per-shard RW cap (engine-derived). Only render shards this service touches.
	for shardName, cap := range d.PerShardRW {
		envName := "PG_" + strings.ToUpper(strings.ReplaceAll(shardName, "-", "_")) + "_RW_MAX_CONNS"
		if cap > 0 {
			m[envName] = fmt.Sprintf("%d", cap)
		}
	}
	for shardName, cap := range d.PerShardRO {
		envName := "PG_" + strings.ToUpper(strings.ReplaceAll(shardName, "-", "_")) + "_RO_MAX_CONNS"
		if cap > 0 {
			m[envName] = fmt.Sprintf("%d", cap)
		}
	}

	// DLQ retry config is now derived from ProcessTimeout (above) — no
	// need to render the hardcoded defaults here.

	// Circuit breaker — direct from Service input, not Derived
	if svc.CB != nil {
		m["CB_ERROR_THRESHOLD"] = fmt.Sprintf("%g", svc.CB.ErrorThreshold)
		m["CB_MIN_REQUESTS"] = fmt.Sprintf("%d", svc.CB.MinRequests)
		m["CB_TIMEOUT_MS"] = fmt.Sprintf("%d", d.CircuitBreakerTimeoutMs)
		m["CB_INTERVAL_MS"] = fmt.Sprintf("%d", svc.CB.IntervalMs)
		m["CB_MAX_FAILS"] = fmt.Sprintf("%d", svc.CB.MaxFails)
		m["CB_HALF_OPEN_PROBES"] = fmt.Sprintf("%d", svc.CB.HalfOpenProbes)
	}

	// HPA
	if svc.HPATargetCPU > 0 {
		m["HPA_TARGET_CPU"] = fmt.Sprintf("%g", svc.HPATargetCPU)
	}
	// Per-service pod memory limit (overrides infra pod default)
	if svc.MemLimitBytes > 0 {
		m["POD_MEM_LIMIT_BYTES"] = fmt.Sprintf("%d", svc.MemLimitBytes)
	}

	// Kafka — only for consumer/producer roles
	if svc.Role == "consumer" || svc.Role == "producer" {
		m["KAFKA_SESSION_MS"] = fmt.Sprintf("%d", d.SessionMs)
		m["KAFKA_HEARTBEAT_MS"] = fmt.Sprintf("%d", d.HeartbeatMs)
	}
	if svc.Role == "consumer" {
		if d.LagThreshold > 0 {
			m["KEDA_LAG_THRESHOLD"] = fmt.Sprintf("%d", d.LagThreshold)
		}
	}

	// API-specific config
	if svc.Role == "api" {
		if svc.JWTAccessHrs > 0 {
			m["JWT_ACCESS_HRS"] = fmt.Sprintf("%d", svc.JWTAccessHrs)
		}
		if svc.Argon2MemoryKib > 0 {
			m["ARGON2_MEMORY_KIB"] = fmt.Sprintf("%d", svc.Argon2MemoryKib)
			m["ARGON2_ITERATIONS"] = fmt.Sprintf("%d", svc.Argon2Iterations)
			m["ARGON2_PARALLELISM"] = fmt.Sprintf("%d", svc.Argon2Parallelism)
		}
		if svc.MaxRequestBytes > 0 {
			m["MAX_REQUEST_BYTES"] = fmt.Sprintf("%d", svc.MaxRequestBytes)
		}
	}

	// Velocity — fraud-worker specific
	if svc.VelocityThreshold > 0 {
		m["VELOCITY_THRESHOLD"] = fmt.Sprintf("%g", svc.VelocityThreshold)
		m["VELOCITY_WINDOW_MS"] = fmt.Sprintf("%d", svc.VelocityWindowMS)
	}

	// HTTP outbound — webhook-worker specific
	if d.HTTPPool > 0 && svc.HTTP != nil {
		m["HTTP_MAX_IDLE_CONNS"] = fmt.Sprintf("%d", d.HTTPPool)
		m["HTTP_MAX_IDLE_PER_HOST"] = fmt.Sprintf("%d", d.HTTPPerHost)
		m["HTTP_TIMEOUT_MS"] = fmt.Sprintf("%d", svc.HTTP.TimeoutMS)
		m["HTTP_IDLE_CONN_TIMEOUT_MS"] = fmt.Sprintf("%d", svc.HTTP.IdleConnTimeoutMS)
		m["HTTP_RESPONSE_HEADER_TIMEOUT_MS"] = fmt.Sprintf("%d", svc.HTTP.ResponseHeaderTimeoutMS)
		m["HTTP_TLS_HANDSHAKE_TIMEOUT_MS"] = fmt.Sprintf("%d", svc.HTTP.TLSHandshakeTimeoutMS)
		m["HTTP_EXPECT_CONTINUE_TIMEOUT_MS"] = fmt.Sprintf("%d", svc.HTTP.ExpectContinueTimeoutMS)
	}

	// Outbox relay
	if d.RelayReplicas > 0 {
		m["FETCH_BATCH_SIZE"] = fmt.Sprintf("%d", d.RelayFetchBatch)
		m["RELAY_POOL_INTERVAL_MS"] = fmt.Sprintf("%d", d.RelayPoolIntervalMS)
		m["RELAY_BATCH_TIMEOUT_MS"] = fmt.Sprintf("%d", d.RelayBatchTimeoutMS)
		if svc.Relay != nil {
			r := svc.Relay
			m["STAGING_KB"] = fmt.Sprintf("%d", r.StagingKB)
			m["RELAY_MAX_PAYLOAD_BYTES"] = fmt.Sprintf("%d", r.MaxPayloadKB*1024)
			m["RELAY_BUFFER_SAMPLE_INTERVAL_MS"] = fmt.Sprintf("%d", r.BufferSampleIntervalMS)
			m["RELAY_BUFFER_MAX_THROTTLE_LEVEL"] = fmt.Sprintf("%d", r.BufferMaxThrottleLevel)
			m["RELAY_BUFFER_MAX_POLL_INTERVAL_MS"] = fmt.Sprintf("%d", r.BufferMaxPollIntervalMS)
			m["AIMD_THROTTLE_FRAC"] = fmt.Sprintf("%g", r.AIMDThrottleFrac)
			m["AIMD_PAUSE_FRAC"] = fmt.Sprintf("%g", r.AIMDPauseFrac)
			m["AIMD_RESUME_FRAC"] = fmt.Sprintf("%g", r.AIMDResumeFrac)
		}
	}

	// Webhook delivery
	if svc.Webhook != nil {
		w := svc.Webhook
		m["DELIVERY_MAX_ATTEMPTS"] = fmt.Sprintf("%d", w.DeliveryMaxAttempts)
		m["DELIVERY_BACKOFF_BASE_MS"] = fmt.Sprintf("%d", w.DeliveryBackoffBaseMS)
		m["DELIVERY_BACKOFF_CAP_MS"] = fmt.Sprintf("%d", w.DeliveryBackoffCapMS)
		m["SCHEDULER_POLL_INTERVAL_MS"] = fmt.Sprintf("%d", w.SchedulerPollIntervalMS)
		m["SCHEDULER_BATCH_SIZE"] = fmt.Sprintf("%d", w.SchedulerBatchSize)
		m["FAST_LANE_GRACE_PERIOD_MS"] = fmt.Sprintf("%d", w.FastLaneGracePeriodMS)
		m["FAST_LANE_BUFFER_SIZE"] = fmt.Sprintf("%d", w.FastLaneBufferSize)
		m["FAST_LANE_WORKER_POOL_SIZE"] = fmt.Sprintf("%d", d.FastLaneWorkerPoolSize)
		m["BREAKER_EVICTION_INTERVAL_MS"] = fmt.Sprintf("%d", w.BreakerEvictionIntervalMS)
		evTTL := w.BreakerEvictionTTLMS
		if evTTL <= 0 {
			evTTL = d.BreakerEvictionTTLMS
		}
		if evTTL > 0 {
			m["BREAKER_EVICTION_TTL_MS"] = fmt.Sprintf("%d", evTTL)
		}
		// Per-merchant bulkhead concurrency. Falls back to the package default
		// (50) at runtime if this is 0 — see issue 24.
		maxConc := w.MaxConcurrencyPerMerchant
		if maxConc <= 0 {
			maxConc = d.WebhookMaxConcurrency
		}
		if maxConc > 0 {
			m["WEBHOOK_MAX_CONCURRENCY"] = fmt.Sprintf("%d", maxConc)
		}
	}

	prefixed := map[string]string{}
	for k, v := range m {
		prefixed[prefix+"_"+k] = v
	}
	return writeYAML(dir+"/"+name+"-configmap.yaml", configMap(name+"-config", prefixed))
}

func platformData(input *SLOInput, svcs map[string]Derived) map[string]string {
	d := input.Defaults
	k := input.Infra.Kafka
	r := input.Infra.Redis

	m := map[string]string{
		"KAFKA_READER_MIN_BYTES":            fmt.Sprintf("%d", k.ReaderMinBytes),
		"KAFKA_READER_MAX_BYTES":            fmt.Sprintf("%d", k.ReaderMaxBytes),
		"KAFKA_READER_MAX_WAIT_MS":          fmt.Sprintf("%d", k.ReaderMaxWaitMs),
		"KAFKA_WRITER_MAX_ATTEMPTS":         fmt.Sprintf("%d", k.WriterMaxAttempts),
		"CONSUMER_MAX_PENDING_BYTES":        fmt.Sprintf("%d", d.ConsumerMaxPendingMB*1024*1024),
		"CONSUMER_CHANNEL_REFRESH_MS":       fmt.Sprintf("%d", consumerChannelRefresh(d.ConsumerSessionBufferMS)),
		"CONSUMER_DRAIN_TIMEOUT_MS":         fmt.Sprintf("%d", consumerDrainTimeout(d.ConsumerSessionBufferMS)),
		"CONSUMER_COMMIT_FLUSH_INTERVAL_MS": fmt.Sprintf("%d", d.ConsumerCommitFlushMS),
		"CONSUMER_COMMIT_FLUSH_TIMEOUT_MS":  fmt.Sprintf("%d", d.ConsumerCommitFlushMS),
		"CONSUMER_COMMIT_BATCH_CAPACITY":    fmt.Sprintf("%d", d.ConsumerPartitionSize),
		"CONSUMER_PARTITION_CHANNEL_SIZE":   fmt.Sprintf("%d", d.ConsumerPartitionSize),
		"CONSUMER_MIN_COMMIT_CAP_FRAC":      fmt.Sprintf("%g", d.ConsumerMinCommitCapFrac),
		"KETAMA_VNODES":                     fmt.Sprintf("%d", r.KetamaVnodes),
		"PG_CONN_MAX_IDLE_TIME_MS":          fmt.Sprintf("%d", input.Infra.PG.Connection.MaxIdleMS),
		"PG_CONN_MAX_LIFETIME_MS":           fmt.Sprintf("%d", input.Infra.PG.Connection.MaxLifetimeMS),
	}

	for name := range input.Infra.PG.Instances {
		roDemand := instRODemand(name, svcs, input)
		formattedName := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		m["PG_"+formattedName+"_RO_MAX_CONNS"] = fmt.Sprintf("%d", roDemand)
	}

	redisNode := input.Infra.Redis
	mem := int64(float64(redisNode.RAMBytesPerNode) * (1 - redisNode.ForkHeadroom) / redisNode.Fragmentation)
	m["REDIS_MAXMEMORY_MIB"] = fmt.Sprintf("%d", mem/(1024*1024))
	return m
}

func instName(inp *SLOInput, inst PGInstance) string {
	for n, i := range inp.Infra.PG.Instances {
		if i.Cores == inst.Cores && i.RAMBytes == inst.RAMBytes && i.EffectiveSpindles == inst.EffectiveSpindles {
			return n
		}
	}
	return ""
}

func serviceValues(name string, d Derived, svc *Service, input *SLOInput) map[string]string {
	if svc == nil {
		return nil
	}
	prefix := envPrefix(name)

	m := map[string]string{
		"DB_POOL_SIZE":           fmt.Sprintf("%d", d.PoolSize),
		"WORKER_POOL_SIZE":       fmt.Sprintf("%d", d.Workers),
		"REQUEST_TIMEOUT_MS":     fmt.Sprintf("%d", d.ProcessTimeoutMs),
		"SERVER_TIMEOUT_MS":      fmt.Sprintf("%d", d.SessionMs),
		"SHUTDOWN_TIMEOUT_MS":    fmt.Sprintf("%d", d.ShutdownTimeoutMs),
		"SERVER_IDLE_TIMEOUT_MS": fmt.Sprintf("%d", serverIdleTimeout(d.SessionMs, d.HeartbeatMs)),
		"MAX_RETRIES":            fmt.Sprintf("%d", d.MaxRetries),
		"BACKOFF_BASE_MS":        fmt.Sprintf("%d", d.BackoffBaseMS),
		"BACKOFF_CAP_MS":         fmt.Sprintf("%d", d.BackoffCapMS),
		"DLQ_MAX_RETRIES":        fmt.Sprintf("%d", d.DLQMaxRetries),
		"DLQ_BASE_DELAY_MS":      fmt.Sprintf("%d", d.DLQBaseDelayMs),
		"DLQ_CAP_DELAY_MS":       fmt.Sprintf("%d", d.DLQCapDelayMs),
		"DLQ_WRITE_TIMEOUT_MS":   fmt.Sprintf("%d", d.DLQWriteTimeoutMs),
		"POD_MEM_REQUEST_MIB":    fmt.Sprintf("%d", d.MemRequest),
	}

	for shardName, cap := range d.PerShardRW {
		envName := "PG_" + strings.ToUpper(strings.ReplaceAll(shardName, "-", "_")) + "_RW_MAX_CONNS"
		if cap > 0 {
			m[envName] = fmt.Sprintf("%d", cap)
		}
	}
	for shardName, cap := range d.PerShardRO {
		envName := "PG_" + strings.ToUpper(strings.ReplaceAll(shardName, "-", "_")) + "_RO_MAX_CONNS"
		if cap > 0 {
			m[envName] = fmt.Sprintf("%d", cap)
		}
	}

	if svc.CB != nil {
		m["CB_ERROR_THRESHOLD"] = fmt.Sprintf("%g", svc.CB.ErrorThreshold)
		m["CB_MIN_REQUESTS"] = fmt.Sprintf("%d", svc.CB.MinRequests)
		m["CB_TIMEOUT_MS"] = fmt.Sprintf("%d", d.CircuitBreakerTimeoutMs)
		m["CB_INTERVAL_MS"] = fmt.Sprintf("%d", svc.CB.IntervalMs)
		m["CB_MAX_FAILS"] = fmt.Sprintf("%d", svc.CB.MaxFails)
		m["CB_HALF_OPEN_PROBES"] = fmt.Sprintf("%d", svc.CB.HalfOpenProbes)
	}

	m["KAFKA_SESSION_MS"] = fmt.Sprintf("%d", d.SessionMs)
	m["KAFKA_HEARTBEAT_MS"] = fmt.Sprintf("%d", d.HeartbeatMs)
	if d.LagThreshold > 0 {
		m["KEDA_LAG_THRESHOLD"] = fmt.Sprintf("%d", d.LagThreshold)
	}

	if svc.JWTAccessHrs > 0 {
		m["JWT_ACCESS_HRS"] = fmt.Sprintf("%d", svc.JWTAccessHrs)
		m["ARGON2_MEMORY_KIB"] = fmt.Sprintf("%d", svc.Argon2MemoryKib)
		m["ARGON2_ITERATIONS"] = fmt.Sprintf("%d", svc.Argon2Iterations)
		m["ARGON2_PARALLELISM"] = fmt.Sprintf("%d", svc.Argon2Parallelism)
	}
	if svc.MaxRequestBytes > 0 {
		m["MAX_REQUEST_BYTES"] = fmt.Sprintf("%d", svc.MaxRequestBytes)
	}

	if svc.VelocityThreshold > 0 {
		m["VELOCITY_THRESHOLD"] = fmt.Sprintf("%g", svc.VelocityThreshold)
		m["VELOCITY_WINDOW_MS"] = fmt.Sprintf("%d", svc.VelocityWindowMS)
	}

	if d.HTTPPool > 0 && svc.HTTP != nil {
		m["HTTP_MAX_IDLE_CONNS"] = fmt.Sprintf("%d", d.HTTPPool)
		m["HTTP_MAX_IDLE_PER_HOST"] = fmt.Sprintf("%d", d.HTTPPerHost)
		m["HTTP_TIMEOUT_MS"] = fmt.Sprintf("%d", svc.HTTP.TimeoutMS)
		m["HTTP_IDLE_CONN_TIMEOUT_MS"] = fmt.Sprintf("%d", svc.HTTP.IdleConnTimeoutMS)
		m["HTTP_RESPONSE_HEADER_TIMEOUT_MS"] = fmt.Sprintf("%d", svc.HTTP.ResponseHeaderTimeoutMS)
		m["HTTP_TLS_HANDSHAKE_TIMEOUT_MS"] = fmt.Sprintf("%d", svc.HTTP.TLSHandshakeTimeoutMS)
		m["HTTP_EXPECT_CONTINUE_TIMEOUT_MS"] = fmt.Sprintf("%d", svc.HTTP.ExpectContinueTimeoutMS)
	}

	if d.RelayReplicas > 0 {
		m["FETCH_BATCH_SIZE"] = fmt.Sprintf("%d", d.RelayFetchBatch)
		m["RELAY_POOL_INTERVAL_MS"] = fmt.Sprintf("%d", d.RelayPoolIntervalMS)
		m["RELAY_BATCH_TIMEOUT_MS"] = fmt.Sprintf("%d", d.RelayBatchTimeoutMS)
		if svc.Relay != nil {
			r := svc.Relay
			m["STAGING_KB"] = fmt.Sprintf("%d", r.StagingKB)
			m["RELAY_MAX_PAYLOAD_BYTES"] = fmt.Sprintf("%d", r.MaxPayloadKB*1024)
			m["RELAY_BUFFER_SAMPLE_INTERVAL_MS"] = fmt.Sprintf("%d", r.BufferSampleIntervalMS)
			m["RELAY_BUFFER_MAX_THROTTLE_LEVEL"] = fmt.Sprintf("%d", r.BufferMaxThrottleLevel)
			m["RELAY_BUFFER_MAX_POLL_INTERVAL_MS"] = fmt.Sprintf("%d", r.BufferMaxPollIntervalMS)
			m["AIMD_THROTTLE_FRAC"] = fmt.Sprintf("%g", r.AIMDThrottleFrac)
			m["AIMD_PAUSE_FRAC"] = fmt.Sprintf("%g", r.AIMDPauseFrac)
			m["AIMD_RESUME_FRAC"] = fmt.Sprintf("%g", r.AIMDResumeFrac)
		}
	}

	if svc.Webhook != nil {
		w := svc.Webhook
		m["DELIVERY_MAX_ATTEMPTS"] = fmt.Sprintf("%d", w.DeliveryMaxAttempts)
		m["DELIVERY_BACKOFF_BASE_MS"] = fmt.Sprintf("%d", w.DeliveryBackoffBaseMS)
		m["DELIVERY_BACKOFF_CAP_MS"] = fmt.Sprintf("%d", w.DeliveryBackoffCapMS)
		m["SCHEDULER_POLL_INTERVAL_MS"] = fmt.Sprintf("%d", w.SchedulerPollIntervalMS)
		m["SCHEDULER_BATCH_SIZE"] = fmt.Sprintf("%d", w.SchedulerBatchSize)
		m["FAST_LANE_GRACE_PERIOD_MS"] = fmt.Sprintf("%d", w.FastLaneGracePeriodMS)
		m["FAST_LANE_BUFFER_SIZE"] = fmt.Sprintf("%d", w.FastLaneBufferSize)
		m["FAST_LANE_WORKER_POOL_SIZE"] = fmt.Sprintf("%d", d.FastLaneWorkerPoolSize)
		m["BREAKER_EVICTION_INTERVAL_MS"] = fmt.Sprintf("%d", w.BreakerEvictionIntervalMS)
		evTTL := w.BreakerEvictionTTLMS
		if evTTL <= 0 {
			evTTL = d.BreakerEvictionTTLMS
		}
		if evTTL > 0 {
			m["BREAKER_EVICTION_TTL_MS"] = fmt.Sprintf("%d", evTTL)
		}
		maxConc := w.MaxConcurrencyPerMerchant
		if maxConc <= 0 {
			maxConc = d.WebhookMaxConcurrency
		}
		if maxConc > 0 {
			m["WEBHOOK_MAX_CONCURRENCY"] = fmt.Sprintf("%d", maxConc)
		}
	}

	if svc.HPATargetCPU > 0 {
		m["HPA_TARGET_CPU"] = fmt.Sprintf("%g", svc.HPATargetCPU)
	}
	if svc.MemLimitBytes > 0 {
		m["POD_MEM_LIMIT_BYTES"] = fmt.Sprintf("%d", svc.MemLimitBytes)
	}

	prefixed := map[string]string{}
	for k, v := range m {
		prefixed[prefix+"_"+k] = v
	}
	return prefixed
}

func findService(input *SLOInput, name string) *Service {
	for i := range input.Services {
		if input.Services[i].Name == name {
			return &input.Services[i]
		}
	}
	return nil
}

func renderReport(path string, svcs map[string]Derived, pg map[string]PGCeiling, kc KafkaCeiling, rc []RedisCeiling, fails []string, warns []string) error {
	uniqTopics := map[string]int{}
	for _, d := range svcs {
		for t, p := range d.Partitions {
			if e, ok := uniqTopics[t]; !ok || p > e {
				uniqTopics[t] = p
			}
		}
	}

	kc.Topics = uniqTopics

	out := EngineOutput{
		Ceilings: pg,
		KafkaCap: kc,
		RedisCap: rc,
		Services: svcs,
		Failures: fails,
		Warnings: warns,
	}
	return writeYAML(path, out)
}

func configMap(name string, data map[string]string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]interface{}{
			"name": name, "namespace": "rrq"},
		"data": data,
	}
}

func writeYAML(path string, v interface{}) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func envPrefix(name string) string {
	// Consistently map all services to MACRO_CASE for strict environment variable matching
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}
