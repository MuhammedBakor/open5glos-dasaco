/*
Package gnbue manages gNB-UE sessions and connections.
*/
package gnbue

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/free5gc/sctp"
	"github.com/hasukiHT/5glos/amf"
	"github.com/hasukiHT/5glos/utils"
)

// Session represents a gNB-UE session
type Session struct {
	ID          string
	GnbID       string
	UeID        string
	GnbAddr     net.Addr
	AMFEndpoint *amf.AMFEndpoint
	GnbConn     *sctp.SCTPConn
	AMFConn     *sctp.SCTPConn
	CreatedAt   time.Time
	LastActive  time.Time
	mu          sync.RWMutex
}

// SessionManager manages all active sessions
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new session
func (sm *SessionManager) CreateSession(gnbID, ueID string, gnbAddr net.Addr, amfEndpoint *amf.AMFEndpoint) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sessionID := fmt.Sprintf("%s:%s", gnbID, ueID)

	session := &Session{
		ID:          sessionID,
		GnbID:       gnbID,
		UeID:        ueID,
		GnbAddr:     gnbAddr,
		AMFEndpoint: amfEndpoint,
		CreatedAt:   time.Now(),
		LastActive:  time.Now(),
	}

	sm.sessions[sessionID] = session

	utils.LogInfo("Created new session", map[string]interface{}{
		"session_id": sessionID,
		"gnb_addr":   gnbAddr.String(),
		"amf_id":     amfEndpoint.ID,
		"amf_addr":   amfEndpoint.Address(),
	})

	return session
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	return session, exists
}

// GetSessionByGnbUe retrieves a session by gNB and UE IDs
func (sm *SessionManager) GetSessionByGnbUe(gnbID, ueID string) (*Session, bool) {
	sessionID := fmt.Sprintf("%s:%s", gnbID, ueID)
	return sm.GetSession(sessionID)
}

// RemoveSession removes a session
func (sm *SessionManager) RemoveSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		session.Close()
		delete(sm.sessions, sessionID)

		utils.LogInfo("Removed session", map[string]interface{}{
			"session_id": sessionID,
		})
	}
}

// CleanupStale removes stale sessions
func (sm *SessionManager) CleanupStale(maxAge time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	var toRemove []string

	for sessionID, session := range sm.sessions {
		if now.Sub(session.LastActive) > maxAge {
			toRemove = append(toRemove, sessionID)
		}
	}

	for _, sessionID := range toRemove {
		if session, exists := sm.sessions[sessionID]; exists {
			session.Close()
			delete(sm.sessions, sessionID)

			utils.LogInfo("Cleaned up stale session", map[string]interface{}{
				"session_id": sessionID,
				"age":        now.Sub(session.LastActive).String(),
			})
		}
	}
}

// CleanupAll closes and removes all sessions
func (sm *SessionManager) CleanupAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for sessionID, session := range sm.sessions {
		session.Close()
		delete(sm.sessions, sessionID)
	}

	utils.LogInfo("Cleaned up all sessions")
}

// GetActiveSessions returns the number of active sessions
func (sm *SessionManager) GetActiveSessions() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// UpdateLastActive updates the last active time for a session
func (s *Session) UpdateLastActive() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActive = time.Now()
}

// SetConnections sets the SCTP connections for the session
func (s *Session) SetConnections(gnbConn, amfConn *sctp.SCTPConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.GnbConn = gnbConn
	s.AMFConn = amfConn
}

// Close closes all connections in the session
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.GnbConn != nil {
		s.GnbConn.Close()
		s.GnbConn = nil
	}

	if s.AMFConn != nil {
		s.AMFConn.Close()
		s.AMFConn = nil
	}
}

// IsActive checks if the session has active connections
func (s *Session) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.GnbConn != nil && s.AMFConn != nil
}
