package utils

import (
	"fmt"
	"net"
	"time"
)

// CheckTCPConnectivity checks if a TCP connection can be established
func CheckTCPConnectivity(address string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", address, err)
	}
	defer conn.Close()
	return nil
}

// GetLocalIP returns the local IP address
func GetLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// IsValidPort checks if a port number is valid
func IsValidPort(port int) bool {
	return port > 0 && port <= 65535
}

// ParseAddress parses an address string into host and port
func ParseAddress(address string) (host string, port string, err error) {
	host, port, err = net.SplitHostPort(address)
	if err != nil {
		return "", "", fmt.Errorf("invalid address format: %w", err)
	}
	return host, port, nil
}
