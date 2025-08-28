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

	// Advanced integration features
	filteringSystem     *FilteringSystem
	notificationService *NotificationService
	eventStorage        *EventStorage

	// Utility components
	messageFactory  *MessageFactory
	errorHandler    *ErrorHandler
	networkHelper   *NetworkHelper
	configValidator *ConfigValidator

	// Channels for internal communication
	eventChan chan Event
	stopChan  chan bool
}

// NewHomeLinkService creates a new HomeLink protocol service
func NewHomeLinkService(deviceID, deviceName string, capabilities []string) *HomeLinkService {
	service := &HomeLinkService{
		deviceID:        deviceID,
		deviceName:      deviceName,
		capabilities:    capabilities,
		devices:         make(map[string]*Device),
		subscriptions:   make(map[string][]string),
		eventChan:       make(chan Event, 100), // Buffer for events
		stopChan:        make(chan bool),
		protocolMode:    ProtocolJSON, // Default to JSON
		messageFactory:  NewMessageFactory(deviceID),
		errorHandler:    NewErrorHandler(deviceID),
		configValidator: NewConfigValidator(),
	}

	// Initialize utility components that depend on error handler
	service.networkHelper = NewNetworkHelper(service.errorHandler)

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
		deviceID:        deviceID,
		deviceName:      deviceName,
		capabilities:    capabilities,
		devices:         make(map[string]*Device),
		subscriptions:   make(map[string][]string),
		securityConfig:  config,
		eventChan:       make(chan Event, 100),
		stopChan:        make(chan bool),
		protocolMode:    ProtocolJSON, // Default to JSON
		messageFactory:  NewMessageFactory(deviceID),
		errorHandler:    NewErrorHandler(deviceID),
		configValidator: NewConfigValidator(),
	}

	// Initialize utility components that depend on error handler
	service.networkHelper = NewNetworkHelper(service.errorHandler)

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

// NewAdvancedHomeLinkService creates a service with all advanced features
func NewAdvancedHomeLinkService(deviceID, deviceName string, capabilities []string,
	securityConfig *SecurityConfig,
	storageConfig *StorageConfig) (*HomeLinkService, error) {

	// Start with secure service if security enabled
	var service *HomeLinkService
	var err error

	if securityConfig != nil && securityConfig.Enabled {
		service, err = NewSecureHomeLinkService(deviceID, deviceName, capabilities, securityConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create secure service: %v", err)
		}
	} else {
		service = NewHomeLinkService(deviceID, deviceName, capabilities)
	}

	// Initialize advanced filtering system
	service.filteringSystem = NewFilteringSystem(5*time.Minute, 10000) // 5 min dedup window, 10k max entries
	log.Printf("Event filtering and deduplication system initialized")

	// Initialize event storage if configured
	if storageConfig != nil && storageConfig.Enabled {
		service.eventStorage, err = NewEventStorage(*storageConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize storage: %v", err)
		}
		log.Printf("Event persistence and storage initialized: %s", storageConfig.DatabasePath)
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

	// Stop event storage
	if s.eventStorage != nil {
		s.eventStorage.Stop()
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
			"max_rate":     s.securityConfig.RateLimitPerSecond,
			"burst_size":   s.securityConfig.RateLimitBurstSize,
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

// Advanced Features - Event Processing Pipeline

// PublishEvent publishes an event through the complete processing pipeline
func (s *HomeLinkService) PublishEvent(eventType MessageType, data interface{}) error {
	return s.PublishEventWithPriority(eventType, data, PriorityNormal)
}

// PublishEventWithPriority publishes an event with specified priority
func (s *HomeLinkService) PublishEventWithPriority(eventType MessageType, data interface{}, priority MessagePriority) error {
	// Create message
	msg := &Message{
		Type:      eventType,
		DeviceID:  s.deviceID,
		Timestamp: time.Now().Unix(),
		Data:      data,
	}

	// Process through filtering system
	if s.filteringSystem != nil {
		filterResult := s.filteringSystem.ProcessEvent(msg)

		switch filterResult.Action {
		case "deny":
			log.Printf("Event filtered out: %s (reason: %s)", eventType, filterResult.Reason)
			return nil
		case "modify":
			// Apply modifications to message
			if filterResult.Modified && len(filterResult.NewData) > 0 {
				// Merge modifications into message data
				if dataMap, ok := msg.Data.(map[string]interface{}); ok {
					for k, v := range filterResult.NewData {
						dataMap[k] = v
					}
				}
			}
		}
	}

	// Store event in database
	if s.eventStorage != nil {
		if err := s.eventStorage.StoreEvent(msg); err != nil {
			log.Printf("Failed to store event: %v", err)
		}
	}

	// Send to network using reliability manager for important events
	if priority >= PriorityHigh {
		dataMap, ok := data.(map[string]string)
		if !ok {
			dataMap = map[string]string{}
		}
		s.SendReliableEvent(string(eventType), "", dataMap, priority)
		return nil
	}

	// Normal network broadcast
	return s.broadcastMessage(Message{
		Type:      eventType,
		DeviceID:  s.deviceID,
		Data:      data,
		Timestamp: time.Now().Unix(),
		Version:   PROTOCOL_VERSION,
	})
}

// ProcessIncomingEvent processes events received from other devices
func (s *HomeLinkService) ProcessIncomingEvent(msg *Message, addr *net.UDPAddr) {
	// Apply filtering system to incoming events too
	if s.filteringSystem != nil {
		filterResult := s.filteringSystem.ProcessEvent(msg)
		if filterResult.Action == "deny" {
			return // Filtered out
		}
	}

	// Update device information
	s.updateDeviceFromMessage(msg, addr)

	// Store incoming event if enabled (optional - may want to filter external events)
	if s.eventStorage != nil {
		if err := s.eventStorage.StoreEvent(msg); err != nil {
			log.Printf("Failed to store incoming event: %v", err)
		}
	}
}

// updateDeviceFromMessage updates device info from received message
func (s *HomeLinkService) updateDeviceFromMessage(msg *Message, addr *net.UDPAddr) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if device, exists := s.devices[msg.DeviceID]; exists {
		device.LastSeen = time.Now()
		if addr != nil {
			device.Address = addr
		}
	} else {
		// Create new device entry
		device := &Device{
			ID:       msg.DeviceID,
			Name:     msg.DeviceID, // Will be updated when we get device info
			LastSeen: time.Now(),
		}
		if addr != nil {
			device.Address = addr
		}
		s.devices[msg.DeviceID] = device
	}
}

// Advanced Integration Management Methods

// AddEventFilter adds a new event filter to the filtering system
func (s *HomeLinkService) AddEventFilter(filter EventFilter) error {
	if s.filteringSystem == nil {
		return fmt.Errorf("filtering system not initialized")
	}
	return s.filteringSystem.AddFilter(filter)
}

// RemoveEventFilter removes an event filter by name
func (s *HomeLinkService) RemoveEventFilter(name string) bool {
	if s.filteringSystem == nil {
		return false
	}
	return s.filteringSystem.RemoveFilter(name)
}

// GetFilteringStats returns event filtering statistics
func (s *HomeLinkService) GetFilteringStats() FilteringStats {
	if s.filteringSystem == nil {
		return FilteringStats{}
	}
	return s.filteringSystem.GetStats()
}

// GetNotificationStats returns notification service statistics
func (s *HomeLinkService) GetNotificationStats() NotificationStats {
	if s.notificationService == nil {
		return NotificationStats{}
	}
	return s.notificationService.GetStats()
}

// GetDeviceID returns the device ID
func (s *HomeLinkService) GetDeviceID() string {
	return s.deviceID
}

// SendTestNotification sends a test notification through the notification service
func (s *HomeLinkService) SendTestNotification(msg *Message) error {
	if s.notificationService == nil {
		return fmt.Errorf("notification service not initialized")
	}
	s.notificationService.SendNotification(msg, "test", s.deviceName)
	return nil
}

// Storage-related methods

// QueryStoredEvents retrieves events from storage based on query criteria
func (s *HomeLinkService) QueryStoredEvents(query EventQuery) ([]StoredEvent, error) {
	if s.eventStorage == nil {
		return []StoredEvent{}, fmt.Errorf("event storage not initialized")
	}
	return s.eventStorage.QueryEvents(query)
}

// GetEventAggregates returns aggregated event statistics from storage
func (s *HomeLinkService) GetEventAggregates(startTime, endTime time.Time) ([]EventAggregate, error) {
	if s.eventStorage == nil {
		return []EventAggregate{}, fmt.Errorf("event storage not initialized")
	}
	return s.eventStorage.GetEventAggregates(startTime, endTime)
}

// GetStorageStats returns storage system statistics
func (s *HomeLinkService) GetStorageStats() StorageStats {
	if s.eventStorage == nil {
		return StorageStats{}
	}
	return s.eventStorage.GetStats()
}

// IsStorageEnabled returns whether event storage is enabled
func (s *HomeLinkService) IsStorageEnabled() bool {
	return s.eventStorage != nil && s.eventStorage.IsEnabled()
}
