package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// BridgeService coordinates the bridging between Frigate and HomeLink
type BridgeService struct {
	frigateClient  *FrigateClient
	homelinkClient *HomeLinkClient
	transformer    *EventTransformer
	config         *Config

	// State management
	lastPollTime    time.Time
	processedEvents map[string]bool
	eventsMutex     sync.RWMutex

	// Control channels
	stopChan   chan bool
	healthChan chan bool

	// Statistics
	stats *BridgeStats

	// Health check server
	healthServer *http.Server
}

// BridgeStats holds statistics about the bridge operation
type BridgeStats struct {
	mutex              sync.RWMutex
	StartTime          time.Time     `json:"start_time"`
	EventsProcessed    int64         `json:"events_processed"`
	EventsFiltered     int64         `json:"events_filtered"`
	EventsPublished    int64         `json:"events_published"`
	EventsFailed       int64         `json:"events_failed"`
	LastPollTime       time.Time     `json:"last_poll_time"`
	LastSuccessfulPoll time.Time     `json:"last_successful_poll"`
	FrigateErrors      int64         `json:"frigate_errors"`
	HomeLinkErrors     int64         `json:"homelink_errors"`
	Uptime             time.Duration `json:"uptime"`
	HealthStatus       string        `json:"health_status"`
}

// NewBridgeService creates a new bridge service
func NewBridgeService(frigateClient *FrigateClient, homelinkClient *HomeLinkClient, transformer *EventTransformer, config *Config) *BridgeService {
	return &BridgeService{
		frigateClient:   frigateClient,
		homelinkClient:  homelinkClient,
		transformer:     transformer,
		config:          config,
		lastPollTime:    time.Now().Add(-5 * time.Minute), // Start polling from 5 minutes ago to catch recent events
		processedEvents: make(map[string]bool),
		stopChan:        make(chan bool),
		healthChan:      make(chan bool, 1),
		stats: &BridgeStats{
			StartTime:    time.Now(),
			HealthStatus: "starting",
		},
	}
}

// Start begins the bridge service operation
func (bs *BridgeService) Start() error {
	log.Printf("Starting Frigate-HomeLink bridge service")

	// Perform initial health checks
	if err := bs.performHealthChecks(); err != nil {
		return fmt.Errorf("initial health checks failed: %w", err)
	}

	// Start health check HTTP server
	if bs.config.HealthCheckPort > 0 {
		bs.startHealthServer()
	}

	// Start background goroutines
	go bs.pollLoop()
	go bs.cleanupLoop()
	go bs.healthMonitorLoop()

	bs.updateHealthStatus("running")
	log.Printf("Bridge service started successfully")

	return nil
}

// Stop gracefully stops the bridge service
func (bs *BridgeService) Stop() {
	log.Printf("Stopping bridge service...")

	bs.updateHealthStatus("stopping")

	// Signal stop to all goroutines
	close(bs.stopChan)

	// Stop health server
	if bs.healthServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		bs.healthServer.Shutdown(ctx)
	}

	bs.updateHealthStatus("stopped")
	log.Printf("Bridge service stopped")
}

// pollLoop is the main polling loop that checks for new Frigate events
func (bs *BridgeService) pollLoop() {
	ticker := time.NewTicker(bs.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-bs.stopChan:
			return
		case <-ticker.C:
			bs.pollForEvents()
		}
	}
}

// pollForEvents retrieves and processes new events from Frigate
func (bs *BridgeService) pollForEvents() {
	bs.stats.mutex.Lock()
	bs.stats.LastPollTime = time.Now()
	bs.stats.mutex.Unlock()

	log.Printf("Polling Frigate for new events since %s", bs.lastPollTime.Format(time.RFC3339))
	log.Printf("DEBUG: Enabled cameras: %v", bs.config.EnabledCameras)
	log.Printf("DEBUG: Enabled event types: %v", bs.config.EnabledEventTypes)
	log.Printf("DEBUG: Max event age: %v", bs.config.MaxEventAge)

	// Get recent events from Frigate
	events, err := bs.frigateClient.GetRecentEvents(
		bs.lastPollTime,
		bs.config.EnabledCameras,
		bs.config.EnabledEventTypes,
	)

	if err != nil {
		log.Printf("Failed to retrieve events from Frigate: %v", err)
		bs.incrementFrigateErrors()
		return
	}

	bs.stats.mutex.Lock()
	bs.stats.LastSuccessfulPoll = time.Now()
	bs.stats.mutex.Unlock()

	// Process each event
	for _, event := range events {
		bs.processEvent(&event)
	}

	// Update last poll time
	bs.lastPollTime = time.Now()

	log.Printf("Polling complete, processed %d events", len(events))
}

// processEvent handles a single Frigate event
func (bs *BridgeService) processEvent(frigateEvent *FrigateEvent) {
	bs.incrementEventsProcessed()

	// Check if we've already processed this event
	bs.eventsMutex.RLock()
	alreadyProcessed := bs.processedEvents[frigateEvent.ID]
	bs.eventsMutex.RUnlock()

	if alreadyProcessed {
		log.Printf("Skipping already processed event: %s", frigateEvent.ID)
		return
	}

	// Check if event should be processed based on filters
	if !bs.transformer.ShouldProcessEvent(frigateEvent, bs.config) {
		bs.incrementEventsFiltered()
		// Still mark as processed to avoid reprocessing
		bs.markEventProcessed(frigateEvent.ID)
		return
	}

	// Transform the event
	homelinkEvent := bs.transformer.TransformEvent(frigateEvent)

	// Determine if we should use reliable publishing
	priority := bs.transformer.GetEventPriority(frigateEvent)

	// Publish the event
	var err error
	if priority == "high" || priority == "emergency" {
		err = bs.homelinkClient.PublishReliableEvent(homelinkEvent, priority)
	} else {
		err = bs.homelinkClient.PublishEvent(homelinkEvent)
	}

	if err != nil {
		log.Printf("Failed to publish event to HomeLink: %v", err)
		bs.incrementHomeLinkErrors()
		bs.incrementEventsFailed()

		// Implement retry logic
		bs.retryPublishEvent(homelinkEvent, priority, 1)
		return
	}

	// Mark as processed and increment success counter
	bs.markEventProcessed(frigateEvent.ID)
	bs.incrementEventsPublished()

	log.Printf("Successfully processed and published event: %s (%s)", frigateEvent.ID, homelinkEvent.EventType)
}

// retryPublishEvent implements retry logic for failed event publishing
func (bs *BridgeService) retryPublishEvent(event *HomeLinkEvent, priority string, attempt int) {
	if attempt > bs.config.RetryAttempts {
		log.Printf("Max retry attempts reached for event: %s", event.EventType)
		return
	}

	// Wait before retrying
	time.Sleep(bs.config.RetryDelay * time.Duration(attempt))

	log.Printf("Retrying event publication (attempt %d/%d): %s", attempt, bs.config.RetryAttempts, event.EventType)

	var err error
	if priority == "high" || priority == "emergency" {
		err = bs.homelinkClient.PublishReliableEvent(event, priority)
	} else {
		err = bs.homelinkClient.PublishEvent(event)
	}

	if err != nil {
		log.Printf("Retry %d failed for event: %v", attempt, err)
		bs.retryPublishEvent(event, priority, attempt+1)
		return
	}

	bs.incrementEventsPublished()
	log.Printf("Event successfully published on retry %d: %s", attempt, event.EventType)
}

// cleanupLoop periodically cleans up old processed event IDs to prevent memory growth
func (bs *BridgeService) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Minute) // Clean up every 30 minutes
	defer ticker.Stop()

	for {
		select {
		case <-bs.stopChan:
			return
		case <-ticker.C:
			bs.cleanupProcessedEvents()
		}
	}
}

// cleanupProcessedEvents removes old event IDs from the processed events map
func (bs *BridgeService) cleanupProcessedEvents() {
	bs.eventsMutex.Lock()
	defer bs.eventsMutex.Unlock()

	initialCount := len(bs.processedEvents)

	// Keep the map from growing too large - only keep recent events
	if len(bs.processedEvents) > bs.config.EventBuffer {
		// Simple cleanup: clear half the entries
		// In a production system, you might want to use timestamps
		count := 0
		targetCount := bs.config.EventBuffer / 2

		for id := range bs.processedEvents {
			if count >= targetCount {
				break
			}
			delete(bs.processedEvents, id)
			count++
		}

		log.Printf("Cleaned up processed events: %d -> %d", initialCount, len(bs.processedEvents))
	}
}

// healthMonitorLoop monitors the health of the bridge service
func (bs *BridgeService) healthMonitorLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-bs.stopChan:
			return
		case <-ticker.C:
			bs.checkHealth()
		}
	}
}

// checkHealth performs health checks and updates status
func (bs *BridgeService) checkHealth() {
	// Check if we've been polling recently
	timeSinceLastPoll := time.Since(bs.stats.LastPollTime)
	if timeSinceLastPoll > bs.config.PollInterval*3 {
		bs.updateHealthStatus("unhealthy - polling stalled")
		return
	}

	// Check if we've had recent successful polls
	timeSinceLastSuccess := time.Since(bs.stats.LastSuccessfulPoll)
	if timeSinceLastSuccess > bs.config.PollInterval*10 {
		bs.updateHealthStatus("unhealthy - no successful polls")
		return
	}

	bs.updateHealthStatus("healthy")
}

// performHealthChecks performs initial health checks on startup
func (bs *BridgeService) performHealthChecks() error {
	log.Printf("Performing initial health checks...")

	// Check Frigate connectivity
	if err := bs.frigateClient.HealthCheck(); err != nil {
		return fmt.Errorf("frigate health check failed: %w", err)
	}
	log.Printf("Frigate health check passed")

	// Check HomeLink connectivity
	if err := bs.homelinkClient.HealthCheck(); err != nil {
		return fmt.Errorf("homelink health check failed: %w", err)
	}
	log.Printf("HomeLink health check passed")

	return nil
}

// startHealthServer starts the HTTP health check server
func (bs *BridgeService) startHealthServer() {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", bs.handleHealth)
	mux.HandleFunc("/stats", bs.handleStats)
	mux.HandleFunc("/", bs.handleRoot)

	bs.healthServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", bs.config.HealthCheckPort),
		Handler: mux,
	}

	go func() {
		log.Printf("Health check server starting on port %d", bs.config.HealthCheckPort)
		if err := bs.healthServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("Health server error: %v", err)
		}
	}()
}

// HTTP handlers for health check server

func (bs *BridgeService) handleHealth(w http.ResponseWriter, r *http.Request) {
	bs.stats.mutex.RLock()
	status := bs.stats.HealthStatus
	bs.stats.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	fmt.Fprintf(w, `{"status": "%s", "service": "frigate-homelink-bridge"}`, status)
}

func (bs *BridgeService) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := bs.GetStats()
	w.Header().Set("Content-Type", "application/json")

	// Simple JSON encoding
	fmt.Fprintf(w, `{
		"start_time": "%s",
		"uptime": "%s",
		"health_status": "%s",
		"events_processed": %d,
		"events_filtered": %d,
		"events_published": %d,
		"events_failed": %d,
		"last_poll_time": "%s",
		"last_successful_poll": "%s",
		"frigate_errors": %d,
		"homelink_errors": %d
	}`,
		stats.StartTime.Format(time.RFC3339),
		stats.Uptime.String(),
		stats.HealthStatus,
		stats.EventsProcessed,
		stats.EventsFiltered,
		stats.EventsPublished,
		stats.EventsFailed,
		stats.LastPollTime.Format(time.RFC3339),
		stats.LastSuccessfulPoll.Format(time.RFC3339),
		stats.FrigateErrors,
		stats.HomeLinkErrors,
	)
}

func (bs *BridgeService) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<h1>Frigate-HomeLink Bridge</h1>
<p>Status: <strong>%s</strong></p>
<p><a href="/health">Health Check</a> | <a href="/stats">Statistics</a></p>
`, bs.stats.HealthStatus)
}

// Utility methods

func (bs *BridgeService) markEventProcessed(eventID string) {
	bs.eventsMutex.Lock()
	defer bs.eventsMutex.Unlock()
	bs.processedEvents[eventID] = true
}

func (bs *BridgeService) updateHealthStatus(status string) {
	bs.stats.mutex.Lock()
	defer bs.stats.mutex.Unlock()
	bs.stats.HealthStatus = status
}

func (bs *BridgeService) incrementEventsProcessed() {
	bs.stats.mutex.Lock()
	defer bs.stats.mutex.Unlock()
	bs.stats.EventsProcessed++
}

func (bs *BridgeService) incrementEventsFiltered() {
	bs.stats.mutex.Lock()
	defer bs.stats.mutex.Unlock()
	bs.stats.EventsFiltered++
}

func (bs *BridgeService) incrementEventsPublished() {
	bs.stats.mutex.Lock()
	defer bs.stats.mutex.Unlock()
	bs.stats.EventsPublished++
}

func (bs *BridgeService) incrementEventsFailed() {
	bs.stats.mutex.Lock()
	defer bs.stats.mutex.Unlock()
	bs.stats.EventsFailed++
}

func (bs *BridgeService) incrementFrigateErrors() {
	bs.stats.mutex.Lock()
	defer bs.stats.mutex.Unlock()
	bs.stats.FrigateErrors++
}

func (bs *BridgeService) incrementHomeLinkErrors() {
	bs.stats.mutex.Lock()
	defer bs.stats.mutex.Unlock()
	bs.stats.HomeLinkErrors++
}

// GetStats returns the current bridge statistics
func (bs *BridgeService) GetStats() BridgeStats {
	bs.stats.mutex.RLock()
	defer bs.stats.mutex.RUnlock()

	stats := *bs.stats
	stats.Uptime = time.Since(stats.StartTime)
	return stats
}
