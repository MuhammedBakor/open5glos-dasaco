package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"
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

// AmfRan represents a RAN node in the network, specifically for Load balancer.
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

// ProxyContext represents the context for a proxy work with gNB RAN nodes.
type ProxyContext struct {
	Name             string
	ServedGUAMIList  []models.Guami
	RelativeCapacity int64
	NgapIPList       []string
	NgapPort         int
	PlmnSupportList  []PlmnSupportItem
	// Act as RAN
	// Mapbinding map[string]*ProxyRan
}

// ProxyRAN represents a proxy as a gNB RAN node for AMF in the network.
type ProxyRan struct {
	Name               string
	NRCellIdentifier   string
	PlmnRANSupportList []PlmnSupportItem
	idlength           int64
	tac                int64
	RanPresent         int
	RanId              *models.GlobalRanNodeId
	SupportedTAList    []SupportedTAI
	DefaultPagingDRX   string
	UERetentionInfo    string
	AnType             models.AccessType
	RanUeList          sync.Map // RanUeNgapId as key
	IPaddress          string
	Port               int
	Log                *log.Logger
}

type SupportedTAI struct {
	Tai        models.Tai
	SNssaiList []models.Snssai
}

type PlmnSupportItem struct {
	PlmnId     *models.PlmnId
	SNssaiList []models.Snssai
}

type AMFInfo struct {
	PodName      string
	NodeIP       string
	InternalPort int32
	NodePort     int32
}

// RAN configuration for ProxyRAN
var proxyRan = &ProxyRan{
	Name:             "ProxyRan",
	NRCellIdentifier: "00000001", // Example NR Cell ID
	PlmnRANSupportList: []PlmnSupportItem{
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
	idlength:   32,
	tac:        1,
	RanPresent: 1,
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
	SupportedTAList: []SupportedTAI{
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
	DefaultPagingDRX: "v128", // Example value, adjust as needed
	UERetentionInfo:  "true", // Example value, adjust as needed
	AnType:           models.AccessType__3_GPP_ACCESS,
	IPaddress:        "0.0.0.0", // Change as needed
	Port:             38412,     // Default NGAP SCTP port
	Log:              log.Default(),
}

// ProxySelf context configuration for gNB RAN nodes
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
				{
					Sst: 1,
					Sd:  "112233",
				},
			},
		},
	},
}

func getMinikubeIP() (string, error) {
	out, err := exec.Command("minikube", "ip").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getAMFPodsAndPorts(clientset *kubernetes.Clientset, namespace string) ([]AMFInfo, error) {
	var amfs []AMFInfo

	// List pods with label "nf=amf"
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "nf=amf",
	})
	if err != nil {
		return nil, err
	}

	// List services with label "nf=amf"
	svcs, err := clientset.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{
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

	// Map pod name to internal port and nodeport
	for _, svc := range svcs.Items {
		for _, port := range svc.Spec.Ports {
			if port.Name == "sctp" || port.Protocol == v1.ProtocolSCTP {
				// Try to find matching pod
				for range svc.Spec.Selector {
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
	}
	return amfs, nil
}

func printAMFs(minikubeIP string, amfs []AMFInfo) {
	fmt.Println("[INFO] Minikube Node IP:", minikubeIP)
	for _, amf := range amfs {
		fmt.Printf("[INFO] AMF Pod: %s, NodeIP: %s, InternalPort: %d, NodePort: %d\n",
			amf.PodName, amf.NodeIP, amf.InternalPort, amf.NodePort)
	}
}

func sendandreceiveNGSetup(proxy *ProxyRan, amf AMFInfo, minikubeIP string, wg *sync.WaitGroup) {
	info := &sctp.SndRcvInfo{
		PPID: 60,
	}

	defer wg.Done()
	addr := fmt.Sprintf("%s:%d", minikubeIP, amf.NodePort)
	raddr, err := sctp.ResolveSCTPAddr("sctp", addr)
	if err != nil {
		log.Printf("[ERROR] ResolveSCTPAddr error for %s: %v", addr, err)
		return
	}
	// Set up local SCTP address if proxyRan.IPaddress is not "0.0.0.0"
	var laddr *sctp.SCTPAddr
	if proxy.IPaddress != "" && proxy.IPaddress != "0.0.0.0" {
		laddr, err = sctp.ResolveSCTPAddr("sctp", fmt.Sprintf("%s:0", proxy.IPaddress))
		if err != nil {
			log.Printf("[ERROR] ResolveSCTPAddr (local) error for %s: %v", proxy.IPaddress, err)
			return
		}
	}
	conn, err := sctp.DialSCTP("sctp", laddr, raddr)
	if err != nil {
		log.Printf("[ERROR] SCTP dial error for %s: %v", addr, err)
		return
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	pkt, err := BuildNGSetupRequest(proxyRan)
	if err != nil {
		log.Printf("[ERROR] Build NGSetupRequest failed : %s\n", err.Error())
		return
	}

	n, err := conn.SCTPWrite(pkt, info)
	if err != nil {
		log.Printf("[INFO] SCTP write error for %s: %v", addr, err)
		return
	}
	log.Printf("[INFO] Sent NGSetupRequest (%d bytes) to %s", n, addr)

	buf := make([]byte, 4096)
	// Keep reading until connection is closed or error occurs
	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Printf("[ERROR] Read error: %v", err)
			return
		}

		fmt.Printf("[INFO] Received %d bytes from gNB\n", n)
		ngapMsg, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[ERROR] NGAP decode error: %v", err)
			continue // Continue reading instead of returning
		}

		if ngapMsg == nil {
			log.Printf("[ERROR] NGAP Message is nil")
			continue // Continue reading instead of returning
		}

		fmt.Println("[MESS] Decoded NGAP message:")
		messageHandling(&amf, ngapMsg)
	}
}

func messageHandling(aMFInfo *AMFInfo, ngapMsg *ngapType.NGAPPDU) {
	switch ngapMsg.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := ngapMsg.InitiatingMessage
		if initiatingMessage == nil {
			log.Printf("[ERROR] InitiatingMessage is nil")
			return
		}
		switch initiatingMessage.ProcedureCode.Value {
		}
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		successfulOutcome := ngapMsg.SuccessfulOutcome
		if successfulOutcome == nil {
			log.Println("[ERROR] SuccessfulOutcome is nil")
			return
		}
		switch successfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			handlingNGSetupResponse(&aMFInfo, successfulOutcome)
		}
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		unsuccessfulOutcome := ngapMsg.UnsuccessfulOutcome
		if unsuccessfulOutcome == nil {
			log.Println("[ERROR] UnsuccessfulOutcome is nil")
			return
		}
		switch unsuccessfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			handlingNGSetupFailure(&aMFInfo, unsuccessfulOutcome)
		}
	}

}

func handlingNGSetupFailure(aMFInfo **AMFInfo, unsuccessfulOutcome *ngapType.UnsuccessfulOutcome) {
	log.Printf("[INFO] NGSetupResponse: %+v", unsuccessfulOutcome.Value.NGSetupFailure)
	log.Println("[INFO] Received NGSetupResponse")
}

func handlingNGSetupResponse(aMFInfo **AMFInfo, successfulOutcome *ngapType.SuccessfulOutcome) {
	log.Printf("[INFO] NGSetupResponse: %+v", successfulOutcome.Value.NGSetupResponse)
	log.Println("[INFO] Received NGSetupResponse")
}

func BuildNGSetupRequest(proxy *ProxyRan) ([]byte, error) {
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

	// Use PLMN from proxy configuration instead of hardcoded bytes
	globalRANNodeID.GlobalGNBID.PLMNIdentity = ngapConvert.PlmnIdToNgap(*proxy.RanId.PlmnId)

	// Properly configure GNBID
	globalRANNodeID.GlobalGNBID.GNBID.Present = ngapType.GNBIDPresentGNBID
	globalRANNodeID.GlobalGNBID.GNBID.GNBID = &aper.BitString{
		Bytes:     []byte(proxy.RanId.GNbId.GNBValue),
		BitLength: uint64(proxy.RanId.GNbId.BitLength),
	}

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie)

	// 2. RAN Node Name
	ie2 := ngapType.NGSetupRequestIEs{}
	ie2.Id.Value = ngapType.ProtocolIEIDRANNodeName
	ie2.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie2.Value.Present = ngapType.NGSetupRequestIEsPresentRANNodeName
	ie2.Value.RANNodeName = new(ngapType.RANNodeName)
	ie2.Value.RANNodeName.Value = proxy.Name

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie2)

	// // 3. SupportedTAList
	ie3 := ngapType.NGSetupRequestIEs{}
	ie3.Id.Value = ngapType.ProtocolIEIDSupportedTAList
	ie3.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie3.Value.Present = ngapType.NGSetupRequestIEsPresentSupportedTAList
	ie3.Value.SupportedTAList = new(ngapType.SupportedTAList)
	supportedTAList := ie3.Value.SupportedTAList

	// supportedTAList.List = make([]ngapType.SupportedTAItem, 0, len(proxy.SupportedTAList))

	for _, tai := range proxy.SupportedTAList {
		taiIE := ngapType.SupportedTAItem{}
		var tacBytes []byte

		// Check if TAC is already raw bytes or hex string
		if len(tai.Tai.Tac) == 3 && (tai.Tai.Tac[0] == 0 || tai.Tai.Tac[1] == 0 || tai.Tai.Tac[2] == 0) {
			// TAC is raw bytes, use directly
			tacBytes = []byte(tai.Tai.Tac)
		} else {
			// TAC is hex string, decode it
			var err error
			tacBytes, err = hex.DecodeString(tai.Tai.Tac)
			if err != nil {
				return nil, fmt.Errorf("[ERROR] invalid TAC format: %v", err)
			}
		}

		// Ensure TAC is exactly 3 bytes
		if len(tacBytes) == 0 {
			tacBytes = []byte{0x00, 0x00, 0x01} // Default TAC
		} else if len(tacBytes) < 3 {
			// Pad with leading zeros if needed
			paddedTac := make([]byte, 3)
			copy(paddedTac[3-len(tacBytes):], tacBytes)
			tacBytes = paddedTac
		} else if len(tacBytes) > 3 {
			// Truncate to 3 bytes if too long
			tacBytes = tacBytes[:3]
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

		// Add SNSSAI items to TAISliceSupportList
		for _, snssai := range tai.SNssaiList {
			sliceSupportItem := ngapType.SliceSupportItem{
				SNSSAI: ngapType.SNSSAI{
					SST: ngapType.SST{
						Value: aper.OctetString{byte(snssai.Sst)},
					},
				},
			}

			// Add SD if present
			if snssai.Sd != "" {
				sdBytes, err := hex.DecodeString(snssai.Sd)
				if err != nil {
					return nil, fmt.Errorf("[ERROR] invalid SD format: %v", err)
				}
				// Ensure SD is exactly 3 bytes
				if len(sdBytes) < 3 {
					paddedSd := make([]byte, 3)
					copy(paddedSd[3-len(sdBytes):], sdBytes)
					sdBytes = paddedSd
				} else if len(sdBytes) > 3 {
					sdBytes = sdBytes[:3]
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

	// // 4. DefaultPagingDRX
	ie4 := ngapType.NGSetupRequestIEs{}
	ie4.Id.Value = ngapType.ProtocolIEIDDefaultPagingDRX
	ie4.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie4.Value.Present = ngapType.NGSetupRequestIEsPresentDefaultPagingDRX
	ie4.Value.DefaultPagingDRX = new(ngapType.PagingDRX)

	// Set DefaultPagingDRX based on proxy configuration
	switch proxy.DefaultPagingDRX {
	case "v128":
		ie4.Value.DefaultPagingDRX = &ngapType.PagingDRX{Value: ngapType.PagingDRXPresentV128}
	case "v64":
		ie4.Value.DefaultPagingDRX = &ngapType.PagingDRX{Value: ngapType.PagingDRXPresentV64}
	case "v32":
		ie4.Value.DefaultPagingDRX = &ngapType.PagingDRX{Value: ngapType.PagingDRXPresentV32}
	case "v256":
		ie4.Value.DefaultPagingDRX = &ngapType.PagingDRX{Value: ngapType.PagingDRXPresentV256}
	default:
		ie4.Value.DefaultPagingDRX = &ngapType.PagingDRX{Value: ngapType.PagingDRXPresentV128}
	}
	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie4)

	// Encode the PDU
	return ngap.Encoder(pdu)
}

func handleConnection(ran *AmfRan, conn *sctp.SCTPConn) {
	defer conn.Close()
	fmt.Printf("[INFO] Accepted connection from %v\n", conn.RemoteAddr())

	buf := make([]byte, 4096)

	// Keep reading until connection is closed or error occurs
	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Printf("[ERROR] Read error: %v", err)
			return
		}

		fmt.Printf("Received %d bytes from gNB\n", n)
		ngapMsg, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[ERROR] NGAP decode error: %v", err)
			continue // Continue reading instead of returning
		}

		if ngapMsg == nil {
			log.Printf("[ERROR] NGAP Message is nil")
			continue // Continue reading instead of returning
		}

		fmt.Println("[MESS] Decoded NGAP message:")
		dispatchMain(ran, ngapMsg)
		// fmt.Println("NGAP message dispatched successfully!")
	}
}

func dispatchMain(ran *AmfRan, message *ngapType.NGAPPDU) {
	switch message.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := message.InitiatingMessage
		if initiatingMessage == nil {
			log.Printf("[ERROR] InitiatingMessage is nil")
			return
		}
		switch initiatingMessage.ProcedureCode.Value {
		case ngapType.ProcedureCodeInitialContextSetup:
			handlerInitialContextSetupRequest(ran, initiatingMessage)
		case ngapType.ProcedureCodeNGSetup:
			handlerNGSetupRequest(ran, initiatingMessage)
		}
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		successfulOutcome := message.SuccessfulOutcome
		if successfulOutcome == nil {
			log.Println("[ERROR] SuccessfulOutcome is nil")
			return
		}
		switch successfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			handlerNGSetupResponse(ran, successfulOutcome)
		}
	}
}

func handlerInitialContextSetupRequest(ran *AmfRan, initiatingMessage *ngapType.InitiatingMessage) {
	initialContextSetupRequest := initiatingMessage.Value.InitialContextSetupRequest
	if initialContextSetupRequest == nil {
		ran.Log.Error("[ERROR] InitialContextSetupRequest is nil")
		return
	}

	ran.Log.Info("[INFO] Handle InitialContextSetupRequest")
}

func handlerNGSetupRequest(ran *AmfRan, initiatingMessage *ngapType.InitiatingMessage) {
	if initiatingMessage.Value.NGSetupRequest == nil {
		ran.Log.Errorln("[ERROR] NGSetupRequest is nil")
		return
	}

	ran.Log.Infoln("[INFO] Handling NGSetupRequest...")

	ran.Log.Infof("[MESS] NGSetupRequest: %+v\n", initiatingMessage.Value.NGSetupRequest)

	handleNGSetupRequestMain(ran, initiatingMessage.Value.NGSetupRequest)
}

func handleNGSetupRequestMain(ran *AmfRan, nGSetupRequest *ngapType.NGSetupRequest) {
	ran.Log.Infoln("[INFO] Send NG-Setup response")

	pkt, err := BuildNGSetupResponse()
	if err != nil {
		ran.Log.Errorf("[ERROR] Build NGSetupResponse failed : %s\n", err.Error())
		return
	}
	// ran.Log.Infof("NGSetupResponse: %s", pkt)

	// TODO: Pass the correct ran instance here
	SendToRan(ran, pkt)
	ran.Log.Infoln("[INFO] Sent NGSetup Response to RAN")
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
	nGSetupResponse := successfulOutcome.Value.NGSetupResponse
	if nGSetupResponse == nil {
		ran.Log.Error("NGSetupResponse is nil")
		return
	}

	ran.Log.Info("Handle NGSetupResponse")

	handleNGSetupResponseMain(ran, nGSetupResponse)
}

func handleNGSetupResponseMain(ran *AmfRan, nGSetupResponse *ngapType.NGSetupResponse) {
	ran.Log.Error("Handle NGSetupResponse: AMF to RAN message")
}

type AMFConnection struct {
	Info     AMFInfo
	Conn     *sctp.SCTPConn
	LastSeen time.Time
	mutex    sync.RWMutex
}

type ProxyServer struct {
	listener   *sctp.SCTPListener
	amfConns   map[string]*AMFConnection
	ranConns   map[string]*AmfRan
	mutex      sync.RWMutex
	clientset  *kubernetes.Clientset
	namespace  string
	minikubeIP string
}

func NewProxyServer(namespace string) (*ProxyServer, error) {
	// Set up SCTP listener
	laddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("127.0.0.10")}},
		Port:    38412,
	}
	listener, err := sctp.ListenSCTP("sctp", laddr)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to listen SCTP: %v", err)
	}

	// Initialize Kubernetes client
	minikubeIP, err := getMinikubeIP()
	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to get minikube IP: %v", err)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		// fallback to kubeconfig
		kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("[ERROR] failed to get kubeconfig: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to create k8s client: %v", err)
	}

	return &ProxyServer{
		listener:   listener,
		amfConns:   make(map[string]*AMFConnection),
		ranConns:   make(map[string]*AmfRan),
		clientset:  clientset,
		namespace:  namespace,
		minikubeIP: minikubeIP,
	}, nil
}

func (ps *ProxyServer) Start() error {
	fmt.Println("[INFO] Proxy SCTP server listening on 127.0.0.10:38412")

	// Start AMF connection manager
	go ps.manageAMFConnections()

	// Start accepting gNB connections
	go ps.acceptGNBConnections()

	// Keep the main goroutine alive
	select {}
}

func (ps *ProxyServer) manageAMFConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ps.refreshAMFConnections()
		}
	}
}

func (ps *ProxyServer) refreshAMFConnections() {
	amfs, err := getAMFPodsAndPorts(ps.clientset, ps.namespace)
	if err != nil {
		log.Printf("[ERROR] Failed to get AMF pods: %v", err)
		return
	}

	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	// Update existing connections and add new ones
	for _, amf := range amfs {
		key := fmt.Sprintf("%s:%d", amf.PodName, amf.NodePort)

		if conn, exists := ps.amfConns[key]; exists {
			// Update existing connection info
			conn.Info = amf
			conn.LastSeen = time.Now()
		} else {
			// Create new connection
			ps.connectToAMF(amf)
		}
	}

	// Remove stale connections
	ps.cleanupStaleAMFConnections()
}

func (ps *ProxyServer) connectToAMF(amf AMFInfo) {
	addr := fmt.Sprintf("%s:%d", ps.minikubeIP, amf.NodePort)
	raddr, err := sctp.ResolveSCTPAddr("sctp", addr)
	if err != nil {
		log.Printf("[ERROR] ResolveSCTPAddr error for %s: %v", addr, err)
		return
	}

	conn, err := sctp.DialSCTP("sctp", nil, raddr)
	if err != nil {
		log.Printf("[ERROR] SCTP dial error for %s: %v", addr, err)
		return
	}

	key := fmt.Sprintf("%s:%d", amf.PodName, amf.NodePort)
	amfConn := &AMFConnection{
		Info:     amf,
		Conn:     conn,
		LastSeen: time.Now(),
	}

	ps.amfConns[key] = amfConn
	log.Printf("[INFO] Connected to AMF: %s", key)

	// Start goroutine to handle AMF messages
	go ps.handleAMFConnection(amfConn)

	// Send initial NG Setup Request
	go ps.sendNGSetupRequest(amfConn)
}

func (ps *ProxyServer) handleAMFConnection(amfConn *AMFConnection) {
	defer func() {
		amfConn.Conn.Close()
		ps.mutex.Lock()
		key := fmt.Sprintf("%s:%d", amfConn.Info.PodName, amfConn.Info.NodePort)
		delete(ps.amfConns, key)
		ps.mutex.Unlock()
		log.Printf("[INFO] AMF connection closed: %s", key)
	}()

	buf := make([]byte, 4096)
	for {
		amfConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		n, err := amfConn.Conn.Read(buf)
		if err != nil {
			log.Printf("[ERROR] AMF read error: %v", err)
			return
		}

		amfConn.mutex.Lock()
		amfConn.LastSeen = time.Now()
		amfConn.mutex.Unlock()

		// Decode NGAP message
		ngapMsg, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[ERROR] NGAP decode error from AMF: %v", err)
			continue
		}

		log.Printf("[INFO] Received %d bytes from AMF %s", n, amfConn.Info.PodName)

		// Forward message to appropriate gNB or handle internally
		ps.forwardToGNB(ngapMsg, buf[:n])
	}
}

func (ps *ProxyServer) sendNGSetupRequest(amfConn *AMFConnection) {
	info := &sctp.SndRcvInfo{PPID: 60}

	pkt, err := BuildNGSetupRequest(proxyRan)
	if err != nil {
		log.Printf("[ERROR] Build NGSetupRequest failed: %v", err)
		return
	}

	amfConn.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	n, err := amfConn.Conn.SCTPWrite(pkt, info)
	if err != nil {
		log.Printf("[ERROR] SCTP write error to AMF: %v", err)
		return
	}

	log.Printf("[INFO] Sent NGSetupRequest (%d bytes) to AMF %s", n, amfConn.Info.PodName)
}

func (ps *ProxyServer) acceptGNBConnections() {
	for {
		conn, err := ps.listener.AcceptSCTP()
		if err != nil {
			log.Printf("[ERROR] Accept error: %v", err)
			continue
		}

		// Create RAN instance
		ran := &AmfRan{
			Conn: conn,
			Name: fmt.Sprintf("RAN-%v", conn.RemoteAddr()),
			Log:  logrus.NewEntry(logrus.New()),
		}

		ps.mutex.Lock()
		ps.ranConns[ran.Name] = ran
		ps.mutex.Unlock()

		log.Printf("[INFO] Accepted gNB connection from %v", conn.RemoteAddr())
		go ps.handleGNBConnection(ran)
	}
}

func (ps *ProxyServer) handleGNBConnection(ran *AmfRan) {
	defer func() {
		ran.Conn.Close()
		ps.mutex.Lock()
		delete(ps.ranConns, ran.Name)
		ps.mutex.Unlock()
		log.Printf("[INFO] gNB connection closed: %s", ran.Name)
	}()

	buf := make([]byte, 4096)
	for {
		ran.Conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5 minute timeout
		n, err := ran.Conn.Read(buf)
		if err != nil {
			log.Printf("[ERROR] gNB read error: %v", err)
			return
		}

		log.Printf("[INFO] Received %d bytes from gNB %s", n, ran.Name)

		// Decode NGAP message
		ngapMsg, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[ERROR] NGAP decode error from gNB: %v", err)
			continue
		}

		// Handle message or forward to AMF
		ps.handleGNBMessage(ran, ngapMsg, buf[:n])
	}
}

func (ps *ProxyServer) handleGNBMessage(ran *AmfRan, ngapMsg *ngapType.NGAPPDU, rawMsg []byte) {
	switch ngapMsg.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := ngapMsg.InitiatingMessage
		if initiatingMessage == nil {
			return
		}

		switch initiatingMessage.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			// Handle NG Setup locally and respond
			ps.handleNGSetupFromGNB(ran, initiatingMessage)
		default:
			// Forward other messages to AMF
			ps.forwardToAMF(rawMsg)
		}
	default:
		// Forward other message types to AMF
		ps.forwardToAMF(rawMsg)
	}
}

func (ps *ProxyServer) handleNGSetupFromGNB(ran *AmfRan, initiatingMessage *ngapType.InitiatingMessage) {
	ran.Log.Infoln("[INFO] Handling NGSetupRequest from gNB...")

	// Build and send response
	pkt, err := BuildNGSetupResponse()
	if err != nil {
		ran.Log.Errorf("[ERROR] Build NGSetupResponse failed: %v", err)
		return
	}

	SendToRan(ran, pkt)
	ran.Log.Infoln("[INFO] Sent NGSetup Response to gNB")
}

func (ps *ProxyServer) forwardToAMF(message []byte) {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	// Simple load balancing - forward to first available AMF
	for _, amfConn := range ps.amfConns {
		amfConn.mutex.RLock()
		if time.Since(amfConn.LastSeen) < 60*time.Second {
			amfConn.mutex.RUnlock()

			info := &sctp.SndRcvInfo{PPID: 60}
			_, err := amfConn.Conn.SCTPWrite(message, info)
			if err != nil {
				log.Printf("[ERROR] Forward to AMF error: %v", err)
				continue
			}
			log.Printf("[INFO] Forwarded message to AMF %s", amfConn.Info.PodName)
			return
		}
		amfConn.mutex.RUnlock()
	}
	log.Printf("[ERROR] No available AMF to forward message")
}

func (ps *ProxyServer) forwardToGNB(ngapMsg *ngapType.NGAPPDU, rawMsg []byte) {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	// Broadcast to all connected gNBs or implement specific routing logic
	for _, ran := range ps.ranConns {
		_, err := ran.Conn.Write(rawMsg)
		if err != nil {
			log.Printf("[INFO] Forward to gNB error: %v", err)
			continue
		}
		log.Printf("[INFO] Forwarded message to gNB %s", ran.Name)
	}
}

func (ps *ProxyServer) cleanupStaleAMFConnections() {
	now := time.Now()
	for key, conn := range ps.amfConns {
		conn.mutex.RLock()
		if now.Sub(conn.LastSeen) > 120*time.Second { // 2 minutes timeout
			conn.mutex.RUnlock()
			conn.Conn.Close()
			delete(ps.amfConns, key)
			log.Printf("[INFO] Removed stale AMF connection: %s", key)
		} else {
			conn.mutex.RUnlock()
		}
	}
}

func (ps *ProxyServer) Close() {
	ps.listener.Close()

	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	// Close all AMF connections
	for _, amfConn := range ps.amfConns {
		amfConn.Conn.Close()
	}

	// Close all gNB connections
	for _, ran := range ps.ranConns {
		ran.Conn.Close()
	}
}

func main() {
	namespace := "free5gc" // Change to your namespace

	proxy, err := NewProxyServer(namespace)
	if err != nil {
		log.Fatalf("[ERROR] Failed to create proxy server: %v", err)
	}
	defer proxy.Close()

	// Start the proxy server (this will block)
	if err := proxy.Start(); err != nil {
		log.Fatalf("[ERROR] Proxy server failed: %v", err)
	}
}
