package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"time"
)

// HomeLinkEvent represents an event that will be sent to HomeLink
type HomeLinkEvent struct {
	EventType   string            `json:"event_type"`
	Description string            `json:"description"`
	Data        map[string]string `json:"data"`
	Snapshot    string            `json:"snapshot,omitempty"` // Base64-encoded snapshot image
}

// EventTransformer handles the transformation of Frigate events to HomeLink events
type EventTransformer struct {
	bridgeID      string
	frigateClient *FrigateClient
}

// NewEventTransformer creates a new event transformer
func NewEventTransformer(bridgeID string, frigateClient *FrigateClient) *EventTransformer {
	return &EventTransformer{
		bridgeID:      bridgeID,
		frigateClient: frigateClient,
	}
}

// TransformEvent converts a Frigate event to a HomeLink event
func (et *EventTransformer) TransformEvent(frigateEvent *FrigateEvent) *HomeLinkEvent {
	// Generate event type based on Frigate event data
	eventType := et.generateEventType(frigateEvent)
	
	// Generate human-readable description
	description := et.generateDescription(frigateEvent)
	
	// Create data payload with relevant information
	data := et.buildEventData(frigateEvent)
	
	// Fetch snapshot if available
	var snapshot string
	if frigateEvent.HasSnapshot {
		if snapshotData, err := et.frigateClient.GetThumbnail(frigateEvent.ID); err == nil {
			snapshot = base64.StdEncoding.EncodeToString(snapshotData)
			log.Printf("Retrieved snapshot for event %s (%d bytes)", frigateEvent.ID, len(snapshotData))
		} else {
			log.Printf("Failed to retrieve snapshot for event %s: %v", frigateEvent.ID, err)
		}
	}
	
	return &HomeLinkEvent{
		EventType:   eventType,
		Description: description,
		Data:        data,
		Snapshot:    snapshot,
	}
}

// generateEventType creates an appropriate event type for HomeLink
func (et *EventTransformer) generateEventType(event *FrigateEvent) string {
	// Base event type on the object label
	switch event.Label {
	case "person":
		return "person_detected"
	case "car", "truck", "bus":
		return "vehicle_detected"
	case "bicycle", "motorcycle":
		return "bicycle_detected"
	case "dog", "cat":
		return "pet_detected"
	case "bird":
		return "bird_detected"
	default:
		return "object_detected"
	}
}

// generateDescription creates a human-readable description
func (et *EventTransformer) generateDescription(event *FrigateEvent) string {
	// Format the description based on event details (removed confidence)
	if event.SubLabel != "" {
		return fmt.Sprintf("%s (%s) detected on %s", 
			et.capitalize(event.Label), event.SubLabel, event.Camera)
	}
	
	return fmt.Sprintf("%s detected on %s", 
		et.capitalize(event.Label), event.Camera)
}

// buildEventData creates the data map for the HomeLink event
func (et *EventTransformer) buildEventData(event *FrigateEvent) map[string]string {
	data := map[string]string{
		"source":       "frigate",
		"bridge_id":    et.bridgeID,
		"frigate_id":   event.ID,
		"camera":       event.Camera,
		"label":        event.Label,
		"start_time":   fmt.Sprintf("%.0f", event.StartTime),
		"has_snapshot": fmt.Sprintf("%t", event.HasSnapshot),
		"has_clip":     fmt.Sprintf("%t", event.HasClip),
		"area":         fmt.Sprintf("%d", event.Area),
	}
	
	// Add sub-label if present
	if event.SubLabel != "" {
		data["sub_label"] = event.SubLabel
	}
	
	// Add end time if event is complete
	if event.EndTime != nil {
		data["end_time"] = fmt.Sprintf("%.0f", *event.EndTime)
		duration := *event.EndTime - event.StartTime
		data["duration"] = fmt.Sprintf("%.1fs", duration)
	} else {
		data["status"] = "ongoing"
	}
	
	// Add zones if present
	if len(event.Zones) > 0 {
		zones := ""
		for i, zone := range event.Zones {
			if i > 0 {
				zones += ","
			}
			zones += zone
		}
		data["zones"] = zones
	}
	
	// Add bounding box information if present
	if len(event.Box) >= 4 {
		data["box_x"] = fmt.Sprintf("%.0f", event.Box[0])
		data["box_y"] = fmt.Sprintf("%.0f", event.Box[1])
		data["box_width"] = fmt.Sprintf("%.0f", event.Box[2])
		data["box_height"] = fmt.Sprintf("%.0f", event.Box[3])
	}
	
	// Add HomeLink-specific timestamp
	data["homelink_timestamp"] = fmt.Sprintf("%d", time.Now().Unix())
	
	// Add URL to view the event in Frigate (if thumbnails are available)
	if event.HasSnapshot {
		data["frigate_thumbnail_url"] = fmt.Sprintf("/api/events/%s/thumbnail.jpg", event.ID)
	}
	
	if event.HasClip {
		data["frigate_clip_url"] = fmt.Sprintf("/api/events/%s/clip.mp4", event.ID)
	}
	
	return data
}

// ShouldProcessEvent determines if a Frigate event should be processed
func (et *EventTransformer) ShouldProcessEvent(event *FrigateEvent, config *Config) bool {
	// Skip false positives
	if event.False_positive {
		log.Printf("Skipping false positive event: %s", event.ID)
		return false
	}
	
	// Note: Confidence filtering removed - we want all detected objects
	
	// Check camera filter
	if len(config.EnabledCameras) > 0 {
		cameraEnabled := false
		for _, camera := range config.EnabledCameras {
			if camera == event.Camera {
				cameraEnabled = true
				break
			}
		}
		if !cameraEnabled {
			log.Printf("Skipping event from disabled camera: %s", event.Camera)
			return false
		}
	}
	
	// Check event type filter
	if len(config.EnabledEventTypes) > 0 {
		labelEnabled := false
		for _, label := range config.EnabledEventTypes {
			if label == event.Label {
				labelEnabled = true
				break
			}
		}
		if !labelEnabled {
			log.Printf("Skipping event with disabled label: %s", event.Label)
			return false
		}
	}
	
	// Check event age
	eventTime := time.Unix(int64(event.StartTime), 0)
	if time.Since(eventTime) > config.MaxEventAge {
		log.Printf("Skipping old event: %s (age: %v)", event.ID, time.Since(eventTime))
		return false
	}
	
	return true
}

// capitalize capitalizes the first letter of a string
func (et *EventTransformer) capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return fmt.Sprintf("%c%s", s[0]-32, s[1:])
}

// GetEventPriority determines the priority level for a HomeLink event
func (et *EventTransformer) GetEventPriority(event *FrigateEvent) string {
	// High priority for person detection
	if event.Label == "person" && event.Score > 0.9 {
		return "high"
	}
	
	// High priority for vehicles in specific zones (if configured)
	if (event.Label == "car" || event.Label == "truck") && event.Score > 0.8 {
		return "high"
	}
	
	// Normal priority for most events
	if event.Score > 0.8 {
		return "normal"
	}
	
	// Low priority for lower confidence events
	return "low"
}