package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/hasukiHT/5glos/internal/config"
	"github.com/hasukiHT/5glos/internal/logger"
	"github.com/hasukiHT/5glos/internal/metrics"
	"github.com/hasukiHT/5glos/internal/sctp"
	"github.com/hasukiHT/5glos/internal/watcher"
)

const (
	Version = "v1.0.0"
)

func main() {
	var configPath = flag.String("config", "config.yaml", "Path to configuration file")
	var version = flag.Bool("version", false, "Show version")
	flag.Parse()

	if *version {
		fmt.Printf("5Glos Gateway %s\n", Version)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.New(cfg.Logging)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("Starting 5Glos Gateway",
		zap.String("version", Version),
		zap.String("config", *configPath),
	)

	// Initialize metrics
	metricsRegistry := metrics.NewRegistry()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Kubernetes watcher
	k8sWatcher, err := watcher.New(cfg.Kubernetes, log.Named("watcher"))
	if err != nil {
		log.Fatal("Failed to create Kubernetes watcher", zap.Error(err))
	}

	// Start AMF discovery
	amfPool, err := k8sWatcher.StartAMFWatcher(ctx)
	if err != nil {
		log.Fatal("Failed to start AMF watcher", zap.Error(err))
	}

	// Initialize SCTP proxy
	proxy, err := sctp.NewProxy(cfg.Proxy, amfPool, metricsRegistry, log.Named("sctp"))
	if err != nil {
		log.Fatal("Failed to create SCTP proxy", zap.Error(err))
	}

	// Start metrics and health server
	go startMetricsServer(cfg.Metrics.Port, log.Named("metrics"))

	// Start SCTP proxy
	go func() {
		if err := proxy.Start(ctx); err != nil {
			log.Error("SCTP proxy error", zap.Error(err))
			cancel()
		}
	}()

	// Wait for termination signal
	setupGracefulShutdown(cancel, log, proxy)

	log.Info("5Glos Gateway started successfully")
	<-ctx.Done()
	log.Info("5Glos Gateway shutdown complete")
}

func startMetricsServer(port int, log *zap.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	log.Info("Starting metrics and health server", zap.Int("port", port))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Metrics and health server error", zap.Error(err))
	}
}

func setupGracefulShutdown(cancel context.CancelFunc, log *zap.Logger, proxy *sctp.Proxy) {
	// Channel to listen for OS signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		log.Info("Received termination signal, shutting down gracefully...")
		cancel()

		// Give proxy and other components time to clean up
		time.Sleep(2 * time.Second)

		log.Info("Shutdown complete")
		os.Exit(0)
	}()
}
