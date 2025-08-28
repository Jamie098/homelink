// HomeLink - Device Discovery and Management
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"log"
	"net"
	"time"
)

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
}

// handleSecureDeviceAnnouncement processes device announcements from authenticated devices
func (s *HomeLinkService) handleSecureDeviceAnnouncement(msg *Message, addr *net.UDPAddr, senderID string) {
	// Only process announcements from trusted devices
	trustedDevices := s.security.GetTrustedDevices()
	trusted := false
	for _, deviceID := range trustedDevices {
		if deviceID == senderID {
			trusted = true
			break
		}
	}

	if !trusted {
		log.Printf("Ignoring announcement from untrusted device: %s", senderID)
		return
	}

	// Parse device data from message
	deviceData, ok := msg.Data.(map[string]interface{})
	if !ok {
		log.Printf("Invalid secure device announcement data")
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
		Trusted:  true,
		AuthTime: time.Now(),
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

	// Get public key from security manager
	device.PublicKey = s.security.GetPublicKey()

	s.devices[device.ID] = device
	log.Printf("Updated trusted device: %s (%s)", device.Name, device.ID)
}

// handleDiscoveryRequest responds to discovery requests from new devices
func (s *HomeLinkService) handleDiscoveryRequest(msg *Message, addr *net.UDPAddr) {
	log.Printf("Received discovery request from %s, announcing ourselves", msg.DeviceID)
	// Small random delay to avoid network congestion when multiple devices respond
	time.Sleep(time.Duration(time.Now().UnixNano()%100) * time.Millisecond)
	s.announceDevice()
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

// requestDiscovery asks other devices on the network to announce themselves
func (s *HomeLinkService) requestDiscovery() {
	msg := Message{
		Type:      MSG_DISCOVERY_REQUEST,
		Version:   PROTOCOL_VERSION,
		Timestamp: time.Now().Unix(),
		DeviceID:  s.deviceID,
		Data:      nil,
	}

	s.broadcastMessage(msg)
	log.Printf("Sent discovery request to find other devices")
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

// sendPeriodicAnnouncements re-announces our device periodically for better discovery
func (s *HomeLinkService) sendPeriodicAnnouncements() {
	// Announce ourselves every 2 minutes to ensure new devices can find us
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.announceDevice()
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

// TriggerDiscovery manually triggers device discovery (useful for debugging)
func (s *HomeLinkService) TriggerDiscovery() {
	log.Printf("Manually triggering device discovery...")
	s.announceDevice()
	time.Sleep(100 * time.Millisecond) // Small delay
	s.requestDiscovery()
}

// GetDiscoveryStats returns statistics about device discovery (useful for debugging)
func (s *HomeLinkService) GetDiscoveryStats() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	stats := map[string]interface{}{
		"total_devices":      len(s.devices),
		"device_count":       len(s.devices),
		"subscription_count": len(s.subscriptions),
		"devices":            make([]map[string]interface{}, 0, len(s.devices)),
	}

	for _, device := range s.devices {
		deviceInfo := map[string]interface{}{
			"id":           device.ID,
			"name":         device.Name,
			"capabilities": device.Capabilities,
			"last_seen":    device.LastSeen.Format(time.RFC3339),
			"address":      device.Address.String(),
		}
		stats["devices"] = append(stats["devices"].([]map[string]interface{}), deviceInfo)
	}

	return stats
}
