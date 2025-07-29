package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/logger"
	"github.com/free5gc/ngap/ngapConvert"
	"github.com/free5gc/ngap/ngapType"
	"github.com/free5gc/openapi/models"
	"github.com/ishidawataru/sctp"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ///////////  NGAP
type NgapMessage = ngapType.NGAPPDU

// SCTP server wrapper
type NgapServer struct {
	listener *sctp.SCTPListener
	done     chan struct{}
}

func newNgapServer() *NgapServer {
	laddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("127.0.0.10")}},
		Port:    38412,
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

func (s *NgapServer) listenLoop(wg *sync.WaitGroup) {
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

			// Create GnB and start handling
			gnb := createGnb(conn, wg)
			GetGnbManager().add(gnb)
		}
	}
}

func (s *NgapServer) Close() {
	close(s.done)
	if s.listener != nil {
		s.listener.Close()
	}
}

// Represent a SCTP connection to GnB or AMF
type NgapConn struct {
	conn    net.Conn
	handler func(*NgapMessage) error
	done    chan struct{}
}

func newNgapConn(conn net.Conn, handler func(*NgapMessage) error) *NgapConn {
	return &NgapConn{
		conn:    conn,
		handler: handler,
		done:    make(chan struct{}, 1),
	}
}

// Loop to read data from connection, decode then send to handler
func (c *NgapConn) readLoop(wg *sync.WaitGroup) {
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
func (c *NgapConn) sendNgap(pdu []byte) error {
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

// //////////////// GNB
type GnbManager struct {
	gnbList map[net.Conn]*Gnb
	mutex   sync.Mutex
}

func newGnbManager() *GnbManager {
	return &GnbManager{
		gnbList: make(map[net.Conn]*Gnb),
	}
}

// Add a GnB
func (m *GnbManager) add(gnb *Gnb) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.gnbList[gnb.conn.conn] = gnb
	log.Printf("[INFO] Added GnB to manager")
}

func (m *GnbManager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, gnb := range m.gnbList {
		gnb.Close()
	}
}

type Gnb struct {
	conn   *NgapConn
	ueList map[int64]*UeContext
	mutex  sync.Mutex
	name   string
}

func newGnb(name string) *Gnb {
	return &Gnb{
		ueList: make(map[int64]*UeContext),
		name:   name,
	}
}

// Add UeContext
func (gnb *Gnb) add(ueCtx *UeContext) {
	gnb.mutex.Lock()
	defer gnb.mutex.Unlock()
	gnb.ueList[ueCtx.gnbUeId] = ueCtx
}

// Create a GnB and start a goroutine to read data from its connection
func createGnb(conn net.Conn, wg *sync.WaitGroup) *Gnb {
	// Create GnB
	gnb := newGnb(fmt.Sprintf("GnB-%v", conn.RemoteAddr()))

	// Create SCTP connection wrapper
	ngapConn := newNgapConn(conn, gnb.handle)
	gnb.conn = ngapConn

	// Listen to NGAP messages from GnB
	wg.Add(1)
	go ngapConn.readLoop(wg)

	return gnb
}

func (gnb *Gnb) handle(msg *NgapMessage) error {
	switch msg.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := msg.InitiatingMessage
		if initiatingMessage == nil {
			return fmt.Errorf("InitiatingMessage is nil")
		}

		switch initiatingMessage.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			log.Printf("[INFO] Handling NGSetupRequest from GnB")
			return gnb.handleNGSetupRequest(initiatingMessage)
		// case ngapType.ProcedureCodeNGReset:
		// 	log.Printf("[INFO] Handling NGReset from GnB")
		// 	return gnb.handleNGReset(initiatingMessage)

		// case ngapType.ProcedureCodeAMFConfigurationUpdate:
		// 	log.Printf("[INFO] Handling AMF Configuration Update from GnB")
		// 	return gnb.handleAMFConfigurationUpdate(initiatingMessage)
		// case ngapType.ProcedureCodeAMFStatusIndication:
		// 	log.Printf("[INFO] Handling AMF Status Indication from GnB")
		// 	return gnb.handleAMFStatusIndication(initiatingMessage)
		// case ngapType.ProcedureCodeCellTrafficTrace:
		// 	log.Printf("[INFO] Handling Cell Traffic Trace from GnB")
		// 	return gnb.handleCellTrafficTrace(initiatingMessage)
		// case ngapType.ProcedureCodeDeactivateTrace:
		// 	log.Printf("[INFO] Handling Deactivate Trace from GnB")
		// 	return gnb.handleDeactivateTrace(initiatingMessage)
		// case ngapType.ProcedureCodeDownlinkNASTransport:
		// 	log.Printf("[INFO] Handling Downlink NAS Transport from GnB")
		// 	return gnb.handleDownlinkNASTransport(initiatingMessage)
		// case ngapType.ProcedureCodeDownlinkRANStatusTransfer:
		// 	log.Printf("[INFO] Handling Downlink RAN Status Transfer from GnB")
		// 	return gnb.handleDownlinkRANStatusTransfer(initiatingMessage)
		// case ngapType.ProcedureCodeErrorIndication:
		// 	log.Printf("[INFO] Handling Error Indication from GnB")
		// 	return gnb.handleErrorIndication(initiatingMessage)
		// case ngapType.ProcedureCodeHandoverCancel:
		// 	log.Printf("[INFO] Handling Handover Cancel from GnB")
		// 	return gnb.handleHandoverCancel(initiatingMessage)
		// case ngapType.ProcedureCodeHandoverNotification:
		// 	log.Printf("[INFO] Handling Handover Notification from GnB")
		// 	return gnb.handleHandoverNotification(initiatingMessage)
		// case ngapType.ProcedureCodeHandoverResourceAllocation:
		// 	log.Printf("[INFO] Handling Handover Resource Allocation from GnB")
		// 	return gnb.handleHandoverResourceAllocation(initiatingMessage)
		// case ngapType.ProcedureCodeHandoverPreparation:
		// 	log.Printf("[INFO] Handling Handover Preparation from GnB")
		// 	return gnb.handleHandoverPreparation(initiatingMessage)
		// case ngapType.ProcedureCodeInitialContextSetup:
		// 	log.Printf("[INFO] Handling Initial Context Setup from GnB")
		// 	return gnb.handleInitialContextSetupRequest(initiatingMessage)

		case ngapType.ProcedureCodeInitialUEMessage:
			log.Printf("[INFO] Handling InitialUEMessage from GnB")
			return gnb.handleInitialUEMessage(initiatingMessage)

		// case ngapType.ProcedureCodeLocationReport:
		// 	log.Printf("[INFO] Handling Location Report from GnB")
		// 	return gnb.handleLocationReport(initiatingMessage)
		// case ngapType.ProcedureCodeLocationReportingControl:
		// 	log.Printf("[INFO] Handling Location Reporting Control from GnB")
		// 	return gnb.handleLocationReportingControl(initiatingMessage)
		// case ngapType.ProcedureCodeLocationReportingFailureIndication:
		// 	log.Printf("[INFO] Handling Location Reporting Failure Indication from GnB")
		// 	return gnb.handleLocationReportingFailureIndication(initiatingMessage)
		// case ngapType.ProcedureCodeNASNonDeliveryIndication:
		// 	log.Printf("[INFO] Handling NAS Non-Delivery Indication from GnB")
		// 	return gnb.handleNASNonDeliveryIndication(initiatingMessage)

		// case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
		// 	log.Printf("[INFO] Handling PDUSession Resource Modify Indication from GnB")
		// 	return gnb.handlePDUSessionResourceModifyIndication(initiatingMessage)
		// case ngapType.ProcedureCodePDUSessionResourceModify:
		// 	log.Printf("[INFO] Handling PDUSession Resource Modify Request from GnB")
		// 	return gnb.handlePDUSessionResourceModifyRequest(initiatingMessage)
		// case ngapType.ProcedureCodePDUSessionResourceNotify:
		// 	log.Printf("[INFO] Handling PDUSession Resource Notify from GnB")
		// 	return gnb.handlePDUSessionResourceNotify(initiatingMessage)
		// case ngapType.ProcedureCodePDUSessionResourceRelease:
		// 	log.Printf("[INFO] Handling PDUSession Resource Release from GnB")
		// 	return gnb.handlePDUSessionResourceRelease(initiatingMessage)
		// case ngapType.ProcedureCodePDUSessionResourceSetup:
		// 	log.Printf("[INFO] Handling PDUSession Resource Setup from GnB")
		// 	return gnb.handlePDUSessionResourceSetup(initiatingMessage)
		// case ngapType.ProcedureCodeRerouteNASRequest:
		// 	log.Printf("[INFO] Handling Reroute NAS Request from GnB")
		// 	return gnb.handleRerouteNASRequest(initiatingMessage)
		// case ngapType.ProcedureCodeSecondaryRATDataUsageReport:
		// 	log.Printf("[INFO] Handling Secondary RAT Data Usage Report from GnB")
		// 	return gnb.handleSecondaryRATDataUsageReport(initiatingMessage)
		// case ngapType.ProcedureCodeTraceFailureIndication:
		// 	log.Printf("[INFO] Handling Trace Failure Indication from GnB")
		// 	return gnb.handleTraceFailureIndication(initiatingMessage)
		// case ngapType.ProcedureCodeTraceStart:
		// 	log.Printf("[INFO] Handling Trace Start from GnB")
		// 	return gnb.handleTraceStart(initiatingMessage)
		// case ngapType.ProcedureCodeUEContextModification:
		// 	log.Printf("[INFO] Handling UE Context Modification Request from GnB")
		// 	return gnb.handleUEContextModificationRequest(initiatingMessage)
		// case ngapType.ProcedureCodeUEContextRelease:
		// 	log.Printf("[INFO] Handling UE Context Release Command from GnB")
		// 	return gnb.handleUEContextReleaseCommand(initiatingMessage)
		// case ngapType.ProcedureCodeUEContextReleaseRequest:
		// 	log.Printf("[INFO] Handling UE Context Release Request from GnB")
		// 	return gnb.handleUEContextReleaseRequest(initiatingMessage)
		// case ngapType.ProcedureCodeUETNLABindingRelease:
		// 	log.Printf("[INFO] Handling UETNLA Binding Release Request from GnB")
		// 	return gnb.handleUETNLABindingReleaseRequest(initiatingMessage)
		// case ngapType.ProcedureCodeUplinkNASTransport:
		// 	log.Printf("[INFO] Handling Uplink NAS Transport from GnB")
		// 	return gnb.handleUplinkNASTransport(initiatingMessage)
		// case ngapType.ProcedureCodeUplinkRANStatusTransfer:
		// 	log.Printf("[INFO] Handling Uplink RAN Status Transfer from GnB")
		// 	return gnb.handleUplinkRANStatusTransfer(initiatingMessage)
		// case ngapType.ProcedureCodeUplinkUEAssociatedNRPPaTransport:
		// 	log.Printf("[INFO] Handling Uplink UE Associated NRPPa Transport from GnB")
		// 	return gnb.handleUplinkUEAssociatedNRPPaTransport(initiatingMessage)

		default:
			// Forward to AMF
			return gnb.forwardToAMF(msg)
		}
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		successfulOutcome := msg.SuccessfulOutcome
		if successfulOutcome == nil {
			return fmt.Errorf("SuccessfulOutcome is nil")
		}

		switch successfulOutcome.ProcedureCode.Value {
		// case ngapType.ProcedureCodeHandoverCancel:
		// 	log.Printf("[INFO] Handling Handover Cancel from GnB")
		// 	return gnb.handlerHandoverCancelAcknowledge(successfulOutcome)
		// case ngapType.ProcedureCodeHandoverPreparation:
		// 	log.Printf("[INFO] Handling Handover Preparation from GnB")
		// 	return gnb.HandoverCommand(successfulOutcome)
		// case ngapType.ProcedureCodeHandoverResourceAllocation:
		// 	log.Printf("[INFO] Handling Handover Resource Allocation from GnB")
		// 	return gnb.HandoverRequestAcknowledge(successfulOutcome)
		// case ngapType.ProcedureCodeInitialContextSetup:
		// 	log.Printf("[INFO] Handling Initial Context Setup from GnB")
		// 	return gnb.InitialContextSetupResponse(successfulOutcome)
		// case ngapType.ProcedureCodeNGReset:
		// 	log.Printf("[INFO] Handling NG Reset from GnB")
		// 	return gnb.NGResetAcknowledge(successfulOutcome)
		// case ngapType.ProcedureCodeNGSetup:
		// 	log.Printf("[INFO] Handling NG Setup from GnB")
		// 	return gnb.NGSetupResponse(successfulOutcome)
		// case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
		// 	log.Printf("[INFO] Handling PDU Session Resource Modify Indication from GnB")
		// 	return gnb.PDUSessionResourceModifyConfirm(successfulOutcome)
		// case ngapType.ProcedureCodePDUSessionResourceModify:
		// 	log.Printf("[INFO] Handling PDU Session Resource Modify from GnB")
		// 	return gnb.PDUSessionResourceModifyResponse(successfulOutcome)
		// case ngapType.ProcedureCodePDUSessionResourceRelease:
		// 	log.Printf("[INFO] Handling PDU Session Resource Release from GnB")
		// 	return gnb.PDUSessionResourceReleaseResponse(successfulOutcome)
		// case ngapType.ProcedureCodePDUSessionResourceSetup:
		// 	log.Printf("[INFO] Handling PDU Session Resource Setup from GnB")
		// 	return gnb.PDUSessionResourceSetupResponse(successfulOutcome)
		// case ngapType.ProcedureCodeUEContextModification:
		// 	log.Printf("[INFO] Handling UE Context Modification from GnB")
		// 	return gnb.UEContextModificationResponse(successfulOutcome)
		// case ngapType.ProcedureCodeUEContextRelease:
		// 	log.Printf("[INFO] Handling UE Context Release from GnB")
		// 	return gnb.UEContextReleaseComplete(successfulOutcome)
		// case ngapType.ProcedureCodeUERadioCapabilityCheck:
		// 	log.Printf("[INFO] Handling UE Radio Capability Check from GnB")
		// 	return gnb.UERadioCapabilityCheckResponse(successfulOutcome)
		default:
			// Forward to AMF
			return gnb.forwardToAMF(msg)
		}
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		unsuccessfulOutcome := msg.UnsuccessfulOutcome
		if unsuccessfulOutcome == nil {
			return fmt.Errorf("UnsuccessfulOutcome is nil")
		}

		switch unsuccessfulOutcome.ProcedureCode.Value {
		// case ngapType.ProcedureCodeHandoverResourceAllocation:
		// 	log.Printf("[INFO] Handling Handover Resource Allocation from GnB")
		// 	return gnb.HandoverFailure(unsuccessfulOutcome)
		// case ngapType.ProcedureCodeHandoverPreparation:
		// 	log.Printf("[INFO] Handling Handover Preparation from GnB")
		// 	return gnb.HandoverPreparationFailure(unsuccessfulOutcome)
		// case ngapType.ProcedureCodeInitialContextSetup:
		// 	log.Printf("[INFO] Handling Initial Context Setup from GnB")
		// 	return gnb.InitialContextSetupFailure(unsuccessfulOutcome)
		// case ngapType.ProcedureCodeNGSetup:
		// 	log.Printf("[INFO] Handling NG Setup from GnB")
		// 	return gnb.NGSetupFailure(unsuccessfulOutcome)
		// case ngapType.ProcedureCodeUEContextModification:
		// 	log.Printf("[INFO] Handling UE Context Modification from GnB")
		// 	return gnb.UEContextModificationFailure(unsuccessfulOutcome)
		default:
			// Forward to AMF
			return gnb.forwardToAMF(msg)
		}
	default:
		// Forward other message types to AMF
		return gnb.forwardToAMF(msg)
	}
}

// func (gnb *Gnb) UEContextModificationFailure(unsuccessfulOutcome *ngapType.UnsuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) NGSetupFailure(unsuccessfulOutcome *ngapType.UnsuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) InitialContextSetupFailure(unsuccessfulOutcome *ngapType.UnsuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) HandoverPreparationFailure(unsuccessfulOutcome *ngapType.UnsuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) HandoverFailure(unsuccessfulOutcome *ngapType.UnsuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) PDUSessionResourceReleaseResponse(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) PDUSessionResourceSetupResponse(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) UEContextModificationResponse(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) UEContextReleaseComplete(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) UERadioCapabilityCheckResponse(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) PDUSessionResourceModifyResponse(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) PDUSessionResourceModifyConfirm(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) NGSetupResponse(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) NGResetAcknowledge(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) InitialContextSetupResponse(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) HandoverRequestAcknowledge(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) HandoverCommand(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handlerHandoverCancelAcknowledge(successfulOutcome *ngapType.SuccessfulOutcome) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleUplinkUEAssociatedNRPPaTransport(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleUplinkRANStatusTransfer(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleUplinkNASTransport(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleUETNLABindingReleaseRequest(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleUEContextReleaseRequest(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleUEContextReleaseCommand(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleUEContextModificationRequest(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleTraceStart(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleTraceFailureIndication(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleSecondaryRATDataUsageReport(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleRerouteNASRequest(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handlePDUSessionResourceSetup(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handlePDUSessionResourceRelease(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handlePDUSessionResourceNotify(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handlePDUSessionResourceModifyRequest(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handlePDUSessionResourceModifyIndication(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleNASNonDeliveryIndication(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleLocationReportingFailureIndication(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleLocationReportingControl(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleLocationReport(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleInitialContextSetupRequest(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleHandoverPreparation(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleHandoverResourceAllocation(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleHandoverNotification(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleHandoverCancel(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleErrorIndication(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleDownlinkRANStatusTransfer(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleDownlinkNASTransport(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleDeactivateTrace(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleCellTrafficTrace(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleAMFStatusIndication(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleAMFConfigurationUpdate(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

// func (gnb *Gnb) handleNGReset(initiatingMessage *ngapType.InitiatingMessage) error {
// 	panic("unimplemented")
// }

func (gnb *Gnb) handleNGSetupRequest(initiatingMessage *ngapType.InitiatingMessage) error {
	// Send response locally
	pkt, err := buildNGSetupResponse()
	if err != nil {
		return fmt.Errorf("build NGSetupResponse failed: %v", err)
	}

	if err := gnb.conn.sendNgap(pkt); err != nil {
		return fmt.Errorf("send NGSetupResponse failed: %v", err)
	}

	log.Printf("[INFO] Sent NGSetupResponse to GnB")
	return nil
}

func (gnb *Gnb) handleInitialUEMessage(initiatingMessage *ngapType.InitiatingMessage) error {
	// Extract GNB UE ID from message (simplified)
	gnbUeId := int64(1) // TODO: extract from actual message

	// Pick AMF
	amf := GetAmfManager().pick()
	if amf == nil {
		return fmt.Errorf("no available AMF")
	}

	log.Printf("[INFO] Forwarding InitialUEMessage to AMF: %s", amf.id)

	// Create UE context
	_service.createUeContext(amf, gnb, gnbUeId)

	// Forward message to AMF with modifications
	pdu := ngapType.NGAPPDU{
		Present:           ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: initiatingMessage,
	}
	encoded, err := ngap.Encoder(pdu)
	if err != nil {
		return fmt.Errorf("encode message failed: %v", err)
	}

	return amf.conn.sendNgap(encoded)
}

func (gnb *Gnb) forwardToAMF(msg *NgapMessage) error {
	amf := GetAmfManager().pick()
	if amf == nil {
		return fmt.Errorf("no available AMF")
	}

	pdu, err := ngap.Encoder(*msg)
	if err != nil {
		return fmt.Errorf("encode message failed: %v", err)
	}

	return amf.conn.sendNgap(pdu)
}

func (gnb *Gnb) Close() {
	if gnb.conn != nil {
		gnb.conn.Close()
	}
}

// /////////////// AMF
type AMFInfo struct {
	PodName      string
	NodeIP       string
	InternalPort int32
	NodePort     int32
}

type AmfManager struct {
	amfList     map[string]*Amf
	indexesById map[string]*Amf
	mutex       sync.Mutex
	clientset   *kubernetes.Clientset
	namespace   string
	minikubeIP  string
	done        chan struct{}
}

func newAmfManager() *AmfManager {
	// Get minikube IP
	minikubeIP, err := getMinikubeIP()
	if err != nil {
		log.Printf("[ERROR] Failed to get minikube IP: %v", err)
		minikubeIP = "127.0.0.1" // fallback
	}

	// Initialize Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig
		kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Printf("[ERROR] Failed to get kubeconfig: %v", err)
		}
	}

	var clientset *kubernetes.Clientset
	if config != nil {
		clientset, err = kubernetes.NewForConfig(config)
		if err != nil {
			log.Printf("[ERROR] Failed to create k8s client: %v", err)
		}
	}

	return &AmfManager{
		amfList:     make(map[string]*Amf),
		indexesById: make(map[string]*Amf),
		clientset:   clientset,
		namespace:   "free5gc", // default namespace
		minikubeIP:  minikubeIP,
		done:        make(chan struct{}),
	}
}

// Monitor K8s events, connect to AMFs/delete AMFs
func (m *AmfManager) monitorLoop(wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.refreshAMFConnections()
		}
	}
}

func (m *AmfManager) refreshAMFConnections() {
	if m.clientset == nil {
		return
	}

	amfs, err := m.getAMFPodsAndPorts()
	if err != nil {
		log.Printf("[ERROR] Failed to get AMF pods: %v", err)
		return
	}
	// Print current AMFs
	// log.Printf("[INFO] Found %d AMFs", len(amfs))

	// Connect to new AMFs
	for _, amfInfo := range amfs {
		// log.Printf("[INFO] AMF ID: %+v, AMF Internal Port: %+v, AMF Node Port: %+v", amfInfo.PodName, amfInfo.InternalPort, amfInfo.NodePort)
		key := fmt.Sprintf("%s:%d", amfInfo.PodName, amfInfo.NodePort)
		if _, exists := m.amfList[key]; !exists {
			go m.connectAmf(amfInfo)
		}
	}
}

func (m *AmfManager) getAMFPodsAndPorts() ([]AMFInfo, error) {
	var amfs []AMFInfo

	// List pods with label "nf=amf"
	pods, err := m.clientset.CoreV1().Pods(m.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "nf=amf",
	})
	if err != nil {
		return nil, err
	}

	// List services with label "nf=amf"
	svcs, err := m.clientset.CoreV1().Services(m.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "nf=amf",
	})
	if err != nil {
		return nil, err
	}

	// Map pod name to node IP
	podNodeIP := make(map[string]string)
	for _, pod := range pods.Items {
		podNodeIP[pod.Name] = pod.Status.HostIP
	}

	// Map services to AMF info
	for _, svc := range svcs.Items {
		for _, port := range svc.Spec.Ports {
			if port.Name == "sctp" || port.Protocol == v1.ProtocolSCTP {
				for _, pod := range pods.Items {
					if pod.Labels["nf"] == "amf" {
						amfs = append(amfs, AMFInfo{
							PodName:      pod.Name,
							NodeIP:       podNodeIP[pod.Name],
							InternalPort: port.TargetPort.IntVal,
							NodePort:     port.NodePort,
						})
					}
				}
			}
		}
	}
	return amfs, nil
}

// Add an AMF
func (m *AmfManager) add(amf *Amf) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	key := fmt.Sprintf("%s:%d", amf.podName, amf.nodePort)
	m.amfList[key] = amf
	m.indexesById[amf.id] = amf
	log.Printf("[INFO] Added AMF to manager: %s", amf.id)
}

func (m *AmfManager) remove(id string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if amf, ok := m.indexesById[id]; ok {
		amf.Close()
		key := fmt.Sprintf("%s:%d", amf.podName, amf.nodePort)
		delete(m.amfList, key)
		delete(m.indexesById, id)
		log.Printf("[INFO] Removed AMF: %s", id)
	}
}

// Pick an AMF for load balancing
func (m *AmfManager) pick() *Amf {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Simple round-robin selection
	// for _, amf := range m.amfList {
	// 	return amf // Return first available
	// }
	// return nil

	// Find AMF with least load (minimum number of UE contexts)
	if len(m.amfList) == 0 {
		return nil
	}

	var selectedAmf *Amf
	minLoad := int64(-1)

	for _, amf := range m.amfList {
		amf.mutex.Lock()
		currentLoad := int64(len(amf.ueList))
		amf.mutex.Unlock()

		// Select AMF with minimum load
		if minLoad == -1 || currentLoad < minLoad {
			minLoad = currentLoad
			selectedAmf = amf
		}
	}

	return selectedAmf
}

// Connect to AMF, send NGSetupRequest then start goroutine to read data
func (m *AmfManager) connectAmf(amfInfo AMFInfo) error {
	// Check if already connecting/connected to this AMF
	key := fmt.Sprintf("%s:%d", amfInfo.PodName, amfInfo.NodePort)
	m.mutex.Lock()
	if _, exists := m.amfList[key]; exists {
		m.mutex.Unlock()
		return nil // Already connected
	}
	// Add placeholder to prevent duplicate connections
	m.amfList[key] = nil
	m.mutex.Unlock()

	addr := fmt.Sprintf("%s:%d", m.minikubeIP, amfInfo.NodePort)
	raddr, err := sctp.ResolveSCTPAddr("sctp", addr)
	if err != nil {
		// Remove placeholder on error
		m.mutex.Lock()
		delete(m.amfList, key)
		m.mutex.Unlock()
		return fmt.Errorf("ResolveSCTPAddr error for %s: %v", addr, err)
	}

	// Assign a unique local port for each AMF connection using a generated value
	// Use a base port plus a hash of PodName to avoid collisions
	basePort := 40000
	hash := 0
	for _, c := range amfInfo.PodName {
		hash += int(c)
	}
	laddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("192.168.0.4")}},
		Port:    basePort + (hash % 1000), // Ensure port is unique per PodName
	}
	conn, err := sctp.DialSCTP("sctp", laddr, raddr)
	if err != nil {
		// Remove placeholder on error
		m.mutex.Lock()
		delete(m.amfList, key)
		m.mutex.Unlock()
		return fmt.Errorf("SCTP dial error for %s: %v", addr, err)
	}

	// Create AMF
	amf := newAmf(amfInfo)

	// Create SCTP connection wrapper
	ngapConn := newNgapConn(conn, amf.handle)
	amf.conn = ngapConn

	// Update manager with actual AMF instance
	m.mutex.Lock()
	m.amfList[key] = amf
	m.indexesById[amf.id] = amf
	m.mutex.Unlock()

	// Send setup request
	if err := amf.sendSetupRequest(); err != nil {
		// Cleanup on setup failure
		m.mutex.Lock()
		delete(m.amfList, key)
		delete(m.indexesById, amf.id)
		m.mutex.Unlock()
		conn.Close()
		return fmt.Errorf("send setup request failed: %v", err)
	}

	// Listen to NGAP messages from AMF
	_service.wg.Add(1)
	go ngapConn.readLoop(&_service.wg)

	log.Printf("[INFO] Connected to AMF: %s", addr)
	return nil
}

func (m *AmfManager) Close() {
	close(m.done)
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, amf := range m.amfList {
		amf.Close()
	}
}

type Amf struct {
	conn     *NgapConn
	podName  string
	nodePort int32
	id       string
	ueList   map[int64]*UeContext
	mutex    sync.Mutex
}

func newAmf(amfInfo AMFInfo) *Amf {
	return &Amf{
		podName:  amfInfo.PodName,
		nodePort: amfInfo.NodePort,
		id:       amfInfo.PodName,
		ueList:   make(map[int64]*UeContext),
	}
}

func (amf *Amf) sendSetupRequest() error {
	pkt, err := buildNGSetupRequest()
	if err != nil {
		return err
	}

	info := &sctp.SndRcvInfo{PPID: 60}
	sctpConn := amf.conn.conn.(*sctp.SCTPConn)
	_, err = sctpConn.SCTPWrite(pkt, info)
	return err
}

// Handle received NGAP message
func (amf *Amf) handle(msg *NgapMessage) error {
	switch msg.Present {
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		successfulOutcome := msg.SuccessfulOutcome
		if successfulOutcome == nil {
			return fmt.Errorf("SuccessfulOutcome is nil")
		}

		switch successfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			log.Printf("[INFO] Received NGSetupResponse from AMF")
			GetAmfManager().add(amf)
			return nil

		}

	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := msg.InitiatingMessage
		if initiatingMessage == nil {
			return fmt.Errorf("InitiatingMessage is nil")
		}

		switch initiatingMessage.ProcedureCode.Value {
		case ngapType.ProcedureCodeInitialContextSetup:
			return amf.handleInitialContextSetup(initiatingMessage)

		}

	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		unsuccessfulOutcome := msg.UnsuccessfulOutcome
		if unsuccessfulOutcome == nil {
			return fmt.Errorf("UnsuccessfulOutcome is nil")
		}

		switch unsuccessfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeInitialContextSetup:
			log.Printf("[INFO] Handling Initial Context Setup from GnB")
			return amf.InitialContextSetupFailure(unsuccessfulOutcome)
		case ngapType.ProcedureCodeNGSetup:
			log.Printf("[INFO] Handling NG Setup from GnB")
			return amf.NGSetupFailure(unsuccessfulOutcome)
		}
	}

	// Forward other messages to GnB
	return amf.forwardToGnB(msg)
}

func (amf *Amf) NGSetupFailure(unsuccessfulOutcome *ngapType.UnsuccessfulOutcome) error {
	panic("unimplemented")
}

func (amf *Amf) InitialContextSetupFailure(unsuccessfulOutcome *ngapType.UnsuccessfulOutcome) error {
	panic("unimplemented")
}

func (amf *Amf) handleInitialContextSetup(initiatingMessage *ngapType.InitiatingMessage) error {
	// Extract AMF UE ID and LB UE ID from message (simplified)
	amfUeId := int64(1) // TODO: extract from actual message
	lbUeId := int64(1)  // TODO: extract from actual message

	// Look for UE context
	ueCtx := FindUeCtx(lbUeId)
	if ueCtx == nil {
		return fmt.Errorf("UE context not found for lbUeId: %d", lbUeId)
	}

	ueCtx.setAmfUeId(amfUeId)
	amf.add(ueCtx)

	// Forward message to GnB
	pdu := ngapType.NGAPPDU{
		Present:           ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: initiatingMessage,
	}
	encoded, err := ngap.Encoder(pdu)
	if err != nil {
		return fmt.Errorf("encode message failed: %v", err)
	}

	return ueCtx.gnb.conn.sendNgap(encoded)
}

func (amf *Amf) forwardToGnB(msg *NgapMessage) error {
	// Find appropriate GnB and forward
	// For simplicity, broadcast to all GnBs
	gnbManager := GetGnbManager()
	gnbManager.mutex.Lock()
	defer gnbManager.mutex.Unlock()

	pdu, err := ngap.Encoder(*msg)
	if err != nil {
		return fmt.Errorf("encode message failed: %v", err)
	}

	for _, gnb := range gnbManager.gnbList {
		if err := gnb.conn.sendNgap(pdu); err != nil {
			log.Printf("[ERROR] Forward to GnB failed: %v", err)
		}
	}

	return nil
}

// Add a new UeContext
func (amf *Amf) add(ueCtx *UeContext) {
	amf.mutex.Lock()
	defer amf.mutex.Unlock()
	amf.ueList[ueCtx.amfUeId] = ueCtx
}

func (amf *Amf) Close() {
	if amf.conn != nil {
		amf.conn.Close()
	}
}

// ////////////////// UE
type UeContext struct {
	amf     *Amf
	amfUeId int64
	gnb     *Gnb
	gnbUeId int64
	lbId    int64
}

func (ue *UeContext) setAmfUeId(id int64) {
	ue.amfUeId = id
}

// ////////////////// Service
type Service struct {
	amfMan  *AmfManager
	gnbMan  *GnbManager
	sctpSrv *NgapServer
	wg      sync.WaitGroup
	lbUeId  int64
	ueList  map[int64]*UeContext
	mutex   sync.Mutex
}

var _service *Service

// Initialize singleton Service instance
func initService() {
	if _service == nil {
		_service = &Service{
			sctpSrv: newNgapServer(),
			amfMan:  newAmfManager(),
			gnbMan:  newGnbManager(),
			ueList:  make(map[int64]*UeContext),
		}
	}
}

// Create new UeContext, allocate LbUeId, then add to the UeContext list
func (s *Service) createUeContext(amf *Amf, gnb *Gnb, ranUeId int64) *UeContext {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	ueCtx := &UeContext{
		amf:     amf,
		gnb:     gnb,
		gnbUeId: ranUeId,
		lbId:    s.lbUeId,
	}

	// Add to global UeContext list
	s.ueList[ueCtx.lbId] = ueCtx

	// Add to UeContext list at GnB
	gnb.add(ueCtx)

	// Increment counter
	s.lbUeId++

	return ueCtx
}

// Find UeContext given LbUeId
func (s *Service) findUeCtx(lbUeId int64) *UeContext {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	ueCtx, _ := s.ueList[lbUeId]
	return ueCtx
}

// Stop all goroutines and release resources
func (s *Service) Kill() {
	s.sctpSrv.Close()
	s.amfMan.Close()
	s.gnbMan.Close()

	s.wg.Wait()
}

// Utility functions
func getMinikubeIP() (string, error) {
	out, err := exec.Command("minikube", "ip").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func buildNGSetupRequest() ([]byte, error) {
	// RAN configuration
	proxyRan := &struct {
		Name            string
		RanId           *models.GlobalRanNodeId
		SupportedTAList []struct {
			Tai        models.Tai
			SNssaiList []models.Snssai
		}
		DefaultPagingDRX string
	}{
		Name: "ProxyRan",
		RanId: &models.GlobalRanNodeId{
			PlmnId: &models.PlmnId{
				Mcc: "208",
				Mnc: "93",
			},
			GNbId: &models.GNbId{
				BitLength: 32,
				GNBValue:  "\x00\x00\x00\x01",
			},
		},
		SupportedTAList: []struct {
			Tai        models.Tai
			SNssaiList []models.Snssai
		}{
			{
				Tai: models.Tai{
					PlmnId: &models.PlmnId{
						Mcc: "208",
						Mnc: "93",
					},
					Tac: "000001",
				},
				SNssaiList: []models.Snssai{
					{
						Sst: 1,
						Sd:  "010203",
					},
				},
			},
		},
		DefaultPagingDRX: "v128",
	}

	var pdu ngapType.NGAPPDU
	pdu.Present = ngapType.NGAPPDUPresentInitiatingMessage
	pdu.InitiatingMessage = new(ngapType.InitiatingMessage)

	InitiatingMessage := pdu.InitiatingMessage
	InitiatingMessage.ProcedureCode.Value = ngapType.ProcedureCodeNGSetup
	InitiatingMessage.Criticality.Value = ngapType.CriticalityPresentReject
	InitiatingMessage.Value.Present = ngapType.InitiatingMessagePresentNGSetupRequest
	InitiatingMessage.Value.NGSetupRequest = new(ngapType.NGSetupRequest)

	nGSetupRequest := InitiatingMessage.Value.NGSetupRequest
	nGSetupRequestIEs := &nGSetupRequest.ProtocolIEs

	// 1. GlobalRANNodeID
	ie := ngapType.NGSetupRequestIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDGlobalRANNodeID
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	ie.Value.Present = ngapType.NGSetupRequestIEsPresentGlobalRANNodeID
	ie.Value.GlobalRANNodeID = new(ngapType.GlobalRANNodeID)

	globalRANNodeID := ie.Value.GlobalRANNodeID
	globalRANNodeID.Present = ngapType.GlobalRANNodeIDPresentGlobalGNBID
	globalRANNodeID.GlobalGNBID = new(ngapType.GlobalGNBID)

	globalRANNodeID.GlobalGNBID.PLMNIdentity = ngapConvert.PlmnIdToNgap(*proxyRan.RanId.PlmnId)

	globalRANNodeID.GlobalGNBID.GNBID.Present = ngapType.GNBIDPresentGNBID
	globalRANNodeID.GlobalGNBID.GNBID.GNBID = &aper.BitString{
		Bytes:     []byte(proxyRan.RanId.GNbId.GNBValue),
		BitLength: uint64(proxyRan.RanId.GNbId.BitLength),
	}

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie)

	// 2. RAN Node Name
	ie2 := ngapType.NGSetupRequestIEs{}
	ie2.Id.Value = ngapType.ProtocolIEIDRANNodeName
	ie2.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie2.Value.Present = ngapType.NGSetupRequestIEsPresentRANNodeName
	ie2.Value.RANNodeName = new(ngapType.RANNodeName)
	ie2.Value.RANNodeName.Value = proxyRan.Name

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie2)

	// 3. SupportedTAList
	ie3 := ngapType.NGSetupRequestIEs{}
	ie3.Id.Value = ngapType.ProtocolIEIDSupportedTAList
	ie3.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie3.Value.Present = ngapType.NGSetupRequestIEsPresentSupportedTAList
	ie3.Value.SupportedTAList = new(ngapType.SupportedTAList)
	supportedTAList := ie3.Value.SupportedTAList

	for _, tai := range proxyRan.SupportedTAList {
		taiIE := ngapType.SupportedTAItem{}

		tacBytes, err := hex.DecodeString(tai.Tai.Tac)
		if err != nil {
			return nil, fmt.Errorf("invalid TAC format: %v", err)
		}

		if len(tacBytes) < 3 {
			paddedTac := make([]byte, 3)
			copy(paddedTac[3-len(tacBytes):], tacBytes)
			tacBytes = paddedTac
		}

		taiIE.TAC = ngapType.TAC{
			Value: aper.OctetString(tacBytes),
		}

		taiIE.BroadcastPLMNList.List = make([]ngapType.BroadcastPLMNItem, 1)
		taiIE.BroadcastPLMNList.List[0] = ngapType.BroadcastPLMNItem{
			PLMNIdentity: ngapConvert.PlmnIdToNgap(*tai.Tai.PlmnId),
			TAISliceSupportList: ngapType.SliceSupportList{
				List: make([]ngapType.SliceSupportItem, 0, len(tai.SNssaiList)),
			},
		}

		for _, snssai := range tai.SNssaiList {
			sliceSupportItem := ngapType.SliceSupportItem{
				SNSSAI: ngapType.SNSSAI{
					SST: ngapType.SST{
						Value: aper.OctetString{byte(snssai.Sst)},
					},
				},
			}

			if snssai.Sd != "" {
				sdBytes, err := hex.DecodeString(snssai.Sd)
				if err != nil {
					return nil, fmt.Errorf("invalid SD format: %v", err)
				}
				if len(sdBytes) < 3 {
					paddedSd := make([]byte, 3)
					copy(paddedSd[3-len(sdBytes):], sdBytes)
					sdBytes = paddedSd
				}

				sliceSupportItem.SNSSAI.SD = &ngapType.SD{
					Value: aper.OctetString(sdBytes),
				}
			}

			taiIE.BroadcastPLMNList.List[0].TAISliceSupportList.List = append(
				taiIE.BroadcastPLMNList.List[0].TAISliceSupportList.List,
				sliceSupportItem,
			)
		}
		supportedTAList.List = append(supportedTAList.List, taiIE)
	}

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie3)

	// 4. DefaultPagingDRX
	ie4 := ngapType.NGSetupRequestIEs{}
	ie4.Id.Value = ngapType.ProtocolIEIDDefaultPagingDRX
	ie4.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie4.Value.Present = ngapType.NGSetupRequestIEsPresentDefaultPagingDRX
	ie4.Value.DefaultPagingDRX = new(ngapType.PagingDRX)
	ie4.Value.DefaultPagingDRX.Value = ngapType.PagingDRXPresentV128

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie4)

	return ngap.Encoder(pdu)
}

func buildNGSetupResponse() ([]byte, error) {
	// AMF configuration
	proxySelf := &struct {
		Name             string
		ServedGUAMIList  []models.Guami
		RelativeCapacity int64
		PlmnSupportList  []struct {
			PlmnId     *models.PlmnId
			SNssaiList []models.Snssai
		}
	}{
		Name: "AMF000",
		ServedGUAMIList: []models.Guami{
			{
				PlmnId: &models.PlmnIdNid{
					Mcc: "208",
					Mnc: "93",
				},
				AmfId: "000000",
			},
		},
		RelativeCapacity: 255,
		PlmnSupportList: []struct {
			PlmnId     *models.PlmnId
			SNssaiList []models.Snssai
		}{
			{
				PlmnId: &models.PlmnId{
					Mcc: "208",
					Mnc: "93",
				},
				SNssaiList: []models.Snssai{
					{
						Sst: 1,
						Sd:  "010203",
					},
				},
			},
		},
	}

	var pdu ngapType.NGAPPDU
	pdu.Present = ngapType.NGAPPDUPresentSuccessfulOutcome
	pdu.SuccessfulOutcome = new(ngapType.SuccessfulOutcome)

	successfulOutcome := pdu.SuccessfulOutcome
	successfulOutcome.ProcedureCode.Value = ngapType.ProcedureCodeNGSetup
	successfulOutcome.Criticality.Value = ngapType.CriticalityPresentReject
	successfulOutcome.Value.Present = ngapType.SuccessfulOutcomePresentNGSetupResponse
	successfulOutcome.Value.NGSetupResponse = new(ngapType.NGSetupResponse)

	nGSetupResponse := successfulOutcome.Value.NGSetupResponse
	nGSetupResponseIEs := &nGSetupResponse.ProtocolIEs

	// AMFName
	ie := ngapType.NGSetupResponseIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDAMFName
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	ie.Value.Present = ngapType.NGSetupResponseIEsPresentAMFName
	ie.Value.AMFName = new(ngapType.AMFName)
	ie.Value.AMFName.Value = proxySelf.Name

	nGSetupResponseIEs.List = append(nGSetupResponseIEs.List, ie)

	// ServedGUAMIList
	ie = ngapType.NGSetupResponseIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDServedGUAMIList
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	ie.Value.Present = ngapType.NGSetupResponseIEsPresentServedGUAMIList
	ie.Value.ServedGUAMIList = new(ngapType.ServedGUAMIList)

	servedGUAMIList := ie.Value.ServedGUAMIList
	for _, guami := range proxySelf.ServedGUAMIList {
		servedGUAMIItem := ngapType.ServedGUAMIItem{}
		// Convert PlmnIdNid to PlmnId
		plmnId := models.PlmnId{
			Mcc: guami.PlmnId.Mcc,
			Mnc: guami.PlmnId.Mnc,
		}
		servedGUAMIItem.GUAMI.PLMNIdentity = ngapConvert.PlmnIdToNgap(plmnId)
		regionId, setId, prtId := ngapConvert.AmfIdToNgap(guami.AmfId)
		servedGUAMIItem.GUAMI.AMFRegionID.Value = regionId
		servedGUAMIItem.GUAMI.AMFSetID.Value = setId
		servedGUAMIItem.GUAMI.AMFPointer.Value = prtId
		servedGUAMIList.List = append(servedGUAMIList.List, servedGUAMIItem)
	}

	nGSetupResponseIEs.List = append(nGSetupResponseIEs.List, ie)

	// RelativeAMFCapacity
	ie = ngapType.NGSetupResponseIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDRelativeAMFCapacity
	ie.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie.Value.Present = ngapType.NGSetupResponseIEsPresentRelativeAMFCapacity
	ie.Value.RelativeAMFCapacity = new(ngapType.RelativeAMFCapacity)
	ie.Value.RelativeAMFCapacity.Value = proxySelf.RelativeCapacity

	nGSetupResponseIEs.List = append(nGSetupResponseIEs.List, ie)

	// PLMNSupportList
	ie = ngapType.NGSetupResponseIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDPLMNSupportList
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	ie.Value.Present = ngapType.NGSetupResponseIEsPresentPLMNSupportList
	ie.Value.PLMNSupportList = new(ngapType.PLMNSupportList)

	pLMNSupportList := ie.Value.PLMNSupportList
	for _, plmnItem := range proxySelf.PlmnSupportList {
		pLMNSupportItem := ngapType.PLMNSupportItem{}
		pLMNSupportItem.PLMNIdentity = ngapConvert.PlmnIdToNgap(*plmnItem.PlmnId)
		for _, snssai := range plmnItem.SNssaiList {
			sliceSupportItem := ngapType.SliceSupportItem{}
			sliceSupportItem.SNSSAI = ngapConvert.SNssaiToNgap(snssai)
			pLMNSupportItem.SliceSupportList.List = append(pLMNSupportItem.SliceSupportList.List, sliceSupportItem)
		}
		pLMNSupportList.List = append(pLMNSupportList.List, pLMNSupportItem)
	}

	nGSetupResponseIEs.List = append(nGSetupResponseIEs.List, ie)

	return ngap.Encoder(pdu)
}

// Global accessors
func GetAmfManager() *AmfManager {
	return _service.amfMan
}

func GetGnbManager() *GnbManager {
	return _service.gnbMan
}

func FindUeCtx(lbUeId int64) *UeContext {
	return _service.findUeCtx(lbUeId)
}

func main() {
	// Initialize logger
	logger.NgapLog.Logger.SetLevel(logrus.InfoLevel)

	// 1. Read configuration file (TODO)

	// 2. Create a singleton service instance which is globally accessible
	initService()

	_service.wg.Add(2)

	// 3. Start SCTP server on a goroutine
	go _service.sctpSrv.listenLoop(&_service.wg)

	// 4. Start AMF status listening goroutine
	go _service.amfMan.monitorLoop(&_service.wg)

	// 5. Listen to keyboard interruption
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("[INFO] Open5GLoS proxy server started")
	log.Println("[INFO] Press Ctrl+C to stop...")

	// Wait for signal
	<-sigChan
	log.Println("[INFO] Shutting down...")

	// Graceful shutdown
	_service.Kill()
	log.Println("[INFO] Server stopped")
}
