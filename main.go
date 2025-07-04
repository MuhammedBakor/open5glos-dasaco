package main

/*
	This go main.app to run all the components of the system.
	It is responsible for initializing the configuration, starting the AMF, GNB-UE, SCTP proxy, and Kubernetes client.

*/

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hasukiHT/5glos/balancer"
	"github.com/hasukiHT/5glos/config"
	"github.com/hasukiHT/5glos/gnbue"
	"github.com/hasukiHT/5glos/kube"
	"github.com/hasukiHT/5glos/sctp"
	"github.com/hasukiHT/5glos/utils"
)

func main() {
	// Load configuration from default config.yaml
	cfg, err := config.LoadConfigFromDefault()
	if err != nil {
		utils.LogFatal("Failed to load configuration", err)
	}

	// Initialize logger with configuration
	utils.InitLogger(cfg.Logging)
	utils.LogInfo("Starting 5GLOS", map[string]interface{}{
		"version":     "1.0.0",
		"listen_port": cfg.ListenPort,
		"namespace":   cfg.Namespace,
		"amf_label":   cfg.AMFLabel,
		"strategy":    cfg.LoadBalancerStrategy,
	})

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Kubernetes client
	kubeClient, err := kube.NewClient()
	if err != nil {
		utils.LogFatal("Failed to create Kubernetes client", err)
	}
	utils.LogInfo("Kubernetes client initialized successfully")

	// Initialize balancer
	var bal balancer.Balancer
	switch cfg.LoadBalancerStrategy {
	case "round-robin":
		bal = balancer.NewRoundRobinBalancer()
	default:
		utils.LogWarn("Unknown load balancer strategy, using round-robin", map[string]interface{}{
			"strategy": cfg.LoadBalancerStrategy,
		})
		bal = balancer.NewRoundRobinBalancer()
	}

	// Initialize session manager
	sessionManager := gnbue.NewSessionManager()

	// Start AMF discovery and watcher
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		startAMFWatcher(ctx, kubeClient, bal, cfg)
	}()

	// Start SCTP proxy server
	wg.Add(1)
	go func() {
		defer wg.Done()
		startSCTPProxy(ctx, bal, sessionManager, cfg)
	}()

	// Setup graceful shutdown
	setupGracefulShutdown(cancel, sessionManager)

	utils.LogInfo("5GLOS started successfully, waiting for connections...")

	// Wait for all goroutines to finish
	wg.Wait()
	utils.LogInfo("5GLOS shutdown complete")
}

func startAMFWatcher(ctx context.Context, kubeClient *kube.Client, bal balancer.Balancer, cfg *config.Config) {
	utils.LogInfo("Starting AMF discovery and watcher", map[string]interface{}{
		"namespace": cfg.Namespace,
		"selector":  cfg.AMFLabel,
	})

	// Initial discovery
	endpoints, err := kubeClient.ListAMFPods(cfg.Namespace, cfg.AMFLabel)
	if err != nil {
		utils.LogError("Failed to discover AMF pods", err)
		return
	}

	for _, endpoint := range endpoints {
		bal.AddEndpoint(endpoint)
		utils.LogInfo("Added AMF endpoint", map[string]interface{}{
			"id":   endpoint.ID,
			"ip":   endpoint.IP,
			"port": endpoint.Port,
		})
	}

	// Start watcher
	watcher, err := kubeClient.WatchAMFPods(ctx, cfg.Namespace, cfg.AMFLabel)
	if err != nil {
		utils.LogError("Failed to start AMF pod watcher", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			utils.LogInfo("Stopping AMF watcher")
			return
		case event := <-watcher:
			handleAMFEvent(event, bal)
		}
	}
}

func handleAMFEvent(event kube.PodEvent, bal balancer.Balancer) {
	switch event.Type {
	case kube.PodEventAdded:
		bal.AddEndpoint(event.Endpoint)
		utils.LogInfo("AMF endpoint added", map[string]interface{}{
			"id":   event.Endpoint.ID,
			"ip":   event.Endpoint.IP,
			"port": event.Endpoint.Port,
		})
	case kube.PodEventModified:
		bal.UpdateEndpoint(event.Endpoint)
		utils.LogInfo("AMF endpoint updated", map[string]interface{}{
			"id":     event.Endpoint.ID,
			"status": event.Endpoint.Status,
		})
	case kube.PodEventDeleted:
		bal.RemoveEndpoint(event.Endpoint.ID)
		utils.LogInfo("AMF endpoint removed", map[string]interface{}{
			"id": event.Endpoint.ID,
		})
	}
}

func startSCTPProxy(ctx context.Context, bal balancer.Balancer, sessionManager *gnbue.SessionManager, cfg *config.Config) {
	utils.LogInfo("Starting SCTP proxy server", map[string]interface{}{
		"port": cfg.ListenPort,
	})

	proxy, err := sctp.NewProxy(cfg.ListenPort, bal, sessionManager)
	if err != nil {
		utils.LogFatal("Failed to create SCTP proxy", err)
	}

	if err := proxy.Start(ctx); err != nil {
		utils.LogError("SCTP proxy error", err)
	}
}

func setupGracefulShutdown(cancel context.CancelFunc, sessionManager *gnbue.SessionManager) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		utils.LogInfo("Received shutdown signal", map[string]interface{}{
			"signal": sig.String(),
		})

		// Cleanup sessions
		sessionManager.CleanupAll()

		// Cancel context to stop all goroutines
		cancel()

		// Give some time for graceful shutdown
		time.Sleep(2 * time.Second)
	}()
}
