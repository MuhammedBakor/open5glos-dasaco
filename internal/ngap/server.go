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

func NewNgapServer() *NgapServer {
	laddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("127.0.0.10")}}, // Use a fixed IP for local address for RAN connections
		// Use a base port for the server
		Port: 38412,
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
	log.Println("[INFO] SCTP server listening on 127.0.0.10:38412")

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
