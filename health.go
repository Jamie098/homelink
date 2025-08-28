// HomeLink - Health Monitoring System
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"encoding/json"
	"runtime"
	"sync"
	"time"
)

// HealthMetrics represents health information for a device
type HealthMetrics struct {
	DeviceID          string                 `json:"device_id"`
	Timestamp         int64                  `json:"timestamp"`
	Uptime            time.Duration          `json:"uptime"`
	CPUUsage          float64                `json:"cpu_usage"`
	MemoryUsage       float64                `json:"memory_usage"`
	NetworkStats      NetworkHealthStats     `json:"network_stats"`
	MessageStats      MessageHealthStats     `json:"message_stats"`
	SecurityStats     SecurityHealthStats    `json:"security_stats"`
	BatteryLevel      *float64               `json:"battery_level,omitempty"`
	SignalStrength    *int                   `json:"signal_strength,omitempty"`
	CustomMetrics     map[string]interface{} `json:"custom_metrics,omitempty"`
}

// NetworkHealthStats tracks network-related health metrics
type NetworkHealthStats struct {
	BytesSent       uint64    `json:"bytes_sent"`
	BytesReceived   uint64    `json:"bytes_received"`
	PacketsDropped  uint64    `json:"packets_dropped"`
	Latency         float64   `json:"latency_ms"`
	LastSeen        time.Time `json:"last_seen"`
	ConnectedDevices int      `json:"connected_devices"`
}

// MessageHealthStats tracks message processing metrics
type MessageHealthStats struct {
	MessagesSent      uint64 `json:"messages_sent"`
	MessagesReceived  uint64 `json:"messages_received"`
	MessagesFailed    uint64 `json:"messages_failed"`
	AverageProcessTime float64 `json:"avg_process_time_ms"`
	QueueSize         int    `json:"queue_size"`
}

// SecurityHealthStats tracks security-related metrics
type SecurityHealthStats struct {
	AuthenticationFailures uint64 `json:"auth_failures"`
	RateLimitViolations   uint64 `json:"rate_limit_violations"`
	TrustedDevicesCount   int    `json:"trusted_devices_count"`
	LastPairingAttempt    *time.Time `json:"last_pairing_attempt,omitempty"`
}

// HealthMonitor manages health metrics collection and reporting
type HealthMonitor struct {
	deviceID       string
	startTime      time.Time
	metrics        *HealthMetrics
	mutex          sync.RWMutex
	
	// Counters
	messagesSent     uint64
	messagesReceived uint64
	messagesFailed   uint64
	authFailures     uint64
	rateLimitHits    uint64
	
	// Network tracking
	bytesSent        uint64
	bytesReceived    uint64
	packetsDropped   uint64
	
	// Custom metrics
	customMetrics    map[string]interface{}
	customMutex      sync.RWMutex
}

// NewHealthMonitor creates a new health monitoring instance
func NewHealthMonitor(deviceID string) *HealthMonitor {
	return &HealthMonitor{
		deviceID:      deviceID,
		startTime:     time.Now(),
		customMetrics: make(map[string]interface{}),
	}
}

// UpdateMetrics refreshes all health metrics
func (hm *HealthMonitor) UpdateMetrics(service *HomeLinkService) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	now := time.Now()
	
	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	// Calculate memory usage percentage (approximation)
	memoryUsage := float64(memStats.Alloc) / float64(memStats.Sys) * 100
	
	// Get network stats from service
	networkStats := hm.calculateNetworkStats(service)
	messageStats := hm.calculateMessageStats()
	securityStats := hm.calculateSecurityStats(service)

	hm.metrics = &HealthMetrics{
		DeviceID:      hm.deviceID,
		Timestamp:     now.Unix(),
		Uptime:        now.Sub(hm.startTime),
		CPUUsage:      hm.getCPUUsage(), // Simplified CPU usage
		MemoryUsage:   memoryUsage,
		NetworkStats:  networkStats,
		MessageStats:  messageStats,
		SecurityStats: securityStats,
	}

	// Add custom metrics
	hm.customMutex.RLock()
	if len(hm.customMetrics) > 0 {
		hm.metrics.CustomMetrics = make(map[string]interface{})
		for k, v := range hm.customMetrics {
			hm.metrics.CustomMetrics[k] = v
		}
	}
	hm.customMutex.RUnlock()
}

// calculateNetworkStats computes network-related health metrics
func (hm *HealthMonitor) calculateNetworkStats(service *HomeLinkService) NetworkHealthStats {
	devices := service.GetDevices()
	connectedDevices := 0
	
	// Count devices seen in last 5 minutes
	cutoff := time.Now().Add(-5 * time.Minute)
	for _, device := range devices {
		if device.LastSeen.After(cutoff) {
			connectedDevices++
		}
	}

	return NetworkHealthStats{
		BytesSent:        hm.bytesSent,
		BytesReceived:    hm.bytesReceived,
		PacketsDropped:   hm.packetsDropped,
		Latency:          hm.calculateLatency(service),
		LastSeen:         time.Now(),
		ConnectedDevices: connectedDevices,
	}
}

// calculateMessageStats computes message processing metrics
func (hm *HealthMonitor) calculateMessageStats() MessageHealthStats {
	queueSize := 0
	// If we had access to the event channel, we could get its length
	// queueSize = len(service.eventChan)
	
	return MessageHealthStats{
		MessagesSent:       hm.messagesSent,
		MessagesReceived:   hm.messagesReceived,
		MessagesFailed:     hm.messagesFailed,
		AverageProcessTime: 1.5, // Placeholder - would need actual timing measurements
		QueueSize:          queueSize,
	}
}

// calculateSecurityStats computes security-related metrics
func (hm *HealthMonitor) calculateSecurityStats(service *HomeLinkService) SecurityHealthStats {
	trustedCount := 0
	if service.IsSecurityEnabled() {
		trustedCount = len(service.GetTrustedDevices())
	}

	return SecurityHealthStats{
		AuthenticationFailures: hm.authFailures,
		RateLimitViolations:   hm.rateLimitHits,
		TrustedDevicesCount:   trustedCount,
	}
}

// calculateLatency estimates network latency (simplified)
func (hm *HealthMonitor) calculateLatency(service *HomeLinkService) float64 {
	// In a real implementation, this would ping other devices and measure response time
	// For now, return a placeholder value
	return 25.0 // milliseconds
}

// getCPUUsage provides a simplified CPU usage metric
func (hm *HealthMonitor) getCPUUsage() float64 {
	// Simplified CPU usage based on number of goroutines
	// In a real implementation, you'd use system calls to get actual CPU usage
	numGoroutines := runtime.NumGoroutine()
	return float64(numGoroutines) * 2.0 // Rough approximation
}

// GetMetrics returns the current health metrics
func (hm *HealthMonitor) GetMetrics() *HealthMetrics {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()
	
	if hm.metrics == nil {
		return nil
	}
	
	// Return a copy to prevent race conditions
	metrics := *hm.metrics
	return &metrics
}

// GetMetricsJSON returns health metrics as JSON
func (hm *HealthMonitor) GetMetricsJSON() ([]byte, error) {
	metrics := hm.GetMetrics()
	if metrics == nil {
		return nil, nil
	}
	return json.Marshal(metrics)
}

// Counter increment methods
func (hm *HealthMonitor) IncrementMessagesSent(bytes uint64) {
	hm.mutex.Lock()
	hm.messagesSent++
	hm.bytesSent += bytes
	hm.mutex.Unlock()
}

func (hm *HealthMonitor) IncrementMessagesReceived(bytes uint64) {
	hm.mutex.Lock()
	hm.messagesReceived++
	hm.bytesReceived += bytes
	hm.mutex.Unlock()
}

func (hm *HealthMonitor) IncrementMessagesFailed() {
	hm.mutex.Lock()
	hm.messagesFailed++
	hm.mutex.Unlock()
}

func (hm *HealthMonitor) IncrementAuthFailures() {
	hm.mutex.Lock()
	hm.authFailures++
	hm.mutex.Unlock()
}

func (hm *HealthMonitor) IncrementRateLimitViolations() {
	hm.mutex.Lock()
	hm.rateLimitHits++
	hm.mutex.Unlock()
}

func (hm *HealthMonitor) IncrementPacketsDropped() {
	hm.mutex.Lock()
	hm.packetsDropped++
	hm.mutex.Unlock()
}

// Custom metrics management
func (hm *HealthMonitor) SetCustomMetric(key string, value interface{}) {
	hm.customMutex.Lock()
	hm.customMetrics[key] = value
	hm.customMutex.Unlock()
}

func (hm *HealthMonitor) GetCustomMetric(key string) (interface{}, bool) {
	hm.customMutex.RLock()
	defer hm.customMutex.RUnlock()
	value, exists := hm.customMetrics[key]
	return value, exists
}

func (hm *HealthMonitor) RemoveCustomMetric(key string) {
	hm.customMutex.Lock()
	delete(hm.customMetrics, key)
	hm.customMutex.Unlock()
}

// SetBatteryLevel sets the battery level for mobile/IoT devices
func (hm *HealthMonitor) SetBatteryLevel(level float64) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()
	
	if hm.metrics != nil {
		hm.metrics.BatteryLevel = &level
	}
}

// SetSignalStrength sets the signal strength for wireless devices
func (hm *HealthMonitor) SetSignalStrength(strength int) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()
	
	if hm.metrics != nil {
		hm.metrics.SignalStrength = &strength
	}
}

// GetHealthSummary returns a simplified health status
func (hm *HealthMonitor) GetHealthSummary() map[string]interface{} {
	metrics := hm.GetMetrics()
	if metrics == nil {
		return map[string]interface{}{
			"status": "unknown",
		}
	}

	status := "healthy"
	issues := []string{}

	// Check for health issues
	if metrics.MemoryUsage > 80 {
		status = "warning"
		issues = append(issues, "high memory usage")
	}
	
	if metrics.CPUUsage > 80 {
		status = "warning" 
		issues = append(issues, "high CPU usage")
	}

	if metrics.NetworkStats.ConnectedDevices == 0 {
		status = "warning"
		issues = append(issues, "no connected devices")
	}

	if metrics.MessageStats.MessagesFailed > 10 {
		status = "error"
		issues = append(issues, "high message failure rate")
	}

	if metrics.BatteryLevel != nil && *metrics.BatteryLevel < 20 {
		status = "warning"
		issues = append(issues, "low battery")
	}

	return map[string]interface{}{
		"status":     status,
		"uptime":     metrics.Uptime.String(),
		"devices":    metrics.NetworkStats.ConnectedDevices,
		"issues":     issues,
		"last_check": time.Unix(metrics.Timestamp, 0).Format(time.RFC3339),
	}
}