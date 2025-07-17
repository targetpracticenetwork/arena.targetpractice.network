# Multi-stage build for ArenaService with WireGuard support
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o arenaservice ./cmd/arenaservice

# Final stage
FROM alpine:latest

# Install WireGuard and other required packages
RUN apk add --no-cache \
    wireguard-tools \
    iptables \
    iproute2 \
    bash \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 -S arenaservice && \
    adduser -u 1000 -S arenaservice -G arenaservice

# Create necessary directories
RUN mkdir -p /etc/wireguard /var/run/wireguard /opt/arenaservice && \
    chown -R arenaservice:arenaservice /etc/wireguard /var/run/wireguard /opt/arenaservice

# Copy the binary from builder stage
COPY --from=builder /app/arenaservice /opt/arenaservice/arenaservice

# Copy WireGuard setup scripts
COPY scripts/wireguard-setup.sh /opt/arenaservice/wireguard-setup.sh
COPY scripts/entrypoint.sh /opt/arenaservice/entrypoint.sh

# Make scripts executable
RUN chmod +x /opt/arenaservice/wireguard-setup.sh /opt/arenaservice/entrypoint.sh

# Set ownership
RUN chown -R arenaservice:arenaservice /opt/arenaservice

# Switch to non-root user
USER arenaservice

# Set working directory
WORKDIR /opt/arenaservice

# Expose ports
EXPOSE 443 11820/udp

# Set entrypoint
ENTRYPOINT ["./entrypoint.sh"] 