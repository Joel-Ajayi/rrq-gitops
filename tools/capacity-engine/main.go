package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: capacity <slo-input.yaml>\n")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fatalf("read input: %v", err)
	}
	var input SLOInput
	if err := yaml.Unmarshal(data, &input); err != nil {
		fatalf("parse input: %v", err)
	}

	pg, kc, rc := supply(&input)
	svcs := derive(&input)
	fails, warns := fitcheck(svcs, pg, kc, rc, &input)

	printResults(pg, kc, rc, svcs, fails, warns)

	if err := render(svcs, pg, kc, rc, fails, warns, &input); err != nil {
		fatalf("render: %v", err)
	}

	// Patch K8s manifests from engine output (KEDA / Kafka / config).
	// Addresses issues 14, 15, 16, 30.
	if err := renderManifests(svcs, pg, &input, ".."); err != nil {
		fatalf("render-manifests: %v", err)
	}

	if len(fails) > 0 {
		fmt.Println("\nCAPACITY CHECK FAILED")
		os.Exit(1)
	}
	fmt.Println("\nCAPACITY CHECK PASSED")
}

func printResults(pg map[string]PGCeiling, kc KafkaCeiling, rc []RedisCeiling, svcs map[string]Derived, fails []string, warns []string) {
	fmt.Println("=== SUPPLY ===")
	for _, c := range pg {
		fmt.Printf("  PG %s: max_conns=%d optimal=%d storage=%.2f GB/day\n", c.Instance, c.MaxConns, c.OptimalActive, c.StorageGBPerDay)
	}
	fmt.Printf("  Kafka: cluster=%d per_broker=%d storage=%.2f GB/day\n", kc.ClusterCap, kc.PerBrokerCap, kc.StorageGBPerDay)
	if len(rc) > 0 {
		fmt.Printf("  Redis: maxmem=%dMiB storage=%.2f GB\n", rc[0].MaxMemoryBytes/1024/1024, rc[0].StorageGB)
	}

	fmt.Println("\n=== DERIVED (per service) ===")
	for name, d := range svcs {
		fmt.Printf("  %s: pool=%d workers=%d replicas=%d/%d partitions=%v\n",
			name, d.PoolSize, d.Workers, d.MinReplicas, d.MaxReplicas, d.Partitions)
		for ep, l := range d.LatencyMS {
			fmt.Printf("    latency[%s]=%.0fms\n", ep, l)
		}
		if d.HTTPPool > 0 {
			fmt.Printf("    http: pool=%d perHost=%d cap=%d\n", d.HTTPPool, d.HTTPPerHost, d.HTTPPerHostCap)
		}
		if d.RelayReplicas > 1 {
			fmt.Printf("    relay_replicas=%d\n", d.RelayReplicas)
		}
		fmt.Printf("    n_cpu=%dm mem=%dMi\n", d.CPURequest, d.MemRequest)
		fmt.Printf("    retries=%d backoff=%d/%d session=%d heartbeat=%d\n",
			d.MaxRetries, d.BackoffBaseMS, d.BackoffCapMS, d.SessionMs, d.HeartbeatMs)
	}

	fmt.Println("\n=== FIT-CHECK ===")
	for _, f := range fails {
		fmt.Printf("  FAIL: %s\n", f)
	}
	for _, w := range warns {
		fmt.Printf("  WARN: %s\n", w)
	}
	for _, d := range svcs {
		if d.MaxReplicasCap < d.MaxReplicas {
			fmt.Printf("  HPA: %s maxReplicas capped at %d\n", nameFromMap(svcs, d), d.MaxReplicasCap)
		}
	}
	fmt.Println("ConfigMaps written to ../rrq/base/config/")
}

func nameFromMap(m map[string]Derived, d Derived) string {
	for k, v := range m {
		if v.Name == d.Name {
			return k
		}
	}
	return d.Name
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
