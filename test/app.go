package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/free5gc/ngap"
	"github.com/free5gc/sctp"
)

// startProxyServer starts a SCTP proxy server that listens for gNB connections
// and forwards messages to the AMF.
// It intercepts and decodes NGAP messages.
func startProxyServer(listenAddrStr, amfAddrStr string) error {
	// Resolve addresses
	listenAddr, err := sctp.ResolveSCTPAddr("sctp", listenAddrStr)
	if err != nil {
		return fmt.Errorf("failed to resolve listen address: %v", err)
	}
	amfAddr, err := sctp.ResolveSCTPAddr("sctp", amfAddrStr)
	if err != nil {
		return fmt.Errorf("failed to resolve AMF address: %v", err)
	}

	// Listen for incoming SCTP connections from gNB
	ln, err := sctp.ListenSCTP("sctp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", listenAddrStr, err)
	}
	fmt.Printf("Proxy listening for gNB on %s\n", listenAddrStr)

	go func() {
		for {
			gnbConn, err := ln.AcceptSCTP(0)
			if err != nil {
				log.Printf("Failed to accept gNB connection: %v", err)
				continue
			}
			fmt.Printf("Accepted gNB connection from %s\n", gnbConn.RemoteAddr())

			// Connect to AMF
			amfConn, err := sctp.DialSCTP("sctp", nil, amfAddr)
			if err != nil {
				log.Printf("Failed to connect to AMF: %v", err)
				gnbConn.Close()
				continue
			}
			fmt.Printf("Connected to AMF at %s\n", amfAddrStr)

			// Start bidirectional proxying
			go proxySCTP(gnbConn, amfConn, "gNB->AMF")
			go proxySCTP(amfConn, gnbConn, "AMF->gNB")
		}
	}()
	select {} // Prevent the main goroutine from exiting so proxySCTP is always reachable

}

// SCTP proxy function that reads from the source connection
// and writes to the destination connection.
// intercept NGAP.
// Note: NGAP messages type is needed to be decoded.
func proxySCTP(src, dst *sctp.SCTPConn, direction string) {
	defer src.Close()
	defer dst.Close()
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if err != nil {
			log.Printf("[%s] Read error: %v", direction, err)
			return
		}
		// Intercept and decode NGAP message if possible
		ngapMsg, err := ngap.Decoder(buf[:n])
		if err == nil {
			log.Printf("[%s] Intercepted NGAP message: %+v", direction, ngapMsg)
		} else {
			log.Printf("[%s] Forwarding non-NGAP or undecodable data (%d bytes)", direction, n)
		}
		_, err = dst.Write(buf[:n])
		if err != nil {
			log.Printf("[%s] Write error: %v", direction, err)
			return
		}
	}
}

// getKubeClient returns a Kubernetes clientset
func getKubeClient(ipAddress, port string) (*kubernetes.Clientset, error) {
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags(fmt.Sprintf("https://%s:%s", ipAddress, port), kubeconfig)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

// getAMFAddresses finds all AMF pod IPs in the cluster
func getAMFAddresses(clientset *kubernetes.Clientset) ([]string, error) {
	pods, err := clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=amf",
	})
	if err != nil {
		return nil, err
	}
	var addrs []string
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Running" && pod.Status.PodIP != "" {
			addrs = append(addrs, pod.Status.PodIP+":38412")
		}
	}
	return addrs, nil
}

// connectToAMFs connects to each AMF pod using SCTP and prints result
func connectToAMFs(amfAddrs []string) {
	for _, addr := range amfAddrs {
		amfAddr, err := sctp.ResolveSCTPAddr("sctp", addr)
		if err != nil {
			log.Printf("Failed to resolve AMF address %s: %v", addr, err)
			continue
		}
		conn, err := sctp.DialSCTP("sctp", nil, amfAddr)
		if err != nil {
			log.Printf("Failed to connect to AMF at %s: %v", addr, err)
			continue
		}
		fmt.Printf("Successfully connected to AMF at %s\n", addr)
		conn.Close()
	}
}

// main function moved to package main above
// func main() {
// 	// Test for local SCTP proxy server and local Free5GC
// 	// Listen address for gNB (proxy listens here)
// 	// listenAddrStr := "127.0.0.10:38412"
// 	// // AMF address and port
// 	// amfAddrStr := "127.0.0.18:38412"

// 	// // Start the proxy server in the background
// 	// if err := startProxyServer(listenAddrStr, amfAddrStr); err != nil {
// 	// 	log.Fatalf("Failed to start proxy server: %v", err)
// 	// }

// 	ipaddress := "192.168.49.2"
// 	port := "8443"
// 	fmt.Printf("Connecting to Kubernetes cluster at %s:%s\n", ipaddress, port)

// 	clientset, err := getKubeClient(ipaddress, port)
// 	if err != nil {
// 		log.Fatalf("Failed to create Kubernetes client: %v", err)
// 	}
// 	amfAddrs, err := getAMFAddresses(clientset)
// 	if err != nil {
// 		log.Fatalf("Failed to get AMF addresses: %v", err)
// 	}
// 	fmt.Println("AMF addresses found in Kubernetes:")
// 	for _, addr := range amfAddrs {
// 		fmt.Println(addr)
// 	}
// 	connectToAMFs(amfAddrs)

// 	/* Need to develop */
// 	// Simulate message sending, received messages, etc.
// 	// Choose the AMF address and port based on your Free5GC configuration.
// 	// Set up a connection between gNB and AMF.
// 	// Set up ending conditions for the proxy server.
// 	// fmt.Printf("Ending")
// }
