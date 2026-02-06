package ngap

import (
	"log"
	"net"
	"sync"

	"github.com/ishidawataru/sctp"
)

// SCTP server wrapper
type NgapServer struct {
	listener *sctp.SCTPListener
	done     chan struct{}
}

func NewNgapServer(listenAddr string, listenPort int) *NgapServer {
	laddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP(listenAddr)}},
		Port:    listenPort,
	}
	listener, err := sctp.ListenSCTP("sctp", laddr)
	if err != nil {
		log.Fatalf("[ERROR] Failed to create SCTP listener: %v", err)
	}

	return &NgapServer{
		listener: listener,
		done:     make(chan struct{}),
	}
}

func (s *NgapServer) ListenLoop(wg *sync.WaitGroup, onConnection func(net.Conn)) {
	defer wg.Done()
	log.Printf("[INFO] SCTP server listening on %s:%d", s.listener.Addr().(*sctp.SCTPAddr).IPAddrs[0].IP, s.listener.Addr().(*sctp.SCTPAddr).Port)

	for {
		select {
		case <-s.done:
			return
		default:
			// Accept new connection
			conn, err := s.listener.AcceptSCTP()
			if err != nil {
				log.Printf("[ERROR] Accept error: %v", err)
				continue
			}
			log.Printf("[INFO] Accepted connection from %v", conn.RemoteAddr())

			// Handle new connection
			if onConnection != nil {
				onConnection(conn)
			}
		}
	}
}

func (s *NgapServer) Close() {
	close(s.done)
	if s.listener != nil {
		s.listener.Close()
	}
}
