package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/free5gc/ngap/logger"
	"github.com/hasukiHT/5glos/internal/service"
	"github.com/sirupsen/logrus"
)

func main() {
	// Initialize logger
	logger.NgapLog.Logger.SetLevel(logrus.InfoLevel)

	// Initialize service
	svc := service.New()

	// Start service
	if err := svc.Start(); err != nil {
		log.Fatalf("[ERROR] Failed to start service: %v", err)
	}

	// Listen to keyboard interruption
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("[INFO] Open5GLoS proxy server started")
	log.Println("[INFO] Press Ctrl+C to stop...")

	// Wait for signal
	<-sigChan
	log.Println("[INFO] Shutting down...")

	// Graceful shutdown
	svc.Stop()
	log.Println("[INFO] Server stopped")
}
