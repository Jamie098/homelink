package main

import (
	"encoding/json"
	"fmt"
	"homelink"
	"log"
	"net/http"
	"os"
	"os/signal"
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

func main() {
	// Get configuration from environment variables
	deviceID := getEnv("HOMELINK_DEVICE_ID", "homelink-service")
	deviceName := getEnv("HOMELINK_DEVICE_NAME", "Generic HomeLink Service")
	capabilitiesStr := getEnv("HOMELINK_CAPABILITIES", "event_publisher,api")
	apiPort := getEnv("HOMELINK_API_PORT", "8081")

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

	// Create a HomeLink service instance
	service := homelink.NewHomeLinkService(deviceID, deviceName, capabilities)

	// Start the HomeLink protocol service
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start HomeLink service: %v", err)
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
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/publish", func(w http.ResponseWriter, r *http.Request) {
		publishEventHandler(w, r, service)
	})
	http.HandleFunc("/devices", func(w http.ResponseWriter, r *http.Request) {
		devicesHandler(w, r, service)
	})
	http.HandleFunc("/subscribe", func(w http.ResponseWriter, r *http.Request) {
		subscribeHandler(w, r, service)
	})
	http.HandleFunc("/discovery", func(w http.ResponseWriter, r *http.Request) {
		discoveryHandler(w, r, service)
	})
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		statsHandler(w, r, service)
	})

	log.Printf("HTTP API server starting on port %s", port)
	log.Printf("  POST /publish    - Publish events")
	log.Printf("  GET  /devices    - List discovered devices")
	log.Printf("  POST /subscribe  - Subscribe to event types")
	log.Printf("  POST /discovery  - Trigger device discovery")
	log.Printf("  GET  /stats      - Discovery statistics")
	log.Printf("  GET  /health     - Health check")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
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

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
