package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"homelink"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// EventRequest represents an incoming event via HTTP API
type EventRequest struct {
	EventType   string            `json:"event_type"`
	Description string            `json:"description"`
	Data        map[string]string `json:"data"`
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SecurityRequest represents security-related API requests
type SecurityRequest struct {
	DeviceID  string `json:"device_id"`
	PublicKey string `json:"public_key"`
	QRCode    string `json:"qr_code"`
}

// Global API key for simple authentication
var apiKey string

func main() {
	// Get configuration from environment variables
	deviceID := getEnv("HOMELINK_DEVICE_ID", "homelink-service")
	deviceName := getEnv("HOMELINK_DEVICE_NAME", "Generic HomeLink Service")
	capabilitiesStr := getEnv("HOMELINK_CAPABILITIES", "event_publisher,api")
	apiPort := getEnv("HOMELINK_API_PORT", "8081")
	apiKey = getEnv("HOMELINK_API_KEY", "")

	// Security configuration
	securityEnabled := getEnv("HOMELINK_SECURITY_ENABLED", "false") == "true"
	requireAuth := getEnv("HOMELINK_REQUIRE_AUTH", "true") == "true"
	networkKey := getEnv("HOMELINK_NETWORK_KEY", "")
	rateLimit, _ := strconv.Atoi(getEnv("HOMELINK_RATE_LIMIT", "10"))
	burstSize, _ := strconv.Atoi(getEnv("HOMELINK_BURST_SIZE", "20"))
	autoAcceptPairing := getEnv("HOMELINK_AUTO_ACCEPT_PAIRING", "false") == "true"

	// Storage configuration
	storageEnabled := getEnv("HOMELINK_STORAGE_ENABLED", "false") == "true"
	databasePath := getEnv("HOMELINK_DATABASE_PATH", "./homelink_events.db")
	retentionDays, _ := strconv.Atoi(getEnv("HOMELINK_RETENTION_DAYS", "30"))
	maxEvents, _ := strconv.Atoi(getEnv("HOMELINK_MAX_EVENTS", "0"))
	batchSize, _ := strconv.Atoi(getEnv("HOMELINK_BATCH_SIZE", "100"))
	flushIntervalStr := getEnv("HOMELINK_FLUSH_INTERVAL", "10s")
	flushInterval, _ := time.ParseDuration(flushIntervalStr)
	backupEnabled := getEnv("HOMELINK_BACKUP_ENABLED", "false") == "true"
	backupPath := getEnv("HOMELINK_BACKUP_PATH", "./backups")
	backupIntervalStr := getEnv("HOMELINK_BACKUP_INTERVAL", "24h")
	backupInterval, _ := time.ParseDuration(backupIntervalStr)

	// Frigate integration configuration
	frigateEnabled := getEnv("HOMELINK_FRIGATE_ENABLED", "false") == "true"
	frigateBaseURL := getEnv("HOMELINK_FRIGATE_BASE_URL", "http://localhost:5000")
	frigateSnapshotCache := getEnv("HOMELINK_FRIGATE_SNAPSHOT_CACHE", "true") == "true"
	frigateSnapshotCacheDir := getEnv("HOMELINK_FRIGATE_CACHE_DIR", "./frigate_snapshots")
	frigateSnapshotCacheTTLStr := getEnv("HOMELINK_FRIGATE_CACHE_TTL", "24h")
	frigateSnapshotCacheTTL, _ := time.ParseDuration(frigateSnapshotCacheTTLStr)
	frigateNotifyOnEvents := strings.Split(getEnv("HOMELINK_FRIGATE_NOTIFY_EVENTS", "new"), ",")
	frigateNotifyOnLabels := strings.Split(getEnv("HOMELINK_FRIGATE_NOTIFY_LABELS", ""), ",")
	frigateMinimumScore, _ := strconv.ParseFloat(getEnv("HOMELINK_FRIGATE_MIN_SCORE", "0.7"), 64)
	frigateIgnoreFalsePositives := getEnv("HOMELINK_FRIGATE_IGNORE_FALSE_POSITIVES", "true") == "true"
	frigateZoneFilter := strings.Split(getEnv("HOMELINK_FRIGATE_ZONE_FILTER", ""), ",")
	frigateWebhookSecret := getEnv("HOMELINK_FRIGATE_WEBHOOK_SECRET", "")

	// Clean up empty strings from comma-separated lists
	frigateNotifyOnEvents = filterEmptyStrings(frigateNotifyOnEvents)
	frigateNotifyOnLabels = filterEmptyStrings(frigateNotifyOnLabels)
	frigateZoneFilter = filterEmptyStrings(frigateZoneFilter)

	// Parse capabilities
	capabilities := strings.Split(capabilitiesStr, ",")
	for i, cap := range capabilities {
		capabilities[i] = strings.TrimSpace(cap)
	}

	log.Printf("Starting HomeLink Service:")
	log.Printf("  Device ID: %s", deviceID)
	log.Printf("  Device Name: %s", deviceName)
	log.Printf("  Capabilities: %v", capabilities)
	log.Printf("  API Port: %s", apiPort)
	log.Printf("  Security Enabled: %t", securityEnabled)

	var service *homelink.HomeLinkService
	var err error

	// Prepare security configuration
	var securityConfig *homelink.SecurityConfig
	if securityEnabled {
		securityConfig = &homelink.SecurityConfig{
			Enabled:               securityEnabled,
			RequireAuthentication: requireAuth,
			AllowedNetworkKey:     networkKey,
			MaxMessageAge:         5 * time.Minute,
			RateLimitPerSecond:    rateLimit,
			RateLimitBurstSize:    burstSize,
			AutoAcceptPairing:     autoAcceptPairing,
			RequireDeviceApproval: !autoAcceptPairing,
		}
	}

	// Storage configuration
	var storageConfig *homelink.StorageConfig
	if storageEnabled {
		if flushInterval == 0 {
			flushInterval = 10 * time.Second
		}
		if backupInterval == 0 {
			backupInterval = 24 * time.Hour
		}

		storageConfig = &homelink.StorageConfig{
			Enabled:        true,
			DatabasePath:   databasePath,
			RetentionDays:  retentionDays,
			MaxEvents:      maxEvents,
			BatchSize:      batchSize,
			FlushInterval:  flushInterval,
			BackupEnabled:  backupEnabled,
			BackupPath:     backupPath,
			BackupInterval: backupInterval,
		}
	}

	// Create service with advanced features
	service, err = homelink.NewAdvancedHomeLinkService(deviceID, deviceName, capabilities, securityConfig, storageConfig)

	if err != nil {
		log.Fatalf("Failed to create HomeLink service: %v", err)
	}

	// Start the HomeLink protocol service
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start HomeLink service: %v", err)
	}

	// Enable Frigate integration if configured
	if frigateEnabled {
		frigateConfig := &homelink.FrigateConfig{
			Enabled:              frigateEnabled,
			FrigateBaseURL:       frigateBaseURL,
			SnapshotCache:        frigateSnapshotCache,
			SnapshotCacheDir:     frigateSnapshotCacheDir,
			SnapshotCacheTTL:     frigateSnapshotCacheTTL,
			NotifyOnEvents:       frigateNotifyOnEvents,
			NotifyOnLabels:       frigateNotifyOnLabels,
			MinimumScore:         frigateMinimumScore,
			IgnoreFalsePositives: frigateIgnoreFalsePositives,
			ZoneFilter:           frigateZoneFilter,
			WebhookSecret:        frigateWebhookSecret,
		}

		if err := service.EnableFrigateIntegration(frigateConfig); err != nil {
			log.Fatalf("Failed to enable Frigate integration: %v", err)
		}
		log.Printf("Frigate integration enabled: %s", frigateBaseURL)
	}

	// Start HTTP API server for event publishing
	go startHTTPAPI(service, apiPort)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("HomeLink service running. Send events via HTTP API or press Ctrl+C to stop.")
	<-sigChan

	log.Println("Shutting down...")
	service.Stop()
}

// startHTTPAPI starts the HTTP API server for event publishing
func startHTTPAPI(service *homelink.HomeLinkService, port string) {
	// Initialize dashboard
	dashboard := homelink.NewDashboardServer(service, service.GetHealthMonitor())

	// Public endpoints (no auth required)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/health", healthHandler)

	// Dashboard endpoints
	dashboard.SetupRoutes(http.DefaultServeMux)

	// Protected endpoints (require API key if set)
	http.HandleFunc("/publish", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		publishEventHandler(w, r, service)
	}))
	http.HandleFunc("/devices", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		devicesHandler(w, r, service)
	}))
	http.HandleFunc("/subscribe", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		subscribeHandler(w, r, service)
	}))
	http.HandleFunc("/discovery", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		discoveryHandler(w, r, service)
	}))
	http.HandleFunc("/stats", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		statsHandler(w, r, service)
	}))

	// Security endpoints
	if service.IsSecurityEnabled() {
		http.HandleFunc("/security/qr-code", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			securityQRCodeHandler(w, r, service)
		}))
		http.HandleFunc("/security/pair", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			securityPairHandler(w, r, service)
		}))
		http.HandleFunc("/security/trust", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			securityTrustHandler(w, r, service)
		}))
		http.HandleFunc("/security/untrust", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			securityUntrustHandler(w, r, service)
		}))
		http.HandleFunc("/security/stats", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			securityStatsHandler(w, r, service)
		}))
	}

	// Health monitoring endpoints
	http.HandleFunc("/health/metrics", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		healthMetricsHandler(w, r, service)
	}))
	http.HandleFunc("/health/summary", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		healthSummaryHandler(w, r, service)
	}))

	// Reliability endpoints
	http.HandleFunc("/reliability/stats", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		reliabilityStatsHandler(w, r, service)
	}))
	http.HandleFunc("/publish/reliable", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		publishReliableEventHandler(w, r, service)
	}))

	// Protocol mode endpoint
	http.HandleFunc("/protocol/mode", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		protocolModeHandler(w, r, service)
	}))

	// Advanced feature endpoints - Event Filtering
	http.HandleFunc("/filtering/stats", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		filteringStatsHandler(w, r, service)
	}))
	http.HandleFunc("/filtering/filters", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			addFilterHandler(w, r, service)
		} else {
			getFiltersHandler(w, r, service)
		}
	}))
	http.HandleFunc("/filtering/remove", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		removeFilterHandler(w, r, service)
	}))

	// Advanced feature endpoints - Notifications
	http.HandleFunc("/notifications/stats", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		notificationStatsHandler(w, r, service)
	}))
	http.HandleFunc("/notifications/test", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		testNotificationHandler(w, r, service)
	}))

	// Advanced feature endpoints - Event Storage
	http.HandleFunc("/storage/stats", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		storageStatsHandler(w, r, service)
	}))
	http.HandleFunc("/storage/query", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		queryEventsHandler(w, r, service)
	}))
	http.HandleFunc("/storage/aggregates", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		eventAggregatesHandler(w, r, service)
	}))

	// Frigate integration endpoints
	if service.IsFrigateEnabled() {
		frigate := service.GetFrigateIntegration()
		// Webhook endpoint (no auth required - handled by webhook secret)
		http.HandleFunc("/frigate/webhook", frigate.HandleWebhook)
		// Snapshot serving endpoint
		http.HandleFunc("/frigate/snapshot/", frigate.ServeSnapshot)
		// Stats endpoint
		http.HandleFunc("/frigate/stats", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			frigateStatsHandler(w, r, service)
		}))
	}

	log.Printf("HTTP API server starting on port %s", port)
	log.Printf("  GET  /dashboard            - Web dashboard")
	log.Printf("  POST /publish              - Publish events")
	log.Printf("  POST /publish/reliable     - Publish reliable events")
	log.Printf("  GET  /devices              - List discovered devices")
	log.Printf("  POST /subscribe            - Subscribe to event types")
	log.Printf("  POST /discovery            - Trigger device discovery")
	log.Printf("  GET  /stats                - Discovery statistics")
	log.Printf("  GET  /health               - Health check")
	log.Printf("  GET  /health/metrics       - Detailed health metrics")
	log.Printf("  GET  /health/summary       - Health summary")
	log.Printf("  GET  /reliability/stats    - Reliability statistics")
	log.Printf("  GET/POST /protocol/mode    - Protocol mode management")
	log.Printf("  GET  /filtering/stats      - Event filtering statistics")
	log.Printf("  GET/POST /filtering/filters - Event filter management")
	log.Printf("  POST /filtering/remove     - Remove event filter")
	log.Printf("  GET  /notifications/stats  - Notification statistics")
	log.Printf("  POST /notifications/test   - Test notifications")
	log.Printf("  GET  /storage/stats        - Storage system statistics")
	log.Printf("  POST /storage/query        - Query stored events")
	log.Printf("  POST /storage/aggregates   - Get event aggregates")

	if service.IsSecurityEnabled() {
		log.Printf("  GET  /security/qr-code  - Generate QR pairing code")
		log.Printf("  POST /security/pair     - Pair with device via QR code")
		log.Printf("  POST /security/trust    - Add trusted device")
		log.Printf("  POST /security/untrust  - Remove trusted device")
		log.Printf("  GET  /security/stats    - Security statistics")
	}

	if service.IsFrigateEnabled() {
		log.Printf("  POST /frigate/webhook      - Frigate event webhook")
		log.Printf("  GET  /frigate/snapshot/*   - Serve cached snapshots")
		log.Printf("  GET  /frigate/stats        - Frigate integration statistics")
	}

	if apiKey != "" {
		log.Printf("API authentication enabled (use X-API-Key header)")
	}

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

// authMiddleware provides simple API key authentication
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if no API key is configured
		if apiKey == "" {
			next(w, r)
			return
		}

		// Check for API key in header
		providedKey := r.Header.Get("X-API-Key")
		if providedKey == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(APIResponse{
				Success: false,
				Message: "API key required (use X-API-Key header)",
			})
			return
		}

		// Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(apiKey)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(APIResponse{
				Success: false,
				Message: "Invalid API key",
			})
			return
		}

		next(w, r)
	}
}

// rootHandler provides API documentation
func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	html := `<!DOCTYPE html>
<html>
<head>
    <title>HomeLink Service API</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .endpoint { background: #f5f5f5; padding: 10px; margin: 10px 0; border-radius: 5px; }
        .method { color: #007acc; font-weight: bold; }
        pre { background: #eeeeee; padding: 10px; border-radius: 3px; }
    </style>
</head>
<body>
    <h1>🏠 HomeLink Service API</h1>
    <p>This service publishes events to the HomeLink network.</p>
    
    <h2>Endpoints</h2>
    
    <div class="endpoint">
        <p><span class="method">POST</span> <code>/publish</code> - Publish an event</p>
        <pre>{
  "event_type": "motion_detected",
  "description": "Motion detected at front door",
  "data": {
    "camera": "front_door",
    "confidence": "95"
  }
}</pre>
    </div>
    
    <div class="endpoint">
        <p><span class="method">GET</span> <code>/devices</code> - List discovered HomeLink devices</p>
    </div>
    
    <div class="endpoint">
        <p><span class="method">POST</span> <code>/subscribe</code> - Subscribe to event types</p>
        <pre>{
  "event_types": ["motion_detected", "person_detected"]
}</pre>
    </div>
    
    <div class="endpoint">
        <p><span class="method">GET</span> <code>/health</code> - Health check</p>
    </div>
    
    <div class="endpoint">
        <p><span class="method">POST</span> <code>/discovery</code> - Trigger device discovery</p>
        <p><small>Manually trigger device discovery to find new devices on the network</small></p>
    </div>
    
    <div class="endpoint">
        <p><span class="method">GET</span> <code>/stats</code> - Get discovery statistics</p>
        <p><small>View detailed information about discovered devices and network statistics</small></p>
    </div>
    
    <h2>Example Usage with curl</h2>
    <pre># Publish a motion detection event
curl -X POST http://localhost:8081/publish \
  -H "Content-Type: application/json" \
  -d '{
    "event_type": "motion_detected",
    "description": "Motion at front door",
    "data": {"camera": "front_door", "confidence": "95"}
  }'

# Subscribe to events
curl -X POST http://localhost:8081/subscribe \
  -H "Content-Type: application/json" \
  -d '{"event_types": ["motion_detected", "person_detected"]}'

# Get discovered devices
curl http://localhost:8081/devices

# Trigger device discovery
curl -X POST http://localhost:8081/discovery

# Get discovery statistics
curl http://localhost:8081/stats</pre>
</body>
</html>`
	w.Write([]byte(html))
}

// healthHandler provides a health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "HomeLink service is running",
	})
}

// publishEventHandler handles incoming event publishing requests
func publishEventHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only POST method allowed",
		})
		return
	}

	var eventReq EventRequest
	if err := json.NewDecoder(r.Body).Decode(&eventReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	// Validate required fields
	if eventReq.EventType == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "event_type is required",
		})
		return
	}

	// Add timestamp if not provided
	if eventReq.Data == nil {
		eventReq.Data = make(map[string]string)
	}
	if _, exists := eventReq.Data["timestamp"]; !exists {
		eventReq.Data["timestamp"] = fmt.Sprintf("%d", time.Now().Unix())
	}

	// Publish the event to the HomeLink network
	service.SendEvent(eventReq.EventType, eventReq.Description, eventReq.Data)

	log.Printf("Published event: %s - %s", eventReq.EventType, eventReq.Description)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Event published to HomeLink network",
	})
}

// devicesHandler returns the list of discovered HomeLink devices
func devicesHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	devices := service.GetDevices()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   len(devices),
		"devices": devices,
	})
}

// subscribeHandler handles subscription requests
func subscribeHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only POST method allowed",
		})
		return
	}

	var subReq struct {
		EventTypes []string `json:"event_types"`
	}

	if err := json.NewDecoder(r.Body).Decode(&subReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	if len(subReq.EventTypes) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "event_types array is required and cannot be empty",
		})
		return
	}

	// Subscribe to the specified event types
	service.Subscribe(subReq.EventTypes)

	log.Printf("Subscribed to event types: %v", subReq.EventTypes)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: fmt.Sprintf("Subscribed to %d event types", len(subReq.EventTypes)),
	})
}

// discoveryHandler triggers device discovery
func discoveryHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only POST method allowed",
		})
		return
	}

	// Trigger discovery
	service.TriggerDiscovery()

	log.Println("Manual device discovery triggered")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Device discovery triggered",
	})
}

// statsHandler returns discovery statistics
func statsHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	stats := service.GetDiscoveryStats()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"stats":   stats,
	})
}

// Security API handlers

// securityQRCodeHandler generates a QR pairing code
func securityQRCodeHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	if !service.IsSecurityEnabled() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Security not enabled",
		})
		return
	}

	qrCode, err := service.GenerateQRPairingCode()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Failed to generate QR code: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"qr_code": qrCode,
		"message": "QR pairing code generated. Share with other devices to allow pairing.",
	})
}

// securityPairHandler pairs with a device via QR code
func securityPairHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only POST method allowed",
		})
		return
	}

	if !service.IsSecurityEnabled() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Security not enabled",
		})
		return
	}

	var secReq SecurityRequest
	if err := json.NewDecoder(r.Body).Decode(&secReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	if secReq.QRCode == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "qr_code is required",
		})
		return
	}

	if err := service.ParseQRPairingCode(secReq.QRCode); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Failed to pair with device: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Successfully paired with device",
	})
}

// securityTrustHandler adds a trusted device
func securityTrustHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only POST method allowed",
		})
		return
	}

	if !service.IsSecurityEnabled() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Security not enabled",
		})
		return
	}

	var secReq SecurityRequest
	if err := json.NewDecoder(r.Body).Decode(&secReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	if secReq.DeviceID == "" || secReq.PublicKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "device_id and public_key are required",
		})
		return
	}

	if err := service.AddTrustedDevice(secReq.DeviceID, secReq.PublicKey); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Failed to add trusted device: " + err.Error(),
		})
		return
	}

	log.Printf("Added trusted device via API: %s", secReq.DeviceID)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Device added to trusted list",
	})
}

// securityUntrustHandler removes a trusted device
func securityUntrustHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only POST method allowed",
		})
		return
	}

	if !service.IsSecurityEnabled() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Security not enabled",
		})
		return
	}

	var secReq SecurityRequest
	if err := json.NewDecoder(r.Body).Decode(&secReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	if secReq.DeviceID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "device_id is required",
		})
		return
	}

	service.RemoveTrustedDevice(secReq.DeviceID)

	log.Printf("Removed trusted device via API: %s", secReq.DeviceID)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Device removed from trusted list",
	})
}

// securityStatsHandler returns security statistics
func securityStatsHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	if !service.IsSecurityEnabled() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Security not enabled",
		})
		return
	}

	stats := service.GetSecurityStats()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"stats":   stats,
	})
}

// New feature handlers

// healthMetricsHandler returns detailed health metrics
func healthMetricsHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	healthMonitor := service.GetHealthMonitor()
	if healthMonitor == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Health monitoring not available",
		})
		return
	}

	// Update metrics before returning
	service.UpdateHealthMetrics()
	metrics := healthMonitor.GetMetrics()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"metrics": metrics,
	})
}

// healthSummaryHandler returns health summary
func healthSummaryHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	summary := service.GetHealthSummary()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"summary": summary,
	})
}

// reliabilityStatsHandler returns reliability statistics
func reliabilityStatsHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	reliabilityManager := service.GetReliabilityManager()
	if reliabilityManager == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Reliability manager not available",
		})
		return
	}

	stats := reliabilityManager.GetPendingMessages()
	config := reliabilityManager.GetRetryConfiguration()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"pending_stats": stats,
		"configuration": config,
	})
}

// publishReliableEventHandler publishes events with reliability guarantees
func publishReliableEventHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only POST method allowed",
		})
		return
	}

	var eventReq struct {
		EventType   string            `json:"event_type"`
		Description string            `json:"description"`
		Data        map[string]string `json:"data"`
		Priority    string            `json:"priority"` // "low", "normal", "high", "emergency"
	}

	if err := json.NewDecoder(r.Body).Decode(&eventReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	if eventReq.EventType == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "event_type is required",
		})
		return
	}

	// Parse priority
	var priority homelink.MessagePriority
	switch strings.ToLower(eventReq.Priority) {
	case "low":
		priority = homelink.PriorityLow
	case "high":
		priority = homelink.PriorityHigh
	case "emergency":
		priority = homelink.PriorityEmergency
	default:
		priority = homelink.PriorityNormal
	}

	// Add timestamp if not provided
	if eventReq.Data == nil {
		eventReq.Data = make(map[string]string)
	}
	if _, exists := eventReq.Data["timestamp"]; !exists {
		eventReq.Data["timestamp"] = fmt.Sprintf("%d", time.Now().Unix())
	}

	// Send reliable event
	service.SendReliableEvent(eventReq.EventType, eventReq.Description, eventReq.Data, priority)

	log.Printf("Published reliable event: %s - %s (priority: %s)", eventReq.EventType, eventReq.Description, eventReq.Priority)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Reliable event published to HomeLink network",
	})
}

// protocolModeHandler manages protocol mode settings
func protocolModeHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		currentMode := service.GetProtocolMode()
		modeStr := "json"
		if currentMode == homelink.ProtocolBinary {
			modeStr = "binary"
		} else if currentMode == homelink.ProtocolAuto {
			modeStr = "auto"
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"mode":    modeStr,
		})

	case http.MethodPost:
		var modeReq struct {
			Mode string `json:"mode"` // "json", "binary", "auto"
		}

		if err := json.NewDecoder(r.Body).Decode(&modeReq); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIResponse{
				Success: false,
				Message: "Invalid JSON payload: " + err.Error(),
			})
			return
		}

		var newMode homelink.ProtocolMode
		switch strings.ToLower(modeReq.Mode) {
		case "binary":
			newMode = homelink.ProtocolBinary
		case "auto":
			newMode = homelink.ProtocolAuto
		case "json":
			newMode = homelink.ProtocolJSON
		default:
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIResponse{
				Success: false,
				Message: "Invalid protocol mode. Use 'json', 'binary', or 'auto'",
			})
			return
		}

		service.SetProtocolMode(newMode)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(APIResponse{
			Success: true,
			Message: fmt.Sprintf("Protocol mode set to %s", modeReq.Mode),
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET and POST methods allowed",
		})
	}
}

// Advanced Feature API Handlers

// filteringStatsHandler returns event filtering statistics
func filteringStatsHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	stats := service.GetFilteringStats()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"stats":   stats,
	})
}

// addFilterHandler adds a new event filter
func addFilterHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	var filter homelink.EventFilter
	if err := json.NewDecoder(r.Body).Decode(&filter); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	if err := service.AddEventFilter(filter); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Failed to add filter: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Event filter added successfully",
	})
}

// getFiltersHandler returns all active filters
func getFiltersHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	// This would require adding a GetFilters method to the service
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"filters": []interface{}{}, // Placeholder
		"message": "Filter listing not yet implemented",
	})
}

// removeFilterHandler removes an event filter
func removeFilterHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	var removeReq struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&removeReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	if removeReq.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Filter name is required",
		})
		return
	}

	success := service.RemoveEventFilter(removeReq.Name)
	if !success {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Filter not found",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Event filter removed successfully",
	})
}

// notificationStatsHandler returns notification statistics
func notificationStatsHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	stats := service.GetNotificationStats()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"stats":   stats,
	})
}

// testNotificationHandler sends a test notification
func testNotificationHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	var testReq struct {
		Message string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&testReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	if testReq.Message == "" {
		testReq.Message = "Test notification from HomeLink service"
	}

	// Create a test message
	testMsg := &homelink.Message{
		Type:      homelink.MSG_EVENT,
		DeviceID:  service.GetDeviceID(),
		Timestamp: time.Now().Unix(),
		Data:      map[string]interface{}{"message": testReq.Message},
	}

	// Send test notification
	if err := service.SendTestNotification(testMsg); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Failed to send test notification: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Test notification sent successfully",
	})
}

// Storage API Handlers

// storageStatsHandler returns storage system statistics
func storageStatsHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	if !service.IsStorageEnabled() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Event storage not enabled",
		})
		return
	}

	stats := service.GetStorageStats()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"stats":   stats,
	})
}

// queryEventsHandler queries stored events
func queryEventsHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only POST method allowed",
		})
		return
	}

	if !service.IsStorageEnabled() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Event storage not enabled",
		})
		return
	}

	var query homelink.EventQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	// Set default limit if not provided
	if query.Limit == 0 {
		query.Limit = 100
	}

	// Enforce maximum limit to prevent performance issues
	if query.Limit > 1000 {
		query.Limit = 1000
	}

	events, err := service.QueryStoredEvents(query)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Failed to query events: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"events":  events,
		"count":   len(events),
	})
}

// eventAggregatesHandler returns event aggregates
func eventAggregatesHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only POST method allowed",
		})
		return
	}

	if !service.IsStorageEnabled() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Event storage not enabled",
		})
		return
	}

	var aggregateReq struct {
		StartTime string `json:"start_time"` // RFC3339 format
		EndTime   string `json:"end_time"`   // RFC3339 format
	}

	if err := json.NewDecoder(r.Body).Decode(&aggregateReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid JSON payload: " + err.Error(),
		})
		return
	}

	// Parse time strings
	var startTime, endTime time.Time
	var err error

	if aggregateReq.StartTime != "" {
		startTime, err = time.Parse(time.RFC3339, aggregateReq.StartTime)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIResponse{
				Success: false,
				Message: "Invalid start_time format. Use RFC3339 format.",
			})
			return
		}
	} else {
		// Default to last 24 hours
		startTime = time.Now().AddDate(0, 0, -1)
	}

	if aggregateReq.EndTime != "" {
		endTime, err = time.Parse(time.RFC3339, aggregateReq.EndTime)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIResponse{
				Success: false,
				Message: "Invalid end_time format. Use RFC3339 format.",
			})
			return
		}
	} else {
		endTime = time.Now()
	}

	aggregates, err := service.GetEventAggregates(startTime, endTime)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Failed to get aggregates: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"aggregates": aggregates,
		"count":      len(aggregates),
		"time_range": map[string]interface{}{
			"start": startTime.Format(time.RFC3339),
			"end":   endTime.Format(time.RFC3339),
		},
	})
}

// frigateStatsHandler returns Frigate integration statistics
func frigateStatsHandler(w http.ResponseWriter, r *http.Request, service *homelink.HomeLinkService) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Only GET method allowed",
		})
		return
	}

	if !service.IsFrigateEnabled() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Frigate integration not enabled",
		})
		return
	}

	stats := service.GetFrigateStats()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"stats":   stats,
	})
}

// filterEmptyStrings removes empty strings from a slice
func filterEmptyStrings(slice []string) []string {
	var result []string
	for _, s := range slice {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
