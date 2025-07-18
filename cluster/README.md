# Cluster Configuration for ArenaService

This directory contains all static Kubernetes configuration and manifests required to bootstrap and manage the ArenaService arena cluster. It ensures strong separation between the control plane and the user "arena area" for secure, reproducible deployments.

## Directory Structure

- `control-plane/`  — Control plane infrastructure (ArenaService deployment with integrated WireGuard)
- `arena-area/`     — Baseline for user challenge area (default network policies, quotas)

## Bootstrapping the Cluster

1. **Install prerequisites:**
   - [kubectl](https://kubernetes.io/docs/tasks/tools/)
   - [k3s](https://k3s.io/) (recommended)

2. **Set up the server:**
   ```sh
   make setup
   ```

3. **Apply the control plane configuration:**
   ```sh
   kubectl apply -k ./control-plane
   ```

4. **Apply the arena area baseline:**
   ```sh
   kubectl apply -k ./arena-area
   ```

5. **Or use the automated setup:**
   ```sh
   make k3s-setup
   ```

## Notes
- The ArenaService binary will manage all dynamic resources (user namespaces, challenge pods, VPN peers) at runtime.
- This directory is for static, baseline configuration only.
- WireGuard functionality is now integrated into the main ArenaService container.
- Update or extend these manifests as your cluster requirements evolve. 