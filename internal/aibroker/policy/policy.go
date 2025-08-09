package policy

import (
    "net"
    "sync"
    "time"

    "golang.org/x/time/rate"
)

// IsPathAllowed is a no-op in simple mode; all paths are allowed.
func IsPathAllowed(_ interface{}, _ string) bool { return true }

type IPLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rps      int
	burst    int
}

func NewIPLimiter(rps, burst int) *IPLimiter {
	return &IPLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rps,
		burst:    burst,
	}
}

func (l *IPLimiter) AllowRequest(ip net.IP) bool {
	// If no rate limiting is configured, allow all requests
	if l.rps == 0 {
		return true
	}

	ipStr := ip.String()
	
	l.mu.RLock()
	limiter, exists := l.limiters[ipStr]
	l.mu.RUnlock()
	
	if !exists {
		l.mu.Lock()
		// Double-check after acquiring write lock
		limiter, exists = l.limiters[ipStr]
		if !exists {
			limiter = rate.NewLimiter(rate.Limit(l.rps), l.burst)
			l.limiters[ipStr] = limiter
		}
		l.mu.Unlock()
	}
	
	return limiter.Allow()
}

// CleanupExpiredLimiters removes limiters that haven't been used recently
// This should be called periodically to prevent memory leaks
func (l *IPLimiter) CleanupExpiredLimiters() {
	if l.rps == 0 {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	
	now := time.Now()
	for ip, limiter := range l.limiters {
		// Remove limiters that have full tokens (haven't been used recently)
		if limiter.TokensAt(now) >= float64(l.burst) {
			delete(l.limiters, ip)
		}
	}
}