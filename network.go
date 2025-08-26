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
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Failed to parse message: %v", err)
		return
	}

	// Ignore our own messages
	if msg.DeviceID == s.deviceID {
		return
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
	}
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
