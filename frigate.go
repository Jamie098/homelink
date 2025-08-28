// HomeLink - Frigate Security System Integration
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// FrigateEvent represents a detection event from Frigate
type FrigateEvent struct {
	EventID     string                 `json:"event_id"`
	Camera      string                 `json:"camera"`
	EventType   string                 `json:"event_type"` // person, vehicle, motion, etc.
	Confidence  float64                `json:"confidence"`
	Zone        string                 `json:"zone"`
	Timestamp   time.Time              `json:"timestamp"`
	Duration    time.Duration          `json:"duration"`
	Snapshot    *FrigateSnapshot       `json:"snapshot,omitempty"`
	BoundingBox *FrigateBoundingBox    `json:"bounding_box,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// FrigateSnapshot contains image data from detection
type FrigateSnapshot struct {
	Data        []byte `json:"data,omitempty"`         // Base64 encoded image data
	URL         string `json:"url,omitempty"`          // URL to snapshot
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Format      string `json:"format"`                 // jpeg, png, webp
	Size        int    `json:"size"`                   // File size in bytes
	Thumbnail   []byte `json:"thumbnail,omitempty"`    // Smaller preview image
}

// FrigateBoundingBox defines the detection area
type FrigateBoundingBox struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// FrigateCamera represents a camera configuration
type FrigateCamera struct {
	Name           string             `json:"name"`
	MinConfidence  float64            `json:"min_confidence"`
	Zones          []string           `json:"zones"`
	Notifications  []string           `json:"notifications"`
	SnapshotConfig FrigateSnapshotConfig `json:"snapshot_config"`
	Enabled        bool               `json:"enabled"`
}

// FrigateSnapshotConfig controls snapshot behavior
type FrigateSnapshotConfig struct {
	Enabled       bool `json:"enabled"`
	IncludeImage  bool `json:"include_image"`
	MaxSize       int  `json:"max_size"`        // Max file size in bytes
	Quality       int  `json:"quality"`         // JPEG quality 1-100
	CreateThumbnail bool `json:"create_thumbnail"`
	ThumbnailSize int  `json:"thumbnail_size"`  // Thumbnail width
}

// FrigateRule represents an automation rule
type FrigateRule struct {
	Name        string                 `json:"name"`
	Enabled     bool                   `json:"enabled"`
	Conditions  []FrigateCondition     `json:"conditions"`
	Actions     []FrigateAction        `json:"actions"`
	Cooldown    time.Duration          `json:"cooldown"`
	LastTrigger *time.Time            `json:"last_trigger,omitempty"`
}

// FrigateCondition defines when a rule should trigger
type FrigateCondition struct {
	Field    string      `json:"field"`    // camera, event_type, confidence, zone, time
	Operator string      `json:"operator"` // ==, !=, >, <, >=, <=, contains, in
	Value    interface{} `json:"value"`
}

// FrigateAction defines what to do when rule triggers
type FrigateAction struct {
	Type       string                 `json:"type"`       // webhook, priority, retain_snapshot, notify
	Parameters map[string]interface{} `json:"parameters"`
}

// FrigateIntegration manages Frigate-specific features
type FrigateIntegration struct {
	service    *HomeLinkService
	cameras    map[string]*FrigateCamera
	rules      []FrigateRule
	eventCache map[string]*FrigateEvent // For deduplication
	config     *FrigateConfig
}

// FrigateConfig holds integration configuration
type FrigateConfig struct {
	DedupeWindow    time.Duration `json:"dedupe_window"`
	MinConfidence   float64       `json:"min_confidence"`
	ZoneFilters     []string      `json:"zone_filters"`
	RetainSnapshots bool          `json:"retain_snapshots"`
	SnapshotPath    string        `json:"snapshot_path"`
	WebhookURL      string        `json:"webhook_url"`
	NtfyURL         string        `json:"ntfy_url"`
}

// NewFrigateIntegration creates a new Frigate integration
func NewFrigateIntegration(service *HomeLinkService, config *FrigateConfig) *FrigateIntegration {
	return &FrigateIntegration{
		service:     service,
		cameras:     make(map[string]*FrigateCamera),
		rules:       make([]FrigateRule, 0),
		eventCache: make(map[string]*FrigateEvent),
		config:      config,
	}
}

// AddCamera adds a camera configuration
func (fi *FrigateIntegration) AddCamera(camera *FrigateCamera) {
	fi.cameras[camera.Name] = camera
	log.Printf("Added Frigate camera: %s (min confidence: %.2f)", camera.Name, camera.MinConfidence)
}

// AddRule adds an automation rule
func (fi *FrigateIntegration) AddRule(rule FrigateRule) {
	fi.rules = append(fi.rules, rule)
	log.Printf("Added Frigate rule: %s", rule.Name)
}

// ProcessEvent processes a Frigate detection event
func (fi *FrigateIntegration) ProcessEvent(event *FrigateEvent) error {
	// Check if we should process this event
	if !fi.shouldProcessEvent(event) {
		return nil
	}

	// Check for duplicates
	if fi.isDuplicateEvent(event) {
		log.Printf("Skipping duplicate Frigate event: %s/%s", event.Camera, event.EventType)
		return nil
	}

	// Cache event for deduplication
	fi.cacheEvent(event)

	// Get camera config
	camera, exists := fi.cameras[event.Camera]
	if !exists {
		// Use default settings if camera not configured
		camera = &FrigateCamera{
			Name:          event.Camera,
			MinConfidence: fi.config.MinConfidence,
			Enabled:       true,
		}
	}

	// Apply camera-specific filtering
	if !fi.passesFilters(event, camera) {
		return nil
	}

	// Create HomeLink message
	msg := fi.createHomeLinkMessage(event)

	// Determine priority based on event type and confidence
	priority := fi.calculatePriority(event, camera)

	// Send as reliable message
	fi.service.SendReliableEvent(
		msg.Type.String(),
		fmt.Sprintf("%s detected on %s", event.EventType, event.Camera),
		fi.createEventData(event),
		priority,
	)

	// Process automation rules
	fi.processRules(event)

	log.Printf("Processed Frigate event: %s/%s (confidence: %.2f)", 
		event.Camera, event.EventType, event.Confidence)

	return nil
}

// shouldProcessEvent determines if an event should be processed
func (fi *FrigateIntegration) shouldProcessEvent(event *FrigateEvent) bool {
	// Check global confidence threshold
	if event.Confidence < fi.config.MinConfidence {
		return false
	}

	// Check zone filters
	if len(fi.config.ZoneFilters) > 0 {
		zoneAllowed := false
		for _, allowedZone := range fi.config.ZoneFilters {
			if event.Zone == allowedZone {
				zoneAllowed = true
				break
			}
		}
		if !zoneAllowed {
			return false
		}
	}

	return true
}

// isDuplicateEvent checks if this event is a duplicate within the dedupe window
func (fi *FrigateIntegration) isDuplicateEvent(event *FrigateEvent) bool {
	cacheKey := fmt.Sprintf("%s:%s:%s", event.Camera, event.EventType, event.Zone)
	
	if cachedEvent, exists := fi.eventCache[cacheKey]; exists {
		if time.Since(cachedEvent.Timestamp) < fi.config.DedupeWindow {
			return true
		}
	}
	
	return false
}

// cacheEvent stores event for deduplication
func (fi *FrigateIntegration) cacheEvent(event *FrigateEvent) {
	cacheKey := fmt.Sprintf("%s:%s:%s", event.Camera, event.EventType, event.Zone)
	fi.eventCache[cacheKey] = event

	// Clean up old cache entries
	go fi.cleanupCache()
}

// cleanupCache removes old events from cache
func (fi *FrigateIntegration) cleanupCache() {
	cutoff := time.Now().Add(-fi.config.DedupeWindow * 2)
	
	for key, event := range fi.eventCache {
		if event.Timestamp.Before(cutoff) {
			delete(fi.eventCache, key)
		}
	}
}

// passesFilters checks camera-specific filters
func (fi *FrigateIntegration) passesFilters(event *FrigateEvent, camera *FrigateCamera) bool {
	// Check if camera is enabled
	if !camera.Enabled {
		return false
	}

	// Check confidence threshold
	if event.Confidence < camera.MinConfidence {
		return false
	}

	// Check if event type is in notifications list
	if len(camera.Notifications) > 0 {
		typeAllowed := false
		for _, allowedType := range camera.Notifications {
			if event.EventType == allowedType {
				typeAllowed = true
				break
			}
		}
		if !typeAllowed {
			return false
		}
	}

	// Check zones
	if len(camera.Zones) > 0 {
		zoneAllowed := false
		for _, allowedZone := range camera.Zones {
			if event.Zone == allowedZone {
				zoneAllowed = true
				break
			}
		}
		if !zoneAllowed {
			return false
		}
	}

	return true
}

// createHomeLinkMessage converts Frigate event to HomeLink message
func (fi *FrigateIntegration) createHomeLinkMessage(event *FrigateEvent) *Message {
	return &Message{
		Type:      MessageType(fmt.Sprintf("%s_detected", event.EventType)),
		Version:   PROTOCOL_VERSION,
		Timestamp: event.Timestamp.Unix(),
		DeviceID:  fi.service.deviceID,
		Data:      fi.createEventData(event),
	}
}

// createEventData creates the data payload for HomeLink message
func (fi *FrigateIntegration) createEventData(event *FrigateEvent) map[string]string {
	data := map[string]string{
		"camera":      event.Camera,
		"event_type":  event.EventType,
		"confidence":  fmt.Sprintf("%.2f", event.Confidence),
		"zone":        event.Zone,
		"event_id":    event.EventID,
		"timestamp":   event.Timestamp.Format(time.RFC3339),
		"duration":    event.Duration.String(),
		"source":      "frigate",
	}

	// Add bounding box if available
	if event.BoundingBox != nil {
		data["bbox_x"] = fmt.Sprintf("%d", event.BoundingBox.X)
		data["bbox_y"] = fmt.Sprintf("%d", event.BoundingBox.Y)
		data["bbox_width"] = fmt.Sprintf("%d", event.BoundingBox.Width)
		data["bbox_height"] = fmt.Sprintf("%d", event.BoundingBox.Height)
	}

	// Add snapshot info if available
	if event.Snapshot != nil {
		data["snapshot_format"] = event.Snapshot.Format
		data["snapshot_size"] = fmt.Sprintf("%d", event.Snapshot.Size)
		data["snapshot_width"] = fmt.Sprintf("%d", event.Snapshot.Width)
		data["snapshot_height"] = fmt.Sprintf("%d", event.Snapshot.Height)
		
		if event.Snapshot.URL != "" {
			data["snapshot_url"] = event.Snapshot.URL
		}
		
		// Include base64 image data if small enough
		if len(event.Snapshot.Data) > 0 && len(event.Snapshot.Data) < 100*1024 { // 100KB limit
			data["snapshot_data"] = base64.StdEncoding.EncodeToString(event.Snapshot.Data)
		}
	}

	// Add any additional metadata
	for key, value := range event.Metadata {
		if strValue, ok := value.(string); ok {
			data[fmt.Sprintf("meta_%s", key)] = strValue
		}
	}

	return data
}

// calculatePriority determines message priority based on event
func (fi *FrigateIntegration) calculatePriority(event *FrigateEvent, camera *FrigateCamera) MessagePriority {
	// High confidence person detection is high priority
	if event.EventType == "person" && event.Confidence > 0.9 {
		return PriorityHigh
	}

	// Vehicle detection at entrance zones
	if event.EventType == "vehicle" && (strings.Contains(event.Zone, "entrance") || strings.Contains(event.Zone, "driveway")) {
		return PriorityHigh
	}

	// Very high confidence anything
	if event.Confidence > 0.95 {
		return PriorityHigh
	}

	// Medium confidence
	if event.Confidence > 0.8 {
		return PriorityNormal
	}

	return PriorityLow
}

// processRules evaluates and executes automation rules
func (fi *FrigateIntegration) processRules(event *FrigateEvent) {
	for _, rule := range fi.rules {
		if !rule.Enabled {
			continue
		}

		// Check cooldown
		if rule.LastTrigger != nil && time.Since(*rule.LastTrigger) < rule.Cooldown {
			continue
		}

		// Evaluate conditions
		if fi.evaluateConditions(event, rule.Conditions) {
			fi.executeActions(event, rule.Actions)
			
			// Update last trigger time
			now := time.Now()
			rule.LastTrigger = &now
			
			log.Printf("Triggered Frigate rule: %s", rule.Name)
		}
	}
}

// evaluateConditions checks if all rule conditions are met
func (fi *FrigateIntegration) evaluateConditions(event *FrigateEvent, conditions []FrigateCondition) bool {
	for _, condition := range conditions {
		if !fi.evaluateCondition(event, condition) {
			return false
		}
	}
	return true
}

// evaluateCondition checks a single condition
func (fi *FrigateIntegration) evaluateCondition(event *FrigateEvent, condition FrigateCondition) bool {
	var fieldValue interface{}

	// Get field value from event
	switch condition.Field {
	case "camera":
		fieldValue = event.Camera
	case "event_type":
		fieldValue = event.EventType
	case "confidence":
		fieldValue = event.Confidence
	case "zone":
		fieldValue = event.Zone
	case "time":
		fieldValue = event.Timestamp.Hour() // Hour of day for time-based rules
	default:
		return false
	}

	// Evaluate based on operator
	switch condition.Operator {
	case "==":
		return fieldValue == condition.Value
	case "!=":
		return fieldValue != condition.Value
	case ">":
		if fv, ok := fieldValue.(float64); ok {
			if cv, ok := condition.Value.(float64); ok {
				return fv > cv
			}
		}
	case "<":
		if fv, ok := fieldValue.(float64); ok {
			if cv, ok := condition.Value.(float64); ok {
				return fv < cv
			}
		}
	case ">=":
		if fv, ok := fieldValue.(float64); ok {
			if cv, ok := condition.Value.(float64); ok {
				return fv >= cv
			}
		}
	case "<=":
		if fv, ok := fieldValue.(float64); ok {
			if cv, ok := condition.Value.(float64); ok {
				return fv <= cv
			}
		}
	case "contains":
		if fv, ok := fieldValue.(string); ok {
			if cv, ok := condition.Value.(string); ok {
				return strings.Contains(fv, cv)
			}
		}
	case "in":
		if values, ok := condition.Value.([]interface{}); ok {
			for _, v := range values {
				if fieldValue == v {
					return true
				}
			}
		}
	}

	return false
}

// executeActions performs rule actions
func (fi *FrigateIntegration) executeActions(event *FrigateEvent, actions []FrigateAction) {
	for _, action := range actions {
		switch action.Type {
		case "webhook":
			if url, ok := action.Parameters["url"].(string); ok {
				go fi.sendWebhook(event, url)
			}
		case "priority":
			if priority, ok := action.Parameters["level"].(string); ok {
				// This would modify the message priority (already sent, so log for now)
				log.Printf("Rule requested priority change to: %s", priority)
			}
		case "retain_snapshot":
			fi.retainSnapshot(event)
		case "notify":
			fi.sendNotification(event, action.Parameters)
		}
	}
}

// sendWebhook sends event data to webhook URL
func (fi *FrigateIntegration) sendWebhook(event *FrigateEvent, url string) {
	// This would implement HTTP webhook sending
	// For now, just log the action
	log.Printf("Would send webhook to %s for event %s/%s", url, event.Camera, event.EventType)
}

// retainSnapshot saves the snapshot to persistent storage
func (fi *FrigateIntegration) retainSnapshot(event *FrigateEvent) {
	if event.Snapshot != nil && len(event.Snapshot.Data) > 0 && fi.config.SnapshotPath != "" {
		// This would save the snapshot to disk
		log.Printf("Would retain snapshot for event %s to %s", event.EventID, fi.config.SnapshotPath)
	}
}

// sendNotification sends a notification via configured service
func (fi *FrigateIntegration) sendNotification(event *FrigateEvent, params map[string]interface{}) {
	// This would send notifications via ntfy, Discord, etc.
	log.Printf("Would send notification for event %s/%s", event.Camera, event.EventType)
}

// GetStats returns Frigate integration statistics
func (fi *FrigateIntegration) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"cameras_configured": len(fi.cameras),
		"rules_active":       len(fi.rules),
		"events_cached":      len(fi.eventCache),
		"config":             fi.config,
	}
}