package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/ishidawataru/sctp"
)

type AMFEndpoint struct {
	Name string
	IP   string
	Port int
}

var amfs []AMFEndpoint
var amfIndex = 0
var mu sync.Mutex

// func getKubeClient() (*kubernetes.Clientset, error) {
// 	// Try in-cluster config
// 	config, err := rest.InClusterConfig()
// 	if err != nil {
// 		// Fallback to local kubeconfig
// 		kubeconfig := filepath.Join(homeDir(), ".kube", "config")
// 		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
// 		if err != nil {
// 			return nil, err
// 		}
// 	}
// 	return kubernetes.NewForConfig(config)
// }

// func homeDir() string {
// 	if h := os.Getenv("HOME"); h != "" {
// 		return h
// 	}
// 	return os.Getenv("USERPROFILE") // Windows
// }

// func listPodsAndServices(namespace string) error {
// 	clientset, err := getKubeClient()
// 	if err != nil {
// 		return err
// 	}

// 	ctx := context.Background()

// 	// List services
// 	services, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
// 	if err != nil {
// 		return err
// 	}
// 	fmt.Println("Services:")
// 	for _, svc := range services.Items {
// 		fmt.Printf("- Name: %s | ClusterIP: %s | Ports: %v\n", svc.Name, svc.Spec.ClusterIP, svc.Spec.Ports)
// 	}

// 	// List pods
// 	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
// 	if err != nil {
// 		return err
// 	}
// 	fmt.Println("\nPods:")
// 	for _, pod := range pods.Items {
// 		fmt.Printf("- Name: %s | IP: %s | Node: %s | Status: %s\n",
// 			pod.Name, pod.Status.PodIP, pod.Spec.NodeName, pod.Status.Phase)
// 	}
// 	return nil
// }

// func listAmfPodsAndConnection(namespace, nf string) error {
// 	clientset, err := getKubeClient()
// 	if err != nil {
// 		return err
// 	}

// 	ctx := context.Background()

// 	// List AMF pods
// 	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
// 		LabelSelector: fmt.Sprintf("nf=%s", nf),
// 	})
// 	if err != nil {
// 		return err
// 	}
// 	fmt.Println("AMF Pods:")
// 	for _, pod := range pods.Items {
// 		fmt.Printf("- Name: %s | IP: %s | NodePort: %s | Node: %s | Status: %s\n",
// 			pod.Name, pod.Status.PodIP, pod.Spec.Containers[0].Ports[0].ContainerPort, pod.Spec.NodeName, pod.Status.Phase)
// 		connectToAMF("192.168.49.2") //(pod.Status.PodIP)
// 	}

// 	return nil
// }

// func connectToAMF(amfIP string) {
// 	addr := &sctp.SCTPAddr{
// 		IPAddrs: []net.IPAddr{{IP: net.ParseIP(amfIP)}},
// 		Port:    32627, // NGAP port
// 	}

// 	conn, err := sctp.DialSCTP("sctp", nil, addr)
// 	if err != nil {
// 		fmt.Printf("[Err][404] Failed to connect to AMF at %s: %v\n", amfIP, err)
// 		return
// 	}
// 	defer conn.Close()
// 	fmt.Printf("[Info][200] Connected to AMF at %s\n", amfIP)

// 	// Next step: initiate NGAP handshake
// 	sendNGAP(conn)
// }

// func sendNGAP(conn *sctp.SCTPConn) {
// 	// Normally this would be ASN.1 encoded NGAP message.
// 	// For now, we send a dummy byte message.
// 	ngapMsg := []byte{0x00, 0x20, 0x00, 0x01} // Dummy placeholder

// 	n, err := conn.Write(ngapMsg)
// 	if err != nil {
// 		fmt.Printf("[Err][NGAP] Failed to send NGAP message: %v\n", err)
// 		return
// 	}

// 	fmt.Printf("[Info][NGAP] Sent %d bytes of NGAP message\n", n)

// 	// Read NGAP response
// 	buf := make([]byte, 1024)
// 	n, err = conn.Read(buf)
// 	if err != nil {
// 		fmt.Printf("[Err][NGAP] No NGAP response or error: %v\n", err)
// 		return
// 	}

// 	fmt.Printf("[Info][NGAP] Received %d bytes from AMF: %x\n", n, buf[:n])
// 	fmt.Println("[Info][NGAP] NGAP connection established — Message successful!")
// }

// func main() {
// 	namespace := "free5gc" // or "free5gc" or your namespace
// 	nf := "amf"            // or "amf" network function or your network function
// 	if err := listPodsAndServices(namespace); err != nil {
// 		fmt.Println("[Err][404]Error to connect to Kubernetes or get resources:", err)
// 	}

// 	if err := listAmfPodsAndConnection(namespace, nf); err != nil {
// 		fmt.Println("[Err][404]Error to connect to AMF Pods:", err)
// 	}
// }

func main() {
	namespace := "free5gc" // Adjust based on your cluster
	clientset, err := getKubeClient()
	if err != nil {
		log.Fatalf("Failed to get Kubernetes client: %v", err)
	}

	ctx := context.Background()
	svcs, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to list services: %v", err)
	}

	nodeIP, err := getNodeIP(clientset)
	if err != nil {
		log.Fatalf("Failed to get node IP: %v", err)
	}

	// Discover AMF NodePort services
	for _, svc := range svcs.Items {
		if strings.Contains(svc.Name, "amf") && svc.Spec.Type == corev1.ServiceTypeNodePort {
			for _, port := range svc.Spec.Ports {
				if port.Protocol == corev1.ProtocolSCTP && port.NodePort > 0 {
					amfs = append(amfs, AMFEndpoint{
						Name: svc.Name,
						IP:   nodeIP,
						Port: int(port.NodePort),
					})
					log.Printf("Discovered AMF: %s at %s:%d", svc.Name, nodeIP, port.NodePort)
				}
			}
		}
	}

	if len(amfs) == 0 {
		log.Fatalln("No AMF NodePort services with SCTP found.")
	}

	listenAddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("0.0.0.0")}},
		Port:    38412,
	}

	listener, err := sctp.ListenSCTP("sctp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to start SCTP listener: %v", err)
	}
	log.Printf("Listening on %s\n", listenAddr.String())

	for {
		conn, err := listener.AcceptSCTP()
		if err != nil {
			log.Printf("Accept failed: %v", err)
			continue
		}
		log.Println("Received connection from gNB")
		go handleGNBConnection(conn)
	}
}

func getKubeClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := filepath.Join(homeDir(), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	return kubernetes.NewForConfig(config)
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}

func getNodeIP(clientset *kubernetes.Clientset) (string, error) {
	nodes, err := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	for _, node := range nodes.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeExternalIP || addr.Type == corev1.NodeInternalIP {
				return addr.Address, nil
			}
		}
	}
	out, err := exec.Command("minikube", "ip").Output()
	if err != nil {
		return "", fmt.Errorf("no valid node IP found and minikube failed: %v", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func pickAMF() AMFEndpoint {
	mu.Lock()
	defer mu.Unlock()
	amf := amfs[amfIndex]
	amfIndex = (amfIndex + 1) % len(amfs)
	return amf
}

func handleGNBConnection(gnbConn *sctp.SCTPConn) {
	amf := pickAMF()
	amfAddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP(amf.IP)}},
		Port:    amf.Port,
	}

	amfConn, err := sctp.DialSCTP("sctp", nil, amfAddr)
	if err != nil {
		log.Printf("Failed to connect to AMF [%s]: %v", amf.Name, err)
		gnbConn.Close()
		return
	}

	log.Printf("🔄 Proxying gNB <-> AMF [%s]", amf.Name)
	go proxySCTP(gnbConn, amfConn)
	go proxySCTP(amfConn, gnbConn)
}

func proxySCTP(src, dst *sctp.SCTPConn) {
	buf := make([]byte, 2048)
	for {
		n, err := src.Read(buf)
		if err != nil {
			log.Printf("Read error: %v", err)
			src.Close()
			dst.Close()
			return
		}
		if n > 0 {
			_, err = dst.Write(buf[:n])
			if err != nil {
				log.Printf("Write error: %v", err)
				src.Close()
				dst.Close()
				return
			}
		}
	}
}
