# ArenaService Deployment Guide

This guide explains how to deploy ArenaService using k3s with the updated configuration.

## Quick Start

### 1. Initial Server Setup
```bash
# Run the server setup script
make setup
```

This will:
- Install k3s cluster
- Configure firewall and security
- Set up monitoring and backup scripts
- Deploy ArenaService automatically (if repository is present)

### 2. Manual Deployment
If you need to deploy manually:

```bash
# Build and deploy to k3s
make k3s-deploy

# Or use the deployment script directly
./scripts/deploy.sh
```

## Architecture

The ArenaService is now deployed using:
- **k3s**: Lightweight Kubernetes cluster
- **Alpine Linux**: Base container image
- **Static Binary**: Go binary built with CGO disabled
- **HostPath Mount**: Binary mounted from host filesystem
- **WireGuard**: Network interface created in container

## Configuration Files

### Deployment YAML (`cluster/control-plane/deployment.yaml`)
- Uses Alpine base image
- Mounts static binary from host
- Creates WireGuard interface
- Runs with privileged mode and NET_ADMIN capabilities

### Service YAML (`cluster/control-plane/service.yaml`)
- LoadBalancer service
- Exposes ports 443 (gRPC) and 11820 (WireGuard)

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make build-static` | Build static binary for k3s deployment |
| `make k3s-deploy` | Deploy ArenaService to k3s |
| `make k3s-restart` | Restart ArenaService in k3s |
| `make k3s-status` | Show k3s cluster status |
| `make k3s-logs` | Show ArenaService logs |
| `make k3s-clean-deployment` | Remove ArenaService from k3s |

## Troubleshooting

### Check Pod Status
```bash
kubectl get pods -n control-plane
```

### View Logs
```bash
kubectl logs -n control-plane deployment/arenaservice -f
```

### Check Service
```bash
kubectl get service -n control-plane
```

### Restart Deployment
```bash
make k3s-restart
```

## Access Points

- **gRPC API**: `localhost:443`
- **WireGuard VPN**: `localhost:11820/udp`
- **k3s API**: `localhost:6443`

## Monitoring

Use the monitoring script to check system status:
```bash
/opt/monitor-arena.sh
```

## Backup

Create a backup of the system:
```bash
/opt/backup-arena.sh
```

## Security

- Firewall configured with UFW
- Fail2ban enabled
- k3s runs with minimal privileges
- ArenaService runs in privileged container for WireGuard

## Performance

Expected performance on ARM64:
- 5-10 concurrent arenas
- Fast arena creation (< 3s)
- Efficient resource usage
- Optimized for ARM architecture 