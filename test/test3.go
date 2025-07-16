package main

import (
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/logger"
	"github.com/free5gc/ngap/ngapConvert"
	"github.com/free5gc/ngap/ngapType"
	"github.com/free5gc/openapi/models"
	"github.com/ishidawataru/sctp"
	"github.com/sirupsen/logrus"
)

type AmfRan struct {
	RanPresent      int
	RanId           *models.GlobalRanNodeId
	Name            string
	AnType          models.AccessType
	Conn            net.Conn
	SupportedTAList []SupportedTAI
	RanUeList       sync.Map // RanUeNgapId as key
	Log             *logrus.Entry
}

type SupportedTAI struct {
	Tai        models.Tai
	SNssaiList []models.Snssai
}

type PlmnSupportItem struct {
	PlmnId     *models.PlmnId
	SNssaiList []models.Snssai
}

type ProxyContext struct {
	Name             string
	ServedGUAMIList  []models.Guami
	RelativeCapacity int64
	NgapIPList       []string
	NgapPort         int
	PlmnSupportList  []PlmnSupportItem
}

var proxySelf = &ProxyContext{
	Name: "AMF000",
	ServedGUAMIList: []models.Guami{
		{
			PlmnId: &models.PlmnIdNid{
				Mcc: "208",
				Mnc: "93",
				Nid: "", // Set NID if needed, otherwise empty
			},
			AmfId: "000000",
		},
	},
	RelativeCapacity: 255,
	NgapIPList:       []string{"127.0.0.10"},
	NgapPort:         38412,
	PlmnSupportList: []PlmnSupportItem{
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

// // Example of AmfRan value:
// var ran = &AmfRan{
// 	RanPresent:      1,
// 	RanId:           &models.GlobalRanNodeId{PlmnId: &models.PlmnId{Mcc: "001", Mnc: "01"}, RanNodeId: "gnb-123"},
// 	Name:            "TestRAN",
// 	AnType:          models.AccessType__3_GPP_ACCESS,
// 	Conn:            nil, // assign actual net.Conn when available
// 	SupportedTAList: []SupportedTAI{}, // fill with SupportedTAI values as needed
// 	Log:             logrus.NewEntry(logrus.New()),
// }

func main() {
	// Listen SCTP on 127.0.0.10:38412
	laddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("127.0.0.10")}},
		Port:    38412,
	}
	listener, err := sctp.ListenSCTP("sctp", laddr)
	if err != nil {
		log.Fatalf("Failed to listen SCTP: %v", err)
	}
	defer listener.Close()
	fmt.Println("Proxy SCTP server listening on 127.0.0.10:38412")

	for {
		conn, err := listener.AcceptSCTP()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}

		// Get AmfRan instance with basic info from connection
		ran := &AmfRan{
			Conn: conn,
			Name: fmt.Sprintf("RAN-%v", conn.RemoteAddr()),
			Log:  logrus.NewEntry(logrus.New()),
			// Fill other fields as needed
		}

		go handleConnection(ran, conn)
	}
}

func handleConnection(ran *AmfRan, conn *sctp.SCTPConn) {
	defer conn.Close()
	fmt.Printf("Accepted connection from %v\n", conn.RemoteAddr())
	// Create a new AmfRan instance

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		log.Printf("Read error: %v", err)
		return
	}

	fmt.Printf("Received %d bytes from gNB\n", n)
	ngapMsg, err := ngap.Decoder(buf[:n])
	if err != nil {
		log.Printf("NGAP decode error: %v", err)
		return
	}

	if ngapMsg == nil {
		log.Printf("NGAP Message is nil")
		return
	}

	fmt.Println("Decoded NGAP message:")
	dispatchMain(ran, ngapMsg)
}

// func printNGAPMessage(msg *ngapType.NGAPPDU) {
// 	fmt.Printf("%#v\n", msg)
// }

func dispatchMain(ran *AmfRan, message *ngapType.NGAPPDU) {
	switch message.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := message.InitiatingMessage
		if initiatingMessage == nil {
			log.Printf("InitiatingMessage is nil")
			return
		}
		switch initiatingMessage.ProcedureCode.Value {
		// case ngapType.ProcedureCodeAMFConfigurationUpdate:
		// 	handlerAMFConfigurationUpdate(ran, initiatingMessage)
		// case ngapType.ProcedureCodeAMFStatusIndication:
		// 	handlerAMFStatusIndication(ran, initiatingMessage)
		// case ngapType.ProcedureCodeCellTrafficTrace:
		// 	handlerCellTrafficTrace(ran, initiatingMessage)
		// case ngapType.ProcedureCodeDeactivateTrace:
		// 	handlerDeactivateTrace(ran, initiatingMessage)
		// case ngapType.ProcedureCodeDownlinkNASTransport:
		// 	handlerDownlinkNASTransport(ran, initiatingMessage)
		// case ngapType.ProcedureCodeDownlinkNonUEAssociatedNRPPaTransport:
		// 	handlerDownlinkNonUEAssociatedNRPPaTransport(ran, initiatingMessage)
		// case ngapType.ProcedureCodeDownlinkRANConfigurationTransfer:
		// 	handlerDownlinkRANConfigurationTransfer(ran, initiatingMessage)
		// case ngapType.ProcedureCodeDownlinkRANStatusTransfer:
		// 	handlerDownlinkRANStatusTransfer(ran, initiatingMessage)
		// case ngapType.ProcedureCodeDownlinkUEAssociatedNRPPaTransport:
		// 	handlerDownlinkUEAssociatedNRPPaTransport(ran, initiatingMessage)
		// case ngapType.ProcedureCodeErrorIndication:
		// 	handlerErrorIndication(ran, initiatingMessage)
		// case ngapType.ProcedureCodeHandoverCancel:
		// 	handlerHandoverCancel(ran, initiatingMessage)
		// case ngapType.ProcedureCodeHandoverNotification:
		// 	handlerHandoverNotify(ran, initiatingMessage)
		// case ngapType.ProcedureCodeHandoverResourceAllocation:
		// 	handlerHandoverRequest(ran, initiatingMessage)
		// case ngapType.ProcedureCodeHandoverPreparation:
		// 	handlerHandoverRequired(ran, initiatingMessage)
		// case ngapType.ProcedureCodeInitialContextSetup:
		// 	handlerInitialContextSetupRequest(ran, initiatingMessage)
		// case ngapType.ProcedureCodeInitialUEMessage:
		// 	handlerInitialUEMessage(ran, message, initiatingMessage)
		// case ngapType.ProcedureCodeLocationReport:
		// 	handlerLocationReport(ran, initiatingMessage)
		// case ngapType.ProcedureCodeLocationReportingControl:
		// 	handlerLocationReportingControl(ran, initiatingMessage)
		// case ngapType.ProcedureCodeLocationReportingFailureIndication:
		// 	handlerLocationReportingFailureIndication(ran, initiatingMessage)
		// case ngapType.ProcedureCodeNASNonDeliveryIndication:
		// 	handlerNASNonDeliveryIndication(ran, initiatingMessage)
		// case ngapType.ProcedureCodeNGReset:
		// 	handlerNGReset(ran, initiatingMessage)
		case ngapType.ProcedureCodeNGSetup:
			handlerNGSetupRequest(ran, initiatingMessage)
			// case ngapType.ProcedureCodeOverloadStart:
			// 	handlerOverloadStart(ran, initiatingMessage)
			// case ngapType.ProcedureCodeOverloadStop:
			// 	handlerOverloadStop(ran, initiatingMessage)
			// case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
			// 	handlerPDUSessionResourceModifyIndication(ran, initiatingMessage)
			// case ngapType.ProcedureCodePDUSessionResourceModify:
			// 	handlerPDUSessionResourceModifyRequest(ran, initiatingMessage)
			// case ngapType.ProcedureCodePDUSessionResourceNotify:
			// 	handlerPDUSessionResourceNotify(ran, initiatingMessage)
			// case ngapType.ProcedureCodePDUSessionResourceRelease:
			// 	handlerPDUSessionResourceReleaseCommand(ran, initiatingMessage)
			// case ngapType.ProcedureCodePDUSessionResourceSetup:
			// 	handlerPDUSessionResourceSetupRequest(ran, initiatingMessage)
			// case ngapType.ProcedureCodePWSCancel:
			// 	handlerPWSCancelRequest(ran, initiatingMessage)
			// case ngapType.ProcedureCodePWSFailureIndication:
			// 	handlerPWSFailureIndication(ran, initiatingMessage)
			// case ngapType.ProcedureCodePWSRestartIndication:
			// 	handlerPWSRestartIndication(ran, initiatingMessage)
			// case ngapType.ProcedureCodePaging:
			// 	handlerPaging(ran, initiatingMessage)
			// case ngapType.ProcedureCodePathSwitchRequest:
			// 	handlerPathSwitchRequest(ran, initiatingMessage)
			// case ngapType.ProcedureCodeRANConfigurationUpdate:
			// 	handlerRANConfigurationUpdate(ran, initiatingMessage)
			// case ngapType.ProcedureCodeRRCInactiveTransitionReport:
			// 	handlerRRCInactiveTransitionReport(ran, initiatingMessage)
			// case ngapType.ProcedureCodeRerouteNASRequest:
			// 	handlerRerouteNASRequest(ran, initiatingMessage)
			// case ngapType.ProcedureCodeSecondaryRATDataUsageReport:
			// 	handlerSecondaryRATDataUsageReport(ran, initiatingMessage)
			// case ngapType.ProcedureCodeTraceFailureIndication:
			// 	handlerTraceFailureIndication(ran, initiatingMessage)
			// case ngapType.ProcedureCodeTraceStart:
			// 	handlerTraceStart(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUEContextModification:
			// 	handlerUEContextModificationRequest(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUEContextRelease:
			// 	handlerUEContextReleaseCommand(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUEContextReleaseRequest:
			// 	handlerUEContextReleaseRequest(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUERadioCapabilityCheck:
			// 	handlerUERadioCapabilityCheckRequest(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUERadioCapabilityInfoIndication:
			// 	handlerUERadioCapabilityInfoIndication(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUETNLABindingRelease:
			// 	handlerUETNLABindingReleaseRequest(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUplinkNASTransport:
			// 	handlerUplinkNASTransport(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUplinkNonUEAssociatedNRPPaTransport:
			// 	handlerUplinkNonUEAssociatedNRPPaTransport(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUplinkRANConfigurationTransfer:
			// 	handlerUplinkRANConfigurationTransfer(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUplinkRANStatusTransfer:
			// 	handlerUplinkRANStatusTransfer(ran, initiatingMessage)
			// case ngapType.ProcedureCodeUplinkUEAssociatedNRPPaTransport:
			// 	handlerUplinkUEAssociatedNRPPaTransport(ran, initiatingMessage)
			// case ngapType.ProcedureCodeWriteReplaceWarning:
			// 	handlerWriteReplaceWarningRequest(ran, initiatingMessage)
			// 	default:
			// 		cause := ngapType.Cause{
			// 			Present:  ngapType.CausePresentProtocol,
			// 			Protocol: &ngapType.CauseProtocol{},
			// 		}
			// 		switch initiatingMessage.Criticality.Value {
			// 		case ngapType.CriticalityPresentReject:
			// 			log.Printf("Not comprehended procedure code of InitiatingMessage (criticality: reject, procedureCode:0x%02x)", initiatingMessage.ProcedureCode.Value)
			// 			cause.Protocol.Value = ngapType.CauseProtocolPresentAbstractSyntaxErrorReject
			// 		case ngapType.CriticalityPresentIgnore:
			// 			log.Printf("Not comprehended procedure code of InitiatingMessage (criticality: ignore, procedureCode:0x%02x)", initiatingMessage.ProcedureCode.Value)
			// 			return
			// 		case ngapType.CriticalityPresentNotify:
			// 			log.Printf("Not comprehended procedure code of InitiatingMessage (criticality: notify, procedureCode:0x%02x)", initiatingMessage.ProcedureCode.Value)
			// 			cause.Protocol.Value = ngapType.CauseProtocolPresentAbstractSyntaxErrorIgnoreAndNotify
			// 		}
			// 		triggeringMessage := ngapType.TriggeringMessagePresentInitiatingMessage
			// 		criticalityDiagnostics := buildCriticalityDiagnostics(&initiatingMessage.ProcedureCode.Value, &triggeringMessage, &initiatingMessage.Criticality.Value, nil)
			// 		ngap_message.SendErrorIndication(ran, nil, nil, &cause, &criticalityDiagnostics)
		}
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		successfulOutcome := message.SuccessfulOutcome
		if successfulOutcome == nil {
			log.Println("SuccessfulOutcome is nil")
			return
		}
		switch successfulOutcome.ProcedureCode.Value {
		// 	case ngapType.ProcedureCodeAMFConfigurationUpdate:
		// 		handlerAMFConfigurationUpdateAcknowledge(ran, successfulOutcome)
		// 	case ngapType.ProcedureCodeHandoverCancel:
		// 		handlerHandoverCancelAcknowledge(ran, successfulOutcome)
		// 	case ngapType.ProcedureCodeHandoverPreparation:
		// 		handlerHandoverCommand(ran, successfulOutcome)
		// 	case ngapType.ProcedureCodeHandoverResourceAllocation:
		// 		handlerHandoverRequestAcknowledge(ran, successfulOutcome)
		// 	case ngapType.ProcedureCodeInitialContextSetup:
		// 		handlerInitialContextSetupResponse(ran, successfulOutcome)
		// 	case ngapType.ProcedureCodeNGReset:
		// 		handlerNGResetAcknowledge(ran, successfulOutcome)
		case ngapType.ProcedureCodeNGSetup:
			handlerNGSetupResponse(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
			// 		handlerPDUSessionResourceModifyConfirm(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodePDUSessionResourceModify:
			// 		handlerPDUSessionResourceModifyResponse(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodePDUSessionResourceRelease:
			// 		handlerPDUSessionResourceReleaseResponse(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodePDUSessionResourceSetup:
			// 		handlerPDUSessionResourceSetupResponse(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodePWSCancel:
			// 		handlerPWSCancelResponse(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodePathSwitchRequest:
			// 		handlerPathSwitchRequestAcknowledge(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodeRANConfigurationUpdate:
			// 		handlerRANConfigurationUpdateAcknowledge(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodeUEContextModification:
			// 		handlerUEContextModificationResponse(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodeUEContextRelease:
			// 		handlerUEContextReleaseComplete(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodeUERadioCapabilityCheck:
			// 		handlerUERadioCapabilityCheckResponse(ran, successfulOutcome)
			// 	case ngapType.ProcedureCodeWriteReplaceWarning:
			// 		handlerWriteReplaceWarningResponse(ran, successfulOutcome)
			// 	default:
			// 		cause := ngapType.Cause{
			// 			Present:  ngapType.CausePresentProtocol,
			// 			Protocol: &ngapType.CauseProtocol{},
			// 		}
			// 		switch successfulOutcome.Criticality.Value {
			// 		case ngapType.CriticalityPresentReject:
			// 			log.Printf("Not comprehended procedure code of SuccessfulOutcome (criticality: reject, procedureCode:0x%02x)", successfulOutcome.ProcedureCode.Value)
			// 			cause.Protocol.Value = ngapType.CauseProtocolPresentAbstractSyntaxErrorReject
			// 		case ngapType.CriticalityPresentIgnore:
			// 			log.Printf("Not comprehended procedure code of SuccessfulOutcome (criticality: ignore, procedureCode:0x%02x)", successfulOutcome.ProcedureCode.Value)
			// 			return
			// 		case ngapType.CriticalityPresentNotify:
			// 			log.Printf("Not comprehended procedure code of SuccessfulOutcome (criticality: notify, procedureCode:0x%02x)", successfulOutcome.ProcedureCode.Value)
			// 			cause.Protocol.Value = ngapType.CauseProtocolPresentAbstractSyntaxErrorIgnoreAndNotify
			// 		}
			// 		triggeringMessage := ngapType.TriggeringMessagePresentSuccessfulOutcome
			// 		criticalityDiagnostics := buildCriticalityDiagnostics(&successfulOutcome.ProcedureCode.Value, &triggeringMessage, &successfulOutcome.Criticality.Value, nil)
			// 		ngap_message.SendErrorIndication(ran, nil, nil, &cause, &criticalityDiagnostics)
			// 	}
			// case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
			// 	unsuccessfulOutcome := message.UnsuccessfulOutcome
			// 	if unsuccessfulOutcome == nil {
			// 		Log.Errorln("UnsuccessfulOutcome is nil")
			// 		return
			// 	}
			// 	switch unsuccessfulOutcome.ProcedureCode.Value {
			// 	case ngapType.ProcedureCodeAMFConfigurationUpdate:
			// 		handlerAMFConfigurationUpdateFailure(ran, unsuccessfulOutcome)
			// 	case ngapType.ProcedureCodeHandoverResourceAllocation:
			// 		handlerHandoverFailure(ran, unsuccessfulOutcome)
			// 	case ngapType.ProcedureCodeHandoverPreparation:
			// 		handlerHandoverPreparationFailure(ran, unsuccessfulOutcome)
			// 	case ngapType.ProcedureCodeInitialContextSetup:
			// 		handlerInitialContextSetupFailure(ran, unsuccessfulOutcome)
			// 	case ngapType.ProcedureCodeNGSetup:
			// 		handlerNGSetupFailure(ran, unsuccessfulOutcome)
			// 	case ngapType.ProcedureCodePathSwitchRequest:
			// 		handlerPathSwitchRequestFailure(ran, unsuccessfulOutcome)
			// 	case ngapType.ProcedureCodeRANConfigurationUpdate:
			// 		handlerRANConfigurationUpdateFailure(ran, unsuccessfulOutcome)
			// 	case ngapType.ProcedureCodeUEContextModification:
			// 		handlerUEContextModificationFailure(ran, unsuccessfulOutcome)
			// 	default:
			// 		cause := ngapType.Cause{
			// 			Present:  ngapType.CausePresentProtocol,
			// 			Protocol: &ngapType.CauseProtocol{},
			// 		}
			// 		switch unsuccessfulOutcome.Criticality.Value {
			// 		case ngapType.CriticalityPresentReject:
			// 			Log.Errorf("Not comprehended procedure code of UnsuccessfulOutcome (criticality: reject, procedureCode:0x%02x)", unsuccessfulOutcome.ProcedureCode.Value)
			// 			cause.Protocol.Value = ngapType.CauseProtocolPresentAbstractSyntaxErrorReject
			// 		case ngapType.CriticalityPresentIgnore:
			// 			Log.Infof("Not comprehended procedure code of UnsuccessfulOutcome (criticality: ignore, procedureCode:0x%02x)", unsuccessfulOutcome.ProcedureCode.Value)
			// 			return
			// 		case ngapType.CriticalityPresentNotify:
			// 			Log.Warnf("Not comprehended procedure code of UnsuccessfulOutcome (criticality: notify, procedureCode:0x%02x)", unsuccessfulOutcome.ProcedureCode.Value)
			// 			cause.Protocol.Value = ngapType.CauseProtocolPresentAbstractSyntaxErrorIgnoreAndNotify
			// 		}
			// 		triggeringMessage := ngapType.TriggeringMessagePresentUnsuccessfullOutcome
			// 		criticalityDiagnostics := buildCriticalityDiagnostics(&unsuccessfulOutcome.ProcedureCode.Value, &triggeringMessage, &unsuccessfulOutcome.Criticality.Value, nil)
			// 		ngap_message.SendErrorIndication(ran, nil, nil, &cause, &criticalityDiagnostics)
		}
	}
}

func handlerNGSetupRequest(ran *AmfRan, initiatingMessage *ngapType.InitiatingMessage) {
	if initiatingMessage.Value.NGSetupRequest == nil {
		ran.Log.Errorln("NGSetupRequest is nil")
		return
	}

	ran.Log.Infoln("Handling NGSetupRequest...")
	// Here you would typically process the NGSetupRequest and send a response
	// For demonstration, we will just print the request
	ran.Log.Infof("NGSetupRequest: %+v\n", initiatingMessage.Value.NGSetupRequest)

	// Simulate sending a response
	// response := &ngapType.SuccessfulOutcome{
	// 	ProcedureCode: ngapType.ProcedureCode{
	// 		Value: ngapType.ProcedureCodeNGSetup,
	// 	},
	// 	Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
	// }
	// fmt.Printf("Sending NGSetupResponse: %+v\n", response)

	handleNGSetupRequestMain(ran, initiatingMessage.Value.NGSetupRequest)
}

func handleNGSetupRequestMain(ran *AmfRan, nGSetupRequest *ngapType.NGSetupRequest) {
	ran.Log.Infoln("Send NG-Setup response")

	pkt, err := BuildNGSetupResponse()
	if err != nil {
		ran.Log.Errorf("Build NGSetupResponse failed : %s\n", err.Error())
		return
	}

	// TODO: Pass the correct ran instance here
	SendToRan(ran, pkt)
}

func BuildNGSetupResponse() ([]byte, error) {
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

	aMFName := ie.Value.AMFName
	aMFName.Value = proxySelf.Name

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
		servedGUAMIItem.GUAMI.PLMNIdentity = ngapConvert.PlmnIdToNgap(PlmnIdNidToModelsPlmnId(*guami.PlmnId))
		regionId, setId, prtId := ngapConvert.AmfIdToNgap(guami.AmfId)
		servedGUAMIItem.GUAMI.AMFRegionID.Value = regionId
		servedGUAMIItem.GUAMI.AMFSetID.Value = setId
		servedGUAMIItem.GUAMI.AMFPointer.Value = prtId
		servedGUAMIList.List = append(servedGUAMIList.List, servedGUAMIItem)
	}

	nGSetupResponseIEs.List = append(nGSetupResponseIEs.List, ie)

	// relativeAMFCapacity
	ie = ngapType.NGSetupResponseIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDRelativeAMFCapacity
	ie.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie.Value.Present = ngapType.NGSetupResponseIEsPresentRelativeAMFCapacity
	ie.Value.RelativeAMFCapacity = new(ngapType.RelativeAMFCapacity)
	relativeAMFCapacity := ie.Value.RelativeAMFCapacity
	relativeAMFCapacity.Value = proxySelf.RelativeCapacity

	nGSetupResponseIEs.List = append(nGSetupResponseIEs.List, ie)

	// ServedGUAMIList
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
func SendToRan(ran *AmfRan, packet []byte) {
	defer func() {
		// This is workaround.
		// TODO: Handle ran.Conn close event correctly
		err := recover()
		if err != nil {
			logger.NgapLog.Warnf("Send error, gNB may have been lost: %+v", err)
		}
	}()

	// if ran == nil {
	// 	logger.NgapLog.Error("Ran is nil")
	// 	return
	// }

	// if len(packet) == 0 {
	// 	ran.Log.Error("packet len is 0")
	// 	return
	// }

	// if ran.Conn == nil {
	// 	ran.Log.Error("Ran conn is nil")
	// 	return
	// }

	// if ran.Conn.RemoteAddr() == nil {
	// 	ran.Log.Error("Ran addr is nil")
	// 	return
	// }

	ran.Log.Debugf("Send NGAP message To Ran")

	if n, err := ran.Conn.Write(packet); err != nil {
		ran.Log.Errorf("Send error: %+v", err)
		return
	} else {
		ran.Log.Debugf("Write %d bytes", n)
	}
}

func PlmnIdNidToModelsPlmnId(plmnIdNid models.PlmnIdNid) (plmnId models.PlmnId) {
	plmnId.Mcc = plmnIdNid.Mcc
	plmnId.Mnc = plmnIdNid.Mnc
	return
}

func handlerNGSetupResponse(ran *AmfRan, successfulOutcome *ngapType.SuccessfulOutcome) {
	panic("unimplemented")
}
