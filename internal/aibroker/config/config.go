package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type ProviderMapping struct {
	InternalHost         string            `json:"internal_host"`
	UpstreamURL          *url.URL          `json:"upstream_url"`
	Token                string            `json:"token"`
	RequiredHeaders      map[string]string `json:"required_headers"`
	Timeout              time.Duration     `json:"timeout"`
}

type BrokerConfig struct {
	ListenAddr             string                      `json:"listen_addr"`
    Mapping                ProviderMapping             `json:"mapping"`
	RequireIdentity        bool                        `json:"require_identity"`
	ServiceAccountAudience string                      `json:"service_account_audience"`
	RateLimitRPS           int                         `json:"rate_limit_rps"`
	RateLimitBurst         int                         `json:"rate_limit_burst"`
	LogLevel               string                      `json:"log_level"`
}

func LoadConfig() (*BrokerConfig, error) {
	config := &BrokerConfig{
		ListenAddr:             getEnv("BROKER_LISTEN_ADDR", ":8080"),
		RequireIdentity:        getBoolEnv("BROKER_REQUIRE_IDENTITY", false),
		ServiceAccountAudience: getEnv("BROKER_SA_AUDIENCE", "servicebroker"),
		RateLimitRPS:           getIntEnv("BROKER_RATE_LIMIT_RPS", 0),
		RateLimitBurst:         getIntEnv("BROKER_RATE_LIMIT_BURST", 0),
		LogLevel:               getEnv("BROKER_LOG_LEVEL", "info"),
	}

    // Dead-simple single-mapping configuration
    internalHost := getEnv("AIBROKER_INTERNAL_HOST", "aibroker")
    upstreamURLStr := getEnv("OPENAI_BASE_URL", getEnv("AIBROKER_UPSTREAM_URL", "https://api.deepseek.com/v1"))

    upstreamURL, err := url.Parse(upstreamURLStr)
    if err != nil {
        return nil, fmt.Errorf("invalid OPENAI_BASE_URL / AIBROKER_UPSTREAM_URL: %w", err)
    }
    if upstreamURL.Scheme != "https" {
        return nil, fmt.Errorf("OPENAI_BASE_URL / AIBROKER_UPSTREAM_URL must use https")
    }

    token := getEnv("OPENAI_API_KEY", getEnv("AIBROKER_TOKEN", ""))
    if token == "" {
        return nil, fmt.Errorf("OPENAI_API_KEY (or AIBROKER_TOKEN) is required")
    }

    timeout := time.Duration(getIntEnv("AIBROKER_TIMEOUT_MS", 60000)) * time.Millisecond

    requiredHeaders := make(map[string]string)
    requiredHeadersStr := getEnv("AIBROKER_REQUIRED_HEADERS", "{}")
    if requiredHeadersStr != "{}" && requiredHeadersStr != "" {
        if err := json.Unmarshal([]byte(requiredHeadersStr), &requiredHeaders); err != nil {
            return nil, fmt.Errorf("invalid AIBROKER_REQUIRED_HEADERS JSON: %w", err)
        }
    }

    config.Mapping = ProviderMapping{
        InternalHost:    internalHost,
        UpstreamURL:     upstreamURL,
        Token:           token,
        RequiredHeaders: requiredHeaders,
        Timeout:         timeout,
    }

    return config, nil
}

func (c *BrokerConfig) MatchByHost(host string) (ProviderMapping, bool) {
    // Single mapping mode: ignore host and always return the configured mapping
    return c.Mapping, true
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	
	return parsed
}

func getIntEnv(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	
	return parsed
}