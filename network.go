// HomeLink - Network Communication
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

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

			s.handleMessage(buffer[:n], addr)
		}
	}
}

// handleMessage processes incoming messages
func (s *HomeLinkService) handleMessage(data []byte, addr *net.UDPAddr) {
	// Try to parse as secure message first if security is enabled
	if s.IsSecurityEnabled() {
		var secureMsg SecureMessage
		if err := json.Unmarshal(data, &secureMsg); err == nil {
			s.handleSecureMessage(&secureMsg, addr)
			return
		}
		// If security is required, reject non-secure messages
		if s.securityConfig.RequireAuthentication {
			log.Printf("Rejecting non-secure message (authentication required)")
			return
		}
	}

	// Handle regular (non-secure) messages
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Failed to parse message: %v", err)
		return
	}

	// Ignore our own messages
	if msg.DeviceID == s.deviceID {
		return
	}

	// Apply rate limiting if enabled
	if s.rateLimiter != nil {
		if !s.rateLimiter.AllowMessage(msg.DeviceID) {
			log.Printf("Rate limit exceeded for device %s", msg.DeviceID)
			return
		}
	}

	switch msg.Type {
	case MSG_DEVICE_ANNOUNCEMENT:
		s.handleDeviceAnnouncement(&msg, addr)
	case MSG_DISCOVERY_REQUEST:
		s.handleDiscoveryRequest(&msg, addr)
	case MSG_SUBSCRIBE:
		s.handleSubscription(&msg)
	case MSG_EVENT:
		s.handleEvent(&msg)
	case MSG_HEARTBEAT:
		s.handleHeartbeat(&msg, addr)
	case MSG_PAIRING_REQUEST:
		s.handlePairingRequest(&msg, addr)
	case MSG_PAIRING_RESPONSE:
		s.handlePairingResponse(&msg, addr)
	}
}

// broadcastMessage sends a message to all HomeLink services on the network
func (s *HomeLinkService) broadcastMessage(msg Message) error {
	var data []byte
	var err error

	// Encrypt message if security is enabled
	if s.IsSecurityEnabled() {
		secureMsg, encErr := s.security.EncryptMessage(&msg)
		if encErr != nil {
			return fmt.Errorf("failed to encrypt message: %v", encErr)
		}
		data, err = json.Marshal(secureMsg)
	} else {
		data, err = json.Marshal(msg)
	}
	
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
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

	secureStr := ""
	if s.IsSecurityEnabled() {
		secureStr = " (encrypted)"
	}
	log.Printf("Network broadcast %s%s: sent %d bytes to %s:%d", msg.Type, secureStr, n, BROADCAST_ADDR, HOMELINK_PORT)
	return nil
}

// handleSecureMessage processes encrypted messages
func (s *HomeLinkService) handleSecureMessage(secureMsg *SecureMessage, addr *net.UDPAddr) {
	// Apply rate limiting first
	if s.rateLimiter != nil {
		if !s.rateLimiter.AllowMessage(secureMsg.DeviceID) {
			log.Printf("Rate limit exceeded for device %s", secureMsg.DeviceID)
			return
		}
	}

	// Decrypt the message
	msg, err := s.security.DecryptMessage(secureMsg)
	if err != nil {
		log.Printf("Failed to decrypt message from %s: %v", secureMsg.DeviceID, err)
		return
	}

	// Ignore our own messages
	if msg.DeviceID == s.deviceID {
		return
	}

	// Process the decrypted message
	switch msg.Type {
	case MSG_DEVICE_ANNOUNCEMENT:
		s.handleSecureDeviceAnnouncement(msg, addr, secureMsg.DeviceID)
	case MSG_DISCOVERY_REQUEST:
		s.handleDiscoveryRequest(msg, addr)
	case MSG_SUBSCRIBE:
		s.handleSubscription(msg)
	case MSG_EVENT:
		s.handleEvent(msg)
	case MSG_HEARTBEAT:
		s.handleHeartbeat(msg, addr)
	}
}

// handlePairingRequest handles device pairing requests
func (s *HomeLinkService) handlePairingRequest(msg *Message, addr *net.UDPAddr) {
	if !s.IsSecurityEnabled() {
		return
	}

	// Parse pairing request data
	requestData, ok := msg.Data.(map[string]interface{})
	if !ok {
		log.Printf("Invalid pairing request data")
		return
	}

	// Convert to DevicePairingRequest struct
	requestJSON, _ := json.Marshal(requestData)
	var pairingReq DevicePairingRequest
	if err := json.Unmarshal(requestJSON, &pairingReq); err != nil {
		log.Printf("Failed to parse pairing request: %v", err)
		return
	}

	// Verify the pairing request
	if !s.security.VerifyPairingRequest(&pairingReq) {
		log.Printf("Invalid pairing request from %s", pairingReq.DeviceID)
		return
	}

	// Decide whether to accept the pairing
	accepted := s.securityConfig.AutoAcceptPairing
	if !accepted && !s.securityConfig.RequireDeviceApproval {
		// TODO: Implement manual approval mechanism
		accepted = true // For now, accept all verified requests
	}

	// Create pairing response
	response, err := s.security.CreatePairingResponse(&pairingReq, accepted)
	if err != nil {
		log.Printf("Failed to create pairing response: %v", err)
		return
	}

	// Send response
	responseMsg := Message{
		Type:      MSG_PAIRING_RESPONSE,
		Version:   PROTOCOL_VERSION,
		Timestamp: time.Now().Unix(),
		DeviceID:  s.deviceID,
		Data:      response,
	}

	// Send directly to requesting device
	s.sendDirectMessage(&responseMsg, addr)

	if accepted {
		// Add device to trusted list
		s.security.AddTrustedDevice(pairingReq.DeviceID, pairingReq.PublicKey)
		log.Printf("Accepted pairing request from %s (%s)", pairingReq.DeviceName, pairingReq.DeviceID)
	} else {
		log.Printf("Rejected pairing request from %s (%s)", pairingReq.DeviceName, pairingReq.DeviceID)
	}
}

// handlePairingResponse handles responses to pairing requests
func (s *HomeLinkService) handlePairingResponse(msg *Message, addr *net.UDPAddr) {
	// This would be handled by a device that initiated pairing
	// Implementation depends on specific pairing flow requirements
	log.Printf("Received pairing response from %s", msg.DeviceID)
}

// sendDirectMessage sends a message directly to a specific device
func (s *HomeLinkService) sendDirectMessage(msg *Message, addr *net.UDPAddr) error {
	var data []byte
	var err error

	// Encrypt message if security is enabled
	if s.IsSecurityEnabled() {
		secureMsg, encErr := s.security.EncryptMessage(msg)
		if encErr != nil {
			return fmt.Errorf("failed to encrypt message: %v", encErr)
		}
		data, err = json.Marshal(secureMsg)
	} else {
		data, err = json.Marshal(msg)
	}
	
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	// Create a connection for direct messaging
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to create direct connection: %v", err)
	}
	defer conn.Close()

	n, err := conn.Write(data)
	if err != nil {
		log.Printf("Failed to send direct %s: %v", msg.Type, err)
		return err
	}

	secureStr := ""
	if s.IsSecurityEnabled() {
		secureStr = " (encrypted)"
	}
	log.Printf("Direct message %s%s: sent %d bytes to %s", msg.Type, secureStr, n, addr.String())
	return nil
}
