# CrowdAudit / dev workflow
# Assumes:
# - docker compose provides: postgres, redpanda (kafka api), opensearch, redis
# - Go binaries: ./cmd/inference-api and ./cmd/indexer

SHELL := /bin/bash

# Load .env file if it exists
ifneq (,$(wildcard .env))
    include .env
    export $(shell sed 's/=.*//' .env)
endif

##############################################################################

# ---- Config (override via environment or make VAR=...) ----
COMPOSE ?= docker compose

DATABASE_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/$(POSTGRES_DB)?sslmode=disable
PG_URL ?= $(DATABASE_URL)
OPENROUTER_API_KEY ?= $(OPENROUTER_API_KEY)
RDS_HOST ?= crowdaudit-dev-pg.c6l0mcewe7fk.us-east-1.rds.amazonaws.com

# Kind / K8s
# REDIS_URL ?= redis://host.docker.internal:6379
# Local Docker Compose
REDIS_URL ?= redis://localhost:6379

MIGRATIONS_DIR ?= services/inference/migrations

KAFKA_BROKERS ?= 127.0.0.1:19092
KAFKA_TOPIC ?= search-index
KAFKA_GROUP_ID ?= search-indexer

OS_URL ?= https://localhost:9200
OS_INSECURE ?= true

ENABLE_OUTBOX_PUBLISHER ?= true

##############################################################################

# ---- Helpers ----
.PHONY: help
help:
	@echo ""
	@echo "Targets:"
	@echo "  make up            Start dev infra (postgres, redpanda, opensearch)"
	@echo "  make down          Stop dev infra"
	@echo "  make ps            Show running containers"
	@echo "  make logs          Tail infra logs"
	@echo ""
	@echo "  make topic         Create kafka topic (safe to run multiple times)"
	@echo ""
	@echo "  make api           Run Go HTTP API (cmd/inference-api)"
	@echo "  make indexer       Run Go indexer (cmd/indexer)"
	@echo ""
	@echo "  make dev           Start infra + create topic (run api/indexer in separate terminals)"
	@echo ""
	@echo "  make fmt           gofmt"
	@echo "  make test          go test ./..."
	@echo ""

.PHONY: up
up:
	$(COMPOSE) up -d

.PHONY: down
down:
	$(COMPOSE) down

.PHONY: ps
ps:
	$(COMPOSE) ps

.PHONY: logs
logs:
	$(COMPOSE) logs -f --tail=200

# Create topic using rpk inside the redpanda container.
# Adjust the container name if yours differs.
.PHONY: topic
topic:
	@set -euo pipefail; \
	RP_CID="$$(docker compose ps -q redpanda)"; \
	if [ -z "$$RP_CID" ]; then \
	  echo "redpanda container not found. Is infra up? (make up)"; \
	  exit 1; \
	fi; \
	docker exec -i $$RP_CID rpk topic create "$(KAFKA_TOPIC)" -p 6 >/dev/null 2>&1 || true; \
	echo "topic ready: $(KAFKA_TOPIC)"

.PHONY: api
api:
	@set -euo pipefail; \
	export DATABASE_URL="$(DATABASE_URL)"; \
	export REDIS_URL="$(REDIS_URL)"; \
	export KAFKA_BROKERS="$(KAFKA_BROKERS)"; \
	export ENABLE_OUTBOX_PUBLISHER="$(ENABLE_OUTBOX_PUBLISHER)"; \
	export ENABLE_SEARCH="false"; \
	cd services/inference && go run ./cmd/inference-api
	
.PHONY: indexer
indexer:
	@set -euo pipefail; \
	export PG_URL="$(PG_URL)"; \
	export KAFKA_BROKERS="$(KAFKA_BROKERS)"; \
	export KAFKA_TOPIC="$(KAFKA_TOPIC)"; \
	export KAFKA_GROUP_ID="$(KAFKA_GROUP_ID)"; \
	export OS_URL="$(OS_URL)"; \
	export OS_INSECURE="$(OS_INSECURE)"; \
	cd services/inference && go run ./cmd/indexer

.PHONY: dev
dev: up topic
	@echo ""
	@echo "Infra is up and topic is ready."
	@echo "In two terminals run:"
	@echo "  make api"
	@echo "  make indexer"
	@echo ""

.PHONY: local-up
local-up: $(MAKE) -j 3 up api indexer

.PHONY: fmt
fmt:
	gofmt -w .

.PHONY: test
test:
	go test ./...

.PHONY: migrate
migrate:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

# Must enable SSL
.PHONY: migrate-rds
migrate-rds:
	migrate -path $(MIGRATIONS_DIR) -database "postgres://crowdaudit:${PGPASSWORD}@localhost:5432/crowdaudit?sslmode=require" up

.PHONY: seed
seed:
	docker exec -i crowdaudit-postgres-1 psql -U crowdaudit -d crowdaudit < services/inference/seed/dev.sql

.PHONY: import-ca
import-ca:
	docker exec -i crowdaudit-postgres-1 psql -U crowdaudit -d crowdaudit \
	  -c "\copy community_alignment_conversations (conversation_id,assigned_lang,annotator_id,first_turn_preferred_response,first_turn_prompt,first_turn_response_a,first_turn_response_b,first_turn_feedback) FROM STDIN WITH (FORMAT csv, HEADER true, NULL '\N', QUOTE '\"', ESCAPE '\"');" \
	  < community_alignment_conversations.csv

.PHONY: import-ca-rds
import-ca-rds:
	psql -h $(RDS_HOST) -U crowdaudit -d crowdaudit \
	  -c "\copy community_alignment_conversations (conversation_id,assigned_lang,annotator_id,first_turn_preferred_response,first_turn_prompt,first_turn_response_a,first_turn_response_b,first_turn_feedback) FROM STDIN WITH (FORMAT csv, HEADER true, NULL '\N', QUOTE '\"', ESCAPE '\"');" \
	  < community_alignment_conversations.csv

# psql -h localhost -p 5432 -U crowdaudit -d crowdaudit \
#   -c "\copy community_alignment_conversations (conversation_id,assigned_lang,annotator_id,first_turn_preferred_response,first_turn_prompt,first_turn_response_a,first_turn_response_b,first_turn_feedback) FROM STDIN WITH (FORMAT csv, HEADER true, NULL '\N', QUOTE '\"', ESCAPE '\"')" \
#   < community_alignment_conversations.csv

##############################################################################

# ------------------------------
# kind / kubernetes local app-layer
# ------------------------------
KIND_CLUSTER ?= crowdaudit
K8S_OVERLAY ?= deploy/k8s/overlays/kind
KIND_CONFIG ?= local/kind/kind.yaml
INGRESS_NGINX_KIND_MANIFEST ?= https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml

.PHONY: kind-create
kind-create:
	kind get clusters | grep -q "^$(KIND_CLUSTER)$$" || kind create cluster --name $(KIND_CLUSTER) --config $(KIND_CONFIG)
	kubectl config use-context kind-$(KIND_CLUSTER)

.PHONY: kind-ingress
kind-ingress:
	@set -euo pipefail; \
	kubectl apply -f $(INGRESS_NGINX_KIND_MANIFEST); \
	kubectl -n ingress-nginx rollout status deployment/ingress-nginx-controller

.PHONY: kind-build
kind-build:
	docker build -t crowdaudit/inference-api:dev -f services/inference/Dockerfile services/inference
	docker build -t crowdaudit/indexer:dev -f services/inference/Dockerfile.indexer services/inference
	docker build -t crowdaudit/web:dev -f apps/web/Dockerfile apps/web

.PHONY: kind-load
kind-load:
	kind load docker-image crowdaudit/inference-api:dev --name $(KIND_CLUSTER)
	kind load docker-image crowdaudit/indexer:dev --name $(KIND_CLUSTER)
	kind load docker-image crowdaudit/web:dev --name $(KIND_CLUSTER)

.PHONY: kind-deploy
kind-deploy:
	kubectl apply -k $(K8S_OVERLAY)
	kubectl -n crowdaudit get pods

# Convenience: one-time setup
.PHONY: kind-up
kind-up: kind-create kind-ingress
	@echo "kind cluster ready. Deploy with: make kind-redeploy"

# Convenience: rebuild + redeploy loop
.PHONY: kind-redeploy
kind-redeploy: kind-build kind-load kind-deploy

.PHONY: kind-down
kind-down:
	kind delete cluster --name $(KIND_CLUSTER)

.PHONY: kind-stop
kind-stop:
	kubectl -n crowdaudit scale deploy/inference-api --replicas=0
	kubectl -n crowdaudit scale deploy/indexer --replicas=0
	kubectl -n crowdaudit scale deploy/web --replicas=0
	kubectl -n crowdaudit get pods

.PHONY: kind-start
kind-start:
	kubectl -n crowdaudit scale deploy/inference-api --replicas=1
	kubectl -n crowdaudit scale deploy/indexer --replicas=1
	kubectl -n crowdaudit scale deploy/web --replicas=1
	kubectl -n crowdaudit get pods

.PHONY: kind-undeploy
kind-undeploy:
	kubectl delete -k $(K8S_OVERLAY)

##############################################################################

# AWS Serverless

LAMBDA_BUILD_ROOT := deploy/aws-serverless/build
GOFLAGS := GOOS=linux GOARCH=amd64 CGO_ENABLED=0

.PHONY: clean-lambda-builds
clean-lambda-builds:
	rm -rf $(LAMBDA_BUILD_ROOT)

define build_lambda_zip
	@set -euo pipefail; \
	name="$(1)"; \
	pkg="$(2)"; \
	outdir="$(LAMBDA_BUILD_ROOT)/$$name"; \
	mkdir -p "$$outdir"; \
	rm -f "$$outdir/bootstrap" "$(LAMBDA_BUILD_ROOT)/$$name.zip"; \
	( cd services/inference && $(GOFLAGS) go build -o "../../$$outdir/bootstrap" "$$pkg" ); \
	( cd "$$outdir" && zip -q -j "../$$name.zip" bootstrap ); \
	unzip -l "$(LAMBDA_BUILD_ROOT)/$$name.zip" | sed -n '1,20p'
endef

.PHONY: build-inference-lambda
build-inference-lambda:
	$(call build_lambda_zip,inference-api,./cmd/inference-lambda)

.PHONY: build-db-api-lambda
build-db-api-lambda:
	$(call build_lambda_zip,db-api,./cmd/db-api-lambda)

.PHONY: build-indexer-lambda
build-indexer-lambda:
	$(call build_lambda_zip,indexer,./cmd/indexer-lambda)

.PHONY: build-publisher-lambda
build-publisher-lambda:
	$(call build_lambda_zip,publisher,./cmd/publisher-lambda)

.PHONY: build-lambdas
build-lambdas: build-inference-lambda build-db-api-lambda build-indexer-lambda build-publisher-lambda

# LAMBDA_BUILD_DIR := ../../deploy/aws-serverless/build

# .PHONY: build-inference-lambda
# build-inference-lambda:
# 	mkdir -p deploy/aws-serverless/build
# 	cd services/inference && \
# 		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
# 		go build -o $(LAMBDA_BUILD_DIR)/bootstrap ./cmd/inference-lambda
# 	cd deploy/aws-serverless/build && zip -j inference-api.zip bootstrap

# .PHONY: build-db-api-lambda
# build-db-api-lambda  :
# 	mkdir -p deploy/aws-serverless/build
# 	cd services/inference && \
# 		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
# 		go build -o $(LAMBDA_BUILD_DIR)/bootstrap ./cmd/db-api-lambda
# 	cd deploy/aws-serverless/build && zip -j db-api.zip bootstrap

# .PHONY: build-indexer-lambda
# build-indexer-lambda:
# 	mkdir -p deploy/aws-serverless/build
# 	cd services/inference && \
# 		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
# 		go build -o ../../deploy/aws-serverless/build/bootstrap ./cmd/indexer-lambda
# 	cd deploy/aws-serverless/build && zip -j indexer.zip bootstrap

# .PHONY: build-publisher-lambda
# build-publisher-lambda:
# 	mkdir -p deploy/aws-serverless/build
# 	cd services/inference && \
# 		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
# 		go build -o ../../deploy/aws-serverless/build/bootstrap ./cmd/publisher-lambda
# 	cd deploy/aws-serverless/build && zip -j publisher.zip bootstrap

.PHONY: tf-apply-serverless-dev
tf-apply-serverless-dev: build-lambdas
# 	export VERCEL_API_TOKEN=***REMOVED***;
# 	export OPENROUTER_API_KEY=$$OPENROUTER_API_KEY;
	cd platform/terraform/envs/dev/serverless && \
		terraform init && \
		terraform apply \
			-var="aws_region=us-east-1" \
			-var="env=dev" \
			-var="api_zip_path=../../../../../deploy/aws-serverless/build/inference-api.zip" \
  			-var="db_zip_path=../../../../../deploy/aws-serverless/build/db-api.zip" \
			-var="indexer_zip_path=../../../../../deploy/aws-serverless/build/indexer.zip" \
			-var="publisher_zip_path=../../../../../deploy/aws-serverless/build/publisher.zip" \
			-var="openrouter_api_key=$$OPENROUTER_API_KEY" \
			-var="os_url=$$OS_URL" \
			-var="os_username=$$OS_USERNAME" \
			-var="os_password=$$OS_PASSWORD" \
			-var="crowdaudit_zone_id=Z05639302JBOKYSU3P8LI" \
			-var-file=serverless.tfvars

.PHONY: tf-temp-import
tf-temp-import:
	cd platform/terraform/envs/dev/serverless && \
		terraform import \
			-var="aws_region=us-east-1" \
			-var="env=dev" \
			-var="api_zip_path=../../../../../deploy/aws-serverless/build/inference-api.zip" \
  			-var="db_zip_path=../../../../../deploy/aws-serverless/build/db-api.zip" \
			-var="indexer_zip_path=../../../../../deploy/aws-serverless/build/indexer.zip" \
			-var="publisher_zip_path=../../../../../deploy/aws-serverless/build/publisher.zip" \
			-var="openrouter_api_key=$$OPENROUTER_API_KEY" \
			-var="os_url=$$OS_URL" \
			-var="os_username=$$OS_USERNAME" \
			-var="os_password=$$OS_PASSWORD" \
			-var="crowdaudit_zone_id=$(CROWDAUDIT_ZONE_ID)" \
			-var-file=serverless.tfvars \
			aws_internet_gateway.igw $(AWS_IGW_ID)