// HomeLink - Security and Cryptography
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// SecurityManager handles all cryptographic operations for HomeLink
type SecurityManager struct {
	deviceID     string
	privateKey   ed25519.PrivateKey
	publicKey    ed25519.PublicKey
	networkKey   []byte // Shared network key for AES encryption
	trustedKeys  map[string]ed25519.PublicKey
	keysMutex    sync.RWMutex
	nonceTracker *NonceTracker
}

// NonceTracker prevents replay attacks
type NonceTracker struct {
	usedNonces map[string]time.Time
	mutex      sync.RWMutex
	maxAge     time.Duration
}

// SecureMessage wraps all encrypted communications
type SecureMessage struct {
	DeviceID  string `json:"device_id"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
	Encrypted string `json:"encrypted"`
	Timestamp int64  `json:"timestamp"`
}

// DevicePairingRequest represents a device requesting to join the network
type DevicePairingRequest struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	Challenge  string `json:"challenge"`
	Timestamp  int64  `json:"timestamp"`
	Signature  string `json:"signature"`
}

// DevicePairingResponse confirms acceptance into the network
type DevicePairingResponse struct {
	Accepted   bool   `json:"accepted"`
	NetworkKey string `json:"network_key,omitempty"` // Encrypted with requester's public key
	Challenge  string `json:"challenge"`
	Signature  string `json:"signature"`
}

// NewSecurityManager creates a new security manager with fresh keys
func NewSecurityManager(deviceID string) (*SecurityManager, error) {
	// Generate Ed25519 key pair for device identity
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate device keys: %v", err)
	}

	// Generate network key for AES encryption (32 bytes for AES-256)
	networkKey := make([]byte, 32)
	if _, err := rand.Read(networkKey); err != nil {
		return nil, fmt.Errorf("failed to generate network key: %v", err)
	}

	nonceTracker := &NonceTracker{
		usedNonces: make(map[string]time.Time),
		maxAge:     5 * time.Minute, // Nonces expire after 5 minutes
	}

	sm := &SecurityManager{
		deviceID:     deviceID,
		privateKey:   privateKey,
		publicKey:    publicKey,
		networkKey:   networkKey,
		trustedKeys:  make(map[string]ed25519.PublicKey),
		nonceTracker: nonceTracker,
	}

	// Start nonce cleanup routine
	go sm.cleanupNonces()

	return sm, nil
}

// GetPublicKey returns the device's public key as base64
func (sm *SecurityManager) GetPublicKey() string {
	return base64.StdEncoding.EncodeToString(sm.publicKey)
}

// GetNetworkKey returns the network key as base64 (for initial setup only)
func (sm *SecurityManager) GetNetworkKey() string {
	return base64.StdEncoding.EncodeToString(sm.networkKey)
}

// SetNetworkKey sets the network key from base64 (for joining existing network)
func (sm *SecurityManager) SetNetworkKey(networkKeyB64 string) error {
	networkKey, err := base64.StdEncoding.DecodeString(networkKeyB64)
	if err != nil {
		return fmt.Errorf("invalid network key format: %v", err)
	}
	if len(networkKey) != 32 {
		return errors.New("network key must be 32 bytes")
	}
	sm.networkKey = networkKey
	return nil
}

// AddTrustedDevice adds a device's public key to the trusted list
func (sm *SecurityManager) AddTrustedDevice(deviceID, publicKeyB64 string) error {
	publicKey, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return fmt.Errorf("invalid public key format: %v", err)
	}

	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size: got %d, want %d", len(publicKey), ed25519.PublicKeySize)
	}

	sm.keysMutex.Lock()
	sm.trustedKeys[deviceID] = ed25519.PublicKey(publicKey)
	sm.keysMutex.Unlock()

	return nil
}

// RemoveTrustedDevice removes a device from the trusted list
func (sm *SecurityManager) RemoveTrustedDevice(deviceID string) {
	sm.keysMutex.Lock()
	delete(sm.trustedKeys, deviceID)
	sm.keysMutex.Unlock()
}

// GetTrustedDevices returns a list of trusted device IDs
func (sm *SecurityManager) GetTrustedDevices() []string {
	sm.keysMutex.RLock()
	devices := make([]string, 0, len(sm.trustedKeys))
	for deviceID := range sm.trustedKeys {
		devices = append(devices, deviceID)
	}
	sm.keysMutex.RUnlock()
	return devices
}

// EncryptMessage encrypts a message for network transmission
func (sm *SecurityManager) EncryptMessage(message *Message) (*SecureMessage, error) {
	// Serialize the message
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message: %v", err)
	}

	// Generate a random nonce for AES-GCM
	nonce := make([]byte, 12) // GCM standard nonce size
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %v", err)
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(sm.networkKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %v", err)
	}

	// Encrypt the message
	encrypted := gcm.Seal(nil, nonce, messageBytes, nil)

	// Create secure message wrapper
	secureMsg := &SecureMessage{
		DeviceID:  sm.deviceID,
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Encrypted: base64.StdEncoding.EncodeToString(encrypted),
		Timestamp: time.Now().Unix(),
	}

	// Sign the secure message
	if err := sm.signSecureMessage(secureMsg); err != nil {
		return nil, fmt.Errorf("failed to sign message: %v", err)
	}

	return secureMsg, nil
}

// DecryptMessage decrypts a received secure message
func (sm *SecurityManager) DecryptMessage(secureMsg *SecureMessage) (*Message, error) {
	// Verify the message signature first
	if !sm.verifySecureMessage(secureMsg) {
		return nil, errors.New("message signature verification failed")
	}

	// Check for replay attacks
	if !sm.nonceTracker.checkAndAddNonce(secureMsg.DeviceID + ":" + secureMsg.Nonce) {
		return nil, errors.New("replay attack detected")
	}

	// Check message age (prevent very old messages)
	if time.Since(time.Unix(secureMsg.Timestamp, 0)) > 5*time.Minute {
		return nil, errors.New("message too old")
	}

	// Decode nonce and encrypted data
	nonce, err := base64.StdEncoding.DecodeString(secureMsg.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %v", err)
	}

	encrypted, err := base64.StdEncoding.DecodeString(secureMsg.Encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted data: %v", err)
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(sm.networkKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %v", err)
	}

	// Decrypt the message
	messageBytes, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt message: %v", err)
	}

	// Deserialize the message
	var message Message
	if err := json.Unmarshal(messageBytes, &message); err != nil {
		return nil, fmt.Errorf("failed to deserialize message: %v", err)
	}

	return &message, nil
}

// signSecureMessage signs a secure message with the device's private key
func (sm *SecurityManager) signSecureMessage(secureMsg *SecureMessage) error {
	// Create signing payload (everything except signature)
	signingData := fmt.Sprintf("%s:%s:%s:%d",
		secureMsg.DeviceID, secureMsg.Nonce, secureMsg.Encrypted, secureMsg.Timestamp)

	signature := ed25519.Sign(sm.privateKey, []byte(signingData))
	secureMsg.Signature = base64.StdEncoding.EncodeToString(signature)

	return nil
}

// verifySecureMessage verifies a secure message signature
func (sm *SecurityManager) verifySecureMessage(secureMsg *SecureMessage) bool {
	sm.keysMutex.RLock()
	publicKey, exists := sm.trustedKeys[secureMsg.DeviceID]
	sm.keysMutex.RUnlock()

	if !exists {
		return false // Unknown device
	}

	signature, err := base64.StdEncoding.DecodeString(secureMsg.Signature)
	if err != nil {
		return false
	}

	signingData := fmt.Sprintf("%s:%s:%s:%d",
		secureMsg.DeviceID, secureMsg.Nonce, secureMsg.Encrypted, secureMsg.Timestamp)

	return ed25519.Verify(publicKey, []byte(signingData), signature)
}

// GeneratePairingChallenge creates a challenge for device pairing
func (sm *SecurityManager) GeneratePairingChallenge() (string, error) {
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return "", fmt.Errorf("failed to generate challenge: %v", err)
	}
	return base64.StdEncoding.EncodeToString(challenge), nil
}

// CreatePairingRequest creates a pairing request for joining a network
func (sm *SecurityManager) CreatePairingRequest(deviceName, challenge string) (*DevicePairingRequest, error) {
	req := &DevicePairingRequest{
		DeviceID:   sm.deviceID,
		DeviceName: deviceName,
		PublicKey:  sm.GetPublicKey(),
		Challenge:  challenge,
		Timestamp:  time.Now().Unix(),
	}

	// Sign the request
	signingData := fmt.Sprintf("%s:%s:%s:%s:%d",
		req.DeviceID, req.DeviceName, req.PublicKey, req.Challenge, req.Timestamp)
	signature := ed25519.Sign(sm.privateKey, []byte(signingData))
	req.Signature = base64.StdEncoding.EncodeToString(signature)

	return req, nil
}

// VerifyPairingRequest verifies a pairing request
func (sm *SecurityManager) VerifyPairingRequest(req *DevicePairingRequest) bool {
	// Decode public key
	publicKey, err := base64.StdEncoding.DecodeString(req.PublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return false
	}

	// Verify signature
	signature, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		return false
	}

	signingData := fmt.Sprintf("%s:%s:%s:%s:%d",
		req.DeviceID, req.DeviceName, req.PublicKey, req.Challenge, req.Timestamp)

	return ed25519.Verify(ed25519.PublicKey(publicKey), []byte(signingData), signature)
}

// CreatePairingResponse creates a response to a pairing request
func (sm *SecurityManager) CreatePairingResponse(req *DevicePairingRequest, accepted bool) (*DevicePairingResponse, error) {
	resp := &DevicePairingResponse{
		Accepted:  accepted,
		Challenge: req.Challenge,
	}

	if accepted {
		// Encrypt network key with requester's public key (hybrid encryption)
		// In a real implementation, you'd use RSA or ECDH for this
		// For simplicity, we'll include it as base64 (this should be properly encrypted)
		resp.NetworkKey = sm.GetNetworkKey()
	}

	// Sign the response
	signingData := fmt.Sprintf("%t:%s:%s", resp.Accepted, resp.NetworkKey, resp.Challenge)
	signature := ed25519.Sign(sm.privateKey, []byte(signingData))
	resp.Signature = base64.StdEncoding.EncodeToString(signature)

	return resp, nil
}

// checkAndAddNonce checks if a nonce has been used and adds it to the tracker
func (nt *NonceTracker) checkAndAddNonce(nonce string) bool {
	nt.mutex.Lock()
	defer nt.mutex.Unlock()

	// Check if nonce already exists
	if _, exists := nt.usedNonces[nonce]; exists {
		return false // Replay attack
	}

	// Add nonce with current timestamp
	nt.usedNonces[nonce] = time.Now()
	return true
}

// cleanupNonces removes expired nonces
func (sm *SecurityManager) cleanupNonces() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C
		sm.nonceTracker.cleanup()
	}
}

// cleanup removes expired nonces from the tracker
func (nt *NonceTracker) cleanup() {
	nt.mutex.Lock()
	defer nt.mutex.Unlock()

	now := time.Now()
	for nonce, timestamp := range nt.usedNonces {
		if now.Sub(timestamp) > nt.maxAge {
			delete(nt.usedNonces, nonce)
		}
	}
}

// GenerateQRPairingCode generates a QR code payload for device pairing
func (sm *SecurityManager) GenerateQRPairingCode(deviceName string) (string, error) {
	// Create a pairing payload with device info and public key
	pairingData := map[string]string{
		"device_id":   sm.deviceID,
		"device_name": deviceName,
		"public_key":  sm.GetPublicKey(),
		"network_key": sm.GetNetworkKey(),
		"timestamp":   fmt.Sprintf("%d", time.Now().Unix()),
	}

	// Create HMAC for integrity
	h := hmac.New(sha256.New, sm.networkKey)
	payload, _ := json.Marshal(pairingData)
	h.Write(payload)
	pairingData["hmac"] = base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Encode as base64 JSON for QR code
	finalPayload, err := json.Marshal(pairingData)
	if err != nil {
		return "", fmt.Errorf("failed to create pairing payload: %v", err)
	}

	return base64.StdEncoding.EncodeToString(finalPayload), nil
}

// ParseQRPairingCode parses a QR pairing code and extracts device info
func (sm *SecurityManager) ParseQRPairingCode(qrCode string) (map[string]string, error) {
	// Decode base64
	payload, err := base64.StdEncoding.DecodeString(qrCode)
	if err != nil {
		return nil, fmt.Errorf("invalid QR code format: %v", err)
	}

	// Parse JSON
	var pairingData map[string]string
	if err := json.Unmarshal(payload, &pairingData); err != nil {
		return nil, fmt.Errorf("invalid pairing data: %v", err)
	}

	// Verify required fields
	required := []string{"device_id", "device_name", "public_key", "network_key", "timestamp", "hmac"}
	for _, field := range required {
		if _, exists := pairingData[field]; !exists {
			return nil, fmt.Errorf("missing required field: %s", field)
		}
	}

	return pairingData, nil
}
