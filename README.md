# ArenaService

ArenaService is a secure, multi-user CTF arena platform written in Go. It manages the entire lifecycle of a Kubernetes-based CTF arena, including user isolation, challenge provisioning, and VPN access, all via a secure gRPC API.

## Project Structure

```
arena/
├── cmd/arenaservice/        # Main Go entrypoint (the ArenaService binary)
├── api/                     # gRPC proto definitions and generated code
├── internal/
│   ├── k8s/                 # Kubernetes management logic
│   ├── vpn/                 # WireGuard management logic
│   ├── auth/                # Auth (mTLS/JWT) middleware
│   ├── challenge/           # PodSource/ChallengeProvider interface & implementations
│   ├── arena/               # Arena orchestration logic
│   └── config/              # Configuration loading/validation
├── pkg/                     # (Optional) Public Go packages for extension
├── cluster/                 # Cluster bootstrap/configuration (see below)
│   ├── control-plane/       # Control plane infra (ArenaService with integrated WireGuard)
│   ├── arena-area/          # Baseline for user challenge area (network policies, quotas)
│   └── README.md            # Cluster setup instructions
├── scripts/                 # Helper scripts (setup.sh, deploy.sh)
├── Makefile
└── README.md
```

## Key Features
- **Cluster Management:** ArenaService is the sole authority for managing the arena cluster, including namespaces, network policies, and resource quotas.
- **User Isolation:** Each user gets a dedicated namespace and challenge pods, isolated via Kubernetes and WireGuard.
- **VPN Management:** WireGuard peers are managed for each user, restricting access to only their resources.
- **Pluggable Challenge Provisioning:** The logic for spawning challenge pods is defined as an interface, allowing for flexible implementations.
- **Secure gRPC API:** All management is performed via a secure, authenticated gRPC API.
- **Self-Contained Connectivity:** Users access challenges only via VPN; the website interacts only with the gRPC API.

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

### Security Features

- **Server-side IP Filtering**: Users can only access service IPs specifically allocated to their arena, enforced at the WireGuard and iptables level.
- **Source IP Validation**: WireGuard peers can only use their assigned VPN IP as source.
- **Destination IP Filtering**: Custom iptables rules prevent users from accessing other users' service endpoints.

## Cluster Configuration
All static cluster configuration (namespaces, RBAC, baseline policies, etc.) lives in the `cluster/` directory, making it easy to bootstrap and manage the arena cluster.

---

See `cluster/README.md` for cluster setup instructions. 