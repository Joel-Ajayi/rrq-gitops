# rrq-gitops/Makefile — GitOps Infrastructure Bootstrapping

SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

CLUSTER        ?= rrq
KIND_NODE_IMAGE ?= kindest/node:v1.36.1
ARGOCD_VERSION  ?= 7.7.5

.PHONY: help
help: ## List GitOps targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | sort | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: cluster-up
cluster-up: ## Create the kind cluster
	@kind get clusters 2>/dev/null | grep -qx "$(CLUSTER)" \
	  && echo "kind cluster '$(CLUSTER)' already exists" \
	  || kind create cluster --name $(CLUSTER) --image $(KIND_NODE_IMAGE) --config rrq/kind/cluster-dev.yaml

.PHONY: cluster-down
cluster-down: ## Delete the kind cluster
	-kind delete cluster --name $(CLUSTER)

.PHONY: argocd
argocd: ## Install Argo CD manually
	helm repo add argo https://argoproj.github.io/argo-helm
	helm repo update
	helm upgrade --install argocd argo/argo-cd \
	  -n argocd --create-namespace --wait \
	  --set configs.cm.timeout.reconciliation=600


# --- DEVELOPMENT ---
.PHONY: dev
dev: ## Bootstrap dev infrastructure operators & sealed-secrets
	@echo "1/4 Provisioning dev sealed secrets..."
	$(MAKE) seal-dev
	@echo "2/4 Deploying infrastructure backends to Argo CD..."
	kubectl apply -f apps/dev/infrastructure.yaml
	@echo "3/4 Waiting for sealed-secrets controller to be ready..."
	-kubectl rollout status deployment/sealed-secrets -n kube-system --timeout=120s
	@echo "4/4 Waiting for infrastructure backends to be ready..."
	-$(MAKE) wait-infra
	@echo "Dev infrastructure ready! Skaffold will now deploy your application code."


.PHONY: seal-dev
seal-dev: ## Seal dev secrets (requires kubeseal installed)
	kubeseal --controller-name sealed-secrets --controller-namespace kube-system --format yaml < rrq/secrets/dev/secret.plain.yaml > rrq/secrets/dev/secret.sealed.yaml
	kubectl apply -k rrq/secrets/dev

.PHONY: wait-infra
wait-infra: ## Wait for Postgres CNPG clusters, Redis, and infrastructure readiness
	@echo "Checking Postgres CNPG cluster health..."
	-kubectl wait --for=jsonpath='{.status.phase}'=ClusterInHealthyState cluster/merchants-db -n rrq --timeout=5s 2>/dev/null || true
	-kubectl wait --for=jsonpath='{.status.phase}'=ClusterInHealthyState cluster/shard-a -n rrq --timeout=5s 2>/dev/null || true
	-kubectl wait --for=jsonpath='{.status.phase}'=ClusterInHealthyState cluster/shard-b -n rrq --timeout=5s 2>/dev/null || true
	@echo "Checking Redis readiness..."
	-kubectl rollout status statefulset/redis-node -n rrq --timeout=60s 2>/dev/null || true

.PHONY: render-dev
render-dev: ## Print fully-rendered dev manifests (no apply)
	kubectl kustomize rrq/overlays/dev

# --- PRODUCTION ---
.PHONY: deploy
deploy: seal ## Bootstrap production via root App-of-Apps
	kubectl apply -f bootstrap/root-app-prod.yaml
	@echo "Production bootstrap complete. Argo CD is syncing databases, operators, and the application."

.PHONY: seal
seal: ## Seal prod secrets (requires kubeseal installed)
	kubeseal --controller-name sealed-secrets --controller-namespace kube-system --format yaml < rrq/secrets/prod/secret.plain.yaml > rrq/secrets/prod/secret.sealed.yaml
	kubectl apply -k rrq/secrets/prod