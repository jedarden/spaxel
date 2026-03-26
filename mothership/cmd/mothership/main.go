// Package main provides the mothership entry point
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/hashicorp/mdns"
	"github.com/spaxel/mothership/internal/ingestion"
)

// Build-time version injection
var version = "dev"

// Config holds application configuration
type Config struct {
	BindAddr    string
	DataDir     string
	MDNSName    string
	MDNSEnabled bool
	LogLevel    string
}

func main() {
	cfg := parseConfig()
	log.Printf("[INFO] Spaxel mothership v%s starting", version)
	log.Printf("[DEBUG] Config: bind=%s data=%s mdns=%s", cfg.BindAddr, cfg.DataDir, cfg.MDNSName)

	// Create context with cancellation for graceful shutdown
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Create router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health check endpoint
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","version":"%s"}`, version)
	})

	// Create ingestion server
	ingestSrv := ingestion.NewServer()
	r.HandleFunc("/ws/node", ingestSrv.HandleNodeWS)

	// Start mDNS advertisement
	var mdnsServer *mdns.Server
	if cfg.MDNSEnabled {
		service, err := mdns.NewMDNSService(
			cfg.MDNSName,
			"_spaxel._tcp",
			"local.",
			"",
			8080,
			nil,
			[]string{"version=1", "ws=/ws/node", "api=/api"},
		)
		if err != nil {
			log.Printf("[ERROR] Failed to create mDNS service: %v", err)
		} else {
			mdnsServer, err = mdns.NewServer(&mdns.Config{Zone: service})
			if err != nil {
				log.Printf("[ERROR] Failed to start mDNS server: %v", err)
			} else {
				log.Printf("[INFO] mDNS advertising %s._spaxel._tcp.local:8080", cfg.MDNSName)
			}
		}
	}

	// Start HTTP server
	srv := &http.Server{
		Addr:         cfg.BindAddr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Run server in goroutine
	go func() {
		log.Printf("[INFO] HTTP server listening on %s", cfg.BindAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	log.Printf("[INFO] Received signal %v, initiating graceful shutdown", sig)

	// Shutdown sequence
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop accepting new connections
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] HTTP server shutdown error: %v", err)
	}

	// Close ingestion server (drains connections)
	ingestSrv.Shutdown(shutdownCtx)

	// Stop mDNS
	if mdnsServer != nil {
		mdnsServer.Shutdown()
	}

	cancel()
	log.Printf("[INFO] Shutdown complete")
}

func parseConfig() Config {
	bindAddr := getEnv("SPAXEL_BIND_ADDR", "0.0.0.0:8080")
	dataDir := getEnv("SPAXEL_DATA_DIR", "/data")
	mdnsName := getEnv("SPAXEL_MDNS_NAME", "spaxel")
	mdnsEnabled := getEnvBool("SPAXEL_MDNS_ENABLED", true)
	logLevel := getEnv("SPAXEL_LOG_LEVEL", "info")

	flag.StringVar(&bindAddr, "bind", bindAddr, "Listen address")
	flag.StringVar(&dataDir, "data", dataDir, "Data directory")
	flag.StringVar(&mdnsName, "mdns-name", mdnsName, "mDNS service name")
	flag.BoolVar(&mdnsEnabled, "mdns", mdnsEnabled, "Enable mDNS advertisement")
	flag.StringVar(&logLevel, "log-level", logLevel, "Log level (debug, info, warn, error)")
	flag.Parse()

	return Config{
		BindAddr:    bindAddr,
		DataDir:     dataDir,
		MDNSName:    mdnsName,
		MDNSEnabled: mdnsEnabled,
		LogLevel:    logLevel,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1"
	}
	return defaultVal
}
