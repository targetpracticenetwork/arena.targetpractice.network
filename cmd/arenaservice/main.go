package main

import (
	"crypto/tls"
	"log"
	"net"
	"os"

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

func main() {
	// Load configuration from environment variables (with defaults)
	wgInterface := getEnvWithDefault("WIREGUARD_INTERFACE", "wg0")
	grpcAddr := getEnvWithDefault("GRPC_LISTEN_ADDR", ":443")
	insecureMode := getEnvWithDefault("ARENA_INSECURE_DISABLE_MTLS", "false") == "true"

	// Initialize managers
	k8sManager := arena.NewK8sManager() // dummy implementation
	vpnManager := &arena.WGManager{InterfaceName: wgInterface}

	// Check VPN interface before starting gRPC service
	if err := vpnManager.CheckDevice(); err != nil {
		log.Fatalf("WireGuard interface %s is not available: %v", wgInterface, err)
	}

	arenaManager := arena.NewArenaManager(k8sManager, vpnManager)

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

	securityMode := "insecure"
	if !insecureMode {
		securityMode = "mTLS"
	}
	log.Printf("ArenaService starting on %s with %s (WireGuard interface: %s)...", grpcAddr, securityMode, wgInterface)
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", grpcAddr, err)
	}
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve gRPC: %v", err)
	}
}
