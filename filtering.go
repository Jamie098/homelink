// HomeLink - Event Filtering and Deduplication System
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"crypto/md5"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
)

// EventFilter defines filtering criteria for events
type EventFilter struct {
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	EventTypes  []string          `json:"event_types"`  // Filter by event type
	DeviceIDs   []string          `json:"device_ids"`   // Filter by device
	MinInterval time.Duration     `json:"min_interval"` // Minimum time between similar events
	Conditions  []FilterCondition `json:"conditions"`   // Advanced conditions
	Actions     FilterAction      `json:"actions"`      // What to do with matched events
}

// FilterCondition represents a single filtering condition
type FilterCondition struct {
	Field    string         `json:"field"`    // Field to check (confidence, zone, etc.)
	Operator string         `json:"operator"` // ==, !=, >, <, contains, matches
	Value    interface{}    `json:"value"`    // Value to compare against
	Regex    *regexp.Regexp `json:"-"`        // Compiled regex for 'matches' operator
}

// FilterAction defines what to do when filter matches
type FilterAction struct {
	Type     string   `json:"type"`     // allow, deny, modify, rate_limit
	Priority *string  `json:"priority"` // Change message priority
	Throttle *int     `json:"throttle"` // Max events per minute
	Tags     []string `json:"tags"`     // Add tags to event
}

// EventDeduplicator manages duplicate event detection
type EventDeduplicator struct {
	eventHashes map[string]*DuplicateInfo
	mutex       sync.RWMutex
	window      time.Duration
	maxEntries  int
}

// DuplicateInfo tracks information about seen events
type DuplicateInfo struct {
	FirstSeen time.Time
	LastSeen  time.Time
	Count     int
	Hash      string
}

// FilteringSystem manages all event filtering and deduplication
type FilteringSystem struct {
	filters      []EventFilter
	deduplicator *EventDeduplicator
	mutex        sync.RWMutex
	stats        FilteringStats
}

// FilteringStats tracks filtering system performance
type FilteringStats struct {
	EventsProcessed  uint64            `json:"events_processed"`
	EventsFiltered   uint64            `json:"events_filtered"`
	EventsDuplicated uint64            `json:"events_duplicated"`
	EventsModified   uint64            `json:"events_modified"`
	FilterHits       map[string]uint64 `json:"filter_hits"`
	LastReset        time.Time         `json:"last_reset"`
}

// FilterResult represents the result of filtering an event
type FilterResult struct {
	Action     string                 `json:"action"`      // allow, deny, modify
	Modified   bool                   `json:"modified"`    // Whether event was modified
	Tags       []string               `json:"tags"`        // Tags added to event
	Reason     string                 `json:"reason"`      // Why this action was taken
	FilterName string                 `json:"filter_name"` // Which filter triggered
	NewData    map[string]interface{} `json:"new_data"`    // Modified event data
}

// NewEventDeduplicator creates a new deduplication system
func NewEventDeduplicator(window time.Duration, maxEntries int) *EventDeduplicator {
	ed := &EventDeduplicator{
		eventHashes: make(map[string]*DuplicateInfo),
		window:      window,
		maxEntries:  maxEntries,
	}

	// Start cleanup routine
	go ed.cleanupRoutine()

	return ed
}

// NewFilteringSystem creates a comprehensive filtering system
func NewFilteringSystem(dedupeWindow time.Duration, maxCacheEntries int) *FilteringSystem {
	return &FilteringSystem{
		filters:      make([]EventFilter, 0),
		deduplicator: NewEventDeduplicator(dedupeWindow, maxCacheEntries),
		stats: FilteringStats{
			FilterHits: make(map[string]uint64),
			LastReset:  time.Now(),
		},
	}
}

// AddFilter adds a new event filter
func (fs *FilteringSystem) AddFilter(filter EventFilter) error {
	// Compile regex patterns
	for i := range filter.Conditions {
		if filter.Conditions[i].Operator == "matches" {
			if pattern, ok := filter.Conditions[i].Value.(string); ok {
				regex, err := regexp.Compile(pattern)
				if err != nil {
					return fmt.Errorf("invalid regex pattern in filter %s: %v", filter.Name, err)
				}
				filter.Conditions[i].Regex = regex
			}
		}
	}

	fs.mutex.Lock()
	fs.filters = append(fs.filters, filter)
	fs.mutex.Unlock()

	log.Printf("Added event filter: %s", filter.Name)
	return nil
}

// RemoveFilter removes a filter by name
func (fs *FilteringSystem) RemoveFilter(name string) bool {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	for i, filter := range fs.filters {
		if filter.Name == name {
			fs.filters = append(fs.filters[:i], fs.filters[i+1:]...)
			log.Printf("Removed event filter: %s", name)
			return true
		}
	}
	return false
}

// ProcessEvent filters and deduplicates an event
func (fs *FilteringSystem) ProcessEvent(msg *Message) *FilterResult {
	fs.mutex.Lock()
	fs.stats.EventsProcessed++
	fs.mutex.Unlock()

	// Check for duplicates first
	if fs.deduplicator.IsDuplicate(msg) {
		fs.mutex.Lock()
		fs.stats.EventsDuplicated++
		fs.mutex.Unlock()

		return &FilterResult{
			Action: "deny",
			Reason: "duplicate event",
		}
	}

	// Apply filters
	fs.mutex.RLock()
	filters := make([]EventFilter, len(fs.filters))
	copy(filters, fs.filters)
	fs.mutex.RUnlock()

	for _, filter := range filters {
		if !filter.Enabled {
			continue
		}

		if result := fs.applyFilter(msg, filter); result != nil {
			// Update stats
			fs.mutex.Lock()
			fs.stats.FilterHits[filter.Name]++
			if result.Action == "deny" {
				fs.stats.EventsFiltered++
			} else if result.Modified {
				fs.stats.EventsModified++
			}
			fs.mutex.Unlock()

			result.FilterName = filter.Name
			return result
		}
	}

	// No filters matched, allow the event
	return &FilterResult{
		Action: "allow",
		Reason: "no filters matched",
	}
}

// applyFilter applies a single filter to an event
func (fs *FilteringSystem) applyFilter(msg *Message, filter EventFilter) *FilterResult {
	// Check event type filter
	if len(filter.EventTypes) > 0 {
		matched := false
		for _, eventType := range filter.EventTypes {
			if string(msg.Type) == eventType {
				matched = true
				break
			}
		}
		if !matched {
			return nil // Filter doesn't apply to this event type
		}
	}

	// Check device ID filter
	if len(filter.DeviceIDs) > 0 {
		matched := false
		for _, deviceID := range filter.DeviceIDs {
			if msg.DeviceID == deviceID {
				matched = true
				break
			}
		}
		if !matched {
			return nil // Filter doesn't apply to this device
		}
	}

	// Check minimum interval
	if filter.MinInterval > 0 {
		eventKey := fmt.Sprintf("%s:%s", msg.DeviceID, msg.Type)
		if fs.deduplicator.IsWithinInterval(eventKey, filter.MinInterval) {
			return &FilterResult{
				Action: "deny",
				Reason: fmt.Sprintf("within minimum interval of %s", filter.MinInterval),
			}
		}
	}

	// Evaluate conditions
	if !fs.evaluateConditions(msg, filter.Conditions) {
		return nil // Conditions not met
	}

	// Apply actions
	return fs.applyFilterActions(msg, filter.Actions)
}

// evaluateConditions checks if all filter conditions are met
func (fs *FilteringSystem) evaluateConditions(msg *Message, conditions []FilterCondition) bool {
	if len(conditions) == 0 {
		return true // No conditions means always match
	}

	for _, condition := range conditions {
		if !fs.evaluateCondition(msg, condition) {
			return false
		}
	}
	return true
}

// evaluateCondition evaluates a single condition
func (fs *FilteringSystem) evaluateCondition(msg *Message, condition FilterCondition) bool {
	var fieldValue interface{}

	// Extract field value from message
	switch condition.Field {
	case "device_id":
		fieldValue = msg.DeviceID
	case "type":
		fieldValue = string(msg.Type)
	case "timestamp":
		fieldValue = msg.Timestamp
	default:
		// Check in message data
		if dataMap, ok := msg.Data.(map[string]interface{}); ok {
			if val, exists := dataMap[condition.Field]; exists {
				fieldValue = val
			}
		} else if dataMap, ok := msg.Data.(map[string]string); ok {
			if val, exists := dataMap[condition.Field]; exists {
				fieldValue = val
			}
		}
	}

	if fieldValue == nil {
		return false
	}

	// Apply operator
	switch condition.Operator {
	case "==":
		return fieldValue == condition.Value
	case "!=":
		return fieldValue != condition.Value
	case ">":
		return fs.compareNumbers(fieldValue, condition.Value, ">")
	case "<":
		return fs.compareNumbers(fieldValue, condition.Value, "<")
	case ">=":
		return fs.compareNumbers(fieldValue, condition.Value, ">=")
	case "<=":
		return fs.compareNumbers(fieldValue, condition.Value, "<=")
	case "contains":
		if fv, ok := fieldValue.(string); ok {
			if cv, ok := condition.Value.(string); ok {
				return strings.Contains(fv, cv)
			}
		}
	case "matches":
		if fv, ok := fieldValue.(string); ok && condition.Regex != nil {
			return condition.Regex.MatchString(fv)
		}
	}

	return false
}

// compareNumbers compares numeric values
func (fs *FilteringSystem) compareNumbers(a, b interface{}, operator string) bool {
	// Convert to float64 for comparison
	var fa, fb float64
	var ok bool

	switch v := a.(type) {
	case float64:
		fa = v
		ok = true
	case float32:
		fa = float64(v)
		ok = true
	case int:
		fa = float64(v)
		ok = true
	case int64:
		fa = float64(v)
		ok = true
	case string:
		// Try to parse as number
		if parsed, err := parseNumber(v); err == nil {
			fa = parsed
			ok = true
		}
	}

	if !ok {
		return false
	}

	switch v := b.(type) {
	case float64:
		fb = v
	case float32:
		fb = float64(v)
	case int:
		fb = float64(v)
	case int64:
		fb = float64(v)
	case string:
		if parsed, err := parseNumber(v); err == nil {
			fb = parsed
		} else {
			return false
		}
	default:
		return false
	}

	switch operator {
	case ">":
		return fa > fb
	case "<":
		return fa < fb
	case ">=":
		return fa >= fb
	case "<=":
		return fa <= fb
	}

	return false
}

// parseNumber attempts to parse a string as a number
func parseNumber(s string) (float64, error) {
	// Try parsing as float
	if val, err := fmt.Sscanf(s, "%f", new(float64)); err == nil && val == 1 {
		var result float64
		fmt.Sscanf(s, "%f", &result)
		return result, nil
	}
	return 0, fmt.Errorf("not a number")
}

// applyFilterActions applies filter actions to an event
func (fs *FilteringSystem) applyFilterActions(msg *Message, actions FilterAction) *FilterResult {
	result := &FilterResult{
		Action:   actions.Type,
		Modified: false,
		Tags:     make([]string, 0),
		NewData:  make(map[string]interface{}),
	}

	switch actions.Type {
	case "allow":
		result.Reason = "explicitly allowed by filter"
	case "deny":
		result.Reason = "explicitly denied by filter"
	case "modify":
		result.Modified = true
		result.Reason = "event modified by filter"

		// Apply modifications
		if actions.Priority != nil {
			result.NewData["priority"] = *actions.Priority
		}

		if len(actions.Tags) > 0 {
			result.Tags = actions.Tags
			result.NewData["tags"] = actions.Tags
		}

	case "rate_limit":
		if actions.Throttle != nil {
			// Check if we're within rate limit
			eventKey := fmt.Sprintf("%s:%s", msg.DeviceID, msg.Type)
			if fs.deduplicator.CheckRateLimit(eventKey, *actions.Throttle) {
				result.Action = "allow"
				result.Reason = "within rate limit"
			} else {
				result.Action = "deny"
				result.Reason = fmt.Sprintf("rate limit exceeded (%d/min)", *actions.Throttle)
			}
		}
	}

	return result
}

// IsDuplicate checks if an event is a duplicate
func (ed *EventDeduplicator) IsDuplicate(msg *Message) bool {
	hash := ed.generateEventHash(msg)

	ed.mutex.Lock()
	defer ed.mutex.Unlock()

	now := time.Now()

	if info, exists := ed.eventHashes[hash]; exists {
		// Check if within deduplication window
		if now.Sub(info.LastSeen) < ed.window {
			info.LastSeen = now
			info.Count++
			return true
		}
		// Outside window, treat as new event
		info.FirstSeen = now
		info.LastSeen = now
		info.Count = 1
		return false
	}

	// New event, add to tracking
	ed.eventHashes[hash] = &DuplicateInfo{
		FirstSeen: now,
		LastSeen:  now,
		Count:     1,
		Hash:      hash,
	}

	// Cleanup if we have too many entries
	if len(ed.eventHashes) > ed.maxEntries {
		ed.cleanup()
	}

	return false
}

// IsWithinInterval checks if an event is within minimum interval
func (ed *EventDeduplicator) IsWithinInterval(eventKey string, interval time.Duration) bool {
	ed.mutex.RLock()
	defer ed.mutex.RUnlock()

	if info, exists := ed.eventHashes[eventKey]; exists {
		return time.Since(info.LastSeen) < interval
	}

	return false
}

// CheckRateLimit checks if event is within rate limit
func (ed *EventDeduplicator) CheckRateLimit(eventKey string, maxPerMinute int) bool {
	ed.mutex.Lock()
	defer ed.mutex.Unlock()

	now := time.Now()
	minuteAgo := now.Add(-1 * time.Minute)

	// Count events in the last minute
	count := 0
	for _, info := range ed.eventHashes {
		if strings.HasPrefix(info.Hash, eventKey) && info.LastSeen.After(minuteAgo) {
			count++
		}
	}

	return count < maxPerMinute
}

// generateEventHash creates a hash for deduplication
func (ed *EventDeduplicator) generateEventHash(msg *Message) string {
	// Create hash based on device, type, and key data fields
	hashInput := fmt.Sprintf("%s:%s:%d", msg.DeviceID, msg.Type, msg.Timestamp/60) // Minute precision

	// Add important data fields
	if dataMap, ok := msg.Data.(map[string]interface{}); ok {
		// Include key fields that should be considered for deduplication
		keyFields := []string{"camera", "zone", "event_type", "confidence"}
		for _, field := range keyFields {
			if val, exists := dataMap[field]; exists {
				hashInput += fmt.Sprintf(":%v", val)
			}
		}
	}

	hash := md5.Sum([]byte(hashInput))
	return fmt.Sprintf("%x", hash)
}

// cleanup removes old entries from the cache
func (ed *EventDeduplicator) cleanup() {
	cutoff := time.Now().Add(-ed.window * 2)

	for hash, info := range ed.eventHashes {
		if info.LastSeen.Before(cutoff) {
			delete(ed.eventHashes, hash)
		}
	}
}

// cleanupRoutine runs periodic cleanup
func (ed *EventDeduplicator) cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C
		ed.mutex.Lock()
		ed.cleanup()
		ed.mutex.Unlock()
	}
}

// GetStats returns filtering system statistics
func (fs *FilteringSystem) GetStats() FilteringStats {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	// Create a copy to avoid race conditions
	stats := fs.stats
	stats.FilterHits = make(map[string]uint64)
	for k, v := range fs.stats.FilterHits {
		stats.FilterHits[k] = v
	}

	return stats
}

// ResetStats resets all statistics
func (fs *FilteringSystem) ResetStats() {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	fs.stats = FilteringStats{
		FilterHits: make(map[string]uint64),
		LastReset:  time.Now(),
	}
}

// GetFilterCount returns the number of active filters
func (fs *FilteringSystem) GetFilterCount() int {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	return len(fs.filters)
}
