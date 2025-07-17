# Makefile for ArenaService

.PHONY: build run stop clean test docker-build docker-run docker-stop k3s-setup k3s-stop k3s-clean

# Build the Go application
build:
	go build -o bin/arenaservice ./cmd/arenaservice

# Run the application locally (requires WireGuard setup)
run: build
	./bin/arenaservice

# Run tests
test:
	go test -v ./...

# Build Docker image
docker-build:
	docker build -t arenaservice:latest .

# Run with Docker Compose
docker-run:
	docker-compose up -d

# Stop Docker Compose
docker-stop:
	docker-compose down

# View logs
docker-logs:
	docker-compose logs -f arenaservice

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

# Clean up
clean:
	rm -rf bin/
	docker-compose down --volumes --remove-orphans
	docker rmi arenaservice:latest 2>/dev/null || true

# Build and run in one command
docker-build-run: docker-build docker-run

# Show WireGuard status
wg-status:
	docker exec arenaservice wg show wg0

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
	@echo "  run                - Run application locally"
	@echo "  test               - Run tests"
	@echo "  docker-build       - Build Docker image"
	@echo "  docker-run         - Start with Docker Compose"
	@echo "  docker-stop        - Stop Docker Compose"
	@echo "  docker-logs        - View logs"
	@echo "  docker-build-run   - Build and run in one command"
	@echo "  k3s-setup          - Set up k3s cluster"
	@echo "  k3s-stop           - Stop k3s cluster"
	@echo "  k3s-clean          - Uninstall k3s"
	@echo "  k3s-status         - Show k3s cluster status"
	@echo "  k3s-logs           - Show ArenaService logs in k3s"
	@echo "  wg-status          - Show WireGuard status"
	@echo "  wg-genkeys         - Generate WireGuard key pair"
	@echo "  clean              - Clean up build artifacts and containers"
	@echo "  help               - Show this help" 