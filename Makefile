# Read the current version from pyproject.toml (no external tooling required)
VERSION_CURRENT := $(shell grep '^version' pyproject.toml | cut -d'"' -f2)

# All docker compose commands use the compose file in deploy/
COMPOSE := docker compose -f deploy/docker-compose.yml

.DEFAULT_GOAL := help

.PHONY: help \
        env-init \
        build-go build-python \
        test-go test-python \
        docker-build up down logs \
        infra-up infra-down \
        run-server run-consumer-greptime run-consumer-doris \
        run-consumer-judge run-consumer-alerting \
        eval-dry \
        pre-commit-install pre-commit-run \
        version bump tag release

# ── help ───────────────────────────────────────────────────────────────────

help: ## Show available targets
	@echo "Cogent $(VERSION_CURRENT)"
	@echo ""
	@echo "Usage: make <target> [VERSION=x.y.z]"
	@echo ""
	@echo "First time:"
	@grep -E '^env-init:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*?## "}; {printf "  %-30s %s\n", $$1, $$2}'
	@echo ""
	@echo "Build:"
	@grep -E '^build-[a-z]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*?## "}; {printf "  %-30s %s\n", $$1, $$2}'
	@echo ""
	@echo "Test:"
	@grep -E '^test-[a-z]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*?## "}; {printf "  %-30s %s\n", $$1, $$2}'
	@echo ""
	@echo "Docker (full stack):"
	@grep -E '^(docker-build|up|down|logs):.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*?## "}; {printf "  %-30s %s\n", $$1, $$2}'
	@echo ""
	@echo "Local dev (infra in Docker, Go binaries native):"
	@grep -E '^(infra-up|infra-down|run-[a-z-]+):.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*?## "}; {printf "  %-30s %s\n", $$1, $$2}'
	@echo ""
	@echo "Eval:"
	@grep -E '^eval-[a-z]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*?## "}; {printf "  %-30s %s\n", $$1, $$2}'
	@echo ""
	@echo "Release:"
	@grep -E '^(version|bump|tag|release):.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*?## "}; {printf "  %-30s %s\n", $$1, $$2}'
	@echo ""
	@echo "Release cycle:"
	@echo "  make bump VERSION=0.2.0    # update pyproject.toml"
	@echo "  make release VERSION=0.2.0 # commit + tag + push → triggers GitHub Actions"

# ── env ────────────────────────────────────────────────────────────────────

env-init: ## Copy .env.example → .env (safe, will not overwrite existing)
	@if [ -f .env ]; then \
	  echo ".env already exists — skipping (edit it directly to change config)"; \
	else \
	  cp .env.example .env; \
	  echo "Created .env from .env.example"; \
	  echo ""; \
	  echo "Edit .env if needed:"; \
	  echo "  JUDGE_BASE_URL / JUDGE_MODEL / JUDGE_API_KEY  — LLM for auto-scoring"; \
	  echo "  ALERT_WEBHOOK_URL                              — Slack/PagerDuty webhook"; \
	fi

# ── build ──────────────────────────────────────────────────────────────────

build-go: ## Compile all six Go binaries → ./bin/
	cd services && go build -o ../bin/consumer-greptime ./cmd/consumer-greptime
	cd services && go build -o ../bin/consumer-doris    ./cmd/consumer-doris
	cd services && go build -o ../bin/consumer-judge    ./cmd/consumer-judge
	cd services && go build -o ../bin/consumer-alerting ./cmd/consumer-alerting
	cd services && go build -o ../bin/server            ./cmd/server
	cd services && go build -o ../bin/eval              ./cmd/eval

build-python: ## pip install -e .[sdk] (editable SDK install)
	pip install -e ".[sdk]"

# ── test ───────────────────────────────────────────────────────────────────

test-go: ## Run Go unit tests (cd services && go test ./...)
	cd services && go test ./...

test-python: ## Run Python tests (pytest tests/python/)
	pytest tests/python/ -v

# ── docker: full stack ─────────────────────────────────────────────────────

docker-build: ## Build all Docker images (Go services compiled inside Docker)
	$(COMPOSE) build

up: ## Start full stack — infra + all Go services in Docker
	$(COMPOSE) up -d

down: ## Stop and remove all containers
	$(COMPOSE) down

logs: ## Tail logs from all containers
	$(COMPOSE) logs -f

# ── local dev: infra in Docker, Go binaries run natively ──────────────────
#
# Workflow:
#   make infra-up                  # start Redpanda, MinIO, GreptimeDB, Doris
#   make run-server                # build + run server against local infra
#   make run-consumer-greptime     # build + run consumer against local infra
#
# Each run-* target sources .env so the binary picks up the same config as
# the Docker stack (BOOTSTRAP_SERVERS=localhost:9092, etc.).

INFRA_SERVICES := redpanda redpanda-init \
                  minio minio-init \
                  greptimedb greptimedb-init \
                  doris-fe doris-be doris-init \
                  grafana

infra-up: ## Start only infra (Redpanda, MinIO, GreptimeDB, Doris, Grafana) — no Go services
	$(COMPOSE) up -d $(INFRA_SERVICES)

infra-down: ## Stop infra containers
	$(COMPOSE) down

run-server: build-go ## Build + run API/UI server locally (port 8090)
	bash -c 'set -a; source .env; set +a; ./bin/server'

run-consumer-greptime: build-go ## Build + run consumer-greptime locally
	bash -c 'set -a; source .env; set +a; ./bin/consumer-greptime'

run-consumer-doris: build-go ## Build + run consumer-doris locally
	bash -c 'set -a; source .env; set +a; ./bin/consumer-doris'

run-consumer-judge: build-go ## Build + run consumer-judge locally
	bash -c 'set -a; source .env; set +a; ./bin/consumer-judge'

run-consumer-alerting: build-go ## Build + run consumer-alerting locally
	bash -c 'set -a; source .env; set +a; ./bin/consumer-alerting'

# ── eval ───────────────────────────────────────────────────────────────────

eval-dry: ## Dry-run batch eval for the last 7 days
	./bin/eval \
	  --start $$(date -v-7d +%Y-%m-%d 2>/dev/null || date -d '7 days ago' +%Y-%m-%d) \
	  --end $$(date +%Y-%m-%d) \
	  --dry-run

# ── pre-commit ──────────────────────────────────────────────────────────────

pre-commit-install: ## Install pre-commit hooks into .git/hooks
	uv tool install pre-commit
	pre-commit install
	@echo "pre-commit hooks installed — they will run on every git commit"

pre-commit-run: ## Run all pre-commit hooks against all files
	pre-commit run --all-files

# ── version control ────────────────────────────────────────────────────────

version: ## Print the version currently in pyproject.toml
	@echo $(VERSION_CURRENT)

bump: ## Rewrite version in pyproject.toml  (VERSION=x.y.z required)
	@if [ -z "$(VERSION)" ]; then echo "Usage: make bump VERSION=x.y.z"; exit 1; fi
	python3 -c "\
import re, pathlib; \
p = pathlib.Path('pyproject.toml'); \
p.write_text(re.sub(r'version = \"[^\"]+\"', 'version = \"$(VERSION)\"', p.read_text(), count=1))"
	@echo "pyproject.toml → version = \"$(VERSION)\""

tag: ## Create and push git tag for current pyproject.toml version
	git tag v$(VERSION_CURRENT)
	git push origin v$(VERSION_CURRENT)
	@echo "Tagged and pushed v$(VERSION_CURRENT)"

release: ## Bump, commit, tag, push — triggers GitHub Actions  (VERSION=x.y.z required)
	@if [ -z "$(VERSION)" ]; then echo "Usage: make release VERSION=x.y.z"; exit 1; fi
	$(MAKE) bump VERSION=$(VERSION)
	git add pyproject.toml
	git diff --cached --quiet || git commit -m "chore: release v$(VERSION)"
	git tag v$(VERSION)
	git push origin HEAD
	git push origin v$(VERSION)
	@echo "Released v$(VERSION) — GitHub Actions will build and publish."
