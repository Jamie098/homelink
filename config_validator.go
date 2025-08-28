// HomeLink - Configuration Validation
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ConfigValidator validates HomeLink configurations
type ConfigValidator struct{}

// NewConfigValidator creates a new configuration validator
func NewConfigValidator() *ConfigValidator {
	return &ConfigValidator{}
}

// ValidateSecurityConfig validates security configuration
func (cv *ConfigValidator) ValidateSecurityConfig(config *SecurityConfig) error {
	if config == nil {
		return nil // Security is optional
	}

	if config.Enabled {
		if config.MaxMessageAge <= 0 {
			return errors.New("max_message_age must be positive when security is enabled")
		}

		if config.RateLimitPerSecond < 0 {
			return errors.New("rate_limit_per_second cannot be negative")
		}

		if config.RateLimitBurstSize < 0 {
			return errors.New("rate_limit_burst_size cannot be negative")
		}

		if config.RateLimitBurstSize < config.RateLimitPerSecond {
			return errors.New("rate_limit_burst_size should be >= rate_limit_per_second")
		}
	}

	return nil
}

// ValidateStorageConfig validates storage configuration
func (cv *ConfigValidator) ValidateStorageConfig(config *StorageConfig) error {
	if config == nil {
		return nil // Storage is optional
	}

	if config.Enabled {
		if config.DatabasePath == "" {
			return errors.New("database_path is required when storage is enabled")
		}

		if config.RetentionDays < 0 {
			return errors.New("retention_days cannot be negative")
		}

		if config.MaxEvents < 0 {
			return errors.New("max_events cannot be negative")
		}

		if config.BatchSize <= 0 {
			return errors.New("batch_size must be positive")
		}

		if config.FlushInterval <= 0 {
			return errors.New("flush_interval must be positive")
		}

		if config.BackupEnabled {
			if config.BackupPath == "" {
				return errors.New("backup_path is required when backup is enabled")
			}

			if config.BackupInterval <= 0 {
				return errors.New("backup_interval must be positive when backup is enabled")
			}
		}
	}

	return nil
}

// ValidateDeviceInfo validates basic device information
func (cv *ConfigValidator) ValidateDeviceInfo(deviceID, deviceName string, capabilities []string) error {
	if deviceID == "" {
		return errors.New("device_id cannot be empty")
	}

	if deviceName == "" {
		return errors.New("device_name cannot be empty")
	}

	if len(deviceID) > 64 {
		return errors.New("device_id cannot be longer than 64 characters")
	}

	if len(deviceName) > 256 {
		return errors.New("device_name cannot be longer than 256 characters")
	}

	// Validate device ID contains only safe characters
	for _, char := range deviceID {
		if !isValidDeviceIDChar(char) {
			return fmt.Errorf("device_id contains invalid character: %c", char)
		}
	}

	// Validate capabilities
	for i, cap := range capabilities {
		if cap == "" {
			return fmt.Errorf("capability at index %d cannot be empty", i)
		}
		if len(cap) > 64 {
			return fmt.Errorf("capability at index %d cannot be longer than 64 characters", i)
		}
	}

	return nil
}

// ValidateProtocolMode validates protocol mode
func (cv *ConfigValidator) ValidateProtocolMode(mode ProtocolMode) error {
	switch mode {
	case ProtocolJSON, ProtocolBinary, ProtocolAuto:
		return nil
	default:
		return fmt.Errorf("invalid protocol mode: %d", mode)
	}
}

// ValidateMessage validates message structure
func (cv *ConfigValidator) ValidateMessage(msg *Message) error {
	if msg == nil {
		return errors.New("message cannot be nil")
	}

	if msg.Type == "" {
		return errors.New("message type cannot be empty")
	}

	if msg.DeviceID == "" {
		return errors.New("device_id cannot be empty")
	}

	if msg.Timestamp <= 0 {
		return errors.New("timestamp must be positive")
	}

	// Check if timestamp is too far in the future or past
	now := time.Now().Unix()
	if msg.Timestamp > now+300 { // 5 minutes in the future
		return errors.New("timestamp too far in the future")
	}

	if msg.Timestamp < now-3600 { // 1 hour in the past
		return errors.New("timestamp too far in the past")
	}

	return nil
}

// isValidDeviceIDChar checks if a character is valid for device IDs
func isValidDeviceIDChar(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == '-' || char == '_' || char == '.'
}

// SanitizeDeviceID sanitizes a device ID by removing invalid characters
func (cv *ConfigValidator) SanitizeDeviceID(deviceID string) string {
	var result strings.Builder
	for _, char := range deviceID {
		if isValidDeviceIDChar(char) {
			result.WriteRune(char)
		}
	}
	return result.String()
}

// ValidateEventTypes validates event type names
func (cv *ConfigValidator) ValidateEventTypes(eventTypes []string) error {
	for i, eventType := range eventTypes {
		if eventType == "" {
			return fmt.Errorf("event type at index %d cannot be empty", i)
		}
		if len(eventType) > 64 {
			return fmt.Errorf("event type at index %d cannot be longer than 64 characters", i)
		}
		// Event types should only contain safe characters
		for _, char := range eventType {
			if !isValidEventTypeChar(char) {
				return fmt.Errorf("event type at index %d contains invalid character: %c", i, char)
			}
		}
	}
	return nil
}

// isValidEventTypeChar checks if a character is valid for event types
func isValidEventTypeChar(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == '-' || char == '_'
}
