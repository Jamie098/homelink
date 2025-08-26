// HomeLink - Local Device Discovery and Communication Protocol
// Copyright (c) 2025 - Open Source Project
// A privacy-first, self-hosted alternative for home device communication

package homelink

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Protocol Constants
const (
	HOMELINK_PORT    = 8080              // Standard HomeLink port
	BROADCAST_ADDR   = "255.255.255.255" // Network broadcast address
	PROTOCOL_VERSION = "1.0"
	PROJECT_NAME     = "HomeLink"
)

// Message Types - these define what kinds of messages we support
type MessageType string

const (
	MSG_DEVICE_ANNOUNCEMENT MessageType = "device_announcement"
	MSG_SUBSCRIBE           MessageType = "subscribe"
	MSG_EVENT               MessageType = "event"
	MSG_HEARTBEAT           MessageType = "heartbeat"
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
}

// Event represents something that happened (like motion detection)
type Event struct {
	EventType   string            `json:"event_type"`
	Source      string            `json:"source"`
	Description string            `json:"description"`
	Data        map[string]string `json:"data"`
}

// Subscription represents what events a device wants to receive
type Subscription struct {
	DeviceID   string   `json:"device_id"`
	EventTypes []string `json:"event_types"`
}

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

	// Announce ourselves to the network
	s.announceDevice()

	log.Printf("HomeLink Service started successfully")
	return nil
}

// setupMulticast configures network broadcasting for device discovery
func (s *HomeLinkService) setupMulticast() error {
	// Listen on the standard HomeLink port
	addr := &net.UDPAddr{
		IP:   net.IPv4zero, // Listen on all interfaces (0.0.0.0)
		Port: HOMELINK_PORT,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed to bind to HomeLink port %d: %v (is another HomeLink service running on this machine?)", HOMELINK_PORT, err)
	}

	log.Printf("HomeLink listening on port %d (all network interfaces)", HOMELINK_PORT)
	s.multicastConn = conn
	return nil
}

// setupUnicast configures unicast for direct device communication
func (s *HomeLinkService) setupUnicast() error {
	// Listen on any available port for direct messages
	addr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	s.unicastConn = conn
	log.Printf("Unicast listener started on %s", conn.LocalAddr().String())
	return nil
}

// listenForMessages handles incoming network messages
func (s *HomeLinkService) listenForMessages() {
	buffer := make([]byte, 4096)

	for {
		select {
		case <-s.stopChan:
			return
		default:
			// Set a read timeout so we don't block forever
			s.multicastConn.SetReadDeadline(time.Now().Add(1 * time.Second))

			// Listen on multicast connection
			n, addr, err := s.multicastConn.ReadFromUDP(buffer)
			if err != nil {
				// Check if it's a timeout (normal) vs real error
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout is normal, just try again
				}
				log.Printf("Error reading multicast message: %v", err)
				continue
			}

			log.Printf("Received %d bytes from %s", n, addr.String())
			s.handleMessage(buffer[:n], addr)
		}
	}
}

// handleMessage processes incoming messages
func (s *HomeLinkService) handleMessage(data []byte, addr *net.UDPAddr) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Failed to parse message: %v", err)
		return
	}

	// Ignore our own messages
	if msg.DeviceID == s.deviceID {
		return
	}

	log.Printf("Received %s from %s", msg.Type, msg.DeviceID)

	switch msg.Type {
	case MSG_DEVICE_ANNOUNCEMENT:
		s.handleDeviceAnnouncement(&msg, addr)
	case MSG_SUBSCRIBE:
		s.handleSubscription(&msg)
	case MSG_EVENT:
		s.handleEvent(&msg)
	case MSG_HEARTBEAT:
		s.handleHeartbeat(&msg, addr)
	}
}

// handleDeviceAnnouncement processes device announcements
func (s *HomeLinkService) handleDeviceAnnouncement(msg *Message, addr *net.UDPAddr) {
	// Parse device data from message
	deviceData, ok := msg.Data.(map[string]interface{})
	if !ok {
		log.Printf("Invalid device announcement data")
		return
	}

	// Create or update device
	s.mutex.Lock()
	defer s.mutex.Unlock()

	device := &Device{
		ID:       msg.DeviceID,
		Address:  addr,
		LastSeen: time.Now(),
		Metadata: make(map[string]string),
	}

	// Extract device information
	if name, ok := deviceData["name"].(string); ok {
		device.Name = name
	}

	if caps, ok := deviceData["capabilities"].([]interface{}); ok {
		device.Capabilities = make([]string, len(caps))
		for i, cap := range caps {
			if capStr, ok := cap.(string); ok {
				device.Capabilities[i] = capStr
			}
		}
	}

	s.devices[device.ID] = device
	log.Printf("Device registered: %s (%s) with capabilities: %v", device.Name, device.ID, device.Capabilities)
}

// handleSubscription processes subscription requests
func (s *HomeLinkService) handleSubscription(msg *Message) {
	subData, ok := msg.Data.(map[string]interface{})
	if !ok {
		return
	}

	eventTypesInterface, ok := subData["event_types"].([]interface{})
	if !ok {
		return
	}

	// Convert to string slice
	eventTypes := make([]string, len(eventTypesInterface))
	for i, et := range eventTypesInterface {
		if etStr, ok := et.(string); ok {
			eventTypes[i] = etStr
		}
	}

	s.mutex.Lock()
	s.subscriptions[msg.DeviceID] = eventTypes
	s.mutex.Unlock()

	log.Printf("Device %s subscribed to: %v", msg.DeviceID, eventTypes)
}

// handleEvent processes incoming events
func (s *HomeLinkService) handleEvent(msg *Message) {
	log.Printf("Received event from %s: %v", msg.DeviceID, msg.Data)
	// In a real implementation, you'd forward this to subscribers
	// For now, just log it
}

// handleHeartbeat keeps track of alive devices
func (s *HomeLinkService) handleHeartbeat(msg *Message, addr *net.UDPAddr) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if device, exists := s.devices[msg.DeviceID]; exists {
		device.LastSeen = time.Now()
		device.Address = addr
	}
}

// announceDevice broadcasts our presence to the network
func (s *HomeLinkService) announceDevice() {
	msg := Message{
		Type:      MSG_DEVICE_ANNOUNCEMENT,
		Version:   PROTOCOL_VERSION,
		Timestamp: time.Now().Unix(),
		DeviceID:  s.deviceID,
		Data: map[string]interface{}{
			"name":         s.deviceName,
			"capabilities": s.capabilities,
		},
	}

	s.broadcastMessage(msg)
	log.Printf("Announced device: %s", s.deviceName)
}

// broadcastMessage sends a message to all HomeLink services on the network
func (s *HomeLinkService) broadcastMessage(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Create broadcast address
	broadcastAddr := &net.UDPAddr{
		IP:   net.ParseIP(BROADCAST_ADDR),
		Port: HOMELINK_PORT,
	}

	// Create a connection for broadcasting
	conn, err := net.DialUDP("udp4", nil, broadcastAddr)
	if err != nil {
		return fmt.Errorf("failed to create broadcast connection: %v", err)
	}
	defer conn.Close()

	n, err := conn.Write(data)
	if err != nil {
		log.Printf("Failed to broadcast %s: %v", msg.Type, err)
		return err
	}

	log.Printf("Network broadcast %s: sent %d bytes to %s:%d", msg.Type, n, BROADCAST_ADDR, HOMELINK_PORT)
	return nil
}

// SendEvent sends an event to all interested subscribers
func (s *HomeLinkService) SendEvent(eventType, description string, data map[string]string) {
	event := Event{
		EventType:   eventType,
		Source:      s.deviceID,
		Description: description,
		Data:        data,
	}

	select {
	case s.eventChan <- event:
		log.Printf("Event queued: %s", eventType)
	default:
		log.Printf("Event channel full, dropping event: %s", eventType)
	}
}

// processEvents handles outgoing events
func (s *HomeLinkService) processEvents() {
	for {
		select {
		case <-s.stopChan:
			return
		case event := <-s.eventChan:
			msg := Message{
				Type:      MSG_EVENT,
				Version:   PROTOCOL_VERSION,
				Timestamp: time.Now().Unix(),
				DeviceID:  s.deviceID,
				Data:      event,
			}

			s.broadcastMessage(msg)
			log.Printf("Event sent: %s - %s", event.EventType, event.Description)
		}
	}
}

// sendHeartbeats sends periodic heartbeats
func (s *HomeLinkService) sendHeartbeats() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			msg := Message{
				Type:      MSG_HEARTBEAT,
				Version:   PROTOCOL_VERSION,
				Timestamp: time.Now().Unix(),
				DeviceID:  s.deviceID,
				Data:      nil,
			}
			s.broadcastMessage(msg)
		}
	}
}

// GetDevices returns all known devices
func (s *HomeLinkService) GetDevices() map[string]*Device {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	devices := make(map[string]*Device)
	for k, v := range s.devices {
		devices[k] = v
	}
	return devices
}

// Subscribe to specific event types
func (s *HomeLinkService) Subscribe(eventTypes []string) {
	msg := Message{
		Type:      MSG_SUBSCRIBE,
		Version:   PROTOCOL_VERSION,
		Timestamp: time.Now().Unix(),
		DeviceID:  s.deviceID,
		Data: map[string]interface{}{
			"event_types": eventTypes,
		},
	}
	s.broadcastMessage(msg)
	log.Printf("Subscribed to events: %v", eventTypes)
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

// Example usage - This would typically be in a separate main.go file
/*
func main() {
	// Create a HomeLink service instance
	service := NewHomeLinkService(
		"frigate-01",
		"Frigate Security System",
		[]string{"motion_detection", "person_detection", "vehicle_detection"},
	)

	// Start the service
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start service: %v", err)
	}

	// Subscribe to events we care about
	service.Subscribe([]string{"motion_detection", "person_detection"})

	// Simulate sending events (this would normally be triggered by external systems)
	go func() {
		time.Sleep(5 * time.Second)
		service.SendEvent("person_detection", "Person detected at front door", map[string]string{
			"camera":    "front_door",
			"confidence": "95",
			"timestamp":  fmt.Sprintf("%d", time.Now().Unix()),
		})
	}()

	// Keep running
	select {}
}
*/
