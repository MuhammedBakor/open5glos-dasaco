/*
Package sctp provides SCTP proxy functionality for 5G NGAP messages.
*/
package sctp

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/hasukiHT/5glos/internal/balancer"
	"github.com/hasukiHT/5glos/internal/config"
	"github.com/hasukiHT/5glos/internal/mapper"
	"github.com/hasukiHT/5glos/internal/metrics"
	"github.com/hasukiHT/5glos/internal/ngap"
	"github.com/hasukiHT/5glos/internal/watcher"
)

// Connection represents a bidirectional SCTP connection
type Connection struct {
	ID           string
	GNBConn      net.Conn
	AMFConn      net.Conn
	UEIF         string
	AMFID        string
	CreatedAt    time.Time
	LastActivity time.Time
	mu           sync.RWMutex
}

// UpdateActivity updates the last activity time
func (c *Connection) UpdateActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastActivity = time.Now()
}

// GetLastActivity returns the last activity time
func (c *Connection) GetLastActivity() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LastActivity
}

// Close closes both connections
func (c *Connection) Close() error {
	var err1, err2 error
	if c.GNBConn != nil {
		err1 = c.GNBConn.Close()
	}
	if c.AMFConn != nil {
		err2 = c.AMFConn.Close()
	}

	if err1 != nil {
		return err1
	}
	return err2
}

// Proxy handles SCTP proxy functionality
type Proxy struct {
	cfg           config.ProxyConfig
	amfPool       *watcher.AMFPool
	sessionMapper *mapper.SessionMapper
	ngapParser    *ngap.Parser
	metrics       *metrics.Registry
	logger        *zap.Logger
	connections   map[string]*Connection
	listener      net.Listener
	mu            sync.RWMutex
}

// NewProxy creates a new SCTP proxy
func NewProxy(cfg config.ProxyConfig, amfPool *watcher.AMFPool, metrics *metrics.Registry, logger *zap.Logger) (*Proxy, error) {
	// Create session mapper
	sessionMapper := mapper.NewSessionMapper(
		config.LoadBalancerConfig{
			SessionTimeout: 30 * time.Minute, // TODO: Get from config
		},
		metrics,
		logger.Named("mapper"),
	)

	return &Proxy{
		cfg:           cfg,
		amfPool:       amfPool,
		sessionMapper: sessionMapper,
		ngapParser:    ngap.NewParser(),
		metrics:       metrics,
		logger:        logger,
		connections:   make(map[string]*Connection),
	}, nil
}

// Start starts the SCTP proxy server
func (p *Proxy) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", p.cfg.ListenAddr, p.cfg.ListenPort)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	p.listener = listener

	p.logger.Info("SCTP proxy started",
		zap.String("address", addr),
		zap.Int("buffer_size", p.cfg.BufferSize),
		zap.Int("max_connections", p.cfg.MaxConnections),
	)

	// Start connection cleanup routine
	go p.cleanupRoutine(ctx)

	// Accept connections
	for {
		select {
		case <-ctx.Done():
			return p.shutdown()
		default:
			conn, err := listener.Accept()
			if err != nil {
				if ctx.Err() != nil {
					return nil // Context cancelled
				}
				p.logger.Error("Failed to accept connection", zap.Error(err))
				p.metrics.RecordConnectionError()
				continue
			}

			// Handle connection in goroutine
			go p.handleConnection(ctx, conn)
		}
	}
}

// handleConnection handles a new gNB connection
func (p *Proxy) handleConnection(ctx context.Context, gnbConn net.Conn) {
	connID := fmt.Sprintf("conn-%d", time.Now().UnixNano())

	p.logger.Info("New gNB connection",
		zap.String("connection_id", connID),
		zap.String("remote_addr", gnbConn.RemoteAddr().String()),
	)

	defer func() {
		gnbConn.Close()
		p.removeConnection(connID)
		p.metrics.RecordConnectionClosed()

		p.logger.Info("gNB connection closed",
			zap.String("connection_id", connID),
		)
	}()

	// Set timeouts
	if p.cfg.ReadTimeout > 0 {
		gnbConn.SetReadDeadline(time.Now().Add(p.cfg.ReadTimeout))
	}
	if p.cfg.WriteTimeout > 0 {
		gnbConn.SetWriteDeadline(time.Now().Add(p.cfg.WriteTimeout))
	}

	p.metrics.RecordConnection()

	// Create connection object
	connection := &Connection{
		ID:           connID,
		GNBConn:      gnbConn,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}

	p.addConnection(connection)

	// Handle the connection
	if err := p.proxyConnection(ctx, connection); err != nil {
		p.logger.Error("Connection proxy error",
			zap.String("connection_id", connID),
			zap.Error(err),
		)
	}
}

// proxyConnection handles bidirectional proxying
func (p *Proxy) proxyConnection(ctx context.Context, conn *Connection) error {
	// Read first message to determine UE and select AMF
	buffer := make([]byte, p.cfg.BufferSize)

	n, err := conn.GNBConn.Read(buffer)
	if err != nil {
		return fmt.Errorf("failed to read initial message: %w", err)
	}

	conn.UpdateActivity()

	// Parse NGAP message
	msgInfo, err := p.ngapParser.ParseMessage(buffer[:n], "uplink")
	if err != nil {
		p.logger.Warn("Failed to parse initial NGAP message",
			zap.String("connection_id", conn.ID),
			zap.Error(err),
		)
		// Continue anyway, treat as unknown message
		msgInfo = &ngap.MessageInfo{
			Type:      "Unknown",
			Direction: "uplink",
		}
	}

	p.logger.Info("Received initial NGAP message",
		zap.String("connection_id", conn.ID),
		zap.String("message_type", msgInfo.Type),
		zap.String("ue_id", msgInfo.GetUEIdentifier()),
	)

	p.metrics.RecordNGAPMessage(msgInfo.Type, msgInfo.Direction, n)

	// Select AMF based on UE identifier or use load balancing
	ueID := msgInfo.GetUEIdentifier()
	if ueID == "" {
		// For non-UE specific messages (like NG Setup), use a default UE ID
		ueID = fmt.Sprintf("gnb-%s", conn.GNBConn.RemoteAddr().String())
	}

	// Check if UE already has a session
	var selectedAMF *balancer.AMFInstance
	if session, exists := p.sessionMapper.GetSession(ueID); exists {
		// Use existing AMF
		if amf, exists := p.amfPool.GetBalancer().GetAMF(session.AMFID); exists && amf.IsHealthy() {
			selectedAMF = amf
			p.metrics.RecordSessionAffinityHit()
		} else {
			// AMF is no longer available, remove session and select new one
			p.sessionMapper.RemoveSession(ueID)
			p.metrics.RecordSessionAffinityMiss()
		}
	}

	if selectedAMF == nil {
		// Select new AMF
		amf, err := p.amfPool.SelectAMF(ueID)
		if err != nil {
			return fmt.Errorf("failed to select AMF: %w", err)
		}
		selectedAMF = amf

		// Create new session
		p.sessionMapper.CreateSession(ueID, selectedAMF.ID, conn.ID)
	}

	// Connect to selected AMF
	amfConn, err := net.DialTimeout("tcp", selectedAMF.GetAddress(), 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to AMF %s: %w", selectedAMF.GetAddress(), err)
	}

	conn.AMFConn = amfConn
	conn.UEIF = ueID
	conn.AMFID = selectedAMF.ID

	p.logger.Info("Established AMF connection",
		zap.String("connection_id", conn.ID),
		zap.String("ue_id", ueID),
		zap.String("amf_id", selectedAMF.ID),
		zap.String("amf_address", selectedAMF.GetAddress()),
	)

	// Forward the initial message to AMF
	if _, err := amfConn.Write(buffer[:n]); err != nil {
		return fmt.Errorf("failed to forward initial message to AMF: %w", err)
	}

	// Update session activity
	p.sessionMapper.UpdateSessionActivity(ueID, msgInfo.RANUENGAPID, msgInfo.AMFUENGAPID)

	// Start bidirectional proxying
	errChan := make(chan error, 2)

	// gNB -> AMF
	go func() {
		err := p.proxyData(conn.GNBConn, conn.AMFConn, "uplink", conn)
		errChan <- err
	}()

	// AMF -> gNB
	go func() {
		err := p.proxyData(conn.AMFConn, conn.GNBConn, "downlink", conn)
		errChan <- err
	}()

	// Wait for either direction to close or error
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

// proxyData proxies data in one direction
func (p *Proxy) proxyData(src, dst net.Conn, direction string, conn *Connection) error {
	buffer := make([]byte, p.cfg.BufferSize)

	for {
		// Set read timeout
		if p.cfg.ReadTimeout > 0 {
			src.SetReadDeadline(time.Now().Add(p.cfg.ReadTimeout))
		}

		n, err := src.Read(buffer)
		if err != nil {
			if err == io.EOF {
				p.logger.Debug("Connection closed",
					zap.String("connection_id", conn.ID),
					zap.String("direction", direction),
				)
				return nil
			}
			return fmt.Errorf("read error in %s direction: %w", direction, err)
		}

		conn.UpdateActivity()

		// Parse NGAP message for logging and metrics
		if msgInfo, err := p.ngapParser.ParseMessage(buffer[:n], direction); err == nil {
			p.logger.Debug("NGAP message",
				zap.String("connection_id", conn.ID),
				zap.String("direction", direction),
				zap.String("message_type", msgInfo.Type),
				zap.String("ue_id", msgInfo.GetUEIdentifier()),
				zap.Int("size", n),
			)

			p.metrics.RecordNGAPMessage(msgInfo.Type, direction, n)

			// Update session activity if UE specific
			if ueID := msgInfo.GetUEIdentifier(); ueID != "" {
				p.sessionMapper.UpdateSessionActivity(ueID, msgInfo.RANUENGAPID, msgInfo.AMFUENGAPID)
			}
		}

		// Set write timeout
		if p.cfg.WriteTimeout > 0 {
			dst.SetWriteDeadline(time.Now().Add(p.cfg.WriteTimeout))
		}

		// Forward data
		if _, err := dst.Write(buffer[:n]); err != nil {
			return fmt.Errorf("write error in %s direction: %w", direction, err)
		}
	}
}

// addConnection adds a connection to the tracking map
func (p *Proxy) addConnection(conn *Connection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.connections[conn.ID] = conn
}

// removeConnection removes a connection from the tracking map
func (p *Proxy) removeConnection(connID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if conn, exists := p.connections[connID]; exists {
		delete(p.connections, connID)

		// Remove associated session
		p.sessionMapper.RemoveSessionByConnection(connID)

		p.logger.Debug("Removed connection",
			zap.String("connection_id", connID),
			zap.String("ue_id", conn.UEIF),
		)
	}
}

// cleanupRoutine periodically cleans up stale connections
func (p *Proxy) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.cleanupStaleConnections()
		}
	}
}

// cleanupStaleConnections removes connections that haven't been active
func (p *Proxy) cleanupStaleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	staleTimeout := 10 * time.Minute // TODO: Make configurable
	var staleConnections []string

	for connID, conn := range p.connections {
		if now.Sub(conn.GetLastActivity()) > staleTimeout {
			staleConnections = append(staleConnections, connID)
		}
	}

	// Close and remove stale connections
	for _, connID := range staleConnections {
		if conn, exists := p.connections[connID]; exists {
			conn.Close()
			delete(p.connections, connID)

			p.logger.Info("Removed stale connection",
				zap.String("connection_id", connID),
				zap.Duration("inactive_duration", now.Sub(conn.GetLastActivity())),
			)
		}
	}

	if len(staleConnections) > 0 {
		p.logger.Info("Cleanup completed",
			zap.Int("removed_connections", len(staleConnections)),
			zap.Int("active_connections", len(p.connections)),
		)
	}
}

// GetActiveConnections returns the number of active connections
func (p *Proxy) GetActiveConnections() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.connections)
}

// shutdown gracefully shuts down the proxy
func (p *Proxy) shutdown() error {
	p.logger.Info("Shutting down SCTP proxy")

	// Close listener
	if p.listener != nil {
		p.listener.Close()
	}

	// Close all connections
	p.mu.Lock()
	defer p.mu.Unlock()

	for connID, conn := range p.connections {
		conn.Close()
		p.logger.Debug("Closed connection during shutdown",
			zap.String("connection_id", connID),
		)
	}

	p.logger.Info("SCTP proxy shutdown complete")
	return nil
}
