package proxy

import (
	"context"
	"log"
	"net"
	"sync"

	"github.com/free5gc/ngap/ngapType"
	"github.com/ishidawataru/sctp"
)

type AMFConnection struct {
	Info     AMFInfo
	Conn     *sctp.SCTPConn
	LastSeen time.Time
	mutex    sync.RWMutex
}

func (ps *ProxyServer) connectToAMF(amf AMFInfo) {
	addr := fmt.Sprintf("%s:%d", ps.minikubeIP, amf.NodePort)
	raddr, err := sctp.ResolveSCTPAddr("sctp", addr)
	if err != nil {
		log.Printf("[ERROR] ResolveSCTPAddr error for %s: %v", addr, err)
		return
	}

	conn, err := sctp.DialSCTP("sctp", nil, raddr)
	if err != nil {
		log.Printf("[ERROR] SCTP dial error for %s: %v", addr, err)
		return
	}

	key := fmt.Sprintf("%s:%d", amf.PodName, amf.NodePort)
	amfConn := &AMFConnection{
		Info:     amf,
		Conn:     conn,
		LastSeen: time.Now(),
	}

	ps.amfConns[key] = amfConn
	log.Printf("[INFO] Connected to AMF: %s", key)

	go ps.handleAMFConnection(amfConn)
	go ps.sendNGSetupRequest(amfConn)
}

func (ps *ProxyServer) handleAMFConnection(amfConn *AMFConnection) {
	defer func() {
		amfConn.Conn.Close()
		ps.mutex.Lock()
		key := fmt.Sprintf("%s:%d", amfConn.Info.PodName, amfConn.Info.NodePort)
		delete(ps.amfConns, key)
		ps.mutex.Unlock()
		log.Printf("[INFO] AMF connection closed: %s", key)
	}()

	buf := make([]byte, 4096)
	for {
		amfConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		n, err := amfConn.Conn.Read(buf)
		if err != nil {
			log.Printf("[ERROR] AMF read error: %v", err)
			return
		}

		amfConn.mutex.Lock()
		amfConn.LastSeen = time.Now()
		amfConn.mutex.Unlock()

		ngapMsg, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[ERROR] NGAP decode error from AMF: %v", err)
			continue
		}

		log.Printf("[INFO] Received %d bytes from AMF %s", n, amfConn.Info.PodName)

		ps.forwardToGNB(ngapMsg, buf[:n])
	}
}

func (ps *ProxyServer) sendNGSetupRequest(amfConn *AMFConnection) {
	info := &sctp.SndRcvInfo{PPID: 60}

	pkt, err := BuildNGSetupRequest(proxyRan)
	if err != nil {
		log.Printf("[ERROR] Build NGSetupRequest failed: %v", err)
		return
	}

	amfConn.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	n, err := amfConn.Conn.SCTPWrite(pkt, info)
	if err != nil {
		log.Printf("[ERROR] SCTP write error to AMF: %v", err)
		return
	}

	log.Printf("[INFO] Sent NGSetupRequest (%d bytes) to AMF %s", n, amfConn.Info.PodName)
}