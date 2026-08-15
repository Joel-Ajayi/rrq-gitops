# Makefile — Global-First Capacity Derivation Engine
#
# Usage:
#   make run     — derive capacity-output.yaml from slo-input.yaml and patch manifests
#   make build   — compile the capacity binary
#   make verify  — run engine, exit non-zero on capacity FAIL
#   make fmt     — gofmt all sources
#   make clean   — remove binary and generated output

GO      ?= go
BINARY  := capacity
INPUT   := slo-input.yaml
OUTPUT  := capacity-output.yaml
RENDER  := ..

.PHONY: run build verify fmt clean

run:
	$(GO) run . -input $(INPUT) -output $(OUTPUT) -render $(RENDER)

build:
	$(GO) build -o $(BINARY) .

verify: run
	@echo "Engine run complete; check exit code"

fmt:
	gofmt -w *.go

.PHONY: clean
clean: ## Remove generated files
	rm -f $(BINARY)
	rm -f $(OUTPUT)