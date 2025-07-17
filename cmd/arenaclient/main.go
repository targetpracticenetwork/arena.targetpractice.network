package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"arena/internal/arena"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// getEnvWithDefault returns the environment variable value or the default if not set
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Parse global flags
	serverAddr := getEnvWithDefault("ARENA_SERVER_ADDR", "localhost:443")
	insecureMode := getEnvWithDefault("ARENA_INSECURE_DISABLE_MTLS", "false") == "true"

	// Create gRPC connection
	var conn *grpc.ClientConn
	var err error

	if insecureMode {
		log.Println("Connecting in insecure mode (mTLS disabled)")
		conn, err = grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		// For mTLS, you would need to load certificates here
		// For now, we'll use insecure credentials for testing
		log.Println("Warning: Using insecure connection. For production, configure proper mTLS certificates.")
		conn, err = grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := arena.NewArenaServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse subcommand
	command := os.Args[1]
	switch command {
	case "create":
		handleCreate(ctx, client, os.Args[2:])
	case "destroy":
		handleDestroy(ctx, client, os.Args[2:])
	case "status":
		handleStatus(ctx, client, os.Args[2:])
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func handleCreate(ctx context.Context, client arena.ArenaServiceClient, args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	publicKey := fs.String("public-key", "", "Public key for the arena (required)")
	vpnIP := fs.String("vpn-ip", "", "VPN IP address (required)")
	challengeName := fs.String("challenge-name", "", "Challenge name (required)")
	challengeSpec := fs.String("challenge-spec", "", "Challenge specification (required)")

	fs.Parse(args)

	if *publicKey == "" || *vpnIP == "" || *challengeName == "" || *challengeSpec == "" {
		fmt.Println("Error: public-key, vpn-ip, challenge-name, and challenge-spec are required")
		fs.PrintDefaults()
		os.Exit(1)
	}

	req := &arena.CreateArenaRequest{
		PublicKey:     *publicKey,
		VpnIp:         *vpnIP,
		ChallengeName: *challengeName,
		ChallengeSpec: *challengeSpec,
	}

	resp, err := client.CreateArena(ctx, req)
	if err != nil {
		log.Fatalf("Failed to create arena: %v", err)
	}

	if resp.Error != "" {
		fmt.Printf("Error creating arena: %s\n", resp.Error)
		os.Exit(1)
	}

	fmt.Println("Arena created successfully!")
	fmt.Printf("Public Key: %s\n", resp.Config.PublicKey)
	fmt.Printf("Allowed IP: %s\n", resp.Config.AllowedIp)
	fmt.Printf("Challenge ID: %s\n", resp.Config.ChallengeId)
	fmt.Printf("Namespace: %s\n", resp.Config.Namespace)

	if len(resp.Config.Endpoints) > 0 {
		fmt.Println("Endpoints:")
		for _, endpoint := range resp.Config.Endpoints {
			fmt.Printf("  - %s: %s:%v\n", endpoint.Name, endpoint.ClusterIp, endpoint.Ports)
		}
	}
}

func handleDestroy(ctx context.Context, client arena.ArenaServiceClient, args []string) {
	fs := flag.NewFlagSet("destroy", flag.ExitOnError)
	publicKey := fs.String("public-key", "", "Public key of the arena to destroy (required)")

	fs.Parse(args)

	if *publicKey == "" {
		fmt.Println("Error: public-key is required")
		fs.PrintDefaults()
		os.Exit(1)
	}

	req := &arena.DestroyArenaRequest{
		PublicKey: *publicKey,
	}

	resp, err := client.DestroyArena(ctx, req)
	if err != nil {
		log.Fatalf("Failed to destroy arena: %v", err)
	}

	if !resp.Success {
		fmt.Printf("Error destroying arena: %s\n", resp.Error)
		os.Exit(1)
	}

	fmt.Println("Arena destroyed successfully!")
}

func handleStatus(ctx context.Context, client arena.ArenaServiceClient, args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	publicKey := fs.String("public-key", "", "Public key of the arena to check (required)")

	fs.Parse(args)

	if *publicKey == "" {
		fmt.Println("Error: public-key is required")
		fs.PrintDefaults()
		os.Exit(1)
	}

	req := &arena.GetArenaStatusRequest{
		PublicKey: *publicKey,
	}

	resp, err := client.GetArenaStatus(ctx, req)
	if err != nil {
		log.Fatalf("Failed to get arena status: %v", err)
	}

	if resp.Error != "" {
		fmt.Printf("Error getting arena status: %s\n", resp.Error)
		os.Exit(1)
	}

	status := resp.Status
	fmt.Printf("Arena Status for %s:\n", status.PublicKey)
	fmt.Printf("Namespace: %s\n", status.Namespace)
	fmt.Printf("Healthy: %t\n", status.Healthy)

	if len(status.Errors) > 0 {
		fmt.Println("Errors:")
		for _, err := range status.Errors {
			fmt.Printf("  - %s\n", err)
		}
	}

	// VPN Status
	if status.VpnStatus != nil {
		fmt.Println("\nVPN Status:")
		fmt.Printf("  Public Key: %s\n", status.VpnStatus.PublicKey)
		fmt.Printf("  Endpoint: %s\n", status.VpnStatus.Endpoint)
		fmt.Printf("  Allowed IPs: %v\n", status.VpnStatus.AllowedIps)
		fmt.Printf("  Last Handshake: %d\n", status.VpnStatus.LastHandshake)
		fmt.Printf("  RX Bytes: %d\n", status.VpnStatus.RxBytes)
		fmt.Printf("  TX Bytes: %d\n", status.VpnStatus.TxBytes)
	}

	// K8s Status
	if status.K8SStatus != nil {
		fmt.Println("\nKubernetes Status:")
		fmt.Printf("  Phase: %s\n", status.K8SStatus.Phase)
		fmt.Printf("  Pods: %d running, %d pending, %d failed\n",
			status.K8SStatus.PodsRunning, status.K8SStatus.PodsPending, status.K8SStatus.PodsFailed)
		fmt.Printf("  Services: %d/%d ready\n", status.K8SStatus.ServicesReady, status.K8SStatus.ServicesTotal)
		fmt.Printf("  Network Policy Applied: %t\n", status.K8SStatus.NetworkPolicyApplied)

		if status.K8SStatus.ResourceUsage != nil {
			fmt.Printf("  Resource Usage: CPU=%s, Memory=%s, Pods=%s\n",
				status.K8SStatus.ResourceUsage.Cpu, status.K8SStatus.ResourceUsage.Memory, status.K8SStatus.ResourceUsage.Pods)
		}
	}
}

func printUsage() {
	fmt.Println("Arena Client - Command line client for Arena Service")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  arenaclient <command> [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  create     Create a new arena")
	fmt.Println("  destroy    Destroy an existing arena")
	fmt.Println("  status     Get status of an arena")
	fmt.Println("  help       Show this help message")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  ARENA_SERVER_ADDR           Server address (default: localhost:443)")
	fmt.Println("  ARENA_INSECURE_DISABLE_MTLS Disable mTLS and use insecure connection (default: false)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  arenaclient create -public-key=abc123 -vpn-ip=10.0.0.2/32 -challenge-spec='...'")
	fmt.Println("  arenaclient destroy -public-key=abc123")
	fmt.Println("  arenaclient status -public-key=abc123")
	fmt.Println()
	fmt.Println("Insecure Mode:")
	fmt.Println("  ARENA_INSECURE_DISABLE_MTLS=true arenaclient status -public-key=abc123")
}
