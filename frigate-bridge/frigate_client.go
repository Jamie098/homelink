package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// FrigateEvent represents an event from Frigate's API
type FrigateEvent struct {
	ID             string                 `json:"id"`
	Label          string                 `json:"label"`
	Camera         string                 `json:"camera"`
	StartTime      float64                `json:"start_time"`
	EndTime        *float64               `json:"end_time,omitempty"`
	Score          float64                `json:"score"`
	Area           int                    `json:"area"`
	Box            []float64              `json:"box,omitempty"`
	Region         []float64              `json:"region,omitempty"`
	HasSnapshot    bool                   `json:"has_snapshot"`
	HasClip        bool                   `json:"has_clip"`
	Thumbnail      string                 `json:"thumbnail,omitempty"`
	Data           map[string]interface{} `json:"data,omitempty"`
	Zones          []string               `json:"zones,omitempty"`
	SubLabel       string                 `json:"sub_label,omitempty"`
	Top_score      float64                `json:"top_score"`
	False_positive bool                   `json:"false_positive"`
}

// FrigateEventsResponse represents the response from Frigate's events API
type FrigateEventsResponse []FrigateEvent

// FrigateClient handles communication with Frigate's API
type FrigateClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewFrigateClient creates a new Frigate API client
func NewFrigateClient(baseURL string, timeout time.Duration) *FrigateClient {
	return &FrigateClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetRecentEvents retrieves recent events from Frigate
func (fc *FrigateClient) GetRecentEvents(after time.Time, cameras []string, labels []string) ([]FrigateEvent, error) {
	// Build query parameters
	url := fmt.Sprintf("%s/api/events", fc.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameters
	q := req.URL.Query()
	q.Set("after", fmt.Sprintf("%.0f", float64(after.Unix())))
	q.Set("include_thumbnails", "1")

	// Filter by cameras if specified
	if len(cameras) > 0 {
		for _, camera := range cameras {
			q.Add("camera", camera)
		}
	}

	// Filter by labels if specified
	if len(labels) > 0 {
		for _, label := range labels {
			q.Add("label", label)
		}
	}

	req.URL.RawQuery = q.Encode()

	// Debug logging - print the full URL
	log.Printf("DEBUG: Making API call to: %s", req.URL.String())

	// Execute request
	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("frigate API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var events FrigateEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("Retrieved %d events from Frigate", len(events))
	return events, nil
}

// GetEvent retrieves a specific event by ID
func (fc *FrigateClient) GetEvent(eventID string) (*FrigateEvent, error) {
	url := fmt.Sprintf("%s/api/events/%s", fc.baseURL, eventID)

	resp, err := fc.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("event not found: %s", eventID)
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("frigate API returned status %d: %s", resp.StatusCode, string(body))
	}

	var event FrigateEvent
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return nil, fmt.Errorf("failed to decode event: %w", err)
	}

	return &event, nil
}

// GetStats retrieves stats from Frigate
func (fc *FrigateClient) GetStats() (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/stats", fc.baseURL)

	resp, err := fc.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("frigate API returned status %d: %s", resp.StatusCode, string(body))
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	return stats, nil
}

// HealthCheck performs a health check on the Frigate API
func (fc *FrigateClient) HealthCheck() error {
	url := fmt.Sprintf("%s/api/version", fc.baseURL)

	resp, err := fc.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("frigate health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("frigate health check failed with status: %d", resp.StatusCode)
	}

	return nil
}

// GetThumbnail retrieves a thumbnail for an event
// Tries multiple endpoints and formats for maximum compatibility
func (fc *FrigateClient) GetThumbnail(eventID string) ([]byte, error) {
	// List of possible endpoints to try in order of preference
	endpoints := []string{
		fmt.Sprintf("%s/api/events/%s/snapshot.jpg", fc.baseURL, eventID),      // Primary snapshot endpoint
		fmt.Sprintf("%s/api/events/%s/thumbnail.jpg", fc.baseURL, eventID),     // Legacy thumbnail endpoint
		fmt.Sprintf("%s/api/events/%s/snapshot.webp", fc.baseURL, eventID),     // WebP format support
		fmt.Sprintf("%s/api/events/%s/thumbnail.webp", fc.baseURL, eventID),    // WebP thumbnail format
	}

	var lastErr error
	for i, url := range endpoints {
		log.Printf("Trying snapshot endpoint %d/%d: %s", i+1, len(endpoints), url)
		
		resp, err := fc.httpClient.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("failed to get thumbnail from %s: %w", url, err)
			continue
		}
		
		if resp.StatusCode == http.StatusOK {
			thumbnail, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				lastErr = fmt.Errorf("failed to read thumbnail from %s: %w", url, err)
				continue
			}
			
			if len(thumbnail) > 0 {
				log.Printf("Successfully retrieved thumbnail from %s (%d bytes)", url, len(thumbnail))
				return thumbnail, nil
			} else {
				log.Printf("Empty thumbnail from %s", url)
				lastErr = fmt.Errorf("empty thumbnail from %s", url)
				continue
			}
		}
		
		// Log the specific error for debugging
		if resp.StatusCode == http.StatusNotFound {
			log.Printf("Thumbnail not found (404) at %s", url)
		} else {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("Thumbnail request failed with status %d at %s: %s", resp.StatusCode, url, string(body))
		}
		resp.Body.Close()
		
		lastErr = fmt.Errorf("thumbnail request failed with status %d from %s", resp.StatusCode, url)
	}

	return nil, fmt.Errorf("failed to retrieve thumbnail from any endpoint: %w", lastErr)
}

// GetClip retrieves a video clip for an event
func (fc *FrigateClient) GetClip(eventID string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/events/%s/clip.mp4", fc.baseURL, eventID)

	resp, err := fc.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get clip: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("clip not found (404)")
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clip request failed with status %d: %s", resp.StatusCode, string(body))
	}

	clip, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read clip: %w", err)
	}

	return clip, nil
}
