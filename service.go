// HomeLink - Core Service Implementation
// Local Device Discovery and Communication Protocol
// Copyright (c) 2025 - Open Source Project
// A privacy-first, self-hosted alternative for home device communication

package homelink

import (
	"fmt"
	"log"
	"net"
	"sync"
)

// HomeLinkService manages the entire HomeLink protocol
type HomeLinkService struct {
	deviceID     string
	deviceName   string
	capabilities []string

	// Network components
	multicastConn *net.UDPConn
	unicastConn   *net.UDPConn

	// State management
	devices       map[string]*Device
	subscriptions map[string][]string // deviceID -> list of event types they want
	mutex         sync.RWMutex

	// Security components
	security       *SecurityManager
	securityConfig *SecurityConfig
	rateLimiter    *RateLimiter

	// Channels for internal communication
	eventChan chan Event
	stopChan  chan bool
}

// NewHomeLinkService creates a new HomeLink protocol service
func NewHomeLinkService(deviceID, deviceName string, capabilities []string) *HomeLinkService {
	return &HomeLinkService{
		deviceID:      deviceID,
		deviceName:    deviceName,
		capabilities:  capabilities,
		devices:       make(map[string]*Device),
		subscriptions: make(map[string][]string),
		eventChan:     make(chan Event, 100), // Buffer for events
		stopChan:      make(chan bool),
	}
}

// NewSecureHomeLinkService creates a new HomeLink service with security enabled
func NewSecureHomeLinkService(deviceID, deviceName string, capabilities []string, config *SecurityConfig) (*HomeLinkService, error) {
	service := &HomeLinkService{
		deviceID:       deviceID,
		deviceName:     deviceName,
		capabilities:   capabilities,
		devices:        make(map[string]*Device),
		subscriptions:  make(map[string][]string),
		securityConfig: config,
		eventChan:      make(chan Event, 100),
		stopChan:       make(chan bool),
	}

	if config.Enabled {
		// Initialize security manager
		security, err := NewSecurityManager(deviceID)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize security: %v", err)
		}
		service.security = security

		// Set network key if provided
		if config.AllowedNetworkKey != "" {
			if err := security.SetNetworkKey(config.AllowedNetworkKey); err != nil {
				return nil, fmt.Errorf("failed to set network key: %v", err)
			}
		}

		// Initialize rate limiter
		service.rateLimiter = NewRateLimiter(config.RateLimitPerSecond, config.RateLimitBurstSize)

		log.Printf("Security enabled for device %s", deviceID)
		log.Printf("  Authentication required: %t", config.RequireAuthentication)
		log.Printf("  Rate limit: %d msg/sec (burst: %d)", config.RateLimitPerSecond, config.RateLimitBurstSize)
	}

	return service, nil
}

// Start initializes and starts the HomeLink service
func (s *HomeLinkService) Start() error {
	log.Printf("Starting HomeLink Service for %s (%s)", s.deviceName, s.deviceID)

	// Set up multicast connection for discovery
	if err := s.setupMulticast(); err != nil {
		return fmt.Errorf("failed to setup multicast: %v", err)
	}

	// Set up unicast connection for direct messages
	if err := s.setupUnicast(); err != nil {
		return fmt.Errorf("failed to setup unicast: %v", err)
	}

	// Start background routines
	go s.listenForMessages()
	go s.processEvents()
	go s.sendHeartbeats()
	go s.sendPeriodicAnnouncements()

	// Request other devices to announce themselves to us
	s.requestDiscovery()

	// Announce ourselves to the network
	s.announceDevice()

	log.Printf("HomeLink Service started successfully")
	return nil
}

// Stop gracefully shuts down the HomeLink service
func (s *HomeLinkService) Stop() {
	log.Printf("Stopping HomeLink Service")
	close(s.stopChan)

	if s.multicastConn != nil {
		s.multicastConn.Close()
	}
	if s.unicastConn != nil {
		s.unicastConn.Close()
	}
}

// Security-related methods

// IsSecurityEnabled returns whether security is enabled for this service
func (s *HomeLinkService) IsSecurityEnabled() bool {
	return s.security != nil && s.securityConfig.Enabled
}

// GetPublicKey returns the device's public key (if security is enabled)
func (s *HomeLinkService) GetPublicKey() string {
	if s.security != nil {
		return s.security.GetPublicKey()
	}
	return ""
}

// GetNetworkKey returns the network key for initial pairing (if security is enabled)
func (s *HomeLinkService) GetNetworkKey() string {
	if s.security != nil {
		return s.security.GetNetworkKey()
	}
	return ""
}

// AddTrustedDevice adds a device to the trusted list
func (s *HomeLinkService) AddTrustedDevice(deviceID, publicKey string) error {
	if s.security == nil {
		return fmt.Errorf("security not enabled")
	}
	return s.security.AddTrustedDevice(deviceID, publicKey)
}

// RemoveTrustedDevice removes a device from the trusted list
func (s *HomeLinkService) RemoveTrustedDevice(deviceID string) {
	if s.security != nil {
		s.security.RemoveTrustedDevice(deviceID)
		
		// Also remove from devices list if present
		s.mutex.Lock()
		delete(s.devices, deviceID)
		delete(s.subscriptions, deviceID)
		s.mutex.Unlock()
		
		log.Printf("Removed untrusted device: %s", deviceID)
	}
}

// GetTrustedDevices returns a list of trusted device IDs
func (s *HomeLinkService) GetTrustedDevices() []string {
	if s.security != nil {
		return s.security.GetTrustedDevices()
	}
	return []string{}
}

// GenerateQRPairingCode generates a QR code for device pairing
func (s *HomeLinkService) GenerateQRPairingCode() (string, error) {
	if s.security == nil {
		return "", fmt.Errorf("security not enabled")
	}
	return s.security.GenerateQRPairingCode(s.deviceName)
}

// ParseQRPairingCode parses a QR pairing code and adds the device if valid
func (s *HomeLinkService) ParseQRPairingCode(qrCode string) error {
	if s.security == nil {
		return fmt.Errorf("security not enabled")
	}

	pairingData, err := s.security.ParseQRPairingCode(qrCode)
	if err != nil {
		return fmt.Errorf("invalid QR pairing code: %v", err)
	}

	// Extract device information
	deviceID := pairingData["device_id"]
	deviceName := pairingData["device_name"]
	publicKey := pairingData["public_key"]
	networkKey := pairingData["network_key"]

	// Set network key if we don't have one
	if s.security.GetNetworkKey() == "" {
		if err := s.security.SetNetworkKey(networkKey); err != nil {
			return fmt.Errorf("failed to set network key: %v", err)
		}
	}

	// Add the device as trusted
	if err := s.security.AddTrustedDevice(deviceID, publicKey); err != nil {
		return fmt.Errorf("failed to add trusted device: %v", err)
	}

	log.Printf("Successfully paired with device: %s (%s)", deviceName, deviceID)
	return nil
}

// GetSecurityStats returns security-related statistics
func (s *HomeLinkService) GetSecurityStats() map[string]interface{} {
	stats := map[string]interface{}{
		"security_enabled": s.IsSecurityEnabled(),
	}

	if s.security != nil {
		stats["trusted_devices"] = s.security.GetTrustedDevices()
		stats["require_authentication"] = s.securityConfig.RequireAuthentication
		stats["auto_accept_pairing"] = s.securityConfig.AutoAcceptPairing
	}

	if s.rateLimiter != nil {
		stats["rate_limiting"] = map[string]interface{}{
			"max_rate":    s.securityConfig.RateLimitPerSecond,
			"burst_size":  s.securityConfig.RateLimitBurstSize,
			"device_rates": s.rateLimiter.GetAllDeviceRates(),
		}
	}

	return stats
}
