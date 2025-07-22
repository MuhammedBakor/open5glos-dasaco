package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapConvert"
	"github.com/free5gc/ngap/ngapType"
	"github.com/free5gc/openapi/models"
	"github.com/ishidawataru/sctp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NGSetupRequest NGAP message (test message)
var NGSetupRequest = []byte{
	0x00, 0x11, 0x22, 0x33, // Replace with real NGAP message
}

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

// Example gNB RAN configuration for UERANSIM with free5gc
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
	fmt.Println("Minikube Node IP:", minikubeIP)
	for _, amf := range amfs {
		fmt.Printf("AMF Pod: %s, NodeIP: %s, InternalPort: %d, NodePort: %d\n",
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
		log.Printf("ResolveSCTPAddr error for %s: %v", addr, err)
		return
	}
	// Set up local SCTP address if proxyRan.IPaddress is not "0.0.0.0"
	var laddr *sctp.SCTPAddr
	if proxy.IPaddress != "" && proxy.IPaddress != "0.0.0.0" {
		laddr, err = sctp.ResolveSCTPAddr("sctp", fmt.Sprintf("%s:0", proxy.IPaddress))
		if err != nil {
			log.Printf("ResolveSCTPAddr (local) error for %s: %v", proxy.IPaddress, err)
			return
		}
	}
	conn, err := sctp.DialSCTP("sctp", laddr, raddr)
	if err != nil {
		log.Printf("SCTP dial error for %s: %v", addr, err)
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
			log.Printf("Read error: %v", err)
			return
		}

		fmt.Printf("Received %d bytes from gNB\n", n)
		ngapMsg, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("NGAP decode error: %v", err)
			continue // Continue reading instead of returning
		}

		if ngapMsg == nil {
			log.Printf("NGAP Message is nil")
			continue // Continue reading instead of returning
		}

		fmt.Println("Decoded NGAP message:")
		messageHandling(&amf, ngapMsg)
	}
}

func messageHandling(aMFInfo *AMFInfo, ngapMsg *ngapType.NGAPPDU) {
	switch ngapMsg.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := ngapMsg.InitiatingMessage
		if initiatingMessage == nil {
			log.Printf("InitiatingMessage is nil")
			return
		}
		switch initiatingMessage.ProcedureCode.Value {
		}
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		successfulOutcome := ngapMsg.SuccessfulOutcome
		if successfulOutcome == nil {
			log.Println("SuccessfulOutcome is nil")
			return
		}
		switch successfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			handlingNGSetupResponse(&aMFInfo, successfulOutcome)
		}
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		unsuccessfulOutcome := ngapMsg.UnsuccessfulOutcome
		if unsuccessfulOutcome == nil {
			log.Println("UnsuccessfulOutcome is nil")
			return
		}
		switch unsuccessfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			handlingNGSetupFailure(&aMFInfo, unsuccessfulOutcome)
		}
	}

}

func handlingNGSetupFailure(aMFInfo **AMFInfo, unsuccessfulOutcome *ngapType.UnsuccessfulOutcome) {
	log.Printf("NGSetupResponse: %+v", unsuccessfulOutcome.Value.NGSetupFailure)
	log.Println("Received NGSetupResponse")
}

func handlingNGSetupResponse(aMFInfo **AMFInfo, successfulOutcome *ngapType.SuccessfulOutcome) {
	log.Printf("NGSetupResponse: %+v", successfulOutcome.Value.NGSetupResponse)
	log.Println("Received NGSetupResponse")
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
				return nil, fmt.Errorf("invalid TAC format: %v", err)
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
					return nil, fmt.Errorf("invalid SD format: %v", err)
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

func main() {
	namespace := "free5gc" // Change to your namespace
	minikubeIP, err := getMinikubeIP()
	if err != nil {
		log.Fatalf("Failed to get minikube IP: %v", err)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		// fallback to kubeconfig
		kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Fatalf("Failed to get kubeconfig: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create k8s client: %v", err)
	}

	amfs, err := getAMFPodsAndPorts(clientset, namespace)
	if err != nil {
		log.Fatalf("Failed to get AMF pods: %v", err)
	}

	printAMFs(minikubeIP, amfs)

	var wg sync.WaitGroup
	for _, amf := range amfs {
		wg.Add(1)
		go sendandreceiveNGSetup(proxyRan, amf, minikubeIP, &wg)
	}
	wg.Wait()
}
