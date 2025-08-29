package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// HomeLinkClient handles communication with HomeLink's HTTP API
type HomeLinkClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// HomeLinkResponse represents a standard response from HomeLink API
type HomeLinkResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// HomeLinkDevicesResponse represents the response from the devices endpoint
type HomeLinkDevicesResponse struct {
	Success bool                     `json:"success"`
	Count   int                      `json:"count"`
	Devices []map[string]interface{} `json:"devices"`
}

// HomeLinkStatsResponse represents the response from the stats endpoint
type HomeLinkStatsResponse struct {
	Success bool                   `json:"success"`
	Stats   map[string]interface{} `json:"stats"`
}

// NewHomeLinkClient creates a new HomeLink API client
func NewHomeLinkClient(baseURL, apiKey string, timeout time.Duration) *HomeLinkClient {
	return &HomeLinkClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// PublishEvent publishes an event to HomeLink
func (hlc *HomeLinkClient) PublishEvent(event *HomeLinkEvent) error {
	return hlc.publishEventWithRetry(event, false, "normal")
}

// PublishReliableEvent publishes an event with reliability guarantees
func (hlc *HomeLinkClient) PublishReliableEvent(event *HomeLinkEvent, priority string) error {
	return hlc.publishEventWithRetry(event, true, priority)
}

// publishEventWithRetry handles the actual event publishing with optional reliability
func (hlc *HomeLinkClient) publishEventWithRetry(event *HomeLinkEvent, reliable bool, priority string) error {
	endpoint := "/publish"
	if reliable {
		endpoint = "/publish/reliable"
		// Add priority to the event data for reliable publishing
		if event.Data == nil {
			event.Data = make(map[string]string)
		}
		event.Data["priority"] = priority
	}

	url := fmt.Sprintf("%s%s", hlc.baseURL, endpoint)

	// Marshal event to JSON
	jsonData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if hlc.apiKey != "" {
		req.Header.Set("X-API-Key", hlc.apiKey)
	}

	// Execute request
	resp, err := hlc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("homelink API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response HomeLinkResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("homelink API reported failure: %s", response.Message)
	}

	log.Printf("Successfully published event to HomeLink: %s", event.EventType)
	return nil
}

// Subscribe subscribes to specific event types
func (hlc *HomeLinkClient) Subscribe(eventTypes []string) error {
	url := fmt.Sprintf("%s/subscribe", hlc.baseURL)

	// Create subscription payload
	payload := map[string][]string{
		"event_types": eventTypes,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal subscription: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if hlc.apiKey != "" {
		req.Header.Set("X-API-Key", hlc.apiKey)
	}

	// Execute request
	resp, err := hlc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("homelink API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response HomeLinkResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("homelink API reported failure: %s", response.Message)
	}

	log.Printf("Successfully subscribed to HomeLink events: %v", eventTypes)
	return nil
}

// GetDevices retrieves the list of discovered HomeLink devices
func (hlc *HomeLinkClient) GetDevices() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/devices", hlc.baseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set API key header if available
	if hlc.apiKey != "" {
		req.Header.Set("X-API-Key", hlc.apiKey)
	}

	// Execute request
	resp, err := hlc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("homelink API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response HomeLinkDevicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("homelink API reported failure")
	}

	return response.Devices, nil
}

// GetStats retrieves HomeLink service statistics
func (hlc *HomeLinkClient) GetStats() (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/stats", hlc.baseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set API key header if available
	if hlc.apiKey != "" {
		req.Header.Set("X-API-Key", hlc.apiKey)
	}

	// Execute request
	resp, err := hlc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("homelink API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response HomeLinkStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("homelink API reported failure")
	}

	return response.Stats, nil
}

// TriggerDiscovery triggers manual device discovery
func (hlc *HomeLinkClient) TriggerDiscovery() error {
	url := fmt.Sprintf("%s/discovery", hlc.baseURL)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set API key header if available
	if hlc.apiKey != "" {
		req.Header.Set("X-API-Key", hlc.apiKey)
	}

	// Execute request
	resp, err := hlc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("homelink API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response HomeLinkResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("homelink API reported failure: %s", response.Message)
	}

	log.Printf("Successfully triggered HomeLink device discovery")
	return nil
}

// HealthCheck performs a health check on the HomeLink API
func (hlc *HomeLinkClient) HealthCheck() error {
	url := fmt.Sprintf("%s/health", hlc.baseURL)

	resp, err := hlc.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("homelink health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("homelink health check failed with status: %d", resp.StatusCode)
	}

	return nil
}
