package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"arena/internal/aibroker/config"
	"arena/internal/aibroker/identity"
	"arena/internal/aibroker/metrics"
	"arena/internal/aibroker/policy"
)

type Server struct {
	config     *config.BrokerConfig
	validator  *identity.Validator
	limiter    *policy.IPLimiter
	logger     *metrics.Logger
	proxy      *httputil.ReverseProxy
}

func NewServer(cfg *config.BrokerConfig, validator *identity.Validator, logger *metrics.Logger) *Server {
	limiter := policy.NewIPLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)
	
	server := &Server{
		config:    cfg,
		validator: validator,
		limiter:   limiter,
		logger:    logger,
	}

	// Create reverse proxy with custom transport
	transport := &http.Transport{
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	server.proxy = &httputil.ReverseProxy{
		Director:       server.director,
		Transport:      transport,
		ErrorHandler:   server.errorHandler,
		ModifyResponse: server.modifyResponse,
		FlushInterval:  50 * time.Millisecond, // For streaming support
	}

	return server
}

func (s *Server) director(req *http.Request) {
	// Get the mapping based on the Host header
    mapping, _ := s.config.MatchByHost(req.Host)

	// Set the mapping in the request context for later use
	ctx := context.WithValue(req.Context(), "mapping", mapping)
	*req = *req.WithContext(ctx)

	// Update the request URL to point to the upstream
	req.URL.Scheme = mapping.UpstreamURL.Scheme
	req.URL.Host = mapping.UpstreamURL.Host
	req.Host = mapping.UpstreamURL.Host

	// Remove hop-by-hop headers
	removeHopByHopHeaders(req.Header)

	// Remove any existing Authorization header from client
	req.Header.Del("Authorization")

	// Add the provider's authorization token
	req.Header.Set("Authorization", "Bearer "+mapping.Token)

	// Add required headers from mapping
	for key, value := range mapping.RequiredHeaders {
		req.Header.Set(key, value)
	}

	// Apply request timeout
	ctx, cancel := context.WithTimeout(req.Context(), mapping.Timeout)
	*req = *req.WithContext(ctx)
	
	// Store the cancel function for cleanup (will be called by the transport)
	req.Header.Set("X-Timeout-Cancel", fmt.Sprintf("%p", cancel))
}

func (s *Server) modifyResponse(resp *http.Response) error {
	// Optionally strip server header for security
	resp.Header.Del("Server")

	// Record metrics for error responses
	if mapping, ok := resp.Request.Context().Value("mapping").(config.ProviderMapping); ok {
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			metrics.RecordRequest(mapping.InternalHost, resp.StatusCode, 0)
		}
	}

	return nil
}

func (s *Server) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.LogRequest("unknown", r.Method, r.URL.Path, 502, 0, err)
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	rw := metrics.NewResponseWriter(w)

	// Handle special endpoints
	switch r.URL.Path {
	case "/healthz", "/readyz":
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("OK"))
		return
	case "/metrics":
		promhttp.Handler().ServeHTTP(rw, r)
		return
	}

	// Get mapping for this request
    mapping, _ := s.config.MatchByHost(r.Host)

	// Validate path against allowed prefixes
    // In simple mode, all paths are allowed

	// Identity validation
	if err := s.validator.ValidateRequest(r); err != nil {
		s.logger.LogRequest(mapping.InternalHost, r.Method, r.URL.Path, 401, time.Since(startTime), err)
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Rate limiting
	clientIP := getClientIP(r)
	if !s.limiter.AllowRequest(clientIP) {
		s.logger.LogRequest(mapping.InternalHost, r.Method, r.URL.Path, 429, time.Since(startTime), fmt.Errorf("rate limit exceeded"))
		http.Error(rw, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// Store start time in context for metrics
	ctx := context.WithValue(r.Context(), "start_time", startTime)
	r = r.WithContext(ctx)

	// Proxy the request
	s.proxy.ServeHTTP(rw, r)

	// Log and record metrics
	latency := time.Since(startTime)
	s.logger.LogRequest(mapping.InternalHost, r.Method, r.URL.Path, rw.StatusCode(), latency, nil)
	metrics.RecordRequest(mapping.InternalHost, rw.StatusCode(), latency)
}

func getClientIP(r *http.Request) net.IP {
	// Try X-Forwarded-For header first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if parsed := net.ParseIP(ip); parsed != nil {
				return parsed
			}
		}
	}

	// Try X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		if parsed := net.ParseIP(xri); parsed != nil {
			return parsed
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP("127.0.0.1")
	}
	
	return net.ParseIP(host)
}

// removeHopByHopHeaders removes headers that should not be forwarded
func removeHopByHopHeaders(headers http.Header) {
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}

	for _, header := range hopByHopHeaders {
		headers.Del(header)
	}
}