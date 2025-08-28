// HomeLink - Message Acknowledgment and Reliability
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// MessagePriority defines message priority levels
type MessagePriority int

const (
	PriorityLow MessagePriority = iota
	PriorityNormal
	PriorityHigh
	PriorityEmergency
)

// DeliveryMode defines how messages should be delivered
type DeliveryMode int

const (
	DeliveryFireAndForget DeliveryMode = iota // No acknowledgment required
	DeliveryReliable                          // Requires acknowledgment
	DeliveryOrdered                           // Reliable + ordered delivery
)

// ReliableMessage extends Message with reliability features
type ReliableMessage struct {
	Message     `json:"message"`
	MessageID   string          `json:"message_id"`
	Priority    MessagePriority `json:"priority"`
	DeliveryMode DeliveryMode   `json:"delivery_mode"`
	RetryCount  int            `json:"retry_count"`
	MaxRetries  int            `json:"max_retries"`
	ExpiresAt   int64          `json:"expires_at"`
}

// MessageAck represents an acknowledgment for a reliable message
type MessageAck struct {
	MessageID     string `json:"message_id"`
	RecipientID   string `json:"recipient_id"`
	Status        string `json:"status"` // "received", "processed", "failed"
	ErrorMessage  string `json:"error_message,omitempty"`
	ProcessedAt   int64  `json:"processed_at"`
}

// PendingMessage tracks messages waiting for acknowledgment
type PendingMessage struct {
	ReliableMessage
	SentAt      time.Time
	LastRetry   time.Time
	RecipientID string
	OnAck       func(ack *MessageAck) // Callback for when ack is received
	OnTimeout   func()                // Callback for timeout
}

// ReliabilityManager handles message acknowledgments and retries
type ReliabilityManager struct {
	deviceID        string
	pendingMessages map[string]*PendingMessage
	ackCallbacks    map[string]func(*MessageAck)
	mutex           sync.RWMutex
	
	// Configuration
	defaultTimeout  time.Duration
	retryInterval   time.Duration
	maxRetries      int
	
	// Channels
	ackChan     chan *MessageAck
	retryChan   chan *PendingMessage
	stopChan    chan bool
}

// NewReliabilityManager creates a new reliability manager
func NewReliabilityManager(deviceID string) *ReliabilityManager {
	rm := &ReliabilityManager{
		deviceID:        deviceID,
		pendingMessages: make(map[string]*PendingMessage),
		ackCallbacks:    make(map[string]func(*MessageAck)),
		defaultTimeout:  30 * time.Second,
		retryInterval:   5 * time.Second,
		maxRetries:      3,
		ackChan:         make(chan *MessageAck, 100),
		retryChan:       make(chan *PendingMessage, 100),
		stopChan:        make(chan bool),
	}
	
	// Start background workers
	go rm.processAcknowledgments()
	go rm.processRetries()
	go rm.cleanupExpiredMessages()
	
	return rm
}

// Stop shuts down the reliability manager
func (rm *ReliabilityManager) Stop() {
	close(rm.stopChan)
}

// GenerateMessageID creates a unique message ID
func (rm *ReliabilityManager) GenerateMessageID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// SendReliableMessage sends a message with reliability guarantees
func (rm *ReliabilityManager) SendReliableMessage(
	msg *Message,
	recipientID string,
	priority MessagePriority,
	deliveryMode DeliveryMode,
	onAck func(*MessageAck),
	onTimeout func(),
) *ReliableMessage {
	
	messageID := rm.GenerateMessageID()
	
	reliableMsg := &ReliableMessage{
		Message:      *msg,
		MessageID:    messageID,
		Priority:     priority,
		DeliveryMode: deliveryMode,
		RetryCount:   0,
		MaxRetries:   rm.maxRetries,
		ExpiresAt:    time.Now().Add(rm.defaultTimeout * time.Duration(rm.maxRetries+1)).Unix(),
	}
	
	// Only track messages that require acknowledgment
	if deliveryMode != DeliveryFireAndForget {
		pending := &PendingMessage{
			ReliableMessage: *reliableMsg,
			SentAt:          time.Now(),
			LastRetry:       time.Now(),
			RecipientID:     recipientID,
			OnAck:           onAck,
			OnTimeout:       onTimeout,
		}
		
		rm.mutex.Lock()
		rm.pendingMessages[messageID] = pending
		rm.mutex.Unlock()
	}
	
	return reliableMsg
}

// ProcessIncomingMessage handles incoming reliable messages
func (rm *ReliabilityManager) ProcessIncomingMessage(reliableMsg *ReliableMessage) (*MessageAck, bool) {
	// Check if message has expired
	if time.Now().Unix() > reliableMsg.ExpiresAt {
		return &MessageAck{
			MessageID:    reliableMsg.MessageID,
			RecipientID:  rm.deviceID,
			Status:       "failed",
			ErrorMessage: "message expired",
			ProcessedAt:  time.Now().Unix(),
		}, false
	}
	
	// Create acknowledgment
	ack := &MessageAck{
		MessageID:   reliableMsg.MessageID,
		RecipientID: rm.deviceID,
		Status:      "received",
		ProcessedAt: time.Now().Unix(),
	}
	
	// For fire-and-forget messages, no ack needed
	if reliableMsg.DeliveryMode == DeliveryFireAndForget {
		return nil, true
	}
	
	return ack, true
}

// ProcessAcknowledgment handles incoming acknowledgments
func (rm *ReliabilityManager) ProcessAcknowledgment(ack *MessageAck) {
	rm.ackChan <- ack
}

// processAcknowledgments runs in background to handle acks
func (rm *ReliabilityManager) processAcknowledgments() {
	for {
		select {
		case <-rm.stopChan:
			return
		case ack := <-rm.ackChan:
			rm.handleAcknowledgment(ack)
		}
	}
}

// handleAcknowledgment processes a single acknowledgment
func (rm *ReliabilityManager) handleAcknowledgment(ack *MessageAck) {
	rm.mutex.Lock()
	pending, exists := rm.pendingMessages[ack.MessageID]
	if exists {
		delete(rm.pendingMessages, ack.MessageID)
	}
	rm.mutex.Unlock()
	
	if exists && pending.OnAck != nil {
		go pending.OnAck(ack) // Call callback in goroutine to avoid blocking
	}
}

// processRetries handles message retries in background
func (rm *ReliabilityManager) processRetries() {
	ticker := time.NewTicker(rm.retryInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-rm.stopChan:
			return
		case <-ticker.C:
			rm.checkForRetries()
		case pending := <-rm.retryChan:
			rm.retryMessage(pending)
		}
	}
}

// checkForRetries looks for messages that need to be retried
func (rm *ReliabilityManager) checkForRetries() {
	rm.mutex.RLock()
	toRetry := make([]*PendingMessage, 0)
	
	for _, pending := range rm.pendingMessages {
		// Check if message needs retry
		if time.Since(pending.LastRetry) >= rm.retryInterval {
			if pending.RetryCount < pending.MaxRetries {
				toRetry = append(toRetry, pending)
			} else {
				// Message has exceeded max retries
				toRetry = append(toRetry, pending)
			}
		}
	}
	rm.mutex.RUnlock()
	
	// Process retries
	for _, pending := range toRetry {
		if pending.RetryCount >= pending.MaxRetries {
			rm.handleTimeout(pending)
		} else {
			rm.retryChan <- pending
		}
	}
}

// retryMessage attempts to resend a message
func (rm *ReliabilityManager) retryMessage(pending *PendingMessage) {
	rm.mutex.Lock()
	pending.RetryCount++
	pending.LastRetry = time.Now()
	rm.mutex.Unlock()
	
	// In a real implementation, this would resend the message through the network layer
	// For now, we just log the retry attempt
}

// handleTimeout processes messages that have timed out
func (rm *ReliabilityManager) handleTimeout(pending *PendingMessage) {
	rm.mutex.Lock()
	delete(rm.pendingMessages, pending.MessageID)
	rm.mutex.Unlock()
	
	if pending.OnTimeout != nil {
		go pending.OnTimeout()
	}
}

// cleanupExpiredMessages removes old messages that have expired
func (rm *ReliabilityManager) cleanupExpiredMessages() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-rm.stopChan:
			return
		case <-ticker.C:
			rm.performCleanup()
		}
	}
}

// performCleanup removes expired messages
func (rm *ReliabilityManager) performCleanup() {
	now := time.Now().Unix()
	toDelete := make([]string, 0)
	
	rm.mutex.RLock()
	for messageID, pending := range rm.pendingMessages {
		if now > pending.ExpiresAt {
			toDelete = append(toDelete, messageID)
		}
	}
	rm.mutex.RUnlock()
	
	rm.mutex.Lock()
	for _, messageID := range toDelete {
		delete(rm.pendingMessages, messageID)
	}
	rm.mutex.Unlock()
}

// GetPendingMessages returns statistics about pending messages
func (rm *ReliabilityManager) GetPendingMessages() map[string]interface{} {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()
	
	stats := map[string]interface{}{
		"total_pending":    len(rm.pendingMessages),
		"by_priority":      make(map[string]int),
		"by_delivery_mode": make(map[string]int),
		"oldest_message":   "",
	}
	
	priorityCounts := map[MessagePriority]int{
		PriorityLow: 0, PriorityNormal: 0, PriorityHigh: 0, PriorityEmergency: 0,
	}
	
	modeCounts := map[DeliveryMode]int{
		DeliveryFireAndForget: 0, DeliveryReliable: 0, DeliveryOrdered: 0,
	}
	
	var oldestTime time.Time
	var oldestID string
	
	for messageID, pending := range rm.pendingMessages {
		priorityCounts[pending.Priority]++
		modeCounts[pending.DeliveryMode]++
		
		if oldestTime.IsZero() || pending.SentAt.Before(oldestTime) {
			oldestTime = pending.SentAt
			oldestID = messageID
		}
	}
	
	stats["by_priority"] = map[string]int{
		"low":       priorityCounts[PriorityLow],
		"normal":    priorityCounts[PriorityNormal], 
		"high":      priorityCounts[PriorityHigh],
		"emergency": priorityCounts[PriorityEmergency],
	}
	
	stats["by_delivery_mode"] = map[string]int{
		"fire_and_forget": modeCounts[DeliveryFireAndForget],
		"reliable":        modeCounts[DeliveryReliable],
		"ordered":         modeCounts[DeliveryOrdered],
	}
	
	if oldestID != "" {
		stats["oldest_message"] = map[string]interface{}{
			"id":      oldestID,
			"sent_at": oldestTime.Format(time.RFC3339),
			"age":     time.Since(oldestTime).String(),
		}
	}
	
	return stats
}

// SetRetryConfiguration updates retry settings
func (rm *ReliabilityManager) SetRetryConfiguration(timeout time.Duration, interval time.Duration, maxRetries int) {
	rm.mutex.Lock()
	rm.defaultTimeout = timeout
	rm.retryInterval = interval
	rm.maxRetries = maxRetries
	rm.mutex.Unlock()
}

// GetRetryConfiguration returns current retry settings
func (rm *ReliabilityManager) GetRetryConfiguration() map[string]interface{} {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()
	
	return map[string]interface{}{
		"default_timeout": rm.defaultTimeout.String(),
		"retry_interval":  rm.retryInterval.String(),
		"max_retries":     rm.maxRetries,
	}
}