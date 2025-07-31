package utils

import (
	"encoding/hex"
	"fmt"
	"log"
	"reflect"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapConvert"
	"github.com/free5gc/ngap/ngapType"
	"github.com/free5gc/openapi/models"
)

// Global proxy configurations for RAN and AMF
var ProxyRAN = &struct {
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

var ProxySelf = &struct {
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

// Helper functions for accessing proxy configurations

// GetProxyRANId returns the RAN Node ID from the global ProxyRAN configuration
func GetProxyRANId() *models.GlobalRanNodeId {
	return ProxyRAN.RanId
}

// GetProxyRANName returns the RAN Node name from the global ProxyRAN configuration
func GetProxyRANName() string {
	return ProxyRAN.Name
}

// GetProxyAMFName returns the AMF name from the global ProxySelf configuration
func GetProxyAMFName() string {
	return ProxySelf.Name
}

// GetProxyGUAMIList returns the served GUAMI list from the global ProxySelf configuration
func GetProxyGUAMIList() []models.Guami {
	return ProxySelf.ServedGUAMIList
}

// GetProxyAMFCapacity returns the relative AMF capacity from the global ProxySelf configuration
func GetProxyAMFCapacity() int64 {
	return ProxySelf.RelativeCapacity
}

// GetProxyPLMNId returns the first PLMN ID from the ProxyRAN configuration
func GetProxyPLMNId() *models.PlmnId {
	if ProxyRAN.RanId != nil && ProxyRAN.RanId.PlmnId != nil {
		return ProxyRAN.RanId.PlmnId
	}
	return nil
}

// GetProxyAMFId returns the first AMF ID from the ProxySelf configuration
func GetProxyAMFId() string {
	if len(ProxySelf.ServedGUAMIList) > 0 {
		return ProxySelf.ServedGUAMIList[0].AmfId
	}
	return ""
}

func BuildNGSetupRequest() ([]byte, error) {
	// Use global RAN configuration
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

	globalRANNodeID.GlobalGNBID.PLMNIdentity = ngapConvert.PlmnIdToNgap(*ProxyRAN.RanId.PlmnId)

	globalRANNodeID.GlobalGNBID.GNBID.Present = ngapType.GNBIDPresentGNBID
	globalRANNodeID.GlobalGNBID.GNBID.GNBID = &aper.BitString{
		Bytes:     []byte(ProxyRAN.RanId.GNbId.GNBValue),
		BitLength: uint64(ProxyRAN.RanId.GNbId.BitLength),
	}

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie)

	// 2. RAN Node Name
	ie2 := ngapType.NGSetupRequestIEs{}
	ie2.Id.Value = ngapType.ProtocolIEIDRANNodeName
	ie2.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie2.Value.Present = ngapType.NGSetupRequestIEsPresentRANNodeName
	ie2.Value.RANNodeName = new(ngapType.RANNodeName)
	ie2.Value.RANNodeName.Value = ProxyRAN.Name

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie2)

	// 3. SupportedTAList
	ie3 := ngapType.NGSetupRequestIEs{}
	ie3.Id.Value = ngapType.ProtocolIEIDSupportedTAList
	ie3.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie3.Value.Present = ngapType.NGSetupRequestIEsPresentSupportedTAList
	ie3.Value.SupportedTAList = new(ngapType.SupportedTAList)
	supportedTAList := ie3.Value.SupportedTAList

	for _, tai := range ProxyRAN.SupportedTAList {
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

func BuildNGSetupResponse() ([]byte, error) {
	// Use global AMF configuration
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
	ie.Value.AMFName.Value = ProxySelf.Name

	nGSetupResponseIEs.List = append(nGSetupResponseIEs.List, ie)

	// ServedGUAMIList
	ie = ngapType.NGSetupResponseIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDServedGUAMIList
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	ie.Value.Present = ngapType.NGSetupResponseIEsPresentServedGUAMIList
	ie.Value.ServedGUAMIList = new(ngapType.ServedGUAMIList)

	servedGUAMIList := ie.Value.ServedGUAMIList
	for _, guami := range ProxySelf.ServedGUAMIList {
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
	ie.Value.RelativeAMFCapacity.Value = ProxySelf.RelativeCapacity

	nGSetupResponseIEs.List = append(nGSetupResponseIEs.List, ie)

	// PLMNSupportList
	ie = ngapType.NGSetupResponseIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDPLMNSupportList
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	ie.Value.Present = ngapType.NGSetupResponseIEsPresentPLMNSupportList
	ie.Value.PLMNSupportList = new(ngapType.PLMNSupportList)

	pLMNSupportList := ie.Value.PLMNSupportList
	for _, plmnItem := range ProxySelf.PlmnSupportList {
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

// ExtractUeIdsFromMessage extracts RAN UE NGAP ID and AMF UE NGAP ID from NGAP message
func ExtractUeIdsFromMessage(msg *ngapType.NGAPPDU) (ranUeNgapId, amfUeNgapId int64, found bool) {
	switch msg.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		if msg.InitiatingMessage != nil {
			return extractUeIdsFromInitiatingMessage(msg.InitiatingMessage)
		}
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		if msg.SuccessfulOutcome != nil {
			return extractUeIdsFromSuccessfulOutcome(msg.SuccessfulOutcome)
		}
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		if msg.UnsuccessfulOutcome != nil {
			return extractUeIdsFromUnsuccessfulOutcome(msg.UnsuccessfulOutcome)
		}
	}
	return 0, 0, false
}

// ModifyUeIdsInMessage replaces UE IDs in NGAP message
func ModifyUeIdsInMessage(msg *ngapType.NGAPPDU, newRanUeNgapId, newAmfUeNgapId int64) error {
	switch msg.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		if msg.InitiatingMessage != nil {
			return modifyUeIdsInInitiatingMessage(msg.InitiatingMessage, newRanUeNgapId, newAmfUeNgapId)
		}
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		if msg.SuccessfulOutcome != nil {
			return modifyUeIdsInSuccessfulOutcome(msg.SuccessfulOutcome, newRanUeNgapId, newAmfUeNgapId)
		}
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		if msg.UnsuccessfulOutcome != nil {
			return modifyUeIdsInUnsuccessfulOutcome(msg.UnsuccessfulOutcome, newRanUeNgapId, newAmfUeNgapId)
		}
	}
	return fmt.Errorf("unsupported message type")
}

func extractUeIdsFromInitiatingMessage(msg *ngapType.InitiatingMessage) (ranUeNgapId, amfUeNgapId int64, found bool) {
	switch msg.ProcedureCode.Value {
	case ngapType.ProcedureCodeInitialUEMessage:
		return extractUeIdsFromInitialUEMessage(msg.Value.InitialUEMessage)
	case ngapType.ProcedureCodeUplinkNASTransport:
		return extractUeIdsFromUplinkNASTransport(msg.Value.UplinkNASTransport)
	case ngapType.ProcedureCodeDownlinkNASTransport:
		return extractUeIdsFromDownlinkNASTransport(msg.Value.DownlinkNASTransport)
	case ngapType.ProcedureCodeUEContextReleaseRequest:
		return extractUeIdsFromUEContextReleaseRequest(msg.Value.UEContextReleaseRequest)
	case ngapType.ProcedureCodePDUSessionResourceSetup:
		return extractUeIdsFromPDUSessionResourceSetup(msg.Value.PDUSessionResourceSetupRequest)
	case ngapType.ProcedureCodePDUSessionResourceRelease:
		return extractUeIdsFromPDUSessionResourceRelease(msg.Value.PDUSessionResourceReleaseCommand)
	case ngapType.ProcedureCodeInitialContextSetup:
		return extractUeIdsFromInitialContextSetup(msg.Value.InitialContextSetupRequest)
	case ngapType.ProcedureCodeCellTrafficTrace:
		return extractUeIdsFromCellTrafficTrace(msg.Value.CellTrafficTrace)
	case ngapType.ProcedureCodeDeactivateTrace:
		return extractUeIdsFromDeactivateTrace(msg.Value.DeactivateTrace)
	case ngapType.ProcedureCodeDownlinkRANStatusTransfer:
		return extractUeIdsFromDownlinkRANStatusTransfer(msg.Value.DownlinkRANStatusTransfer)
	case ngapType.ProcedureCodeHandoverCancel:
		return extractUeIdsFromHandoverCancel(msg.Value.HandoverCancel)
	case ngapType.ProcedureCodeHandoverNotification:
		return extractUeIdsFromHandoverNotification(msg.Value.HandoverNotify)
	case ngapType.ProcedureCodeHandoverResourceAllocation:
		return extractUeIdsFromHandoverResourceAllocation(msg.Value.HandoverRequest)
	case ngapType.ProcedureCodeHandoverPreparation:
		return extractUeIdsFromHandoverPreparation(msg.Value.HandoverRequired)
	case ngapType.ProcedureCodeLocationReport:
		return extractUeIdsFromLocationReport(msg.Value.LocationReport)
	case ngapType.ProcedureCodeLocationReportingControl:
		return extractUeIdsFromLocationReportingControl(msg.Value.LocationReportingControl)
	case ngapType.ProcedureCodeLocationReportingFailureIndication:
		return extractUeIdsFromLocationReportingFailureIndication(msg.Value.LocationReportingFailureIndication)
	case ngapType.ProcedureCodeNASNonDeliveryIndication:
		return extractUeIdsFromNASNonDeliveryIndication(msg.Value.NASNonDeliveryIndication)
	case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
		return extractUeIdsFromPDUSessionResourceModifyIndication(msg.Value.PDUSessionResourceModifyIndication)
	case ngapType.ProcedureCodePDUSessionResourceModify:
		return extractUeIdsFromPDUSessionResourceModify(msg.Value.PDUSessionResourceModifyRequest)
	case ngapType.ProcedureCodePDUSessionResourceNotify:
		return extractUeIdsFromPDUSessionResourceNotify(msg.Value.PDUSessionResourceNotify)
	case ngapType.ProcedureCodeRerouteNASRequest:
		return extractUeIdsFromRerouteNASRequest(msg.Value.RerouteNASRequest)
	case ngapType.ProcedureCodeSecondaryRATDataUsageReport:
		return extractUeIdsFromSecondaryRATDataUsageReport(msg.Value.SecondaryRATDataUsageReport)
	case ngapType.ProcedureCodeTraceFailureIndication:
		return extractUeIdsFromTraceFailureIndication(msg.Value.TraceFailureIndication)
	case ngapType.ProcedureCodeTraceStart:
		return extractUeIdsFromTraceStart(msg.Value.TraceStart)
	case ngapType.ProcedureCodeUEContextModification:
		return extractUeIdsFromUEContextModification(msg.Value.UEContextModificationRequest)
	case ngapType.ProcedureCodeUEContextRelease:
		return extractUeIdsFromUEContextRelease(msg.Value.UEContextReleaseCommand)
	case ngapType.ProcedureCodeUETNLABindingRelease:
		return extractUeIdsFromUETNLABindingRelease(msg.Value.UETNLABindingReleaseRequest)
	case ngapType.ProcedureCodeUplinkRANStatusTransfer:
		return extractUeIdsFromUplinkRANStatusTransfer(msg.Value.UplinkRANStatusTransfer)
	case ngapType.ProcedureCodeUplinkUEAssociatedNRPPaTransport:
		return extractUeIdsFromUplinkUEAssociatedNRPPaTransport(msg.Value.UplinkUEAssociatedNRPPaTransport)
	case ngapType.ProcedureCodePathSwitchRequest:
		return extractUeIdsFromPathSwitchRequest(msg.Value.PathSwitchRequest)
	case ngapType.ProcedureCodeUERadioCapabilityCheck:
		return extractUeIdsInUERadioCapabilityCheckRequest(msg.Value.UERadioCapabilityCheckRequest)
	case ngapType.ProcedureCodeDownlinkUEAssociatedNRPPaTransport:
		return extractUeIdsInDownlinkUEAssociatedNRPPaTransport(msg.Value.DownlinkUEAssociatedNRPPaTransport)
	case ngapType.ProcedureCodeRRCInactiveTransitionReport:
		return extractUeIdsInRRCInactiveTransitionReport(msg.Value.RRCInactiveTransitionReport)
	case ngapType.ProcedureCodeUERadioCapabilityInfoIndication:
		return extractUeIdsInUERadioCapabilityInfoIndication(msg.Value.UERadioCapabilityInfoIndication)
	// Add more cases as needed
	default:
		// log.Printf("[DEBUG] Unhandled initiating message procedure code: %d", msg.ProcedureCode.Value)
		return extractUeIdsFromGenericIEs(msg.Value)
	}
}

func extractUeIdsFromSuccessfulOutcome(msg *ngapType.SuccessfulOutcome) (ranUeNgapId, amfUeNgapId int64, found bool) {
	switch msg.ProcedureCode.Value {
	case ngapType.ProcedureCodeInitialContextSetup:
		return extractUeIdsFromInitialContextSetupResponse(msg.Value.InitialContextSetupResponse)
	case ngapType.ProcedureCodePDUSessionResourceSetup:
		return extractUeIdsFromPDUSessionResourceSetupResponse(msg.Value.PDUSessionResourceSetupResponse)
	case ngapType.ProcedureCodePDUSessionResourceRelease:
		return extractUeIdsFromPDUSessionResourceReleaseResponse(msg.Value.PDUSessionResourceReleaseResponse)
	case ngapType.ProcedureCodeUEContextRelease:
		return extractUeIdsFromUEContextReleaseComplete(msg.Value.UEContextReleaseComplete)
	case ngapType.ProcedureCodeHandoverCancel:
		return extractUeIdsInHandoverCancel(msg.Value.HandoverCancelAcknowledge)
	case ngapType.ProcedureCodeHandoverPreparation:
		return extractUeIdsInHandoverCommand(msg.Value.HandoverCommand)
	case ngapType.ProcedureCodeHandoverResourceAllocation:
		return extractUeIdsInHandoverRequestAcknowledge(msg.Value.HandoverRequestAcknowledge)
	case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
		return extractUeIdsInPDUSessionResourceModifyIndication(msg.Value.PDUSessionResourceModifyConfirm)
	case ngapType.ProcedureCodePDUSessionResourceModify:
		return extractUeIdsInPDUSessionResourceModifyResponse(msg.Value.PDUSessionResourceModifyResponse)
	case ngapType.ProcedureCodeUEContextModification:
		return extractUeIdsInUEContextModificationResponse(msg.Value.UEContextModificationResponse)
	case ngapType.ProcedureCodeUERadioCapabilityCheck:
		return extractUeIdsInUERadioCapabilityCheckResponse(msg.Value.UERadioCapabilityCheckResponse)
	case ngapType.ProcedureCodePathSwitchRequest:
		return extractUeIdsInPathSwitchRequestAcknowledge(msg.Value.PathSwitchRequestAcknowledge)
	// Add more cases as needed
	default:
		// log.Printf("[DEBUG] Unhandled successful outcome procedure code: %d", msg.ProcedureCode.Value)
		return extractUeIdsFromGenericIEs(msg.Value)
	}
}

func extractUeIdsFromUnsuccessfulOutcome(msg *ngapType.UnsuccessfulOutcome) (ranUeNgapId, amfUeNgapId int64, found bool) {
	switch msg.ProcedureCode.Value {
	case ngapType.ProcedureCodeInitialContextSetup:
		return extractUeIdsFromInitialContextSetupFailure(msg.Value.InitialContextSetupFailure)
	case ngapType.ProcedureCodeHandoverResourceAllocation:
		return extractUeIdsFromHandoverResourceAllocationFailure(msg.Value.HandoverFailure)
	case ngapType.ProcedureCodeHandoverPreparation:
		return extractUeIdsFromHandoverPreparationFailure(msg.Value.HandoverPreparationFailure)
	case ngapType.ProcedureCodeUEContextModification:
		return extractUeIdsFromUEContextModificationFailure(msg.Value.UEContextModificationFailure)
	case ngapType.ProcedureCodePathSwitchRequest:
		return extractUeIdsFromPathSwitchRequestFailure(msg.Value.PathSwitchRequestFailure)
	// Add more cases as needed for other unsuccessful outcome messages
	default:
		// log.Printf("[DEBUG] Unhandled unsuccessful outcome procedure code: %d", msg.ProcedureCode.Value)
		return extractUeIdsFromGenericIEs(msg.Value)
	}
}

// Helper functions for specific message types
func extractUeIdsFromInitialUEMessage(msg *ngapType.InitialUEMessage) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
			// InitialUEMessage doesn't contain AMF UE NGAP ID - it's assigned later by AMF
		}
	}
	return ranUeNgapId, 0, ranFound
}

func extractUeIdsFromUplinkNASTransport(msg *ngapType.UplinkNASTransport) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromDownlinkNASTransport(msg *ngapType.DownlinkNASTransport) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

// Additional extraction functions for other message types
func extractUeIdsFromUEContextReleaseRequest(msg *ngapType.UEContextReleaseRequest) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromPDUSessionResourceSetup(msg *ngapType.PDUSessionResourceSetupRequest) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromPDUSessionResourceRelease(msg *ngapType.PDUSessionResourceReleaseCommand) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromInitialContextSetup(msg *ngapType.InitialContextSetupRequest) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromCellTrafficTrace(msg *ngapType.CellTrafficTrace) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromDeactivateTrace(msg *ngapType.DeactivateTrace) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromDownlinkRANStatusTransfer(msg *ngapType.DownlinkRANStatusTransfer) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromHandoverCancel(msg *ngapType.HandoverCancel) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromHandoverNotification(msg *ngapType.HandoverNotify) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromHandoverResourceAllocation(msg *ngapType.HandoverRequest) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromHandoverPreparation(msg *ngapType.HandoverRequired) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromLocationReport(msg *ngapType.LocationReport) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromLocationReportingControl(msg *ngapType.LocationReportingControl) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromLocationReportingFailureIndication(msg *ngapType.LocationReportingFailureIndication) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromNASNonDeliveryIndication(msg *ngapType.NASNonDeliveryIndication) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromPDUSessionResourceModifyIndication(msg *ngapType.PDUSessionResourceModifyIndication) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromPDUSessionResourceModify(msg *ngapType.PDUSessionResourceModifyRequest) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromPDUSessionResourceNotify(msg *ngapType.PDUSessionResourceNotify) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromRerouteNASRequest(msg *ngapType.RerouteNASRequest) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromSecondaryRATDataUsageReport(msg *ngapType.SecondaryRATDataUsageReport) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromTraceFailureIndication(msg *ngapType.TraceFailureIndication) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromTraceStart(msg *ngapType.TraceStart) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromUEContextModification(msg *ngapType.UEContextModificationRequest) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromUEContextRelease(msg *ngapType.UEContextReleaseCommand) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.UENGAPIDs != nil {
				ranUeNgapId = ie.Value.UENGAPIDs.AMFUENGAPID.Value
				ranFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromUETNLABindingRelease(msg *ngapType.UETNLABindingReleaseRequest) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromUplinkRANStatusTransfer(msg *ngapType.UplinkRANStatusTransfer) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromUplinkUEAssociatedNRPPaTransport(msg *ngapType.UplinkUEAssociatedNRPPaTransport) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromPathSwitchRequest(msg *ngapType.PathSwitchRequest) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInUERadioCapabilityCheckRequest(msg *ngapType.UERadioCapabilityCheckRequest) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInDownlinkUEAssociatedNRPPaTransport(msg *ngapType.DownlinkUEAssociatedNRPPaTransport) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInRRCInactiveTransitionReport(msg *ngapType.RRCInactiveTransitionReport) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInUERadioCapabilityInfoIndication(msg *ngapType.UERadioCapabilityInfoIndication) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

// Response message extraction functions
func extractUeIdsFromInitialContextSetupResponse(msg *ngapType.InitialContextSetupResponse) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromPDUSessionResourceSetupResponse(msg *ngapType.PDUSessionResourceSetupResponse) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromPDUSessionResourceReleaseResponse(msg *ngapType.PDUSessionResourceReleaseResponse) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromUEContextReleaseComplete(msg *ngapType.UEContextReleaseComplete) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInHandoverCancel(msg *ngapType.HandoverCancelAcknowledge) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInHandoverCommand(msg *ngapType.HandoverCommand) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInHandoverRequestAcknowledge(msg *ngapType.HandoverRequestAcknowledge) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInPDUSessionResourceModifyIndication(msg *ngapType.PDUSessionResourceModifyConfirm) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInUEContextModificationResponse(msg *ngapType.UEContextModificationResponse) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInUERadioCapabilityCheckResponse(msg *ngapType.UERadioCapabilityCheckResponse) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInPDUSessionResourceModifyResponse(msg *ngapType.PDUSessionResourceModifyResponse) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsInPathSwitchRequestAcknowledge(msg *ngapType.PathSwitchRequestAcknowledge) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

// Failure message extraction functions
func extractUeIdsFromInitialContextSetupFailure(msg *ngapType.InitialContextSetupFailure) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromHandoverResourceAllocationFailure(msg *ngapType.HandoverFailure) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromHandoverPreparationFailure(msg *ngapType.HandoverPreparationFailure) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromUEContextModificationFailure(msg *ngapType.UEContextModificationFailure) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

func extractUeIdsFromPathSwitchRequestFailure(msg *ngapType.PathSwitchRequestFailure) (ranUeNgapId, amfUeNgapId int64, found bool) {
	if msg == nil {
		return 0, 0, false
	}

	var ranFound, amfFound bool
	for _, ie := range msg.ProtocolIEs.List {
		switch ie.Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID != nil {
				ranUeNgapId = ie.Value.RANUENGAPID.Value
				ranFound = true
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if ie.Value.AMFUENGAPID != nil {
				amfUeNgapId = ie.Value.AMFUENGAPID.Value
				amfFound = true
			}
		}
	}
	return ranUeNgapId, amfUeNgapId, ranFound || amfFound
}

// getProcedureCodeName returns the human-readable name for a procedure code
func getProcedureCodeName(procedureCode int64) string {
	switch procedureCode {
	case ngapType.ProcedureCodeAMFConfigurationUpdate:
		return "AMF Configuration Update"
	case ngapType.ProcedureCodeAMFStatusIndication:
		return "AMF Status Indication"
	case ngapType.ProcedureCodeCellTrafficTrace:
		return "Cell Traffic Trace"
	case ngapType.ProcedureCodeDeactivateTrace:
		return "Deactivate Trace"
	case ngapType.ProcedureCodeDownlinkNASTransport:
		return "Downlink NAS Transport"
	case ngapType.ProcedureCodeDownlinkNonUEAssociatedNRPPaTransport:
		return "Downlink Non-UE Associated NRPPa Transport"
	case ngapType.ProcedureCodeDownlinkRANConfigurationTransfer:
		return "Downlink RAN Configuration Transfer"
	case ngapType.ProcedureCodeDownlinkRANStatusTransfer:
		return "Downlink RAN Status Transfer"
	case ngapType.ProcedureCodeDownlinkUEAssociatedNRPPaTransport:
		return "Downlink UE Associated NRPPa Transport"
	case ngapType.ProcedureCodeErrorIndication:
		return "Error Indication"
	case ngapType.ProcedureCodeHandoverCancel:
		return "Handover Cancel"
	case ngapType.ProcedureCodeHandoverNotification:
		return "Handover Notification"
	case ngapType.ProcedureCodeHandoverPreparation:
		return "Handover Preparation"
	case ngapType.ProcedureCodeHandoverResourceAllocation:
		return "Handover Resource Allocation"
	case ngapType.ProcedureCodeInitialContextSetup:
		return "Initial Context Setup"
	case ngapType.ProcedureCodeInitialUEMessage:
		return "Initial UE Message"
	case ngapType.ProcedureCodeLocationReportingControl:
		return "Location Reporting Control"
	case ngapType.ProcedureCodeLocationReportingFailureIndication:
		return "Location Reporting Failure Indication"
	case ngapType.ProcedureCodeLocationReport:
		return "Location Report"
	case ngapType.ProcedureCodeNASNonDeliveryIndication:
		return "NAS Non-Delivery Indication"
	case ngapType.ProcedureCodeNGReset:
		return "NG Reset"
	case ngapType.ProcedureCodeNGSetup:
		return "NG Setup"
	case ngapType.ProcedureCodeOverloadStart:
		return "Overload Start"
	case ngapType.ProcedureCodeOverloadStop:
		return "Overload Stop"
	case ngapType.ProcedureCodePaging:
		return "Paging"
	case ngapType.ProcedureCodePathSwitchRequest:
		return "Path Switch Request"
	case ngapType.ProcedureCodePDUSessionResourceModify:
		return "PDU Session Resource Modify"
	case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
		return "PDU Session Resource Modify Indication"
	case ngapType.ProcedureCodePDUSessionResourceRelease:
		return "PDU Session Resource Release"
	case ngapType.ProcedureCodePDUSessionResourceSetup:
		return "PDU Session Resource Setup"
	case ngapType.ProcedureCodePDUSessionResourceNotify:
		return "PDU Session Resource Notify"
	case ngapType.ProcedureCodePrivateMessage:
		return "Private Message"
	case ngapType.ProcedureCodePWSCancel:
		return "PWS Cancel"
	case ngapType.ProcedureCodePWSFailureIndication:
		return "PWS Failure Indication"
	case ngapType.ProcedureCodePWSRestartIndication:
		return "PWS Restart Indication"
	case ngapType.ProcedureCodeRANConfigurationUpdate:
		return "RAN Configuration Update"
	case ngapType.ProcedureCodeRerouteNASRequest:
		return "Reroute NAS Request"
	case ngapType.ProcedureCodeRRCInactiveTransitionReport:
		return "RRC Inactive Transition Report"
	case ngapType.ProcedureCodeTraceFailureIndication:
		return "Trace Failure Indication"
	case ngapType.ProcedureCodeTraceStart:
		return "Trace Start"
	case ngapType.ProcedureCodeUEContextModification:
		return "UE Context Modification"
	case ngapType.ProcedureCodeUEContextRelease:
		return "UE Context Release"
	case ngapType.ProcedureCodeUEContextReleaseRequest:
		return "UE Context Release Request"
	case ngapType.ProcedureCodeUERadioCapabilityCheck:
		return "UE Radio Capability Check"
	case ngapType.ProcedureCodeUERadioCapabilityInfoIndication:
		return "UE Radio Capability Info Indication"
	case ngapType.ProcedureCodeUETNLABindingRelease:
		return "UE TNLA Binding Release"
	case ngapType.ProcedureCodeUplinkNASTransport:
		return "Uplink NAS Transport"
	case ngapType.ProcedureCodeUplinkNonUEAssociatedNRPPaTransport:
		return "Uplink Non-UE Associated NRPPa Transport"
	case ngapType.ProcedureCodeUplinkRANConfigurationTransfer:
		return "Uplink RAN Configuration Transfer"
	case ngapType.ProcedureCodeUplinkRANStatusTransfer:
		return "Uplink RAN Status Transfer"
	case ngapType.ProcedureCodeUplinkUEAssociatedNRPPaTransport:
		return "Uplink UE Associated NRPPa Transport"
	case ngapType.ProcedureCodeWriteReplaceWarning:
		return "Write-Replace Warning"
	case ngapType.ProcedureCodeSecondaryRATDataUsageReport:
		return "Secondary RAT Data Usage Report"
	default:
		return fmt.Sprintf("Unknown Procedure Code (%d)", procedureCode)
	}
}

// getProcedureCodeDetails returns detailed information about a procedure code and potential error conditions
func getProcedureCodeDetails(procedureCode int64) string {
	switch procedureCode {
	case ngapType.ProcedureCodeAMFConfigurationUpdate:
		return "Used to update AMF configuration parameters. Potential errors: Invalid configuration parameters, resource unavailable."
	case ngapType.ProcedureCodeAMFStatusIndication:
		return "Indicates AMF status changes. Potential errors: Communication failure, status inconsistency."
	case ngapType.ProcedureCodeCellTrafficTrace:
		return "Initiates cell traffic tracing for debugging. Potential errors: Trace session already active, insufficient resources."
	case ngapType.ProcedureCodeDeactivateTrace:
		return "Deactivates an active trace session. Potential errors: Trace session not found, deactivation failure."
	case ngapType.ProcedureCodeDownlinkNASTransport:
		return "Transports NAS messages from AMF to UE. Potential errors: UE context not found, NAS message delivery failure."
	case ngapType.ProcedureCodeDownlinkNonUEAssociatedNRPPaTransport:
		return "Transports non-UE associated NRPPa messages. Potential errors: Invalid destination, transport failure."
	case ngapType.ProcedureCodeDownlinkRANConfigurationTransfer:
		return "Transfers RAN configuration from AMF to gNB. Potential errors: Invalid configuration, application failure."
	case ngapType.ProcedureCodeDownlinkRANStatusTransfer:
		return "Transfers RAN status during handover. Potential errors: Context not found, transfer timeout."
	case ngapType.ProcedureCodeDownlinkUEAssociatedNRPPaTransport:
		return "Transports UE-associated NRPPa messages. Potential errors: UE context not found, positioning failure."
	case ngapType.ProcedureCodeErrorIndication:
		return "Indicates protocol errors or abnormal conditions. Common causes: Invalid message format, unknown procedure code, resource exhaustion."
	case ngapType.ProcedureCodeHandoverCancel:
		return "Cancels an ongoing handover procedure. Potential errors: Handover not in progress, cancellation too late."
	case ngapType.ProcedureCodeHandoverNotification:
		return "Notifies completion of handover. Potential errors: Context inconsistency, notification timeout."
	case ngapType.ProcedureCodeHandoverPreparation:
		return "Prepares resources for handover. Potential errors: Target cell unavailable, resource allocation failure, UE capability mismatch."
	case ngapType.ProcedureCodeHandoverResourceAllocation:
		return "Allocates resources at target gNB for handover. Potential errors: Insufficient resources, configuration mismatch, admission control failure."
	case ngapType.ProcedureCodeInitialContextSetup:
		return "Establishes initial UE context and security. Potential errors: Security setup failure, resource allocation failure, QoS configuration error."
	case ngapType.ProcedureCodeInitialUEMessage:
		return "First message from UE to establish connection. Potential errors: Authentication failure, invalid UE identity, network congestion."
	case ngapType.ProcedureCodeLocationReportingControl:
		return "Controls UE location reporting. Potential errors: Location services not supported, configuration failure."
	case ngapType.ProcedureCodeLocationReportingFailureIndication:
		return "Indicates location reporting failure. Common causes: Positioning method unavailable, measurement timeout, privacy restrictions."
	case ngapType.ProcedureCodeLocationReport:
		return "Reports UE location information. Potential errors: Measurement failure, insufficient accuracy, privacy violation."
	case ngapType.ProcedureCodeNASNonDeliveryIndication:
		return "Indicates failure to deliver NAS message to UE. Common causes: UE unreachable, radio link failure, congestion."
	case ngapType.ProcedureCodeNGReset:
		return "Resets NG interface between AMF and gNB. Potential errors: Reset already in progress, configuration mismatch."
	case ngapType.ProcedureCodeNGSetup:
		return "Establishes NG interface connection. Potential errors: Version mismatch, authentication failure, configuration incompatibility."
	case ngapType.ProcedureCodeOverloadStart:
		return "Indicates network overload condition. Common causes: CPU overload, memory exhaustion, traffic congestion."
	case ngapType.ProcedureCodeOverloadStop:
		return "Indicates end of overload condition. Potential errors: Premature overload stop, system instability."
	case ngapType.ProcedureCodePaging:
		return "Pages UE for incoming services. Potential errors: UE not reachable, paging area mismatch, resource exhaustion."
	case ngapType.ProcedureCodePathSwitchRequest:
		return "Requests path switch during handover. Potential errors: Path not available, routing failure, QoS mismatch."
	case ngapType.ProcedureCodePDUSessionResourceModify:
		return "Modifies existing PDU session resources. Potential errors: Session not found, QoS modification failure, resource unavailable."
	case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
		return "Indicates PDU session modification. Potential errors: Modification rejected, resource conflict, policy violation."
	case ngapType.ProcedureCodePDUSessionResourceRelease:
		return "Releases PDU session resources. Potential errors: Session not found, release failure, cleanup incomplete."
	case ngapType.ProcedureCodePDUSessionResourceSetup:
		return "Establishes new PDU session resources. Potential errors: QoS allocation failure, tunnel setup failure, policy rejection."
	case ngapType.ProcedureCodePDUSessionResourceNotify:
		return "Notifies PDU session resource changes. Potential errors: Notification failure, state inconsistency."
	case ngapType.ProcedureCodePrivateMessage:
		return "Vendor-specific private messaging. Potential errors: Unsupported message, vendor mismatch, parsing failure."
	case ngapType.ProcedureCodePWSCancel:
		return "Cancels Public Warning System broadcast. Potential errors: Broadcast not active, cancellation failure."
	case ngapType.ProcedureCodePWSFailureIndication:
		return "Indicates PWS broadcast failure. Common causes: Cell unavailable, message too large, resource shortage."
	case ngapType.ProcedureCodePWSRestartIndication:
		return "Indicates PWS broadcast restart. Potential errors: Restart conflict, configuration error."
	case ngapType.ProcedureCodeRANConfigurationUpdate:
		return "Updates RAN configuration parameters. Potential errors: Invalid configuration, application failure, dependency conflict."
	case ngapType.ProcedureCodeRerouteNASRequest:
		return "Requests NAS message rerouting. Potential errors: Routing failure, target AMF unavailable, context transfer failure."
	case ngapType.ProcedureCodeRRCInactiveTransitionReport:
		return "Reports RRC state transition to inactive. Potential errors: State inconsistency, context preservation failure."
	case ngapType.ProcedureCodeTraceFailureIndication:
		return "Indicates trace session failure. Common causes: Trace session expired, storage full, configuration error."
	case ngapType.ProcedureCodeTraceStart:
		return "Starts new trace session for debugging. Potential errors: Maximum sessions reached, invalid parameters, permission denied."
	case ngapType.ProcedureCodeUEContextModification:
		return "Modifies existing UE context. Potential errors: Context not found, modification rejected, security failure."
	case ngapType.ProcedureCodeUEContextRelease:
		return "Releases UE context and resources. Potential errors: Context not found, release failure, cleanup incomplete."
	case ngapType.ProcedureCodeUEContextReleaseRequest:
		return "Requests UE context release. Potential errors: Release not allowed, ongoing procedures, policy restriction."
	case ngapType.ProcedureCodeUERadioCapabilityCheck:
		return "Checks UE radio capabilities. Potential errors: Capability mismatch, check timeout, UE not responding."
	case ngapType.ProcedureCodeUERadioCapabilityInfoIndication:
		return "Indicates UE radio capability information. Potential errors: Invalid capability info, version mismatch."
	case ngapType.ProcedureCodeUETNLABindingRelease:
		return "Releases UE Transport Network Layer Association binding. Potential errors: Binding not found, release failure."
	case ngapType.ProcedureCodeUplinkNASTransport:
		return "Transports NAS messages from UE to AMF. Potential errors: Authentication failure, message integrity check failure, routing error."
	case ngapType.ProcedureCodeUplinkNonUEAssociatedNRPPaTransport:
		return "Transports non-UE associated NRPPa messages. Potential errors: Invalid source, processing failure."
	case ngapType.ProcedureCodeUplinkRANConfigurationTransfer:
		return "Transfers RAN configuration from gNB to AMF. Potential errors: Invalid configuration, processing failure."
	case ngapType.ProcedureCodeUplinkRANStatusTransfer:
		return "Transfers RAN status during handover. Potential errors: Context mismatch, transfer failure."
	case ngapType.ProcedureCodeUplinkUEAssociatedNRPPaTransport:
		return "Transports UE-associated NRPPa messages. Potential errors: UE context error, positioning service failure."
	case ngapType.ProcedureCodeWriteReplaceWarning:
		return "Writes or replaces emergency warning messages. Potential errors: Message validation failure, broadcast area mismatch."
	case ngapType.ProcedureCodeSecondaryRATDataUsageReport:
		return "Reports data usage on secondary RAT. Potential errors: Measurement failure, reporting timeout, data inconsistency."
	default:
		return "Unknown procedure - may indicate protocol version mismatch, implementation error, or corrupted message."
	}
}

// Generic extraction function for unknown message types
func extractUeIdsFromGenericIEs(value interface{}) (ranUeNgapId, amfUeNgapId int64, found bool) {
	// log.Printf("[DEBUG] Message extraction attempting for type: %T", value)
	switch v := value.(type) {
	case *ngapType.InitiatingMessage:
		if v != nil {
			procedureName := getProcedureCodeName(v.ProcedureCode.Value)
			procedureDetails := getProcedureCodeDetails(v.ProcedureCode.Value)
			log.Printf("[INFO] Ignoring InitiatingMessage - Procedure: %s (Code: %d)", procedureName, v.ProcedureCode.Value)
			log.Printf("[DEBUG] Procedure Details: %s", procedureDetails)
		}
	case *ngapType.SuccessfulOutcome:
		if v != nil {
			procedureName := getProcedureCodeName(v.ProcedureCode.Value)
			procedureDetails := getProcedureCodeDetails(v.ProcedureCode.Value)
			log.Printf("[INFO] Ignoring SuccessfulOutcome - Procedure: %s (Code: %d)", procedureName, v.ProcedureCode.Value)
			log.Printf("[DEBUG] Procedure Details: %s", procedureDetails)
		}
	case *ngapType.UnsuccessfulOutcome:
		if v != nil {
			procedureName := getProcedureCodeName(v.ProcedureCode.Value)
			procedureDetails := getProcedureCodeDetails(v.ProcedureCode.Value)
			log.Printf("[WARN] Ignoring UnsuccessfulOutcome - Procedure: %s (Code: %d)", procedureName, v.ProcedureCode.Value)
			log.Printf("[WARN] This indicates a procedure failure. Details: %s", procedureDetails)
			log.Printf("[WARN] Consider implementing specific handling for this procedure to extract error information.")
		}
	default:
		log.Printf("[INFO] Ignoring unknown message type: %T", value)
		log.Printf("[DEBUG] This may indicate an unsupported message format or protocol version mismatch.")
	}

	return 0, 0, false
}

// Modification functions
func modifyUeIdsInInitiatingMessage(msg *ngapType.InitiatingMessage, newRanUeNgapId, newAmfUeNgapId int64) error {
	switch msg.ProcedureCode.Value {
	case ngapType.ProcedureCodeInitialUEMessage:
		return modifyUeIdsInInitialUEMessage(msg.Value.InitialUEMessage, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUplinkNASTransport:
		return modifyUeIdsInUplinkNASTransport(msg.Value.UplinkNASTransport, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeDownlinkNASTransport:
		return modifyUeIdsInDownlinkNASTransport(msg.Value.DownlinkNASTransport, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUEContextReleaseRequest:
		return modifyUeIdsInUEContextReleaseRequest(msg.Value.UEContextReleaseRequest, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePDUSessionResourceSetup:
		return modifyUeIdsInPDUSessionResourceSetup(msg.Value.PDUSessionResourceSetupRequest, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePDUSessionResourceRelease:
		return modifyUeIdsInPDUSessionResourceRelease(msg.Value.PDUSessionResourceReleaseCommand, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeInitialContextSetup:
		return modifyUeIdsInInitialContextSetup(msg.Value.InitialContextSetupRequest, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeCellTrafficTrace:
		return modifyUeIdsFromCellTrafficTrace(msg.Value.CellTrafficTrace, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeDeactivateTrace:
		return modifyUeIdsFromDeactivateTrace(msg.Value.DeactivateTrace, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeDownlinkRANStatusTransfer:
		return modifyUeIdsFromDownlinkRANStatusTransfer(msg.Value.DownlinkRANStatusTransfer, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeHandoverCancel:
		return modifyUeIdsFromHandoverCancel(msg.Value.HandoverCancel, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeHandoverNotification:
		return modifyUeIdsFromHandoverNotification(msg.Value.HandoverNotify, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeHandoverResourceAllocation:
		return modifyUeIdsFromHandoverResourceAllocation(msg.Value.HandoverRequest, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeHandoverPreparation:
		return modifyUeIdsFromHandoverPreparation(msg.Value.HandoverRequired, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeLocationReport:
		return modifyUeIdsFromLocationReport(msg.Value.LocationReport, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeLocationReportingControl:
		return modifyUeIdsFromLocationReportingControl(msg.Value.LocationReportingControl, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeLocationReportingFailureIndication:
		return modifyUeIdsFromLocationReportingFailureIndication(msg.Value.LocationReportingFailureIndication, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeNASNonDeliveryIndication:
		return modifyUeIdsFromNASNonDeliveryIndication(msg.Value.NASNonDeliveryIndication, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
		return modifyUeIdsFromPDUSessionResourceModifyIndication(msg.Value.PDUSessionResourceModifyIndication, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePDUSessionResourceModify:
		return modifyUeIdsFromPDUSessionResourceModify(msg.Value.PDUSessionResourceModifyRequest, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePDUSessionResourceNotify:
		return modifyUeIdsFromPDUSessionResourceNotify(msg.Value.PDUSessionResourceNotify, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeRerouteNASRequest:
		return modifyUeIdsFromRerouteNASRequest(msg.Value.RerouteNASRequest, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeSecondaryRATDataUsageReport:
		return modifyUeIdsFromSecondaryRATDataUsageReport(msg.Value.SecondaryRATDataUsageReport, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeTraceFailureIndication:
		return modifyUeIdsFromTraceFailureIndication(msg.Value.TraceFailureIndication, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeTraceStart:
		return modifyUeIdsFromTraceStart(msg.Value.TraceStart, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUEContextModification:
		return modifyUeIdsFromUEContextModification(msg.Value.UEContextModificationRequest, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUEContextRelease:
		return modifyUeIdsFromUEContextRelease(msg.Value.UEContextReleaseCommand, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUETNLABindingRelease:
		return modifyUeIdsFromUETNLABindingRelease(msg.Value.UETNLABindingReleaseRequest, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUplinkRANStatusTransfer:
		return modifyUeIdsFromUplinkRANStatusTransfer(msg.Value.UplinkRANStatusTransfer, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUplinkUEAssociatedNRPPaTransport:
		return modifyUeIdsFromUplinkUEAssociatedNRPPaTransport(msg.Value.UplinkUEAssociatedNRPPaTransport, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePathSwitchRequest:
		return modifyUeIdsFromPathSwitchRequest(msg.Value.PathSwitchRequest, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUERadioCapabilityCheck:
		return modifyUeIdsFromUERadioCapabilityCheckRequest(msg.Value.UERadioCapabilityCheckRequest, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeDownlinkUEAssociatedNRPPaTransport:
		return modifyUeIdsFromDownlinkUEAssociatedNRPPaTransport(msg.Value.DownlinkUEAssociatedNRPPaTransport, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeRRCInactiveTransitionReport:
		return modifyUeIdsFromRRCInactiveTransitionReport(msg.Value.RRCInactiveTransitionReport, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUERadioCapabilityInfoIndication:
		return modifyUeIdsFromUERadioCapabilityInfoIndication(msg.Value.UERadioCapabilityInfoIndication, newRanUeNgapId, newAmfUeNgapId)
	// Add more cases as needed
	default:
		return modifyUeIdsInGenericIEs(msg.Value, newRanUeNgapId, newAmfUeNgapId)
	}
}

func modifyUeIdsInSuccessfulOutcome(msg *ngapType.SuccessfulOutcome, newRanUeNgapId, newAmfUeNgapId int64) error {
	switch msg.ProcedureCode.Value {
	case ngapType.ProcedureCodeInitialContextSetup:
		return modifyUeIdsInInitialContextSetupResponse(msg.Value.InitialContextSetupResponse, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePDUSessionResourceSetup:
		return modifyUeIdsInPDUSessionResourceSetupResponse(msg.Value.PDUSessionResourceSetupResponse, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePDUSessionResourceRelease:
		return modifyUeIdsInPDUSessionResourceReleaseResponse(msg.Value.PDUSessionResourceReleaseResponse, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeHandoverCancel:
		return modifyUeIdsInHandoverCancel(msg.Value.HandoverCancelAcknowledge, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeHandoverPreparation:
		return modifyUeIdsInHandoverCommand(msg.Value.HandoverCommand, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeHandoverResourceAllocation:
		return modifyUeIdsInHandoverRequestAcknowledge(msg.Value.HandoverRequestAcknowledge, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
		return modifyUeIdsInPDUSessionResourceModifyIndication(msg.Value.PDUSessionResourceModifyConfirm, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePDUSessionResourceModify:
		return modifyUeIdsInPDUSessionResourceModifyResponse(msg.Value.PDUSessionResourceModifyResponse, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUEContextModification:
		return modifyUeIdsInUEContextModificationResponse(msg.Value.UEContextModificationResponse, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUERadioCapabilityCheck:
		return modifyUeIdsInUERadioCapabilityCheckResponse(msg.Value.UERadioCapabilityCheckResponse, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePathSwitchRequest:
		return modifyUeIdsInPathSwitchRequestAcknowledge(msg.Value.PathSwitchRequestAcknowledge, newRanUeNgapId, newAmfUeNgapId)
	// Add more cases as needed
	default:
		return modifyUeIdsInGenericIEs(msg.Value, newRanUeNgapId, newAmfUeNgapId)
	}
}

func modifyUeIdsInUnsuccessfulOutcome(msg *ngapType.UnsuccessfulOutcome, newRanUeNgapId, newAmfUeNgapId int64) error {
	switch msg.ProcedureCode.Value {
	case ngapType.ProcedureCodeInitialContextSetup:
		return modifyUeIdsInInitialContextSetupFailure(msg.Value.InitialContextSetupFailure, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeHandoverResourceAllocation:
		return modifyUeIdsInHandoverResourceAllocation(msg.Value.HandoverFailure, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeHandoverPreparation:
		return modifyUeIdsInHandoverPreparation(msg.Value.HandoverPreparationFailure, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodeUEContextModification:
		return modifyUeIdsInUEContextModification(msg.Value.UEContextModificationFailure, newRanUeNgapId, newAmfUeNgapId)
	case ngapType.ProcedureCodePathSwitchRequest:
		return modifyUeIdsInPathSwitchRequest(msg.Value.PathSwitchRequestFailure, newRanUeNgapId, newAmfUeNgapId)
		// Add more cases as needed
	default:
		return modifyUeIdsInGenericIEs(msg.Value, newRanUeNgapId, newAmfUeNgapId)
	}
}

// Helper functions for modifying specific message types
func modifyUeIdsInInitialUEMessage(msg *ngapType.InitialUEMessage, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("InitialUEMessage is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
			// InitialUEMessage doesn't contain AMF UE NGAP ID - it's assigned later by AMF
		}
	}
	return nil
}

func modifyUeIdsInUplinkNASTransport(msg *ngapType.UplinkNASTransport, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UplinkNASTransport is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
			fmt.Printf("[DEBUG] Updated RAN UE NGAP ID to %d in UplinkNASTransport", msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value)
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
			fmt.Printf("[DEBUG] Updated AMF UE NGAP ID to %d in UplinkNASTransport", msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value)
		}
	}

	return nil
}

func modifyUeIdsInDownlinkNASTransport(msg *ngapType.DownlinkNASTransport, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("DownlinkNASTransport is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

// Additional modification functions for other message types
func modifyUeIdsInUEContextReleaseRequest(msg *ngapType.UEContextReleaseRequest, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UEContextReleaseRequest is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInPDUSessionResourceSetup(msg *ngapType.PDUSessionResourceSetupRequest, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PDUSessionResourceSetupRequest is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInPDUSessionResourceRelease(msg *ngapType.PDUSessionResourceReleaseCommand, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PDUSessionResourceReleaseCommand is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInInitialContextSetup(msg *ngapType.InitialContextSetupRequest, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("InitialContextSetupRequest is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromCellTrafficTrace(msg *ngapType.CellTrafficTrace, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("CellTrafficTrace is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromDeactivateTrace(msg *ngapType.DeactivateTrace, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("DeactivateTrace is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromDownlinkRANStatusTransfer(msg *ngapType.DownlinkRANStatusTransfer, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("DownlinkRANStatusTransfer is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromHandoverCancel(msg *ngapType.HandoverCancel, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("HandoverCancel is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromHandoverNotification(msg *ngapType.HandoverNotify, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Handover Notify is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromHandoverResourceAllocation(msg *ngapType.HandoverRequest, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Handover Request is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromHandoverPreparation(msg *ngapType.HandoverRequired, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Handover Required is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromLocationReport(msg *ngapType.LocationReport, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Location Report is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromLocationReportingControl(msg *ngapType.LocationReportingControl, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Location Reporting Control is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromLocationReportingFailureIndication(msg *ngapType.LocationReportingFailureIndication, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Location Reporting Failure Indication is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromNASNonDeliveryIndication(msg *ngapType.NASNonDeliveryIndication, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("NAS Non Delivery Indication is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromPDUSessionResourceModifyIndication(msg *ngapType.PDUSessionResourceModifyIndication, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PDUSession Resource Modify Indication is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromPDUSessionResourceModify(msg *ngapType.PDUSessionResourceModifyRequest, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PDUSession Resource Modify Request is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromPDUSessionResourceNotify(msg *ngapType.PDUSessionResourceNotify, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PDUSession Resource Notify is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromRerouteNASRequest(msg *ngapType.RerouteNASRequest, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Reroute NAS Request is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromSecondaryRATDataUsageReport(msg *ngapType.SecondaryRATDataUsageReport, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Secondary RAT Data Usage Report is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromTraceFailureIndication(msg *ngapType.TraceFailureIndication, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Trace Failure Indication is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromTraceStart(msg *ngapType.TraceStart, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Trace Start is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromUEContextModification(msg *ngapType.UEContextModificationRequest, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UE Context Modification Request is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromUEContextRelease(msg *ngapType.UEContextReleaseCommand, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UE Context Release Command is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.UENGAPIDs.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.UENGAPIDs.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}

	return nil
}

func modifyUeIdsFromUETNLABindingRelease(msg *ngapType.UETNLABindingReleaseRequest, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UE TNL Association Binding Release Request is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromUplinkRANStatusTransfer(msg *ngapType.UplinkRANStatusTransfer, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Uplink RAN Status Transfer is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromUplinkUEAssociatedNRPPaTransport(msg *ngapType.UplinkUEAssociatedNRPPaTransport, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Uplink UE Associated NRPPa Transport is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromPathSwitchRequest(msg *ngapType.PathSwitchRequest, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Path Switch Request is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromUERadioCapabilityCheckRequest(msg *ngapType.UERadioCapabilityCheckRequest, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UE Radio Capability Check Request is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromDownlinkUEAssociatedNRPPaTransport(msg *ngapType.DownlinkUEAssociatedNRPPaTransport, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("Downlink UE Associated NRPPa Transport is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromRRCInactiveTransitionReport(msg *ngapType.RRCInactiveTransitionReport, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("RRC Inactive Transition Report is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsFromUERadioCapabilityInfoIndication(msg *ngapType.UERadioCapabilityInfoIndication, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UE Radio Capability Info Indication is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

// Response message modification functions
func modifyUeIdsInInitialContextSetupResponse(msg *ngapType.InitialContextSetupResponse, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("InitialContextSetupResponse is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInPDUSessionResourceSetupResponse(msg *ngapType.PDUSessionResourceSetupResponse, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PDUSessionResourceSetupResponse is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInPDUSessionResourceReleaseResponse(msg *ngapType.PDUSessionResourceReleaseResponse, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PDUSessionResourceReleaseResponse is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInHandoverCancel(msg *ngapType.HandoverCancelAcknowledge, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("HandoverCancelAcknowledge is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInHandoverCommand(msg *ngapType.HandoverCommand, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("HandoverCommand is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInHandoverRequestAcknowledge(msg *ngapType.HandoverRequestAcknowledge, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("HandoverRequestAcknowledge is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInPDUSessionResourceModifyIndication(msg *ngapType.PDUSessionResourceModifyConfirm, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PDUSessionResourceModifyIndication is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInPDUSessionResourceModifyResponse(msg *ngapType.PDUSessionResourceModifyResponse, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PDUSessionResourceModifyResponse is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInUEContextModificationResponse(msg *ngapType.UEContextModificationResponse, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UEContextModificationResponse is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInUERadioCapabilityCheckResponse(msg *ngapType.UERadioCapabilityCheckResponse, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UERadioCapabilityCheckResponse is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInPathSwitchRequestAcknowledge(msg *ngapType.PathSwitchRequestAcknowledge, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PathSwitchRequestAcknowledge is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInUEContextReleaseComplete(msg *ngapType.UEContextReleaseComplete, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UEContextReleaseComplete is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

// Failure message modification functions
func modifyUeIdsInInitialContextSetupFailure(msg *ngapType.InitialContextSetupFailure, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("InitialContextSetupFailure is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInHandoverResourceAllocation(msg *ngapType.HandoverFailure, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("HandoverResourceAllocation is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInHandoverPreparation(msg *ngapType.HandoverPreparationFailure, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("HandoverPreparation is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInUEContextModification(msg *ngapType.UEContextModificationFailure, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("UEContextModification is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

func modifyUeIdsInPathSwitchRequest(msg *ngapType.PathSwitchRequestFailure, newRanUeNgapId, newAmfUeNgapId int64) error {
	if msg == nil {
		return fmt.Errorf("PathSwitchRequest is nil")
	}

	for i := range msg.ProtocolIEs.List {
		switch msg.ProtocolIEs.List[i].Id.Value {
		case ngapType.ProtocolIEIDRANUENGAPID:
			if msg.ProtocolIEs.List[i].Value.RANUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.RANUENGAPID.Value = newRanUeNgapId
			}
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
		}
	}
	return nil
}

// Generic modification function for unknown message types
func modifyUeIdsInGenericIEs(value interface{}, newRanUeNgapId, newAmfUeNgapId int64) error {
	// First check if value is nil
	if value == nil {
		log.Printf("[DEBUG] NGAP UE ID modification: value is nil")
		return fmt.Errorf("message value is nil")
	}

	log.Printf("[DEBUG] NGAP UE ID modification attempting for type: %T", value)

	// Use reflection to access ProtocolIEs field
	v := reflect.ValueOf(value)

	// Handle pointer types and check for nil
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			log.Printf("[DEBUG] NGAP UE ID modification: pointer is nil")
			return fmt.Errorf("message pointer is nil")
		}
		v = v.Elem()
	}

	// Additional nil check after dereferencing
	if !v.IsValid() {
		log.Printf("[DEBUG] NGAP UE ID modification: dereferenced value is invalid")
		return fmt.Errorf("dereferenced message value is invalid")
	}

	// Look for ProtocolIEs field
	protocolIEsField := v.FieldByName("ProtocolIEs")
	if !protocolIEsField.IsValid() {
		log.Printf("[DEBUG] NGAP UE ID modification: ProtocolIEs field not found")
		return fmt.Errorf("ProtocolIEs field not found in message type %T", value)
	}

	// Check if ProtocolIEs is a pointer and handle nil
	if protocolIEsField.Kind() == reflect.Ptr {
		if protocolIEsField.IsNil() {
			log.Printf("[DEBUG] NGAP UE ID modification: ProtocolIEs pointer is nil")
			return fmt.Errorf("ProtocolIEs pointer is nil")
		}
		protocolIEsField = protocolIEsField.Elem()
	}

	// Get the List field from ProtocolIEs
	listField := protocolIEsField.FieldByName("List")
	if !listField.IsValid() || listField.Kind() != reflect.Slice {
		log.Printf("[DEBUG] NGAP UE ID modification: ProtocolIEs.List field not found or not a slice")
		return fmt.Errorf("ProtocolIEs.List field not found or not a slice")
	}

	// Check if the slice is nil
	if listField.IsNil() {
		log.Printf("[DEBUG] NGAP UE ID modification: ProtocolIEs.List is nil")
		return fmt.Errorf("ProtocolIEs.List is nil")
	}

	var ranModified, amfModified bool

	// Iterate through the protocol IEs
	for i := 0; i < listField.Len(); i++ {
		ie := listField.Index(i)
		if !ie.IsValid() {
			log.Printf("[DEBUG] NGAP UE ID modification: IE at index %d is invalid", i)
			continue
		}

		// Get the Id field
		idField := ie.FieldByName("Id")
		if !idField.IsValid() {
			log.Printf("[DEBUG] NGAP UE ID modification: Id field not found at index %d", i)
			continue
		}

		valueField := idField.FieldByName("Value")
		if !valueField.IsValid() {
			log.Printf("[DEBUG] NGAP UE ID modification: Id.Value field not found at index %d", i)
			continue
		}

		// Check if this is a RAN UE NGAP ID or AMF UE NGAP ID
		idValue := valueField.Int()

		// Get the Value field from the IE
		ieValueField := ie.FieldByName("Value")
		if !ieValueField.IsValid() {
			log.Printf("[DEBUG] NGAP UE ID modification: IE.Value field not found at index %d", i)
			continue
		}

		switch idValue {
		case ngapType.ProtocolIEIDRANUENGAPID:
			// Try to modify RAN UE NGAP ID
			ranUeField := ieValueField.FieldByName("RANUENGAPID")
			if ranUeField.IsValid() {
				// Check if RANUENGAPID is nil before accessing
				if ranUeField.IsNil() {
					log.Printf("[DEBUG] NGAP UE ID modification: RANUENGAPID field is nil")
					continue
				}

				ranValueField := ranUeField.Elem().FieldByName("Value")
				if ranValueField.IsValid() && ranValueField.CanSet() {
					ranValueField.SetInt(newRanUeNgapId)
					ranModified = true
					log.Printf("[DEBUG] NGAP UE ID modification set RAN UE NGAP ID to: %d", newRanUeNgapId)
				} else {
					log.Printf("[DEBUG] NGAP UE ID modification: RANUENGAPID.Value field not found or cannot be set")
				}
			} else {
				log.Printf("[DEBUG] NGAP UE ID modification: RANUENGAPID field not found")
			}

		case ngapType.ProtocolIEIDAMFUENGAPID:
			// Try to modify AMF UE NGAP ID
			amfUeField := ieValueField.FieldByName("AMFUENGAPID")
			if amfUeField.IsValid() {
				// Check if AMFUENGAPID is nil before accessing
				if amfUeField.IsNil() {
					log.Printf("[DEBUG] NGAP UE ID modification: AMFUENGAPID field is nil")
					continue
				}

				amfValueField := amfUeField.Elem().FieldByName("Value")
				if amfValueField.IsValid() && amfValueField.CanSet() {
					amfValueField.SetInt(newAmfUeNgapId)
					amfModified = true
					log.Printf("[DEBUG] NGAP UE ID modification set AMF UE NGAP ID to: %d", newAmfUeNgapId)
				} else {
					log.Printf("[DEBUG] NGAP UE ID modification: AMFUENGAPID.Value field not found or cannot be set")
				}
			} else {
				log.Printf("[DEBUG] NGAP UE ID modification: AMFUENGAPID field not found")
			}
		}
	}

	if ranModified || amfModified {
		log.Printf("[DEBUG] NGAP UE ID modification successful - RAN modified: %t, AMF modified: %t", ranModified, amfModified)
		return nil
	}

	log.Printf("[DEBUG] NGAP UE ID modification: no UE IDs were modified in message")
	// Don't return an error if no UE IDs were found - some messages may not contain them
	return nil
}
