# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ArenaService is a secure, multi-user CTF arena platform written in Go that manages Kubernetes-based CTF environments. It orchestrates the entire lifecycle of isolated user arenas including challenge provisioning, user isolation via Kubernetes namespaces, and VPN access management through WireGuard, all exposed via a secure gRPC API.

## Key Architecture Components

- **ArenaManager**: Core orchestration layer that coordinates K8s and VPN managers for full arena lifecycle management (`internal/arena/orchestration.go`)
- **gRPC API**: Primary interface for arena operations defined in `api/arena.proto` with implementation in `internal/arena/arenaservice.go`
- **K8s Manager**: Handles Kubernetes operations including namespace creation, pod deployment, and resource management (`internal/arena/k8s.go`)
- **VPN Manager**: Manages WireGuard peer configuration for user network isolation (`internal/arena/vpn.go`)
- **Challenge Provisioning**: Pluggable interface system for different challenge deployment strategies

## Development Commands

### Building
```bash
# Build for development
make build

# Build static binary for k3s deployment (ARM64)
make build-static
```

### Testing
```bash
# Run all tests
make test
```

### Deployment and Infrastructure
```bash
# Initial server setup (k3s, firewall, security)
make setup

# Deploy to k3s cluster
make k3s-deploy

# Restart ArenaService in k3s
make k3s-restart

# Check cluster status
make k3s-status

# View logs
make k3s-logs

# Clean up deployment
make k3s-clean-deployment
```

### WireGuard Management
```bash
# Generate WireGuard key pairs
make wg-genkeys
```

## Environment Configuration

Key environment variables for ArenaService:

- `WIREGUARD_INTERFACE`: WireGuard interface name (default: wg0)
- `WIREGUARD_PORT`: WireGuard server port (default: 11820)
- `GRPC_LISTEN_ADDR`: gRPC server address (default: :443)
- `VPN_SUBNET`: VPN IP allocation range (default: 10.0.0.0/24)
- `CLUSTER_CIDR`: Kubernetes pod network (default: 10.42.0.0/24)
- `SERVICE_CIDR`: Kubernetes service network (default: 10.43.0.0/16)
- `ARENA_INSECURE_DISABLE_MTLS`: Disable mTLS for development (default: false)

## Security Architecture

The system implements multi-layered isolation:
- **Network Isolation**: Each user gets dedicated VPN IP with iptables filtering
- **Kubernetes Isolation**: Dedicated namespaces with network policies and resource quotas
- **Source/Destination IP Validation**: WireGuard peer restrictions and custom iptables rules
- **mTLS Authentication**: Secure gRPC communications (can be disabled for development)

## Key Design Patterns

- **Interface-based Architecture**: Core managers (ArenaManager, K8sManager, VPNManager) use interfaces for testability
- **Namespace Convention**: User arenas use `arena-{ip-with-dashes}` naming (e.g., `arena-10-0-0-23`)
- **Error Handling**: Comprehensive logging with emoji indicators for operations
- **Stateless Operations**: Arena management is idempotent where possible

## Private Registry Support

ArenaService supports private container registries with automatic image pre-warming for improved performance.

### Setting up Registry Credentials

Create a Kubernetes secret in each arena namespace with your private registry credentials:

```bash
# Create registry secret (replace with your values)
kubectl create secret docker-registry my-registry-secret \
  --docker-server=registry.example.com \
  --docker-username=myuser \
  --docker-password=mypassword \
  --docker-email=user@example.com \
  --namespace=arena-10-0-0-23
```

### Using Private Registry in Challenge Specs

Add registry configuration to your challenge YAML:

```yaml
registry:
  credentialsSecret: "my-registry-secret"
  url: "registry.example.com"  # optional, inferred from image paths

containers:
  - name: web-server
    image: "registry.example.com/ctf/web:latest"  # full path
    ports:
      - containerPort: 80
  - name: database  
    image: "mysql:8.0"  # relative path (uses registry.url prefix)
    env:
      - name: MYSQL_ROOT_PASSWORD
        value: "password"
```

### Image Pre-warming

The system automatically pre-warms (pre-pulls) private images using Kubernetes Jobs before creating arena pods. This improves pod startup time and ensures images are available. Pre-warmed images persist on cluster nodes until evicted by Kubernetes.

## Protocol Buffer Generation

When modifying `api/arena.proto`, regenerate Go code:
```bash
protoc --go_out=. --go-grpc_out=. api/arena.proto
```

## Testing Infrastructure

Tests use Kubernetes test clients and can be run with standard Go testing. The system includes both unit tests and integration test capabilities for K8s operations.