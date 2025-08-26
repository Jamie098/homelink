// HomeLink - Message and Data Types
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"net"
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
}

// Subscription represents what events a device wants to receive
type Subscription struct {
	DeviceID   string   `json:"device_id"`
	EventTypes []string `json:"event_types"`
}
