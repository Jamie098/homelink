// HomeLink - Network Helper Functions
// Copyright (c) 2025 - Open Source Project

package homelink

import (
	"fmt"
	"net"
	"time"
)

// NetworkHelper provides network utility functions
type NetworkHelper struct {
	errorHandler *ErrorHandler
}

// NewNetworkHelper creates a new network helper
func NewNetworkHelper(errorHandler *ErrorHandler) *NetworkHelper {
	return &NetworkHelper{
		errorHandler: errorHandler,
	}
}

// CreateBroadcastAddress creates a broadcast address for the given port
func (nh *NetworkHelper) CreateBroadcastAddress(port int) *net.UDPAddr {
	return &net.UDPAddr{
		IP:   net.ParseIP(BROADCAST_ADDR),
		Port: port,
	}
}

// CreateBroadcastConnection creates a UDP connection for broadcasting
func (nh *NetworkHelper) CreateBroadcastConnection(port int) (*net.UDPConn, error) {
	broadcastAddr := nh.CreateBroadcastAddress(port)
	conn, err := net.DialUDP("udp4", nil, broadcastAddr)
	if err != nil {
		return nil, nh.errorHandler.HandleNetworkError("create broadcast connection", err)
	}
	return conn, nil
}

// CreateListener creates a UDP listener on the specified port
func (nh *NetworkHelper) CreateListener(port int) (*net.UDPConn, error) {
	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: port,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, nh.errorHandler.HandleNetworkError("create listener", err)
	}
	return conn, nil
}

// CreateUnicastListener creates a listener on any available port
func (nh *NetworkHelper) CreateUnicastListener() (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return nil, nh.errorHandler.HandleNetworkError("resolve address", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, nh.errorHandler.HandleNetworkError("create unicast listener", err)
	}
	return conn, nil
}

// SendWithRetry sends data with retry logic
func (nh *NetworkHelper) SendWithRetry(conn *net.UDPConn, data []byte, maxRetries int, retryDelay time.Duration) error {
	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		_, err := conn.Write(data)
		if err == nil {
			return nil
		}
		lastErr = err

		if i < maxRetries {
			nh.errorHandler.LogWarning("send retry", fmt.Sprintf("Send attempt %d failed, retrying in %v", i+1, retryDelay))
			time.Sleep(retryDelay)
		}
	}
	return nh.errorHandler.HandleNetworkError("send with retry", lastErr)
}

// ReadWithTimeout reads from a connection with timeout
func (nh *NetworkHelper) ReadWithTimeout(conn *net.UDPConn, buffer []byte, timeout time.Duration) (int, *net.UDPAddr, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return 0, nil, nh.errorHandler.HandleNetworkError("set read deadline", err)
	}

	n, addr, err := conn.ReadFromUDP(buffer)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return 0, nil, nil // Timeout is normal, return without error
		}
		return 0, nil, nh.errorHandler.HandleNetworkError("read with timeout", err)
	}
	return n, addr, nil
}

// IsValidUDPAddress validates if an address is a valid UDP address
func (nh *NetworkHelper) IsValidUDPAddress(addr *net.UDPAddr) bool {
	if addr == nil {
		return false
	}
	if addr.IP == nil {
		return false
	}
	if addr.Port <= 0 || addr.Port > 65535 {
		return false
	}
	return true
}

// GetLocalIPAddresses returns all local IP addresses
func (nh *NetworkHelper) GetLocalIPAddresses() ([]net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, nh.errorHandler.HandleNetworkError("get local IP addresses", err)
	}

	var ips []net.IP
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP)
			}
		}
	}
	return ips, nil
}

// IsLocalAddress checks if an address is local to this machine
func (nh *NetworkHelper) IsLocalAddress(addr *net.UDPAddr) bool {
	if addr == nil {
		return false
	}

	localIPs, err := nh.GetLocalIPAddresses()
	if err != nil {
		nh.errorHandler.LogWarning("address check", "Failed to get local IPs")
		return false
	}

	for _, localIP := range localIPs {
		if localIP.Equal(addr.IP) {
			return true
		}
	}
	return false
}

// CloseConnection safely closes a UDP connection
func (nh *NetworkHelper) CloseConnection(conn *net.UDPConn, name string) {
	if conn != nil {
		if err := conn.Close(); err != nil {
			nh.errorHandler.LogWarning("connection close", fmt.Sprintf("Failed to close %s connection: %v", name, err))
		} else {
			nh.errorHandler.LogInfo("connection close", fmt.Sprintf("Closed %s connection", name))
		}
	}
}
