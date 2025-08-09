# Makefile for ArenaService (k3s optimized)

# Variables
KUSTOMIZE_AIBROKER_DIR ?= ./cluster/ai-broker

.PHONY: build build-aibroker test k3s-setup k3s-stop k3s-clean setup k3s-deploy-aibroker k3s-restart-aibroker k3s-status-aibroker k3s-logs-aibroker k3s-delete-aibroker sync-node-ips

# Build the Go application
build:
	go build -o bin/arenaservice ./cmd/arenaservice
	go build -o bin/arenaclient ./cmd/arenaclient

# Build aibroker (host arch)
build-aibroker:
	go build -o bin/aibroker ./cmd/aibroker

# Build aibroker static for k3s node arch (default arm64)
# Auto-detect GOARCH from host (maps x86_64→amd64, aarch64→arm64). Override if your k3s node differs.
AIBROKER_GOARCH ?= $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
build-aibroker-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(AIBROKER_GOARCH) go build -a -installsuffix cgo -o bin/aibroker-static ./cmd/aibroker

# Build static binary for k3s deployment
build-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -installsuffix cgo -o bin/arenaservice-static ./cmd/arenaservice

# Run tests
test:
	go test -v ./...



# Server setup (replaces old hetzner/arm setup scripts)
setup:
	chmod +x scripts/setup.sh
	./scripts/setup.sh

# Stop k3s
k3s-stop:
	sudo systemctl stop k3s

# Clean up k3s
k3s-clean:
	sudo /usr/local/bin/k3s-uninstall.sh

# Show k3s status
k3s-status:
	kubectl get nodes
	kubectl get pods --all-namespaces

# Show ArenaService logs in k3s
k3s-logs:
	kubectl logs -n control-plane deployment/arenaservice -f

# Deploy ArenaService to k3s
k3s-deploy: build-static
	chmod +x scripts/deploy.sh
	./scripts/deploy.sh
	kubectl rollout restart deployment/arenaservice -n control-plane
	@echo "🔄 Restarting deployment to pick up new binary..."
	kubectl wait --for=condition=available --timeout=120s deployment/arenaservice -n control-plane
	@echo "✅ ArenaService deployed and restarted successfully!"

# Restart ArenaService in k3s
k3s-restart: build-static
	kubectl rollout restart deployment/arenaservice -n control-plane
	@echo "ArenaService restarted in k3s cluster"


# AI Broker k3s targets
k3s-deploy-aibroker: build-aibroker-static
	@# Ensure namespace exists
	@if ! kubectl get namespace ai-broker >/dev/null 2>&1; then \
	  echo "Creating namespace ai-broker"; \
	  kubectl create namespace ai-broker; \
	fi
	@# Create minimal secret if missing (token from deepseek.key if exists, else placeholder)
	@if ! kubectl get secret aibroker-env -n ai-broker >/dev/null 2>&1; then \
	  if [ -f deepseek.key ]; then \
	    TOKEN=$$(tr -d '\n' < deepseek.key); \
	    echo "Creating aibroker-env secret with token from deepseek.key"; \
	    kubectl -n ai-broker create secret generic aibroker-env --from-literal=AIBROKER_TOKEN="$$TOKEN"; \
	  else \
	    echo "Creating aibroker-env secret with placeholder token (set AIBROKER_TOKEN)"; \
	    kubectl -n ai-broker create secret generic aibroker-env --from-literal=AIBROKER_TOKEN="REPLACE_ME"; \
	  fi; \
	fi
	kubectl apply -k $(KUSTOMIZE_AIBROKER_DIR)

k3s-restart-aibroker:
	kubectl -n ai-broker rollout restart deployment/aibroker

k3s-status-aibroker:
	kubectl -n ai-broker get all

k3s-logs-aibroker:
	kubectl -n ai-broker logs deployment/aibroker -f

k3s-delete-aibroker:
	-kubectl delete -k $(KUSTOMIZE_AIBROKER_DIR)


# Clean up
clean:
	rm -rf bin/

# Clean up k3s deployment
k3s-clean-deployment:
	kubectl delete -k cluster/control-plane/ 2>/dev/null || true
	@echo "ArenaService deployment cleaned up from k3s cluster"

# Generate WireGuard keys
wg-genkeys:
	@echo "Generating WireGuard key pair..."
	@echo "Private key:"
	@wg genkey
	@echo "Public key:"
	@wg genkey | wg pubkey

# Help
help:
	@echo "Available commands:"
	@echo "  build              - Build Go application"
	@echo "  build-aibroker     - Build AI broker binary"
	@echo "  build-static       - Build static binary for k3s deployment"
	@echo "  test               - Run tests"
	@echo "  setup              - Set up server (firewall, k3s, etc.)"
	@echo "  k3s-setup          - Set up k3s cluster"
	@echo "  k3s-deploy         - Deploy ArenaService to k3s"
	@echo "  k3s-restart        - Restart ArenaService in k3s"
	@echo "  k3s-stop           - Stop k3s cluster"
	@echo "  k3s-clean          - Uninstall k3s"
	@echo "  k3s-clean-deployment - Clean up ArenaService deployment"
	@echo "  k3s-status         - Show k3s cluster status"
	@echo "  k3s-logs           - Show ArenaService logs in k3s"
	@echo "  k3s-deploy-aibroker - Deploy AI broker to k3s"
	@echo "  k3s-restart-aibroker - Restart AI broker in k3s"
	@echo "  k3s-status-aibroker - Show AI broker status in k3s"
	@echo "  k3s-logs-aibroker  - Show AI broker logs in k3s"
	@echo "  k3s-delete-aibroker - Delete AI broker from k3s"
	@echo "  k3s-set-deepseek-token - Create/patch DeepSeek token in ai-broker secret"
	@echo "  wg-genkeys         - Generate WireGuard key pair"
	@echo "  clean              - Clean up build artifacts"
	@echo "  help               - Show this help" 