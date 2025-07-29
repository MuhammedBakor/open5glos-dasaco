package gnb

import (
	"log"
	"net"
	"sync"

	"github.com/hasukiHT/5glos/internal/ue"
)

type Manager struct {
	gnbList map[net.Conn]*Gnb
	mutex   sync.Mutex
}

func NewManager() *Manager {
	return &Manager{
		gnbList: make(map[net.Conn]*Gnb),
	}
}

// Add a GnB
func (m *Manager) Add(gnb *Gnb) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.gnbList[gnb.GetConn()] = gnb
	log.Printf("[INFO] Added GnB to manager")
}

func (m *Manager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, gnb := range m.gnbList {
		gnb.Close()
	}
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
