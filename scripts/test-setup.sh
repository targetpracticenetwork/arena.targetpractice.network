#!/bin/bash

set -e

echo "🚀 Setting up ArenaService test environment..."

# Check if Docker is running
if ! docker ps > /dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker Desktop first."
    exit 1
fi

echo "✅ Docker is running"

# Build and start ArenaService
echo "🔨 Building ArenaService..."
make docker-build

echo "🚀 Starting ArenaService..."
make docker-run

# Wait for service to be ready
echo "⏳ Waiting for ArenaService to start..."
sleep 10

# Check if container is running
if ! docker ps | grep -q arenaservice; then
    echo "❌ ArenaService container is not running"
    make docker-logs
    exit 1
fi

echo "✅ ArenaService container is running"

# Check WireGuard status
echo "🔒 Checking WireGuard status..."
make wg-status

# Test gRPC connection (if client is available)
if command -v grpcurl > /dev/null 2>&1; then
    echo "🔍 Testing gRPC connection..."
    sleep 5
    if grpcurl -plaintext localhost:443 list; then
        echo "✅ gRPC connection successful"
    else
        echo "⚠️  gRPC connection failed (this is normal if no TLS certs)"
    fi
else
    echo "ℹ️  Install grpcurl to test gRPC: brew install grpcurl"
fi

echo ""
echo "🎉 ArenaService is ready!"
echo ""
echo "📋 Useful commands:"
echo "  make docker-logs    - View logs"
echo "  make wg-status      - Check WireGuard status"
echo "  make docker-stop    - Stop ArenaService"
echo "  make clean          - Clean up everything"
echo ""
echo "🔗 ArenaService is running on:"
echo "  gRPC: localhost:443"
echo "  WireGuard: localhost:11820/udp"
echo ""
echo "📖 For more info, see WIREGUARD_INTEGRATION.md" 