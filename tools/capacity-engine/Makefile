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
	@echo "Engine run complete; check exit code and ../../base/config/ for output"

fmt:
	gofmt -w *.go

.PHONY: clean
clean: ## Remove generated files
	rm -f $(BINARY)
	rm -f ../../base/config/capacity-output.yaml
	rm -f ../../base/config/rrq-platform-config.yaml
	rm -f ../../base/config/*-configmap.yaml