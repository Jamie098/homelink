// HomeLink - MQTT Bridge for Home Assistant Integration
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTTConfig contains MQTT broker configuration
type MQTTConfig struct {
	Enabled       bool              `json:"enabled"`
	BrokerURL     string            `json:"broker_url"`     // mqtt://localhost:1883 or mqtts://broker:8883
	Username      string            `json:"username"`
	Password      string            `json:"password"`
	ClientID      string            `json:"client_id"`
	TLSEnabled    bool              `json:"tls_enabled"`
	TLSSkipVerify bool              `json:"tls_skip_verify"`
	QoS           byte              `json:"qos"`               // 0, 1, or 2
	Retained      bool              `json:"retained"`
	TopicPrefix   string            `json:"topic_prefix"`      // homelink
	HADiscovery   HADiscoveryConfig `json:"ha_discovery"`
	DeviceFilters []string          `json:"device_filters"`    // Filter specific devices
	EventFilters  []string          `json:"event_filters"`     // Filter specific event types
}

// HADiscoveryConfig configures Home Assistant MQTT Discovery
type HADiscoveryConfig struct {
	Enabled           bool   `json:"enabled"`
	DiscoveryPrefix   string `json:"discovery_prefix"`   // homeassistant
	NodeID            string `json:"node_id"`            // homelink_service
	DeviceName        string `json:"device_name"`        // HomeLink Service
	DeviceModel       string `json:"device_model"`       // HomeLink Protocol v1.0
	DeviceManufacturer string `json:"device_manufacturer"` // HomeLink Project
	AutoExpiry        int    `json:"auto_expiry"`        // Seconds before entity expires
}

// MQTTBridge manages Home Assistant MQTT integration
type MQTTBridge struct {
	config      MQTTConfig
	client      mqtt.Client
	mutex       sync.RWMutex
	connected   bool
	stats       MQTTStats
	entities    map[string]*HAEntity  // Track discovered entities
	lastSeen    map[string]time.Time  // Device last seen timestamps
	service     *HomeLinkService      // Reference to main service
}

// MQTTStats tracks MQTT bridge statistics
type MQTTStats struct {
	MessagesPublished   uint64            `json:"messages_published"`
	MessagesReceived    uint64            `json:"messages_received"`
	ConnectionAttempts  uint64            `json:"connection_attempts"`
	ConnectionFailures  uint64            `json:"connection_failures"`
	LastConnected       time.Time         `json:"last_connected"`
	EntitiesDiscovered  uint64            `json:"entities_discovered"`
	DevicesTracked      uint64            `json:"devices_tracked"`
	TopicStats          map[string]uint64 `json:"topic_stats"`
	ErrorCount          uint64            `json:"error_count"`
}

// HAEntity represents a Home Assistant entity
type HAEntity struct {
	EntityID      string                 `json:"entity_id"`
	Name          string                 `json:"name"`
	DeviceClass   string                 `json:"device_class"`
	Component     string                 `json:"component"`     // sensor, binary_sensor, etc.
	ConfigTopic   string                 `json:"config_topic"`
	StateTopic    string                 `json:"state_topic"`
	CommandTopic  string                 `json:"command_topic"`
	Config        map[string]interface{} `json:"config"`
	LastUpdated   time.Time             `json:"last_updated"`
}

// NewMQTTBridge creates a new Home Assistant MQTT bridge
func NewMQTTBridge(config MQTTConfig, service *HomeLinkService) *MQTTBridge {
	bridge := &MQTTBridge{
		config:   config,
		entities: make(map[string]*HAEntity),
		lastSeen: make(map[string]time.Time),
		service:  service,
		stats: MQTTStats{
			TopicStats: make(map[string]uint64),
		},
	}

	if config.Enabled {
		if err := bridge.Connect(); err != nil {
			log.Printf("Failed to connect to MQTT broker: %v", err)
		}
	}

	return bridge
}

// Connect establishes connection to MQTT broker
func (mb *MQTTBridge) Connect() error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.stats.ConnectionAttempts++

	// Parse broker URL
	brokerURL, err := url.Parse(mb.config.BrokerURL)
	if err != nil {
		mb.stats.ConnectionFailures++
		return fmt.Errorf("invalid broker URL: %v", err)
	}

	// Configure MQTT client options
	opts := mqtt.NewClientOptions()
	opts.AddBroker(mb.config.BrokerURL)
	opts.SetClientID(mb.config.ClientID)

	if mb.config.Username != "" {
		opts.SetUsername(mb.config.Username)
		opts.SetPassword(mb.config.Password)
	}

	// Configure TLS
	if mb.config.TLSEnabled || brokerURL.Scheme == "mqtts" {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: mb.config.TLSSkipVerify,
		}
		opts.SetTLSConfig(tlsConfig)
	}

	// Set connection callbacks
	opts.SetOnConnectHandler(mb.onConnect)
	opts.SetConnectionLostHandler(mb.onConnectionLost)
	opts.SetReconnectingHandler(mb.onReconnecting)

	// Set will message for device availability
	willTopic := fmt.Sprintf("%s/bridge/status", mb.config.TopicPrefix)
	opts.SetWill(willTopic, "offline", mb.config.QoS, mb.config.Retained)

	// Create and connect client
	mb.client = mqtt.NewClient(opts)
	if token := mb.client.Connect(); token.Wait() && token.Error() != nil {
		mb.stats.ConnectionFailures++
		return fmt.Errorf("failed to connect: %v", token.Error())
	}

	mb.connected = true
	mb.stats.LastConnected = time.Now()
	log.Printf("MQTT bridge connected to %s", mb.config.BrokerURL)

	return nil
}

// onConnect callback when MQTT connection established
func (mb *MQTTBridge) onConnect(client mqtt.Client) {
	log.Printf("MQTT bridge connected")

	// Publish online status
	statusTopic := fmt.Sprintf("%s/bridge/status", mb.config.TopicPrefix)
	mb.publishMessage(statusTopic, "online", true)

	// Subscribe to command topics if needed
	commandTopic := fmt.Sprintf("%s/+/command", mb.config.TopicPrefix)
	if token := client.Subscribe(commandTopic, mb.config.QoS, mb.onCommand); token.Wait() && token.Error() != nil {
		log.Printf("Failed to subscribe to command topic: %v", token.Error())
	}

	// Discover existing devices from service
	mb.discoverExistingDevices()
}

// onConnectionLost callback when MQTT connection lost
func (mb *MQTTBridge) onConnectionLost(client mqtt.Client, err error) {
	mb.mutex.Lock()
	mb.connected = false
	mb.stats.ErrorCount++
	mb.mutex.Unlock()

	log.Printf("MQTT connection lost: %v", err)
}

// onReconnecting callback when MQTT client is reconnecting
func (mb *MQTTBridge) onReconnecting(client mqtt.Client, opts *mqtt.ClientOptions) {
	log.Printf("MQTT bridge reconnecting...")
}

// onCommand handles commands received via MQTT
func (mb *MQTTBridge) onCommand(client mqtt.Client, msg mqtt.Message) {
	mb.mutex.Lock()
	mb.stats.MessagesReceived++
	mb.mutex.Unlock()

	log.Printf("Received MQTT command on topic %s: %s", msg.Topic(), msg.Payload())

	// Parse topic to extract device and command
	parts := strings.Split(msg.Topic(), "/")
	if len(parts) < 3 {
		return
	}

	deviceID := parts[len(parts)-2]
	
	// Handle different command types
	switch string(msg.Payload()) {
	case "discover":
		mb.triggerDiscovery(deviceID)
	case "restart":
		mb.sendRestartCommand(deviceID)
	}
}

// PublishEvent publishes HomeLink events to MQTT
func (mb *MQTTBridge) PublishEvent(msg *Message) {
	if !mb.config.Enabled || !mb.connected {
		return
	}

	// Apply device filters
	if !mb.shouldPublishDevice(msg.DeviceID) {
		return
	}

	// Apply event filters
	if !mb.shouldPublishEvent(string(msg.Type)) {
		return
	}

	// Create Home Assistant compatible payload
	payload := mb.createHAPayload(msg)
	
	// Publish to device-specific topic
	topic := fmt.Sprintf("%s/%s/state", mb.config.TopicPrefix, msg.DeviceID)
	mb.publishMessage(topic, payload, false)

	// Update device availability
	mb.updateDeviceAvailability(msg.DeviceID)

	// Auto-discover device if needed
	if mb.config.HADiscovery.Enabled {
		mb.autoDiscoverDevice(msg)
	}
}

// PublishDeviceUpdate publishes device status updates
func (mb *MQTTBridge) PublishDeviceUpdate(device *Device) {
	if !mb.config.Enabled || !mb.connected {
		return
	}

	if !mb.shouldPublishDevice(device.ID) {
		return
	}

	// Create device state payload
	payload := map[string]interface{}{
		"device_id":    device.ID,
		"name":         device.Name,
		"capabilities": device.Capabilities,
		"last_seen":    device.LastSeen.Unix(),
		"status":       "online",
		"ip_address":   device.IPAddress,
		"port":         device.Port,
	}

	topic := fmt.Sprintf("%s/%s/device", mb.config.TopicPrefix, device.ID)
	mb.publishMessage(topic, payload, false)

	// Update Home Assistant discovery
	if mb.config.HADiscovery.Enabled {
		mb.discoverDevice(device)
	}
}

// createHAPayload creates Home Assistant compatible event payload
func (mb *MQTTBridge) createHAPayload(msg *Message) interface{} {
	payload := map[string]interface{}{
		"timestamp":  msg.Timestamp,
		"device_id":  msg.DeviceID,
		"event_type": string(msg.Type),
		"data":       msg.Data,
	}

	// Add HomeLink specific attributes
	payload["protocol"] = "homelink"
	payload["version"] = "1.0"

	// Extract common sensor values for HA
	if dataMap, ok := msg.Data.(map[string]interface{}); ok {
		// Map common sensor types
		if confidence, exists := dataMap["confidence"]; exists {
			payload["confidence"] = confidence
		}
		if zone, exists := dataMap["zone"]; exists {
			payload["zone"] = zone
		}
		if camera, exists := dataMap["camera"]; exists {
			payload["camera"] = camera
		}
		if temperature, exists := dataMap["temperature"]; exists {
			payload["temperature"] = temperature
		}
		if humidity, exists := dataMap["humidity"]; exists {
			payload["humidity"] = humidity
		}
	}

	return payload
}

// autoDiscoverDevice automatically discovers device entities for Home Assistant
func (mb *MQTTBridge) autoDiscoverDevice(msg *Message) {
	deviceID := msg.DeviceID
	
	// Check if already discovered recently
	if lastDiscovery, exists := mb.lastSeen[deviceID]; exists {
		if time.Since(lastDiscovery) < 5*time.Minute {
			return
		}
	}

	mb.lastSeen[deviceID] = time.Now()

	// Create main device sensor
	mb.createDeviceSensor(deviceID, msg)

	// Create event-specific sensors based on message data
	if dataMap, ok := msg.Data.(map[string]interface{}); ok {
		mb.createDataSensors(deviceID, dataMap)
	}
}

// createDeviceSensor creates main device sensor for Home Assistant
func (mb *MQTTBridge) createDeviceSensor(deviceID string, msg *Message) {
	entityID := fmt.Sprintf("%s_%s_state", mb.config.HADiscovery.NodeID, deviceID)
	
	config := map[string]interface{}{
		"name":                fmt.Sprintf("%s State", deviceID),
		"unique_id":           entityID,
		"state_topic":         fmt.Sprintf("%s/%s/state", mb.config.TopicPrefix, deviceID),
		"value_template":      "{{ value_json.event_type }}",
		"json_attributes_topic": fmt.Sprintf("%s/%s/state", mb.config.TopicPrefix, deviceID),
		"device": map[string]interface{}{
			"identifiers":  []string{deviceID},
			"name":         deviceID,
			"model":        mb.config.HADiscovery.DeviceModel,
			"manufacturer": mb.config.HADiscovery.DeviceManufacturer,
			"sw_version":   "1.0",
		},
		"availability": map[string]interface{}{
			"topic":                fmt.Sprintf("%s/%s/availability", mb.config.TopicPrefix, deviceID),
			"payload_available":    "online",
			"payload_not_available": "offline",
		},
	}

	// Add expiry if configured
	if mb.config.HADiscovery.AutoExpiry > 0 {
		config["expire_after"] = mb.config.HADiscovery.AutoExpiry
	}

	configTopic := fmt.Sprintf("%s/sensor/%s/%s/config", 
		mb.config.HADiscovery.DiscoveryPrefix,
		mb.config.HADiscovery.NodeID,
		entityID,
	)

	mb.publishMessage(configTopic, config, true)

	// Track entity
	entity := &HAEntity{
		EntityID:     entityID,
		Name:         fmt.Sprintf("%s State", deviceID),
		Component:    "sensor",
		ConfigTopic:  configTopic,
		StateTopic:   fmt.Sprintf("%s/%s/state", mb.config.TopicPrefix, deviceID),
		Config:       config,
		LastUpdated:  time.Now(),
	}

	mb.mutex.Lock()
	mb.entities[entityID] = entity
	mb.stats.EntitiesDiscovered++
	mb.mutex.Unlock()
}

// createDataSensors creates sensors for specific data fields
func (mb *MQTTBridge) createDataSensors(deviceID string, data map[string]interface{}) {
	sensorMappings := map[string]map[string]interface{}{
		"temperature": {
			"device_class": "temperature",
			"unit_of_measurement": "°C",
		},
		"humidity": {
			"device_class": "humidity",
			"unit_of_measurement": "%",
		},
		"confidence": {
			"device_class": "signal_strength",
			"unit_of_measurement": "%",
		},
		"battery": {
			"device_class": "battery",
			"unit_of_measurement": "%",
		},
	}

	for field, value := range data {
		if sensorConfig, exists := sensorMappings[field]; exists {
			mb.createFieldSensor(deviceID, field, value, sensorConfig)
		}
	}
}

// createFieldSensor creates a sensor for a specific data field
func (mb *MQTTBridge) createFieldSensor(deviceID, field string, value interface{}, sensorConfig map[string]interface{}) {
	entityID := fmt.Sprintf("%s_%s_%s", mb.config.HADiscovery.NodeID, deviceID, field)
	
	config := map[string]interface{}{
		"name":          fmt.Sprintf("%s %s", deviceID, strings.Title(field)),
		"unique_id":     entityID,
		"state_topic":   fmt.Sprintf("%s/%s/state", mb.config.TopicPrefix, deviceID),
		"value_template": fmt.Sprintf("{{ value_json.data.%s }}", field),
		"device": map[string]interface{}{
			"identifiers": []string{deviceID},
		},
	}

	// Add sensor-specific configuration
	for k, v := range sensorConfig {
		config[k] = v
	}

	configTopic := fmt.Sprintf("%s/sensor/%s/%s/config",
		mb.config.HADiscovery.DiscoveryPrefix,
		mb.config.HADiscovery.NodeID,
		entityID,
	)

	mb.publishMessage(configTopic, config, true)

	// Track entity
	entity := &HAEntity{
		EntityID:     entityID,
		Name:         fmt.Sprintf("%s %s", deviceID, strings.Title(field)),
		Component:    "sensor",
		ConfigTopic:  configTopic,
		StateTopic:   fmt.Sprintf("%s/%s/state", mb.config.TopicPrefix, deviceID),
		Config:       config,
		LastUpdated:  time.Now(),
	}

	mb.mutex.Lock()
	mb.entities[entityID] = entity
	mb.stats.EntitiesDiscovered++
	mb.mutex.Unlock()
}

// discoverDevice creates Home Assistant discovery config for a device
func (mb *MQTTBridge) discoverDevice(device *Device) {
	// Create device availability sensor
	entityID := fmt.Sprintf("%s_%s_availability", mb.config.HADiscovery.NodeID, device.ID)
	
	config := map[string]interface{}{
		"name":          fmt.Sprintf("%s Availability", device.Name),
		"unique_id":     entityID,
		"device_class":  "connectivity",
		"state_topic":   fmt.Sprintf("%s/%s/availability", mb.config.TopicPrefix, device.ID),
		"payload_on":    "online",
		"payload_off":   "offline",
		"device": map[string]interface{}{
			"identifiers":  []string{device.ID},
			"name":         device.Name,
			"model":        mb.config.HADiscovery.DeviceModel,
			"manufacturer": mb.config.HADiscovery.DeviceManufacturer,
		},
	}

	configTopic := fmt.Sprintf("%s/binary_sensor/%s/%s/config",
		mb.config.HADiscovery.DiscoveryPrefix,
		mb.config.HADiscovery.NodeID,
		entityID,
	)

	mb.publishMessage(configTopic, config, true)
	
	// Publish initial availability
	availTopic := fmt.Sprintf("%s/%s/availability", mb.config.TopicPrefix, device.ID)
	mb.publishMessage(availTopic, "online", false)
}

// discoverExistingDevices discovers all currently known devices
func (mb *MQTTBridge) discoverExistingDevices() {
	if mb.service == nil {
		return
	}

	devices := mb.service.GetDiscoveredDevices()
	mb.mutex.Lock()
	mb.stats.DevicesTracked = uint64(len(devices))
	mb.mutex.Unlock()

	for _, device := range devices {
		if mb.shouldPublishDevice(device.ID) {
			mb.discoverDevice(device)
		}
	}
}

// updateDeviceAvailability updates device availability status
func (mb *MQTTBridge) updateDeviceAvailability(deviceID string) {
	availTopic := fmt.Sprintf("%s/%s/availability", mb.config.TopicPrefix, deviceID)
	mb.publishMessage(availTopic, "online", false)
	mb.lastSeen[deviceID] = time.Now()
}

// publishMessage publishes a message to MQTT broker
func (mb *MQTTBridge) publishMessage(topic string, payload interface{}, retained bool) {
	if !mb.connected {
		return
	}

	var data []byte
	var err error

	switch v := payload.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		data, err = json.Marshal(payload)
		if err != nil {
			log.Printf("Failed to marshal MQTT payload: %v", err)
			return
		}
	}

	token := mb.client.Publish(topic, mb.config.QoS, retained, data)
	if token.Wait() && token.Error() != nil {
		mb.mutex.Lock()
		mb.stats.ErrorCount++
		mb.mutex.Unlock()
		log.Printf("Failed to publish MQTT message to %s: %v", topic, token.Error())
		return
	}

	mb.mutex.Lock()
	mb.stats.MessagesPublished++
	mb.stats.TopicStats[topic]++
	mb.mutex.Unlock()
}

// shouldPublishDevice checks if device should be published based on filters
func (mb *MQTTBridge) shouldPublishDevice(deviceID string) bool {
	if len(mb.config.DeviceFilters) == 0 {
		return true
	}

	for _, filter := range mb.config.DeviceFilters {
		if matched, _ := filepath.Match(filter, deviceID); matched {
			return true
		}
	}

	return false
}

// shouldPublishEvent checks if event type should be published based on filters
func (mb *MQTTBridge) shouldPublishEvent(eventType string) bool {
	if len(mb.config.EventFilters) == 0 {
		return true
	}

	for _, filter := range mb.config.EventFilters {
		if matched, _ := filepath.Match(filter, eventType); matched {
			return true
		}
	}

	return false
}

// triggerDiscovery triggers device discovery via MQTT command
func (mb *MQTTBridge) triggerDiscovery(deviceID string) {
	if mb.service != nil {
		mb.service.TriggerDiscovery()
		log.Printf("Discovery triggered via MQTT for device: %s", deviceID)
	}
}

// sendRestartCommand sends restart command to device (if supported)
func (mb *MQTTBridge) sendRestartCommand(deviceID string) {
	// This would send a restart command to the specific device
	// Implementation depends on protocol capabilities
	log.Printf("Restart command requested for device: %s", deviceID)
}

// Disconnect closes MQTT connection
func (mb *MQTTBridge) Disconnect() {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	if mb.client != nil && mb.connected {
		// Publish offline status
		statusTopic := fmt.Sprintf("%s/bridge/status", mb.config.TopicPrefix)
		mb.client.Publish(statusTopic, mb.config.QoS, mb.config.Retained, "offline")
		
		mb.client.Disconnect(250)
		mb.connected = false
		log.Printf("MQTT bridge disconnected")
	}
}

// GetStats returns MQTT bridge statistics
func (mb *MQTTBridge) GetStats() MQTTStats {
	mb.mutex.RLock()
	defer mb.mutex.RUnlock()

	// Create copy to avoid race conditions
	stats := mb.stats
	stats.TopicStats = make(map[string]uint64)
	for k, v := range mb.stats.TopicStats {
		stats.TopicStats[k] = v
	}

	return stats
}

// GetEntities returns discovered Home Assistant entities
func (mb *MQTTBridge) GetEntities() map[string]*HAEntity {
	mb.mutex.RLock()
	defer mb.mutex.RUnlock()

	entities := make(map[string]*HAEntity)
	for k, v := range mb.entities {
		entities[k] = v
	}

	return entities
}

// ResetStats resets MQTT statistics
func (mb *MQTTBridge) ResetStats() {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.stats = MQTTStats{
		TopicStats: make(map[string]uint64),
	}
}

// IsConnected returns current connection status
func (mb *MQTTBridge) IsConnected() bool {
	mb.mutex.RLock()
	defer mb.mutex.RUnlock()

	return mb.connected
}

// UpdateConfig updates MQTT configuration
func (mb *MQTTBridge) UpdateConfig(config MQTTConfig) error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	// Disconnect if currently connected
	if mb.connected {
		mb.client.Disconnect(250)
		mb.connected = false
	}

	mb.config = config

	// Reconnect if enabled
	if config.Enabled {
		return mb.Connect()
	}

	return nil
}