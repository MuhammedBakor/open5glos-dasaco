/*
Package sctp provides SCTP proxy functionality for 5G NGAP messages.
*/
package sctp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/free5gc/sctp"
	"github.com/hasukiHT/5glos/balancer"
	"github.com/hasukiHT/5glos/gnbue"
	"github.com/hasukiHT/5glos/ngap"
	"github.com/hasukiHT/5glos/utils"
)

// Proxy represents an SCTP proxy server
type Proxy struct {
	port           int
	balancer       balancer.Balancer
	sessionManager *gnbue.SessionManager
	listener       *sctp.SCTPListener
}

// NewProxy creates a new SCTP proxy
func NewProxy(port int, bal balancer.Balancer, sessionManager *gnbue.SessionManager) (*Proxy, error) {
	return &Proxy{
		port:           port,
		balancer:       bal,
		sessionManager: sessionManager,
	}, nil
}

// Start starts the SCTP proxy server
func (p *Proxy) Start(ctx context.Context) error {
	addr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("0.0.0.0")}},
		Port:    p.port,
	}

	listener, err := sctp.ListenSCTP("sctp", addr)
	if err != nil {
		return fmt.Errorf("failed to start SCTP listener: %w", err)
	}
	p.listener = listener

	utils.LogInfo("SCTP proxy started", map[string]interface{}{
		"address": addr.String(),
	})

	// Start cleanup routine
	go p.cleanupRoutine(ctx)

	// Accept connections
	for {
		select {
		case <-ctx.Done():
			utils.LogInfo("Stopping SCTP proxy")
			return p.listener.Close()
		default:
			conn, err := listener.AcceptSCTP()
			if err != nil {
				select {
				case <-ctx.Done():
					return nil
				default:
					utils.LogError("Failed to accept SCTP connection", err)
					continue
				}
			}

			utils.LogInfo("Accepted gNB connection", map[string]interface{}{
				"remote_addr": conn.RemoteAddr().String(),
			})

			go p.handleConnection(ctx, conn)
		}
	}
}

// handleConnection handles a new gNB connection
func (p *Proxy) handleConnection(ctx context.Context, gnbConn *sctp.SCTPConn) {
	defer gnbConn.Close()

	// Read the first NGAP message to extract gNB and UE information
	buf := make([]byte, 4096)
	n, err := gnbConn.Read(buf)
	if err != nil {
		utils.LogError("Failed to read initial NGAP message", err)
		return
	}

	// Parse NGAP message to extract IDs
	gnbID, ueID, err := ngap.ParseInitialMessage(buf[:n])
	if err != nil {
		utils.LogError("Failed to parse initial NGAP message", err)
		return
	}

	utils.LogInfo("Parsed initial NGAP message", map[string]interface{}{
		"gnb_id": gnbID,
		"ue_id":  ueID,
		"size":   n,
	})

	// Select AMF endpoint
	amfEndpoint, err := p.balancer.SelectEndpoint(gnbID, ueID)
	if err != nil {
		utils.LogError("Failed to select AMF endpoint", err)
		return
	}

	// Connect to AMF
	amfAddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP(amfEndpoint.IP)}},
		Port:    amfEndpoint.Port,
	}

	amfConn, err := sctp.DialSCTP("sctp", nil, amfAddr)
	if err != nil {
		utils.LogError("Failed to connect to AMF", err, map[string]interface{}{
			"amf_addr": amfEndpoint.Address(),
		})
		return
	}
	defer amfConn.Close()

	utils.LogInfo("Connected to AMF", map[string]interface{}{
		"amf_id":   amfEndpoint.ID,
		"amf_addr": amfEndpoint.Address(),
	})

	// Create session
	session := p.sessionManager.CreateSession(gnbID, ueID, gnbConn.RemoteAddr(), amfEndpoint)
	session.SetConnections(gnbConn, amfConn)
	defer p.sessionManager.RemoveSession(session.ID)

	// Forward the initial message to AMF
	_, err = amfConn.Write(buf[:n])
	if err != nil {
		utils.LogError("Failed to forward initial message to AMF", err)
		return
	}

	// Start bidirectional proxying
	done := make(chan error, 2)

	go func() {
		done <- p.proxyData(gnbConn, amfConn, "gNB->AMF", session)
	}()

	go func() {
		done <- p.proxyData(amfConn, gnbConn, "AMF->gNB", session)
	}()

	// Wait for either direction to complete or context cancellation
	select {
	case <-ctx.Done():
		utils.LogInfo("Session terminated by context", map[string]interface{}{
			"session_id": session.ID,
		})
	case err := <-done:
		if err != nil {
			utils.LogError("Proxy error", err, map[string]interface{}{
				"session_id": session.ID,
			})
		}
	}
}

// proxyData proxies data between two SCTP connections
func (p *Proxy) proxyData(src, dst *sctp.SCTPConn, direction string, session *gnbue.Session) error {
	buf := make([]byte, 4096)

	for {
		n, err := src.Read(buf)
		if err != nil {
			return fmt.Errorf("read error in %s: %w", direction, err)
		}

		if n > 0 {
			// Update session activity
			session.UpdateLastActive()

			// Log NGAP message if possible
			msgType, err := ngap.GetMessageType(buf[:n])
			if err == nil {
				utils.LogDebug("Proxying NGAP message", map[string]interface{}{
					"direction":    direction,
					"message_type": msgType,
					"size":         n,
					"session_id":   session.ID,
				})
			}

			_, err = dst.Write(buf[:n])
			if err != nil {
				return fmt.Errorf("write error in %s: %w", direction, err)
			}
		}
	}
}

// cleanupRoutine periodically cleans up stale sessions
func (p *Proxy) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.sessionManager.CleanupStale(5 * time.Minute)
		}
	}
}
