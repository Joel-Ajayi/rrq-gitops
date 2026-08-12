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
	WorkerAmplification      float64 `yaml:"worker_amplification"`
	HTTPHeadroom             float64 `yaml:"http_headroom"`
	PoolFloor                int     `yaml:"pool_floor"`
	ConsumerSessionBufferMS  int     `yaml:"consumer_session_buffer_ms"`
	ConsumerCommitFlushMS    int     `yaml:"consumer_commit_flush_ms"`
	ConsumerPartitionSize    int     `yaml:"consumer_partition_size"`
	ConsumerMaxPendingMB     int     `yaml:"consumer_max_pending_mb"`
	ConsumerMinCommitCapFrac float64 `yaml:"consumer_min_commit_cap_frac"`
	DLQMaxRetries            int     `yaml:"dlq_max_retries"`
	RelayMaxFetchBatch       int     `yaml:"relay_max_fetch_batch"`
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
	Argon2MemoryKib       int            `yaml:"argon2_memory_kib"`
	Argon2Iterations      int            `yaml:"argon2_iterations"`
	Argon2Parallelism     int            `yaml:"argon2_parallelism"`
	MaxRequestBytes       int            `yaml:"max_request_bytes"`
	VelocityThreshold     float64        `yaml:"velocity_threshold"`
	VelocityWindowMS      int            `yaml:"velocity_window_ms"`
	ConsumerPollTimeoutMS int            `yaml:"consumer_poll_timeout_ms"`
	Webhook               *WebhookConfig `yaml:"webhook"`
	CoresPerPod           int            `yaml:"cores_per_pod"`
	HPATargetCPU          float64        `yaml:"hpa_target_cpu"`
	MemLimitBytes         int64          `yaml:"mem_limit_bytes"`
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
	Name             string  `yaml:"name"`
	NominalQPS       float64 `yaml:"nominal_qps"`
	PeakQPS          float64 `yaml:"peak_qps"`
	AvgQueryTimeMS   float64 `yaml:"avg_query_time_ms"`
	CSquaredS        float64 `yaml:"c_s_squared"`
	CSquaredA        float64 `yaml:"c_a_squared"`
	DBInstance       string  `yaml:"db_instance"`
	WritesPerMessage int     `yaml:"writes_per_message"`
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
	Instance      string
	MaxConns      int // models.go: pgMaxConnections — derived from RAM + work_mem
	OptimalActive int
}

type KafkaCeiling struct {
	ClusterCap     int
	PerBrokerCap   int
	FDWarning      bool
	LatencyWarning bool
	Warnings       []string
}

type RedisCeiling struct {
	Node           int
	MaxMemoryBytes int64
}

type Derived struct {
	Name                    string
	PoolSize                int
	Workers                 int
	MinReplicas             int
	MaxReplicas             int
	MaxReplicasCap          int
	Partitions              map[string]int
	LagThreshold            int
	HTTPPool                int
	HTTPPerHost             int
	HTTPPerHostCap          int
	RelayReplicas           int
	RelayFetchBatch         int
	RelayPoolIntervalMS     int
	RelayBatchTimeoutMS     int
	SessionMs               int
	HeartbeatMs             int
	MaxRetries              int
	BackoffBaseMS           int
	BackoffCapMS            int
	CPURequest              int
	MemRequest              int
	LatencyMS               map[string]float64
	InstDemand              map[string]int // per-instance total connection demand
	WebhookMaxConcurrency   int
	BreakerEvictionTTLMS    int
	ProcessTimeoutMs        int
	CircuitBreakerTimeoutMs int
	ShutdownTimeoutMs       int
	DLQMaxRetries           int
	DLQBaseDelayMs          int
	DLQCapDelayMs           int
	PerShardRW              map[string]int // per-pod, per-shard RW cap (keyed by shard ID)
	DLQWriteTimeoutMs       int            // outer DLQ write deadline (== total DLQ retry time, ≤ ProcessTimeoutMs)
	FastLaneWorkerPoolSize  int            // engine-derived fast lane HTTP delivery pool size (webhook workers)
}

type EngineOutput struct {
	Ceilings []PGCeiling
	KafkaCap KafkaCeiling
	RedisCap []RedisCeiling
	Services map[string]Derived
	Failures []string
	Warnings []string
}
