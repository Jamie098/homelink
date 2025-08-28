// HomeLink - Web Dashboard
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"
)

// DashboardServer serves the web dashboard
type DashboardServer struct {
	service *HomeLinkService
	health  *HealthMonitor
}

// NewDashboardServer creates a new dashboard server
func NewDashboardServer(service *HomeLinkService, health *HealthMonitor) *DashboardServer {
	return &DashboardServer{
		service: service,
		health:  health,
	}
}

// SetupRoutes configures the dashboard HTTP routes
func (ds *DashboardServer) SetupRoutes(mux *http.ServeMux) {
	// Serve dashboard HTML
	mux.HandleFunc("/dashboard", ds.dashboardHandler)
	mux.HandleFunc("/dashboard/", ds.dashboardHandler)

	// API endpoints for dashboard data
	mux.HandleFunc("/api/dashboard/status", ds.statusHandler)
	mux.HandleFunc("/api/dashboard/devices", ds.devicesHandler)
	mux.HandleFunc("/api/dashboard/health", ds.healthHandler)
	mux.HandleFunc("/api/dashboard/events", ds.eventsHandler)
	mux.HandleFunc("/api/dashboard/security", ds.securityHandler)
}

// dashboardHandler serves the main dashboard HTML
func (ds *DashboardServer) dashboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get basic service info for template
	status := ds.getServiceStatus()

	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))
	tmpl.Execute(w, status)
}

// statusHandler returns basic service status
func (ds *DashboardServer) statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	status := ds.getServiceStatus()
	json.NewEncoder(w).Encode(status)
}

// devicesHandler returns device information
func (ds *DashboardServer) devicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	devices := ds.service.GetDevices()
	deviceList := make([]map[string]interface{}, 0, len(devices))

	for _, device := range devices {
		deviceInfo := map[string]interface{}{
			"id":           device.ID,
			"name":         device.Name,
			"capabilities": device.Capabilities,
			"last_seen":    device.LastSeen.Format(time.RFC3339),
			"trusted":      device.Trusted,
			"address":      "",
		}

		if device.Address != nil {
			deviceInfo["address"] = device.Address.String()
		}

		// Add time since last seen
		timeSince := time.Since(device.LastSeen)
		if timeSince < time.Minute {
			deviceInfo["status"] = "online"
			deviceInfo["last_seen_human"] = "just now"
		} else if timeSince < time.Hour {
			deviceInfo["status"] = "recent"
			deviceInfo["last_seen_human"] = fmt.Sprintf("%d minutes ago", int(timeSince.Minutes()))
		} else {
			deviceInfo["status"] = "offline"
			deviceInfo["last_seen_human"] = fmt.Sprintf("%d hours ago", int(timeSince.Hours()))
		}

		deviceList = append(deviceList, deviceInfo)
	}

	response := map[string]interface{}{
		"devices": deviceList,
		"count":   len(deviceList),
	}

	json.NewEncoder(w).Encode(response)
}

// healthHandler returns health metrics
func (ds *DashboardServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if ds.health != nil {
		// Update metrics before returning
		ds.health.UpdateMetrics(ds.service)
		metrics := ds.health.GetMetrics()
		summary := ds.health.GetHealthSummary()

		response := map[string]interface{}{
			"metrics": metrics,
			"summary": summary,
		}

		json.NewEncoder(w).Encode(response)
	} else {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Health monitoring not enabled",
		})
	}
}

// eventsHandler returns recent events (placeholder)
func (ds *DashboardServer) eventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// This is a placeholder - in a real implementation you'd store recent events
	events := []map[string]interface{}{
		{
			"timestamp":   time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
			"type":        "device_connected",
			"device_id":   "camera-01",
			"description": "Device camera-01 connected to network",
		},
		{
			"timestamp":   time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
			"type":        "motion_detected",
			"device_id":   "sensor-front",
			"description": "Motion detected at front door",
		},
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
	})
}

// securityHandler returns security information
func (ds *DashboardServer) securityHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if ds.service.IsSecurityEnabled() {
		stats := ds.service.GetSecurityStats()
		json.NewEncoder(w).Encode(stats)
	} else {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"security_enabled": false,
			"message":          "Security features are not enabled",
		})
	}
}

// getServiceStatus returns basic service information
func (ds *DashboardServer) getServiceStatus() map[string]interface{} {
	devices := ds.service.GetDevices()

	// Count online devices (seen within last 5 minutes)
	onlineCount := 0
	cutoff := time.Now().Add(-5 * time.Minute)
	for _, device := range devices {
		if device.LastSeen.After(cutoff) {
			onlineCount++
		}
	}

	status := map[string]interface{}{
		"device_name":      "HomeLink Service", // Would come from service
		"device_id":        "homelink-service", // Would come from service
		"total_devices":    len(devices),
		"online_devices":   onlineCount,
		"security_enabled": ds.service.IsSecurityEnabled(),
		"uptime":           time.Since(time.Now().Add(-1 * time.Hour)).String(), // Placeholder
		"version":          "1.0.0",
	}

	return status
}

// dashboardHTML contains the embedded HTML for the dashboard
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>HomeLink Dashboard</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: #f5f7fa;
            color: #333;
            line-height: 1.6;
        }
        
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 1rem 2rem;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        
        .header h1 {
            font-size: 2rem;
            margin-bottom: 0.5rem;
        }
        
        .header p {
            opacity: 0.9;
        }
        
        .container {
            max-width: 1200px;
            margin: 2rem auto;
            padding: 0 1rem;
        }
        
        .dashboard-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 2rem;
            margin-bottom: 2rem;
        }
        
        .card {
            background: white;
            border-radius: 12px;
            padding: 1.5rem;
            box-shadow: 0 4px 20px rgba(0,0,0,0.08);
            transition: transform 0.2s ease, box-shadow 0.2s ease;
        }
        
        .card:hover {
            transform: translateY(-2px);
            box-shadow: 0 8px 30px rgba(0,0,0,0.12);
        }
        
        .card h3 {
            color: #4a5568;
            margin-bottom: 1rem;
            font-size: 1.2rem;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        
        .status-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 1rem;
        }
        
        .status-item {
            text-align: center;
            padding: 1rem;
            background: #f8fafc;
            border-radius: 8px;
        }
        
        .status-value {
            font-size: 2rem;
            font-weight: bold;
            color: #667eea;
        }
        
        .status-label {
            font-size: 0.9rem;
            color: #718096;
            margin-top: 0.25rem;
        }
        
        .device-list {
            max-height: 300px;
            overflow-y: auto;
        }
        
        .device-item {
            display: flex;
            justify-content: between;
            align-items: center;
            padding: 0.75rem;
            border-bottom: 1px solid #e2e8f0;
            transition: background-color 0.2s ease;
        }
        
        .device-item:hover {
            background-color: #f7fafc;
        }
        
        .device-item:last-child {
            border-bottom: none;
        }
        
        .device-info {
            flex-grow: 1;
        }
        
        .device-name {
            font-weight: 600;
            color: #2d3748;
        }
        
        .device-id {
            font-size: 0.85rem;
            color: #718096;
        }
        
        .device-status {
            padding: 0.25rem 0.75rem;
            border-radius: 20px;
            font-size: 0.8rem;
            font-weight: 500;
        }
        
        .status-online {
            background: #c6f6d5;
            color: #2f855a;
        }
        
        .status-recent {
            background: #fef5e7;
            color: #d69e2e;
        }
        
        .status-offline {
            background: #fed7d7;
            color: #c53030;
        }
        
        .health-metrics {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 1rem;
        }
        
        .metric-item {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 0.5rem 0;
        }
        
        .metric-label {
            color: #4a5568;
        }
        
        .metric-value {
            font-weight: 600;
            color: #2d3748;
        }
        
        .events-list {
            max-height: 250px;
            overflow-y: auto;
        }
        
        .event-item {
            padding: 0.75rem;
            border-left: 4px solid #667eea;
            margin-bottom: 0.5rem;
            background: #f7fafc;
            border-radius: 0 8px 8px 0;
        }
        
        .event-type {
            font-weight: 600;
            color: #4a5568;
            font-size: 0.9rem;
        }
        
        .event-description {
            color: #718096;
            font-size: 0.85rem;
            margin: 0.25rem 0;
        }
        
        .event-time {
            color: #a0aec0;
            font-size: 0.8rem;
        }
        
        .loading {
            text-align: center;
            padding: 2rem;
            color: #718096;
        }
        
        .error {
            background: #fed7d7;
            color: #c53030;
            padding: 1rem;
            border-radius: 8px;
            margin: 1rem 0;
        }
        
        .refresh-btn {
            background: #667eea;
            color: white;
            border: none;
            padding: 0.5rem 1rem;
            border-radius: 6px;
            cursor: pointer;
            font-size: 0.9rem;
            transition: background-color 0.2s ease;
        }
        
        .refresh-btn:hover {
            background: #5a67d8;
        }
        
        @media (max-width: 768px) {
            .header {
                padding: 1rem;
            }
            
            .container {
                margin: 1rem auto;
                padding: 0 0.5rem;
            }
            
            .dashboard-grid {
                grid-template-columns: 1fr;
                gap: 1rem;
            }
            
            .status-grid {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <header class="header">
        <h1>🏠 HomeLink Dashboard</h1>
        <p>Real-time device monitoring and management</p>
    </header>
    
    <div class="container">
        <div class="dashboard-grid">
            <!-- Service Status -->
            <div class="card">
                <h3>📊 Service Status</h3>
                <div class="status-grid" id="serviceStatus">
                    <div class="loading">Loading...</div>
                </div>
                <button class="refresh-btn" onclick="loadAllData()">🔄 Refresh</button>
            </div>
            
            <!-- Connected Devices -->
            <div class="card">
                <h3>🔗 Connected Devices</h3>
                <div class="device-list" id="deviceList">
                    <div class="loading">Loading devices...</div>
                </div>
            </div>
            
            <!-- Health Metrics -->
            <div class="card">
                <h3>💚 Health Metrics</h3>
                <div class="health-metrics" id="healthMetrics">
                    <div class="loading">Loading health data...</div>
                </div>
            </div>
            
            <!-- Recent Events -->
            <div class="card">
                <h3>📅 Recent Events</h3>
                <div class="events-list" id="eventsList">
                    <div class="loading">Loading events...</div>
                </div>
            </div>
            
            <!-- Security Status -->
            <div class="card">
                <h3>🔐 Security Status</h3>
                <div id="securityStatus">
                    <div class="loading">Loading security info...</div>
                </div>
            </div>
        </div>
    </div>

    <script>
        // Auto-refresh every 30 seconds
        let refreshInterval;
        
        // Load all dashboard data
        async function loadAllData() {
            try {
                await Promise.all([
                    loadServiceStatus(),
                    loadDevices(),
                    loadHealthMetrics(),
                    loadRecentEvents(),
                    loadSecurityStatus()
                ]);
            } catch (error) {
                console.error('Error loading dashboard data:', error);
            }
        }
        
        // Load service status
        async function loadServiceStatus() {
            try {
                const response = await fetch('/api/dashboard/status');
                const data = await response.json();
                
                document.getElementById('serviceStatus').innerHTML = ` + "`" + `
                    <div class="status-item">
                        <div class="status-value">${data.total_devices || 0}</div>
                        <div class="status-label">Total Devices</div>
                    </div>
                    <div class="status-item">
                        <div class="status-value">${data.online_devices || 0}</div>
                        <div class="status-label">Online Now</div>
                    </div>
                    <div class="status-item">
                        <div class="status-value">${data.security_enabled ? '🔒' : '🔓'}</div>
                        <div class="status-label">Security</div>
                    </div>
                    <div class="status-item">
                        <div class="status-value">${data.uptime || 'N/A'}</div>
                        <div class="status-label">Uptime</div>
                    </div>
                ` + "`" + `;
            } catch (error) {
                document.getElementById('serviceStatus').innerHTML = '<div class="error">Failed to load status</div>';
            }
        }
        
        // Load devices
        async function loadDevices() {
            try {
                const response = await fetch('/api/dashboard/devices');
                const data = await response.json();
                
                if (data.devices && data.devices.length > 0) {
                    const deviceHtml = data.devices.map(device => ` + "`" + `
                        <div class="device-item">
                            <div class="device-info">
                                <div class="device-name">${device.name || 'Unknown Device'}</div>
                                <div class="device-id">${device.id}</div>
                            </div>
                            <div class="device-status status-${device.status}">
                                ${device.status}
                            </div>
                        </div>
                    ` + "`" + `).join('');
                    
                    document.getElementById('deviceList').innerHTML = deviceHtml;
                } else {
                    document.getElementById('deviceList').innerHTML = '<div style="text-align: center; color: #718096; padding: 2rem;">No devices found</div>';
                }
            } catch (error) {
                document.getElementById('deviceList').innerHTML = '<div class="error">Failed to load devices</div>';
            }
        }
        
        // Load health metrics
        async function loadHealthMetrics() {
            try {
                const response = await fetch('/api/dashboard/health');
                const data = await response.json();
                
                if (data.metrics) {
                    const metrics = data.metrics;
                    document.getElementById('healthMetrics').innerHTML = ` + "`" + `
                        <div class="metric-item">
                            <span class="metric-label">CPU Usage</span>
                            <span class="metric-value">${metrics.cpu_usage ? metrics.cpu_usage.toFixed(1) + '%' : 'N/A'}</span>
                        </div>
                        <div class="metric-item">
                            <span class="metric-label">Memory Usage</span>
                            <span class="metric-value">${metrics.memory_usage ? metrics.memory_usage.toFixed(1) + '%' : 'N/A'}</span>
                        </div>
                        <div class="metric-item">
                            <span class="metric-label">Messages Sent</span>
                            <span class="metric-value">${metrics.message_stats?.messages_sent || 0}</span>
                        </div>
                        <div class="metric-item">
                            <span class="metric-label">Messages Received</span>
                            <span class="metric-value">${metrics.message_stats?.messages_received || 0}</span>
                        </div>
                    ` + "`" + `;
                } else {
                    document.getElementById('healthMetrics').innerHTML = '<div style="text-align: center; color: #718096;">Health monitoring not available</div>';
                }
            } catch (error) {
                document.getElementById('healthMetrics').innerHTML = '<div class="error">Failed to load health metrics</div>';
            }
        }
        
        // Load recent events
        async function loadRecentEvents() {
            try {
                const response = await fetch('/api/dashboard/events');
                const data = await response.json();
                
                if (data.events && data.events.length > 0) {
                    const eventsHtml = data.events.map(event => ` + "`" + `
                        <div class="event-item">
                            <div class="event-type">${event.type}</div>
                            <div class="event-description">${event.description}</div>
                            <div class="event-time">${new Date(event.timestamp).toLocaleString()}</div>
                        </div>
                    ` + "`" + `).join('');
                    
                    document.getElementById('eventsList').innerHTML = eventsHtml;
                } else {
                    document.getElementById('eventsList').innerHTML = '<div style="text-align: center; color: #718096; padding: 2rem;">No recent events</div>';
                }
            } catch (error) {
                document.getElementById('eventsList').innerHTML = '<div class="error">Failed to load events</div>';
            }
        }
        
        // Load security status
        async function loadSecurityStatus() {
            try {
                const response = await fetch('/api/dashboard/security');
                const data = await response.json();
                
                if (data.security_enabled) {
                    document.getElementById('securityStatus').innerHTML = ` + "`" + `
                        <div class="metric-item">
                            <span class="metric-label">Security</span>
                            <span class="metric-value">✅ Enabled</span>
                        </div>
                        <div class="metric-item">
                            <span class="metric-label">Trusted Devices</span>
                            <span class="metric-value">${data.trusted_devices ? data.trusted_devices.length : 0}</span>
                        </div>
                        <div class="metric-item">
                            <span class="metric-label">Auth Required</span>
                            <span class="metric-value">${data.require_authentication ? '✅' : '❌'}</span>
                        </div>
                    ` + "`" + `;
                } else {
                    document.getElementById('securityStatus').innerHTML = '<div style="text-align: center; color: #718096;">Security features are disabled</div>';
                }
            } catch (error) {
                document.getElementById('securityStatus').innerHTML = '<div class="error">Failed to load security status</div>';
            }
        }
        
        // Initialize dashboard
        document.addEventListener('DOMContentLoaded', function() {
            loadAllData();
            
            // Set up auto-refresh
            refreshInterval = setInterval(loadAllData, 30000);
        });
        
        // Clean up on page unload
        window.addEventListener('beforeunload', function() {
            if (refreshInterval) {
                clearInterval(refreshInterval);
            }
        });
    </script>
</body>
</html>`
