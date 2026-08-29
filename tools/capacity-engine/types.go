package main

type SLOInput struct {
	Infra    Infra     `yaml:"infrastructure"`
	Defaults Defaults  `yaml:"defaults"`
	Services []Service `yaml:"services"`
}

// --- Input types ---

type Infra struct {
	PG    PGInfra    `yaml:"postgres"`
	Kafka KafkaInfra `yaml:"kafka"`
	Redis RedisInfra `yaml:"redis"`
	Pod   PodInfra   `yaml:"pod"`
}

type PGInfra struct {
	Instances  map[string]PGInstance `yaml:"instances"`
	Workload   PGWorkload            `yaml:"workload"`
	Connection PGConnection          `yaml:"connection"`
	Tuning     PGTuning              `yaml:"tuning"`
}

type PGTuning struct {
	SharedBuffersPct float64 `yaml:"shared_buffers_pct"` // 0.25 standard — PG wiki
	OSPct            float64 `yaml:"os_buffer_pct"`      // 0.25 standard — OS page cache
	MaintenancePct   float64 `yaml:"maintenance_pct"`    // 0.15 standard — VACUUM/ANALYZE
}

type PGConnection struct {
	MaxIdleMS     int `yaml:"max_idle_ms"`
	MaxLifetimeMS int `yaml:"max_lifetime_ms"`
}

type PGInstance struct {
	Cores             int   `yaml:"cores"`
	RAMBytes          int64 `yaml:"ram_bytes"`
	EffectiveSpindles int   `yaml:"effective_spindles"`
	ParallelIOLimit   int   `yaml:"parallel_io_limit"`
	WorkMemMB         int   `yaml:"work_mem_mb"`
}

type PGWorkload struct {
	SessionBusyRatio float64 `yaml:"session_busy_ratio"`
	AvgParallelism   float64 `yaml:"avg_parallelism"`
}

type KafkaInfra struct {
	Brokers               int                `yaml:"brokers"`
	PerBrokerPartitionCap int                `yaml:"per_broker_partition_cap"`
	ReplicationFactor     int                `yaml:"replication_factor"`
	RetentionDays         int                `yaml:"retention_days"`
	SegmentSeconds        int                `yaml:"segment_seconds"`
	BrokerFDULimit        int64              `yaml:"broker_fd_ulimit"`
	LatencyCritical       bool               `yaml:"latency_critical"`
	PartitionConsumeRPS   map[string]float64 `yaml:"partition_consume_rps"`
	ReaderMinBytes        int                `yaml:"reader_min_bytes"`
	ReaderMaxBytes        int                `yaml:"reader_max_bytes"`
	ReaderMaxWaitMs       int                `yaml:"reader_max_wait_ms"`
	WriterMaxAttempts     int                `yaml:"writer_max_attempts"`
}

type RedisInfra struct {
	Nodes           int     `yaml:"nodes"`
	RAMBytesPerNode int64   `yaml:"ram_bytes_per_node"`
	ForkHeadroom    float64 `yaml:"fork_headroom"`
	Fragmentation   float64 `yaml:"fragmentation_factor"`
	PerKeyBytes     int     `yaml:"per_key_bytes"`
	KetamaVnodes    int     `yaml:"ketama_vnodes"`
}

type PodInfra struct {
	CPULimitM     int   `yaml:"cpu_limit_millicores"`
	MemLimitBytes int64 `yaml:"mem_limit_bytes"`
}

type Defaults struct {
	SlackPercent             float64 `yaml:"slack_percent"`
	AZRedundancy             float64 `yaml:"az_redundancy_factor"`
	GrowthHeadroom           float64 `yaml:"growth_headroom"`
	RetryBudgetFraction      float64 `yaml:"retry_budget_fraction"`
	RetryBudgetMinTokens     int     `yaml:"retry_budget_min_tokens"`
	RetryBudgetMaxTokens     int     `yaml:"retry_budget_max_tokens"`
	WorkerAmplification      float64 `yaml:"worker_amplification"`
	HTTPHeadroom             float64 `yaml:"http_headroom"`
	PoolFloor                int     `yaml:"pool_floor"`
	MinReplicas              int     `yaml:"min_replicas"`
	MaxReplicas              int     `yaml:"max_replicas"`
	ConsumerSessionBufferMS  int     `yaml:"consumer_session_buffer_ms"`
	ConsumerCommitFlushMS    int     `yaml:"consumer_commit_flush_ms"`
	ConsumerPartitionSize    int     `yaml:"consumer_partition_size"`
	ConsumerMaxPendingMB     int     `yaml:"consumer_max_pending_mb"`
	ConsumerMinCommitCapFrac float64 `yaml:"consumer_min_commit_cap_frac"`
	DLQMaxRetries            int     `yaml:"dlq_max_retries"`
	RelayMaxFetchBatch       int     `yaml:"relay_max_fetch_batch"`
	OtelExporterEndpoint     string  `yaml:"otel_exporter_endpoint"`
}
type Service struct {
	Name                  string         `yaml:"name"`
	SLO                   ServiceSLO     `yaml:"slo"`
	Role                  string         `yaml:"role"`
	Endpoints             []Endpoint     `yaml:"endpoints"`
	Topics                []string       `yaml:"topics"`
	Redis                 *RedisSvc      `yaml:"redis"`
	HTTP                  *HTTPSvc       `yaml:"http_outbound"`
	CB                    *ServiceCB     `yaml:"circuit_breaker"`
	RPSPerCore            float64        `yaml:"rps_per_core"`
	ProducerThroughputRPS float64        `yaml:"producer_throughput_rps"`
	Relay                 *RelayConfig   `yaml:"relay"`
	JWTAccessHrs          int            `yaml:"jwt_access_hrs"`
	MaxRequestBytes       int            `yaml:"max_request_bytes"`
	VelocityThreshold     float64        `yaml:"velocity_threshold"`
	VelocityWindowMS      int            `yaml:"velocity_window_ms"`
	Webhook               *WebhookConfig `yaml:"webhook"`
	CoresPerPod           int            `yaml:"cores_per_pod"`
	HPATargetCPU          float64        `yaml:"hpa_target_cpu"`
	MemLimitBytes         int64          `yaml:"mem_limit_bytes"`
	MinReplicas           int            `yaml:"min_replicas"`
	MaxReplicas           int            `yaml:"max_replicas"`
}

type WebhookConfig struct {
	DeliveryMaxAttempts       int `yaml:"delivery_max_attempts"`
	DeliveryBackoffBaseMS     int `yaml:"delivery_backoff_base_ms"`
	DeliveryBackoffCapMS      int `yaml:"delivery_backoff_cap_ms"`
	SchedulerPollIntervalMS   int `yaml:"scheduler_poll_interval_ms"`
	SchedulerBatchSize        int `yaml:"scheduler_batch_size"`
	FastLaneGracePeriodMS     int `yaml:"fast_lane_grace_period_ms"`
	FastLaneBufferSize        int `yaml:"fast_lane_buffer_size"`
	FastLaneWorkerPoolSize    int `yaml:"fast_lane_worker_pool_size"`
	BreakerEvictionIntervalMS int `yaml:"breaker_eviction_interval_ms"`
	BreakerEvictionTTLMS      int `yaml:"breaker_eviction_ttl_ms"`
	MaxConcurrencyPerMerchant int `yaml:"max_concurrency_per_merchant"`
}

type ServiceSLO struct {
	Target            float64 `yaml:"target"`
	LatencyMS         int     `yaml:"latency_ms"`
	WindowHrs         int     `yaml:"window_hours"`
	TargetUtilization float64 `yaml:"target_utilization"`
}

type ServiceCB struct {
	ErrorThreshold float64 `yaml:"error_threshold"`
	MaxFails       int     `yaml:"max_fails"`
	MinRequests    int     `yaml:"min_requests"`
	IntervalMs     int     `yaml:"interval_ms"`
	HalfOpenProbes int     `yaml:"half_open_probes"`
}

type Endpoint struct {
	Name             string   `yaml:"name"`
	NominalQPS       float64  `yaml:"nominal_qps"`
	PeakQPS          float64  `yaml:"peak_qps"`
	AvgQueryTimeMS   float64  `yaml:"avg_query_time_ms"`
	CSquaredS        float64  `yaml:"c_s_squared"`
	CSquaredA        float64  `yaml:"c_a_squared"`
	DBInstance       string   `yaml:"db_instance"`
	DBInstances      []string `yaml:"db_instances"`
	WritesPerMessage int      `yaml:"writes_per_message"`
}

func (e *Endpoint) GetDBInstances() []string {
	if len(e.DBInstances) > 0 {
		return e.DBInstances
	}
	if e.DBInstance != "" {
		return []string{e.DBInstance}
	}
	return nil
}

type RedisSvc struct {
	Merchants     int64 `yaml:"merchants"`
	WindowBuckets int   `yaml:"window_buckets"`
}

type HTTPSvc struct {
	PeakQPSPerPod           float64 `yaml:"peak_qps_per_pod"`
	AvgLatencyS             float64 `yaml:"avg_latency_s"`
	HostCount               int     `yaml:"host_count"`
	PerHostHeadroom         int     `yaml:"per_host_headroom"`
	TimeoutMS               int     `yaml:"timeout_ms"`
	IdleConnTimeoutMS       int     `yaml:"idle_conn_timeout_ms"`
	ResponseHeaderTimeoutMS int     `yaml:"response_header_timeout_ms"`
	TLSHandshakeTimeoutMS   int     `yaml:"tls_handshake_timeout_ms"`
	ExpectContinueTimeoutMS int     `yaml:"expect_continue_timeout_ms"`
}

type RelayConfig struct {
	StagingKB               int     `yaml:"staging_kb"`
	MaxPayloadKB            int     `yaml:"max_payload_kb"`
	BufferSampleIntervalMS  int     `yaml:"buffer_sample_interval_ms"`
	BufferMaxThrottleLevel  int     `yaml:"buffer_max_throttle_level"`
	BufferMaxPollIntervalMS int     `yaml:"buffer_max_poll_interval_ms"`
	AIMDThrottleFrac        float64 `yaml:"aimd_throttle_frac"`
	AIMDPauseFrac           float64 `yaml:"aimd_pause_frac"`
	AIMDResumeFrac          float64 `yaml:"aimd_resume_frac"`
}

// --- Output types ---

type PGCeiling struct {
	Instance        string  `yaml:"instance"`
	MaxConns        int     `yaml:"max_conns"` // models.go: pgMaxConnections — derived from RAM + work_mem
	OptimalActive   int     `yaml:"optimal_active"`
	StorageGBPerDay float64 `yaml:"storage_gb_per_day"`
}

type KafkaCeiling struct {
	ClusterCap      int            `yaml:"cluster_cap"`
	PerBrokerCap    int            `yaml:"per_broker_cap"`
	FDWarning       bool           `yaml:"fd_warning"`
	LatencyWarning  bool           `yaml:"latency_warning"`
	Warnings        []string       `yaml:"warnings,omitempty"`
	StorageGBPerDay float64        `yaml:"storage_gb_per_day"`
	Topics          map[string]int `yaml:"topics,omitempty"`
}

type RedisCeiling struct {
	Node           int     `yaml:"node"`
	MaxMemoryBytes int64   `yaml:"max_memory_bytes"`
	StorageGB      float64 `yaml:"storage_gb"`
}

type Derived struct {
	Name                    string             `yaml:"name"`
	PoolSize                int                `yaml:"pool_size"`
	Workers                 int                `yaml:"workers"`
	MinReplicas             int                `yaml:"min_replicas"`
	MaxReplicas             int                `yaml:"max_replicas"`
	MaxReplicasCap          int                `yaml:"max_replicas_cap"`
	Partitions              map[string]int     `yaml:"partitions,omitempty"`
	LagThreshold            int                `yaml:"lag_threshold"`
	HTTPPool                int                `yaml:"http_pool"`
	HTTPPerHost             int                `yaml:"http_per_host"`
	HTTPPerHostCap          int                `yaml:"http_per_host_cap"`
	RelayReplicas           int                `yaml:"relay_replicas"`
	RelayFetchBatch         int                `yaml:"relay_fetch_batch"`
	RelayPoolIntervalMS     int                `yaml:"relay_pool_interval_ms"`
	RelayBatchTimeoutMS     int                `yaml:"relay_batch_timeout_ms"`
	SessionMs               int                `yaml:"session_ms"`
	HeartbeatMs             int                `yaml:"heartbeat_ms"`
	MaxRetries              int                `yaml:"max_retries"`
	BackoffBaseMS           int                `yaml:"backoff_base_ms"`
	BackoffCapMS            int                `yaml:"backoff_cap_ms"`
	CPURequest              int                `yaml:"cpu_request"`
	MemRequest              int                `yaml:"mem_request"`
	LatencyMS               map[string]float64 `yaml:"latency_ms"`
	InstDemand              map[string]int     `yaml:"inst_demand,omitempty"` // per-instance total connection demand
	WebhookMaxConcurrency   int                `yaml:"webhook_max_concurrency"`
	BreakerEvictionTTLMS    int                `yaml:"breaker_eviction_ttl_ms"`
	ProcessTimeoutMs        int                `yaml:"process_timeout_ms"`
	CircuitBreakerTimeoutMs int                `yaml:"circuit_breaker_timeout_ms"`
	ShutdownTimeoutMs       int                `yaml:"shutdown_timeout_ms"`
	DLQMaxRetries           int                `yaml:"dlq_max_retries"`
	DLQBaseDelayMs          int                `yaml:"dlq_base_delay_ms"`
	DLQCapDelayMs           int                `yaml:"dlq_cap_delay_ms"`
	PerShardRW              map[string]int     `yaml:"per_shard_rw,omitempty"`
	PerShardRO              map[string]int     `yaml:"per_shard_ro,omitempty"`
	DLQWriteTimeoutMs       int                `yaml:"dlq_write_timeout_ms"`
	FastLaneWorkerPoolSize  int                `yaml:"fast_lane_worker_pool_size"`
	RetryBudgetMinTokens    int                `yaml:"retry_budget_min_tokens"`
	RetryBudgetMaxTokens    int                `yaml:"retry_budget_max_tokens"`
}

type EngineOutput struct {
	Ceilings map[string]PGCeiling `yaml:"ceilings"`
	KafkaCap KafkaCeiling         `yaml:"kafka_cap"`
	RedisCap []RedisCeiling       `yaml:"redis_cap"`
	Services map[string]Derived   `yaml:"services"`
	Failures []string             `yaml:"failures,omitempty"`
	Warnings []string             `yaml:"warnings,omitempty"`
}
