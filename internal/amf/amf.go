package amf

import (
	"fmt"
	"log"
	"sync"

	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
	ngapconn "github.com/hasukiHT/5glos/internal/ngap"
	"github.com/hasukiHT/5glos/internal/ue"
	"github.com/hasukiHT/5glos/internal/utils"
	"github.com/ishidawataru/sctp"
)

type Amf struct {
	conn       *ngapconn.NgapConn
	podName    string
	nodePort   int32
	id         string
	ueList     map[int64]*ue.Context
	mutex      sync.Mutex
	gnbManager GnbManagerInterface // Add interface dependency
	manager    *Manager            // Reference to AMF manager
	service    ServiceInterface    // Add service interface dependency
	isHealthy  bool                // Health status of the AMF
}

// Add interface for Service to avoid circular dependency
type ServiceInterface interface {
	FindUeCtx(lbUeId int64) *ue.Context
}

// Add interface for GnB manager to avoid circular dependency
type GnbManagerInterface interface {
	GetGnBList() map[interface{}]GnbInterface
	FindGnBByUeContext(ueCtx *ue.Context) GnbInterface
}

type GnbInterface interface {
	SendNgap(pdu []byte) error
	GetConn() interface{}
	GetId() string
	GetName() string
}

func New(amfInfo AMFInfo) *Amf {
	return &Amf{
		podName:   amfInfo.PodName,
		nodePort:  amfInfo.NodePort,
		id:        amfInfo.PodName,
		ueList:    make(map[int64]*ue.Context),
		isHealthy: true, // Initialize as healthy
	}
}

// Add method to set GnB manager dependency
func (amf *Amf) SetGnbManager(gnbManager GnbManagerInterface) {
	amf.gnbManager = gnbManager
}

// Add method to set AMF manager dependency
func (amf *Amf) SetManager(manager *Manager) {
	amf.manager = manager
}

// Add method to set Service dependency
func (amf *Amf) SetService(service ServiceInterface) {
	amf.service = service
}

func (amf *Amf) SendSetupRequest() error {
	pkt, err := utils.BuildNGSetupRequest()
	if err != nil {
		return err
	}

	info := &sctp.SndRcvInfo{PPID: 60}
	sctpConn := amf.conn.GetConn().(*sctp.SCTPConn)
	_, err = sctpConn.SCTPWrite(pkt, info)
	return err
}

// Handle received NGAP message
func (amf *Amf) Handle(msg *ngapconn.NgapMessage) error {
	switch msg.Present {
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		successfulOutcome := msg.SuccessfulOutcome
		if successfulOutcome == nil {
			return fmt.Errorf("SuccessfulOutcome is nil")
		}

		switch successfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			log.Printf("[INFO] Received NGSetupResponse from AMF %s", amf.id)
			// Add this AMF to the manager when NGSetup is successful
			if amf.manager != nil {
				amf.manager.Add(amf)
				log.Printf("[INFO] AMF %s successfully registered with manager", amf.id)
			} else {
				log.Printf("[WARNING] AMF manager reference is nil, cannot register AMF %s", amf.id)
			}
		case ngapType.ProcedureCodeAMFConfigurationUpdate:
			handlerIgnoreMessage(successfulOutcome)
		case ngapType.ProcedureCodePWSCancel:
			handlerIgnoreMessage(successfulOutcome)
		case ngapType.ProcedureCodeRANConfigurationUpdate:
			handlerIgnoreMessage(successfulOutcome)
		case ngapType.ProcedureCodeWriteReplaceWarning:
			handlerIgnoreMessage(successfulOutcome)
		default:
			// Forward other messages to AMF
			return amf.forwardToGnB(msg)
		}
	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := msg.InitiatingMessage
		if initiatingMessage == nil {
			return fmt.Errorf("InitiatingMessage is nil")
		}

		switch initiatingMessage.ProcedureCode.Value {
		case ngapType.ProcedureCodeDownlinkNASTransport:
			log.Printf("[INFO] Handling Downlink NASTransport from AMF")
			return amf.handleDownlinkNASTransport(initiatingMessage)
		case ngapType.ProcedureCodeAMFConfigurationUpdate:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodeAMFStatusIndication:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodeDownlinkNonUEAssociatedNRPPaTransport:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodeDownlinkRANConfigurationTransfer:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodeOverloadStart:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodeOverloadStop:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodePWSCancel:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodePWSFailureIndication:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodePWSRestartIndication:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodePaging:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodeRANConfigurationUpdate:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodeUplinkNonUEAssociatedNRPPaTransport:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodeUplinkRANConfigurationTransfer:
			handlerIgnoreMessage(initiatingMessage)
		case ngapType.ProcedureCodeWriteReplaceWarning:
			handlerIgnoreMessage(initiatingMessage)
		default:
			// Forward other messages to AMF
			return amf.forwardToGnB(msg)
		}

	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		unsuccessfulOutcome := msg.UnsuccessfulOutcome
		if unsuccessfulOutcome == nil {
			return fmt.Errorf("UnsuccessfulOutcome is nil")
		}
		switch unsuccessfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeAMFConfigurationUpdate:
			handlerIgnoreMessage(unsuccessfulOutcome)
		case ngapType.ProcedureCodeRANConfigurationUpdate:
			handlerIgnoreMessage(unsuccessfulOutcome)
		default:
			// Forward other messages to AMF
			return amf.forwardToGnB(msg)
		}
	}

	// Forward other messages to GnB
	return amf.forwardToGnB(msg)
}

func handlerIgnoreMessage(msg interface{}) {
	// Log the ignored message type for debugging purposes
	switch v := msg.(type) {
	case *ngapType.InitiatingMessage:
		if v != nil {
			log.Printf("[INFO] Ignoring InitiatingMessage with ProcedureCode: %d", v.ProcedureCode.Value)
		}
	case *ngapType.SuccessfulOutcome:
		if v != nil {
			log.Printf("[INFO] Ignoring SuccessfulOutcome with ProcedureCode: %d", v.ProcedureCode.Value)
		}
	case *ngapType.UnsuccessfulOutcome:
		if v != nil {
			log.Printf("[INFO] Ignoring UnsuccessfulOutcome with ProcedureCode: %d", v.ProcedureCode.Value)
		}
	default:
		log.Printf("[INFO] Ignoring unknown message type: %T", msg)
	}
	// Intentionally ignore the message (no-op)
}

func (amf *Amf) handleDownlinkNASTransport(initiatingMessage *ngapType.InitiatingMessage) error {
	// Extract AMF UE ID and RAN UE ID from the actual message
	ranUeId, amfUeId, found := utils.ExtractUeIdsFromMessage(&ngapType.NGAPPDU{
		Present:           ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: initiatingMessage,
	})

	if !found {
		return fmt.Errorf("could not extract UE IDs from Downlink NAS Transport message")
	}

	log.Printf("[INFO] Extracted UE IDs from Downlink NAS Transport: AMF-UE-ID=%d, RAN-UE-ID=%d", amfUeId, ranUeId)

	// The RAN UE ID in this context is actually the LB ID assigned by the service
	lbUeId := ranUeId

	// Look for UE context using the LB UE ID
	if amf.service == nil {
		return fmt.Errorf("service reference is nil, cannot find UE context")
	}

	ueCtx := amf.service.FindUeCtx(lbUeId)
	if ueCtx == nil {
		return fmt.Errorf("UE context not found for lbUeId: %d", lbUeId)
	}

	// Set the AMF UE ID in the UE context
	ueCtx.SetAmfUeId(amfUeId)

	// Add UE context to AMF's mapping (indexed by AMF UE ID)
	amf.Add(ueCtx)

	log.Printf("[INFO] Added UE context to AMF mapping: AMF-UE-ID=%d, LB-UE-ID=%d, GNB-UE-ID=%d",
		amfUeId, lbUeId, ueCtx.GetGnbUeId())

	// Get the target GnB from the UE context
	targetGnb := amf.gnbManager.FindGnBByUeContext(ueCtx)
	if targetGnb == nil {
		return fmt.Errorf("could not find target GnB for UE context")
	}

	// Modify the message to use the original GnB UE ID
	pdu := &ngapType.NGAPPDU{
		Present:           ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: initiatingMessage,
	}

	// Replace RAN UE NGAP ID with the original GnB UE ID, keep AMF UE ID as 0 for gNB
	newRanUeId := ueCtx.GetGnbUeId()
	newAmfUeId := ueCtx.GetLbId()

	err := utils.ModifyUeIdsInMessage(pdu, newRanUeId, newAmfUeId)
	if err != nil {
		log.Printf("[WARN] Could not modify UE IDs in message, using original: %v", err)
	}

	// Encode and forward the message to the target GnB
	encoded, err := ngap.Encoder(*pdu)
	if err != nil {
		return fmt.Errorf("encode message failed: %v", err)
	}

	err = targetGnb.SendNgap(encoded)
	if err != nil {
		return fmt.Errorf("forward to gNB failed: %v", err)
	}

	log.Printf("[INFO] Successfully forwarded Downlink NAS Transport to gNB (ID: %s) with modified UE IDs (AMF-UE-ID: %d->%d, RAN-UE-ID: %d->%d)",
		targetGnb.GetId(), amfUeId, newAmfUeId, ranUeId, newRanUeId)

	return nil
}

func (amf *Amf) forwardToGnB(msg *ngapconn.NgapMessage) error {
	if amf.gnbManager == nil {
		log.Printf("[WARN] GnB manager not set, cannot forward message")
		return fmt.Errorf("GnB manager not set")
	}

	// Validate message structure
	if msg == nil {
		return fmt.Errorf("message is nil")
	}

	// Convert to NGAPPDU for processing
	ngapPdu := &ngapType.NGAPPDU{
		Present:             msg.Present,
		InitiatingMessage:   msg.InitiatingMessage,
		SuccessfulOutcome:   msg.SuccessfulOutcome,
		UnsuccessfulOutcome: msg.UnsuccessfulOutcome,
	}

	// Extract original UE IDs from the message
	originalRanUeId, originalAmfUeId, hasUeIds := utils.ExtractUeIdsFromMessage(ngapPdu)

	// Log debug info about UE ID extraction
	if hasUeIds {
		log.Printf("[DEBUG] Extracted UE IDs from message: RAN-UE-ID=%d, AMF-UE-ID=%d", originalRanUeId, originalAmfUeId)
	} else {
		log.Printf("[DEBUG] No UE IDs found in message, will broadcast to all gNBs")
		// For messages without UE IDs, just broadcast to all gNBs immediately
		return amf.broadcastMessageToAllGnBs(msg, hasUeIds, originalRanUeId, originalAmfUeId)
	}

	// Try to find specific GnB and UE context based on message content
	var targetGnb GnbInterface
	var ueCtx *ue.Context

	if hasUeIds {
		// Find UE context using AMF UE ID
		amf.mutex.Lock()
		ueCtx = amf.ueList[originalAmfUeId]
		amf.mutex.Unlock()

		if ueCtx != nil && amf.gnbManager != nil {
			// Find the GnB associated with this UE context
			targetGnb = amf.gnbManager.FindGnBByUeContext(ueCtx)
		}
	}

	if targetGnb != nil && ueCtx != nil {
		// Replace UE IDs with the appropriate mapping for the target gNB
		// Use the original gNB UE ID and remove AMF UE ID (set to 0 or keep original based on message type)
		newRanUeId := ueCtx.GetGnbUeId() // Use original gNB UE ID
		newAmfUeId := ueCtx.GetLbId()    // Use LB ID for AMF

		// Create a copy of the NGAPPDU for modification
		msgCopy := *ngapPdu
		if err := utils.ModifyUeIdsInMessage(&msgCopy, newRanUeId, newAmfUeId); err != nil {
			log.Printf("[ERROR] Failed to modify UE IDs in message: %v", err)
			// Continue with original message if modification fails
			msgCopy = *ngapPdu
		}

		log.Printf("[DEBUG] Replace AMF NGAP UE ID %d to LB NGAP UE ID %d", originalRanUeId, newAmfUeId)
		log.Printf("[DEBUG] Replace LB NGAP UE ID %d to RAN NGAP UE ID %d", originalAmfUeId, newRanUeId)
		// Encode the modified message
		pdu, err := ngap.Encoder(msgCopy)
		if err != nil {
			return fmt.Errorf("encode message failed: %v", err)
		}

		// Forward to specific GnB
		if err := targetGnb.SendNgap(pdu); err != nil {
			log.Printf("[ERROR] Forward to specific GnB (ID: %s, Name: %s) failed: %v",
				targetGnb.GetId(), targetGnb.GetName(), err)
			return err
		}

		log.Printf("[INFO] Successfully forwarded NGAP message to gNB (ID: %s, Name: %s) for UE (AMF-UE-ID: %d->%d, RAN-UE-ID: %d->%d) - UE context mapping found",
			targetGnb.GetId(), targetGnb.GetName(), originalAmfUeId, newAmfUeId, originalRanUeId, newRanUeId)
		return nil
	}

	// If no specific gNB found, broadcast to all gNBs with original message
	return amf.broadcastMessageToAllGnBs(msg, hasUeIds, originalRanUeId, originalAmfUeId)
}

// broadcastMessageToAllGnBs broadcasts a message to all available gNBs
func (amf *Amf) broadcastMessageToAllGnBs(msg *ngapconn.NgapMessage, hasUeIds bool, originalRanUeId, originalAmfUeId int64) error {
	if amf.gnbManager == nil {
		return fmt.Errorf("GnB manager is nil, cannot broadcast to gNBs")
	}

	gnbList := amf.gnbManager.GetGnBList()
	if len(gnbList) == 0 {
		return fmt.Errorf("no gNBs available for forwarding")
	}

	// Encode the original message for broadcasting
	ngapPdu := &ngapType.NGAPPDU{
		Present:             msg.Present,
		InitiatingMessage:   msg.InitiatingMessage,
		SuccessfulOutcome:   msg.SuccessfulOutcome,
		UnsuccessfulOutcome: msg.UnsuccessfulOutcome,
	}
	pdu, err := ngap.Encoder(*ngapPdu)
	if err != nil {
		return fmt.Errorf("encode message failed: %v", err)
	}

	var lastError error
	successCount := 0

	for _, gnb := range gnbList {
		if gnb == nil {
			log.Printf("[WARN] Encountered nil gNB in list, skipping")
			continue
		}
		if err := gnb.SendNgap(pdu); err != nil {
			log.Printf("[ERROR] Forward to gNB (ID: %s, Name: %s) failed: %v", gnb.GetId(), gnb.GetName(), err)
			lastError = err
		} else {
			successCount++
		}
	}

	if successCount == 0 {
		return fmt.Errorf("failed to forward to any gNB, last error: %v", lastError)
	}

	if hasUeIds {
		log.Printf("[INFO] Successfully broadcasted NGAP message with UE IDs (AMF-UE-ID: %d, RAN-UE-ID: %d) to %d GnB(s) - no specific mapping found",
			originalAmfUeId, originalRanUeId, successCount)
	} else {
		log.Printf("[INFO] Successfully broadcasted NGAP message to %d GnB(s) - no UE IDs in message", successCount)
	}
	return nil
}

// Add a new UeContext
func (amf *Amf) Add(ueCtx *ue.Context) {
	amf.mutex.Lock()
	defer amf.mutex.Unlock()
	amf.ueList[ueCtx.GetAmfUeId()] = ueCtx
}

func (amf *Amf) Close() {
	if amf.conn != nil {
		amf.conn.Close()
	}
}

// SendNgap sends NGAP message to AMF
func (amf *Amf) SendNgap(pdu []byte) error {
	if amf.conn == nil {
		return fmt.Errorf("AMF connection is nil")
	}
	return amf.conn.SendNgap(pdu)
}

// GetId returns the AMF identifier
func (amf *Amf) GetId() string {
	return amf.id
}
