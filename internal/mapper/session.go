package mapper

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/hasukiHT/5glos/internal/balancer"
	"github.com/hasukiHT/5glos/internal/config"
	"github.com/hasukiHT/5glos/internal/metrics"
)

// SessionInfo contains information about a UE session
type SessionInfo struct {
	UEIF         string
	AMFID        string
	CreatedAt    time.Time
	LastActivity time.Time
	ConnectionID string
	RANUENGAPID  *int64
	AMFUENGAPID  *int64
}

// SessionMapper manages UE session mappings
type SessionMapper struct {
	sessions    map[string]*SessionInfo // ueID -> SessionInfo
	connections map[string]string       // connectionID -> ueID
	cfg         config.LoadBalancerConfig
	metrics     *metrics.Registry
	logger      *zap.Logger
	mu          sync.RWMutex
}

// NewSessionMapper creates a new session mapper
func NewSessionMapper(cfg config.LoadBalancerConfig, metrics *metrics.Registry, logger *zap.Logger) *SessionMapper {
	mapper := &SessionMapper{
		sessions:    make(map[string]*SessionInfo),
		connections: make(map[string]string),
		cfg:         cfg,
		metrics:     metrics,
		logger:      logger,
	}

	// Start cleanup routine
	go mapper.cleanupRoutine()

	return mapper
}

// CreateSession creates a new UE session
func (m *SessionMapper) CreateSession(ueID, amfID, connectionID string) *SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := &SessionInfo{
		UEIF:         ueID,
		AMFID:        amfID,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		ConnectionID: connectionID,
	}

	m.sessions[ueID] = session
	m.connections[connectionID] = ueID

	if m.metrics != nil {
		m.metrics.RecordUESession()
	}

	m.logger.Info("Created UE session",
		zap.String("ue_id", ueID),
		zap.String("amf_id", amfID),
		zap.String("connection_id", connectionID),
	)

	return session
}

// GetSession retrieves a session by UE ID
func (m *SessionMapper) GetSession(ueID string) (*SessionInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[ueID]
	return session, exists
}

// GetSessionByConnection retrieves a session by connection ID
func (m *SessionMapper) GetSessionByConnection(connectionID string) (*SessionInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if ueID, exists := m.connections[connectionID]; exists {
		if session, exists := m.sessions[ueID]; exists {
			return session, true
		}
	}
	return nil, false
}

// UpdateSessionActivity updates the last activity time for a session
func (m *SessionMapper) UpdateSessionActivity(ueID string, ranUENGAPID, amfUENGAPID *int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, exists := m.sessions[ueID]; exists {
		session.LastActivity = time.Now()
		if ranUENGAPID != nil {
			session.RANUENGAPID = ranUENGAPID
		}
		if amfUENGAPID != nil {
			session.AMFUENGAPID = amfUENGAPID
		}

		m.logger.Debug("Updated session activity",
			zap.String("ue_id", ueID),
			zap.Time("last_activity", session.LastActivity),
		)
	}
}

// RemoveSession removes a UE session
func (m *SessionMapper) RemoveSession(ueID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, exists := m.sessions[ueID]; exists {
		delete(m.sessions, ueID)
		delete(m.connections, session.ConnectionID)

		duration := time.Since(session.CreatedAt).Seconds()
		if m.metrics != nil {
			m.metrics.RecordUESessionClosed(duration)
		}

		m.logger.Info("Removed UE session",
			zap.String("ue_id", ueID),
			zap.String("amf_id", session.AMFID),
			zap.Duration("duration", time.Since(session.CreatedAt)),
		)
	}
}

// RemoveSessionByConnection removes a session by connection ID
func (m *SessionMapper) RemoveSessionByConnection(connectionID string) {
	m.mu.RLock()
	ueID, exists := m.connections[connectionID]
	m.mu.RUnlock()

	if exists {
		m.RemoveSession(ueID)
	}
}

// GetActiveSessions returns the number of active sessions
func (m *SessionMapper) GetActiveSessions() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.sessions)
}

// GetSessionsForAMF returns all sessions for a specific AMF
func (m *SessionMapper) GetSessionsForAMF(amfID string) []*SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sessions []*SessionInfo
	for _, session := range m.sessions {
		if session.AMFID == amfID {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

// MigrateSessions migrates sessions from one AMF to another
func (m *SessionMapper) MigrateSessions(fromAMF, toAMF string, lb balancer.LoadBalancer) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	migratedCount := 0
	for _, session := range m.sessions {
		if session.AMFID == fromAMF {
			session.AMFID = toAMF
			lb.SetUEAffinity(session.UEIF, toAMF)
			migratedCount++

			m.logger.Info("Migrated UE session",
				zap.String("ue_id", session.UEIF),
				zap.String("from_amf", fromAMF),
				zap.String("to_amf", toAMF),
			)
		}
	}

	m.logger.Info("Session migration completed",
		zap.String("from_amf", fromAMF),
		zap.String("to_amf", toAMF),
		zap.Int("migrated_count", migratedCount),
	)

	return migratedCount
}

// cleanupRoutine periodically removes stale sessions
func (m *SessionMapper) cleanupRoutine() {
	ticker := time.NewTicker(m.cfg.SessionTimeout / 4) // Cleanup every 1/4 of session timeout
	defer ticker.Stop()

	for range ticker.C {
		m.cleanupStaleSessions()
	}
}

// cleanupStaleSessions removes sessions that haven't been active
func (m *SessionMapper) cleanupStaleSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	var staleUEIDs []string

	for ueID, session := range m.sessions {
		if now.Sub(session.LastActivity) > m.cfg.SessionTimeout {
			staleUEIDs = append(staleUEIDs, ueID)
		}
	}

	// Remove stale sessions
	for _, ueID := range staleUEIDs {
		if session, exists := m.sessions[ueID]; exists {
			delete(m.sessions, ueID)
			delete(m.connections, session.ConnectionID)

			duration := time.Since(session.CreatedAt).Seconds()
			if m.metrics != nil {
				m.metrics.RecordUESessionClosed(duration)
			}

			m.logger.Info("Removed stale UE session",
				zap.String("ue_id", ueID),
				zap.String("amf_id", session.AMFID),
				zap.Duration("inactive_duration", now.Sub(session.LastActivity)),
			)
		}
	}

	if len(staleUEIDs) > 0 {
		m.logger.Info("Cleanup completed",
			zap.Int("removed_sessions", len(staleUEIDs)),
			zap.Int("active_sessions", len(m.sessions)),
		)
	}
}

// GetSessionStats returns session statistics
func (m *SessionMapper) GetSessionStats() SessionStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	amfDistribution := make(map[string]int)
	for _, session := range m.sessions {
		amfDistribution[session.AMFID]++
	}

	return SessionStats{
		TotalSessions:    len(m.sessions),
		AMFDistribution:  amfDistribution,
		TotalConnections: len(m.connections),
	}
}

// SessionStats contains session statistics
type SessionStats struct {
	TotalSessions    int
	AMFDistribution  map[string]int
	TotalConnections int
}

// String returns a string representation of session stats
func (s SessionStats) String() string {
	return fmt.Sprintf("Sessions: %d, Connections: %d, AMF Distribution: %v",
		s.TotalSessions, s.TotalConnections, s.AMFDistribution)
}
