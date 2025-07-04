/*
Package ngap provides NGAP message parsing functionality.
*/
package ngap

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
)

// TS 38.412
const PPID uint32 = 0x3c000000

// Decoder is to decode raw data to NGAP pdu pointer with PER Aligned
func Decoder(b []byte) (pdu *ngapType.NGAPPDU, err error) {
	pdu = &ngapType.NGAPPDU{}

	err = aper.UnmarshalWithParams(b, pdu, "valueExt,valueLB:0,valueUB:2")
	return
}

// Encoder is to NGAP pdu to raw data with PER Aligned
func Encoder(pdu ngapType.NGAPPDU) ([]byte, error) {
	return aper.MarshalWithParams(pdu, "valueExt,valueLB:0,valueUB:2")
}

func PrintResult(v reflect.Value, layer int) string {
	fieldType := v.Type()
	if v.Kind() == reflect.Ptr {
		return "&" + PrintResult(v.Elem(), layer)
	}
	switch fieldType {
	case aper.OctetStringType:
		return fmt.Sprintf("OctetString (0x%x)[%d]\n", v.Bytes(), len(v.Bytes()))

	case aper.BitStringType:
		return fmt.Sprintf("BitString (%0.8b)[%d]\n", v.Field(0).Bytes(), v.Field(1).Uint())

	case aper.EnumeratedType:
		return fmt.Sprintf("Enumerated(%d)\n", v.Uint())
	}

	var s string
	switch v.Kind() {
	case reflect.Struct:
		structType := fieldType
		s += "{\n"
		end := strings.Repeat(" ", layer) + "}\n"
		layer += 2
		space := strings.Repeat(" ", layer)
		if structType.Field(0).Name == "Present" {
			present := v.Field(0).Int()
			s += (space + fmt.Sprintf("Present: %d\n", present))
			s += (space + fmt.Sprintf("%s : ", structType.Field(int(present)).Name))
			s += PrintResult(v.Field(int(present)), layer)
			s += end
			return s
		}
		for i := 0; i < v.NumField(); i++ {
			// optional
			if v.Field(i).Type().Kind() == reflect.Ptr && v.Field(i).IsNil() {
				continue
			}

			s += (space + fmt.Sprintf("%s : ", structType.Field(i).Name))
			s += PrintResult(v.Field(i), layer)
		}
		s += end
	case reflect.Slice:
		s += fmt.Sprintf("[%d]{\n", v.Len())
		end := strings.Repeat(" ", layer) + "}\n"
		layer += 2
		space := strings.Repeat(" ", layer)
		for i := 0; i < v.Len(); i++ {
			s += space
			s += PrintResult(v.Index(i), layer)
			s += (space + ",\n")
		}
		s += end
	case reflect.String:
		s = fmt.Sprintf("PrintableString(\"%s\")\n", v.String())
	case reflect.Int32, reflect.Int64:
		s = fmt.Sprintf("INTEGER(%d)\n", v.Int())
	default:
		fmt.Printf("Type: %s does not handle", v.Type())
	}
	return s
}

// ParseInitialMessage parses the initial NGAP message to extract gNB and UE IDs
func ParseInitialMessage(data []byte) (gnbID, ueID string, err error) {
	pdu, err := ngap.Decoder(data)
	if err != nil {
		return "", "", fmt.Errorf("failed to decode NGAP PDU: %w", err)
	}

	switch pdu.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		if pdu.InitiatingMessage == nil {
			return "", "", fmt.Errorf("initiating message is nil")
		}

		switch pdu.InitiatingMessage.ProcedureCode.Value {
		case ngapType.ProcedureCodeInitialUEMessage:
			return parseInitialUEMessage(pdu.InitiatingMessage)
		case ngapType.ProcedureCodeNGSetupRequest:
			return parseNGSetupRequest(pdu.InitiatingMessage)
		default:
			return "", "", fmt.Errorf("unsupported procedure code: %d", pdu.InitiatingMessage.ProcedureCode.Value)
		}
	default:
		return "", "", fmt.Errorf("unsupported NGAP PDU type: %d", pdu.Present)
	}
}

// GetMessageType returns the message type of an NGAP PDU
func GetMessageType(data []byte) (string, error) {
	pdu, err := ngap.Decoder(data)
	if err != nil {
		return "", fmt.Errorf("failed to decode NGAP PDU: %w", err)
	}

	switch pdu.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		if pdu.InitiatingMessage == nil {
			return "unknown", nil
		}
		return getProcedureName(pdu.InitiatingMessage.ProcedureCode.Value), nil
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		if pdu.SuccessfulOutcome == nil {
			return "unknown", nil
		}
		return getProcedureName(pdu.SuccessfulOutcome.ProcedureCode.Value) + "Response", nil
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		if pdu.UnsuccessfulOutcome == nil {
			return "unknown", nil
		}
		return getProcedureName(pdu.UnsuccessfulOutcome.ProcedureCode.Value) + "Failure", nil
	default:
		return "unknown", nil
	}
}

// parseInitialUEMessage parses InitialUEMessage to extract IDs
func parseInitialUEMessage(msg *ngapType.InitiatingMessage) (gnbID, ueID string, err error) {
	if msg.Value.InitialUEMessage == nil {
		return "", "", fmt.Errorf("InitialUEMessage is nil")
	}

	for _, ie := range msg.Value.InitialUEMessage.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				gnbID = fmt.Sprintf("gnb-%d", ie.Value.RANUENGAPID.Value)
			}
		case ngapType.ProtocolIEIDNASPDU:
			// Extract UE ID from NAS PDU if needed
			// For now, generate a placeholder
			ueID = "ue-unknown"
		case ngapType.ProtocolIEIDUserLocationInformation:
			// Could extract more specific location info
		}
	}

	if gnbID == "" {
		gnbID = "gnb-default"
	}
	if ueID == "" {
		ueID = "ue-default"
	}

	return gnbID, ueID, nil
}

// parseNGSetupRequest parses NGSetupRequest to extract gNB ID
func parseNGSetupRequest(msg *ngapType.InitiatingMessage) (gnbID, ueID string, err error) {
	if msg.Value.NGSetupRequest == nil {
		return "", "", fmt.Errorf("NGSetupRequest is nil")
	}

	for _, ie := range msg.Value.NGSetupRequest.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDGlobalRANNodeID:
			if ie.Value.GlobalRANNodeID != nil {
				// Extract gNB ID from GlobalRANNodeID
				gnbID = "gnb-setup"
			}
		}
	}

	if gnbID == "" {
		gnbID = "gnb-setup"
	}
	ueID = "" // No UE involved in NG Setup

	return gnbID, ueID, nil
}

// getProcedureName returns the procedure name for a given code
func getProcedureName(code int64) string {
	switch code {
	case ngapType.ProcedureCodeAMFConfigurationUpdate:
		return "AMFConfigurationUpdate"
	case ngapType.ProcedureCodeAMFStatusIndication:
		return "AMFStatusIndication"
	case ngapType.ProcedureCodeCellTrafficTrace:
		return "CellTrafficTrace"
	case ngapType.ProcedureCodeDeactivateTrace:
		return "DeactivateTrace"
	case ngapType.ProcedureCodeDownlinkNASTransport:
		return "DownlinkNASTransport"
	case ngapType.ProcedureCodeDownlinkNonUEAssociatedNRPPaTransport:
		return "DownlinkNonUEAssociatedNRPPaTransport"
	case ngapType.ProcedureCodeDownlinkRANConfigurationTransfer:
		return "DownlinkRANConfigurationTransfer"
	case ngapType.ProcedureCodeDownlinkRANStatusTransfer:
		return "DownlinkRANStatusTransfer"
	case ngapType.ProcedureCodeDownlinkUEAssociatedNRPPaTransport:
		return "DownlinkUEAssociatedNRPPaTransport"
	case ngapType.ProcedureCodeErrorIndication:
		return "ErrorIndication"
	case ngapType.ProcedureCodeHandoverCancel:
		return "HandoverCancel"
	case ngapType.ProcedureCodeHandoverNotification:
		return "HandoverNotification"
	case ngapType.ProcedureCodeHandoverPreparation:
		return "HandoverPreparation"
	case ngapType.ProcedureCodeHandoverResourceAllocation:
		return "HandoverResourceAllocation"
	case ngapType.ProcedureCodeInitialContextSetup:
		return "InitialContextSetup"
	case ngapType.ProcedureCodeInitialUEMessage:
		return "InitialUEMessage"
	case ngapType.ProcedureCodeLocationReportingControl:
		return "LocationReportingControl"
	case ngapType.ProcedureCodeLocationReportingFailureIndication:
		return "LocationReportingFailureIndication"
	case ngapType.ProcedureCodeLocationReport:
		return "LocationReport"
	case ngapType.ProcedureCodeNASNonDeliveryIndication:
		return "NASNonDeliveryIndication"
	case ngapType.ProcedureCodeNGReset:
		return "NGReset"
	case ngapType.ProcedureCodeNGSetup:
		return "NGSetup"
	case ngapType.ProcedureCodeOverloadStart:
		return "OverloadStart"
	case ngapType.ProcedureCodeOverloadStop:
		return "OverloadStop"
	case ngapType.ProcedureCodePaging:
		return "Paging"
	case ngapType.ProcedureCodePathSwitchRequest:
		return "PathSwitchRequest"
	case ngapType.ProcedureCodePDUSessionResourceModify:
		return "PDUSessionResourceModify"
	case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
		return "PDUSessionResourceModifyIndication"
	case ngapType.ProcedureCodePDUSessionResourceRelease:
		return "PDUSessionResourceRelease"
	case ngapType.ProcedureCodePDUSessionResourceSetup:
		return "PDUSessionResourceSetup"
	case ngapType.ProcedureCodePDUSessionResourceNotify:
		return "PDUSessionResourceNotify"
	case ngapType.ProcedureCodePrivateMessage:
		return "PrivateMessage"
	case ngapType.ProcedureCodePWSCancel:
		return "PWSCancel"
	case ngapType.ProcedureCodePWSFailureIndication:
		return "PWSFailureIndication"
	case ngapType.ProcedureCodePWSRestartIndication:
		return "PWSRestartIndication"
	case ngapType.ProcedureCodeRANConfigurationUpdate:
		return "RANConfigurationUpdate"
	case ngapType.ProcedureCodeRerouteNASRequest:
		return "RerouteNASRequest"
	case ngapType.ProcedureCodeRRCInactiveTransitionReport:
		return "RRCInactiveTransitionReport"
	case ngapType.ProcedureCodeSecondaryRATDataUsageReport:
		return "SecondaryRATDataUsageReport"
	case ngapType.ProcedureCodeTraceFailureIndication:
		return "TraceFailureIndication"
	case ngapType.ProcedureCodeTraceStart:
		return "TraceStart"
	case ngapType.ProcedureCodeUEContextModification:
		return "UEContextModification"
	case ngapType.ProcedureCodeUEContextRelease:
		return "UEContextRelease"
	case ngapType.ProcedureCodeUEContextReleaseRequest:
		return "UEContextReleaseRequest"
	case ngapType.ProcedureCodeUERadioCapabilityCheckRequest:
		return "UERadioCapabilityCheckRequest"
	case ngapType.ProcedureCodeUERadioCapabilityInfoIndication:
		return "UERadioCapabilityInfoIndication"
	case ngapType.ProcedureCodeUETNLABindingReleaseRequest:
		return "UETNLABindingReleaseRequest"
	case ngapType.ProcedureCodeUplinkNASTransport:
		return "UplinkNASTransport"
	case ngapType.ProcedureCodeUplinkNonUEAssociatedNRPPaTransport:
		return "UplinkNonUEAssociatedNRPPaTransport"
	case ngapType.ProcedureCodeUplinkRANConfigurationTransfer:
		return "UplinkRANConfigurationTransfer"
	case ngapType.ProcedureCodeUplinkRANStatusTransfer:
		return "UplinkRANStatusTransfer"
	case ngapType.ProcedureCodeUplinkUEAssociatedNRPPaTransport:
		return "UplinkUEAssociatedNRPPaTransport"
	case ngapType.ProcedureCodeWriteReplaceWarningRequest:
		return "WriteReplaceWarningRequest"
	default:
		return fmt.Sprintf("Unknown-%d", code)
	}
}
