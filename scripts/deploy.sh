#!/bin/bash

set -e

echo "🚀 Deploying ArenaService to k3s cluster..."

# Check if we're in the right directory
if [ ! -f "go.mod" ]; then
    echo "❌ Error: go.mod not found. Please run this script from the ArenaService root directory."
    exit 1
fi

# Check if k3s is running
if ! kubectl get nodes > /dev/null 2>&1; then
    echo "❌ Error: k3s is not running or kubectl is not configured."
    echo "   Please run: make k3s-setup"
    exit 1
fi

# Build static binary
echo "🔨 Building static binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -installsuffix cgo -o bin/arenaservice-static ./cmd/arenaservice

# Create bin directory if it doesn't exist
mkdir -p bin

# Deploy to k3s
echo "📦 Deploying to k3s cluster..."
kubectl apply -k ./cluster/control-plane/

# Wait for deployment to be ready
echo "⏳ Waiting for ArenaService to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/arenaservice -n control-plane

# Show status
echo "✅ ArenaService deployed successfully!"
echo ""
echo "📋 Pod Status:"
kubectl get pods -n control-plane
echo ""
echo "📋 Service (LoadBalancer):"
kubectl get service -n control-plane
echo ""
echo "🔧 WireGuard Status:"
kubectl exec -n control-plane deployment/arenaservice -- ip link show wg0 2>/dev/null || echo "  WireGuard interface not yet ready"
echo ""
echo "📋 Recent Logs:"
kubectl logs -n control-plane deployment/arenaservice --tail=5
echo ""
echo "🔗 Access points:"
echo "  ArenaService gRPC: localhost:443 (via LoadBalancer)"
echo "  WireGuard Server: localhost:11820/udp"
echo ""
echo "📖 Useful commands:"
echo "  kubectl logs -n control-plane deployment/arenaservice -f"
echo "  kubectl get pods -n control-plane"
echo "  kubectl describe service -n control-plane arenaservice"
echo "  kubectl exec -n control-plane deployment/arenaservice -- ip link show wg0" 