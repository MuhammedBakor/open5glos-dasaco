package service

import (
	"net"
	"sync"

	"github.com/hasukiHT/5glos/internal/amf"
	"github.com/hasukiHT/5glos/internal/gnb"
	ngapconn "github.com/hasukiHT/5glos/internal/ngap"
	"github.com/hasukiHT/5glos/internal/ue"
)

// Adapter to make AMF manager compatible with GnB interface
type AMFManagerAdapter struct {
	manager *amf.Manager
}

func (a *AMFManagerAdapter) Pick() gnb.AMFInterface {
	amfInstance := a.manager.Pick()
	if amfInstance == nil {
		return nil
	}
	return &AMFAdapter{amf: amfInstance}
}

// Adapter to make AMF compatible with GnB interface
type AMFAdapter struct {
	amf *amf.Amf
}

func (a *AMFAdapter) SendNgap(pdu []byte) error {
	return a.amf.SendNgap(pdu)
}

func (a *AMFAdapter) GetId() string {
	return a.amf.GetId()
}

// Adapter to make GnB manager compatible with AMF interface
type GnBManagerAdapter struct {
	manager *gnb.Manager
}

func (a *GnBManagerAdapter) GetGnBList() map[interface{}]amf.GnbInterface {
	gnbList := a.manager.GetGnBList()
	result := make(map[interface{}]amf.GnbInterface)

	for conn, gnbInstance := range gnbList {
		if gnb, ok := gnbInstance.(*gnb.Gnb); ok {
			result[conn] = &GnBAdapter{gnb: gnb}
		}
	}
	return result
}

func (a *GnBManagerAdapter) FindGnBByUeContext(ueCtx *ue.Context) amf.GnbInterface {
	gnbInstance := a.manager.FindGnBByUeContext(ueCtx)
	if gnb, ok := gnbInstance.(*gnb.Gnb); ok {
		return &GnBAdapter{gnb: gnb}
	}
	return nil
}

// Adapter to make GnB compatible with AMF interface
type GnBAdapter struct {
	gnb *gnb.Gnb
}

func (a *GnBAdapter) SendNgap(pdu []byte) error {
	return a.gnb.SendNgap(pdu)
}

func (a *GnBAdapter) GetConn() interface{} {
	return a.gnb.GetConn()
}

func (a *GnBAdapter) GetId() string {
	return a.gnb.GetId()
}

func (a *GnBAdapter) GetName() string {
	return a.gnb.GetName()
}

type Service struct {
	amfMan  *amf.Manager
	gnbMan  *gnb.Manager
	sctpSrv *ngapconn.NgapServer
	wg      sync.WaitGroup
	lbUeId  int64
	ueList  map[int64]*ue.Context
	mutex   sync.Mutex
}

func New() *Service {
	return &Service{
		sctpSrv: ngapconn.NewNgapServer(),
		amfMan:  amf.NewManager(),
		gnbMan:  gnb.NewManager(),
		ueList:  make(map[int64]*ue.Context),
	}
}

func (s *Service) Start() error {
	s.wg.Add(2)

	// Inject dependencies into AMF manager using adapters
	gnbManagerAdapter := &GnBManagerAdapter{manager: s.gnbMan}
	s.amfMan.SetGnbManager(gnbManagerAdapter)
	s.amfMan.SetService(s)

	// Start SCTP server
	go s.sctpSrv.ListenLoop(&s.wg, s.onGnBConnection)

	// Start AMF monitoring
	go s.amfMan.MonitorLoop(&s.wg)

	return nil
}

func (s *Service) onGnBConnection(conn net.Conn) {
	// Create GnB and start handling
	gnbInstance := gnb.Create(conn, &s.wg)

	// Inject dependencies using adapter
	amfManagerAdapter := &AMFManagerAdapter{manager: s.amfMan}
	gnbInstance.SetAMFManager(amfManagerAdapter)
	gnbInstance.SetService(s)

	s.gnbMan.Add(gnbInstance)
}

// Create new UeContext, allocate LbUeId, then add to the UeContext list
func (s *Service) CreateUeContext(amfPtr interface{}, gnbPtr interface{}, ranUeId int64) *ue.Context {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	ueCtx := ue.New(amfPtr, gnbPtr, ranUeId, s.lbUeId)

	// Add to global UeContext list
	s.ueList[ueCtx.GetLbId()] = ueCtx

	// Add to UeContext list at GnB (need type assertion)
	if gnb, ok := gnbPtr.(*gnb.Gnb); ok {
		gnb.Add(ueCtx)
	}

	// Increment counter
	s.lbUeId++

	return ueCtx
}

// Find UeContext given LbUeId
func (s *Service) FindUeCtx(lbUeId int64) *ue.Context {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	ueCtx, _ := s.ueList[lbUeId]
	return ueCtx
}

// Find UeContext by RAN UE NGAP ID for a specific GnB
func (s *Service) FindUeCtxByRanUeId(gnbInterface interface{}, ranUeNgapId int64) *ue.Context {
	gnbInstance, ok := gnbInterface.(*gnb.Gnb)
	if !ok {
		return nil
	}

	gnbInstance.GetMutex().Lock()
	defer gnbInstance.GetMutex().Unlock()

	ueCtx, exists := gnbInstance.GetUeList()[ranUeNgapId]
	if !exists {
		return nil
	}

	return ueCtx
}

func (s *Service) GetAmfManager() *amf.Manager {
	return s.amfMan
}

func (s *Service) GetGnbManager() *gnb.Manager {
	return s.gnbMan
}

// Stop all goroutines and release resources
func (s *Service) Stop() {
	s.sctpSrv.Close()
	s.amfMan.Close()
	s.gnbMan.Close()

	s.wg.Wait()
}
