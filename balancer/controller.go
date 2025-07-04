/*
Package balancer provides load balancing functionality for AMF endpoints.
*/
package balancer

import (
	"fmt"
	"sync"

	"github.com/hasukiHT/5glos/amf"
	"github.com/hasukiHT/5glos/utils"
)

// Balancer interface defines load balancing methods
type Balancer interface {
	// AddEndpoint adds a new AMF endpoint
	AddEndpoint(endpoint *amf.AMFEndpoint)
	// RemoveEndpoint removes an AMF endpoint by ID
	RemoveEndpoint(id string)
	// UpdateEndpoint updates an existing AMF endpoint
	UpdateEndpoint(endpoint *amf.AMFEndpoint)
	// SelectEndpoint selects an AMF endpoint for a new session
	SelectEndpoint(gnbID, ueID string) (*amf.AMFEndpoint, error)
	// GetEndpointForSession gets the AMF endpoint for an existing session
	GetEndpointForSession(gnbID, ueID string) (*amf.AMFEndpoint, error)
	// BindSession binds a UE session to an AMF endpoint
	BindSession(gnbID, ueID string, endpoint *amf.AMFEndpoint)
	// UnbindSession unbinds a UE session
	UnbindSession(gnbID, ueID string)
	// GetHealthyEndpoints returns all healthy endpoints
	GetHealthyEndpoints() []*amf.AMFEndpoint
}

// RoundRobinBalancer implements round-robin load balancing
type RoundRobinBalancer struct {
	mu        sync.RWMutex
	endpoints map[string]*amf.AMFEndpoint
	index     int
	sessions  map[string]*amf.AMFEndpoint // key: "gnbID:ueID"
}

// NewRoundRobinBalancer creates a new round-robin balancer
func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{
		endpoints: make(map[string]*amf.AMFEndpoint),
		sessions:  make(map[string]*amf.AMFEndpoint),
		index:     0,
	}
}

// AddEndpoint adds a new AMF endpoint
func (b *RoundRobinBalancer) AddEndpoint(endpoint *amf.AMFEndpoint) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.endpoints[endpoint.ID] = endpoint.Copy()
	utils.LogInfo("Added AMF endpoint to balancer", map[string]interface{}{
		"id":      endpoint.ID,
		"address": endpoint.Address(),
		"total":   len(b.endpoints),
	})
}

// RemoveEndpoint removes an AMF endpoint by ID
func (b *RoundRobinBalancer) RemoveEndpoint(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.endpoints, id)

	// Remove sessions bound to this endpoint
	var sessionsToRemove []string
	for sessionKey, endpoint := range b.sessions {
		if endpoint.ID == id {
			sessionsToRemove = append(sessionsToRemove, sessionKey)
		}
	}

	for _, sessionKey := range sessionsToRemove {
		delete(b.sessions, sessionKey)
	}

	utils.LogInfo("Removed AMF endpoint from balancer", map[string]interface{}{
		"id":               id,
		"remaining":        len(b.endpoints),
		"sessions_removed": len(sessionsToRemove),
	})
}

// UpdateEndpoint updates an existing AMF endpoint
func (b *RoundRobinBalancer) UpdateEndpoint(endpoint *amf.AMFEndpoint) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.endpoints[endpoint.ID]; exists {
		b.endpoints[endpoint.ID] = endpoint.Copy()
		utils.LogDebug("Updated AMF endpoint", map[string]interface{}{
			"id":     endpoint.ID,
			"status": endpoint.Status,
		})
	}
}

// SelectEndpoint selects an AMF endpoint for a new session
func (b *RoundRobinBalancer) SelectEndpoint(gnbID, ueID string) (*amf.AMFEndpoint, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sessionKey := fmt.Sprintf("%s:%s", gnbID, ueID)

	// Check if session already exists
	if endpoint, exists := b.sessions[sessionKey]; exists {
		if endpoint.IsReady() {
			return endpoint, nil
		}
		// Remove stale session
		delete(b.sessions, sessionKey)
	}

	// Get healthy endpoints
	healthyEndpoints := b.getHealthyEndpointsLocked()
	if len(healthyEndpoints) == 0 {
		return nil, fmt.Errorf("no healthy AMF endpoints available")
	}

	// Round-robin selection
	selectedEndpoint := healthyEndpoints[b.index%len(healthyEndpoints)]
	b.index++

	// Bind the session
	b.sessions[sessionKey] = selectedEndpoint

	utils.LogInfo("Selected AMF endpoint for session", map[string]interface{}{
		"gnb_id":   gnbID,
		"ue_id":    ueID,
		"amf_id":   selectedEndpoint.ID,
		"amf_addr": selectedEndpoint.Address(),
	})

	return selectedEndpoint, nil
}

// GetEndpointForSession gets the AMF endpoint for an existing session
func (b *RoundRobinBalancer) GetEndpointForSession(gnbID, ueID string) (*amf.AMFEndpoint, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sessionKey := fmt.Sprintf("%s:%s", gnbID, ueID)
	endpoint, exists := b.sessions[sessionKey]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionKey)
	}

	if !endpoint.IsReady() {
		return nil, fmt.Errorf("AMF endpoint not ready: %s", endpoint.ID)
	}

	return endpoint, nil
}

// BindSession binds a UE session to an AMF endpoint
func (b *RoundRobinBalancer) BindSession(gnbID, ueID string, endpoint *amf.AMFEndpoint) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sessionKey := fmt.Sprintf("%s:%s", gnbID, ueID)
	b.sessions[sessionKey] = endpoint

	utils.LogDebug("Bound session to AMF", map[string]interface{}{
		"gnb_id": gnbID,
		"ue_id":  ueID,
		"amf_id": endpoint.ID,
	})
}

// UnbindSession unbinds a UE session
func (b *RoundRobinBalancer) UnbindSession(gnbID, ueID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sessionKey := fmt.Sprintf("%s:%s", gnbID, ueID)
	delete(b.sessions, sessionKey)

	utils.LogDebug("Unbound session", map[string]interface{}{
		"gnb_id": gnbID,
		"ue_id":  ueID,
	})
}

// GetHealthyEndpoints returns all healthy endpoints
func (b *RoundRobinBalancer) GetHealthyEndpoints() []*amf.AMFEndpoint {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.getHealthyEndpointsLocked()
}

// getHealthyEndpointsLocked returns healthy endpoints (must be called with lock held)
func (b *RoundRobinBalancer) getHealthyEndpointsLocked() []*amf.AMFEndpoint {
	var healthy []*amf.AMFEndpoint
	for _, endpoint := range b.endpoints {
		if endpoint.IsReady() {
			healthy = append(healthy, endpoint)
		}
	}
	return healthy
}
