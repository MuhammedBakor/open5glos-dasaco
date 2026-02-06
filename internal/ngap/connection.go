package ngap

import (
	"log"
	"net"
	"time"

	"sync"

	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
	"github.com/ishidawataru/sctp"
)

type NgapMessage = ngapType.NGAPPDU

// Represent a SCTP connection to GnB or AMF
type NgapConn struct {
	conn    net.Conn
	handler func(*NgapMessage) error
	done    chan struct{}
}

func NewNgapConn(conn net.Conn, handler func(*NgapMessage) error) *NgapConn {
	return &NgapConn{
		conn:    conn,
		handler: handler,
		done:    make(chan struct{}, 1),
	}
}

// Loop to read data from connection, decode then send to handler
func (c *NgapConn) ReadLoop(wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 4096)

	for {
		select {
		case <-c.done:
			return
		default:
			// Read from connection
			c.conn.SetReadDeadline(time.Now().Add(300 * time.Second))
			n, err := c.conn.Read(buf)
			if err != nil {
				log.Printf("[ERROR] Read error: %v", err)
				return
			}

			// Decode message
			msg, err := ngap.Decoder(buf[:n])
			if err != nil {
				log.Printf("[ERROR] NGAP decode error: %v", err)
				continue
			}

			if msg == nil {
				log.Printf("[ERROR] NGAP Message is nil")
				continue
			}

			// Handle NGAP message
			if err := c.handler(msg); err != nil {
				log.Printf("[ERROR] Message handler error: %v", err)
			}
		}
	}
}

// Send an NGAP PDU
func (c *NgapConn) SendNgap(pdu []byte) error {
	// Check if this is an SCTP connection (for AMF connections)
	if sctpConn, ok := c.conn.(*sctp.SCTPConn); ok {
		info := &sctp.SndRcvInfo{PPID: 60}
		_, err := sctpConn.SCTPWrite(pdu, info)
		return err
	}

	// Fallback to regular Write for non-SCTP connections
	_, err := c.conn.Write(pdu)
	return err
}

func (c *NgapConn) Close() {
	c.conn.Close()
	select {
	case c.done <- struct{}{}:
	default:
	}
}

// GetConn returns the underlying connection
func (c *NgapConn) GetConn() net.Conn {
	return c.conn
}
