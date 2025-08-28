// HomeLink - Event System
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"fmt"
	"log"
)

// Event represents something that happened (like motion detection)
type Event struct {
	EventType   string            `json:"event_type"`
	Source      string            `json:"source"`
	Description string            `json:"description"`
	Data        map[string]string `json:"data"`
}

// SendEvent sends an event to all interested subscribers
func (s *HomeLinkService) SendEvent(eventType, description string, data map[string]string) {
	event := Event{
		EventType:   eventType,
		Source:      s.deviceID,
		Description: description,
		Data:        data,
	}

	select {
	case s.eventChan <- event:
		log.Printf("Event queued: %s", eventType)
	default:
		log.Printf("Event channel full, dropping event: %s", eventType)
	}
}

// processEvents handles outgoing events
func (s *HomeLinkService) processEvents() {
	for {
		select {
		case <-s.stopChan:
			return
		case event := <-s.eventChan:
			msg := s.messageFactory.CreateEvent(event)
			if err := s.broadcastMessage(msg); err != nil {
				s.errorHandler.HandleNetworkError("broadcast event", err)
			} else {
				s.errorHandler.LogInfo("event", fmt.Sprintf("Event sent: %s - %s", event.EventType, event.Description))
			}
		}
	}
}

// handleEvent processes incoming events
func (s *HomeLinkService) handleEvent(msg *Message) {
	log.Printf("Received event from %s: %v", msg.DeviceID, msg.Data)
	// In a real implementation, you'd forward this to subscribers
	// For now, just log it
}

// handleSubscription processes subscription requests
func (s *HomeLinkService) handleSubscription(msg *Message) {
	subData, ok := msg.Data.(map[string]interface{})
	if !ok {
		return
	}

	eventTypesInterface, ok := subData["event_types"].([]interface{})
	if !ok {
		return
	}

	// Convert to string slice
	eventTypes := make([]string, len(eventTypesInterface))
	for i, et := range eventTypesInterface {
		if etStr, ok := et.(string); ok {
			eventTypes[i] = etStr
		}
	}

	s.mutex.Lock()
	s.subscriptions[msg.DeviceID] = eventTypes
	s.mutex.Unlock()

	log.Printf("Device %s subscribed to: %v", msg.DeviceID, eventTypes)
}

// Subscribe to specific event types
func (s *HomeLinkService) Subscribe(eventTypes []string) {
	msg := s.messageFactory.CreateSubscription(eventTypes)
	if err := s.broadcastMessage(msg); err != nil {
		s.errorHandler.HandleNetworkError("subscribe", err)
	} else {
		s.errorHandler.LogInfo("subscription", fmt.Sprintf("Subscribed to events: %v", eventTypes))
	}
}
