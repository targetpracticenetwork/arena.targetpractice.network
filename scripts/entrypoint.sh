#!/bin/bash

set -e

echo "Starting ArenaService container..."

# Set default environment variables
export WIREGUARD_INTERFACE=${WIREGUARD_INTERFACE:-wg0}
export GRPC_LISTEN_ADDR=${GRPC_LISTEN_ADDR:-:443}
export ARENA_INSECURE_DISABLE_MTLS=${ARENA_INSECURE_DISABLE_MTLS:-false}

# Run WireGuard setup
echo "Setting up WireGuard..."
./wireguard-setup.sh

# Wait a moment for WireGuard to fully initialize
sleep 2

# Verify WireGuard interface is working
if ! ip link show $WIREGUARD_INTERFACE > /dev/null 2>&1; then
    echo "ERROR: WireGuard interface $WIREGUARD_INTERFACE is not available"
    exit 1
fi

# Check if the interface is up
if ! ip link show $WIREGUARD_INTERFACE | grep -q "state UP"; then
    echo "ERROR: WireGuard interface $WIREGUARD_INTERFACE is not up"
    exit 1
fi

echo "WireGuard interface $WIREGUARD_INTERFACE is ready"

# Display WireGuard status
echo "WireGuard status:"
wg show $WIREGUARD_INTERFACE

# Start ArenaService
echo "Starting ArenaService..."
exec ./arenaservice 