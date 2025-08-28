// HomeLink - Error Handling and Definitions
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"errors"
	"fmt"
	"log"
)

// Common error types
var (
	ErrInvalidMessage       = errors.New("invalid message format")
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrDeviceNotFound       = errors.New("device not found")
	ErrConnectionFailed     = errors.New("connection failed")
	ErrTimeout              = errors.New("operation timeout")
	ErrRateLimit            = errors.New("rate limit exceeded")
	ErrInvalidPayload       = errors.New("invalid payload")
	ErrSecurityRequired     = errors.New("security required")
	ErrInvalidConfiguration = errors.New("invalid configuration")
)

// ErrorHandler provides centralized error handling
type ErrorHandler struct {
	deviceID string
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(deviceID string) *ErrorHandler {
	return &ErrorHandler{
		deviceID: deviceID,
	}
}

// HandleNetworkError handles network-related errors with context
func (eh *ErrorHandler) HandleNetworkError(operation string, err error) error {
	if err != nil {
		log.Printf("[%s] Network error in %s: %v", eh.deviceID, operation, err)
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	return nil
}

// HandleParsingError handles message parsing errors
func (eh *ErrorHandler) HandleParsingError(source string, err error) error {
	if err != nil {
		log.Printf("[%s] Failed to parse message from %s: %v", eh.deviceID, source, err)
		return fmt.Errorf("message parsing failed: %w", ErrInvalidMessage)
	}
	return nil
}

// HandleSecurityError handles security-related errors
func (eh *ErrorHandler) HandleSecurityError(operation string, err error) error {
	if err != nil {
		log.Printf("[%s] Security error in %s: %v", eh.deviceID, operation, err)
		return fmt.Errorf("security %s failed: %w", operation, err)
	}
	return nil
}

// LogError logs an error with device context
func (eh *ErrorHandler) LogError(operation string, err error) {
	if err != nil {
		log.Printf("[%s] Error in %s: %v", eh.deviceID, operation, err)
	}
}

// LogWarning logs a warning with device context
func (eh *ErrorHandler) LogWarning(operation string, message string) {
	log.Printf("[%s] Warning in %s: %s", eh.deviceID, operation, message)
}

// LogInfo logs an info message with device context
func (eh *ErrorHandler) LogInfo(operation string, message string) {
	log.Printf("[%s] %s: %s", eh.deviceID, operation, message)
}

// IsNetworkError checks if an error is network-related
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrConnectionFailed) ||
		errors.Is(err, ErrTimeout)
}

// IsSecurityError checks if an error is security-related
func IsSecurityError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrAuthenticationFailed) ||
		errors.Is(err, ErrSecurityRequired)
}

// WrapError wraps an error with additional context
func WrapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
