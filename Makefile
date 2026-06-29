# rrq-gitops/Makefile — GitOps Infrastructure Bootstrapping

SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

CLUSTER        ?= rrq
KIND_NODE_IMAGE ?= kindest/node:v1.31.4
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
argocd: ## Install Argo CD manually (the only non-GitOps operator)
	helm repo add argo https://argoproj.github.io/argo-helm
	helm repo update
	helm upgrade --install argocd argo/argo-cd \
	  -n argocd --create-namespace --version $(ARGOCD_VERSION) --wait

.PHONY: bootstrap-dev
bootstrap-dev: cluster-up argocd ## Bootstrap the dev cluster and apply infrastructure
	kubectl apply -f apps/dev/infrastructure.yaml
	@echo "Infrastructure bootstrap complete. Argo CD is syncing databases and operators."
	@echo "Run 'make dev' in the river-rust-queue repository to deploy the application."

.PHONY: render-dev
render-dev: ## Print fully-rendered dev manifests (no apply)
	kubectl kustomize rrq/overlays/dev

.PHONY: bootstrap-prod
bootstrap-prod: argocd ## Bootstrap a production cluster (assumes active kubectl context is your prod cluster)
	kubectl apply -f apps/prod/infrastructure.yaml
	kubectl apply -f apps/prod/rrq-app.yaml
	@echo "Production bootstrap complete. Argo CD is syncing databases, operators, and the application."

.PHONY: seal
seal: ## Seal prod secrets (requires kubeseal installed)
	kubeseal --controller-name sealed-secrets --controller-namespace kube-system --format yaml < rrq/overlays/prod/secret.plain.yaml > rrq/overlays/prod/patches/secret.sealed.yaml

