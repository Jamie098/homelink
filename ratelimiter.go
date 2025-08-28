// HomeLink - Rate Limiting for Network Security
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"time"
)

// NewRateLimiter creates a new rate limiter with specified limits
func NewRateLimiter(maxRate, burstSize int) *RateLimiter {
	return &RateLimiter{
		deviceRates: make(map[string]*DeviceRateInfo),
		maxRate:     maxRate,
		burstSize:   burstSize,
	}
}

// AllowMessage checks if a message from a device should be allowed
func (rl *RateLimiter) AllowMessage(deviceID string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()

	// Get or create device rate info
	rateInfo, exists := rl.deviceRates[deviceID]
	if !exists {
		rateInfo = &DeviceRateInfo{
			tokens:     rl.burstSize,
			lastRefill: now,
		}
		rl.deviceRates[deviceID] = rateInfo
	}

	// Calculate tokens to add based on time elapsed
	elapsed := now.Sub(rateInfo.lastRefill)
	tokensToAdd := int(elapsed.Seconds() * float64(rl.maxRate))

	if tokensToAdd > 0 {
		rateInfo.tokens += tokensToAdd
		if rateInfo.tokens > rl.burstSize {
			rateInfo.tokens = rl.burstSize
		}
		rateInfo.lastRefill = now
	}

	// Check if we have tokens available
	if rateInfo.tokens > 0 {
		rateInfo.tokens--
		return true
	}

	return false
}

// GetRateInfo returns current rate limiting info for a device
func (rl *RateLimiter) GetRateInfo(deviceID string) (tokens int, lastRefill time.Time) {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	if rateInfo, exists := rl.deviceRates[deviceID]; exists {
		return rateInfo.tokens, rateInfo.lastRefill
	}
	return rl.burstSize, time.Now() // New device starts with full tokens
}

// ResetDevice resets rate limiting for a specific device
func (rl *RateLimiter) ResetDevice(deviceID string) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	if _, exists := rl.deviceRates[deviceID]; exists {
		rl.deviceRates[deviceID] = &DeviceRateInfo{
			tokens:     rl.burstSize,
			lastRefill: time.Now(),
		}
	}
}

// RemoveDevice removes rate limiting data for a device
func (rl *RateLimiter) RemoveDevice(deviceID string) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	delete(rl.deviceRates, deviceID)
}

// GetAllDeviceRates returns rate info for all devices
func (rl *RateLimiter) GetAllDeviceRates() map[string]DeviceRateInfo {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	result := make(map[string]DeviceRateInfo)
	for deviceID, rateInfo := range rl.deviceRates {
		result[deviceID] = *rateInfo
	}
	return result
}

// CleanupOldDevices removes rate limiting data for devices not seen recently
func (rl *RateLimiter) CleanupOldDevices(maxAge time.Duration) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	for deviceID, rateInfo := range rl.deviceRates {
		if now.Sub(rateInfo.lastRefill) > maxAge {
			delete(rl.deviceRates, deviceID)
		}
	}
}
