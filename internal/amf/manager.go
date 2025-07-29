package amf

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	ngapconn "github.com/hasukiHT/5glos/internal/ngap"
	"github.com/hasukiHT/5glos/internal/utils"
	"github.com/ishidawataru/sctp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type AMFInfo struct {
	PodName      string
	NodeIP       string
	InternalPort int32
	NodePort     int32
}

type Manager struct {
	amfList     map[string]*Amf
	indexesById map[string]*Amf
	mutex       sync.Mutex
	clientset   *kubernetes.Clientset
	namespace   string
	minikubeIP  string
	done        chan struct{}
	wg          sync.WaitGroup
	gnbManager  GnbManagerInterface // Add GnB manager reference
	service     ServiceInterface    // Add service reference
}

func NewManager() *Manager {
	// Get minikube IP
	minikubeIP, err := utils.GetMinikubeIP()
	if err != nil {
		log.Printf("[ERROR] Failed to get minikube IP: %v", err)
		minikubeIP = "127.0.0.1" // fallback
	}

	// Initialize Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig
		kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Printf("[ERROR] Failed to get kubeconfig: %v", err)
		}
	}

	var clientset *kubernetes.Clientset
	if config != nil {
		clientset, err = kubernetes.NewForConfig(config)
		if err != nil {
			log.Printf("[ERROR] Failed to create k8s client: %v", err)
		}
	}

	return &Manager{
		amfList:     make(map[string]*Amf),
		indexesById: make(map[string]*Amf),
		clientset:   clientset,
		namespace:   "free5gc", // default namespace
		minikubeIP:  minikubeIP,
		done:        make(chan struct{}),
	}
}

// SetDependencies sets the GnB manager and service dependencies
func (m *Manager) SetGnbManager(gnbManager GnbManagerInterface) {
	m.gnbManager = gnbManager
}

func (m *Manager) SetService(service ServiceInterface) {
	m.service = service
}

// Monitor K8s events, connect to AMFs/delete AMFs
func (m *Manager) MonitorLoop(wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.refreshAMFConnections()
		}
	}
}

func (m *Manager) refreshAMFConnections() {
	if m.clientset == nil {
		return
	}

	amfs, err := m.getAMFPodsAndPorts()
	if err != nil {
		log.Printf("[ERROR] Failed to get AMF pods: %v", err)
		return
	}

	// Connect to new AMFs
	for _, amfInfo := range amfs {
		key := fmt.Sprintf("%s:%d", amfInfo.PodName, amfInfo.NodePort)
		if _, exists := m.amfList[key]; !exists {
			go m.connectAmf(amfInfo)
		}
	}
}

func (m *Manager) getAMFPodsAndPorts() ([]AMFInfo, error) {
	var amfs []AMFInfo

	// List pods with label "nf=amf"
	pods, err := m.clientset.CoreV1().Pods(m.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "nf=amf",
	})
	if err != nil {
		return nil, err
	}

	// List services with label "nf=amf"
	svcs, err := m.clientset.CoreV1().Services(m.namespace).List(context.TODO(), metav1.ListOptions{
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

	// Map services to AMF info
	for _, svc := range svcs.Items {
		for _, port := range svc.Spec.Ports {
			if port.Name == "sctp" || port.Protocol == v1.ProtocolSCTP {
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
	return amfs, nil
}

// Add an AMF
func (m *Manager) Add(amf *Amf) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	key := fmt.Sprintf("%s:%d", amf.podName, amf.nodePort)
	m.amfList[key] = amf
	m.indexesById[amf.id] = amf
	log.Printf("[INFO] Added AMF to manager: %s", amf.id)
}

func (m *Manager) Remove(id string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if amf, ok := m.indexesById[id]; ok {
		amf.Close()
		key := fmt.Sprintf("%s:%d", amf.podName, amf.nodePort)
		delete(m.amfList, key)
		delete(m.indexesById, id)
		log.Printf("[INFO] Removed AMF: %s", id)
	}
}

// Pick an AMF for load balancing
func (m *Manager) Pick() *Amf {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Find AMF with least load (minimum number of UE contexts)
	if len(m.amfList) == 0 {
		return nil
	}

	var selectedAmf *Amf
	minLoad := int64(-1)

	for _, amf := range m.amfList {
		amf.mutex.Lock()
		currentLoad := int64(len(amf.ueList))
		amf.mutex.Unlock()

		// Select AMF with minimum load
		if minLoad == -1 || currentLoad < minLoad {
			minLoad = currentLoad
			selectedAmf = amf
		}
	}

	return selectedAmf
}

// Connect to AMF, send NGSetupRequest then start goroutine to read data
func (m *Manager) connectAmf(amfInfo AMFInfo) error {
	// Check if already connecting/connected to this AMF
	key := fmt.Sprintf("%s:%d", amfInfo.PodName, amfInfo.NodePort)
	m.mutex.Lock()
	if _, exists := m.amfList[key]; exists {
		m.mutex.Unlock()
		return nil // Already connected
	}
	// Add placeholder to prevent duplicate connections
	m.amfList[key] = nil
	m.mutex.Unlock()

	addr := fmt.Sprintf("%s:%d", m.minikubeIP, amfInfo.NodePort)
	raddr, err := sctp.ResolveSCTPAddr("sctp", addr)
	if err != nil {
		// Remove placeholder on error
		m.mutex.Lock()
		delete(m.amfList, key)
		m.mutex.Unlock()
		return fmt.Errorf("ResolveSCTPAddr error for %s: %v", addr, err)
	}

	// Assign a unique local port for each AMF connection using a generated value
	basePort := 40000
	hash := 0
	for _, c := range amfInfo.PodName {
		hash += int(c)
	}
	laddr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("192.168.0.4")}}, // Use a fixed IP for local address
		// Use a base port and add a hash to ensure uniqueness
		Port: basePort + (hash % 1000), // Ensure port is unique per PodName
	}
	conn, err := sctp.DialSCTP("sctp", laddr, raddr)
	if err != nil {
		// Remove placeholder on error
		m.mutex.Lock()
		delete(m.amfList, key)
		m.mutex.Unlock()
		return fmt.Errorf("SCTP dial error for %s: %v", addr, err)
	}

	// Create AMF
	amf := New(amfInfo)

	// Set manager reference for the AMF
	amf.SetManager(m)

	// Inject dependencies into the AMF
	if m.gnbManager != nil {
		amf.SetGnbManager(m.gnbManager)
	}
	if m.service != nil {
		amf.SetService(m.service)
	}

	// Create SCTP connection wrapper
	ngapConn := ngapconn.NewNgapConn(conn, amf.Handle)
	amf.conn = ngapConn

	// Update manager with actual AMF instance
	m.mutex.Lock()
	m.amfList[key] = amf
	m.indexesById[amf.id] = amf
	m.mutex.Unlock()

	// Send setup request
	if err := amf.SendSetupRequest(); err != nil {
		// Cleanup on setup failure
		m.mutex.Lock()
		delete(m.amfList, key)
		delete(m.indexesById, amf.id)
		m.mutex.Unlock()
		conn.Close()
		return fmt.Errorf("send setup request failed: %v", err)
	}

	// Start read loop to handle incoming messages from AMF
	m.wg.Add(1)
	go ngapConn.ReadLoop(&m.wg)

	log.Printf("[INFO] Connected to AMF: %s, ID: %s", addr, amf.id)
	return nil
}

func (m *Manager) Close() {
	close(m.done)
	m.mutex.Lock()
	for _, amf := range m.amfList {
		if amf != nil {
			amf.Close()
		}
	}
	m.mutex.Unlock()

	// Wait for all AMF read loops to finish
	m.wg.Wait()
}
