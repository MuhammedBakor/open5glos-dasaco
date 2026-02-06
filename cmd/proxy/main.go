package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/free5gc/ngap/logger"
	"github.com/hasukiHT/5glos/internal/config"
	"github.com/hasukiHT/5glos/internal/service"
	"github.com/sirupsen/logrus"
)

func main() {
	// Initialize logger
	logger.NgapLog.Logger.SetLevel(logrus.InfoLevel)

	// Load configuration
	configPath := "config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("[ERROR] Failed to load configuration from %s: %v", configPath, err)
	}

	// Initialize service with configuration
	svc := service.New(cfg)

	// Start service
	if err := svc.Start(); err != nil {
		log.Fatalf("[ERROR] Failed to start service: %v", err)
	}

	// Listen to keyboard interruption
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("[INFO] Open5GLoS proxy server started on %s:%d", cfg.Proxy.ListenAddr, cfg.Proxy.ListenPort)
	log.Println("[INFO] Press Ctrl+C to stop...")

	// Wait for signal
	<-sigChan
	log.Println("[INFO] Shutting down...")

	// Graceful shutdown
	svc.Stop()
	log.Println("[INFO] Server stopped")
}
