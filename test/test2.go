package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ishidawataru/sctp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// NGAP PDU Types and Procedure Codes
const (
	// PDU Choice Types
	NGAPInitiatingMessage   = 0x00
	NGAPSuccessfulOutcome   = 0x01
	NGAPUnsuccessfulOutcome = 0x02

	// NG Setup Procedure Codes
	NGSetupRequestProcCode  = 0x15 // 21
	NGSetupResponseProcCode = 0x15 // 21
	NGSetupFailureProcCode  = 0x15 // 21

	// UE Registration Procedure Codes
	InitialUEMessageProcCode            = 0x0F // 15
	DownlinkNASTransportProcCode        = 0x04 // 4
	UplinkNASTransportProcCode          = 0x2E // 46
	InitialContextSetupRequestProcCode  = 0x0E // 14
	InitialContextSetupResponseProcCode = 0x0E // 14
	InitialContextSetupFailureProcCode  = 0x0E // 14
	UEContextReleaseRequestProcCode     = 0x29 // 41
	UEContextReleaseCommandProcCode     = 0x29 // 41
	UEContextReleaseCompleteProcCode    = 0x29 // 41
)

// Proxy config
const (
	gNBListenAddr = "127.0.0.10:38412" // Local SCTP listen for gNB
	amfPort       = 38412              // AMF SCTP port
	amfNodePort   = 30469              // NodePort for AMF
	amfLabelKey   = "nf"
	amfLabelValue = "amf"
)

type AMFEndpoint struct {
	IP   string
	Port int
	Name string
}

type NGAPMessage struct {
	PDUChoice     uint8
	ProcedureCode uint8
	MessageType   string
}

func main() {
	// Get AMF endpoints from Kubernetes
	amfs, err := getAMFEndpoints()
	if err != nil {
		log.Fatalf("Failed to get AMF endpoints: %v", err)
	}
	log.Printf("Discovered AMFs: %+v", amfs)

	// Listen for gNB SCTP connections
	laddr, err := sctp.ResolveSCTPAddr("sctp", gNBListenAddr)
	if err != nil {
		log.Fatalf("Failed to resolve SCTP addr: %v", err)
	}
	ln, err := sctp.ListenSCTP("sctp", laddr)
	if err != nil {
		log.Fatalf("Failed to listen SCTP: %v", err)
	}
	defer ln.Close()
	log.Printf("Listening for gNB on %s", gNBListenAddr)

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Add 5-second timeout
	timeoutCh := time.After(5 * time.Second)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Start accept goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				log.Println("Accept goroutine shutting down...")
				return
			default:
				// AcceptSCTP does not support SetDeadline, so use a goroutine and channel for non-blocking accept
				acceptCh := make(chan *sctp.SCTPConn, 1)
				errCh := make(chan error, 1)
				go func() {
					conn, err := ln.AcceptSCTP()
					if err != nil {
						errCh <- err
						return
					}
					acceptCh <- conn
				}()

				select {
				case <-ctx.Done():
					log.Println("Accept goroutine shutting down...")
					return
				case conn := <-acceptCh:
					log.Printf("Accepted gNB connection from %s", conn.RemoteAddr())
					wg.Add(1)
					go handleGNB(conn, amfs, &wg)
				case err := <-errCh:
					// Only log errors if context is not cancelled
					select {
					case <-ctx.Done():
						return // Context cancelled, exit gracefully
					default:
						log.Printf("Accept error: %v", err)
						continue
					}
				case <-time.After(100 * time.Millisecond):
					// Timeout, loop again to check context
					continue
				}
			}
		}
	}()

	select {
	case <-sigCh:
		log.Println("Shutting down due to signal...")
	case <-timeoutCh:
		log.Println("Shutting down due to 5-second timeout...")
	}

	ln.Close()
	wg.Wait()
	log.Println("Program terminated")
}

func getAMFEndpoints() ([]AMFEndpoint, error) {
	var config *rest.Config
	var err error

	// Try in-cluster config first
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fallback to local kubeconfig
		var kubeconfig string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("could not get k8s config (tried in-cluster and %s): %v", kubeconfig, err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("could not create k8s client: %v", err)
	}

	var amfs []AMFEndpoint

	// Get AMF pods
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", amfLabelKey, amfLabelValue),
	})
	if err != nil {
		return nil, err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Running" {
			amfs = append(amfs, AMFEndpoint{
				IP:   pod.Status.PodIP,
				Port: amfPort,
				Name: pod.Name,
			})
		}
	}

	// Get Minikube Node IP and NodePort
	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err == nil && len(nodes.Items) > 0 {
		nodeIP := ""
		for _, addr := range nodes.Items[0].Status.Addresses {
			if addr.Type == "InternalIP" {
				nodeIP = addr.Address
				break
			}
		}
		if nodeIP != "" {
			amfs = append(amfs, AMFEndpoint{
				IP:   nodeIP,
				Port: amfNodePort,
				Name: "minikube-nodeport",
			})
		}
	}
	return amfs, nil
}

func handleGNB(gnbConn *sctp.SCTPConn, amfs []AMFEndpoint, wg *sync.WaitGroup) {
	defer wg.Done()
	defer gnbConn.Close()

	// Connect to all AMFs
	var amfConns []*sctp.SCTPConn
	var amfWg sync.WaitGroup
	for _, amf := range amfs {
		addr := fmt.Sprintf("%s:%d", amf.IP, amf.Port)
		raddr, err := sctp.ResolveSCTPAddr("sctp", addr)
		if err != nil {
			log.Printf("Failed to resolve AMF addr %s: %v", addr, err)
			continue
		}
		conn, err := sctp.DialSCTP("sctp", nil, raddr)
		if err != nil {
			log.Printf("Failed to connect to AMF %s: %v", addr, err)
			continue
		}
		log.Printf("Connected to AMF %s (%s)", amf.Name, addr)
		amfConns = append(amfConns, conn)
	}

	// Proxy data between gNB and all AMFs
	for _, amfConn := range amfConns {
		amfWg.Add(2)
		go proxyAndIntercept(gnbConn, amfConn, "gNB->AMF", &amfWg)
		go proxyAndIntercept(amfConn, gnbConn, "AMF->gNB", &amfWg)
	}
	amfWg.Wait()
	for _, c := range amfConns {
		c.Close()
	}
}

func proxyAndIntercept(src, dst *sctp.SCTPConn, direction string, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if err != nil {
			log.Printf("[%s] Read error: %v", direction, err)
			return
		}

		// Intercept and analyze NGAP messages
		if ngapMsg := parseNGAPMessage(buf[:n]); ngapMsg != nil {
			log.Printf("[%s] Intercepted NGAP %s (PDU: 0x%02x, Proc: 0x%02x): %x",
				direction, ngapMsg.MessageType, ngapMsg.PDUChoice, ngapMsg.ProcedureCode, buf[:n])

			// Log specific procedure information
			logNGAPProcedureDetails(ngapMsg, direction, buf[:n])
		}

		_, err = dst.Write(buf[:n])
		if err != nil {
			log.Printf("[%s] Write error: %v", direction, err)
			return
		}
	}
}

func parseNGAPMessage(data []byte) *NGAPMessage {
	// Basic NGAP message structure validation
	if len(data) < 3 {
		return nil
	}

	// Check if this looks like an NGAP message (basic heuristic)
	// NGAP messages typically start with specific patterns
	if data[0] > 0x02 {
		return nil // Invalid PDU choice
	}

	pduChoice := data[0]

	// Extract procedure code (usually at offset 2 in the encoded message)
	// This is a simplified extraction - real NGAP parsing would need proper ASN.1 decoding
	var procedureCode uint8
	if len(data) >= 4 {
		// Look for procedure code in typical positions
		for i := 1; i < 10 && i < len(data); i++ {
			if isValidProcedureCode(data[i]) {
				procedureCode = data[i]
				break
			}
		}
	}

	messageType := getMessageTypeName(pduChoice, procedureCode)
	if messageType == "Unknown" && !looksLikeNGAP(data) {
		return nil
	}

	return &NGAPMessage{
		PDUChoice:     pduChoice,
		ProcedureCode: procedureCode,
		MessageType:   messageType,
	}
}

func isValidProcedureCode(code uint8) bool {
	validCodes := []uint8{
		NGSetupRequestProcCode,
		InitialUEMessageProcCode,
		DownlinkNASTransportProcCode,
		UplinkNASTransportProcCode,
		InitialContextSetupRequestProcCode,
		UEContextReleaseRequestProcCode,
	}

	for _, valid := range validCodes {
		if code == valid {
			return true
		}
	}
	return false
}

func looksLikeNGAP(data []byte) bool {
	// Simple heuristic to detect NGAP messages
	// Look for common NGAP patterns or ASN.1 BER/PER encoding indicators
	if len(data) < 2 {
		return false
	}

	// Check for ASN.1 constructed types and reasonable lengths
	return data[0] <= 0x02 && len(data) > 10
}

func getMessageTypeName(pduChoice, procedureCode uint8) string {
	switch pduChoice {
	case NGAPInitiatingMessage:
		switch procedureCode {
		case NGSetupRequestProcCode:
			return "NG Setup Request"
		case InitialUEMessageProcCode:
			return "Initial UE Message"
		case DownlinkNASTransportProcCode:
			return "Downlink NAS Transport"
		case UplinkNASTransportProcCode:
			return "Uplink NAS Transport"
		case InitialContextSetupRequestProcCode:
			return "Initial Context Setup Request"
		case UEContextReleaseRequestProcCode:
			return "UE Context Release Request"
		}
	case NGAPSuccessfulOutcome:
		switch procedureCode {
		case NGSetupResponseProcCode:
			return "NG Setup Response"
		case InitialContextSetupResponseProcCode:
			return "Initial Context Setup Response"
		case UEContextReleaseCompleteProcCode:
			return "UE Context Release Complete"
		}
	case NGAPUnsuccessfulOutcome:
		switch procedureCode {
		case NGSetupFailureProcCode:
			return "NG Setup Failure"
		case InitialContextSetupFailureProcCode:
			return "Initial Context Setup Failure"
		}
	}
	return "Unknown"
}

func logNGAPProcedureDetails(msg *NGAPMessage, direction string, data []byte) {
	switch msg.MessageType {
	case "NG Setup Request":
		log.Printf("[%s] NG SETUP PROCEDURE: gNB initiating setup with AMF", direction)
		log.Printf("[%s] Message contains: Global gNB ID, Supported TA List, Default Paging DRX", direction)

	case "NG Setup Response":
		log.Printf("[%s] NG SETUP PROCEDURE: AMF accepted gNB setup", direction)
		log.Printf("[%s] Message contains: AMF Name, Served GUAMI List, Relative AMF Capacity", direction)

	case "NG Setup Failure":
		log.Printf("[%s] NG SETUP PROCEDURE: AMF rejected gNB setup", direction)
		log.Printf("[%s] Message contains: Cause, Time to Wait, Critical Diagnostics", direction)

	case "Initial UE Message":
		log.Printf("[%s] UE REGISTRATION: Initial UE message from gNB", direction)
		log.Printf("[%s] UE Registration Step 1: NAS Registration Request encapsulated", direction)

	case "Downlink NAS Transport":
		log.Printf("[%s] UE REGISTRATION: AMF->gNB NAS message transport", direction)
		log.Printf("[%s] Likely contains: Authentication Request, Security Mode Command, or Registration Accept", direction)

	case "Uplink NAS Transport":
		log.Printf("[%s] UE REGISTRATION: gNB->AMF NAS message transport", direction)
		log.Printf("[%s] Likely contains: Authentication Response, Security Mode Complete, or Registration Complete", direction)

	case "Initial Context Setup Request":
		log.Printf("[%s] UE REGISTRATION: AMF requesting UE context setup", direction)
		log.Printf("[%s] UE Registration Step: Establishing security context and PDU sessions", direction)

	case "Initial Context Setup Response":
		log.Printf("[%s] UE REGISTRATION: gNB confirmed context setup", direction)
		log.Printf("[%s] UE Registration Complete: UE is now registered and connected", direction)

	case "Initial Context Setup Failure":
		log.Printf("[%s] UE REGISTRATION: gNB failed to setup context", direction)
		log.Printf("[%s] UE Registration Failed: Context establishment failed", direction)

	case "UE Context Release Request":
		log.Printf("[%s] UE DEREGISTRATION: Requesting UE context release", direction)

	case "UE Context Release Complete":
		log.Printf("[%s] UE DEREGISTRATION: UE context released", direction)

	default:
		if msg.MessageType != "Unknown" {
			log.Printf("[%s] NGAP Message: %s", direction, msg.MessageType)
		}
	}

	// Log message size and timing
	log.Printf("[%s] Message size: %d bytes", direction, len(data))
}
