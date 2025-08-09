package main

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"arena/internal/arena"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// getEnvWithDefault returns the environment variable value or the default if not set
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntWithDefault returns the environment variable value as int or the default if not set
func getEnvIntWithDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func main() {
	// Load configuration from environment variables (with defaults)
	wgInterface := getEnvWithDefault("WIREGUARD_INTERFACE", "wg0")
	wgPort := getEnvIntWithDefault("WIREGUARD_PORT", 11820)
	wgServerHost := getEnvWithDefault("WIREGUARD_SERVER_HOST", "localhost")
	grpcAddr := getEnvWithDefault("GRPC_LISTEN_ADDR", ":443")
	insecureMode := getEnvWithDefault("ARENA_INSECURE_DISABLE_MTLS", "false") == "true"

	// Load CIDR configuration from environment variables (with defaults)
	vpnSubnet := getEnvWithDefault("VPN_SUBNET", "10.0.0.0/24")
	clusterCIDR := getEnvWithDefault("CLUSTER_CIDR", "10.42.0.0/24")
	serviceCIDR := getEnvWithDefault("SERVICE_CIDR", "10.43.0.0/16")

	// Load arena creation timeout configuration
	creationTimeoutSeconds := getEnvIntWithDefault("ARENA_CREATION_TIMEOUT_SECONDS", 300) // default 5 minutes
	creationTimeout := time.Duration(creationTimeoutSeconds) * time.Second

	log.Printf("🔧 Network Configuration:")
	log.Printf("  VPN Subnet: %s", vpnSubnet)
	log.Printf("  Cluster CIDR: %s", clusterCIDR)
	log.Printf("  Service CIDR: %s", serviceCIDR)
	log.Printf("  WireGuard Port: %d", wgPort)
	log.Printf("  WireGuard Server Host: %s", wgServerHost)
	log.Printf("  Arena Creation Timeout: %s", creationTimeout)

	// Initialize managers with CIDR configuration
	k8sManager := arena.NewK8sManager(clusterCIDR, serviceCIDR)
	vpnManager, err := arena.NewWGManager(wgInterface, vpnSubnet, serviceCIDR, wgPort, wgServerHost)
	if err != nil {
		log.Fatalf("Failed to initialize WireGuard manager: %v", err)
	}

	// Check VPN interface before starting gRPC service
	if err := vpnManager.CheckDevice(); err != nil {
		log.Fatalf("WireGuard interface %s is not available: %v", wgInterface, err)
	}

	arenaManager := arena.NewArenaManagerWithTimeout(k8sManager, vpnManager, creationTimeout)

	// Set up gRPC server
	var grpcServer *grpc.Server

	if insecureMode {
		log.Println("Warning: Running in insecure mode (mTLS disabled)")
		grpcServer = grpc.NewServer()
	} else {
		// Set up mTLS credentials
		tlsConfig := &tls.Config{
			ClientAuth: tls.RequireAndVerifyClientCert,
			MinVersion: tls.VersionTLS12,
		}

		// Create gRPC server with mTLS credentials
		creds := credentials.NewTLS(tlsConfig)
		grpcServer = grpc.NewServer(grpc.Creds(creds))
	}

	arena.RegisterArenaServiceServer(grpcServer, arena.NewArenaServiceServer(arenaManager))

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("🛑 Received shutdown signal, initiating graceful shutdown...")

		// Create a context with timeout for shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Shutdown arena manager if it supports it
		if shutdowner, ok := arenaManager.(interface{ Shutdown(context.Context) error }); ok {
			log.Println("🛑 Shutting down arena manager...")
			if err := shutdowner.Shutdown(shutdownCtx); err != nil {
				log.Printf("❌ Error shutting down arena manager: %v", err)
			}
		}

		// Gracefully stop the gRPC server
		log.Println("🛑 Shutting down gRPC server...")
		grpcServer.GracefulStop()

		// Close VPN manager if it supports it
		if closer, ok := vpnManager.(interface{ Close() error }); ok {
			log.Println("🛑 Closing VPN manager...")
			if err := closer.Close(); err != nil {
				log.Printf("❌ Error closing VPN manager: %v", err)
			}
		}

		log.Println("✅ Graceful shutdown complete")
		os.Exit(0)
	}()

	securityMode := "insecure"
	if !insecureMode {
		securityMode = "mTLS"
	}
	log.Printf("🚀 ArenaService starting on %s with %s (WireGuard interface: %s)...", grpcAddr, securityMode, wgInterface)
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", grpcAddr, err)
	}
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve gRPC: %v", err)
	}
}
