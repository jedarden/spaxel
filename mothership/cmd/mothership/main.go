// Package main provides the mothership entry point
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
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
	_ "modernc.org/sqlite"
	"github.com/spaxel/mothership/internal/api"
	"github.com/spaxel/mothership/internal/apdetector"
	"github.com/spaxel/mothership/internal/auth"
	"github.com/spaxel/mothership/internal/ble"
	"github.com/spaxel/mothership/internal/dashboard"
	"github.com/spaxel/mothership/internal/diagnostics"
	"github.com/spaxel/mothership/internal/explainability"
	"github.com/spaxel/mothership/internal/fleet"
	"github.com/spaxel/mothership/internal/ingestion"
	"github.com/spaxel/mothership/internal/ota"
	"github.com/spaxel/mothership/internal/provisioning"
	"github.com/spaxel/mothership/internal/recorder"
	"github.com/spaxel/mothership/internal/replay"
	sigproc "github.com/spaxel/mothership/internal/signal"
)

// Phase 5: Configuration constants
const (
	baselineSaveInterval = 30 * time.Second
	healthComputeInterval = 5 * time.Second
	weatherRecordInterval = 60 * time.Second
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

	// Create auth handler for PIN-based authentication
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "/data"
	}
	var authHandler *auth.Handler
	// Open a SQLite connection for auth
	authDBPath := filepath.Join(dataDir, "spaxel.db")
	authDB, err := sql.Open("sqlite", authDBPath)
	if err != nil {
		log.Printf("[WARN] Failed to open auth database: %v", err)
	} else {
		authDB.SetMaxOpenConns(1) // SQLite is single-writer
		defer authDB.Close()

		// Initialize auth handler
		authHandler, err = auth.NewHandler(auth.Config{DB: authDB})
		if err != nil {
			log.Printf("[WARN] Failed to initialize auth handler: %v", err)
			authHandler = nil // Disable auth on error
		} else {
			defer authHandler.Close()
			// Register auth routes (public endpoints)
			authHandler.RegisterRoutes(r)
			log.Printf("[INFO] Authentication enabled")
		}
	}

	// Set up node token validator for ingestion server
	// Note: authHandler will be nil if auth is disabled, which is fine for development
	if authHandler != nil {
		ingestSrv.SetTokenValidator(authHandler.ValidateNodeToken)
		log.Printf("[INFO] Node token validation enabled")
	}

	// Helper function to wrap handlers with auth middleware
	requireAuth := func(next http.HandlerFunc) http.HandlerFunc {
		if authHandler == nil {
			return next // No auth if handler not initialized
		}
		return authHandler.RequireAuth(next)
	}

	requireAuthHandler := func(next http.Handler) http.Handler {
		if authHandler == nil {
			return next // No auth if handler not initialized
		}
		return authHandler.RequireAuthHandler(next)
	}

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","version":"%s"}`, version)
	})

	// Create ingestion server
	ingestSrv := ingestion.NewServer()
	r.HandleFunc("/ws/node", ingestSrv.HandleNodeWS)

	// Passive radar: AP detector for auto-detecting router as virtual TX node
	// Uses the main database for virtual node storage
	if authDB != nil {
		apDet := apdetector.NewDetector(authDB)
		ingestSrv.SetAPDetector(apDet)
		log.Printf("[INFO] AP detector enabled for passive radar auto-detection")
	}

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

	// Fleet node registry
	fleetReg, err := fleet.NewRegistry(filepath.Join(cfg.DataDir, "fleet.db"))
	if err != nil {
		log.Fatalf("[FATAL] Failed to open fleet registry: %v", err)
	}
	defer fleetReg.Close()
	log.Printf("[INFO] Fleet registry at %s", filepath.Join(cfg.DataDir, "fleet.db"))

	// Phase 5: Self-healing fleet manager with GDOP optimization
	fleetHealer := fleet.NewFleetHealer(fleetReg, fleet.FleetHealerConfig{
		HealInterval:   60 * time.Second,
		MinOnlineNodes: 2,
		MaxHistorySize: 100,
	})

	// Phase 5: Link weather diagnostics
	weatherDiagnostics := fleet.NewLinkWeatherDiagnostics()

	// Legacy fleet manager (kept for basic operations)
	fleetMgr := fleet.NewManager(fleetReg)

	// Phase 5: Multi-notifier broadcasts node events to both legacy manager and healer
	multiNotify := newMultiNotifier(fleetMgr, fleetHealer)
	ingestSrv.SetFleetNotifier(multiNotify)

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

	// Phase 5: Wire advanced fleet healer
	fleetHealer.SetNotifier(ingestSrv)
	fleetHealer.SetBroadcaster(dashboardHub)
	go fleetHealer.Run(ctx)

	// Phase 5: Wire weather diagnostics with node position accessor
	weatherDiagnostics.SetNodePositionAccessor(func(mac string) (x, z float64, ok bool) {
		node, err := fleetReg.GetNode(mac)
		if err != nil {
			return 0, 0, false
		}
		return node.PosX, node.PosZ, true
	})
	weatherDiagnostics.SetPositionSuggester(func() (x, z, improvement float64) {
		return fleetHealer.SuggestNodePosition()
	})

	// Phase 5: Advanced diagnostic engine with root-cause analysis
	diagnosticEngine := diagnostics.NewDiagnosticEngine(diagnostics.DiagnosticConfig{
		DiagnosticInterval: 15 * time.Minute,
		HistoryWindow:      1 * time.Hour,
		MinSamples:         10,
	})

	// Wire health history accessor for diagnostic engine
	diagnosticEngine.SetHealthHistoryAccessor(func(linkID string, window time.Duration) []diagnostics.LinkHealthSnapshot {
		// Get history from weather diagnostics
		snapshots := weatherDiagnostics.GetHistory(linkID, window)
		result := make([]diagnostics.LinkHealthSnapshot, len(snapshots))
		for i, s := range snapshots {
			result[i] = diagnostics.LinkHealthSnapshot{
				Timestamp:       s.Timestamp,
				SNR:             s.SNR,
				PhaseStability:  s.PhaseStability,
				PacketRate:      s.PacketRate,
				DriftRate:       s.DriftRate,
				CompositeScore:  s.CompositeScore,
				DeltaRMSVariance: s.DeltaRMSVariance,
				IsQuietPeriod:   s.IsQuietPeriod,
			}
		}
		return result
	})

	// Wire link ID accessor
	diagnosticEngine.SetAllLinkIDsAccessor(func() []string {
		return pm.GetAllLinkIDs()
	})

	// Wire node position accessor for diagnostics
	diagnosticEngine.SetNodePositionAccessor(func(mac string) (diagnostics.Vec3, bool) {
		node, err := fleetReg.GetNode(mac)
		if err != nil {
			return diagnostics.Vec3{}, false
		}
		return diagnostics.Vec3{X: node.PosX, Y: node.PosY, Z: node.PosZ}, true
	})

	// Wire GDOP improvement accessor
	diagnosticEngine.SetGDOPImprovementAccessor(func(nodeMAC string, targetPos diagnostics.Vec3) float64 {
		// Calculate current worst GDOP vs new worst GDOP with node at target position
		currentWorstX, currentWorstZ, currentWorstGDOP := fleetHealer.GetWorstCoverageZone()
		_ = currentWorstX
		_ = currentWorstZ
		// Estimate improvement - this is a simplified calculation
		return currentWorstGDOP * 0.2 // Assume 20% improvement as placeholder
	})

	// Wire repositioning computer for Rule 4
	diagnosticEngine.SetRepositioningComputer(func(linkID string, blockedZone diagnostics.Vec3) (diagnostics.Vec3, float64, error) {
		// Use fleet healer's position suggestion
		sugX, sugZ, improvement := fleetHealer.SuggestNodePosition()
		return diagnostics.Vec3{X: sugX, Z: sugZ}, improvement, nil
	})

	// Wire occupancy accessor for quiet period detection
	diagnosticEngine.SetOccupancyAccessor(func() int {
		return pm.GetStationaryPersonCount()
	})

	// Start diagnostic engine
	go diagnosticEngine.Run(ctx)
	log.Printf("[INFO] Phase 5 diagnostic engine started (interval: 15m)")

	// Phase 5: Baseline persistence store
	baselineStore, err := sigproc.NewBaselineStore(filepath.Join(cfg.DataDir, "baselines.db"))
	if err != nil {
		log.Printf("[WARN] Failed to open baseline store: %v (persistence disabled)", err)
	} else {
		defer baselineStore.Close()
		// Restore saved baselines
		if err := baselineStore.RestoreAll(pm, 64); err != nil {
			log.Printf("[WARN] Failed to restore baselines: %v", err)
		}
		// Start periodic saves
		baselineStore.StartPeriodicSave(ctx, pm, baselineSaveInterval)
		log.Printf("[INFO] Baseline persistence enabled (save interval: %v)", baselineSaveInterval)
	}

	// Phase 6: Health persistence store for diagnostics and weekly trends
	healthStore, err := sigproc.NewHealthStore(filepath.Join(cfg.DataDir, "health.db"))
	if err != nil {
		log.Printf("[WARN] Failed to open health store: %v (health persistence disabled)", err)
	} else {
		defer healthStore.Close()
		healthStore.StartPeriodicTasks(ctx)
		log.Printf("[INFO] Health persistence enabled at %s", filepath.Join(cfg.DataDir, "health.db"))

		// Wire feedback accessor for diagnostic engine Rule 4 (Fresnel blockage)
		diagnosticEngine.SetFeedbackAccessor(func(linkID string, window time.Duration) []diagnostics.FeedbackEvent {
			events, err := healthStore.GetFeedbackEvents(linkID, window)
			if err != nil {
				return nil
			}
			result := make([]diagnostics.FeedbackEvent, len(events))
			for i, e := range events {
				result[i] = diagnostics.FeedbackEvent{
					LinkID:    e.LinkID,
					EventType: e.EventType,
					Position:  diagnostics.Vec3{X: e.PosX, Y: e.PosY, Z: e.PosZ},
					Timestamp: e.Timestamp,
				}
			}
			return result
		})
	}

	// Phase 5: Periodic health computation for all links
	go func() {
		ticker := time.NewTicker(healthComputeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pm.ComputeAllHealth()
			}
		}
	}()

	// Phase 6: Diurnal patterns learned notification
	// Track which links have already broadcast their "patterns learned" notification
	diurnalNotified := make(map[string]bool)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				statuses := pm.GetDiurnalLearningStatus()
				for _, status := range statuses {
					// If link is ready and we haven't notified yet
					if status.IsReady && !diurnalNotified[status.LinkID] {
						diurnalNotified[status.LinkID] = true
						log.Printf("[INFO] Diurnal patterns learned for link %s after 7 days", status.LinkID)
						// Broadcast notification to dashboard
						msg := map[string]interface{}{
							"type":    "diurnal_ready",
							"link_id": status.LinkID,
							"message": "Your system has learned your daily patterns. Accuracy should improve this week.",
						}
						data, _ := json.Marshal(msg)
						dashboardHub.Broadcast(data)
					}
				}
			}
		}
	}()

	// Phase 5: Periodic weather snapshot recording
	go func() {
		ticker := time.NewTicker(weatherRecordInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Record health snapshots for all active links
				states := pm.GetAllMotionStates()
				var healthEntries []sigproc.HealthLogEntry
				for _, state := range states {
					processor := pm.GetProcessor(state.LinkID)
					if processor == nil {
						continue
					}
					health := processor.GetHealth()
					if health == nil {
						continue
					}
					snr, phaseStability, packetRate, driftRate, deltaRMSVar := health.GetHealthMetrics()
					isQuiet := !state.MotionDetected
					weatherDiagnostics.RecordSnapshot(state.LinkID, snr, phaseStability, packetRate, driftRate)

					// Also persist to HealthStore for long-term diagnostics
					if healthStore != nil {
						composite := 0.3*snr + 0.25*(1-phaseStability) + 0.25*math.Min(packetRate/20.0, 1.0) + 0.2*(1-driftRate)
						if composite < 0 {
							composite = 0
						}
						if composite > 1 {
							composite = 1
						}
						healthEntries = append(healthEntries, sigproc.HealthLogEntry{
							LinkID:           state.LinkID,
							Timestamp:        time.Now(),
							SNR:              snr,
							PhaseStability:   phaseStability,
							PacketRate:       packetRate,
							DriftRate:        driftRate,
							CompositeScore:   composite,
							DeltaRMSVariance: deltaRMSVar,
							IsQuietPeriod:    isQuiet,
						})
					}
				}
				// Batch persist to health store
				if healthStore != nil && len(healthEntries) > 0 {
					if err := healthStore.LogHealthBatch(healthEntries); err != nil {
						log.Printf("[WARN] Failed to persist health entries: %v", err)
					}
				}
			}
		}
	}()
	log.Printf("[INFO] Phase 5 health monitoring enabled (health: %v, weather: %v)", healthComputeInterval, weatherRecordInterval)

	// Fleet REST API
	fleetHandler := fleet.NewHandler(fleetMgr)
	if authHandler != nil {
		// Create an authenticated sub-router for fleet API
		fleetRouter := chi.NewRouter()
		fleetRouter.Use(func(next http.Handler) http.Handler {
			return authHandler.RequireAuthHandler(next)
		})
		fleetHandler.RegisterRoutes(fleetRouter)
		r.Mount("/", fleetRouter)
	} else {
		fleetHandler.RegisterRoutes(r)
	}

	// Settings API
	settingsHandler, err := api.NewSettingsHandler(filepath.Join(cfg.DataDir, "settings.db"))
	if err != nil {
		log.Printf("[WARN] Failed to create settings handler: %v (settings API disabled)", err)
	} else {
		defer settingsHandler.Close()
		if authHandler != nil {
			settingsRouter := chi.NewRouter()
			settingsRouter.Use(func(next http.Handler) http.Handler {
				return authHandler.RequireAuthHandler(next)
			})
			settingsHandler.RegisterRoutes(settingsRouter)
			r.Mount("/", settingsRouter)
		} else {
			settingsHandler.RegisterRoutes(r)
		}
		log.Printf("[INFO] Settings API enabled")
	}

	// Zones and Portals API
	zonesHandler, err := api.NewZonesHandler(filepath.Join(cfg.DataDir, "zones.db"))
	if err != nil {
		log.Printf("[WARN] Failed to create zones handler: %v (zones/portals API disabled)", err)
	} else {
		defer zonesHandler.Close()
		if authHandler != nil {
			zonesRouter := chi.NewRouter()
			zonesRouter.Use(func(next http.Handler) http.Handler {
				return authHandler.RequireAuthHandler(next)
			})
			zonesHandler.RegisterRoutes(zonesRouter)
			r.Mount("/", zonesRouter)
		} else {
			zonesHandler.RegisterRoutes(r)
		}
		log.Printf("[INFO] Zones/Portals API enabled")
	}

	// Triggers API
	triggersHandler, err := api.NewTriggersHandler(filepath.Join(cfg.DataDir, "triggers.db"))
	if err != nil {
		log.Printf("[WARN] Failed to create triggers handler: %v (triggers API disabled)", err)
	} else {
		defer triggersHandler.Close()
		if authHandler != nil {
			triggersRouter := chi.NewRouter()
			triggersRouter.Use(func(next http.Handler) http.Handler {
				return authHandler.RequireAuthHandler(next)
			})
			triggersHandler.RegisterRoutes(triggersRouter)
			r.Mount("/", triggersRouter)
		} else {
			triggersHandler.RegisterRoutes(r)
		}
		log.Printf("[INFO] Triggers API enabled")
	}

	// Notifications API
	notificationsHandler, err := api.NewNotificationsHandler(filepath.Join(cfg.DataDir, "notifications.db"))
	if err != nil {
		log.Printf("[WARN] Failed to create notifications handler: %v (notifications API disabled)", err)
	} else {
		defer notificationsHandler.Close()
		if authHandler != nil {
			notificationsRouter := chi.NewRouter()
			notificationsRouter.Use(func(next http.Handler) http.Handler {
				return authHandler.RequireAuthHandler(next)
			})
			notificationsHandler.RegisterRoutes(notificationsRouter)
			r.Mount("/", notificationsRouter)
		} else {
			notificationsHandler.RegisterRoutes(r)
		}
		log.Printf("[INFO] Notifications API enabled")
	}

	// Events API (timeline)
	eventsHandler, err := api.NewEventsHandler(filepath.Join(cfg.DataDir, "events.db"))
	if err != nil {
		log.Printf("[WARN] Failed to create events handler: %v (events API disabled)", err)
	} else {
		defer eventsHandler.Close()
		if authHandler != nil {
			eventsRouter := chi.NewRouter()
			eventsRouter.Use(func(next http.Handler) http.Handler {
				return authHandler.RequireAuthHandler(next)
			})
			eventsHandler.RegisterRoutes(eventsRouter)
			r.Mount("/", eventsRouter)
		} else {
			eventsHandler.RegisterRoutes(r)
		}
		// Wire events handler to dashboard hub for live event broadcasts
		eventsHandler.SetHub(dashboardHub)
		log.Printf("[INFO] Events API enabled")
	}

	// Replay API
	if replayStore != nil {
		replayHandler, err := api.NewReplayHandler(filepath.Join(cfg.DataDir, "csi_replay.bin"), replayStore)
		if err != nil {
			log.Printf("[WARN] Failed to create replay handler: %v (replay API disabled)", err)
		} else {
			defer replayHandler.Close()
			if authHandler != nil {
				replayRouter := chi.NewRouter()
				replayRouter.Use(func(next http.Handler) http.Handler {
					return authHandler.RequireAuthHandler(next)
				})
				replayHandler.RegisterRoutes(replayRouter)
				r.Mount("/", replayRouter)
			} else {
				replayHandler.RegisterRoutes(r)
			}
			log.Printf("[INFO] Replay API enabled")
		}
	}

	// BLE Devices API
	bleRegistry, err := ble.NewRegistry(filepath.Join(cfg.DataDir, "ble.db"))
	if err != nil {
		log.Printf("[WARN] Failed to create BLE registry: %v (BLE API disabled)", err)
	} else {
		defer bleRegistry.Close()
		bleHandler := ble.NewHandler(bleRegistry)
		if authHandler != nil {
			bleRouter := chi.NewRouter()
			bleRouter.Use(func(next http.Handler) http.Handler {
				return authHandler.RequireAuthHandler(next)
			})
			bleHandler.RegisterRoutes(bleRouter)
			r.Mount("/", bleRouter)
		} else {
			bleHandler.RegisterRoutes(r)
		}
		log.Printf("[INFO] BLE Devices API enabled")
	}

	// Detection explainability API
	explainabilityHandler := explainability.NewHandler()
	if authHandler != nil {
		explainabilityRouter := chi.NewRouter()
		explainabilityRouter.Use(func(next http.Handler) http.Handler {
			return authHandler.RequireAuthHandler(next)
		})
		explainabilityHandler.RegisterRoutes(explainabilityRouter)
		r.Mount("/", explainabilityRouter)
	} else {
		explainabilityHandler.RegisterRoutes(r)
	}
	log.Printf("[INFO] Detection explainability API enabled")

	// Phase 5: Weather diagnostics REST API
	r.Get("/api/weather", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		reports := weatherDiagnostics.GetAllLinkReports()
		writeJSON(w, reports)
	})
	r.Get("/api/weather/{linkID}", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		linkID := chi.URLParam(r, "linkID")
		report := weatherDiagnostics.GetReport(linkID)
		writeJSON(w, report)
	})
	r.Get("/api/weather/summary", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		condition, avgConfidence, issueCount := weatherDiagnostics.GetSystemWeatherSummary()
		writeJSON(w, map[string]interface{}{
			"condition":     condition,
			"avg_confidence": avgConfidence,
			"issue_count":   issueCount,
		})
	})
	r.Get("/api/weather/{linkID}/weekly", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		linkID := chi.URLParam(r, "linkID")
		trend := weatherDiagnostics.GetWeeklyTrend(linkID)
		writeJSON(w, trend)
	})

	// Phase 5: Coverage and healing status API
	r.Get("/api/coverage", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		coverage := fleetHealer.GetCoverage()
		writeJSON(w, coverage)
	})
	r.Get("/api/coverage/history", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		limitStr := r.URL.Query().Get("limit")
		limit := 10
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				limit = n
			}
		}
		history := fleetHealer.GetCoverageHistory(limit)
		writeJSON(w, history)
	})
	r.Get("/api/healing/status", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, map[string]interface{}{
			"degraded":      fleetHealer.IsDegraded(),
			"online_nodes":  fleetHealer.GetOnlineNodes(),
			"optimal_roles": fleetHealer.GetOptimalRoles(),
		})
	})
	r.Get("/api/healing/suggest", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		x, z, improvement := fleetHealer.SuggestNodePosition()
		worstX, worstZ, worstGDOP := fleetHealer.GetWorstCoverageZone()
		writeJSON(w, map[string]interface{}{
			"suggested_position":     map[string]float64{"x": x, "z": z},
			"expected_improvement":   improvement,
			"worst_coverage_zone":    map[string]float64{"x": worstX, "z": worstZ, "gdop": worstGDOP},
		})
	})

	// Phase 5: System health API
	r.Get("/api/health/system", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, map[string]interface{}{
			"system_health":     pm.GetSystemHealth(),
			"link_count":        pm.LinkCount(),
			"active_links":      pm.ActiveLinks(),
			"stationary_count":  pm.GetStationaryPersonCount(),
			"worst_link":        func() string { id, _ := pm.GetWorstLink(); return id }(),
		})
	})

	// Phase 6: Diurnal learning status API
	r.Get("/api/diurnal/status", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		statuses := pm.GetDiurnalLearningStatus()
		writeJSON(w, statuses)
	})
	r.Get("/api/diurnal/status/{linkID}", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		linkID := chi.URLParam(r, "linkID")
		allStatuses := pm.GetDiurnalLearningStatus()
		for _, status := range allStatuses {
			if status.LinkID == linkID {
				writeJSON(w, status)
				return
			}
		}
		http.Error(w, "link not found", http.StatusNotFound)
	})

	// Link health API - returns all links with health scores and details
	r.Get("/api/links", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		links := ingestSrv.GetAllLinksWithHealth()
		writeJSON(w, links)
	})

	// Phase 6: Link diagnostics API
	r.Get("/api/links/{linkID}/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		linkID := chi.URLParam(r, "linkID")
		diagnoses := diagnosticEngine.GetDiagnoses(linkID)
		writeJSON(w, diagnoses)
	})

	r.Get("/api/links/{linkID}/health-history", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		linkID := chi.URLParam(r, "linkID")
		windowStr := r.URL.Query().Get("window")
		window := 24 * time.Hour // default 24h
		if windowStr != "" {
			if hours, err := strconv.Atoi(windowStr); err == nil && hours > 0 {
				window = time.Duration(hours) * time.Hour
			}
		}
		if healthStore == nil {
			http.Error(w, "health store not available", http.StatusServiceUnavailable)
			return
		}
		history, err := healthStore.GetHealthHistory(linkID, window)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, history)
	})

	r.Get("/api/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		allDiagnoses := diagnosticEngine.GetAllDiagnoses()
		writeJSON(w, allDiagnoses)
	})

	// OTA firmware server and manager
	firmwareDir := filepath.Join(cfg.DataDir, "firmware")
	otaSrv := ota.NewServer(firmwareDir)
	otaMgr := ota.NewManager(otaSrv, "http://"+cfg.BindAddr)
	otaMgr.SetSender(ingestSrv)
	ingestSrv.SetOTAManager(otaMgr)
	log.Printf("[INFO] OTA firmware server at %s", firmwareDir)

	// OTA REST API
	r.Get("/api/firmware", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		otaSrv.HandleList(w, r)
	})
	r.Post("/api/firmware/upload", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		otaSrv.HandleUpload(w, r)
	})
	r.Get("/firmware/{filename}", otaSrv.HandleServe) // Public - URL contains SHA256
	r.Get("/api/firmware/progress", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(otaMgr.GetProgress())
	})
	r.Post("/api/firmware/ota-all", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		// Rolling update of all connected nodes
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := otaMgr.SendOTAAll(ctx, 60*time.Second); err != nil {
				log.Printf("[ERROR] Rolling OTA failed: %v", err)
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "started"})
	})

	// Provisioning API (used by onboarding wizard)
	_, msPortStr, _ := net.SplitHostPort(cfg.BindAddr)
	msPort, _ := strconv.Atoi(msPortStr)
	if msPort == 0 {
		msPort = 8080
	}
	provSrv := provisioning.NewServer(cfg.DataDir, cfg.MDNSName, msPort)
	r.Post("/api/provision", provSrv.HandleProvision)

	// Firmware manifest for esp-web-tools (onboarding wizard flashing) - public
	r.Get("/api/firmware/manifest", func(w http.ResponseWriter, r *http.Request) {
		latest := otaSrv.GetLatest()
		manifest := map[string]interface{}{
			"name":                    "Spaxel Node",
			"version":                 version,
			"new_install_prompt_erase": true,
			"builds": []map[string]interface{}{},
		}

		if latest != nil {
			manifest["builds"] = []map[string]interface{}{
				{
					"chipFamily": "ESP32-S3",
					"parts": []map[string]interface{}{
						{
							"path":    "/firmware/" + latest.Filename,
							"address": 0x0,
						},
					},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest) //nolint:errcheck
	})

	go dashboardHub.Run()

	// Protect dashboard WebSocket with auth
	r.HandleFunc("/ws/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if authHandler != nil && !authHandler.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		dashboardSrv.HandleDashboardWS(w, r)
	})

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

// writeJSON is a helper for JSON responses
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// multiNotifier broadcasts node events to multiple FleetNotifier targets
type multiNotifier struct {
	notifiers []ingestion.FleetNotifier
}

func newMultiNotifier(notifiers ...ingestion.FleetNotifier) *multiNotifier {
	return &multiNotifier{notifiers: notifiers}
}

func (m *multiNotifier) OnNodeConnected(mac, firmware, chip string) {
	for _, n := range m.notifiers {
		n.OnNodeConnected(mac, firmware, chip)
	}
}

func (m *multiNotifier) OnNodeDisconnected(mac string) {
	for _, n := range m.notifiers {
		n.OnNodeDisconnected(mac)
	}
}
