package main

import (
	"log"

	"github.com/yourusername/ngap-proxy/internal/proxy"
)

func main() {
	namespace := "free5gc" // Change to your namespace

	proxyServer, err := proxy.NewProxyServer(namespace)
	if err != nil {
		log.Fatalf("[ERROR] Failed to create proxy server: %v", err)
	}
	defer proxyServer.Close()

	// Start the proxy server (this will block)
	if err := proxyServer.Start(); err != nil {
		log.Fatalf("[ERROR] Proxy server failed: %v", err)
	}
}