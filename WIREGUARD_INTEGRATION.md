# WireGuard Integration with ArenaService

This document explains how WireGuard is integrated with ArenaService to provide secure VPN access to user arenas.

## Architecture Overview

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   User Client   │    │   ArenaService   │    │  Kubernetes     │
│                 │    │                  │    │   Cluster       │
│ ┌─────────────┐ │    │ ┌──────────────┐ │    │ ┌─────────────┐ │
│ │ WireGuard   │◄────►│ │ WireGuard    │ │    │ │ User Arena  │ │
│ │ Client      │ │    │ │ Interface    │ │    │ │ Namespace   │ │
│ └─────────────┘ │    │ └──────────────┘ │    │ └─────────────┘ │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## How It Works

1. **ArenaService starts** and initializes the WireGuard interface (`wg0`)
2. **User requests arena creation** via gRPC API
3. **ArenaService creates**:
   - Kubernetes namespace and resources
   - WireGuard peer for the user
   - Network policy allowing only the user's VPN IP
4. **User connects** to WireGuard using provided configuration
5. **User accesses** arena resources through the VPN tunnel

## WireGuard Setup Process

### 1. Container Initialization
When the ArenaService container starts:

```bash
# Entrypoint script runs
./entrypoint.sh
  ├── ./wireguard-setup.sh  # Sets up WireGuard interface
  └── ./arenaservice        # Starts the gRPC service
```

### 2. WireGuard Interface Setup
The `wireguard-setup.sh` script:

1. **Loads WireGuard kernel module**
2. **Generates or uses provided keys**
3. **Creates WireGuard interface** (`wg0`)
4. **Configures interface** with:
   - Private key
   - IP address (default: `10.0.0.1/24`)
   - Listen port (default: `11820`)
   - IP forwarding and NAT rules
5. **Brings interface up**

### 3. ArenaService Integration
ArenaService uses the `WGManager` to:

```go
// Initialize VPN manager
vpnManager := &arena.WGManager{InterfaceName: "wg0"}

// Check WireGuard interface is available
if err := vpnManager.CheckDevice(); err != nil {
    log.Fatalf("WireGuard interface %s is not available: %v", wgInterface, err)
}
```

## Key Components

### WGManager (internal/arena/vpn.go)
- **CreatePeer()**: Adds a new WireGuard peer for a user
- **RemovePeer()**: Removes a user's WireGuard peer
- **GetPeerStatus()**: Gets connection status for a peer
- **CheckDevice()**: Verifies WireGuard interface is working

### WireGuard Configuration
The WireGuard interface is configured with:

```ini
[Interface]
PrivateKey = <server_private_key>
Address = 10.0.0.1/24
ListenPort = 11820
SaveConfig = true

# Enable IP forwarding
PostUp = sysctl -w net.ipv4.ip_forward=1
PostUp = sysctl -w net.ipv6.conf.all.forwarding=1

# NAT rules for routing
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT
PostUp = iptables -A FORWARD -o wg0 -j ACCEPT
PostUp = iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
```

## User Peer Management

### Creating a User Peer
When a user requests an arena:

```go
// Parse user's VPN IP
_, ipnet, err := net.ParseCIDR(vpnIP) // e.g., "10.0.0.2/32"
allowedIPNets := []net.IPNet{*ipnet}

// Create WireGuard peer
peerCfg, err := vpnManager.CreatePeer(publicKey, allowedIPNets)
```

### User Configuration
The user receives a WireGuard configuration like:

```ini
[Interface]
PrivateKey = <user_private_key>
Address = 10.0.0.2/24
DNS = 8.8.8.8

[Peer]
PublicKey = <server_public_key>
Endpoint = <server_ip>:11820
AllowedIPs = 10.0.0.0/24
PersistentKeepalive = 25
```

## Network Security

### Network Policies
Each user namespace gets a NetworkPolicy that:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: arena-vpn-policy
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
  ingress:
  - from:
    - ipBlock:
        cidr: 10.0.0.2/32  # Only user's VPN IP
  egress:
  - to:
    - ipBlock:
        cidr: 10.0.0.2/32  # Only to user's VPN IP
```

This ensures:
- **Isolation**: Users can only access their own arenas
- **Security**: No cross-user communication
- **VPN-only access**: Resources only accessible via WireGuard

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `WIREGUARD_INTERFACE` | `wg0` | WireGuard interface name |
| `WIREGUARD_ADDRESS` | `10.0.0.1/24` | Server IP address |
| `WIREGUARD_PORT` | `11820` | WireGuard listen port |
| `WIREGUARD_PRIVATE_KEY` | auto-generated | Server private key |
| `WIREGUARD_PUBLIC_KEY` | auto-generated | Server public key |

## Running with Docker

### Quick Start
```bash
# Build and run
make docker-build-run

# Check status
make wg-status

# View logs
make docker-logs
```

### Manual Setup
```bash
# Build image
docker build -t arenaservice:latest .

# Run container
docker run --privileged --network host \
  -e WIREGUARD_INTERFACE=wg0 \
  -e ARENA_INSECURE_DISABLE_MTLS=true \
  arenaservice:latest
```

## Troubleshooting

### Common Issues

1. **WireGuard interface not found**
   ```bash
   # Check if WireGuard module is loaded
   lsmod | grep wireguard
   
   # Load module manually
   modprobe wireguard
   ```

2. **Permission denied**
   ```bash
   # Container needs privileged mode
   docker run --privileged --network host ...
   ```

3. **Network connectivity issues**
   ```bash
   # Check WireGuard status
   wg show wg0
   
   # Check interface status
   ip link show wg0
   ```

### Debug Commands
```bash
# Check WireGuard status
docker exec arenaservice wg show wg0

# Check interface configuration
docker exec arenaservice ip addr show wg0

# Check routing
docker exec arenaservice ip route show

# Check iptables rules
docker exec arenaservice iptables -L -n -v
```

## Security Considerations

1. **Key Management**: WireGuard keys should be securely stored and rotated
2. **Network Isolation**: Each user gets isolated network access
3. **Access Control**: Only VPN IPs can access arena resources
4. **Audit Logging**: All peer operations are logged
5. **Resource Limits**: Users are limited by Kubernetes resource quotas

## Performance

- **Low Latency**: WireGuard provides near-native performance
- **High Throughput**: Efficient cryptographic operations
- **Scalability**: Supports hundreds of concurrent users
- **Resource Usage**: Minimal CPU and memory overhead 