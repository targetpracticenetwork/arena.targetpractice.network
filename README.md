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
├── scripts/                 # Helper scripts (setup.sh, k3s-setup.sh, wireguard-setup.sh)
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

## Cluster Configuration
All static cluster configuration (namespaces, RBAC, baseline policies, etc.) lives in the `cluster/` directory, making it easy to bootstrap and manage the arena cluster.

---

See `cluster/README.md` for cluster setup instructions. 