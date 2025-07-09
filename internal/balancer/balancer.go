package balancer

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/hasukiHT/5glos/internal/config"
	"github.com/hasukiHT/5glos/internal/metrics"
)

// AMFInstance represents an AMF instance
type AMFInstance struct {
	ID              string
	Address         string
	Port            int
	Healthy         bool
	Connections     int
	LastHealthCheck time.Time
	Weight          int
	mu              sync.RWMutex
}

// GetAddress returns the full address of the AMF instance
func (amf *AMFInstance) GetAddress() string {
	return fmt.Sprintf("%s:%d", amf.Address, amf.Port)
}

// AddConnection increments the connection count
func (amf *AMFInstance) AddConnection() {
	amf.mu.Lock()
	defer amf.mu.Unlock()
	amf.Connections++
}

// RemoveConnection decrements the connection count
func (amf *AMFInstance) RemoveConnection() {
	amf.mu.Lock()
	defer amf.mu.Unlock()
	if amf.Connections > 0 {
		amf.Connections--
	}
}

// GetConnections returns the current connection count
func (amf *AMFInstance) GetConnections() int {
	amf.mu.RLock()
	defer amf.mu.RUnlock()
	return amf.Connections
}

// SetHealthy sets the health status
func (amf *AMFInstance) SetHealthy(healthy bool) {
	amf.mu.Lock()
	defer amf.mu.Unlock()
	amf.Healthy = healthy
	amf.LastHealthCheck = time.Now()
}

// IsHealthy returns the health status
func (amf *AMFInstance) IsHealthy() bool {
	amf.mu.RLock()
	defer amf.mu.RUnlock()
	return amf.Healthy
}

// LoadBalancer interface for different balancing strategies
type LoadBalancer interface {
	AddAMF(amf *AMFInstance)
	RemoveAMF(id string)
	UpdateAMF(amf *AMFInstance)
	SelectAMF(ueID string) (*AMFInstance, error)
	GetAMF(id string) (*AMFInstance, bool)
	GetAllAMFs() []*AMFInstance
	GetHealthyAMFs() []*AMFInstance
	SetUEAffinity(ueID string, amfID string)
	GetUEAffinity(ueID string) (string, bool)
	RemoveUEAffinity(ueID string)
	GetStats() BalancerStats
}

// BalancerStats contains load balancer statistics
type BalancerStats struct {
	TotalAMFs     int
	HealthyAMFs   int
	TotalSessions int
	Strategy      string
}

// BaseBalancer provides common functionality for all balancer implementations
type BaseBalancer struct {
	amfs     map[string]*AMFInstance
	sessions map[string]string // ueID -> amfID mapping
	strategy string
	metrics  *metrics.Registry
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewBaseBalancer creates a new base balancer
func NewBaseBalancer(strategy string, metrics *metrics.Registry, logger *zap.Logger) *BaseBalancer {
	return &BaseBalancer{
		amfs:     make(map[string]*AMFInstance),
		sessions: make(map[string]string),
		strategy: strategy,
		metrics:  metrics,
		logger:   logger,
	}
}

// AddAMF adds an AMF instance
func (b *BaseBalancer) AddAMF(amf *AMFInstance) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.amfs[amf.ID] = amf
	b.updateMetrics()

	b.logger.Info("Added AMF instance",
		zap.String("amf_id", amf.ID),
		zap.String("address", amf.GetAddress()),
	)
}

// RemoveAMF removes an AMF instance
func (b *BaseBalancer) RemoveAMF(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if amf, exists := b.amfs[id]; exists {
		delete(b.amfs, id)

		// Remove sessions associated with this AMF
		for ueID, amfID := range b.sessions {
			if amfID == id {
				delete(b.sessions, ueID)
			}
		}

		b.updateMetrics()

		b.logger.Info("Removed AMF instance",
			zap.String("amf_id", id),
			zap.String("address", amf.GetAddress()),
		)
	}
}

// UpdateAMF updates an AMF instance
func (b *BaseBalancer) UpdateAMF(amf *AMFInstance) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, exists := b.amfs[amf.ID]; exists {
		existing.Address = amf.Address
		existing.Port = amf.Port
		existing.Healthy = amf.Healthy
		existing.Weight = amf.Weight

		b.updateMetrics()

		b.logger.Debug("Updated AMF instance",
			zap.String("amf_id", amf.ID),
			zap.String("address", amf.GetAddress()),
			zap.Bool("healthy", amf.Healthy),
		)
	}
}

// GetAMF retrieves an AMF instance by ID
func (b *BaseBalancer) GetAMF(id string) (*AMFInstance, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	amf, exists := b.amfs[id]
	return amf, exists
}

// GetAllAMFs returns all AMF instances
func (b *BaseBalancer) GetAllAMFs() []*AMFInstance {
	b.mu.RLock()
	defer b.mu.RUnlock()

	amfs := make([]*AMFInstance, 0, len(b.amfs))
	for _, amf := range b.amfs {
		amfs = append(amfs, amf)
	}
	return amfs
}

// GetHealthyAMFs returns only healthy AMF instances
func (b *BaseBalancer) GetHealthyAMFs() []*AMFInstance {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var healthy []*AMFInstance
	for _, amf := range b.amfs {
		if amf.IsHealthy() {
			healthy = append(healthy, amf)
		}
	}
	return healthy
}

// SetUEAffinity sets UE to AMF affinity
func (b *BaseBalancer) SetUEAffinity(ueID string, amfID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.sessions[ueID] = amfID
	b.updateMetrics()

	b.logger.Debug("Set UE affinity",
		zap.String("ue_id", ueID),
		zap.String("amf_id", amfID),
	)
}

// GetUEAffinity gets UE to AMF affinity
func (b *BaseBalancer) GetUEAffinity(ueID string) (string, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	amfID, exists := b.sessions[ueID]
	return amfID, exists
}

// RemoveUEAffinity removes UE to AMF affinity
func (b *BaseBalancer) RemoveUEAffinity(ueID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.sessions[ueID]; exists {
		delete(b.sessions, ueID)
		b.updateMetrics()

		b.logger.Debug("Removed UE affinity",
			zap.String("ue_id", ueID),
		)
	}
}

// GetStats returns balancer statistics
func (b *BaseBalancer) GetStats() BalancerStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	healthyCount := 0
	for _, amf := range b.amfs {
		if amf.IsHealthy() {
			healthyCount++
		}
	}

	return BalancerStats{
		TotalAMFs:     len(b.amfs),
		HealthyAMFs:   healthyCount,
		TotalSessions: len(b.sessions),
		Strategy:      b.strategy,
	}
}

// updateMetrics updates Prometheus metrics
func (b *BaseBalancer) updateMetrics() {
	if b.metrics == nil {
		return
	}

	healthyCount := 0
	for _, amf := range b.amfs {
		if amf.IsHealthy() {
			healthyCount++
		}

		// Update per-AMF metrics
		b.metrics.UpdateAMFConnections(amf.ID, amf.GetAddress(), amf.GetConnections())
		b.metrics.UpdateAMFHealth(amf.ID, amf.GetAddress(), amf.IsHealthy())
	}

	b.metrics.UpdateAMFInstances(len(b.amfs))
}

// RoundRobinBalancer implements round-robin load balancing
type RoundRobinBalancer struct {
	*BaseBalancer
	index int
}

// NewRoundRobinBalancer creates a new round-robin balancer
func NewRoundRobinBalancer(metrics *metrics.Registry, logger *zap.Logger) *RoundRobinBalancer {
	return &RoundRobinBalancer{
		BaseBalancer: NewBaseBalancer("round-robin", metrics, logger),
		index:        0,
	}
}

// SelectAMF selects an AMF using round-robin strategy
func (b *RoundRobinBalancer) SelectAMF(ueID string) (*AMFInstance, error) {
	// Check if UE already has affinity
	if amfID, exists := b.GetUEAffinity(ueID); exists {
		if amf, exists := b.GetAMF(amfID); exists && amf.IsHealthy() {
			b.metrics.RecordSessionAffinityHit()
			b.metrics.RecordLoadBalancerDecision("round-robin", "affinity-hit")
			return amf, nil
		} else {
			// Remove stale affinity
			b.RemoveUEAffinity(ueID)
			b.metrics.RecordSessionAffinityMiss()
		}
	}

	// Get healthy AMFs
	healthyAMFs := b.GetHealthyAMFs()
	if len(healthyAMFs) == 0 {
		b.metrics.RecordLoadBalancerDecision("round-robin", "no-healthy-amfs")
		return nil, fmt.Errorf("no healthy AMF instances available")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Round-robin selection
	selectedAMF := healthyAMFs[b.index%len(healthyAMFs)]
	b.index++

	// Set affinity
	b.sessions[ueID] = selectedAMF.ID

	b.metrics.RecordLoadBalancerDecision("round-robin", "new-session")

	b.logger.Debug("Selected AMF using round-robin",
		zap.String("ue_id", ueID),
		zap.String("amf_id", selectedAMF.ID),
		zap.String("amf_address", selectedAMF.GetAddress()),
	)

	return selectedAMF, nil
}

// LeastConnectionsBalancer implements least connections load balancing
type LeastConnectionsBalancer struct {
	*BaseBalancer
}

// NewLeastConnectionsBalancer creates a new least connections balancer
func NewLeastConnectionsBalancer(metrics *metrics.Registry, logger *zap.Logger) *LeastConnectionsBalancer {
	return &LeastConnectionsBalancer{
		BaseBalancer: NewBaseBalancer("least-connections", metrics, logger),
	}
}

// SelectAMF selects an AMF using least connections strategy
func (b *LeastConnectionsBalancer) SelectAMF(ueID string) (*AMFInstance, error) {
	// Check if UE already has affinity
	if amfID, exists := b.GetUEAffinity(ueID); exists {
		if amf, exists := b.GetAMF(amfID); exists && amf.IsHealthy() {
			b.metrics.RecordSessionAffinityHit()
			b.metrics.RecordLoadBalancerDecision("least-connections", "affinity-hit")
			return amf, nil
		} else {
			// Remove stale affinity
			b.RemoveUEAffinity(ueID)
			b.metrics.RecordSessionAffinityMiss()
		}
	}

	// Get healthy AMFs
	healthyAMFs := b.GetHealthyAMFs()
	if len(healthyAMFs) == 0 {
		b.metrics.RecordLoadBalancerDecision("least-connections", "no-healthy-amfs")
		return nil, fmt.Errorf("no healthy AMF instances available")
	}

	// Find AMF with least connections
	var selectedAMF *AMFInstance
	minConnections := int(^uint(0) >> 1) // Max int

	for _, amf := range healthyAMFs {
		connections := amf.GetConnections()
		if connections < minConnections {
			minConnections = connections
			selectedAMF = amf
		}
	}

	if selectedAMF == nil {
		b.metrics.RecordLoadBalancerDecision("least-connections", "selection-failed")
		return nil, fmt.Errorf("failed to select AMF")
	}

	// Set affinity
	b.SetUEAffinity(ueID, selectedAMF.ID)

	b.metrics.RecordLoadBalancerDecision("least-connections", "new-session")

	b.logger.Debug("Selected AMF using least connections",
		zap.String("ue_id", ueID),
		zap.String("amf_id", selectedAMF.ID),
		zap.String("amf_address", selectedAMF.GetAddress()),
		zap.Int("connections", minConnections),
	)

	return selectedAMF, nil
}

// NewLoadBalancer creates a new load balancer based on strategy
func NewLoadBalancer(cfg config.LoadBalancerConfig, metrics *metrics.Registry, logger *zap.Logger) (LoadBalancer, error) {
	switch cfg.Strategy {
	case "round-robin":
		return NewRoundRobinBalancer(metrics, logger), nil
	case "least-connections":
		return NewLeastConnectionsBalancer(metrics, logger), nil
	default:
		return nil, fmt.Errorf("unsupported load balancer strategy: %s", cfg.Strategy)
	}
}
