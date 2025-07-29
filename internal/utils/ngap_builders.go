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
	// Add more cases as needed
	default:
		log.Printf("[DEBUG] Unhandled initiating message procedure code: %d", msg.ProcedureCode.Value)
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
	// Add more cases as needed
	default:
		return extractUeIdsFromGenericIEs(msg.Value)
	}
}

func extractUeIdsFromUnsuccessfulOutcome(msg *ngapType.UnsuccessfulOutcome) (ranUeNgapId, amfUeNgapId int64, found bool) {
	switch msg.ProcedureCode.Value {
	case ngapType.ProcedureCodeInitialContextSetup:
		return extractUeIdsFromInitialContextSetupFailure(msg.Value.InitialContextSetupFailure)
	// Add more cases as needed for other unsuccessful outcome messages
	default:
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

// Generic extraction function for unknown message types
func extractUeIdsFromGenericIEs(value interface{}) (ranUeNgapId, amfUeNgapId int64, found bool) {
	// First check if value is nil
	if value == nil {
		log.Printf("[DEBUG] Generic UE ID extraction: value is nil")
		return 0, 0, false
	}

	log.Printf("[DEBUG] Generic UE ID extraction attempting for type: %T", value)

	// Use reflection to access ProtocolIEs field
	v := reflect.ValueOf(value)

	// Handle pointer types and check for nil
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			log.Printf("[DEBUG] Generic UE ID extraction: pointer is nil")
			return 0, 0, false
		}
		v = v.Elem()
	}

	// Additional nil check after dereferencing
	if !v.IsValid() {
		log.Printf("[DEBUG] Generic UE ID extraction: dereferenced value is invalid")
		return 0, 0, false
	}

	// Look for ProtocolIEs field
	protocolIEsField := v.FieldByName("ProtocolIEs")
	if !protocolIEsField.IsValid() {
		log.Printf("[DEBUG] Generic UE ID extraction: ProtocolIEs field not found")
		return 0, 0, false
	}

	// Check if ProtocolIEs is a pointer and handle nil
	if protocolIEsField.Kind() == reflect.Ptr {
		if protocolIEsField.IsNil() {
			log.Printf("[DEBUG] Generic UE ID extraction: ProtocolIEs pointer is nil")
			return 0, 0, false
		}
		protocolIEsField = protocolIEsField.Elem()
	}

	// Get the List field from ProtocolIEs
	listField := protocolIEsField.FieldByName("List")
	if !listField.IsValid() || listField.Kind() != reflect.Slice {
		log.Printf("[DEBUG] Generic UE ID extraction: ProtocolIEs.List field not found or not a slice")
		return 0, 0, false
	}

	// Check if the slice is nil
	if listField.IsNil() {
		log.Printf("[DEBUG] Generic UE ID extraction: ProtocolIEs.List is nil")
		return 0, 0, false
	}

	var ranFound, amfFound bool

	// Iterate through the protocol IEs
	for i := 0; i < listField.Len(); i++ {
		ie := listField.Index(i)
		if !ie.IsValid() {
			log.Printf("[DEBUG] Generic UE ID extraction: IE at index %d is invalid", i)
			continue
		}

		// Get the Id field
		idField := ie.FieldByName("Id")
		if !idField.IsValid() {
			log.Printf("[DEBUG] Generic UE ID extraction: Id field not found at index %d", i)
			continue
		}

		valueField := idField.FieldByName("Value")
		if !valueField.IsValid() {
			log.Printf("[DEBUG] Generic UE ID extraction: Id.Value field not found at index %d", i)
			continue
		}

		// Check if this is a RAN UE NGAP ID or AMF UE NGAP ID
		idValue := valueField.Int()

		// Get the Value field from the IE
		ieValueField := ie.FieldByName("Value")
		if !ieValueField.IsValid() {
			log.Printf("[DEBUG] Generic UE ID extraction: IE.Value field not found at index %d", i)
			continue
		}

		switch idValue {
		case ngapType.ProtocolIEIDRANUENGAPID:
			// Try to extract RAN UE NGAP ID
			ranUeField := ieValueField.FieldByName("RANUENGAPID")
			if ranUeField.IsValid() {
				// Check if RANUENGAPID is nil before accessing
				if ranUeField.IsNil() {
					log.Printf("[DEBUG] Generic extraction: RANUENGAPID field is nil")
					continue
				}

				ranValueField := ranUeField.Elem().FieldByName("Value")
				if ranValueField.IsValid() {
					ranUeNgapId = ranValueField.Int()
					ranFound = true
					log.Printf("[DEBUG] Generic extraction found RAN UE NGAP ID: %d", ranUeNgapId)
				} else {
					log.Printf("[DEBUG] Generic extraction: RANUENGAPID.Value field not found")
				}
			} else {
				log.Printf("[DEBUG] Generic extraction: RANUENGAPID field not found")
			}

		case ngapType.ProtocolIEIDAMFUENGAPID:
			// Try to extract AMF UE NGAP ID
			amfUeField := ieValueField.FieldByName("AMFUENGAPID")
			if amfUeField.IsValid() {
				// Check if AMFUENGAPID is nil before accessing
				if amfUeField.IsNil() {
					log.Printf("[DEBUG] Generic extraction: AMFUENGAPID field is nil")
					continue
				}

				amfValueField := amfUeField.Elem().FieldByName("Value")
				if amfValueField.IsValid() {
					amfUeNgapId = amfValueField.Int()
					amfFound = true
					log.Printf("[DEBUG] Generic extraction found AMF UE NGAP ID: %d", amfUeNgapId)
				} else {
					log.Printf("[DEBUG] Generic extraction: AMFUENGAPID.Value field not found")
				}
			} else {
				log.Printf("[DEBUG] Generic extraction: AMFUENGAPID field not found")
			}
		}
	}

	found = ranFound || amfFound
	if found {
		log.Printf("[DEBUG] Generic UE ID extraction successful - RAN: %d (found: %t), AMF: %d (found: %t)",
			ranUeNgapId, ranFound, amfUeNgapId, amfFound)
	} else {
		log.Printf("[DEBUG] Generic UE ID extraction: no UE IDs found in message")
	}

	return ranUeNgapId, amfUeNgapId, found
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
	case ngapType.ProcedureCodeUEContextRelease:
		return modifyUeIdsInUEContextReleaseComplete(msg.Value.UEContextReleaseComplete, newRanUeNgapId, newAmfUeNgapId)
	// Add more cases as needed
	default:
		return modifyUeIdsInGenericIEs(msg.Value, newRanUeNgapId, newAmfUeNgapId)
	}
}

func modifyUeIdsInUnsuccessfulOutcome(msg *ngapType.UnsuccessfulOutcome, newRanUeNgapId, newAmfUeNgapId int64) error {
	switch msg.ProcedureCode.Value {
	case ngapType.ProcedureCodeInitialContextSetup:
		return modifyUeIdsInInitialContextSetupFailure(msg.Value.InitialContextSetupFailure, newRanUeNgapId, newAmfUeNgapId)
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
		case ngapType.ProtocolIEIDAMFUENGAPID:
			if msg.ProtocolIEs.List[i].Value.AMFUENGAPID != nil {
				msg.ProtocolIEs.List[i].Value.AMFUENGAPID.Value = newAmfUeNgapId
			}
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

// Generic modification function for unknown message types
func modifyUeIdsInGenericIEs(value interface{}, newRanUeNgapId, newAmfUeNgapId int64) error {
	// First check if value is nil
	if value == nil {
		log.Printf("[DEBUG] Generic UE ID modification: value is nil")
		return fmt.Errorf("message value is nil")
	}

	log.Printf("[DEBUG] Generic UE ID modification attempting for type: %T", value)

	// Use reflection to access ProtocolIEs field
	v := reflect.ValueOf(value)

	// Handle pointer types and check for nil
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			log.Printf("[DEBUG] Generic UE ID modification: pointer is nil")
			return fmt.Errorf("message pointer is nil")
		}
		v = v.Elem()
	}

	// Additional nil check after dereferencing
	if !v.IsValid() {
		log.Printf("[DEBUG] Generic UE ID modification: dereferenced value is invalid")
		return fmt.Errorf("dereferenced message value is invalid")
	}

	// Look for ProtocolIEs field
	protocolIEsField := v.FieldByName("ProtocolIEs")
	if !protocolIEsField.IsValid() {
		log.Printf("[DEBUG] Generic UE ID modification: ProtocolIEs field not found")
		return fmt.Errorf("ProtocolIEs field not found in message type %T", value)
	}

	// Check if ProtocolIEs is a pointer and handle nil
	if protocolIEsField.Kind() == reflect.Ptr {
		if protocolIEsField.IsNil() {
			log.Printf("[DEBUG] Generic UE ID modification: ProtocolIEs pointer is nil")
			return fmt.Errorf("ProtocolIEs pointer is nil")
		}
		protocolIEsField = protocolIEsField.Elem()
	}

	// Get the List field from ProtocolIEs
	listField := protocolIEsField.FieldByName("List")
	if !listField.IsValid() || listField.Kind() != reflect.Slice {
		log.Printf("[DEBUG] Generic UE ID modification: ProtocolIEs.List field not found or not a slice")
		return fmt.Errorf("ProtocolIEs.List field not found or not a slice")
	}

	// Check if the slice is nil
	if listField.IsNil() {
		log.Printf("[DEBUG] Generic UE ID modification: ProtocolIEs.List is nil")
		return fmt.Errorf("ProtocolIEs.List is nil")
	}

	var ranModified, amfModified bool

	// Iterate through the protocol IEs
	for i := 0; i < listField.Len(); i++ {
		ie := listField.Index(i)
		if !ie.IsValid() {
			log.Printf("[DEBUG] Generic UE ID modification: IE at index %d is invalid", i)
			continue
		}

		// Get the Id field
		idField := ie.FieldByName("Id")
		if !idField.IsValid() {
			log.Printf("[DEBUG] Generic UE ID modification: Id field not found at index %d", i)
			continue
		}

		valueField := idField.FieldByName("Value")
		if !valueField.IsValid() {
			log.Printf("[DEBUG] Generic UE ID modification: Id.Value field not found at index %d", i)
			continue
		}

		// Check if this is a RAN UE NGAP ID or AMF UE NGAP ID
		idValue := valueField.Int()

		// Get the Value field from the IE
		ieValueField := ie.FieldByName("Value")
		if !ieValueField.IsValid() {
			log.Printf("[DEBUG] Generic UE ID modification: IE.Value field not found at index %d", i)
			continue
		}

		switch idValue {
		case ngapType.ProtocolIEIDRANUENGAPID:
			// Try to modify RAN UE NGAP ID
			ranUeField := ieValueField.FieldByName("RANUENGAPID")
			if ranUeField.IsValid() {
				// Check if RANUENGAPID is nil before accessing
				if ranUeField.IsNil() {
					log.Printf("[DEBUG] Generic modification: RANUENGAPID field is nil")
					continue
				}

				ranValueField := ranUeField.Elem().FieldByName("Value")
				if ranValueField.IsValid() && ranValueField.CanSet() {
					ranValueField.SetInt(newRanUeNgapId)
					ranModified = true
					log.Printf("[DEBUG] Generic modification set RAN UE NGAP ID to: %d", newRanUeNgapId)
				} else {
					log.Printf("[DEBUG] Generic modification: RANUENGAPID.Value field not found or cannot be set")
				}
			} else {
				log.Printf("[DEBUG] Generic modification: RANUENGAPID field not found")
			}

		case ngapType.ProtocolIEIDAMFUENGAPID:
			// Try to modify AMF UE NGAP ID
			amfUeField := ieValueField.FieldByName("AMFUENGAPID")
			if amfUeField.IsValid() {
				// Check if AMFUENGAPID is nil before accessing
				if amfUeField.IsNil() {
					log.Printf("[DEBUG] Generic modification: AMFUENGAPID field is nil")
					continue
				}

				amfValueField := amfUeField.Elem().FieldByName("Value")
				if amfValueField.IsValid() && amfValueField.CanSet() {
					amfValueField.SetInt(newAmfUeNgapId)
					amfModified = true
					log.Printf("[DEBUG] Generic modification set AMF UE NGAP ID to: %d", newAmfUeNgapId)
				} else {
					log.Printf("[DEBUG] Generic modification: AMFUENGAPID.Value field not found or cannot be set")
				}
			} else {
				log.Printf("[DEBUG] Generic modification: AMFUENGAPID field not found")
			}
		}
	}

	if ranModified || amfModified {
		log.Printf("[DEBUG] Generic UE ID modification successful - RAN modified: %t, AMF modified: %t", ranModified, amfModified)
		return nil
	}

	log.Printf("[DEBUG] Generic UE ID modification: no UE IDs were modified in message")
	// Don't return an error if no UE IDs were found - some messages may not contain them
	return nil
}
