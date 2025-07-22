package proxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/ishidawataru/sctp"
	"github.com/free5gc/ngap/ngapType"
	"github.com/free5gc/ngap/logger"
)

type AMFConnection struct {
	Info     AMFInfo
	Conn     *sctp.SCTPConn
	LastSeen time.Time
	mutex    sync.RWMutex
}

type ProxyServer struct {
	listener   *sctp.SCTPListener
	amfConns   map[string]*AMFConnection
	ranConns   map[string]*AmfRan
	mutex      sync.RWMutex
	clientset  *kubernetes.Clientset
	namespace  string
	minikubeIP string
}

func NewProxyServer(namespace string) (*ProxyServer, error) {
	laddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("127.0.0.10")}},
		Port:    38412,
	}
	listener, err := sctp.ListenSCTP("sctp", laddr)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to listen SCTP: %v", err)
	}

	minikubeIP, err := getMinikubeIP()
	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to get minikube IP: %v", err)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("[ERROR] failed to get kubeconfig: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to create k8s client: %v", err)
	}

	return &ProxyServer{
		listener:   listener,
		amfConns:   make(map[string]*AMFConnection),
		ranConns:   make(map[string]*AmfRan),
		clientset:  clientset,
		namespace:  namespace,
		minikubeIP: minikubeIP,
	}, nil
}

func (ps *ProxyServer) Start() error {
	fmt.Println("[INFO] Proxy SCTP server listening on 127.0.0.10:38412")

	go ps.manageAMFConnections()
	go ps.acceptGNBConnections()

	select {}
}

func (ps *ProxyServer) manageAMFConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ps.refreshAMFConnections()
		}
	}
}

func (ps *ProxyServer) refreshAMFConnections() {
	amfs, err := getAMFPodsAndPorts(ps.clientset, ps.namespace)
	if err != nil {
		log.Printf("[ERROR] Failed to get AMF pods: %v", err)
		return
	}

	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	for _, amf := range amfs {
		key := fmt.Sprintf("%s:%d", amf.PodName, amf.NodePort)

		if conn, exists := ps.amfConns[key]; exists {
			conn.Info = amf
			conn.LastSeen = time.Now()
		} else {
			ps.connectToAMF(amf)
		}
	}

	ps.cleanupStaleAMFConnections()
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

func (ps *ProxyServer) acceptGNBConnections() {
	for {
		conn, err := ps.listener.AcceptSCTP()
		if err != nil {
			log.Printf("[ERROR] Accept error: %v", err)
			continue
		}

		ran := &AmfRan{
			Conn: conn,
			Name: fmt.Sprintf("RAN-%v", conn.RemoteAddr()),
			Log:  logrus.NewEntry(logrus.New()),
		}

		ps.mutex.Lock()
		ps.ranConns[ran.Name] = ran
		ps.mutex.Unlock()

		log.Printf("[INFO] Accepted gNB connection from %v", conn.RemoteAddr())
		go ps.handleGNBConnection(ran)
	}
}

func (ps *ProxyServer) handleGNBConnection(ran *AmfRan) {
	defer func() {
		ran.Conn.Close()
		ps.mutex.Lock()
		delete(ps.ranConns, ran.Name)
		ps.mutex.Unlock()
		log.Printf("[INFO] gNB connection closed: %s", ran.Name)
	}()

	buf := make([]byte, 4096)
	for {
		ran.Conn.SetReadDeadline(time.Now().Add(300 * time.Second))
		n, err := ran.Conn.Read(buf)
		if err != nil {
			log.Printf("[ERROR] gNB read error: %v", err)
			return
		}

		log.Printf("[INFO] Received %d bytes from gNB %s", n, ran.Name)

		ngapMsg, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[ERROR] NGAP decode error from gNB: %v", err)
			continue
		}

		ps.handleGNBMessage(ran, ngapMsg, buf[:n])
	}
}

func (ps *ProxyServer) handleGNBMessage(ran *AmfRan, ngapMsg *ngapType.NGAPPDU, rawMsg []byte) {
	switch ngapMsg.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := ngapMsg.InitiatingMessage
		if initiatingMessage == nil {
			return
		}

		switch initiatingMessage.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			ps.handleNGSetupFromGNB(ran, initiatingMessage)
		default:
			ps.forwardToAMF(rawMsg)
		}
	default:
		ps.forwardToAMF(rawMsg)
	}
}

func (ps *ProxyServer) handleNGSetupFromGNB(ran *AmfRan, initiatingMessage *ngapType.InitiatingMessage) {
	ran.Log.Infoln("[INFO] Handling NGSetupRequest from gNB...")

	pkt, err := BuildNGSetupResponse()
	if err != nil {
		ran.Log.Errorf("[ERROR] Build NGSetupResponse failed: %v", err)
		return
	}

	SendToRan(ran, pkt)
	ran.Log.Infoln("[INFO] Sent NGSetup Response to gNB")
}

func (ps *ProxyServer) forwardToAMF(message []byte) {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	for _, amfConn := range ps.amfConns {
		amfConn.mutex.RLock()
		if time.Since(amfConn.LastSeen) < 60*time.Second {
			amfConn.mutex.RUnlock()

			info := &sctp.SndRcvInfo{PPID: 60}
			_, err := amfConn.Conn.SCTPWrite(message, info)
			if err != nil {
				log.Printf("[ERROR] Forward to AMF error: %v", err)
				continue
			}
			log.Printf("[INFO] Forwarded message to AMF %s", amfConn.Info.PodName)
			return
		}
		amfConn.mutex.RUnlock()
	}
	log.Printf("[ERROR] No available AMF to forward message")
}

func (ps *ProxyServer) forwardToGNB(ngapMsg *ngapType.NGAPPDU, rawMsg []byte) {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	for _, ran := range ps.ranConns {
		_, err := ran.Conn.Write(rawMsg)
		if err != nil {
			log.Printf("[INFO] Forward to gNB error: %v", err)
			continue
		}
		log.Printf("[INFO] Forwarded message to gNB %s", ran.Name)
	}
}

func (ps *ProxyServer) cleanupStaleAMFConnections() {
	now := time.Now()
	for key, conn := range ps.amfConns {
		conn.mutex.RLock()
		if now.Sub(conn.LastSeen) > 120*time.Second {
			conn.mutex.RUnlock()
			conn.Conn.Close()
			delete(ps.amfConns, key)
			log.Printf("[INFO] Removed stale AMF connection: %s", key)
		} else {
			conn.mutex.RUnlock()
		}
	}
}

func (ps *ProxyServer) Close() {
	ps.listener.Close()

	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	for _, amfConn := range ps.amfConns {
		amfConn.Conn.Close()
	}

	for _, ran := range ps.ranConns {
		ran.Conn.Close()
	}
}