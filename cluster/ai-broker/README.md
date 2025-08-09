## AI Broker Deployment

Deploy the AI broker service to k3s using the Makefile.

### Why exec probes?

NetworkPolicy may block kubelet’s HTTP probes when they originate from node IPs not in allowed sources. Switching to exec probes makes the probe run inside the container and call `http://127.0.0.1/...`, bypassing the network layer entirely. This ensures liveness/readiness do not depend on NetworkPolicy and avoids kubelet probe failures.

### Quick Start

1. Create secret with your API tokens (if not already created by Makefile on deploy):
   ```bash
   kubectl create secret generic aibroker-env \
     --namespace=ai-broker \
     --from-literal=AIBROKER_TOKEN="your-deepseek-token-here"
   ```

2. Deploy:
   ```bash
   make k3s-deploy-aibroker
   ```

3. Verify readiness/liveness and endpoints:
   ```bash
   kubectl -n ai-broker describe pod -l app=aibroker | grep -A2 -E 'Liveness|Readiness'
   kubectl -n ai-broker get endpoints aibroker -o wide
   ```

### Usage

- base_url: `http://aibroker.ai-broker.svc.cluster.local`
  - The broker forwards to `https://api.deepseek.com/v1` with Authorization injected.

### Optional: syncing node IPs for HTTP probes (not needed with exec probes)

If you ever switch back to HTTP probes, you can allow kubelet source IPs by syncing node InternalIPs into the NetworkPolicy’s `kubelet-allow` rule:

```bash
make sync-node-ips
```

This updates `cluster/ai-broker/networkpolicy.yaml` with `/32` entries and applies it. It is disabled by default and not required for the current exec-probe configuration.

### Management Commands

- `make k3s-deploy-aibroker` - Deploy/update the broker
- `make k3s-restart-aibroker` - Restart the broker
- `make k3s-status-aibroker` - Check broker status
- `make k3s-logs-aibroker` - View broker logs
- `make k3s-delete-aibroker` - Remove the broker