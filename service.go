// HomeLink - Core Service Implementation
// Local Device Discovery and Communication Protocol
// Copyright (c) 2025 - Open Source Project
// A privacy-first, self-hosted alternative for home device communication

package homelink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
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

	// Performance and reliability components
	healthMonitor      *HealthMonitor
	reliabilityManager *ReliabilityManager
	binaryEncoder      *BinaryEncoder
	binaryDecoder      *BinaryDecoder
	protocolMode       ProtocolMode

	// Channels for internal communication
	eventChan chan Event
	stopChan  chan bool
}

// NewHomeLinkService creates a new HomeLink protocol service
func NewHomeLinkService(deviceID, deviceName string, capabilities []string) *HomeLinkService {
	service := &HomeLinkService{
		deviceID:      deviceID,
		deviceName:    deviceName,
		capabilities:  capabilities,
		devices:       make(map[string]*Device),
		subscriptions: make(map[string][]string),
		eventChan:     make(chan Event, 100), // Buffer for events
		stopChan:      make(chan bool),
		protocolMode:  ProtocolJSON, // Default to JSON
	}

	// Initialize health monitoring
	service.healthMonitor = NewHealthMonitor(deviceID)
	
	// Initialize reliability manager
	service.reliabilityManager = NewReliabilityManager(deviceID)
	
	// Initialize binary protocol components
	service.binaryEncoder = NewBinaryEncoder(deviceID)
	service.binaryDecoder = NewBinaryDecoder()

	return service
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
		protocolMode:   ProtocolJSON, // Default to JSON
	}

	// Initialize health monitoring
	service.healthMonitor = NewHealthMonitor(deviceID)
	
	// Initialize reliability manager
	service.reliabilityManager = NewReliabilityManager(deviceID)
	
	// Initialize binary protocol components
	service.binaryEncoder = NewBinaryEncoder(deviceID)
	service.binaryDecoder = NewBinaryDecoder()

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

	// Stop reliability manager
	if s.reliabilityManager != nil {
		s.reliabilityManager.Stop()
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

// New feature methods

// GetHealthMonitor returns the health monitor instance
func (s *HomeLinkService) GetHealthMonitor() *HealthMonitor {
	return s.healthMonitor
}

// GetReliabilityManager returns the reliability manager instance
func (s *HomeLinkService) GetReliabilityManager() *ReliabilityManager {
	return s.reliabilityManager
}

// SetProtocolMode sets the protocol mode for communication
func (s *HomeLinkService) SetProtocolMode(mode ProtocolMode) {
	s.protocolMode = mode
	log.Printf("Protocol mode changed to: %d", mode)
}

// GetProtocolMode returns the current protocol mode
func (s *HomeLinkService) GetProtocolMode() ProtocolMode {
	return s.protocolMode
}

// SendReliableEvent sends an event with reliability guarantees
func (s *HomeLinkService) SendReliableEvent(eventType, description string, data map[string]string, priority MessagePriority) {
	msg := Message{
		Type:      MSG_EVENT,
		Version:   PROTOCOL_VERSION,
		Timestamp: time.Now().Unix(),
		DeviceID:  s.deviceID,
		Data: map[string]interface{}{
			"event_type":  eventType,
			"description": description,
			"data":        data,
		},
	}

	// Send as reliable message to all devices
	devices := s.GetDevices()
	for _, device := range devices {
		reliableMsg := s.reliabilityManager.SendReliableMessage(
			&msg,
			device.ID,
			priority,
			DeliveryReliable,
			func(ack *MessageAck) {
				log.Printf("Event acknowledged by %s: %s", ack.RecipientID, ack.Status)
			},
			func() {
				log.Printf("Event delivery timed out for device %s", device.ID)
				if s.healthMonitor != nil {
					s.healthMonitor.IncrementMessagesFailed()
				}
			},
		)

		// Send via appropriate protocol
		s.sendReliableMessage(reliableMsg, device.Address)
	}
}

// sendReliableMessage sends a reliable message using the configured protocol
func (s *HomeLinkService) sendReliableMessage(reliableMsg *ReliableMessage, addr *net.UDPAddr) error {
	var data []byte
	var err error

	switch s.protocolMode {
	case ProtocolBinary:
		binaryMsg, binaryErr := s.binaryEncoder.EncodeReliableMessage(reliableMsg)
		if binaryErr != nil {
			return fmt.Errorf("failed to encode binary message: %v", binaryErr)
		}
		
		// Convert binary message to bytes
		buf := make([]byte, 0, binaryMsg.GetSize())
		writer := bytes.NewBuffer(buf)
		_, err = binaryMsg.WriteTo(writer)
		data = writer.Bytes()

	default: // JSON protocol
		if s.IsSecurityEnabled() {
			secureMsg, encErr := s.security.EncryptMessage(&reliableMsg.Message)
			if encErr != nil {
				return fmt.Errorf("failed to encrypt message: %v", encErr)
			}
			data, err = json.Marshal(secureMsg)
		} else {
			data, err = json.Marshal(reliableMsg)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	// Send via UDP
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to create connection: %v", err)
	}
	defer conn.Close()

	n, err := conn.Write(data)
	if err != nil {
		if s.healthMonitor != nil {
			s.healthMonitor.IncrementMessagesFailed()
		}
		return err
	}

	if s.healthMonitor != nil {
		s.healthMonitor.IncrementMessagesSent(uint64(n))
	}

	return nil
}

// UpdateHealthMetrics updates the health monitoring data
func (s *HomeLinkService) UpdateHealthMetrics() {
	if s.healthMonitor != nil {
		s.healthMonitor.UpdateMetrics(s)
	}
}

// GetHealthSummary returns a summary of system health
func (s *HomeLinkService) GetHealthSummary() map[string]interface{} {
	if s.healthMonitor != nil {
		return s.healthMonitor.GetHealthSummary()
	}
	return map[string]interface{}{
		"status": "health monitoring disabled",
	}
}
