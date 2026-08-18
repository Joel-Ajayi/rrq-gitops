# rrq-gitops/Makefile — GitOps Infrastructure Bootstrapping

SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

CLUSTER         ?= rrq
KIND_NODE_IMAGE ?= kindest/node:v1.36.1
ENV             ?= dev

# Pinned tool versions
KIND_VERSION               ?= v0.31.0
KUBECTL_VERSION            ?= v1.31.4
HELM_VERSION               ?= v3.17.0
KUSTOMIZE_VERSION          ?= v5.6.0
KUBESEAL_VERSION           ?= 0.27.3
ARGOCD_VERSION             ?= v2.14.0
SKAFFOLD_VERSION           ?= v2.23.0
K6_VERSION                 ?= v0.55.0
JQ_VERSION                 ?= latest
YQ_VERSION                 ?= v4.44.3

BIN            ?= $(HOME)/.local/bin
OS             := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH           := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

.PHONY: help
help: ## List GitOps targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
		| sort | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# 1. TOOLS INSTALLATION
.PHONY: tools
tools: $(BIN) tools-kubectl tools-helm tools-kind tools-kubeseal tools-argocd tools-skaffold tools-k6 tools-jq tools-yq ## Install every GitOps CLI
	@echo "All GitOps tools installed."

$(BIN):
	@mkdir -p $(BIN)

.PHONY: tools-kubectl
tools-kubectl: $(BIN) ## Install kubectl
	@command -v kubectl >/dev/null && echo "kubectl present" || { \
	  curl -fsSLo $(BIN)/kubectl "https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$(OS)/$(ARCH)/kubectl" && \
	  chmod +x $(BIN)/kubectl ; }

.PHONY: tools-helm
tools-helm: $(BIN) ## Install helm
	@command -v helm >/dev/null && echo "helm present" || \
	  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 \
	    | USE_SUDO=false HELM_INSTALL_DIR=$(BIN) DESIRED_VERSION=$(HELM_VERSION) bash

.PHONY: tools-kind
tools-kind: ## Install kind
	@command -v kind >/dev/null && echo "kind present" || go install sigs.k8s.io/kind@$(KIND_VERSION)

.PHONY: tools-kubeseal
tools-kubeseal: $(BIN) ## Install kubeseal
	@command -v kubeseal >/dev/null && echo "kubeseal present" || { \
	  curl -fsSL "https://github.com/bitnami-labs/sealed-secrets/releases/download/v$(KUBESEAL_VERSION)/kubeseal-$(KUBESEAL_VERSION)-$(OS)-$(ARCH).tar.gz" \
	    | tar -xz -C $(BIN) kubeseal && chmod +x $(BIN)/kubeseal ; }

.PHONY: tools-argocd
tools-argocd: $(BIN) ## Install argocd CLI
	@command -v argocd >/dev/null && echo "argocd present" || { \
	  curl -fsSLo $(BIN)/argocd "https://github.com/argoproj/argo-cd/releases/download/$(ARGOCD_VERSION)/argocd-$(OS)-$(ARCH)" && \
	  chmod +x $(BIN)/argocd ; }

.PHONY: tools-skaffold
tools-skaffold: $(BIN) ## Install skaffold
	@command -v skaffold >/dev/null && echo "skaffold present" || { \
	  curl -fsSLo $(BIN)/skaffold "https://storage.googleapis.com/skaffold/releases/$(SKAFFOLD_VERSION)/skaffold-$(OS)-$(ARCH)" && \
	  chmod +x $(BIN)/skaffold ; }

.PHONY: tools-k6
tools-k6: $(BIN) ## Install k6
	@command -v k6 >/dev/null && echo "k6 present" || { \
	  curl -fsSL "https://github.com/grafana/k6/releases/download/$(K6_VERSION)/k6-$(K6_VERSION)-$(OS)-$(ARCH).tar.gz" \
	    | tar -xz --strip-components=1 -C $(BIN) "k6-$(K6_VERSION)-$(OS)-$(ARCH)/k6" ; }

.PHONY: tools-jq
tools-jq: $(BIN) ## Install jq
	@command -v jq >/dev/null && echo "jq present" || { \
	  curl -fsSLo $(BIN)/jq "https://github.com/jqlang/jq/releases/latest/download/jq-$(OS)-$(ARCH)" && \
	  chmod +x $(BIN)/jq ; }

.PHONY: tools-yq
tools-yq: $(BIN) ## Install yq (YAML CLI)
	@command -v yq >/dev/null && echo "yq present" || { \
	  curl -fsSLo $(BIN)/yq "https://github.com/mikefarah/yq/releases/download/$(YQ_VERSION)/yq_$(OS)_$(ARCH)" && \
	  chmod +x $(BIN)/yq ; }

# 2. CLUSTER LIFECYCLE
.PHONY: cluster-up
cluster-up: ## Create the local Kind cluster
	@kind get clusters 2>/dev/null | grep -qx "$(CLUSTER)" \
		&& echo "kind cluster '$(CLUSTER)' already exists" \
		|| kind create cluster --name $(CLUSTER) --image $(KIND_NODE_IMAGE) --config kind/cluster-dev.yaml

.PHONY: cluster-down
cluster-down: ## Delete the local Kind cluster
	-kind delete cluster --name $(CLUSTER)

# 3. SECRETS MANAGEMENT
.PHONY: seal
seal: ## Encrypt plain secrets into the overlay structure (uses ENV variable)
	@echo "Sealing $(ENV) secrets..."
	@for comp in datastores observability workloads operators; do \
		if [ -f secrets/$(ENV)/$$comp.plain.yaml ]; then \
			kubeseal --controller-name sealed-secrets --controller-namespace kube-system --format yaml < secrets/$(ENV)/$$comp.plain.yaml > overlays/$(ENV)/$$comp/secrets.yaml; \
			echo " -> Sealed $$comp secrets for $(ENV)"; \
		fi \
	done

# 4. GITOPS BOOTSTRAPPING (ARGO CD)
.PHONY: argocd
argocd: ## Install Argo CD manually
	helm repo add argo https://argoproj.github.io/argo-helm
	helm repo update
	helm upgrade --install argocd argo/argo-cd \
		-n argocd --create-namespace --wait \
		--set configs.cm.timeout.reconciliation=600

.PHONY: bootstrap
bootstrap: ## Bootstrap the cluster (dispatches to bootstrap-dev or bootstrap-prod based on ENV)
	@$(MAKE) bootstrap-$(ENV)

.PHONY: bootstrap-dev
bootstrap-dev: ## Local dev bootstrap: Manual kubectl apply of overlays to bypass GitOps
	@echo "Local Bootstrapping (bypassing Argo CD)..."
	@echo "Deploying sealed-secrets operator (Wave -3)..."
	kubectl kustomize --enable-helm overlays/dev/sealed-secrets | kubectl apply -f -
	@echo "Waiting for sealed-secrets operator..."
	kubectl rollout status deployment/sealed-secrets -n kube-system --timeout=180s
	@echo "Sealing local dev secrets..."
	$(MAKE) seal ENV=dev
	@echo "Deploying operators (Wave -2)..."
	kubectl kustomize --enable-helm overlays/dev/operators | kubectl apply -f -
	@echo "Deploying gateway (Wave -1)..."
	kubectl kustomize --enable-helm overlays/dev/gateway | kubectl apply -f -
	@echo "Deploying datastores (Wave 0)..."
	kubectl kustomize --enable-helm overlays/dev/datastores | kubectl apply -f -
	@echo "Deploying observability (Wave 1)..."
	kubectl kustomize --enable-helm overlays/dev/observability | kubectl apply -f -
	@echo "Deploying workloads (Wave 2)..."
	kubectl kustomize --enable-helm overlays/dev/workloads | kubectl apply -f -
	@echo "Local dev bootstrap complete."

.PHONY: bootstrap-prod
bootstrap-prod: argocd ## Prod bootstrap: Argo CD App-of-Apps GitOps
	@echo "Deploying Root Application to Argo CD..."
	kubectl apply -f bootstrap/root-app.yaml
	@echo "Waiting for Argo CD to deploy sealed-secrets operator (Wave -3)..."
	kubectl rollout status deployment/sealed-secrets -n kube-system --timeout=120s
	@echo "Sealing initial prod secrets..."
	$(MAKE) seal ENV=prod
	@echo "Production GitOps bootstrap complete."


# 5. BENCHMARKING (k6)
.PHONY: bench
bench: ## Run k6 scenario (SCENARIO=performance/load-sustained)
	./tools/load-tests/run.sh $(SCENARIO)

.PHONY: bench-seed
bench-seed: ## Seed test data (create merchants, wallets, pre-fund)
	./tools/load-tests/run.sh seed

.PHONY: bench-smoke
bench-smoke: ## Quick smoke test (1min, low VUs, for CI)
	DURATION=1m ./tools/load-tests/run.sh performance/load-sustained --no-verify

.PHONY: bench-verify
bench-verify: ## Run k6 scenario with post-test observability verification (default)
	./tools/load-tests/run.sh $(SCENARIO) --verify

.PHONY: bench-all
bench-all: ## Run all k6 scenarios sequentially with verification (nightly)
	@for s in performance/load-sustained performance/stress-bulk-payout performance/ramp-to-peak performance/spike-surge reliability/fraud-throughput reliability/circuit-breaker reliability/reconciliation-integrity scalability/cross-shard-throughput compatibility/contract-compliance security/edge-protection; do \
	  echo "=== Running $$s ==="; \
	  $(MAKE) bench-verify SCENARIO=$$s; \
	done

.PHONY: bench-soak
bench-soak: ## Run 4-hour soak test with verification (pre-release)
	$(MAKE) bench-verify SCENARIO=performance/soak-endurance

.PHONY: bench-full
bench-full: bench-all bench-soak ## Full performance qualification suite (nightly + pre-release)

# 6. UTILITIES
.PHONY: capacity
capacity: ## Regenerate GitOps manifests from capacity models
	cd tools/capacity-engine && go run . -input slo-input.yaml -output capacity-output.yaml -render ../..

.PHONY: render
render: ## Print fully-rendered Kustomize manifests (dry-run, uses ENV variable)
	kubectl kustomize overlays/$(ENV) --enable-helm