package gnb

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/hasukiHT/5glos/internal/ue"
)

type Manager struct {
	gnbList     map[net.Conn]*Gnb
	gnbSessions map[string]*GnBSession // Key: gNB ID
	mutex       sync.RWMutex
}

type GnBSession struct {
	gnb          *Gnb
	lastActivity time.Time
	messageCount int64
	errorCount   int64
	established  bool
}

func NewManager() *Manager {
	return &Manager{
		gnbList:     make(map[net.Conn]*Gnb),
		gnbSessions: make(map[string]*GnBSession),
	}
}

// Add a GnB
func (m *Manager) Add(gnb *Gnb) {
	m.mutex.Lock()

	conn := gnb.GetConn()
	_, exists := m.gnbList[conn]

	if !exists {
		m.gnbList[conn] = gnb
		incrementActiveGnBConnections()
	}

	active := len(m.gnbList)
	m.mutex.Unlock()

	log.Printf(
		"[INFO] Added GnB to manager; active=%d",
		active,
	)
}

func (m *Manager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, gnb := range m.gnbList {
		gnb.Close()
	}
}

// Remove deletes a disconnected gNB from all manager indexes.
// This prevents delayed AMF responses from using a closed connection.
func (m *Manager) Remove(conn net.Conn) {
	m.mutex.Lock()

	gnbInstance, exists := m.gnbList[conn]
	if exists {
		delete(m.gnbList, conn)
		delete(m.gnbSessions, gnbInstance.GetId())
	}

	m.mutex.Unlock()

	if !exists {
		return
	}

	gnbInstance.Close()

	decrementActiveGnBConnections()

	log.Printf(
		"[INFO] Removed disconnected GnB from manager: %s; active=%d",
		gnbInstance.GetId(),
		ActiveGnBConnectionCount(),
	)
}

// Implement interface method for AMF
func (m *Manager) GetGnBList() map[interface{}]interface{} {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	result := make(map[interface{}]interface{})
	for conn, gnb := range m.gnbList {
		result[conn] = gnb
	}
	return result
}

// Find GnB by UE context
func (m *Manager) FindGnBByUeContext(ueCtx *ue.Context) interface{} {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Iterate through all GnBs to find the one containing this UE
	for _, gnb := range m.gnbList {
		if gnb.HasUeContext(ueCtx) {
			return gnb
		}
	}

	log.Printf("[WARN] GnB not found for UE context with LB ID: %d", ueCtx.GetLbId())
	return nil
}

func (m *Manager) trackGnBActivity(gnbId string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if session, exists := m.gnbSessions[gnbId]; exists {
		session.lastActivity = time.Now()
		session.messageCount++
	}
}

func (m *Manager) cleanupInactiveSessions() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	timeout := 5 * time.Minute
	now := time.Now()

	for id, session := range m.gnbSessions {
		if now.Sub(session.lastActivity) > timeout {
			log.Printf("[INFO] Cleaning up inactive gNB session: %s", id)
			session.gnb.Close()
			delete(m.gnbSessions, id)
		}
	}
}
