// Package main provides the mothership entry point
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/hashicorp/mdns"
	"github.com/spaxel/mothership/internal/dashboard"
	"github.com/spaxel/mothership/internal/fleet"
	"github.com/spaxel/mothership/internal/ingestion"
	"github.com/spaxel/mothership/internal/provisioning"
	"github.com/spaxel/mothership/internal/recorder"
	"github.com/spaxel/mothership/internal/replay"
	sigproc "github.com/spaxel/mothership/internal/signal"
)

// Build-time version injection
var version = "dev"

// Config holds application configuration
type Config struct {
	BindAddr    string
	DataDir     string
	StaticDir   string
	MDNSName    string
	MDNSEnabled bool
	LogLevel    string
	ReplayMaxMB int
}

func main() {
	cfg := parseConfig()
	log.Printf("[INFO] Spaxel mothership v%s starting", version)
	log.Printf("[DEBUG] Config: bind=%s data=%s static=%s mdns=%s", cfg.BindAddr, cfg.DataDir, cfg.StaticDir, cfg.MDNSName)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","version":"%s"}`, version)
	})

	// Create ingestion server
	ingestSrv := ingestion.NewServer()
	r.HandleFunc("/ws/node", ingestSrv.HandleNodeWS)

	// Signal processing pipeline
	pm := sigproc.NewProcessorManager(sigproc.ProcessorManagerConfig{
		NSub:       64,
		FusionRate: 10.0,
		Tau:        30.0,
	})
	ingestSrv.SetProcessorManager(pm)

	// Replay recording store
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Printf("[WARN] Failed to create data dir %s: %v", cfg.DataDir, err)
	} else {
		store, err := replay.NewRecordingStore(filepath.Join(cfg.DataDir, "csi_replay.bin"), cfg.ReplayMaxMB)
		if err != nil {
			log.Printf("[WARN] Failed to open replay store: %v (CSI recording disabled)", err)
		} else {
			ingestSrv.SetReplayStore(store)
			defer store.Close()
			log.Printf("[INFO] CSI replay store at %s (%d MB max)", filepath.Join(cfg.DataDir, "csi_replay.bin"), cfg.ReplayMaxMB)
		}
	}

	// Per-link CSI recorder
	recorderDir := filepath.Join(cfg.DataDir, "csi")
	recMgr, err := recorder.NewManager(recorder.DefaultConfig(recorderDir))
	if err != nil {
		log.Printf("[WARN] Failed to create recorder: %v (per-link recording disabled)", err)
	} else {
		ingestSrv.SetRecorder(recMgr)
		defer recMgr.Close()
		log.Printf("[INFO] Per-link CSI recorder at %s (retention=%dh, max=%dMB/link)",
			recorderDir, recorder.DefaultConfig(recorderDir).RetentionHours,
			recorder.DefaultConfig(recorderDir).MaxBytesPerLink/1<<20)
	}

	// Fleet node registry and manager
	fleetReg, err := fleet.NewRegistry(filepath.Join(cfg.DataDir, "fleet.db"))
	if err != nil {
		log.Fatalf("[FATAL] Failed to open fleet registry: %v", err)
	}
	defer fleetReg.Close()
	fleetMgr := fleet.NewManager(fleetReg)
	ingestSrv.SetFleetNotifier(fleetMgr)
	log.Printf("[INFO] Fleet registry at %s", filepath.Join(cfg.DataDir, "fleet.db"))

	// Adaptive rate controller
	rateCtrl := ingestion.NewRateController(func(mac string, rateHz int, varianceThreshold float64) {
		ingestSrv.SendConfigToMAC(mac, rateHz, varianceThreshold)
	})
	ingestSrv.SetRateController(rateCtrl)
	go rateCtrl.Run(ctx)

	// Dashboard hub and server
	dashboardHub := dashboard.NewHub()
	dashboardSrv := dashboard.NewServer(dashboardHub)

	dashboardHub.SetIngestionState(ingestSrv)

	// Wire ingestion → dashboard for CSI and motion broadcasts
	ingestSrv.SetDashboardBroadcaster(dashboardHub)
	ingestSrv.SetMotionBroadcaster(dashboardHub)

	// Wire fleet notifier/broadcaster and start self-healing loop
	fleetMgr.SetNotifier(ingestSrv)
	fleetMgr.SetBroadcaster(dashboardHub)
	go fleetMgr.Run(ctx)

	// Fleet REST API
	fleetHandler := fleet.NewHandler(fleetMgr)
	fleetHandler.RegisterRoutes(r)

	// Provisioning API (used by onboarding wizard)
	_, msPortStr, _ := net.SplitHostPort(cfg.BindAddr)
	msPort, _ := strconv.Atoi(msPortStr)
	if msPort == 0 {
		msPort = 8080
	}
	provSrv := provisioning.NewServer(cfg.DataDir, cfg.MDNSName, msPort)
	r.Post("/api/provision", provSrv.HandleProvision)

	// Firmware manifest for esp-web-tools (onboarding wizard flashing)
	r.Get("/api/firmware/manifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":                   "Spaxel Node",
			"version":                version,
			"new_install_prompt_erase": true,
			"builds": []map[string]interface{}{
				{
					"chipFamily": "ESP32-S3",
					"parts": []map[string]interface{}{
						{
							"path":    "/firmware/latest",
							"address": 0x0,
						},
					},
				},
			},
		}) //nolint:errcheck
	})

	go dashboardHub.Run()

	r.HandleFunc("/ws/dashboard", dashboardSrv.HandleDashboardWS)

	// Serve dashboard static files
	staticDir := cfg.StaticDir
	if staticDir == "" {
		staticDir = findDashboardDir()
	}

	if staticDir != "" {
		if _, err := os.Stat(staticDir); err == nil {
			log.Printf("[INFO] Serving dashboard from %s", staticDir)
			r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
				path := filepath.Join(staticDir, r.URL.Path)

				if info, err := os.Stat(path); err == nil && info.IsDir() {
					path = filepath.Join(path, "index.html")
				}

				if _, err := os.Stat(path); err == nil {
					http.ServeFile(w, r, path)
					return
				}

				if filepath.Ext(r.URL.Path) == "" {
					http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
					return
				}

				http.NotFound(w, r)
			})
		} else {
			log.Printf("[WARN] Dashboard directory not found: %s", staticDir)
		}
	} else {
		log.Printf("[WARN] No dashboard directory found, static files not served")
	}

	// mDNS advertisement
	var mdnsServer *mdns.Server
	if cfg.MDNSEnabled {
		service, err := mdns.NewMDNSService(
			cfg.MDNSName,
			"_spaxel._tcp",
			"local.",
			"",
			8080,
			nil,
			[]string{"version=1", "ws=/ws/node", "dashboard=/ws/dashboard"},
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

	srv := &http.Server{
		Addr:         cfg.BindAddr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("[INFO] HTTP server listening on %s", cfg.BindAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] HTTP server error: %v", err)
		}
	}()

	sig := <-sigChan
	log.Printf("[INFO] Received signal %v, initiating graceful shutdown", sig)

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] HTTP server shutdown error: %v", err)
	}

	ingestSrv.Shutdown(shutdownCtx)

	if mdnsServer != nil {
		mdnsServer.Shutdown()
	}

	log.Printf("[INFO] Shutdown complete")
}

func parseConfig() Config {
	bindAddr := getEnv("SPAXEL_BIND_ADDR", "0.0.0.0:8080")
	dataDir := getEnv("SPAXEL_DATA_DIR", "/data")
	staticDir := getEnv("SPAXEL_STATIC_DIR", "")
	mdnsName := getEnv("SPAXEL_MDNS_NAME", "spaxel")
	mdnsEnabled := getEnvBool("SPAXEL_MDNS_ENABLED", true)
	logLevel := getEnv("SPAXEL_LOG_LEVEL", "info")
	replayMaxMB := getEnvInt("SPAXEL_REPLAY_MAX_MB", replay.DefaultMaxMB)

	flag.StringVar(&bindAddr, "bind", bindAddr, "Listen address")
	flag.StringVar(&dataDir, "data", dataDir, "Data directory")
	flag.StringVar(&staticDir, "static", staticDir, "Static files directory (dashboard)")
	flag.StringVar(&mdnsName, "mdns-name", mdnsName, "mDNS service name")
	flag.BoolVar(&mdnsEnabled, "mdns", mdnsEnabled, "Enable mDNS advertisement")
	flag.StringVar(&logLevel, "log-level", logLevel, "Log level (debug, info, warn, error)")
	flag.IntVar(&replayMaxMB, "replay-max-mb", replayMaxMB, "CSI replay buffer size in MB")
	flag.Parse()

	return Config{
		BindAddr:    bindAddr,
		DataDir:     dataDir,
		StaticDir:   staticDir,
		MDNSName:    mdnsName,
		MDNSEnabled: mdnsEnabled,
		LogLevel:    logLevel,
		ReplayMaxMB: replayMaxMB,
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

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}

func findDashboardDir() string {
	candidates := []string{
		"dashboard",
		"../dashboard",
		"../../dashboard",
		"/app/dashboard",
	}

	for _, dir := range candidates {
		absPath, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(absPath, "index.html")); err == nil {
			return absPath
		}
	}

	return ""
}
