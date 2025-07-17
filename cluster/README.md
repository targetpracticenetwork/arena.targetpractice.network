# Cluster Configuration for ArenaService

This directory contains all static Kubernetes configuration and manifests required to bootstrap and manage the ArenaService arena cluster. It ensures strong separation between the control plane, VPN plane, and the user "arena area" for secure, reproducible deployments.

## Directory Structure

- `base/`           — Core manifests: CRDs, RBAC, baseline namespaces
- `control-plane/`  — Control plane infrastructure (ArenaService deployment, admin tools)
- `vpn-plane/`      — VPN infrastructure (WireGuard DaemonSet, supporting services)
- `arena-area/`     — Baseline for user challenge area (default network policies, quotas)
- `overlays/`       — Kustomize overlays for dev/staging/prod environments

## Bootstrapping the Cluster

1. **Install prerequisites:**
   - [kubectl](https://kubernetes.io/docs/tasks/tools/)
   - [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/)

2. **Apply the base configuration:**
   ```sh
   kubectl apply -k ./base
   ```

3. **Apply the control plane and VPN plane configuration:**
   ```sh
   kubectl apply -k ./control-plane
   kubectl apply -k ./vpn-plane
   ```

4. **Apply the arena area baseline:**
   ```sh
   kubectl apply -k ./arena-area
   ```

5. **(Optional) Use overlays for environment-specific configuration:**
   ```sh
   kubectl apply -k ./overlays/dev
   # or ./overlays/prod, etc.
   ```

## Notes
- The ArenaService binary will manage all dynamic resources (user namespaces, challenge pods, VPN peers) at runtime.
- This directory is for static, baseline configuration only.
- Update or extend these manifests as your cluster requirements evolve. 