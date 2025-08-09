package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"arena/internal/aibroker/config"
	"arena/internal/aibroker/identity"
	"arena/internal/aibroker/metrics"
	"arena/internal/aibroker/proxy"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := metrics.NewLogger(cfg.LogLevel)

	// Log configuration (without tokens)
    logger.Info("AI Broker starting",
        "listen_addr", cfg.ListenAddr,
        "require_identity", cfg.RequireIdentity,
        "service_account_audience", cfg.ServiceAccountAudience,
        "rate_limit_rps", cfg.RateLimitRPS,
        "rate_limit_burst", cfg.RateLimitBurst,
        "internal_host", cfg.Mapping.InternalHost,
        "upstream_url", cfg.Mapping.UpstreamURL.String(),
        "timeout", cfg.Mapping.Timeout,
        "required_headers_count", len(cfg.Mapping.RequiredHeaders),
    )

	// Initialize identity validator
	validator, err := identity.NewValidator(cfg.ServiceAccountAudience, cfg.RequireIdentity)
	if err != nil {
		logger.Error("Failed to initialize identity validator", "error", err)
		os.Exit(1)
	}

	// Initialize proxy server
	server := proxy.NewServer(cfg, validator, logger)

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: server,
		// Security timeouts
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // Long timeout for streaming responses
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server starting", "addr", cfg.ListenAddr)
		serverErrors <- httpServer.ListenAndServe()
	}()

	// Wait for interrupt signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Block until we receive an interrupt or server error
	select {
	case err := <-serverErrors:
		logger.Error("Server error", "error", err)
	case sig := <-interrupt:
		logger.Info("Received interrupt signal", "signal", sig)
	}

	// Graceful shutdown
	logger.Info("Shutting down server")
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error", "error", err)
		os.Exit(1)
	}

	logger.Info("Server stopped gracefully")
}