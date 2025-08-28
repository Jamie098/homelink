// HomeLink - Message Factory
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"time"
)

// MessageFactory creates standardized messages
type MessageFactory struct {
	deviceID string
}

// NewMessageFactory creates a new message factory
func NewMessageFactory(deviceID string) *MessageFactory {
	return &MessageFactory{
		deviceID: deviceID,
	}
}

// CreateMessage creates a basic message with common fields
func (mf *MessageFactory) CreateMessage(msgType MessageType, data interface{}) Message {
	return Message{
		Type:      msgType,
		Version:   PROTOCOL_VERSION,
		Timestamp: time.Now().Unix(),
		DeviceID:  mf.deviceID,
		Data:      data,
	}
}

// CreateDeviceAnnouncement creates a device announcement message
func (mf *MessageFactory) CreateDeviceAnnouncement(deviceName string, capabilities []string) Message {
	return mf.CreateMessage(MSG_DEVICE_ANNOUNCEMENT, map[string]interface{}{
		"name":         deviceName,
		"capabilities": capabilities,
	})
}

// CreateSubscription creates a subscription message
func (mf *MessageFactory) CreateSubscription(eventTypes []string) Message {
	return mf.CreateMessage(MSG_SUBSCRIBE, map[string]interface{}{
		"event_types": eventTypes,
	})
}

// CreateEvent creates an event message
func (mf *MessageFactory) CreateEvent(event Event) Message {
	return mf.CreateMessage(MSG_EVENT, event)
}

// CreateHeartbeat creates a heartbeat message
func (mf *MessageFactory) CreateHeartbeat() Message {
	return mf.CreateMessage(MSG_HEARTBEAT, nil)
}

// CreateDiscoveryRequest creates a discovery request message
func (mf *MessageFactory) CreateDiscoveryRequest() Message {
	return mf.CreateMessage(MSG_DISCOVERY_REQUEST, nil)
}

// CreatePairingRequest creates a pairing request message
func (mf *MessageFactory) CreatePairingRequest(publicKey string, networkKey string) Message {
	return mf.CreateMessage(MSG_PAIRING_REQUEST, map[string]interface{}{
		"public_key":  publicKey,
		"network_key": networkKey,
	})
}

// CreatePairingResponse creates a pairing response message
func (mf *MessageFactory) CreatePairingResponse(accepted bool, reason string) Message {
	return mf.CreateMessage(MSG_PAIRING_RESPONSE, map[string]interface{}{
		"accepted": accepted,
		"reason":   reason,
	})
}
