package main

import (
	"etrib5gc/sctp"
	"net"
	"sync"
)

/////////////  NGAP
type NgapMessage interface{} //

//a SCTP server wrapper
type NgapServer struct {
	// srv sctp.Server
}

func (s *NgapServer) listenLoop(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		default:
			//accept new connection
			//conn, err := s.srv.SCTPAccept()
			gnb := createGnb(conn, wg)
		case <-s.done:
			return
		}
	}
}

// represent a sctp connection to GnB or AMF
type NgapConn struct {
	conn    sctp.SCTPConn
	buf     [4096]byte
	handler func(*NgapMessage) error //to handle decoded ngap message
	done    chan struct{}
}

func newNgapConn(conn sctp.SCTPConn, handler func(*NgapMessage) error) *NgapConn {
	return &NgapConn{
		conn:    conn,
		handler: handler,
		done:    make(chan struct{}, 1),
	}
}

//loop to read data from connection, decode then send to handler to handle
func (c *NgapConn) readLoop(wg *sync.WaitGroup) {
	defer wg.Done()
	var buf [4096]byte

	for {
		select {
		default: //Read from connection
			if err := gnb.conn.Read(buf); err != nil {
				//handle error or connection close
			} else {
				//Decode message
				if msg, err := ngap.Decode(buf); err != nil {
					//handle error
				} else {
					//handle Ngap message
					c.handler.handle(msg)
				}
			}
		case <-c.done:
			return
		}
	}

}

//send an Ngap PDU
func (c *NgapConn) sendNgap(pdu []byte) error {
	//call SCTP connection Write
	return c.conn.Write(pdu)
}

func (c *NgapConn) Close() {
	//close SCTP connection
	c.conn.Close()
	//send close signal
	c.done <- struct{}{}
}

////////////////// GNB
type GnbManager struct {
	gnbList map[net.Conn]*Gnb
	mutex   sync.Mutex
}

// add an GnB
func (m *GnbManager) add(gnb *Gnb) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.gnbList[gnb.conn] = gnb
}

func (m *GnbManager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, gnb := range m.gnbList {
		gnb.Close()
	}
}

type Gnb struct {
	conn   *NgapConn            //keep for writing Ngap message
	ueList map[int64]*UeContext //index by gnbUeId
	mutex  sync.Mutex           //protect conccurency read/write to ueList
}

func newGnb() *Gnb {
	return &Gnb{
		ueList: make(map[int64]*UeContext), //Note: map must be alloated, otherwise accesing it will panic
	}
}

// an UeContext for GnB
func (gnb *Gnb) add(ueCtx *UeContext) {
	gnb.mutex.Lock()
	defer gnb.mutex.Unlock()
	gnb.ueList[ueCtx.gnbUeId] = ueCtx
}

//create a gnb and start a go routine to read data from its connection
func createGnb(conn sctp.SCTPConn, wg *sync.WaitGroup) (*GnB, error) {
	//create GnB
	gnb := newGnb()

	//create SCTP connection wrapper
	ngapConn := &newNgapConn(conn, gnb.handle)
	gnb.conn = ngapConn //link Gnb to the connection

	//listen to Ngap messages from AMF
	wg.Add(1) //track one more go routine
	go ngapConn.readLoop()

	//Note: add Gnb to the list only after we send a NGAPSetupResponse

}

func (gnb *Gnb) handle(msg *NgapMessage) error {
	switch msg.MsgType() {
	case ngap.NgapSetupRequest:
		//send a respone
		GetGnbManager().add(gnb) //add gnb to the list
	case ngap.InitialUeMessage:
		gnbUeId := 0 // get from message
		//pick amf
		amf := GetAmfManager().pick() //do load balancing here
		//create ueContext (and allocate a lbUeId)
		ueCtx := _service.createUeContext(amf, gnb, gnbUeId)
		//edit then forward message to Amf as follows:
		// 1. edit message
		// 2. pdu, err := ngap.Encode(msg)
		// 3. amf.conn.sendNgap(pdu)]
	default:
		//TODO: add more cases to handle other Ngap messages, you may want to dispatch to different handler
		//functions
	}

}

func (gnb *Gnb) Close() {
	//clean resources
	gnb.conn.Close() //close connection
}

///////////////// AMF
type AmfManager struct {
	amfList     map[net.Conn]*Amf //indexed by connection
	indexesById map[string]*Amf   //indexed by Amf identity (can be podId)
	mutex       sync.Mutex
}

//wait for K8s events, connect to AMFs/ delete AMFs
func (m *AmfManager) monitorLoop(wg *sync.WaitGroup) {
	defer wg.Done()
	//TODO:
	for {
		//listen to event from K8s controller
		//m.connectAmf(amfInfo, wg)
		//m.remove(AmfId)
	}
}

// add an AMF
func (m *AmfManager) add(amf *Amf) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.amfList[amf.conn] = amf
	m.indexesById[amf.id] = amf
}

func (m *AmfManager) remove(id string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if amf, ok := m.indexesById[id]; ok {
		amf.Close() //close connection and release resource
		delete(m.amfList, amf.conn)
		delete(m.indexesById, id)
	}
}

//create an AMF, send NGapSetupRequest then start a go routin to read data from
//its connection
func (m *AmfManager) connectAmf(amfInfo string, wg *sync.WaitGroup) error {
	//create connection first
	//conn := sctp.Dial ( ...)

	//create AMF
	amf := newAmf(amfInfo)

	//create SCTP connection wrapper
	ngapConn := &newNgapConn(conn, amf.handle)

	amf.conn = ngapConn //link AMF to the connection

	amf.sendSetupRequest() //check for error

	//listen to Ngap messages from AMF
	wg.Add(1) //track one more go routine
	go ngapConn.readLoop()

	//Note: add AMF to the list only when we receive a NGapSetupResponse
	return nil
}

func (m *AmfManager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, amf := range m.amfList {
		amf.Close()
	}
}

type Amf struct {
	conn   *NgapConn //keep for writing Ngap message
	podIp  string
	id     string               //can be podId
	ueList map[int64]*UeContext //index by amfUeId
	mutex  sync.Mutex           //protect conccurency read/write to ueList

	//TODO: add more attributes
}

func newAmf(amfInfo string /*add other infos*/) *Amf {
	return &Amf{
		//TODO: init othe attributes
		ueList: make(map[int64]*UeContext),
	}
}

//handle received Ngap message
func (amf *Amf) handle(msg *NgapMessage) error {
	switch msg.MsgType() {
	case ngap.NgapSetupResponse:
		//send a respone
		GetAmfManager().add(amf) //add gnb to the list

	case ngap.InitialUeContextSetupRequest:
		amfUeId := 0 // get from message
		lbUeId := 0  //get from message
		//look for UeContext
		ueCtx := FindUeContext(lbUeId)
		ueCtx.setAmfUeId(amfUeId) //set Id received from AMF

		amf.add(ueCtx) //now add the UeContext list at this AMF

		//edit then forward message to gnB as follows:
		// 1. edit message
		// 2. pdu, err := ngap.Encode(msg)
		// 3. ueCtx.gnb.conn.sendNgap(pdu)]

	default:
		//TODO: add more cases to handle other Ngap messages, you may want to dispatch to different handler
		//functions
	}
}

//add a new UeContext
func (amf *Amf) add(ueCtx *UeContext) {
	amf.mutex.Lock()
	defer amf.mutex.Unlock()
	amf.ueList[ueCtx.amfUeId] = ueCtx
}

func (amf *Amf) Close() {
	//clean resources
	amf.conn.Close() //close connection
}

/////////////////////// UE

type UeContext struct {
	amf     *Amf
	amfUeId int64
	gnb     *Gnb
	gnbUeId int64
	lbId    int64 //LbUeNgapId
}

//////////////////// Service
type Service struct {
	amfMan  *AmfManager
	gnbMan  *GnbManager
	sctpSrv *SctpServer
	wg      sync.WaitGroup
	lbUeId  int64                //for generating UeId
	ueList  map[int64]*UeContext //indexes by lbUeId
	mutex   sync.Mutex           //protect read/write ueList
}

func (s *Service) createUeContext(amf *Amf, gnb *Gnb, ranUeId int64) *UeContext {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	ueCtx := &UeContext{
		//init here
		lbId: s.lbUeId,
	}
	//add to global UeContext list
	s.ueList[ueCtx.lbId] = ueCtx

	//add to UeContext list at gnB
	gnb.add(ueCtx)

	//increate counter
	s.lbUeId++
}

func (s *Service) findUeCtx(lbUeId int64) *UeContext {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	ueCtx, _ := s.ueList[lbUeId]
	return ueCtx
}

var _service *Service //singleton instance for global access

func (s *Service) Kill() {
	s.sctpSrv.Close()
	s.amfManager.Close()
	s.gnbManager.Close()

	s.wg.Wait() //wait for all go routine to complete
}

func main() {
	//1. READ configuration file

	//2. creat a singleton service instance which is globally accessible
	_service = &Service{
		sctpSrv: newSctpServer(),
		amfMan:  newAmfManager(),
		gnbMan:  newGnbManager(),
	}

	_service.wg.Add(2)

	//3. start SCTP server on a go routing
	go _service.sctpServer.listenLoop(&_service.wg)

	//4. start AMF status listening go routing
	go _service.amfMan.monitorLoop(&_service.wg)

	//5.  TODO listen to keyboard interuption
	for {
		//when receive an interuption
		// service.Kill()
	}
}

func GetAmfManager() *AmfManager {
	return _service.amfMan
}

func GetGnbManager() *GnbManager {
	return _service.gnbMan
}

func FindUeCtx(lbUeId int64) *UeContext {
	return _service.findUeContext(lbUeId)
}
