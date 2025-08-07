# ArenaService

ArenaService is a secure, multi-user CTF arena platform written in Go. It manages the entire lifecycle of a Kubernetes-based CTF arena, including user isolation, challenge provisioning, and VPN access, all via a secure gRPC API.

## Project Structure

```
arena/
├── cmd/
│   ├── arenaservice/        # Main ArenaService binary (gRPC server)
│   └── arenaclient/         # CLI client for interacting with ArenaService
├── api/                     # gRPC proto definitions and generated code
│   └── arena.proto          # Core arena service API definition
├── internal/arena/          # Arena orchestration and management logic
│   ├── arenaservice.go      # gRPC service implementation
│   ├── orchestration.go    # Core ArenaManager orchestration layer
│   ├── k8s.go              # Kubernetes management logic
│   ├── vpn.go              # WireGuard VPN management
│   └── *_test.go           # Comprehensive test suite
├── cluster/                 # Kubernetes cluster configuration
│   ├── control-plane/       # ArenaService deployment manifests
│   ├── arena-area/          # Baseline for user challenge areas
│   └── README.md            # Cluster setup instructions
├── docs/                    # Additional documentation
│   └── REGISTRYPLAN.md      # Private registry integration guide
├── scripts/                 # Helper scripts (setup.sh, deploy.sh)
├── bin/                     # Build output directory
├── Makefile                 # Build and deployment automation
├── DEPLOYMENT.md            # Deployment guide and architecture
├── CLAUDE.md                # Development guidance for AI assistants
└── README.md
```

## Key Features
- **Cluster Management:** ArenaService is the sole authority for managing the arena cluster, including namespaces, network policies, and resource quotas.
- **User Isolation:** Each user gets a dedicated namespace and challenge pods, isolated via Kubernetes and WireGuard.
- **VPN Management:** WireGuard peers are managed for each user, restricting access to only their resources.
- **Challenge Provisioning:** Flexible challenge deployment system supporting both public and private container registries.
- **Private Registry Support:** Automatic image pre-warming and credential management for private container registries.
- **Secure gRPC API:** All management is performed via a secure, authenticated gRPC API with optional mTLS.
- **Self-Contained Connectivity:** Users access challenges only via VPN; the website interacts only with the gRPC API.
- **k3s Integration:** Optimized for deployment on k3s clusters with ARM64 support.
- **Comprehensive Monitoring:** Detailed arena status reporting including VPN peer status, Kubernetes resource usage, and health checks.
- **CLI Client:** Command-line client for easy interaction with the ArenaService API.

## Quick Start

### 1. Server Setup
```bash
# Complete server setup (k3s, firewall, security)
make setup
```

### 2. Build and Deploy
```bash
# Build static binary for ARM64 k3s deployment
make build-static

# Deploy ArenaService to k3s cluster
make k3s-deploy
```

### 3. Monitor Status
```bash
# Check cluster status
make k3s-status

# View ArenaService logs
make k3s-logs
```

## Configuration

### Environment Variables

The ArenaService can be configured using the following environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `WIREGUARD_INTERFACE` | `wg0` | WireGuard interface name |
| `WIREGUARD_PORT` | `11820` | WireGuard server port |
| `WIREGUARD_SERVER_HOST` | `localhost` | WireGuard server hostname/IP for client configuration |
| `GRPC_LISTEN_ADDR` | `:443` | gRPC server listen address |
| `ARENA_INSECURE_DISABLE_MTLS` | `false` | Disable mTLS for development/testing |
| `VPN_SUBNET` | `10.0.0.0/24` | VPN subnet CIDR for user IP allocation |
| `CLUSTER_CIDR` | `10.42.0.0/24` | Kubernetes cluster pod network CIDR |
| `SERVICE_CIDR` | `10.43.0.0/16` | Kubernetes service network CIDR |

### Network Configuration

- **VPN Subnet**: The `VPN_SUBNET` defines the IP range allocated to users for VPN access. Each user gets a `/32` IP from this range.
- **Cluster CIDR**: The `CLUSTER_CIDR` defines the Kubernetes pod network range. This is used for network policies to allow pod-to-pod communication within the cluster.
- **Service CIDR**: The `SERVICE_CIDR` defines the Kubernetes service network range. Services get cluster IPs from this range.

### Private Registry Configuration

ArenaService supports private container registries with automatic credential management. To use private registries, you need to create Kubernetes secrets with registry credentials.

#### Creating Registry Credentials Secret

```bash
# Create a registry secret for private registry access
kubectl create secret docker-registry my-registry-secret \
  --docker-server=registry.example.com \
  --docker-username=myuser \
  --docker-password=mypassword \
  --docker-email=user@example.com \
  --namespace=default

# The secret will be automatically copied to arena namespaces when created
# You can also create secrets in specific arena namespaces:
kubectl create secret docker-registry my-registry-secret \
  --docker-server=registry.example.com \
  --docker-username=myuser \
  --docker-password=mypassword \
  --docker-email=user@example.com \
  --namespace=arena-10-0-0-23
```

#### Using Registry Credentials in Challenge Specs

Reference the secret in your challenge specification:

```yaml
registry:
  credentialsSecret: "my-registry-secret"
  url: "registry.example.com"  # optional, inferred from image paths

containers:
  - name: web-server
    image: "registry.example.com/ctf/web:latest"
    ports:
      - containerPort: 80
```

### Security Features

- **Server-side IP Filtering**: Users can only access service IPs specifically allocated to their arena, enforced at the WireGuard and iptables level.
- **Source IP Validation**: WireGuard peers can only use their assigned VPN IP as source.
- **Destination IP Filtering**: Custom iptables rules prevent users from accessing other users' service endpoints.
- **mTLS Authentication**: Optional mutual TLS authentication for gRPC API security.

## Development & Build

### Available Make Targets

| Command | Description |
|---------|-------------|
| `make build` | Build Go application for development |
| `make build-static` | Build static binary for k3s deployment (ARM64) |
| `make test` | Run all tests |
| `make setup` | Complete server setup (k3s, firewall, security) |
| `make k3s-deploy` | Deploy ArenaService to k3s cluster |
| `make k3s-restart` | Restart ArenaService in k3s |
| `make k3s-status` | Show k3s cluster status |
| `make k3s-logs` | Show ArenaService logs |
| `make k3s-clean-deployment` | Remove ArenaService from k3s |
| `make wg-genkeys` | Generate WireGuard key pairs |
| `make clean` | Clean up build artifacts |

### gRPC API

The ArenaService exposes three main gRPC operations:

- **CreateArena**: Create a new isolated arena for a user with VPN and challenge provisioning
- **DestroyArena**: Clean up and remove a user's arena and associated resources  
- **GetArenaStatus**: Get comprehensive status of an arena including VPN and Kubernetes details

See `api/arena.proto` for complete API definitions.

### CLI Client

The `arenaclient` command provides a convenient CLI interface for interacting with ArenaService:

```bash
# Build the client
go build -o bin/arenaclient ./cmd/arenaclient

# Use the client (see client help for available commands)
./bin/arenaclient --help
```

## Architecture

ArenaService uses a layered architecture:

- **gRPC API Layer**: Handles incoming requests and authentication
- **ArenaManager**: Orchestrates the full lifecycle of arena creation/destruction
- **K8sManager**: Manages Kubernetes resources (namespaces, pods, services, network policies)
- **VPNManager**: Handles WireGuard peer configuration and network isolation
- **Challenge Provisioning**: Pluggable system for deploying different types of challenges

## Cluster Configuration
All static cluster configuration (namespaces, RBAC, baseline policies, etc.) lives in the `cluster/` directory, making it easy to bootstrap and manage the arena cluster.

---

See `cluster/README.md` for cluster setup instructions and `DEPLOYMENT.md` for detailed deployment information. 