package main

// Physical constants — not tunable, not defaultable.
// These are facts about the infrastructure, not configuration choices.

// Kafka — source: Confluent 2023, KIP-578, Jun Rao 2015
const (
	KafkaClusterMaxParts  = 200_000 // hard partition ceiling per cluster
	KafkaLatencyAdvisoryR = 100     // r * brokers * replication_factor: latency advisory limit
)

// Pod — source: go pgx, net/http Transport memory profiles
const (
	PodPGConnMemBytes      = 50_000           // ~50 KB per PG conn (pgx client-side overhead)
	PodHTTPTLSMemBytes     = 50_000           // ~50 KB per TLS conn (net/http Transport)
	PodAPPBaselineMemBytes = 64 * 1024 * 1024 // 64 MiB Go runtime + GC + libraries
)
