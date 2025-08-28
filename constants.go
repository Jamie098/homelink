// HomeLink - Protocol Constants and Configuration
// Copyright (c) 2025 - Open Source Project

package homelink

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
	MSG_DISCOVERY_REQUEST   MessageType = "discovery_request"
	MSG_SUBSCRIBE           MessageType = "subscribe"
	MSG_EVENT               MessageType = "event"
	MSG_HEARTBEAT           MessageType = "heartbeat"
	MSG_PAIRING_REQUEST     MessageType = "pairing_request"
	MSG_PAIRING_RESPONSE    MessageType = "pairing_response"
)
