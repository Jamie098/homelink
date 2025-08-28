// HomeLink - Frigate Integration
// Privacy-first security camera integration for motion detection and alerts
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FrigateEvent represents a webhook event from Frigate
type FrigateEvent struct {
	// Core Frigate event fields
	Type         string  `json:"type"`          // "new", "update", "end"
	BeforeID     string  `json:"before_id"`     // Event ID
	AfterID      string  `json:"after_id"`      // Event ID  
	Camera       string  `json:"camera"`        // Camera name
	Label        string  `json:"label"`         // Detected object label (person, car, etc.)
	SubLabel     string  `json:"sub_label"`     // Sub-classification
	TopScore     float64 `json:"top_score"`     // Confidence score (0-1)
	FalsePositive bool   `json:"false_positive"` // Whether marked as false positive
	StartTime    float64 `json:"start_time"`    // Unix timestamp
	EndTime      float64 `json:"end_time"`      // Unix timestamp
	Score        float64 `json:"score"`         // Current confidence score
	Box          []int   `json:"box"`           // Bounding box coordinates [x1, y1, x2, y2]
	Area         int     `json:"area"`          // Area of bounding box
	Ratio        float64 `json:"ratio"`         // Width/height ratio
	Region       []int   `json:"region"`        // Region coordinates
	CurrentZones []string `json:"current_zones"` // Active zones
	EnteredZones []string `json:"entered_zones"` // Zones object entered
	HasSnapshot  bool     `json:"has_snapshot"`  // Whether snapshot is available
	HasClip      bool     `json:"has_clip"`      // Whether video clip is available
	Stationary   bool     `json:"stationary"`    // Whether object is stationary
	MotionlessCount int  `json:"motionless_count"` // Frames without motion
	PositionChanges int  `json:"position_changes"` // Position change count
	
	// Frigate URLs (constructed from base URL)
	FrigateBaseURL string `json:"-"` // Base URL for Frigate server (set by handler)
}

// FrigateConfig holds configuration for Frigate integration
type FrigateConfig struct {
	Enabled              bool          `json:"enabled"`
	FrigateBaseURL       string        `json:"frigate_base_url"`        // e.g., "http://frigate.local:5000"
	SnapshotCache        bool          `json:"snapshot_cache"`          // Whether to cache snapshots locally
	SnapshotCacheDir     string        `json:"snapshot_cache_dir"`      // Directory for snapshot cache
	SnapshotCacheTTL     time.Duration `json:"snapshot_cache_ttl"`      // How long to keep cached snapshots
	NotifyOnEvents       []string      `json:"notify_on_events"`        // Event types to notify on ["new", "update", "end"]
	NotifyOnLabels       []string      `json:"notify_on_labels"`        // Object labels to notify on ["person", "car", etc.]
	MinimumScore         float64       `json:"minimum_score"`           // Minimum confidence score (0-1)
	IgnoreFalsePositives bool          `json:"ignore_false_positives"`  // Skip events marked as false positive
	ZoneFilter           []string      `json:"zone_filter"`             // Only notify for specific zones (empty = all)
	WebhookSecret        string        `json:"webhook_secret"`          // Optional webhook authentication secret
}

// FrigateIntegration manages Frigate NVR integration
type FrigateIntegration struct {
	config      *FrigateConfig
	service     *HomeLinkService
	httpClient  *http.Client
	snapshots   map[string]*CachedSnapshot // Event ID -> snapshot data
	snapshotMux sync.RWMutex
}

// CachedSnapshot represents a cached snapshot from Frigate
type CachedSnapshot struct {
	EventID     string    `json:"event_id"`
	Camera      string    `json:"camera"`
	FilePath    string    `json:"file_path"`    // Local cache file path
	URL         string    `json:"url"`          // Original Frigate URL
	ContentType string    `json:"content_type"` // Image MIME type
	Size        int64     `json:"size"`         // File size in bytes
	CachedAt    time.Time `json:"cached_at"`    // When snapshot was cached
	ExpiresAt   time.Time `json:"expires_at"`   // When cache expires
}

// FrigateNotificationData represents HomeLink notification data for Frigate events
type FrigateNotificationData struct {
	Type         string    `json:"type"`
	EventID      string    `json:"event_id"`
	Camera       string    `json:"camera"`
	Label        string    `json:"label"`
	SubLabel     string    `json:"sub_label"`
	Score        float64   `json:"score"`
	TopScore     float64   `json:"top_score"`
	Zones        []string  `json:"zones"`
	Box          []int     `json:"box"`
	Area         int       `json:"area"`
	HasSnapshot  bool      `json:"has_snapshot"`
	HasClip      bool      `json:"has_clip"`
	SnapshotURL  string    `json:"snapshot_url"`  // HomeLink snapshot URL
	ClipURL      string    `json:"clip_url"`      // Frigate clip URL
	FrigateURL   string    `json:"frigate_url"`   // Direct link to Frigate event
	Timestamp    time.Time `json:"timestamp"`
	Duration     float64   `json:"duration"`      // Event duration in seconds
	Stationary   bool      `json:"stationary"`
}

// NewFrigateIntegration creates a new Frigate integration
func NewFrigateIntegration(config *FrigateConfig, service *HomeLinkService) *FrigateIntegration {
	if config.SnapshotCacheDir == "" {
		config.SnapshotCacheDir = "./frigate_snapshots"
	}
	if config.SnapshotCacheTTL == 0 {
		config.SnapshotCacheTTL = 24 * time.Hour // Default 24 hours
	}
	if config.MinimumScore == 0 {
		config.MinimumScore = 0.7 // Default 70% confidence
	}
	if len(config.NotifyOnEvents) == 0 {
		config.NotifyOnEvents = []string{"new"} // Only notify on new events by default
	}

	// Create snapshot cache directory
	if config.SnapshotCache {
		if err := os.MkdirAll(config.SnapshotCacheDir, 0755); err != nil {
			log.Printf("Failed to create snapshot cache directory: %v", err)
			config.SnapshotCache = false
		}
	}

	return &FrigateIntegration{
		config: config,
		service: service,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		snapshots: make(map[string]*CachedSnapshot),
	}
}

// HandleWebhook processes incoming Frigate webhooks
func (f *FrigateIntegration) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify webhook secret if configured
	if f.config.WebhookSecret != "" {
		providedSecret := r.Header.Get("X-Frigate-Secret")
		if providedSecret != f.config.WebhookSecret {
			http.Error(w, "Invalid webhook secret", http.StatusUnauthorized)
			return
		}
	}

	// Parse webhook payload
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read webhook body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var frigateEvent FrigateEvent
	if err := json.Unmarshal(body, &frigateEvent); err != nil {
		log.Printf("Failed to parse Frigate webhook: %v", err)
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Set the Frigate base URL for constructing URLs
	frigateEvent.FrigateBaseURL = f.config.FrigateBaseURL

	// Process the event
	if err := f.processEvent(&frigateEvent); err != nil {
		log.Printf("Failed to process Frigate event: %v", err)
		http.Error(w, "Failed to process event", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true, "message": "Event processed"}`))
}

// processEvent processes a Frigate event and sends appropriate HomeLink notifications
func (f *FrigateIntegration) processEvent(event *FrigateEvent) error {
	// Apply filters
	if !f.shouldNotifyForEvent(event) {
		log.Printf("Skipping event %s - filtered out", event.AfterID)
		return nil
	}

	log.Printf("Processing Frigate event: %s - %s %s on %s (score: %.2f)", 
		event.Type, event.Label, event.SubLabel, event.Camera, event.Score)

	// Cache snapshot if enabled and available
	var snapshotURL string
	if f.config.SnapshotCache && event.HasSnapshot {
		cached, err := f.cacheSnapshot(event)
		if err != nil {
			log.Printf("Failed to cache snapshot for event %s: %v", event.AfterID, err)
		} else if cached != nil {
			// Provide HomeLink-hosted snapshot URL
			snapshotURL = fmt.Sprintf("/frigate/snapshot/%s", event.AfterID)
		}
	}

	// Create HomeLink notification data (for future webhook extensions)
	_ = f.createNotificationData(event, snapshotURL)

	// Send to HomeLink network based on event type
	eventType := fmt.Sprintf("frigate_%s", event.Type) // "frigate_new", "frigate_update", "frigate_end"
	description := f.generateEventDescription(event)

	// Convert notification data to map[string]string for HomeLink compatibility
	data := map[string]string{
		"source":       "frigate",
		"event_type":   event.Type,
		"event_id":     event.AfterID,
		"camera":       event.Camera,
		"label":        event.Label,
		"score":        fmt.Sprintf("%.2f", event.Score),
		"has_snapshot": fmt.Sprintf("%t", event.HasSnapshot),
		"has_clip":     fmt.Sprintf("%t", event.HasClip),
		"timestamp":    fmt.Sprintf("%.0f", event.StartTime),
		"zones":        strings.Join(event.CurrentZones, ","),
	}

	if snapshotURL != "" {
		data["snapshot_url"] = snapshotURL
	}
	if event.HasClip && event.FrigateBaseURL != "" {
		data["clip_url"] = fmt.Sprintf("%s/api/events/%s/clip.mp4", event.FrigateBaseURL, event.AfterID)
	}
	if event.FrigateBaseURL != "" {
		data["frigate_url"] = fmt.Sprintf("%s/events", event.FrigateBaseURL)
	}
	if event.SubLabel != "" {
		data["sub_label"] = event.SubLabel
	}
	if len(event.Box) >= 4 {
		data["box"] = fmt.Sprintf("%d,%d,%d,%d", event.Box[0], event.Box[1], event.Box[2], event.Box[3])
		data["area"] = fmt.Sprintf("%d", event.Area)
	}

	// Determine priority based on event characteristics
	priority := f.determineEventPriority(event)

	// Send as reliable event if high priority, otherwise normal event
	if priority >= PriorityHigh {
		f.service.SendReliableEvent(eventType, description, data, priority)
	} else {
		f.service.SendEvent(eventType, description, data)
	}

	return nil
}

// shouldNotifyForEvent determines if we should send notifications for this event
func (f *FrigateIntegration) shouldNotifyForEvent(event *FrigateEvent) bool {
	// Check if we should notify for this event type
	notifyForType := false
	for _, eventType := range f.config.NotifyOnEvents {
		if eventType == event.Type {
			notifyForType = true
			break
		}
	}
	if !notifyForType {
		return false
	}

	// Check if we should notify for this label
	if len(f.config.NotifyOnLabels) > 0 {
		notifyForLabel := false
		for _, label := range f.config.NotifyOnLabels {
			if label == event.Label {
				notifyForLabel = true
				break
			}
		}
		if !notifyForLabel {
			return false
		}
	}

	// Check minimum score threshold
	if event.Score < f.config.MinimumScore {
		return false
	}

	// Check if we should ignore false positives
	if f.config.IgnoreFalsePositives && event.FalsePositive {
		return false
	}

	// Check zone filter
	if len(f.config.ZoneFilter) > 0 {
		inFilteredZone := false
		for _, zone := range f.config.ZoneFilter {
			for _, eventZone := range event.CurrentZones {
				if zone == eventZone {
					inFilteredZone = true
					break
				}
			}
			if inFilteredZone {
				break
			}
		}
		if !inFilteredZone {
			return false
		}
	}

	return true
}

// cacheSnapshot downloads and caches a snapshot from Frigate
func (f *FrigateIntegration) cacheSnapshot(event *FrigateEvent) (*CachedSnapshot, error) {
	if event.FrigateBaseURL == "" {
		return nil, fmt.Errorf("Frigate base URL not configured")
	}

	eventID := event.AfterID
	if eventID == "" {
		eventID = event.BeforeID
	}
	if eventID == "" {
		return nil, fmt.Errorf("no event ID available")
	}

	// Check if already cached
	f.snapshotMux.RLock()
	if cached, exists := f.snapshots[eventID]; exists && time.Now().Before(cached.ExpiresAt) {
		f.snapshotMux.RUnlock()
		return cached, nil
	}
	f.snapshotMux.RUnlock()

	// Download snapshot from Frigate
	snapshotURL := fmt.Sprintf("%s/api/events/%s/snapshot.jpg", event.FrigateBaseURL, eventID)
	resp, err := f.httpClient.Get(snapshotURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download snapshot: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download snapshot: HTTP %d", resp.StatusCode)
	}

	// Read snapshot data
	snapshotData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot data: %v", err)
	}

	// Save to cache directory
	filename := fmt.Sprintf("%s_%s_%d.jpg", event.Camera, eventID, time.Now().Unix())
	filepath := filepath.Join(f.config.SnapshotCacheDir, filename)
	
	if err := os.WriteFile(filepath, snapshotData, 0644); err != nil {
		return nil, fmt.Errorf("failed to save snapshot to cache: %v", err)
	}

	// Create cached snapshot record
	cached := &CachedSnapshot{
		EventID:     eventID,
		Camera:      event.Camera,
		FilePath:    filepath,
		URL:         snapshotURL,
		ContentType: resp.Header.Get("Content-Type"),
		Size:        int64(len(snapshotData)),
		CachedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(f.config.SnapshotCacheTTL),
	}

	// Store in memory cache
	f.snapshotMux.Lock()
	f.snapshots[eventID] = cached
	f.snapshotMux.Unlock()

	log.Printf("Cached snapshot for event %s: %s (%d bytes)", eventID, filename, len(snapshotData))
	return cached, nil
}

// GetCachedSnapshot retrieves a cached snapshot by event ID
func (f *FrigateIntegration) GetCachedSnapshot(eventID string) (*CachedSnapshot, error) {
	f.snapshotMux.RLock()
	cached, exists := f.snapshots[eventID]
	f.snapshotMux.RUnlock()

	if !exists {
		return nil, fmt.Errorf("snapshot not found in cache")
	}

	if time.Now().After(cached.ExpiresAt) {
		// Snapshot expired, remove from cache
		f.snapshotMux.Lock()
		delete(f.snapshots, eventID)
		f.snapshotMux.Unlock()
		
		// Clean up file
		os.Remove(cached.FilePath)
		return nil, fmt.Errorf("snapshot expired")
	}

	return cached, nil
}

// ServeSnapshot serves cached snapshots via HTTP
func (f *FrigateIntegration) ServeSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract event ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/frigate/snapshot/")
	eventID := strings.Split(path, "/")[0] // Handle potential additional path components

	cached, err := f.GetCachedSnapshot(eventID)
	if err != nil {
		http.Error(w, "Snapshot not found", http.StatusNotFound)
		return
	}

	// Serve the cached file
	w.Header().Set("Content-Type", cached.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
	w.Header().Set("X-Frigate-Event-ID", cached.EventID)
	w.Header().Set("X-Frigate-Camera", cached.Camera)
	
	http.ServeFile(w, r, cached.FilePath)
}

// createNotificationData creates structured notification data for HomeLink
func (f *FrigateIntegration) createNotificationData(event *FrigateEvent, snapshotURL string) *FrigateNotificationData {
	data := &FrigateNotificationData{
		Type:        event.Type,
		EventID:     event.AfterID,
		Camera:      event.Camera,
		Label:       event.Label,
		SubLabel:    event.SubLabel,
		Score:       event.Score,
		TopScore:    event.TopScore,
		Zones:       event.CurrentZones,
		Box:         event.Box,
		Area:        event.Area,
		HasSnapshot: event.HasSnapshot,
		HasClip:     event.HasClip,
		Timestamp:   time.Unix(int64(event.StartTime), 0),
		Stationary:  event.Stationary,
	}

	// Add URLs
	if snapshotURL != "" {
		data.SnapshotURL = snapshotURL
	}
	if event.HasClip && event.FrigateBaseURL != "" {
		data.ClipURL = fmt.Sprintf("%s/api/events/%s/clip.mp4", event.FrigateBaseURL, event.AfterID)
	}
	if event.FrigateBaseURL != "" {
		data.FrigateURL = fmt.Sprintf("%s/events", event.FrigateBaseURL)
	}

	// Calculate duration for ended events
	if event.Type == "end" && event.EndTime > event.StartTime {
		data.Duration = event.EndTime - event.StartTime
	}

	return data
}

// generateEventDescription creates a human-readable description for the event
func (f *FrigateIntegration) generateEventDescription(event *FrigateEvent) string {
	confidence := fmt.Sprintf("%.0f%%", event.Score*100)
	
	switch event.Type {
	case "new":
		if event.SubLabel != "" {
			return fmt.Sprintf("New %s %s detected on %s (%s confidence)", 
				event.Label, event.SubLabel, event.Camera, confidence)
		}
		return fmt.Sprintf("New %s detected on %s (%s confidence)", 
			event.Label, event.Camera, confidence)
	case "update":
		return fmt.Sprintf("%s on %s updated (%s confidence)", 
			event.Label, event.Camera, confidence)
	case "end":
		duration := ""
		if event.EndTime > event.StartTime {
			duration = fmt.Sprintf(" (%.1fs)", event.EndTime-event.StartTime)
		}
		return fmt.Sprintf("%s on %s ended%s", event.Label, event.Camera, duration)
	default:
		return fmt.Sprintf("%s %s on %s", event.Label, event.Type, event.Camera)
	}
}

// determineEventPriority assigns priority based on event characteristics
func (f *FrigateIntegration) determineEventPriority(event *FrigateEvent) MessagePriority {
	// High priority for person detection
	if event.Label == "person" && event.Score >= 0.8 {
		return PriorityHigh
	}
	
	// High priority for events in specific zones (if configured)
	if len(f.config.ZoneFilter) > 0 {
		for _, zone := range f.config.ZoneFilter {
			for _, eventZone := range event.CurrentZones {
				if zone == eventZone && strings.Contains(strings.ToLower(zone), "front") {
					return PriorityHigh
				}
			}
		}
	}
	
	// High confidence detections get higher priority
	if event.Score >= 0.9 {
		return PriorityHigh
	}
	
	return PriorityNormal
}

// CleanupExpiredSnapshots removes expired snapshots from cache
func (f *FrigateIntegration) CleanupExpiredSnapshots() {
	f.snapshotMux.Lock()
	defer f.snapshotMux.Unlock()

	now := time.Now()
	var toDelete []string

	for eventID, cached := range f.snapshots {
		if now.After(cached.ExpiresAt) {
			toDelete = append(toDelete, eventID)
			// Delete the file
			if err := os.Remove(cached.FilePath); err != nil {
				log.Printf("Failed to delete expired snapshot %s: %v", cached.FilePath, err)
			}
		}
	}

	for _, eventID := range toDelete {
		delete(f.snapshots, eventID)
	}

	if len(toDelete) > 0 {
		log.Printf("Cleaned up %d expired snapshots", len(toDelete))
	}
}

// GetStats returns statistics about the Frigate integration
func (f *FrigateIntegration) GetStats() map[string]interface{} {
	f.snapshotMux.RLock()
	snapshotCount := len(f.snapshots)
	
	var totalSize int64
	for _, cached := range f.snapshots {
		totalSize += cached.Size
	}
	f.snapshotMux.RUnlock()

	return map[string]interface{}{
		"enabled":           f.config.Enabled,
		"frigate_base_url":  f.config.FrigateBaseURL,
		"cached_snapshots":  snapshotCount,
		"cache_size_bytes":  totalSize,
		"cache_size_mb":     float64(totalSize) / (1024 * 1024),
		"cache_ttl_hours":   f.config.SnapshotCacheTTL.Hours(),
		"minimum_score":     f.config.MinimumScore,
		"notify_on_events":  f.config.NotifyOnEvents,
		"notify_on_labels":  f.config.NotifyOnLabels,
		"zone_filter":       f.config.ZoneFilter,
	}
}