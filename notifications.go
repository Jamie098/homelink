// HomeLink - Webhook and Push Notification System
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// NotificationService manages all outbound notifications
type NotificationService struct {
	webhooks    []WebhookEndpoint
	pushServices []PushService
	client      *http.Client
	mutex       sync.RWMutex
	stats       NotificationStats
	rateLimiter map[string]*NotificationRateLimit
}

// WebhookEndpoint represents a webhook destination
type WebhookEndpoint struct {
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`      // GET, POST, PUT
	Headers     map[string]string `json:"headers"`
	Template    string            `json:"template"`    // JSON template for payload
	Enabled     bool              `json:"enabled"`
	Timeout     time.Duration     `json:"timeout"`
	Retries     int               `json:"retries"`
	RetryDelay  time.Duration     `json:"retry_delay"`
	EventFilter []string          `json:"event_filter"` // Only send these event types
}

// PushService represents a push notification service
type PushService struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`        // ntfy, discord, slack, telegram
	Config      map[string]string `json:"config"`      // Service-specific config
	Template    string            `json:"template"`    // Message template
	Enabled     bool              `json:"enabled"`
	Priority    string            `json:"priority"`    // low, normal, high, emergency
	EventFilter []string          `json:"event_filter"`
}

// NotificationStats tracks notification system performance
type NotificationStats struct {
	WebhooksSent      uint64            `json:"webhooks_sent"`
	WebhooksFailed    uint64            `json:"webhooks_failed"`
	PushNotificationsSent uint64        `json:"push_notifications_sent"`
	PushNotificationsFailed uint64      `json:"push_notifications_failed"`
	ByEndpoint        map[string]uint64 `json:"by_endpoint"`
	LastReset         time.Time         `json:"last_reset"`
}

// NotificationRateLimit tracks rate limiting per endpoint
type NotificationRateLimit struct {
	Count      int
	WindowStart time.Time
	MaxPerHour int
}

// NotificationPayload represents data sent to notifications
type NotificationPayload struct {
	Event       *Message               `json:"event"`
	Timestamp   time.Time              `json:"timestamp"`
	DeviceName  string                 `json:"device_name"`
	Priority    string                 `json:"priority"`
	Summary     string                 `json:"summary"`
	Details     map[string]interface{} `json:"details"`
	Attachments []NotificationAttachment `json:"attachments,omitempty"`
}

// NotificationAttachment represents files attached to notifications
type NotificationAttachment struct {
	Type     string `json:"type"`      // image, video, document
	URL      string `json:"url"`
	Data     []byte `json:"data"`
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
}

// WebhookResponse represents response from webhook
type WebhookResponse struct {
	StatusCode int               `json:"status_code"`
	Body       string            `json:"body"`
	Headers    map[string]string `json:"headers"`
	Duration   time.Duration     `json:"duration"`
	Error      error             `json:"error,omitempty"`
}

// NewNotificationService creates a new notification service
func NewNotificationService() *NotificationService {
	return &NotificationService{
		webhooks:     make([]WebhookEndpoint, 0),
		pushServices: make([]PushService, 0),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		stats: NotificationStats{
			ByEndpoint: make(map[string]uint64),
			LastReset:  time.Now(),
		},
		rateLimiter: make(map[string]*NotificationRateLimit),
	}
}

// AddWebhook adds a webhook endpoint
func (ns *NotificationService) AddWebhook(webhook WebhookEndpoint) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	
	// Set defaults
	if webhook.Method == "" {
		webhook.Method = "POST"
	}
	if webhook.Timeout == 0 {
		webhook.Timeout = 10 * time.Second
	}
	if webhook.Retries == 0 {
		webhook.Retries = 3
	}
	if webhook.RetryDelay == 0 {
		webhook.RetryDelay = 1 * time.Second
	}
	
	ns.webhooks = append(ns.webhooks, webhook)
	ns.stats.ByEndpoint[webhook.Name] = 0
	
	log.Printf("Added webhook endpoint: %s (%s)", webhook.Name, webhook.URL)
}

// AddPushService adds a push notification service
func (ns *NotificationService) AddPushService(service PushService) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	
	ns.pushServices = append(ns.pushServices, service)
	ns.stats.ByEndpoint[service.Name] = 0
	
	log.Printf("Added push service: %s (%s)", service.Name, service.Type)
}

// SendNotification sends a notification for an event
func (ns *NotificationService) SendNotification(event *Message, priority string, deviceName string) {
	payload := &NotificationPayload{
		Event:      event,
		Timestamp:  time.Now(),
		DeviceName: deviceName,
		Priority:   priority,
		Summary:    ns.generateSummary(event),
		Details:    ns.extractDetails(event),
	}
	
	// Add attachments if available (e.g., Frigate snapshots)
	if attachments := ns.extractAttachments(event); len(attachments) > 0 {
		payload.Attachments = attachments
	}
	
	// Send to webhooks
	ns.sendToWebhooks(payload)
	
	// Send to push services
	ns.sendToPushServices(payload)
}

// sendToWebhooks sends notification to all configured webhooks
func (ns *NotificationService) sendToWebhooks(payload *NotificationPayload) {
	ns.mutex.RLock()
	webhooks := make([]WebhookEndpoint, len(ns.webhooks))
	copy(webhooks, ns.webhooks)
	ns.mutex.RUnlock()
	
	for _, webhook := range webhooks {
		if !webhook.Enabled {
			continue
		}
		
		// Check event filter
		if !ns.eventMatchesFilter(payload.Event, webhook.EventFilter) {
			continue
		}
		
		// Check rate limit
		if !ns.checkRateLimit(webhook.Name, 100) { // 100 per hour default
			log.Printf("Rate limit exceeded for webhook: %s", webhook.Name)
			continue
		}
		
		// Send asynchronously
		go ns.sendWebhook(webhook, payload)
	}
}

// sendWebhook sends to a single webhook endpoint
func (ns *NotificationService) sendWebhook(webhook WebhookEndpoint, payload *NotificationPayload) {
	startTime := time.Now()
	
	// Generate request body
	var requestBody []byte
	var err error
	
	if webhook.Template != "" {
		requestBody, err = ns.applyTemplate(webhook.Template, payload)
	} else {
		requestBody, err = json.Marshal(payload)
	}
	
	if err != nil {
		ns.recordWebhookFailure(webhook.Name, err)
		return
	}
	
	// Create request
	req, err := http.NewRequest(webhook.Method, webhook.URL, bytes.NewBuffer(requestBody))
	if err != nil {
		ns.recordWebhookFailure(webhook.Name, err)
		return
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "HomeLink-Notification-Service/1.0")
	
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}
	
	// Send with retries
	var resp *http.Response
	for attempt := 0; attempt <= webhook.Retries; attempt++ {
		if attempt > 0 {
			time.Sleep(webhook.RetryDelay * time.Duration(attempt))
		}
		
		client := &http.Client{Timeout: webhook.Timeout}
		resp, err = client.Do(req)
		
		if err == nil && resp.StatusCode < 500 {
			break // Success or client error (don't retry)
		}
		
		if resp != nil {
			resp.Body.Close()
		}
	}
	
	duration := time.Since(startTime)
	
	if err != nil {
		ns.recordWebhookFailure(webhook.Name, err)
		log.Printf("Webhook failed after %d retries: %s - %v", webhook.Retries, webhook.Name, err)
		return
	}
	
	// Read response
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		ns.recordWebhookSuccess(webhook.Name)
		log.Printf("Webhook sent successfully: %s (%d, %s)", webhook.Name, resp.StatusCode, duration)
	} else {
		ns.recordWebhookFailure(webhook.Name, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body)))
		log.Printf("Webhook failed: %s - HTTP %d", webhook.Name, resp.StatusCode)
	}
}

// sendToPushServices sends notification to push services
func (ns *NotificationService) sendToPushServices(payload *NotificationPayload) {
	ns.mutex.RLock()
	services := make([]PushService, len(ns.pushServices))
	copy(services, ns.pushServices)
	ns.mutex.RUnlock()
	
	for _, service := range services {
		if !service.Enabled {
			continue
		}
		
		// Check event filter
		if !ns.eventMatchesFilter(payload.Event, service.EventFilter) {
			continue
		}
		
		// Send asynchronously
		go ns.sendPushNotification(service, payload)
	}
}

// sendPushNotification sends to a specific push service
func (ns *NotificationService) sendPushNotification(service PushService, payload *NotificationPayload) {
	switch service.Type {
	case "ntfy":
		ns.sendNtfyNotification(service, payload)
	case "discord":
		ns.sendDiscordNotification(service, payload)
	case "slack":
		ns.sendSlackNotification(service, payload)
	case "telegram":
		ns.sendTelegramNotification(service, payload)
	default:
		log.Printf("Unknown push service type: %s", service.Type)
	}
}

// sendNtfyNotification sends to ntfy.sh
func (ns *NotificationService) sendNtfyNotification(service PushService, payload *NotificationPayload) {
	url := service.Config["url"]
	if url == "" {
		log.Printf("Missing URL for ntfy service: %s", service.Name)
		return
	}
	
	message := ns.generateMessage(service.Template, payload)
	if message == "" {
		message = payload.Summary
	}
	
	// Create ntfy request
	req, err := http.NewRequest("POST", url, strings.NewReader(message))
	if err != nil {
		ns.recordPushFailure(service.Name, err)
		return
	}
	
	req.Header.Set("Title", fmt.Sprintf("HomeLink - %s", payload.DeviceName))
	req.Header.Set("Tags", ns.getEventTags(payload.Event))
	
	// Set priority
	switch payload.Priority {
	case "emergency":
		req.Header.Set("Priority", "urgent")
	case "high":
		req.Header.Set("Priority", "high")
	case "low":
		req.Header.Set("Priority", "low")
	default:
		req.Header.Set("Priority", "default")
	}
	
	resp, err := ns.client.Do(req)
	if err != nil {
		ns.recordPushFailure(service.Name, err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		ns.recordPushSuccess(service.Name)
		log.Printf("Ntfy notification sent: %s", service.Name)
	} else {
		ns.recordPushFailure(service.Name, fmt.Errorf("HTTP %d", resp.StatusCode))
	}
}

// sendDiscordNotification sends to Discord webhook
func (ns *NotificationService) sendDiscordNotification(service PushService, payload *NotificationPayload) {
	webhookURL := service.Config["webhook_url"]
	if webhookURL == "" {
		log.Printf("Missing webhook_url for Discord service: %s", service.Name)
		return
	}
	
	message := ns.generateMessage(service.Template, payload)
	if message == "" {
		message = payload.Summary
	}
	
	discordPayload := map[string]interface{}{
		"content": message,
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("HomeLink Alert - %s", payload.DeviceName),
				"description": payload.Summary,
				"color":       ns.getPriorityColor(payload.Priority),
				"timestamp":   payload.Timestamp.Format(time.RFC3339),
				"fields": []map[string]interface{}{
					{"name": "Event Type", "value": string(payload.Event.Type), "inline": true},
					{"name": "Device", "value": payload.Event.DeviceID, "inline": true},
					{"name": "Priority", "value": payload.Priority, "inline": true},
				},
			},
		},
	}
	
	// Add image if available
	if len(payload.Attachments) > 0 {
		for _, attachment := range payload.Attachments {
			if attachment.Type == "image" && attachment.URL != "" {
				if embeds, ok := discordPayload["embeds"].([]map[string]interface{}); ok && len(embeds) > 0 {
					embeds[0]["image"] = map[string]interface{}{
						"url": attachment.URL,
					}
				}
				break
			}
		}
	}
	
	jsonData, _ := json.Marshal(discordPayload)
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		ns.recordPushFailure(service.Name, err)
		return
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := ns.client.Do(req)
	if err != nil {
		ns.recordPushFailure(service.Name, err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		ns.recordPushSuccess(service.Name)
		log.Printf("Discord notification sent: %s", service.Name)
	} else {
		ns.recordPushFailure(service.Name, fmt.Errorf("HTTP %d", resp.StatusCode))
	}
}

// sendSlackNotification sends to Slack webhook
func (ns *NotificationService) sendSlackNotification(service PushService, payload *NotificationPayload) {
	webhookURL := service.Config["webhook_url"]
	if webhookURL == "" {
		log.Printf("Missing webhook_url for Slack service: %s", service.Name)
		return
	}
	
	message := ns.generateMessage(service.Template, payload)
	if message == "" {
		message = payload.Summary
	}
	
	slackPayload := map[string]interface{}{
		"text": fmt.Sprintf("HomeLink Alert - %s", payload.DeviceName),
		"attachments": []map[string]interface{}{
			{
				"color":      ns.getPriorityColorHex(payload.Priority),
				"title":      payload.Summary,
				"text":       message,
				"timestamp":  payload.Timestamp.Unix(),
				"fields": []map[string]interface{}{
					{"title": "Event Type", "value": string(payload.Event.Type), "short": true},
					{"title": "Device", "value": payload.Event.DeviceID, "short": true},
					{"title": "Priority", "value": payload.Priority, "short": true},
				},
			},
		},
	}
	
	jsonData, _ := json.Marshal(slackPayload)
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		ns.recordPushFailure(service.Name, err)
		return
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := ns.client.Do(req)
	if err != nil {
		ns.recordPushFailure(service.Name, err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		ns.recordPushSuccess(service.Name)
		log.Printf("Slack notification sent: %s", service.Name)
	} else {
		ns.recordPushFailure(service.Name, fmt.Errorf("HTTP %d", resp.StatusCode))
	}
}

// sendTelegramNotification sends to Telegram bot
func (ns *NotificationService) sendTelegramNotification(service PushService, payload *NotificationPayload) {
	botToken := service.Config["bot_token"]
	chatID := service.Config["chat_id"]
	
	if botToken == "" || chatID == "" {
		log.Printf("Missing bot_token or chat_id for Telegram service: %s", service.Name)
		return
	}
	
	message := ns.generateMessage(service.Template, payload)
	if message == "" {
		message = fmt.Sprintf("🏠 *HomeLink Alert*\n\n*Device:* %s\n*Event:* %s\n*Priority:* %s\n\n%s", 
			payload.DeviceName, string(payload.Event.Type), payload.Priority, payload.Summary)
	}
	
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	
	telegramPayload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "Markdown",
	}
	
	jsonData, _ := json.Marshal(telegramPayload)
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		ns.recordPushFailure(service.Name, err)
		return
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := ns.client.Do(req)
	if err != nil {
		ns.recordPushFailure(service.Name, err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		ns.recordPushSuccess(service.Name)
		log.Printf("Telegram notification sent: %s", service.Name)
	} else {
		ns.recordPushFailure(service.Name, fmt.Errorf("HTTP %d", resp.StatusCode))
	}
}

// Helper methods

// eventMatchesFilter checks if event matches filter criteria
func (ns *NotificationService) eventMatchesFilter(event *Message, filter []string) bool {
	if len(filter) == 0 {
		return true // No filter means all events
	}
	
	for _, eventType := range filter {
		if string(event.Type) == eventType {
			return true
		}
	}
	return false
}

// checkRateLimit checks if we're within rate limit
func (ns *NotificationService) checkRateLimit(endpoint string, maxPerHour int) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	
	now := time.Now()
	limit, exists := ns.rateLimiter[endpoint]
	
	if !exists {
		ns.rateLimiter[endpoint] = &NotificationRateLimit{
			Count:       1,
			WindowStart: now,
			MaxPerHour:  maxPerHour,
		}
		return true
	}
	
	// Reset window if it's been an hour
	if now.Sub(limit.WindowStart) >= time.Hour {
		limit.Count = 1
		limit.WindowStart = now
		return true
	}
	
	// Check if within limit
	if limit.Count < limit.MaxPerHour {
		limit.Count++
		return true
	}
	
	return false
}

// generateSummary creates a human-readable summary
func (ns *NotificationService) generateSummary(event *Message) string {
	if dataMap, ok := event.Data.(map[string]interface{}); ok {
		if desc, exists := dataMap["description"].(string); exists && desc != "" {
			return desc
		}
		
		// Try to build summary from available data
		if eventType, exists := dataMap["event_type"].(string); exists {
			summary := fmt.Sprintf("%s event", strings.Title(eventType))
			
			if camera, exists := dataMap["camera"].(string); exists {
				summary += fmt.Sprintf(" on %s", camera)
			}
			
			if confidence, exists := dataMap["confidence"].(string); exists {
				summary += fmt.Sprintf(" (confidence: %s)", confidence)
			}
			
			return summary
		}
	}
	
	return fmt.Sprintf("%s event from %s", string(event.Type), event.DeviceID)
}

// extractDetails extracts detailed information from event
func (ns *NotificationService) extractDetails(event *Message) map[string]interface{} {
	details := make(map[string]interface{})
	
	if dataMap, ok := event.Data.(map[string]interface{}); ok {
		for k, v := range dataMap {
			details[k] = v
		}
	}
	
	details["timestamp"] = time.Unix(event.Timestamp, 0).Format(time.RFC3339)
	details["device_id"] = event.DeviceID
	details["event_type"] = string(event.Type)
	
	return details
}

// extractAttachments extracts attachments from event data
func (ns *NotificationService) extractAttachments(event *Message) []NotificationAttachment {
	var attachments []NotificationAttachment
	
	if dataMap, ok := event.Data.(map[string]interface{}); ok {
		// Check for Frigate snapshot
		if snapshotURL, exists := dataMap["snapshot_url"].(string); exists && snapshotURL != "" {
			attachments = append(attachments, NotificationAttachment{
				Type:     "image",
				URL:      snapshotURL,
				MimeType: "image/jpeg",
			})
		}
		
		// Check for embedded image data
		if snapshotData, exists := dataMap["snapshot_data"].(string); exists && snapshotData != "" {
			// Would decode base64 data here
			attachments = append(attachments, NotificationAttachment{
				Type:     "image",
				Data:     []byte(snapshotData), // This would be base64 decoded
				MimeType: "image/jpeg",
			})
		}
	}
	
	return attachments
}

// applyTemplate applies a template to generate custom payload
func (ns *NotificationService) applyTemplate(template string, payload *NotificationPayload) ([]byte, error) {
	// Simple template substitution - in a real implementation you'd use text/template
	result := template
	
	replacements := map[string]string{
		"{{.DeviceName}}": payload.DeviceName,
		"{{.Summary}}":    payload.Summary,
		"{{.Priority}}":   payload.Priority,
		"{{.EventType}}":  string(payload.Event.Type),
		"{{.DeviceID}}":   payload.Event.DeviceID,
		"{{.Timestamp}}":  payload.Timestamp.Format(time.RFC3339),
	}
	
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	
	return []byte(result), nil
}

// generateMessage generates message text from template
func (ns *NotificationService) generateMessage(template string, payload *NotificationPayload) string {
	if template == "" {
		return payload.Summary
	}
	
	result := template
	replacements := map[string]string{
		"{{.DeviceName}}": payload.DeviceName,
		"{{.Summary}}":    payload.Summary,
		"{{.Priority}}":   payload.Priority,
		"{{.EventType}}":  string(payload.Event.Type),
		"{{.DeviceID}}":   payload.Event.DeviceID,
		"{{.Timestamp}}":  payload.Timestamp.Format("2006-01-02 15:04:05"),
	}
	
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	
	return result
}

// getEventTags returns emoji tags for event types
func (ns *NotificationService) getEventTags(event *Message) string {
	eventType := string(event.Type)
	
	tags := []string{"homelink"}
	
	if strings.Contains(eventType, "person") {
		tags = append(tags, "person")
	}
	if strings.Contains(eventType, "vehicle") {
		tags = append(tags, "car")
	}
	if strings.Contains(eventType, "motion") {
		tags = append(tags, "eyes")
	}
	if strings.Contains(eventType, "door") {
		tags = append(tags, "door")
	}
	
	return strings.Join(tags, ",")
}

// getPriorityColor returns Discord embed color for priority
func (ns *NotificationService) getPriorityColor(priority string) int {
	switch priority {
	case "emergency":
		return 15158332 // Red
	case "high":
		return 15844367 // Orange
	case "low":
		return 5763719  // Gray
	default:
		return 3447003  // Blue
	}
}

// getPriorityColorHex returns hex color for Slack attachments
func (ns *NotificationService) getPriorityColorHex(priority string) string {
	switch priority {
	case "emergency":
		return "danger"
	case "high":
		return "warning"
	case "low":
		return "#cccccc"
	default:
		return "good"
	}
}

// Record statistics methods
func (ns *NotificationService) recordWebhookSuccess(name string) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	ns.stats.WebhooksSent++
	ns.stats.ByEndpoint[name]++
}

func (ns *NotificationService) recordWebhookFailure(name string, err error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	ns.stats.WebhooksFailed++
	log.Printf("Webhook %s failed: %v", name, err)
}

func (ns *NotificationService) recordPushSuccess(name string) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	ns.stats.PushNotificationsSent++
	ns.stats.ByEndpoint[name]++
}

func (ns *NotificationService) recordPushFailure(name string, err error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	ns.stats.PushNotificationsFailed++
	log.Printf("Push notification %s failed: %v", name, err)
}

// GetStats returns notification statistics
func (ns *NotificationService) GetStats() NotificationStats {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	
	// Create a copy
	stats := ns.stats
	stats.ByEndpoint = make(map[string]uint64)
	for k, v := range ns.stats.ByEndpoint {
		stats.ByEndpoint[k] = v
	}
	
	return stats
}