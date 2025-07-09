package ngap

import (
	"encoding/hex"
	"fmt"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap/ngapType"
)

// MessageInfo contains parsed NGAP message information
type MessageInfo struct {
	Type            string
	ProcedureCode   int64
	Criticality     string
	Direction       string
	RANUENGAPID     *int64
	AMFUENGAPID     *int64
	PLMNID          string
	GlobalRANNodeID string
}

// Parser handles NGAP message parsing
type Parser struct{}

// NewParser creates a new NGAP parser
func NewParser() *Parser {
	return &Parser{}
}

// ParseMessage parses an NGAP message and extracts key information
func (p *Parser) ParseMessage(data []byte, direction string) (*MessageInfo, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty NGAP message")
	}

	// Decode NGAP PDU
	pdu := &ngapType.NGAPPDU{}
	err := aper.UnmarshalWithParams(data, pdu, "valueExt,valueLB:0,valueUB:2")
	if err != nil {
		return nil, fmt.Errorf("failed to decode NGAP PDU: %w", err)
	}

	info := &MessageInfo{
		Direction: direction,
	}

	// Parse based on PDU type
	switch pdu.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		return p.parseInitiatingMessage(pdu.InitiatingMessage, info)
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		return p.parseSuccessfulOutcome(pdu.SuccessfulOutcome, info)
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		return p.parseUnsuccessfulOutcome(pdu.UnsuccessfulOutcome, info)
	default:
		return nil, fmt.Errorf("unknown NGAP PDU type: %d", pdu.Present)
	}
}

// parseInitiatingMessage parses initiating messages
func (p *Parser) parseInitiatingMessage(msg *ngapType.InitiatingMessage, info *MessageInfo) (*MessageInfo, error) {
	info.ProcedureCode = msg.ProcedureCode.Value
	info.Criticality = getCriticalityString(int64(msg.Criticality.Value))
	info.Type = getProcedureName(msg.ProcedureCode.Value)

	switch msg.ProcedureCode.Value {
	case 21: // NGSetupRequest
		return p.parseNGSetupRequest(msg, info)
	case 15: // InitialUEMessage
		return p.parseInitialUEMessage(msg, info)
	case 46: // UplinkNASTransport
		return p.parseUplinkNASTransport(msg, info)
	case 4: // DownlinkNASTransport
		return p.parseDownlinkNASTransport(msg, info)
	case 14: // InitialContextSetupRequest
		return p.parseInitialContextSetupRequest(msg, info)
	case 41: // UEContextReleaseRequest
		return p.parseUEContextReleaseRequest(msg, info)
	default:
		// For unknown messages, just return basic info
		return info, nil
	}
}

// parseSuccessfulOutcome parses successful outcome messages
func (p *Parser) parseSuccessfulOutcome(msg *ngapType.SuccessfulOutcome, info *MessageInfo) (*MessageInfo, error) {
	info.ProcedureCode = msg.ProcedureCode.Value
	info.Criticality = getCriticalityString(int64(msg.Criticality.Value))
	info.Type = getProcedureName(msg.ProcedureCode.Value) + "Response"

	switch msg.ProcedureCode.Value {
	case 21: // NGSetupRequest
		return p.parseNGSetupResponse(msg, info)
	case 14: // InitialContextSetupRequest
		return p.parseInitialContextSetupResponse(msg, info)
	default:
		return info, nil
	}
}

// parseUnsuccessfulOutcome parses unsuccessful outcome messages
func (p *Parser) parseUnsuccessfulOutcome(msg *ngapType.UnsuccessfulOutcome, info *MessageInfo) (*MessageInfo, error) {
	info.ProcedureCode = msg.ProcedureCode.Value
	info.Criticality = getCriticalityString(int64(msg.Criticality.Value))
	info.Type = getProcedureName(msg.ProcedureCode.Value) + "Failure"

	return info, nil
}

// parseNGSetupRequest parses NG Setup Request
func (p *Parser) parseNGSetupRequest(msg *ngapType.InitiatingMessage, info *MessageInfo) (*MessageInfo, error) {
	if msg.Value.NGSetupRequest == nil {
		return info, nil
	}

	for _, ie := range msg.Value.NGSetupRequest.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDGlobalRANNodeID:
			if ie.Value.GlobalRANNodeID != nil {
				info.GlobalRANNodeID = p.extractGlobalRANNodeID(ie.Value.GlobalRANNodeID)
			}
		}
	}

	return info, nil
}

// parseInitialUEMessage parses Initial UE Message
func (p *Parser) parseInitialUEMessage(msg *ngapType.InitiatingMessage, info *MessageInfo) (*MessageInfo, error) {
	if msg.Value.InitialUEMessage == nil {
		return info, nil
	}

	for _, ie := range msg.Value.InitialUEMessage.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				val := ie.Value.RANUENGAPID.Value
				info.RANUENGAPID = &val
			}
		case ngapType.ProtocolIEIDUserLocationInformation:
			if ie.Value.UserLocationInformation != nil {
				info.PLMNID = p.extractPLMNID(ie.Value.UserLocationInformation)
			}
		}
	}

	return info, nil
}

// parseUplinkNASTransport parses Uplink NAS Transport
func (p *Parser) parseUplinkNASTransport(msg *ngapType.InitiatingMessage, info *MessageInfo) (*MessageInfo, error) {
	if msg.Value.UplinkNASTransport == nil {
		return info, nil
	}

	for _, ie := range msg.Value.UplinkNASTransport.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				val := ie.Value.RANUENGAPID.Value
				info.RANUENGAPID = &val
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				val := ie.Value.AMFUENGAPID.Value
				info.AMFUENGAPID = &val
			}
		}
	}

	return info, nil
}

// parseDownlinkNASTransport parses Downlink NAS Transport
func (p *Parser) parseDownlinkNASTransport(msg *ngapType.InitiatingMessage, info *MessageInfo) (*MessageInfo, error) {
	if msg.Value.DownlinkNASTransport == nil {
		return info, nil
	}

	for _, ie := range msg.Value.DownlinkNASTransport.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				val := ie.Value.RANUENGAPID.Value
				info.RANUENGAPID = &val
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				val := ie.Value.AMFUENGAPID.Value
				info.AMFUENGAPID = &val
			}
		}
	}

	return info, nil
}

// parseInitialContextSetupRequest parses Initial Context Setup Request
func (p *Parser) parseInitialContextSetupRequest(msg *ngapType.InitiatingMessage, info *MessageInfo) (*MessageInfo, error) {
	if msg.Value.InitialContextSetupRequest == nil {
		return info, nil
	}

	for _, ie := range msg.Value.InitialContextSetupRequest.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				val := ie.Value.RANUENGAPID.Value
				info.RANUENGAPID = &val
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				val := ie.Value.AMFUENGAPID.Value
				info.AMFUENGAPID = &val
			}
		}
	}

	return info, nil
}

// parseUEContextReleaseRequest parses UE Context Release Request
func (p *Parser) parseUEContextReleaseRequest(msg *ngapType.InitiatingMessage, info *MessageInfo) (*MessageInfo, error) {
	if msg.Value.UEContextReleaseRequest == nil {
		return info, nil
	}

	for _, ie := range msg.Value.UEContextReleaseRequest.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDUENGAPIDs:
			// Extract UE NGAP IDs - need to check actual field name in the library
			// This may need adjustment based on the actual ngapType structure
			if ie.Value.UENGAPIDs != nil {
				p.extractUENGAPIDs(ie.Value.UENGAPIDs, info)
			}
		}
	}

	return info, nil
}

// parseNGSetupResponse parses NG Setup Response
func (p *Parser) parseNGSetupResponse(msg *ngapType.SuccessfulOutcome, info *MessageInfo) (*MessageInfo, error) {
	// NG Setup Response doesn't contain UE-specific information
	return info, nil
}

// parseInitialContextSetupResponse parses Initial Context Setup Response
func (p *Parser) parseInitialContextSetupResponse(msg *ngapType.SuccessfulOutcome, info *MessageInfo) (*MessageInfo, error) {
	if msg.Value.InitialContextSetupResponse == nil {
		return info, nil
	}

	for _, ie := range msg.Value.InitialContextSetupResponse.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				val := ie.Value.RANUENGAPID.Value
				info.RANUENGAPID = &val
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				val := ie.Value.AMFUENGAPID.Value
				info.AMFUENGAPID = &val
			}
		}
	}

	return info, nil
}

// Helper functions

// extractGlobalRANNodeID extracts Global RAN Node ID
func (p *Parser) extractGlobalRANNodeID(globalRANNodeID *ngapType.GlobalRANNodeID) string {
	switch globalRANNodeID.Present {
	case ngapType.GlobalRANNodeIDPresentGlobalGNBID:
		if globalRANNodeID.GlobalGNBID != nil {
			// Handle BitString properly
			if globalRANNodeID.GlobalGNBID.GNBID.GNBID != nil {
				return fmt.Sprintf("gNB-%s", hex.EncodeToString(globalRANNodeID.GlobalGNBID.GNBID.GNBID.Bytes))
			}
		}
	case ngapType.GlobalRANNodeIDPresentGlobalNgENBID:
		if globalRANNodeID.GlobalNgENBID != nil && globalRANNodeID.GlobalNgENBID.NgENBID != nil {
			return fmt.Sprintf("NgENB-%s", hex.EncodeToString(globalRANNodeID.GlobalNgENBID.NgENBID.Bytes))
		}
	}
	return "unknown"
}

// extractPLMNID extracts PLMN ID from User Location Information
func (p *Parser) extractPLMNID(userLocationInfo *ngapType.UserLocationInformation) string {
	switch userLocationInfo.Present {
	case ngapType.UserLocationInformationPresentUserLocationInformationEUTRA:
		if userLocationInfo.UserLocationInformationEUTRA != nil &&
			userLocationInfo.UserLocationInformationEUTRA.EUTRACGI != nil {
			return hex.EncodeToString(userLocationInfo.UserLocationInformationEUTRA.EUTRACGI.PLMNIdentity.Value)
		}
	case ngapType.UserLocationInformationPresentUserLocationInformationNR:
		if userLocationInfo.UserLocationInformationNR != nil &&
			userLocationInfo.UserLocationInformationNR.NRCGI != nil {
			return hex.EncodeToString(userLocationInfo.UserLocationInformationNR.NRCGI.PLMNIdentity.Value)
		}
	}
	return ""
}

// extractUENGAPIDs extracts UE NGAP IDs
func (p *Parser) extractUENGAPIDs(ueNGAPIDs *ngapType.UENGAPIDs, info *MessageInfo) {
	switch ueNGAPIDs.Present {
	case ngapType.UENGAPIDsPresentUENGAPIDPair:
		if ueNGAPIDs.UENGAPIDPair != nil {
			if ueNGAPIDs.UENGAPIDPair.RANUENGAPID != nil {
				val := ueNGAPIDs.UENGAPIDPair.RANUENGAPID.Value
				info.RANUENGAPID = &val
			}
			if ueNGAPIDs.UENGAPIDPair.AMFUENGAPID != nil {
				val := ueNGAPIDs.UENGAPIDPair.AMFUENGAPID.Value
				info.AMFUENGAPID = &val
			}
		}
	case ngapType.UENGAPIDsPresentAMFUENGAPID:
		if ueNGAPIDs.AMFUENGAPID != nil {
			val := ueNGAPIDs.AMFUENGAPID.Value
			info.AMFUENGAPID = &val
		}
	}
}

// getCriticalityString converts criticality value to string
func getCriticalityString(criticality int64) string {
	switch criticality {
	case 0: // reject
		return "reject"
	case 1: // ignore
		return "ignore"
	case 2: // notify
		return "notify"
	default:
		return "unknown"
	}
}

// getProcedureName returns the procedure name for a given code
func getProcedureName(code int64) string {
	switch code {
	case 21:
		return "NGSetupRequest"
	case 15:
		return "InitialUEMessage"
	case 4:
		return "DownlinkNASTransport"
	case 46:
		return "UplinkNASTransport"
	case 14:
		return "InitialContextSetupRequest"
	case 41:
		return "UEContextReleaseRequest"
	case 42:
		return "UEContextReleaseCommand"
	case 29:
		return "PDUSessionResourceSetupRequest"
	case 34:
		return "PDUSessionResourceModifyRequest"
	case 27:
		return "PDUSessionResourceReleaseCommand"
	default:
		return fmt.Sprintf("Unknown(%d)", code)
	}
}

// GetUEIdentifier returns a unique identifier for the UE from the message info
func (info *MessageInfo) GetUEIdentifier() string {
	if info.RANUENGAPID != nil && info.AMFUENGAPID != nil {
		return fmt.Sprintf("ue-%d-%d", *info.RANUENGAPID, *info.AMFUENGAPID)
	}
	if info.RANUENGAPID != nil {
		return fmt.Sprintf("ue-ran-%d", *info.RANUENGAPID)
	}
	if info.AMFUENGAPID != nil {
		return fmt.Sprintf("ue-amf-%d", *info.AMFUENGAPID)
	}
	return ""
}

// IsUESpecific returns true if the message is UE-specific
func (info *MessageInfo) IsUESpecific() bool {
	return info.RANUENGAPID != nil || info.AMFUENGAPID != nil
}
