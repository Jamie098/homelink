// HomeLink - High-Performance Binary Protocol
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// BinaryProtocolVersion defines the binary protocol version
const BinaryProtocolVersion uint8 = 1

// BinaryMessageType defines message types in binary protocol
type BinaryMessageType uint8

const (
	BinaryDeviceAnnouncement BinaryMessageType = 0x01
	BinaryDiscoveryRequest   BinaryMessageType = 0x02
	BinarySubscribe          BinaryMessageType = 0x03
	BinaryEvent              BinaryMessageType = 0x04
	BinaryHeartbeat          BinaryMessageType = 0x05
	BinaryPairingRequest     BinaryMessageType = 0x06
	BinaryPairingResponse    BinaryMessageType = 0x07
	BinaryMessageAck         BinaryMessageType = 0x08
	BinaryHealthReport       BinaryMessageType = 0x09
)

// BinaryHeader represents the fixed header for all binary messages
type BinaryHeader struct {
	Magic       uint32 // 0x484C4E4B ("HLNK")
	Version     uint8
	MessageType BinaryMessageType
	Flags       uint8
	Reserved    uint8
	Length      uint32   // Total message length including header
	DeviceID    [16]byte // Fixed-size device ID (truncated/padded)
	Timestamp   uint64
}

// Header flags
const (
	FlagCompressed   uint8 = 0x01
	FlagEncrypted    uint8 = 0x02
	FlagReliable     uint8 = 0x04
	FlagHighPriority uint8 = 0x08
)

// Magic number for HomeLink binary protocol
const BinaryMagic uint32 = 0x484C4E4B

// BinaryMessage represents a complete binary protocol message
type BinaryMessage struct {
	Header  BinaryHeader
	Payload []byte
}

// BinaryEncoder handles encoding messages to binary format
type BinaryEncoder struct {
	deviceID      string
	deviceIDBytes [16]byte
}

// BinaryDecoder handles decoding binary messages
type BinaryDecoder struct{}

// NewBinaryEncoder creates a new binary encoder
func NewBinaryEncoder(deviceID string) *BinaryEncoder {
	encoder := &BinaryEncoder{
		deviceID: deviceID,
	}

	// Convert device ID to fixed-size byte array
	copy(encoder.deviceIDBytes[:], []byte(deviceID))

	return encoder
}

// NewBinaryDecoder creates a new binary decoder
func NewBinaryDecoder() *BinaryDecoder {
	return &BinaryDecoder{}
}

// EncodeMessage converts a standard message to binary format
func (e *BinaryEncoder) EncodeMessage(msg *Message) (*BinaryMessage, error) {
	// Determine message type
	msgType, err := e.convertMessageType(msg.Type)
	if err != nil {
		return nil, err
	}

	// Encode payload based on message type
	payload, err := e.encodePayload(msg)
	if err != nil {
		return nil, err
	}

	// Create header
	header := BinaryHeader{
		Magic:       BinaryMagic,
		Version:     BinaryProtocolVersion,
		MessageType: msgType,
		Flags:       0, // No flags by default
		Reserved:    0,
		Length:      uint32(32 + len(payload)), // Header size + payload size
		Timestamp:   uint64(time.Now().Unix()),
	}

	copy(header.DeviceID[:], e.deviceIDBytes[:])

	return &BinaryMessage{
		Header:  header,
		Payload: payload,
	}, nil
}

// EncodeReliableMessage encodes a reliable message with additional metadata
func (e *BinaryEncoder) EncodeReliableMessage(reliableMsg *ReliableMessage) (*BinaryMessage, error) {
	binaryMsg, err := e.EncodeMessage(&reliableMsg.Message)
	if err != nil {
		return nil, err
	}

	// Set reliable flag
	binaryMsg.Header.Flags |= FlagReliable

	// Set priority flag
	if reliableMsg.Priority >= PriorityHigh {
		binaryMsg.Header.Flags |= FlagHighPriority
	}

	// Prepend reliability metadata to payload
	reliabilityHeader := make([]byte, 28) // MessageID(16) + Priority(1) + DeliveryMode(1) + RetryCount(1) + MaxRetries(1) + ExpiresAt(8)

	copy(reliabilityHeader[0:16], []byte(reliableMsg.MessageID))
	reliabilityHeader[16] = uint8(reliableMsg.Priority)
	reliabilityHeader[17] = uint8(reliableMsg.DeliveryMode)
	reliabilityHeader[18] = uint8(reliableMsg.RetryCount)
	reliabilityHeader[19] = uint8(reliableMsg.MaxRetries)
	binary.LittleEndian.PutUint64(reliabilityHeader[20:28], uint64(reliableMsg.ExpiresAt))

	// Combine reliability header with original payload
	newPayload := make([]byte, len(reliabilityHeader)+len(binaryMsg.Payload))
	copy(newPayload, reliabilityHeader)
	copy(newPayload[len(reliabilityHeader):], binaryMsg.Payload)

	binaryMsg.Payload = newPayload
	binaryMsg.Header.Length = uint32(32 + len(newPayload))

	return binaryMsg, nil
}

// WriteTo writes a binary message to an io.Writer
func (bm *BinaryMessage) WriteTo(w io.Writer) (int64, error) {
	written := int64(0)

	// Write header
	err := binary.Write(w, binary.LittleEndian, &bm.Header)
	if err != nil {
		return written, err
	}
	written += 32 // Header size

	// Write payload
	n, err := w.Write(bm.Payload)
	written += int64(n)

	return written, err
}

// ReadFrom reads a binary message from an io.Reader
func (bm *BinaryMessage) ReadFrom(r io.Reader) (int64, error) {
	read := int64(0)

	// Read header
	err := binary.Read(r, binary.LittleEndian, &bm.Header)
	if err != nil {
		return read, err
	}
	read += 32

	// Validate magic number
	if bm.Header.Magic != BinaryMagic {
		return read, errors.New("invalid magic number")
	}

	// Validate version
	if bm.Header.Version != BinaryProtocolVersion {
		return read, fmt.Errorf("unsupported protocol version: %d", bm.Header.Version)
	}

	// Calculate payload size
	payloadSize := bm.Header.Length - 32
	if payloadSize > 1024*1024 { // 1MB limit
		return read, errors.New("payload too large")
	}

	// Read payload
	bm.Payload = make([]byte, payloadSize)
	n, err := io.ReadFull(r, bm.Payload)
	read += int64(n)

	return read, err
}

// DecodeMessage converts a binary message back to standard format
func (d *BinaryDecoder) DecodeMessage(binaryMsg *BinaryMessage) (*Message, error) {
	// Extract device ID
	deviceID := string(binaryMsg.Header.DeviceID[:])
	// Remove null padding
	for i, b := range binaryMsg.Header.DeviceID {
		if b == 0 {
			deviceID = string(binaryMsg.Header.DeviceID[:i])
			break
		}
	}

	// Convert message type
	msgType, err := d.convertBinaryMessageType(binaryMsg.Header.MessageType)
	if err != nil {
		return nil, err
	}

	// Decode payload
	data, err := d.decodePayload(binaryMsg.Header.MessageType, binaryMsg.Payload)
	if err != nil {
		return nil, err
	}

	return &Message{
		Type:      msgType,
		Version:   fmt.Sprintf("binary-%d", binaryMsg.Header.Version),
		Timestamp: int64(binaryMsg.Header.Timestamp),
		DeviceID:  deviceID,
		Data:      data,
	}, nil
}

// DecodeReliableMessage decodes a reliable binary message
func (d *BinaryDecoder) DecodeReliableMessage(binaryMsg *BinaryMessage) (*ReliableMessage, error) {
	if binaryMsg.Header.Flags&FlagReliable == 0 {
		return nil, errors.New("message is not marked as reliable")
	}

	// Extract reliability header
	if len(binaryMsg.Payload) < 28 {
		return nil, errors.New("invalid reliable message format")
	}

	messageID := string(binaryMsg.Payload[0:16])
	// Remove null padding from message ID
	for i, b := range binaryMsg.Payload[0:16] {
		if b == 0 {
			messageID = string(binaryMsg.Payload[0:i])
			break
		}
	}

	priority := MessagePriority(binaryMsg.Payload[16])
	deliveryMode := DeliveryMode(binaryMsg.Payload[17])
	retryCount := int(binaryMsg.Payload[18])
	maxRetries := int(binaryMsg.Payload[19])
	expiresAt := int64(binary.LittleEndian.Uint64(binaryMsg.Payload[20:28]))

	// Create a new binary message with the payload minus reliability header
	innerMsg := &BinaryMessage{
		Header:  binaryMsg.Header,
		Payload: binaryMsg.Payload[28:],
	}
	innerMsg.Header.Flags &^= FlagReliable // Remove reliable flag for inner decoding

	// Decode inner message
	msg, err := d.DecodeMessage(innerMsg)
	if err != nil {
		return nil, err
	}

	return &ReliableMessage{
		Message:      *msg,
		MessageID:    messageID,
		Priority:     priority,
		DeliveryMode: deliveryMode,
		RetryCount:   retryCount,
		MaxRetries:   maxRetries,
		ExpiresAt:    expiresAt,
	}, nil
}

// convertMessageType converts standard message type to binary
func (e *BinaryEncoder) convertMessageType(msgType MessageType) (BinaryMessageType, error) {
	switch msgType {
	case MSG_DEVICE_ANNOUNCEMENT:
		return BinaryDeviceAnnouncement, nil
	case MSG_DISCOVERY_REQUEST:
		return BinaryDiscoveryRequest, nil
	case MSG_SUBSCRIBE:
		return BinarySubscribe, nil
	case MSG_EVENT:
		return BinaryEvent, nil
	case MSG_HEARTBEAT:
		return BinaryHeartbeat, nil
	case MSG_PAIRING_REQUEST:
		return BinaryPairingRequest, nil
	case MSG_PAIRING_RESPONSE:
		return BinaryPairingResponse, nil
	default:
		return 0, fmt.Errorf("unknown message type: %s", msgType)
	}
}

// convertBinaryMessageType converts binary message type to standard
func (d *BinaryDecoder) convertBinaryMessageType(binaryType BinaryMessageType) (MessageType, error) {
	switch binaryType {
	case BinaryDeviceAnnouncement:
		return MSG_DEVICE_ANNOUNCEMENT, nil
	case BinaryDiscoveryRequest:
		return MSG_DISCOVERY_REQUEST, nil
	case BinarySubscribe:
		return MSG_SUBSCRIBE, nil
	case BinaryEvent:
		return MSG_EVENT, nil
	case BinaryHeartbeat:
		return MSG_HEARTBEAT, nil
	case BinaryPairingRequest:
		return MSG_PAIRING_REQUEST, nil
	case BinaryPairingResponse:
		return MSG_PAIRING_RESPONSE, nil
	case BinaryMessageAck:
		return "message_ack", nil
	case BinaryHealthReport:
		return "health_report", nil
	default:
		return "", fmt.Errorf("unknown binary message type: %d", binaryType)
	}
}

// encodePayload encodes message data based on type
func (e *BinaryEncoder) encodePayload(msg *Message) ([]byte, error) {
	// This is a simplified implementation
	// In a real system, you'd have specific binary encodings for each message type

	switch msg.Type {
	case MSG_DEVICE_ANNOUNCEMENT:
		return e.encodeDeviceAnnouncement(msg.Data)
	case MSG_DISCOVERY_REQUEST:
		return []byte{}, nil // No payload
	case MSG_SUBSCRIBE:
		return e.encodeSubscription(msg.Data)
	case MSG_EVENT:
		return e.encodeEvent(msg.Data)
	case MSG_HEARTBEAT:
		return []byte{}, nil // No payload
	default:
		// Fallback to simple encoding
		return []byte(fmt.Sprintf("%v", msg.Data)), nil
	}
}

// encodeDeviceAnnouncement creates binary payload for device announcement
func (e *BinaryEncoder) encodeDeviceAnnouncement(data interface{}) ([]byte, error) {
	// Simplified: encode as length-prefixed strings
	// Real implementation would use proper binary format
	payload := make([]byte, 0, 256)

	if dataMap, ok := data.(map[string]interface{}); ok {
		if name, ok := dataMap["name"].(string); ok {
			nameBytes := []byte(name)
			payload = append(payload, byte(len(nameBytes)))
			payload = append(payload, nameBytes...)
		}

		if caps, ok := dataMap["capabilities"].([]string); ok {
			payload = append(payload, byte(len(caps)))
			for _, cap := range caps {
				capBytes := []byte(cap)
				payload = append(payload, byte(len(capBytes)))
				payload = append(payload, capBytes...)
			}
		}
	}

	return payload, nil
}

// encodeSubscription creates binary payload for subscription
func (e *BinaryEncoder) encodeSubscription(data interface{}) ([]byte, error) {
	if dataMap, ok := data.(map[string]interface{}); ok {
		if eventTypes, ok := dataMap["event_types"].([]string); ok {
			payload := make([]byte, 0, 128)
			payload = append(payload, byte(len(eventTypes)))

			for _, eventType := range eventTypes {
				eventBytes := []byte(eventType)
				payload = append(payload, byte(len(eventBytes)))
				payload = append(payload, eventBytes...)
			}

			return payload, nil
		}
	}
	return []byte{}, nil
}

// encodeEvent creates binary payload for event
func (e *BinaryEncoder) encodeEvent(data interface{}) ([]byte, error) {
	// Simplified event encoding
	// Real implementation would use efficient binary format
	return []byte(fmt.Sprintf("%v", data)), nil
}

// decodePayload decodes binary payload based on message type
func (d *BinaryDecoder) decodePayload(msgType BinaryMessageType, payload []byte) (interface{}, error) {
	switch msgType {
	case BinaryDeviceAnnouncement:
		return d.decodeDeviceAnnouncement(payload)
	case BinaryDiscoveryRequest:
		return nil, nil
	case BinarySubscribe:
		return d.decodeSubscription(payload)
	case BinaryEvent:
		return d.decodeEvent(payload)
	case BinaryHeartbeat:
		return nil, nil
	default:
		// Fallback: return as string
		return string(payload), nil
	}
}

// decodeDeviceAnnouncement decodes device announcement payload
func (d *BinaryDecoder) decodeDeviceAnnouncement(payload []byte) (interface{}, error) {
	if len(payload) < 1 {
		return nil, errors.New("invalid device announcement payload")
	}

	result := make(map[string]interface{})
	offset := 0

	// Decode name
	if offset < len(payload) {
		nameLen := int(payload[offset])
		offset++
		if offset+nameLen <= len(payload) {
			result["name"] = string(payload[offset : offset+nameLen])
			offset += nameLen
		}
	}

	// Decode capabilities
	if offset < len(payload) {
		capCount := int(payload[offset])
		offset++
		capabilities := make([]string, 0, capCount)

		for i := 0; i < capCount && offset < len(payload); i++ {
			capLen := int(payload[offset])
			offset++
			if offset+capLen <= len(payload) {
				capabilities = append(capabilities, string(payload[offset:offset+capLen]))
				offset += capLen
			}
		}

		result["capabilities"] = capabilities
	}

	return result, nil
}

// decodeSubscription decodes subscription payload
func (d *BinaryDecoder) decodeSubscription(payload []byte) (interface{}, error) {
	if len(payload) < 1 {
		return nil, errors.New("invalid subscription payload")
	}

	eventCount := int(payload[0])
	offset := 1
	eventTypes := make([]string, 0, eventCount)

	for i := 0; i < eventCount && offset < len(payload); i++ {
		eventLen := int(payload[offset])
		offset++
		if offset+eventLen <= len(payload) {
			eventTypes = append(eventTypes, string(payload[offset:offset+eventLen]))
			offset += eventLen
		}
	}

	return map[string]interface{}{
		"event_types": eventTypes,
	}, nil
}

// decodeEvent decodes event payload
func (d *BinaryDecoder) decodeEvent(payload []byte) (interface{}, error) {
	// Simplified: return as string
	// Real implementation would properly decode binary event data
	return string(payload), nil
}

// GetSize returns the total size of the binary message in bytes
func (bm *BinaryMessage) GetSize() int {
	return 32 + len(bm.Payload) // Header + payload
}

// IsCompressed returns whether the message is compressed
func (bm *BinaryMessage) IsCompressed() bool {
	return bm.Header.Flags&FlagCompressed != 0
}

// IsEncrypted returns whether the message is encrypted
func (bm *BinaryMessage) IsEncrypted() bool {
	return bm.Header.Flags&FlagEncrypted != 0
}

// IsReliable returns whether the message requires acknowledgment
func (bm *BinaryMessage) IsReliable() bool {
	return bm.Header.Flags&FlagReliable != 0
}

// IsHighPriority returns whether the message has high priority
func (bm *BinaryMessage) IsHighPriority() bool {
	return bm.Header.Flags&FlagHighPriority != 0
}
