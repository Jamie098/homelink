// HomeLink - Message and Data Types
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"net"
	"sync"
	"time"
)

// Core message structure - all messages follow this format
type Message struct {
	Type      MessageType `json:"type"`
	Version   string      `json:"version"`
	Timestamp int64       `json:"timestamp"`
	DeviceID  string      `json:"device_id"`
	Data      interface{} `json:"data"`
}

// Device represents a device on the network
type Device struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Capabilities []string          `json:"capabilities"`
	Address      *net.UDPAddr      `json:"-"` // Don't serialize network address
	LastSeen     time.Time         `json:"last_seen"`
	Metadata     map[string]string `json:"metadata"`
	PublicKey    string            `json:"public_key,omitempty"` // Base64 encoded Ed25519 public key
	Trusted      bool              `json:"trusted"`              // Whether this device is trusted
	AuthTime     time.Time         `json:"auth_time,omitempty"`  // When device was authenticated
}

// Subscription represents what events a device wants to receive
type Subscription struct {
	DeviceID   string   `json:"device_id"`
	EventTypes []string `json:"event_types"`
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	Enabled                bool          `json:"enabled"`
	RequireAuthentication  bool          `json:"require_authentication"`
	AllowedNetworkKey      string        `json:"allowed_network_key,omitempty"`
	MaxMessageAge          time.Duration `json:"max_message_age"`
	RateLimitPerSecond     int           `json:"rate_limit_per_second"`
	RateLimitBurstSize     int           `json:"rate_limit_burst_size"`
	AutoAcceptPairing      bool          `json:"auto_accept_pairing"`
	RequireDeviceApproval  bool          `json:"require_device_approval"`
}

// RateLimiter tracks message rates from devices
type RateLimiter struct {
	deviceRates map[string]*DeviceRateInfo
	mutex       sync.RWMutex
	maxRate     int
	burstSize   int
}

// DeviceRateInfo tracks rate limiting info for a specific device
type DeviceRateInfo struct {
	tokens    int
	lastRefill time.Time
}

// AuthenticationResult represents the result of device authentication
type AuthenticationResult struct {
	Authenticated bool
	DeviceID      string
	PublicKey     string
	Error         error
}

// ProtocolMode defines which protocol to use for communication
type ProtocolMode int

const (
	ProtocolJSON   ProtocolMode = iota // JSON over UDP (default)
	ProtocolBinary                     // Binary protocol for performance
	ProtocolAuto                       // Automatically choose based on peer capabilities
)
