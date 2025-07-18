# Makefile for ArenaService (k3s optimized)

.PHONY: build test k3s-setup k3s-stop k3s-clean setup

# Build the Go application
build:
	go build -o bin/arenaservice ./cmd/arenaservice

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

# k3s cluster setup
k3s-setup:
	chmod +x scripts/k3s-setup.sh
	./scripts/k3s-setup.sh

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

# Restart ArenaService in k3s
k3s-restart: build-static
	kubectl rollout restart deployment/arenaservice -n control-plane
	@echo "ArenaService restarted in k3s cluster"

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
	@echo "  wg-genkeys         - Generate WireGuard key pair"
	@echo "  clean              - Clean up build artifacts"
	@echo "  help               - Show this help" 