package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the Frigate-HomeLink bridge
type Config struct {
	// Frigate configuration
	FrigateURL     string        `json:"frigate_url"`
	FrigateTimeout time.Duration `json:"frigate_timeout"`

	// HomeLink configuration
	HomeLinkURL     string        `json:"homelink_url"`
	HomeLinkAPIKey  string        `json:"homelink_api_key"`
	HomeLinkTimeout time.Duration `json:"homelink_timeout"`

	// Bridge configuration
	BridgeID     string        `json:"bridge_id"`
	PollInterval time.Duration `json:"poll_interval"`

	// Event filtering
	EnabledCameras    []string `json:"enabled_cameras"`
	EnabledEventTypes []string `json:"enabled_event_types"`
	MinConfidence     float64  `json:"min_confidence"`

	// Advanced options
	MaxEventAge     time.Duration `json:"max_event_age"`
	EventBuffer     int           `json:"event_buffer"`
	RetryAttempts   int           `json:"retry_attempts"`
	RetryDelay      time.Duration `json:"retry_delay"`
	LogLevel        string        `json:"log_level"`
	HealthCheckPort int           `json:"health_check_port"`
}

// NewConfig creates a new configuration from environment variables
func NewConfig() *Config {
	return &Config{
		// Frigate settings
		FrigateURL:     getEnv("FRIGATE_URL", "http://localhost:5000"),
		FrigateTimeout: parseDuration("FRIGATE_TIMEOUT", 30*time.Second),

		// HomeLink settings
		HomeLinkURL:     getEnv("HOMELINK_URL", "http://localhost:8081"),
		HomeLinkAPIKey:  getEnv("HOMELINK_API_KEY", ""),
		HomeLinkTimeout: parseDuration("HOMELINK_TIMEOUT", 30*time.Second),

		// Bridge settings
		BridgeID:     getEnv("BRIDGE_ID", "frigate-bridge"),
		PollInterval: parseDuration("POLL_INTERVAL", 5*time.Second),

		// Event filtering
		EnabledCameras:    parseStringSlice("ENABLED_CAMERAS", []string{}),
		EnabledEventTypes: parseStringSlice("ENABLED_EVENT_TYPES", []string{"person", "car", "bicycle", "dog", "cat"}),
		MinConfidence:     parseFloat("MIN_CONFIDENCE", 0.7),

		// Advanced options
		MaxEventAge:     parseDuration("MAX_EVENT_AGE", 10*time.Minute),
		EventBuffer:     parseInt("EVENT_BUFFER", 100),
		RetryAttempts:   parseInt("RETRY_ATTEMPTS", 3),
		RetryDelay:      parseDuration("RETRY_DELAY", 5*time.Second),
		LogLevel:        getEnv("LOG_LEVEL", "INFO"),
		HealthCheckPort: parseInt("HEALTH_CHECK_PORT", 8082),
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.FrigateURL == "" {
		return fmt.Errorf("FRIGATE_URL is required")
	}

	if c.HomeLinkURL == "" {
		return fmt.Errorf("HOMELINK_URL is required")
	}

	if c.BridgeID == "" {
		return fmt.Errorf("BRIDGE_ID is required")
	}

	if c.PollInterval < time.Second {
		return fmt.Errorf("POLL_INTERVAL must be at least 1 second")
	}

	if c.MinConfidence < 0 || c.MinConfidence > 1 {
		return fmt.Errorf("MIN_CONFIDENCE must be between 0 and 1")
	}

	if c.MaxEventAge < time.Minute {
		return fmt.Errorf("MAX_EVENT_AGE must be at least 1 minute")
	}

	if c.EventBuffer < 1 {
		return fmt.Errorf("EVENT_BUFFER must be at least 1")
	}

	if c.RetryAttempts < 0 {
		return fmt.Errorf("RETRY_ATTEMPTS must be >= 0")
	}

	return nil
}

// Helper functions for parsing environment variables

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseStringSlice(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	// Simple comma-separated parsing
	var result []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}

	if len(result) == 0 {
		return defaultValue
	}

	return result
}
