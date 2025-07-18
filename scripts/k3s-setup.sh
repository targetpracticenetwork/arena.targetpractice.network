#!/bin/bash

set -e

echo "🚀 Setting up k3s cluster for ArenaService..."

# Check if k3s is already installed
if command -v k3s > /dev/null 2>&1; then
    echo "✅ k3s is already installed"
else
    echo "📦 Installing k3s..."
    curl -sfL https://get.k3s.io | sh -
    
    # Wait for k3s to be ready
    echo "⏳ Waiting for k3s to start..."
    sleep 30
fi

# Get kubeconfig
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
sudo chmod 644 $KUBECONFIG

# Verify cluster is ready
echo "🔍 Checking cluster status..."
kubectl get nodes
kubectl get pods --all-namespaces

# Install required tools
echo "📦 Installing additional tools..."
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.2/deploy/static/provider/baremetal/deploy.yaml

# Build static binary for deployment
echo "🔨 Building static binary for ArenaService..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -installsuffix cgo -o bin/arenaservice-static ./cmd/arenaservice

# Apply ArenaService cluster configuration
echo "🏗️ Applying ArenaService cluster configuration..."

# Control plane (includes integrated WireGuard)
echo "🎮 Applying control plane..."
kubectl apply -k ./cluster/control-plane

# Arena area
echo "🏟️ Applying arena area..."
kubectl apply -k ./cluster/arena-area

# Wait for all pods to be ready
echo "⏳ Waiting for all pods to be ready..."
kubectl wait --for=condition=ready pod --all --all-namespaces --timeout=300s

echo ""
echo "🎉 k3s cluster is ready for ArenaService!"
echo ""
echo "📋 Cluster info:"
kubectl cluster-info
echo ""
echo "🔗 Access points:"
echo "  Kubernetes API: https://localhost:6443"
echo "  ArenaService gRPC: localhost:443"
echo "  WireGuard: localhost:11820/udp"
echo ""
echo "📖 Useful commands:"
echo "  kubectl get pods --all-namespaces"
echo "  kubectl logs -n control-plane deployment/arenaservice"
echo "  kubectl get networkpolicies --all-namespaces"
echo ""
echo "🛑 To stop k3s: sudo systemctl stop k3s"
echo "🗑️  To uninstall k3s: /usr/local/bin/k3s-uninstall.sh" 