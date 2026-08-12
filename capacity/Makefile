# Makefile — Global-First Capacity Derivation Engine
#
# Usage:
#   make run     — derive configmaps + capacity-output.yaml from slo-input.yaml
#   make build   — compile the capacity binary
#   make verify  — run engine, exit non-zero on capacity FAIL
#   make fmt     — gofmt all sources
#   make clean   — remove binary and generated configs

GO      ?= go
BINARY  := capacity
INPUT   := slo-input.yaml

.PHONY: run build verify fmt clean

run:
	$(GO) run . $(INPUT)

build:
	$(GO) build -o $(BINARY) .

verify: run
	@echo "Engine run complete; check exit code and ../rrq/base/config/ for output"

fmt:
	gofmt -w *.go

clean:
	rm -f $(BINARY)
	rm -f ../rrq/base/config/capacity-output.yaml
	rm -f ../rrq/base/config/rrq-platform-config.yaml
	rm -f ../rrq/base/config/*-configmap.yaml