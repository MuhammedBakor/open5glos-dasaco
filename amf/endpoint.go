/*
Package amf provides AMF endpoint management functionality.
*/
package amf

import (
	"fmt"
	"time"
)

// Status represents the health status of an AMF endpoint
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusUnknown   Status = "unknown"
)

// AMFEndpoint represents an AMF instance endpoint
type AMFEndpoint struct {
	ID       string    `json:"id"`
	IP       string    `json:"ip"`
	Port     int       `json:"port"`
	LastSeen time.Time `json:"last_seen"`
	Status   Status    `json:"status"`
}

// IsReady checks if the AMF endpoint is ready to accept connections
func (e *AMFEndpoint) IsReady() bool {
	return e.Status == StatusHealthy && time.Since(e.LastSeen) < 30*time.Second
}

// UpdateLastSeen updates the last seen timestamp
func (e *AMFEndpoint) UpdateLastSeen() {
	e.LastSeen = time.Now()
}

// UpdateStatus updates the endpoint status
func (e *AMFEndpoint) UpdateStatus(status Status) {
	e.Status = status
	e.UpdateLastSeen()
}

// Address returns the full address string
func (e *AMFEndpoint) Address() string {
	return fmt.Sprintf("%s:%d", e.IP, e.Port)
}

// Copy creates a copy of the endpoint
func (e *AMFEndpoint) Copy() *AMFEndpoint {
	return &AMFEndpoint{
		ID:       e.ID,
		IP:       e.IP,
		Port:     e.Port,
		LastSeen: e.LastSeen,
		Status:   e.Status,
	}
}
