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
