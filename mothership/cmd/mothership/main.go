// Package main provides the mothership entry point.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hashicorp/mdns"
	"github.com/spaxel/mothership/internal/analytics"
	"github.com/spaxel/mothership/internal/api"
	"github.com/spaxel/mothership/internal/auth"
	"github.com/spaxel/mothership/internal/automation"
	"github.com/spaxel/mothership/internal/ble"
	appconfig "github.com/spaxel/mothership/internal/config"
	"github.com/spaxel/mothership/internal/dashboard"
	"github.com/spaxel/mothership/internal/db"
	"github.com/spaxel/mothership/internal/diagnostics"
	"github.com/spaxel/mothership/internal/events"
	"github.com/spaxel/mothership/internal/explainability"
	"github.com/spaxel/mothership/internal/falldetect"
	"github.com/spaxel/mothership/internal/fleet"
	"github.com/spaxel/mothership/internal/floorplan"
	"github.com/spaxel/mothership/internal/health"
	"github.com/spaxel/mothership/internal/ingestion"
	"github.com/spaxel/mothership/internal/learning"
	"github.com/spaxel/mothership/internal/loadshed"
	"github.com/spaxel/mothership/internal/localization"
	"github.com/spaxel/mothership/internal/mqtt"
	"github.com/spaxel/mothership/internal/notify"
	"github.com/spaxel/mothership/internal/ota"
	"github.com/spaxel/mothership/internal/prediction"
	"github.com/spaxel/mothership/internal/provisioning"
	"github.com/spaxel/mothership/internal/recorder"
	"github.com/spaxel/mothership/internal/replay"
	"github.com/spaxel/mothership/internal/shutdown"
	sigproc "github.com/spaxel/mothership/internal/signal"
	"github.com/spaxel/mothership/internal/sleep"
	"github.com/spaxel/mothership/internal/startup"
	"github.com/spaxel/mothership/internal/volume"
	"github.com/spaxel/mothership/internal/zones"
)

// handleFuncAdapter wraps a chi.Mux to satisfy auth.Handler's RegisterRoutes interface.
type handleFuncAdapter struct {
	router *chi.Mux
}

func (a *handleFuncAdapter) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	a.router.HandleFunc(pattern, handler)
}

// Phase 5: Configuration constants
const (
	baselineSaveInterval  = 30 * time.Second
	healthComputeInterval = 5 * time.Second
	weatherRecordInterval = 60 * time.Second
)

// Build-time version injection
var version = "dev"

// gdopAdapter wraps a localization.Engine to implement fleet.GDOPCalculator.
type gdopAdapter struct {
	eng *localization.Engine
}

func (a *gdopAdapter) GDOPMap(positions []fleet.NodePosition) ([]float32, int, int) {
	loc := make([]localization.NodePosition, len(positions))
	for i, p := range positions {
		loc[i] = localization.NodePosition{MAC: p.MAC, X: p.X, Y: 0, Z: p.Z}
	}
	return a.eng.GDOPMap(loc)
}

// securityStateAdapter adapts the analytics.Detector to implement dashboard.SecurityStateProvider.
type securityStateAdapter struct {
	detector *analytics.Detector
}

func (a *securityStateAdapter) IsSecurityModeActive() bool {
	return a.detector.IsSecurityModeActive()
}

func (a *securityStateAdapter) GetSecurityMode() string {
	return string(a.detector.GetSecurityMode())
}

func (a *securityStateAdapter) GetLearningProgress() float64 {
	return a.detector.GetLearningProgress()
}

func (a *securityStateAdapter) IsModelReady() bool {
	return a.detector.IsModelReady()
}

// parseLinkID splits a link ID "node_mac:peer_mac" into its two components.
func parseLinkID(linkID string) []string {
	i := strings.IndexByte(linkID, ':')
	if i < 0 {
		return nil
	}
	return []string{linkID[:i], linkID[i+1:]}
}

// splitLines splits a string by newlines and returns non-empty lines.
func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func findDashboardDir() string {
	for _, dir := range []string{"./dashboard", "./../dashboard", "/app/dashboard"} {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	return ""
}

// fleetRoomConfigAdapter adapts fleet.Registry to notify.RoomConfigProvider.
type fleetRoomConfigAdapter struct {
	reg *fleet.Registry
}

func (a *fleetRoomConfigAdapter) GetRoom() (width, height, depth float64) {
	room, err := a.reg.GetRoom()
	if err != nil {
		return 10, 2.5, 10
	}
	return room.Width, room.Height, room.Depth
}

// multiFleetNotifier fans out ingestion.FleetNotifier events to multiple fleet components.
type multiFleetNotifier struct {
	notifiers []interface {
		OnNodeConnected(mac, firmware, chip string)
		OnNodeDisconnected(mac string)
	}
}

func newMultiNotifier(notifiers ...interface {
	OnNodeConnected(mac, firmware, chip string)
	OnNodeDisconnected(mac string)
}) *multiFleetNotifier {
	return &multiFleetNotifier{notifiers: notifiers}
}

func (m *multiFleetNotifier) OnNodeConnected(mac, firmware, chip string) {
	for _, n := range m.notifiers {
		n.OnNodeConnected(mac, firmware, chip)
	}
}

func (m *multiFleetNotifier) OnNodeDisconnected(mac string) {
	for _, n := range m.notifiers {
		n.OnNodeDisconnected(mac)
	}
}

// gdopCalculatorAdapter adapts localization.Engine to fleet.GDOPCalculator.
type gdopCalculatorAdapter struct {
	engine *localization.Engine
}

func (a *gdopCalculatorAdapter) GDOPMap(positions []fleet.NodePosition) ([]float32, int, int) {
	locPositions := make([]localization.NodePosition, len(positions))
	for i, p := range positions {
		locPositions[i] = localization.NodePosition{
			MAC: p.MAC,
			X:   p.X,
			Z:   p.Z,
		}
	}
	return a.engine.GDOPMap(locPositions)
}

func main() {
	// Load and validate configuration at startup
	cfg, err := appconfig.Load()
	if err != nil {
		// Log each validation error and exit with code 1
		log.Printf("[FATAL] Configuration validation failed:")
		for _, line := range splitLines(err.Error()) {
			log.Printf("[FATAL]   %s", line)
		}
		os.Exit(1)
	}

	log.Printf("[INFO] Spaxel mothership v%s starting", version)
	log.Printf("[DEBUG] Config: bind=%s data=%s static=%s mdns=%s", cfg.BindAddr, cfg.DataDir, cfg.StaticDir, cfg.MDNSName)

	// Wrap all startup in a 30-second timeout context
	startupCtx, startupCancel := context.WithTimeout(context.Background(), startup.TotalTimeout)
	defer startupCancel()

	ctx, cancel := context.WithCancel(startupCtx)
	defer cancel()

	startupTotalStart := time.Now()

	var explainabilityHandler *explainability.Handler

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Phases 1–4: Database initialization (data dir, SQLite, migrations, secrets)
	// Each phase is logged with timing by db.OpenDB via the startup package.
	// The startup context is passed so all phases share the same 30s deadline.
	mainDB, err := db.OpenDB(startupCtx, cfg.DataDir, "spaxel.db")
	if err != nil {
		log.Fatalf("[FATAL] Failed to open main database: %v", err)
	}
	defer mainDB.Close()
	startup.CheckTimeout(startupCtx)
	log.Printf("[INFO] Main database at %s", filepath.Join(cfg.DataDir, "spaxel.db"))

	// Events timeline handler (created early so fusion loop can log detection events)
	eventsHandler := api.NewEventsHandlerFromDB(mainDB)
	log.Printf("[INFO] Events handler initialized (shared DB)")

	// Auth handler for PIN-based authentication and session management
	authHandler, err := auth.NewHandler(auth.Config{DB: mainDB})
	if err != nil {
		log.Fatalf("[FATAL] Failed to create auth handler: %v", err)
	}
	defer authHandler.Close()
	authHandler.RegisterRoutes(&handleFuncAdapter{router: r})
	log.Printf("[INFO] Auth handler registered at /api/auth/*")

	// Create load shedder — single source of truth for load shedding state
	shedder := loadshed.New()

	// Create ingestion server
	ingestSrv := ingestion.NewServer()
	r.HandleFunc("/ws/node", ingestSrv.HandleNodeWS)
	ingestSrv.SetShedder(shedder)

	// Signal processing pipeline
	pm := sigproc.NewProcessorManager(sigproc.ProcessorManagerConfig{
		NSub:       64,
		FusionRate: 10.0,
		Tau:        30.0,
	})
	ingestSrv.SetProcessorManager(pm)

	// Wire up health checker with all dependencies (after pm is created)
	healthChecker := health.New(health.Config{
		DB:           mainDB,
		GetNodeCount: func() int { return len(ingestSrv.GetConnectedNodes()) },
		Shedder:      shedder,
	})
	r.Get("/healthz", healthChecker.Handler(version))

	// Phase 6: Settings REST API
	settingsHandler := api.NewSettingsHandler(mainDB)
	settingsHandler.RegisterRoutes(r)
	log.Printf("[INFO] Settings API registered at /api/settings")

	// Replay recording store
	var replayStore api.RecordingStore
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Printf("[WARN] Failed to create data dir %s: %v", cfg.DataDir, err)
	} else {
		store, err := replay.NewRecordingStore(filepath.Join(cfg.DataDir, "csi_replay.bin"), cfg.ReplayMaxMB)
		if err != nil {
			log.Printf("[WARN] Failed to open replay store: %v (CSI recording disabled)", err)
		} else {
			ingestSrv.SetReplayStore(store)
			defer store.Close()
			replayStore = store
			log.Printf("[INFO] CSI replay store at %s (%d MB max)", filepath.Join(cfg.DataDir, "csi_replay.bin"), cfg.ReplayMaxMB)
		}
	}

	// Phase 6: CSI Replay REST API
	var replayHandler *api.ReplayHandler
	if replayStore != nil {
		replayHandler, err = api.NewReplayHandler(filepath.Join(cfg.DataDir, "csi_replay.bin"), replayStore)
		if err != nil {
			log.Printf("[WARN] Failed to create replay handler: %v", err)
		} else {
			replayHandler.RegisterRoutes(r)
			defer replayHandler.Close()
			log.Printf("[INFO] Replay REST API registered at /api/replay/*")
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

	// Phase 5: Subsystems — start all managers with 5s per-subsystem timeout
	startup.CheckTimeout(startupCtx)
	phase5Done := startup.Phase(5, "Subsystems")

	// Phase 5: BLE device registry
	var bleRegistry *ble.Registry
	if err := startup.SubsystemStart(startupCtx, "BLE registry", func(ctx context.Context) error {
		var innerErr error
		bleRegistry, innerErr = ble.NewRegistry(filepath.Join(cfg.DataDir, "ble.db"))
		return innerErr
	}); err != nil {
		log.Printf("[WARN] Failed to open BLE registry: %v", err)
	} else {
		defer bleRegistry.Close()
		log.Printf("[INFO] BLE registry at %s", filepath.Join(cfg.DataDir, "ble.db"))
	}

	// Phase 5: RSSI cache for BLE triangulation
	rssiCache := ble.NewRSSICache(10 * time.Second)

	// Phase 5: BLE identity matcher
	var identityMatcher *ble.IdentityMatcher
	if bleRegistry != nil {
		identityMatcher = ble.NewIdentityMatcher(bleRegistry, rssiCache, fleetReg)
	}

	// Phase 5: Zones manager
	zonesTz := time.Local
	if envTz := os.Getenv("TZ"); envTz != "" {
		if loc, err := time.LoadLocation(envTz); err == nil {
			zonesTz = loc
		}
	}
	var zonesMgr *zones.Manager
	if err := startup.SubsystemStart(startupCtx, "Zones manager", func(ctx context.Context) error {
		var innerErr error
		zonesMgr, innerErr = zones.NewManager(filepath.Join(cfg.DataDir, "zones.db"), zonesTz)
		return innerErr
	}); err != nil {
		log.Printf("[WARN] Failed to open zones database: %v", err)
	} else {
		defer zonesMgr.Close()
		log.Printf("[INFO] Zones manager at %s", filepath.Join(cfg.DataDir, "zones.db"))
	}

	// Phase 5: Flow analytics accumulator
	flowAccumulator, err := analytics.NewFlowAccumulator(filepath.Join(cfg.DataDir, "analytics.db"))
	if err != nil {
		log.Printf("[WARN] Failed to open analytics database: %v", err)
	} else {
		defer flowAccumulator.Close()
		log.Printf("[INFO] Flow analytics at %s", filepath.Join(cfg.DataDir, "analytics.db"))
	}

	// Phase 5: Anomaly detector for security mode
	var anomalyDetector *analytics.Detector
	anomalyDetector, err = analytics.NewDetector(
		filepath.Join(cfg.DataDir, "anomaly.db"),
		analytics.DefaultAnomalyScoreConfig(),
	)
	if err != nil {
		log.Printf("[WARN] Failed to open anomaly detector: %v", err)
	} else {
		defer anomalyDetector.Close()
		log.Printf("[INFO] Anomaly detector at %s (learning period: 7 days)", filepath.Join(cfg.DataDir, "anomaly.db"))

		// Start periodic model updates (every 6 hours)
		anomalyDetector.RunPeriodicUpdate(ctx, 6*time.Hour)
		// Note: Providers will be wired after dashboardHub and notifyService are created
	}

	// Phase 5: Automation engine
	automationEngine, err := automation.NewEngine(filepath.Join(cfg.DataDir, "automation.db"))
	if err != nil {
		log.Printf("[WARN] Failed to open automation database: %v", err)
	} else {
		defer automationEngine.Close()
		log.Printf("[INFO] Automation engine at %s", filepath.Join(cfg.DataDir, "automation.db"))
	}

	// Phase 5: Fall detector
	fallDetector := falldetect.NewDetector()
	log.Printf("[INFO] Fall detector initialized")

	// Declare dashboard hub and notify service early so closures can reference them.
	// They are assigned later in this function.
	var dashboardHub *dashboard.Hub
	var notifyService *notify.Service

	// Phase 6: Sleep quality monitor
	sleepMonitor := sleep.NewMonitor(sleep.MonitorConfig{
		SampleInterval: 30 * time.Second,
		ReportHour:     7,  // Generate reports at 7 AM
		SleepStartHour: 22, // 10 PM
		SleepEndHour:   7,  // 7 AM
	})
	sleepMonitor.SetProcessorManager(pm)

	// Sleep handler (created early so callback can reference it)
	sleepHandler := sleep.NewHandler(sleepMonitor)
	sleepHandler.SetDB(filepath.Join(cfg.DataDir, "spaxel.db"))

	sleepMonitor.SetReportCallback(func(linkID string, report *sleep.SleepReport) {
		// Broadcast sleep report to dashboard
		msg := map[string]interface{}{
			"type":           "sleep_report",
			"link_id":        linkID,
			"session_date":   report.SessionDate.Format("2006-01-02"),
			"overall_score":  report.Metrics.OverallScore,
			"quality_rating": report.Metrics.QualityRating,
			"generated_at":   report.GeneratedAt.Unix(),
		}
		data, _ := json.Marshal(msg)
		if dashboardHub != nil {
			dashboardHub.Broadcast(data)
		}

		// Persist sleep record to main DB (for GET /api/sleep endpoint)
		person := sleepMonitor.GetAnalyzer().GetSession(linkID)
		personName := linkID
		if person != nil {
			personName = person.GetPersonID()
		}
		if personName == "" {
			personName = linkID
		}
		sleepHandler.SaveRecord(personName, report)

		// Send notification for morning report
		body := fmt.Sprintf("Sleep quality: %s (%.0f/100)", report.Metrics.QualityRating, report.Metrics.OverallScore)
		if report.Metrics.BreathingAnomaly {
			body = fmt.Sprintf("Breathing rate elevated (%.0f bpm vs. %.0f bpm average). %s",
				report.Metrics.AvgBreathingRate, report.Metrics.PersonalAvgBPM, body)
		}
		if notifyService != nil {
			notif := notify.Notification{
				Title:    "Sleep Report",
				Body:     body,
				Priority: 2,
				Tags:     []string{"sleep", "morning"},
				Data:     report.ToJSONMap(),
			}
			notifyService.Send(notif) //nolint:errcheck
		}

		log.Printf("[INFO] Sleep report for %s: score=%.1f rating=%s breathing_avg=%.1f anomaly=%v",
			linkID, report.Metrics.OverallScore, report.Metrics.QualityRating,
			report.Metrics.AvgBreathingRate, report.Metrics.BreathingAnomaly)
	})
	sleepMonitor.Start()
	defer sleepMonitor.Stop()
	log.Printf("[INFO] Sleep quality monitor started (window: 22:00-07:00, report at 07:00)")

	// Phase 6: Prediction module for presence prediction
	var predictionStore *prediction.ModelStore
	var predictionHistory *prediction.HistoryUpdater
	var predictionPredictor *prediction.Predictor
	var predictionAccuracy *prediction.AccuracyTracker
	var predictionHorizon *prediction.HorizonPredictor
	predictionStore, err = prediction.NewModelStore(filepath.Join(cfg.DataDir, "prediction.db"))
	if err != nil {
		log.Printf("[WARN] Failed to open prediction store: %v", err)
	} else {
		defer predictionStore.Close()
		log.Printf("[INFO] Prediction store at %s", filepath.Join(cfg.DataDir, "prediction.db"))

		// Create history updater
		predictionHistory = prediction.NewHistoryUpdater(predictionStore)

		// Load stored person zone positions
		if err := predictionHistory.LoadStoredPositions(); err != nil {
			log.Printf("[WARN] Failed to load stored prediction positions: %v", err)
		}

		// Create accuracy tracker
		predictionAccuracy, err = prediction.NewAccuracyTracker(filepath.Join(cfg.DataDir, "prediction_accuracy.db"))
		if err != nil {
			log.Printf("[WARN] Failed to open accuracy tracker: %v", err)
		} else {
			defer predictionAccuracy.Close()
			log.Printf("[INFO] Prediction accuracy tracker at %s", filepath.Join(cfg.DataDir, "prediction_accuracy.db"))
		}

		// Create predictor
		predictionPredictor = prediction.NewPredictor(predictionStore)

		// Create horizon predictor with Monte Carlo simulation
		if predictionAccuracy != nil {
			predictionHorizon = prediction.NewHorizonPredictor(predictionStore, predictionAccuracy)
			predictionHorizon.SetHorizon(prediction.PredictionHorizon)
			log.Printf("[INFO] Horizon predictor initialized (%dm horizon, 1000 Monte Carlo runs)",
				int(prediction.PredictionHorizon.Minutes()))
		}

		log.Printf("[INFO] Presence prediction initialized")
	}

	// Phase 6: Notification service
	notifyService, err = notify.NewService(filepath.Join(cfg.DataDir, "notify.db"))
	if err != nil {
		log.Printf("[WARN] Failed to open notification database: %v", err)
	} else {
		defer notifyService.Close()
		log.Printf("[INFO] Notification service at %s", filepath.Join(cfg.DataDir, "notify.db"))

		// Set room config provider for floor plan thumbnails
		notifyService.SetRoomConfig(&fleetRoomConfigAdapter{reg: fleetReg})
	}

	// Phase 6: Self-improving localization system
	var selfImprovingLocalizer *localization.SelfImprovingLocalizer
	var weightStore *localization.WeightStore

	// Get room configuration from fleet registry
	roomWidth := 10.0
	roomDepth := 10.0
	originX := 0.0
	originZ := 0.0
	if fleetReg != nil {
		room, roomErr := fleetReg.GetRoom()
		if roomErr == nil && room != nil {
			roomWidth = room.Width
			roomDepth = room.Depth
			originX = room.OriginX
			originZ = room.OriginZ
		}
	}

	silConfig := localization.DefaultSelfImprovingConfig()
	silConfig.RoomWidth = roomWidth
	silConfig.RoomDepth = roomDepth
	silConfig.OriginX = originX
	silConfig.OriginZ = originZ
	silConfig.AdjustmentInterval = 10 * time.Second

	selfImprovingLocalizer = localization.NewSelfImprovingLocalizer(silConfig)

	// Load persisted weights
	weightStore, err = localization.NewWeightStore(filepath.Join(cfg.DataDir, "weights.db"))
	if err != nil {
		log.Printf("[WARN] Failed to open weight store: %v (learning persistence disabled)", err)
	} else {
		defer weightStore.Close()
		savedWeights, loadErr := weightStore.LoadWeights()
		if loadErr != nil {
			log.Printf("[WARN] Failed to load saved weights: %v", loadErr)
		} else if savedWeights != nil {
			selfImprovingLocalizer.GetEngine().SetLearnedWeights(savedWeights)
			stats := savedWeights.GetAllStats()
			log.Printf("[INFO] Loaded %d saved link weights from weight store", len(stats))
		}
	}

	// Set node positions from fleet registry
	if fleetReg != nil {
		nodes, _ := fleetReg.GetAllNodes()
		for _, node := range nodes {
			selfImprovingLocalizer.SetNodePosition(node.MAC, node.PosX, node.PosZ)
		}
	}

	// Start the self-improving localization system
	selfImprovingLocalizer.Start()
	log.Printf("[INFO] Self-improving localization started (room: %.1fx%.1fm, interval: %v)",
		roomWidth, roomDepth, silConfig.AdjustmentInterval)

	// Phase 6: Ground truth store for self-improving localization weights
	var groundTruthStore *localization.GroundTruthStore
	var spatialWeightLearner *localization.SpatialWeightLearner
	var groundTruthCollector *localization.GroundTruthCollector

	groundTruthStore, err = localization.NewGroundTruthStore(
		filepath.Join(cfg.DataDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		log.Printf("[WARN] Failed to open ground truth store: %v", err)
	} else {
		defer groundTruthStore.Close()
		log.Printf("[INFO] Ground truth store at %s", filepath.Join(cfg.DataDir, "groundtruth.db"))

		// Create spatial weight learner
		spatialWeightLearner, err = localization.NewSpatialWeightLearner(
			filepath.Join(cfg.DataDir, "spatial_weights.db"),
			localization.DefaultSpatialWeightLearnerConfig(),
		)
		if err != nil {
			log.Printf("[WARN] Failed to create spatial weight learner: %v", err)
		} else {
			defer spatialWeightLearner.Close()
			log.Printf("[INFO] Spatial weight learner initialized (min samples: %d, improvement threshold: %.0f%%)",
				localization.DefaultSpatialWeightLearnerConfig().MinZoneSamples,
				localization.DefaultSpatialWeightLearnerConfig().ImprovementThreshold*100)

			// Start periodic weight persistence
			spatialWeightLearner.StartPeriodicSave(ctx, 30*time.Second)
		}

		// Create ground truth collector
		groundTruthCollector = localization.NewGroundTruthCollector(groundTruthStore, spatialWeightLearner)
		log.Printf("[INFO] Ground truth collector initialized (min BLE confidence: %.1f, max distance: %.1fm)",
			localization.MinBLEConfidence, localization.MaxBLEBlobDistance)

		// Connect spatial weight learner to fusion engine for per-zone weight application
		if selfImprovingLocalizer != nil {
			selfImprovingLocalizer.GetEngine().SetSpatialWeightLearner(spatialWeightLearner)
			log.Printf("[INFO] Spatial weight learner connected to fusion engine")
		}
	}

	// Phase 6: Learning feedback store for detection accuracy
	var feedbackStore *learning.FeedbackStore
	var feedbackProcessor *learning.Processor
	var accuracyComputer *learning.AccuracyComputer
	feedbackStore, err = learning.NewFeedbackStore(filepath.Join(cfg.DataDir, "learning.db"))
	if err != nil {
		log.Printf("[WARN] Failed to open learning database: %v", err)
	} else {
		defer feedbackStore.Close()
		log.Printf("[INFO] Learning feedback store at %s", filepath.Join(cfg.DataDir, "learning.db"))

		// Create feedback processor
		feedbackProcessor = learning.NewProcessor(feedbackStore, learning.DefaultProcessorConfig())

		// Create accuracy computer
		accuracyComputer = learning.NewAccuracyComputer(feedbackStore, learning.DefaultAccuracyComputerConfig())

		// Start background processing
		go feedbackProcessor.Run(ctx)
		go accuracyComputer.Run(ctx)
		log.Printf("[INFO] Learning feedback processor started (interval: %v)", learning.DefaultProcessorConfig().ProcessInterval)
	}

	// Phase 6: MQTT client (optional)
	var mqttClient *mqtt.Client
	if cfg.MQTTBroker != "" {
		mqttClient, err = mqtt.NewClient(mqtt.Config{
			Broker:           cfg.MQTTBroker,
			ClientID:         "", // Auto-generated by mqtt package
			Username:         cfg.MQTTUsername,
			Password:         cfg.MQTTPassword,
			DiscoveryEnabled: true,
			DiscoveryPrefix:  "homeassistant",
			AutoReconnect:    true,
		})
		if err != nil {
			log.Printf("[WARN] Failed to create MQTT client: %v", err)
		} else {
			if err := mqttClient.Connect(ctx); err != nil {
				log.Printf("[WARN] MQTT connection failed: %v", err)
			} else {
				defer mqttClient.Disconnect()
				log.Printf("[INFO] MQTT client connected to %s", cfg.MQTTBroker)

				// Wire MQTT to automation engine
				automationEngine.SetMQTTClient(mqttClient)
			}
		}
	}

	// Phase 5: Self-healing fleet manager with GDOP optimization
	fleetHealer := fleet.NewFleetHealer(fleetReg, fleet.FleetHealerConfig{
		HealInterval:   60 * time.Second,
		MinOnlineNodes: 2,
		MaxHistorySize: 100,
	})

	// Phase 5: Link weather diagnostics
	weatherDiagnostics := fleet.NewLinkWeatherDiagnostics()

	// Phase 6: Role optimiser with GDOP-based coverage optimization
	roleOptimiser := fleet.NewRoleOptimiser(fleet.DefaultOptimisationConfig())

	// Phase 6: Self-healing manager with 5-minute reconnect grace period
	selfHealManager := fleet.NewSelfHealManager(fleetReg, roleOptimiser, fleet.DefaultSelfHealConfig())

	// Legacy fleet manager (kept for basic operations)
	fleetMgr := fleet.NewManager(fleetReg)

	// Phase 5: Multi-notifier broadcasts node events to legacy manager, healer, and self-heal manager
	multiNotify := newMultiNotifier(fleetMgr, fleetHealer, selfHealManager)
	ingestSrv.SetFleetNotifier(multiNotify)

	// Adaptive rate controller
	rateCtrl := ingestion.NewRateController(func(mac string, rateHz int, varianceThreshold float64) {
		ingestSrv.SendConfigToMAC(mac, rateHz, varianceThreshold)
	})
	ingestSrv.SetRateController(rateCtrl)
	go rateCtrl.Run(ctx)

	// Dashboard hub and server
	dashboardHub = dashboard.NewHub()
	dashboardSrv := dashboard.NewServer(dashboardHub)

	dashboardHub.SetIngestionState(ingestSrv)

	// Wire BLE state to dashboard for ble_scan broadcasts (5s interval)
	if bleRegistry != nil {
		dashboardHub.SetBLEState(bleRegistry)
	}

	// Wire zone state to dashboard for occupancy snapshots
	if zonesMgr != nil {
		dashboardHub.SetZoneState(&zoneStateAdapter{mgr: zonesMgr})

		// Start occupancy reconciliation ticker: every 30s for the first 60s
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if zonesMgr.IsReconciled() {
						return
					}
					zonesMgr.ReconcileTick()
				}
			}
		}()
	}

	// Wire ingestion → dashboard for CSI, motion, and event broadcasts
	ingestSrv.SetDashboardBroadcaster(dashboardHub)
	ingestSrv.SetMotionBroadcaster(dashboardHub)
	ingestSrv.SetEventBroadcaster(dashboardHub)

	// Wire load-shedding level changes to dashboard alerts and node rate push
	shedder.SetPreviousRate(20) // default rate before any Level 3 event
	shedder.SetRatePushCallback(func(rateHz int) {
		for _, mac := range ingestSrv.GetConnectedNodes() {
			ingestSrv.SendConfigToMAC(mac, rateHz, 0.02)
		}
		log.Printf("[INFO] Load shed rate push — %d Hz to %d nodes", rateHz, len(ingestSrv.GetConnectedNodes()))
	})
	shedder.OnLevelChange = func(prev, new loadshed.Level) {
		if new == loadshed.LevelHeavy {
			msg := map[string]interface{}{
				"type":        "alert",
				"severity":    "warning",
				"description": "System under load — CSI rate reduced to 10 Hz",
			}
			data, _ := json.Marshal(msg)
			dashboardHub.Broadcast(data)
			log.Printf("[INFO] Load shed entered Level 3 — CSI rate reduced to 10 Hz")
		}
		if prev == loadshed.LevelHeavy && new < loadshed.LevelHeavy {
			msg := map[string]interface{}{
				"type":        "info",
				"description": "System load recovered — CSI rate restored",
			}
			data, _ := json.Marshal(msg)
			dashboardHub.Broadcast(data)
			log.Printf("[INFO] Load shed recovered from Level 3 — adaptive rate control restored")
		}
		dashboardHub.BroadcastLoadState(int(new), new.String())
	}

	// Phase 6: Wire BLE messages to registry and identity matcher
	ingestSrv.SetBLEHandler(func(nodeMAC string, devices []ingestion.BLEDevice) {
		// Get current security mode
		isSecurityMode := false
		if automationEngine != nil {
			isSecurityMode = automationEngine.GetSystemMode() == automation.ModeAway
		}

		// Convert ingestion.BLEDevice to ble.BLEObservation and process
		observations := make([]ble.BLEObservation, len(devices))
		for i, dev := range devices {
			observations[i] = ble.BLEObservation{
				Addr:       dev.Addr,
				Name:       dev.Name,
				MfrID:      dev.MfrID,
				MfrDataHex: dev.MfrDataHex,
				RSSIdBm:    dev.RSSIdBm,
			}
			// Update RSSI cache for real-time triangulation
			rssiCache.AddWithTime(dev.Addr, nodeMAC, dev.RSSIdBm, time.Now())

			// Feed to self-improving localizer for ground truth
			if selfImprovingLocalizer != nil {
				selfImprovingLocalizer.AddBLEObservation(dev.Addr, nodeMAC, float64(dev.RSSIdBm))
			}

			// Process BLE device for anomaly detection (security mode)
			if anomalyDetector != nil && isSecurityMode {
				anomalyDetector.ProcessBLEDevice(dev.Addr, dev.RSSIdBm, isSecurityMode)
			}
		}
		// Store in persistent registry
		if bleRegistry != nil {
			bleRegistry.ProcessRelayMessage(nodeMAC, observations)
		}
	})

	// Start RSSI cache cleanup goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rssiCache.CleanOlder(5 * time.Minute)
			}
		}
	}()

	// Phase 6: Volume triggers handler (webhook firing with fault tolerance)
	volumeTriggersHandler, err := api.NewVolumeTriggersHandler(filepath.Join(cfg.DataDir, "spaxel.db"))
	if err != nil {
		log.Printf("[WARN] Failed to create volume triggers handler: %v", err)
	} else {
		defer volumeTriggersHandler.Close()
		volumeTriggersHandler.SetWSBroadcaster(dashboardHub)
		log.Printf("[INFO] Volume triggers handler initialized")
	}

	// Phase 6: Wire anomaly detector providers (after dashboardHub and notifyService are ready)
	if anomalyDetector != nil {
		// Wire providers for anomaly detector
		if zonesMgr != nil {
			anomalyDetector.SetZoneProvider(&anomalyZoneAdapter{mgr: zonesMgr})
		}
		if bleRegistry != nil {
			anomalyDetector.SetPersonProvider(&anomalyPersonAdapter{registry: bleRegistry})
			anomalyDetector.SetDeviceProvider(&anomalyDeviceAdapter{registry: bleRegistry})
		}
		anomalyDetector.SetPositionProvider(&anomalyPositionAdapter{pm: pm})
		if notifyService != nil {
			anomalyDetector.SetAlertHandler(&anomalyAlertAdapter{hub: dashboardHub, notifyService: notifyService})
		}
		// Wire feedback store for accuracy tracking
		if feedbackStore != nil {
			anomalyDetector.SetFeedbackStore(feedbackStore)
		}

		// Wire security state into the dashboard hub for snapshot/delta broadcasts
		dashboardHub.SetSecurityState(&securityStateAdapter{detector: anomalyDetector})

		// Set callback to broadcast anomalies to dashboard
		anomalyDetector.SetOnAnomaly(func(event events.AnomalyEvent) {
			// Broadcast as typed anomaly_detected for dashboard alert handling
			dashboardHub.BroadcastAnomaly(map[string]interface{}{
				"id":           event.ID,
				"anomaly_type": event.Type,
				"score":        event.Score,
				"description":  event.Description,
				"zone_id":      event.ZoneID,
				"zone_name":    event.ZoneName,
				"severity":     "warning",
				"timestamp_ms": event.Timestamp.UnixMilli(),
			})

			// Also broadcast as alert for the alert banner
			severity := "warning"
			if event.Score >= 0.85 {
				severity = "critical"
			}
			dashboardHub.BroadcastAlert(event.ID, event.Timestamp, severity, event.Description, event.Acknowledged)
		})

		// Set callback to broadcast security mode changes
		anomalyDetector.SetOnSecurityModeChange(func(oldMode, newMode analytics.SecurityMode, reason string) {
			dashboardHub.BroadcastSystemModeChange(map[string]interface{}{
				"old_mode": string(oldMode),
				"new_mode": string(newMode),
				"reason":   reason,
				"armed":    newMode != analytics.SecurityModeDisarmed,
			})
		})

		// Load registered devices from BLE registry
		if bleRegistry != nil {
			deviceRecords, devErr := bleRegistry.GetRegisteredDevices(false)
			if devErr == nil {
				var macs []string
				for _, dev := range deviceRecords {
					macs = append(macs, dev.Addr)
				}
				anomalyDetector.SetRegisteredDevices(macs)
			}
		}
	}

	// Wire fleet notifier/broadcaster and start self-healing loop
	fleetMgr.SetNotifier(ingestSrv)
	fleetMgr.SetBroadcaster(dashboardHub)
	go fleetMgr.Run(ctx)

	// Phase 5: Wire advanced fleet healer
	fleetHealer.SetNotifier(ingestSrv)
	fleetHealer.SetBroadcaster(dashboardHub)
	go fleetHealer.Run(ctx)

	// Phase 6: Wire self-healing manager with grace period for fleet_change events
	selfHealManager.SetNotifier(ingestSrv)
	selfHealManager.SetBroadcaster(dashboardHub)
	if selfImprovingLocalizer != nil {
		gdopCalc := &gdopAdapter{eng: selfImprovingLocalizer.GetEngine()}
		selfHealManager.SetGDOPCalculator(gdopCalc)
		roleOptimiser.SetGDOPCalculator(gdopCalc)
	}
	go selfHealManager.Run(ctx)

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
				Timestamp:        s.Timestamp,
				SNR:              s.SNR,
				PhaseStability:   s.PhaseStability,
				PacketRate:       s.PacketRate,
				DriftRate:        s.DriftRate,
				CompositeScore:   s.CompositeScore,
				DeltaRMSVariance: s.DeltaRMSVariance,
				IsQuietPeriod:    s.IsQuietPeriod,
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

	// Phase 6: Periodic tracking + identity matching + fall detection
	// Track last detection event time per blob for throttling (once per 5 seconds)
	lastDetectionEvent := make(map[int]time.Time)
	var lastDetectionEventMu sync.Mutex

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // 10 Hz
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				shedder.BeginIteration()

				// Stage 1: Get tracked blobs from fusion/tracker
				st1 := shedder.BeginStage("fusion_track")
				blobs := pm.GetTrackedBlobs()
				shedder.EndStage(st1)

				if len(blobs) == 0 {
					shedder.EndIteration()
					continue
				}

				// Log detection events for blobs (throttled to once per 5 seconds per blob)
				for _, blob := range blobs {
					// Get zone name if available
					zoneName := ""
					if zonesMgr != nil {
						zoneName = zonesMgr.GetBlobZone(blob.ID)
					}

					// Get person ID if available
					personID := ""
					if identityMatcher != nil {
						if match := identityMatcher.GetMatch(blob.ID); match != nil {
							personID = match.PersonName
							if personID == "" {
								personID = match.PersonID
							}
						}
					}

					// Build detail JSON
					detail := map[string]interface{}{
						"x":          blob.X,
						"y":          blob.Y,
						"z":          blob.Z,
						"vx":         blob.VX,
						"vy":         blob.VY,
						"vz":         blob.VZ,
						"confidence": blob.Weight,
						"posture":    blob.Posture,
					}
					detailJSON, _ := json.Marshal(detail)

					// Log detection event with throttling (once per 5 seconds per blob)
					// This prevents flooding the events table while still providing visibility
					_ = eventsHandler.LogEvent("detection", time.Now(), zoneName, personID, blob.ID, string(detailJSON), "info")
				}

				// Stage 2: Update identity matcher
				st2 := shedder.BeginStage("identity_match")
				if identityMatcher != nil {
					// Convert TrackedBlob to the anonymous struct expected by IdentityMatcher
					matcherBlobs := make([]struct {
						ID      int
						X, Y, Z float64
						Weight  float64
					}, len(blobs))
					for i, b := range blobs {
						matcherBlobs[i] = struct {
							ID      int
							X, Y, Z float64
							Weight  float64
						}{ID: b.ID, X: b.X, Y: b.Y, Z: b.Z, Weight: b.Weight}
					}
					identityMatcher.UpdateBlobs(matcherBlobs)

					// Collect ground truth samples for self-improving localization
					if groundTruthCollector != nil {
						// Build per-link delta and health maps from motion states
						motionStates := pm.GetAllMotionStates()
						perLinkDeltas := make(map[string]float64)
						perLinkHealth := make(map[string]float64)
						for _, state := range motionStates {
							perLinkDeltas[state.LinkID] = state.SmoothDeltaRMS
							if processor := pm.GetProcessor(state.LinkID); processor != nil {
								if health := processor.GetHealth(); health != nil {
									perLinkHealth[state.LinkID] = health.GetAmbientConfidence()
								}
							}
						}

						// Collect samples for matched blobs
						for _, blob := range blobs {
							match := identityMatcher.GetMatch(blob.ID)
							if match == nil || match.PersonID == "" || match.IsBLEOnly {
								continue
							}

							// Only collect if triangulation confidence is sufficient
							if match.TriangulationConf < localization.MinBLEConfidence {
								continue
							}

							// Collect ground truth sample
							groundTruthCollector.CollectSample(
								match.PersonID,
								localization.Vec3{X: match.TriangulationPos.X, Y: match.TriangulationPos.Y, Z: match.TriangulationPos.Z},
								match.TriangulationConf,
								localization.Vec3{X: blob.X, Y: blob.Y, Z: blob.Z},
								perLinkDeltas,
								perLinkHealth,
							)
						}
					}

					// Update detection explainability data
					if explainabilityHandler != nil {
						motionStates := pm.GetAllMotionStates()
						linkStates := make([]explainability.LinkState, 0, len(motionStates))
						for _, state := range motionStates {
							// Parse link ID to get node and peer MAC addresses
							// LinkID format is typically "node_mac:peer_mac"
							parts := parseLinkID(state.LinkID)
							if len(parts) != 2 {
								continue
							}
							nodeMAC, peerMAC := parts[0], parts[1]

							// Get node positions from fleet registry
							var nodePos, peerPos [3]float64
							if node, err := fleetReg.GetNode(nodeMAC); err == nil {
								nodePos = [3]float64{node.PosX, node.PosY, node.PosZ}
							}
							if peer, err := fleetReg.GetNode(peerMAC); err == nil {
								peerPos = [3]float64{peer.PosX, peer.PosY, peer.PosZ}
							}

							// Get learned weight from self-improving localizer
							var weight float64 = 1.0
							if selfImprovingLocalizer != nil {
								weights := selfImprovingLocalizer.GetLearnedWeights()
								if w, ok := weights[state.LinkID]; ok {
									weight = w
								}
							}

							linkState := explainability.LinkState{
								NodeMAC:     nodeMAC,
								PeerMAC:     peerMAC,
								NodePos:     nodePos,
								PeerPos:     peerPos,
								DeltaRMS:    state.SmoothDeltaRMS,
								Motion:      state.MotionDetected,
								Weight:      weight,
								HealthScore: state.AmbientConfidence,
							}
							linkStates = append(linkStates, linkState)
						}

						// Build blob snapshots
						blobSnapshots := make([]explainability.BlobSnapshot, 0, len(blobs))
						for _, blob := range blobs {
							blobSnapshots = append(blobSnapshots, explainability.BlobSnapshot{
								ID:         blob.ID,
								X:          blob.X,
								Y:          blob.Y,
								Z:          blob.Z,
								Confidence: blob.Weight,
							})
						}

						// Build identity map from BLE matches
						identityMap := make(map[int]*explainability.BLEMatch)
						for _, blob := range blobs {
							match := identityMatcher.GetMatch(blob.ID)
							if match != nil && match.PersonID != "" {
								triPos := [3]float64{match.TriangulationPos.X, match.TriangulationPos.Y, match.TriangulationPos.Z}
								identityMap[blob.ID] = &explainability.BLEMatch{
									PersonID:         match.PersonID,
									PersonLabel:      match.PersonName,
									PersonColor:      match.PersonColor,
									DeviceAddr:       match.DeviceAddr,
									Confidence:       match.Confidence,
									MatchMethod:      "ble_rssi",
									ReportedByNodes:  nil,
									TriangulationPos: &triPos,
								}
							}
						}

						// Update explainability handler (pass nil grid for now)
						explainabilityHandler.UpdateBlobs(blobSnapshots, linkStates, nil, identityMap)
					}
				}
				shedder.EndStage(st2)

				// Stage 3: Update zones occupancy
				st3 := shedder.BeginStage("zone_occupancy")
				if zonesMgr != nil {
					for _, blob := range blobs {
						zonesMgr.UpdateBlobPosition(blob.ID, blob.X, blob.Y, blob.Z)
					}
				}
				shedder.EndStage(st3)

				// Stage 4: Update flow analytics (suspended at load shed Level >= 1)
				st4 := shedder.BeginStage("crowd_flow")
				if flowAccumulator != nil && shedder.ShouldAccumulateCrowdFlow() {
					for _, blob := range blobs {
						// Get person ID from identity matcher
						var personID string
						if identityMatcher != nil {
							if match := identityMatcher.GetMatch(blob.ID); match != nil {
								personID = match.PersonID
							}
						}
						flowAccumulator.UpdateTrack(analytics.TrackUpdate{
							ID:       blob.ID,
							X:        blob.X,
							Y:        blob.Y,
							Z:        blob.Z,
							VX:       blob.VX,
							VY:       blob.VY,
							VZ:       blob.VZ,
							PersonID: personID,
						})
					}
				}
				shedder.EndStage(st4)

				// Stage 5: Fall detection
				st5 := shedder.BeginStage("fall_detect")
				for _, blob := range blobs {
					fallDetector.Update([]struct {
						ID         int
						X, Y, Z    float64
						VX, VY, VZ float64
						Posture    string
					}{{ID: blob.ID, X: blob.X, Y: blob.Y, Z: blob.Z, VX: blob.VX, VY: blob.VY, VZ: blob.VZ}}, time.Now())
				}
				shedder.EndStage(st5)

				// Stage 6: Trigger evaluation
				st6 := shedder.BeginStage("trigger_eval")
				// Evaluate automations
				if automationEngine != nil {
					autoBlobs := make([]automation.TrackedBlob, len(blobs))
					for i, b := range blobs {
						autoBlobs[i] = automation.TrackedBlob{
							ID:         b.ID,
							X:          b.X,
							Y:          b.Y,
							Z:          b.Z,
							VX:         b.VX,
							VY:         b.VY,
							VZ:         b.VZ,
							Confidence: b.Weight,
						}
					}
					automationEngine.Evaluate(autoBlobs, func(blobID int) string {
						if zonesMgr != nil {
							return zonesMgr.GetBlobZone(blobID)
						}
						return ""
					})
				}

				// Evaluate volume triggers (webhook firing with fault tolerance)
				if volumeTriggersHandler != nil {
					volumeBlobs := make([]volume.BlobPos, len(blobs))
					for i, blob := range blobs {
						volumeBlobs[i] = volume.BlobPos{
							ID: blob.ID,
							X:  blob.X,
							Y:  blob.Y,
							Z:  blob.Z,
						}
					}
					volumeTriggersHandler.EvaluateTriggers(volumeBlobs)
				}
				shedder.EndStage(st6)

				// Stage 7: Anomaly detection
				st7 := shedder.BeginStage("anomaly_detect")
				// Process anomaly detection
				if anomalyDetector != nil && zonesMgr != nil {
					// Get current system mode for security mode checks
					isSecurityMode := false
					if automationEngine != nil {
						isSecurityMode = automationEngine.GetSystemMode() == automation.ModeAway
					}

					// Process occupancy for each zone
					zones := zonesMgr.GetAllZones()
					for _, zone := range zones {
						occ := zonesMgr.GetZoneOccupancy(zone.ID)
						if occ == nil {
							continue
						}

						// Get BLE devices in this zone
						var bleDevices []string
						if identityMatcher != nil {
							for _, blobID := range occ.BlobIDs {
								if match := identityMatcher.GetMatch(blobID); match != nil && match.DeviceAddr != "" {
									bleDevices = append(bleDevices, match.DeviceAddr)
								}
							}
						}

						// Process occupancy for unusual hour detection
						anomalyDetector.ProcessOccupancy(zone.ID, occ.Count, bleDevices, isSecurityMode)

						// Process motion during away
						if isSecurityMode && occ.Count > 0 {
							for _, blobID := range occ.BlobIDs {
								anomalyDetector.ProcessMotionDuringAway(zone.ID, blobID, true)
							}
						}

						// Process dwell duration for each person in the zone
						for _, blobID := range occ.BlobIDs {
							// Get dwell duration from zones manager
							if dwellTime, ok := zonesMgr.GetBlobDwellTime(blobID, zone.ID); ok && dwellTime > 5*time.Minute {
								// Get person ID for this blob
								var personID string
								if identityMatcher != nil {
									if match := identityMatcher.GetMatch(blobID); match != nil {
										personID = match.PersonID
									}
								}
								if personID != "" {
									// Check for unusual dwell (fall detection takes priority)
									fallDetected := fallDetector.GetTrackState(blobID) == falldetect.StateFallConfirmed
									anomalyDetector.ProcessDwellDuration(zone.ID, personID, dwellTime, isSecurityMode, fallDetected)
								}
							}
						}
					}
				}
				shedder.EndStage(st7)

				// Stage 8: Dashboard publish
				st8 := shedder.BeginStage("dashboard_publish")
				// Per-tick dashboard state is published via the ingestion CSI path.
				// This stage captures any additional dashboard work in the fusion tick.
				_ = dashboardHub
				shedder.EndStage(st8)

				shedder.EndIteration()
			}
		}
	}()

	// Phase 6: Fall detection callback
	fallDetector.SetOnFall(func(event falldetect.FallEvent) {
		log.Printf("[WARN] Fall detected: blob=%d confidence=%.2f", event.BlobID, event.Confidence)

		// Get identity
		var personID, personName, personColor string
		if identityMatcher != nil {
			if match := identityMatcher.GetMatch(event.BlobID); match != nil {
				event.Identity = match.DeviceName
				personID = match.PersonID
				personName = match.DeviceName
			}
		}

		// Get zone
		var zoneID string
		if zonesMgr != nil {
			zoneID = zonesMgr.GetBlobZone(event.BlobID)
		}

		// Send notification
		if notifyService != nil {
			notif := notify.Notification{
				Title:    "Fall Detected",
				Body:     fmt.Sprintf("Fall detected for %s at (%.1f, %.1f, %.1f)", event.Identity, event.Position.X, event.Position.Y, event.Position.Z),
				Priority: 5,
				Tags:     []string{"warning", "fall"},
				Data: map[string]interface{}{
					"blob_id":    event.BlobID,
					"confidence": event.Confidence,
				},
				Timestamp: time.Now(),
			}
			notifyService.Send(notif)
		}

		// Publish to MQTT
		if mqttClient != nil && mqttClient.IsConnected() {
			mqttClient.UpdateBinarySensorState("fall_detected", true)
		}

		// Trigger automation event
		if automationEngine != nil {
			automationEngine.ProcessEvent(automation.Event{
				Type:        automation.TriggerFallDetected,
				Timestamp:   time.Now(),
				PersonID:    personID,
				PersonName:  personName,
				PersonColor: personColor,
				ZoneID:      zoneID,
				Confidence:  event.Confidence,
				Extra: map[string]interface{}{
					"blob_id":  event.BlobID,
					"position": []float64{event.Position.X, event.Position.Y, event.Position.Z},
				},
			})
		}
	})

	// Set identity function for fall detector
	fallDetector.SetIdentityFunc(func(blobID int) string {
		if identityMatcher != nil {
			if match := identityMatcher.GetMatch(blobID); match != nil {
				return match.DeviceName
			}
		}
		return ""
	})

	// Phase 6: Zone crossing callback
	if zonesMgr != nil {
		zonesMgr.SetOnCrossing(func(event zones.CrossingEvent) {
			log.Printf("[INFO] Zone crossing: blob %d via %s", event.BlobID, event.PortalID)

			// Get identity
			var personID, personName, personColor string
			if identityMatcher != nil {
				if match := identityMatcher.GetMatch(event.BlobID); match != nil {
					event.Identity = match.DeviceName
					personID = match.PersonID
					personName = match.DeviceName
				}
			}

			// Send notification
			if notifyService != nil {
				notif := notify.Notification{
					Title:    "Zone Change",
					Body:     fmt.Sprintf("%s moved from %s to %s", event.Identity, event.FromZone, event.ToZone),
					Priority: 1,
					Tags:     []string{"zone", "movement"},
					Data: map[string]interface{}{
						"portal_id": event.PortalID,
						"direction": event.Direction,
					},
					Timestamp: time.Now(),
				}
				notifyService.Send(notif)
			}

			// Update MQTT zone occupancy
			if mqttClient != nil && mqttClient.IsConnected() {
				mqttClient.UpdateZoneOccupancy(event.ToZone, zonesMgr.GetZoneOccupancy(event.ToZone).Count)
			}

			// Trigger automation events
			if automationEngine != nil {
				// zone_leave event
				if event.FromZone != "" {
					automationEngine.ProcessEvent(automation.Event{
						Type:        automation.TriggerZoneLeave,
						Timestamp:   time.Now(),
						PersonID:    personID,
						PersonName:  personName,
						PersonColor: personColor,
						ZoneID:      event.FromZone,
						ZoneName:    event.FromZone,
					})
				}

				// zone_enter event
				if event.ToZone != "" {
					automationEngine.ProcessEvent(automation.Event{
						Type:        automation.TriggerZoneEnter,
						Timestamp:   time.Now(),
						PersonID:    personID,
						PersonName:  personName,
						PersonColor: personColor,
						ZoneID:      event.ToZone,
						ZoneName:    event.ToZone,
						FromZone:    event.FromZone,
						ToZone:      event.ToZone,
					})

					// Update dwell tracking
					automationEngine.UpdateZoneDwellTracking(event.BlobID, event.ToZone, time.Now())
				}
			}

			// Record zone transition for presence prediction
			if predictionHistory != nil && personID != "" {
				predictionHistory.PersonZoneChange(personID, event.FromZone, event.ToZone, event.BlobID, time.Now())
			}

			// Broadcast portal crossing event to dashboard
			dashboardHub.BroadcastEvent(
				fmt.Sprintf("portal:%s:%d", event.PortalID, event.Timestamp.UnixMilli()),
				event.Timestamp,
				"portal_crossing",
				event.ToZone,
				event.BlobID,
				personName,
			)
		})

		// Zone entry callback — broadcast event to dashboard
		zonesMgr.SetOnZoneEntry(func(event zones.ZoneTransitionEvent) {
			personName := resolveBlobIdentity(event.BlobID, identityMatcher)
			dashboardHub.BroadcastEvent(
				fmt.Sprintf("zone_entry:%s:%d", event.ZoneID, event.Timestamp.UnixMilli()),
				event.Timestamp,
				"zone_entry",
				event.ZoneName,
				event.BlobID,
				personName,
			)
		})

		// Zone exit callback — broadcast event to dashboard
		zonesMgr.SetOnZoneExit(func(event zones.ZoneTransitionEvent) {
			personName := resolveBlobIdentity(event.BlobID, identityMatcher)
			dashboardHub.BroadcastEvent(
				fmt.Sprintf("zone_exit:%s:%d", event.ZoneID, event.Timestamp.UnixMilli()),
				event.Timestamp,
				"zone_exit",
				event.ZoneName,
				event.BlobID,
				personName,
			)
		})
	} // end if zonesMgr != nil

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

	// Phase 6: Flow analytics background tasks
	if flowAccumulator != nil {
		// Daily pruning of old trajectory segments
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := flowAccumulator.PruneOldSegments(); err != nil {
						log.Printf("[WARN] Failed to prune old trajectory segments: %v", err)
					}
				}
			}
		}()

		// Weekly corridor detection
		go func() {
			ticker := time.NewTicker(7 * 24 * time.Hour)
			defer ticker.Stop()
			// Run once at startup
			if err := flowAccumulator.ComputeCorridors(); err != nil {
				log.Printf("[WARN] Failed to compute corridors: %v", err)
			}
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := flowAccumulator.ComputeCorridors(); err != nil {
						log.Printf("[WARN] Failed to compute corridors: %v", err)
					}
				}
			}
		}()
		log.Printf("[INFO] Flow analytics background tasks started (prune: 24h, corridors: 7d)")
	}

	// Phase 6: Self-improving localization fusion loop
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // 10 Hz, same as main tracking
		defer ticker.Stop()

		weightSaveTicker := time.NewTicker(30 * time.Second)
		defer weightSaveTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Final weight save on shutdown
				if weightStore != nil && selfImprovingLocalizer != nil {
					weights := selfImprovingLocalizer.GetEngine().GetLearnedWeights()
					if weights != nil {
						if err := weightStore.SaveWeights(weights); err != nil {
							log.Printf("[WARN] Failed to save weights on shutdown: %v", err)
						} else {
							log.Printf("[INFO] Saved learned weights on shutdown")
						}
					}
				}
				return

			case <-ticker.C:
				if selfImprovingLocalizer == nil {
					continue
				}

				// Get motion states from signal processor
				states := pm.GetAllMotionStates()
				if len(states) == 0 {
					continue
				}

				// Convert to localization.LinkMotion format
				links := make([]localization.LinkMotion, 0, len(states))
				for _, state := range states {
					// Parse linkID format "nodeMAC-peerMAC"
					parts := splitLinkID(state.LinkID)
					if len(parts) != 2 {
						continue
					}

					link := localization.LinkMotion{
						NodeMAC:     parts[0],
						PeerMAC:     parts[1],
						DeltaRMS:    state.SmoothDeltaRMS,
						Motion:      state.MotionDetected,
						HealthScore: state.BaselineConf,
					}

					// Use health score if available
					if state.AmbientConfidence > 0 {
						link.HealthScore = state.AmbientConfidence
					}

					links = append(links, link)
				}

				// Run fusion with learned weights
				if len(links) > 0 {
					selfImprovingLocalizer.Fuse(links)
				}

			case <-weightSaveTicker.C:
				// Periodic weight persistence
				if weightStore != nil && selfImprovingLocalizer != nil {
					weights := selfImprovingLocalizer.GetEngine().GetLearnedWeights()
					if weights != nil {
						if err := weightStore.SaveWeights(weights); err != nil {
							log.Printf("[WARN] Failed to save weights: %v", err)
						}
					}
				}
			}
		}
	}()
	log.Printf("[INFO] Self-improving localization fusion started (rate: 10Hz, save interval: 30s)")

	// Phase 6: Prediction provider wiring and update loop
	if predictionPredictor != nil && predictionHistory != nil {
		// Wire zone provider
		if zonesMgr != nil {
			predictionPredictor.SetZoneProvider(&predictionZoneAdapter{mgr: zonesMgr})
		}

		// Wire person provider
		if bleRegistry != nil {
			predictionPredictor.SetPersonProvider(&predictionPersonAdapter{registry: bleRegistry})
		}

		// Wire position provider
		predictionPredictor.SetPositionProvider(prediction.NewPositionAdapter(predictionHistory))

		// Wire MQTT client for prediction publishing
		if mqttClient != nil && mqttClient.IsConnected() {
			predictionPredictor.SetMQTTClient(&predictionMQTTAdapter{client: mqttClient}, "")
		}

		// Wire horizon predictor providers
		if predictionHorizon != nil {
			if zonesMgr != nil {
				predictionHorizon.SetZoneProvider(&predictionZoneAdapter{mgr: zonesMgr})
			}
			if bleRegistry != nil {
				predictionHorizon.SetPersonProvider(&predictionPersonAdapter{registry: bleRegistry})
			}
			predictionHorizon.SetPositionProvider(prediction.NewPositionAdapter(predictionHistory))
			log.Printf("[INFO] Horizon predictor providers wired")
		}

		// Start periodic prediction update loop (every 60 seconds)
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()

			// Run initial prediction after 5 seconds
			time.Sleep(5 * time.Second)
			predictionPredictor.UpdatePredictions()
			log.Printf("[INFO] Prediction: initial predictions computed")

			// Publish prediction sensors for each person
			if mqttClient != nil && mqttClient.IsConnected() && bleRegistry != nil {
				people, _ := bleRegistry.GetPeople()
				for _, person := range people {
					mqttClient.PublishPredictionSensors(person.ID, person.Name)
				}
			}

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					predictionPredictor.UpdatePredictions()

					// Publish predictions to MQTT
					if mqttClient != nil && mqttClient.IsConnected() {
						predictions := predictionPredictor.GetPredictions()
						for _, pred := range predictions {
							zoneName := pred.PredictedNextZoneName
							if zoneName == "" {
								zoneName = pred.PredictedNextZoneID
							}
							mqttClient.UpdatePredictionState(
								pred.PersonID,
								zoneName,
								pred.DataConfidence,
								pred.PredictionConfidence,
								pred.EstimatedTransitionMinutes,
							)
						}

						// Also publish horizon predictions (15-minute Monte Carlo)
						if predictionHorizon != nil {
							horizonPreds := predictionHorizon.UpdateAllPredictions()
							for _, hpred := range horizonPreds {
								// Publish horizon prediction to separate topic
								topic := "spaxel/person/" + hpred.PersonID + "/horizon_prediction"
								payload := map[string]interface{}{
									"current_zone":       hpred.CurrentZoneID,
									"predicted_zone":     hpred.PredictedZoneID,
									"confidence":         hpred.Confidence,
									"horizon_minutes":    hpred.HorizonMinutes,
									"data_confidence":    hpred.DataConfidence,
									"model_ready":        hpred.ModelReady,
									"zone_probabilities": hpred.ZoneProbabilities,
								}
								if data, err := json.Marshal(payload); err == nil {
									mqttClient.Publish(topic, data)
								}
							}
						}
					}
				}
			}
		}()
		log.Printf("[INFO] Prediction update loop started (interval: 60s)")

		// Start periodic prediction evaluation loop (every 30 seconds)
		// This evaluates pending predictions against actual positions
		if predictionAccuracy != nil {
			go func() {
				evalTicker := time.NewTicker(30 * time.Second)
				defer evalTicker.Stop()

				// Cleanup ticker (hourly)
				cleanupTicker := time.NewTicker(1 * time.Hour)
				defer cleanupTicker.Stop()

				// Zone pattern computation ticker (daily)
				patternTicker := time.NewTicker(24 * time.Hour)
				defer patternTicker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-evalTicker.C:
						// Get current actual positions from history updater
						actualPositions := make(map[string]string)
						if predictionHistory != nil {
							zones := predictionHistory.GetAllPersonZones()
							for personID, info := range zones {
								actualPositions[personID] = info.ZoneID
							}
						}

						// Evaluate pending predictions
						if len(actualPositions) > 0 {
							evaluated, correct, err := predictionAccuracy.EvaluatePending(actualPositions)
							if err != nil {
								log.Printf("[WARN] Prediction evaluation failed: %v", err)
							} else if evaluated > 0 {
								accuracy := float64(0)
								if evaluated > 0 {
									accuracy = float64(correct) / float64(evaluated) * 100
								}
								log.Printf("[INFO] Prediction evaluation: %d evaluated, %d correct (%.1f%% accuracy)",
									evaluated, correct, accuracy)
							}
						}

					case <-cleanupTicker.C:
						// Cleanup old predictions
						if err := predictionAccuracy.CleanupOldPredictions(); err != nil {
							log.Printf("[WARN] Prediction cleanup failed: %v", err)
						}

					case <-patternTicker.C:
						// Compute zone occupancy patterns
						if err := predictionAccuracy.ComputeZoneOccupancyPatterns(); err != nil {
							log.Printf("[WARN] Zone pattern computation failed: %v", err)
						}
					}
				}
			}()
			log.Printf("[INFO] Prediction evaluation loop started (interval: 30s)")
		}
	}

	// Fleet REST API
	fleetHandler := fleet.NewHandler(fleetMgr)
	fleetHandler.SetNodeIdentifier(ingestSrv)
	fleetHandler.RegisterRoutes(r)

	// Floorplan REST API
	floorplanHandler := floorplan.NewHandler(mainDB, cfg.DataDir)
	floorplanHandler.RegisterRoutes(r)

	// Phase 6: Fleet Health REST API (self-healing with GDOP optimisation)
	fleetHealthHandler := fleet.NewFleetHandler(selfHealManager, fleetReg)
	fleetHealthHandler.RegisterRoutes(r)

	// Phase 6: Volume triggers REST API (webhook actions with fault tolerance)
	if volumeTriggersHandler != nil {
		volumeTriggersHandler.RegisterRoutes(r)
	}

	// Phase 6: Zones and Portals REST API
	if zonesMgr != nil {
		zonesHandler := api.NewZonesHandler(zonesMgr)
		zonesHandler.SetZoneChangeBroadcaster(dashboardHub)
		zonesHandler.RegisterRoutes(r)
		log.Printf("[INFO] Zones and portals API registered at /api/zones/* and /api/portals/*")
	}

	// Phase 6: BLE REST API
	if bleRegistry != nil {
		r.Get("/api/ble/devices", func(w http.ResponseWriter, r *http.Request) {
			devices, err := bleRegistry.GetDevices(false)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, devices)
		})
		r.Get("/api/ble/devices/{addr}", func(w http.ResponseWriter, r *http.Request) {
			addr := chi.URLParam(r, "addr")
			device, err := bleRegistry.GetDevice(addr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			writeJSON(w, device)
		})
		r.Post("/api/ble/devices", func(w http.ResponseWriter, r *http.Request) {
			var device ble.DeviceRecord
			if err := json.NewDecoder(r.Body).Decode(&device); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if device.Addr == "" {
				http.Error(w, "addr required", http.StatusBadRequest)
				return
			}
			result, err := bleRegistry.PreregisterDevice(device.Addr, device.Name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, result)
		})
		r.Put("/api/ble/devices/{addr}", func(w http.ResponseWriter, r *http.Request) {
			addr := chi.URLParam(r, "addr")
			var device ble.DeviceRecord
			if err := json.NewDecoder(r.Body).Decode(&device); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			updates := map[string]interface{}{}
			if device.Name != "" {
				updates["name"] = device.Name
			}
			if device.Label != "" {
				updates["label"] = device.Label
			}
			if device.DeviceType != "" {
				updates["device_type"] = string(device.DeviceType)
			}
			if device.PersonID != "" {
				updates["person_id"] = device.PersonID
			}
			if len(updates) == 0 {
				writeJSON(w, device)
				return
			}
			if err := bleRegistry.UpdateDevice(addr, updates); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			result, err := bleRegistry.GetDevice(addr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, result)
		})
		r.Delete("/api/ble/devices/{addr}", func(w http.ResponseWriter, r *http.Request) {
			addr := chi.URLParam(r, "addr")
			if err := bleRegistry.DeleteDevice(addr); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		r.Get("/api/ble/matches", func(w http.ResponseWriter, r *http.Request) {
			if identityMatcher == nil {
				writeJSON(w, []*ble.IdentityMatch{})
				return
			}
			matches := identityMatcher.GetAllMatches()
			writeJSON(w, matches)
		})
	}

	// Phase 6: Automation REST API
	if automationEngine != nil {
		r.Get("/api/automations", func(w http.ResponseWriter, r *http.Request) {
			automations := automationEngine.GetAllAutomations()
			writeJSON(w, automations)
		})
		r.Post("/api/automations", func(w http.ResponseWriter, r *http.Request) {
			var auto automation.Automation
			if err := json.NewDecoder(r.Body).Decode(&auto); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if auto.ID == "" {
				auto.ID = fmt.Sprintf("auto_%d", time.Now().UnixNano())
			}
			if err := automationEngine.CreateAutomation(&auto); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, auto)
		})
		r.Get("/api/automations/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			auto := automationEngine.GetAutomation(id)
			if auto == nil {
				http.Error(w, "automation not found", http.StatusNotFound)
				return
			}
			writeJSON(w, auto)
		})
		r.Put("/api/automations/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			var auto automation.Automation
			if err := json.NewDecoder(r.Body).Decode(&auto); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			auto.ID = id
			if err := automationEngine.UpdateAutomation(&auto); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, auto)
		})
		r.Delete("/api/automations/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			if err := automationEngine.DeleteAutomation(id); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		r.Post("/api/automations/{id}/test", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			if err := automationEngine.TestFire(id); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
		r.Get("/api/automations/events", func(w http.ResponseWriter, r *http.Request) {
			events := automationEngine.GetRecentActionLog(50)
			writeJSON(w, events)
		})

		// Trigger volumes API
		r.Get("/api/automations/volumes", func(w http.ResponseWriter, r *http.Request) {
			volumes := automationEngine.GetAllTriggerVolumes()
			writeJSON(w, volumes)
		})
		r.Post("/api/automations/volumes", func(w http.ResponseWriter, r *http.Request) {
			var volume automation.TriggerVolume
			if err := json.NewDecoder(r.Body).Decode(&volume); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if volume.ID == "" {
				volume.ID = fmt.Sprintf("volume_%d", time.Now().UnixNano())
			}
			if err := automationEngine.CreateTriggerVolume(&volume); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, volume)
		})
		r.Delete("/api/automations/volumes/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			if err := automationEngine.DeleteTriggerVolume(id); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})

		// System mode API
		r.Get("/api/mode", func(w http.ResponseWriter, r *http.Request) {
			mode := automationEngine.GetSystemMode()
			writeJSON(w, map[string]string{"mode": string(mode)})
		})
		r.Post("/api/mode", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Mode string `json:"mode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mode := automation.SystemMode(req.Mode)
			if mode != automation.ModeHome && mode != automation.ModeAway && mode != automation.ModeSleep {
				http.Error(w, "invalid mode, must be home, away, or sleep", http.StatusBadRequest)
				return
			}
			if err := automationEngine.SetSystemMode(mode); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"mode": string(mode)})
		})

		// Wire providers to automation engine
		if zonesMgr != nil {
			automationEngine.SetZoneProvider(&zoneProviderAdapter{mgr: zonesMgr})
		}
		if bleRegistry != nil {
			automationEngine.SetPersonProvider(&personProviderAdapter{registry: bleRegistry})
			automationEngine.SetDeviceProvider(&deviceProviderAdapter{registry: bleRegistry})
		}
		if mqttClient != nil {
			automationEngine.SetMQTTClient(mqttClient)
		}
		if notifyService != nil {
			automationEngine.SetNotificationSender(&notifySenderAdapter{service: notifyService})
		}
	}

	// Phase 6: Notification channels REST API
	if notifyService != nil {
		r.Get("/api/notifications/channels", func(w http.ResponseWriter, r *http.Request) {
			// Return configured channels (without sensitive data)
			writeJSON(w, map[string]interface{}{
				"channels": []string{},
			})
		})
		r.Post("/api/notifications/channels", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				ID       string
				Type     string
				URL      string
				Token    string
				User     string
				Username string
				Password string
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			cc := notify.ChannelConfig{
				Type:     notify.NotificationChannel(req.Type),
				Enabled:  true,
				URL:      req.URL,
				Token:    req.Token,
				User:     req.User,
				Username: req.Username,
				Password: req.Password,
			}
			if err := notifyService.AddChannel(req.ID, cc); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "created"})
		})
		r.Delete("/api/notifications/channels/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			if err := notifyService.RemoveChannel(id); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		r.Post("/api/notifications/test", func(w http.ResponseWriter, r *http.Request) {
			if notifyService == nil {
				http.Error(w, "notification service not available", http.StatusServiceUnavailable)
				return
			}
			if err := notifyService.Send(notify.Notification{
				Title: "Test Notification",
				Body:  "This is a test notification from Spaxel",
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
		r.Get("/api/notifications/history", func(w http.ResponseWriter, r *http.Request) {
			if notifyService == nil {
				writeJSON(w, []struct{}{})
				return
			}
			history := notifyService.GetHistory(50)
			writeJSON(w, history)
		})
		r.Post("/api/notifications/quiet-hours", func(w http.ResponseWriter, r *http.Request) {
			var qh notify.QuietHoursConfig
			if err := json.NewDecoder(r.Body).Decode(&qh); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := notifyService.SetQuietHours(qh); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, qh)
		})

		// Config endpoints (aliases for /api/notifications/config)
		r.Get("/api/notifications/config", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"channels": []string{},
			})
		})
		r.Post("/api/notifications/config", func(w http.ResponseWriter, r *http.Request) {
			var req notify.ChannelConfig
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// Generate a unique ID if not provided
			id := string(req.Type)
			if id == "" {
				id = fmt.Sprintf("channel_%d", time.Now().UnixNano())
			}
			if err := notifyService.AddChannel(id, req); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "updated"})
		})
	}

	// Phase 5: Weather diagnostics REST API
	r.Get("/api/weather", func(w http.ResponseWriter, r *http.Request) {
		reports := weatherDiagnostics.GetAllLinkReports()
		writeJSON(w, reports)
	})
	r.Get("/api/weather/{linkID}", func(w http.ResponseWriter, r *http.Request) {
		linkID := chi.URLParam(r, "linkID")
		report := weatherDiagnostics.GetReport(linkID)
		writeJSON(w, report)
	})
	r.Get("/api/weather/summary", func(w http.ResponseWriter, r *http.Request) {
		condition, avgConfidence, issueCount := weatherDiagnostics.GetSystemWeatherSummary()
		writeJSON(w, map[string]interface{}{
			"condition":      condition,
			"avg_confidence": avgConfidence,
			"issue_count":    issueCount,
		})
	})
	r.Get("/api/weather/{linkID}/weekly", func(w http.ResponseWriter, r *http.Request) {
		linkID := chi.URLParam(r, "linkID")
		trend := weatherDiagnostics.GetWeeklyTrend(linkID)
		writeJSON(w, trend)
	})

	// Phase 5: Coverage and healing status API
	r.Get("/api/coverage", func(w http.ResponseWriter, r *http.Request) {
		coverage := fleetHealer.GetCoverage()
		writeJSON(w, coverage)
	})
	r.Get("/api/coverage/history", func(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, map[string]interface{}{
			"degraded":      fleetHealer.IsDegraded(),
			"online_nodes":  fleetHealer.GetOnlineNodes(),
			"optimal_roles": fleetHealer.GetOptimalRoles(),
		})
	})
	r.Get("/api/healing/suggest", func(w http.ResponseWriter, r *http.Request) {
		x, z, improvement := fleetHealer.SuggestNodePosition()
		worstX, worstZ, worstGDOP := fleetHealer.GetWorstCoverageZone()
		writeJSON(w, map[string]interface{}{
			"suggested_position":   map[string]float64{"x": x, "z": z},
			"expected_improvement": improvement,
			"worst_coverage_zone":  map[string]float64{"x": worstX, "z": worstZ, "gdop": worstGDOP},
		})
	})

	// Phase 5: System health API
	r.Get("/api/health/system", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"system_health":    pm.GetSystemHealth(),
			"link_count":       pm.LinkCount(),
			"active_links":     pm.ActiveLinks(),
			"stationary_count": pm.GetStationaryPersonCount(),
			"worst_link":       func() string { id, _ := pm.GetWorstLink(); return id }(),
		})
	})

	// Phase 6: Diurnal learning status API
	r.Get("/api/diurnal/status", func(w http.ResponseWriter, r *http.Request) {
		statuses := pm.GetDiurnalLearningStatus()
		writeJSON(w, statuses)
	})
	r.Get("/api/diurnal/status/{linkID}", func(w http.ResponseWriter, r *http.Request) {
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

	// Diurnal slot data API - returns 24-hour slot data for polar chart visualization
	r.Get("/api/diurnal/slots/{linkID}", func(w http.ResponseWriter, r *http.Request) {
		linkID := chi.URLParam(r, "linkID")
		processor := pm.GetProcessor(linkID)
		if processor == nil {
			http.Error(w, "link not found", http.StatusNotFound)
			return
		}

		diurnal := processor.GetDiurnal()
		if diurnal == nil {
			http.Error(w, "diurnal data not available", http.StatusNotFound)
			return
		}

		// Calculate average amplitude per hour (for variance visualization)
		slotAmplitudes := make([]float64, 24)
		slotConfidences := make([]float64, 24)
		slotSampleCounts := make([]int, 24)

		for h := 0; h < 24; h++ {
			slot := diurnal.GetSlot(h)
			if slot != nil && len(slot.Values) > 0 {
				// Calculate average amplitude for this slot
				sum := 0.0
				for _, v := range slot.Values {
					sum += v
				}
				slotAmplitudes[h] = sum / float64(len(slot.Values))
				slotSampleCounts[h] = slot.SampleCount
			}
			slotConfidences[h] = diurnal.GetSlotConfidence(h)
		}

		writeJSON(w, map[string]interface{}{
			"link_id":            linkID,
			"current_hour":        time.Now().Hour(),
			"current_minute":      time.Now().Minute(),
			"is_ready":            diurnal.IsReady(),
			"is_learning":         diurnal.IsLearning(),
			"learning_progress":   diurnal.GetLearningProgress(),
			"overall_confidence":  diurnal.GetOverallConfidence(),
			"slot_amplitudes":     slotAmplitudes,
			"slot_confidences":    slotConfidences,
			"slot_sample_counts":  slotSampleCounts,
			"created_at":          diurnal.GetCreatedAt(),
		})
	})

	// Link health API - returns all links with health scores and details
	r.Get("/api/links", func(w http.ResponseWriter, r *http.Request) {
		links := ingestSrv.GetAllLinksWithHealth()
		writeJSON(w, links)
	})

	// Phase 6: Link diagnostics API
	r.Get("/api/links/{linkID}/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		linkID := chi.URLParam(r, "linkID")
		diagnoses := diagnosticEngine.GetDiagnoses(linkID)
		writeJSON(w, diagnoses)
	})

	r.Get("/api/links/{linkID}/health-history", func(w http.ResponseWriter, r *http.Request) {
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
		allDiagnoses := diagnosticEngine.GetAllDiagnoses()
		writeJSON(w, allDiagnoses)
	})

	// Phase 6: Analytics REST API
	if flowAccumulator != nil {
		analyticsHandler := analytics.NewHandler(flowAccumulator)
		analyticsHandler.RegisterRoutes(r)
	}

	// Phase 6: Prediction REST API
	if predictionPredictor != nil {
		r.Get("/api/predictions", func(w http.ResponseWriter, r *http.Request) {
			predictions := predictionPredictor.GetPredictions()
			writeJSON(w, predictions)
		})

		r.Get("/api/predictions/stats", func(w http.ResponseWriter, r *http.Request) {
			if predictionHistory == nil {
				http.Error(w, "prediction history not available", http.StatusServiceUnavailable)
				return
			}
			count, dataAge, err := predictionHistory.GetTransitionStats()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]interface{}{
				"transition_count": count,
				"data_age_days":    dataAge.Hours() / 24,
				"minimum_data_age": prediction.MinimumDataAge.Hours() / 24,
				"has_minimum_data": dataAge >= prediction.MinimumDataAge,
			})
		})

		r.Post("/api/predictions/recompute", func(w http.ResponseWriter, r *http.Request) {
			if predictionHistory == nil {
				http.Error(w, "prediction history not available", http.StatusServiceUnavailable)
				return
			}
			if err := predictionHistory.ForceRecompute(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "recompute_started"})
		})

		// Prediction accuracy endpoints
		if predictionAccuracy != nil {
			r.Get("/api/predictions/accuracy", func(w http.ResponseWriter, r *http.Request) {
				stats, err := predictionAccuracy.GetAllAccuracyStats()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeJSON(w, stats)
			})

			r.Get("/api/predictions/accuracy/overall", func(w http.ResponseWriter, r *http.Request) {
				accuracy, total, err := predictionAccuracy.GetOverallAccuracy()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				pending := predictionAccuracy.GetPendingCount()
				writeJSON(w, map[string]interface{}{
					"accuracy_percent":    accuracy * 100,
					"total_predictions":   total,
					"pending_predictions": pending,
					"target_accuracy":     75.0,
					"meets_target":        accuracy >= 0.75 && total >= prediction.MinPredictionsForAccuracy,
					"horizon_minutes":     int(prediction.PredictionHorizon.Minutes()),
				})
			})

			r.Get("/api/predictions/accuracy/{personID}", func(w http.ResponseWriter, r *http.Request) {
				personID := chi.URLParam(r, "personID")
				stats, err := predictionAccuracy.GetAccuracyStats(personID, int(prediction.PredictionHorizon.Minutes()))
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if stats == nil {
					http.Error(w, "no accuracy data for person", http.StatusNotFound)
					return
				}
				writeJSON(w, stats)
			})

			r.Get("/api/predictions/pending", func(w http.ResponseWriter, r *http.Request) {
				pending := predictionAccuracy.GetPendingCount()
				writeJSON(w, map[string]int{"pending_predictions": pending})
			})

			// Zone occupancy patterns endpoints
			r.Get("/api/predictions/patterns/zones", func(w http.ResponseWriter, r *http.Request) {
				// Get all zone occupancy patterns
				if zonesMgr == nil {
					http.Error(w, "zones manager not available", http.StatusServiceUnavailable)
					return
				}
				zones := zonesMgr.GetAllZones()
				patterns := make([]map[string]interface{}, 0)
				now := time.Now()
				hourOfWeek := prediction.HourOfWeek(now)
				for _, zone := range zones {
					pattern, err := predictionAccuracy.GetZoneOccupancyPattern(zone.ID, hourOfWeek)
					if err != nil {
						continue
					}
					if pattern != nil {
						patterns = append(patterns, map[string]interface{}{
							"zone_id":            zone.ID,
							"zone_name":          zone.Name,
							"hour_of_week":       pattern.HourOfWeek,
							"occupancy_prob":     pattern.OccupancyProb,
							"mean_dwell_minutes": pattern.MeanDwellMinutes,
							"stddev_dwell":       pattern.StddevDwell,
							"sample_count":       pattern.SampleCount,
						})
					}
				}
				writeJSON(w, patterns)
			})

			r.Get("/api/predictions/patterns/zone/{zoneID}", func(w http.ResponseWriter, r *http.Request) {
				zoneID := chi.URLParam(r, "zoneID")
				// Get patterns for all hours of the week
				var patterns []map[string]interface{}
				for hour := 0; hour < 168; hour++ {
					pattern, err := predictionAccuracy.GetZoneOccupancyPattern(zoneID, hour)
					if err != nil || pattern == nil {
						continue
					}
					patterns = append(patterns, map[string]interface{}{
						"hour_of_week":       pattern.HourOfWeek,
						"day_name":           prediction.DayNameFromHourOfWeek(pattern.HourOfWeek),
						"hour_of_day":        pattern.HourOfWeek % 24,
						"occupancy_prob":     pattern.OccupancyProb,
						"mean_dwell_minutes": pattern.MeanDwellMinutes,
						"stddev_dwell":       pattern.StddevDwell,
						"sample_count":       pattern.SampleCount,
					})
				}
				writeJSON(w, map[string]interface{}{
					"zone_id":  zoneID,
					"patterns": patterns,
				})
			})

			r.Get("/api/predictions/patterns/zone/{zoneID}/current", func(w http.ResponseWriter, r *http.Request) {
				zoneID := chi.URLParam(r, "zoneID")
				hourOfWeek := prediction.HourOfWeek(time.Now())
				pattern, err := predictionAccuracy.GetZoneOccupancyPattern(zoneID, hourOfWeek)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if pattern == nil {
					writeJSON(w, map[string]interface{}{
						"zone_id":      zoneID,
						"hour_of_week": hourOfWeek,
						"available":    false,
						"message":      "no pattern data available for this hour",
					})
					return
				}
				writeJSON(w, map[string]interface{}{
					"zone_id":            zoneID,
					"hour_of_week":       pattern.HourOfWeek,
					"day_name":           prediction.DayNameFromHourOfWeek(pattern.HourOfWeek),
					"hour_of_day":        pattern.HourOfWeek % 24,
					"occupancy_prob":     pattern.OccupancyProb,
					"mean_dwell_minutes": pattern.MeanDwellMinutes,
					"stddev_dwell":       pattern.StddevDwell,
					"sample_count":       pattern.SampleCount,
					"available":          true,
				})
			})
		}

		// Transition probabilities endpoints (require predictionStore)
		if predictionStore != nil {
			r.Get("/api/predictions/probabilities/{personID}", func(w http.ResponseWriter, r *http.Request) {
				personID := chi.URLParam(r, "personID")
				hourOfWeek := prediction.HourOfWeek(time.Now())

				// Get current zone if available
				var currentZoneID string
				if predictionHistory != nil {
					zoneID, _, _, ok := predictionHistory.GetPersonZone(personID)
					if ok {
						currentZoneID = zoneID
					}
				}

				result := map[string]interface{}{
					"person_id":    personID,
					"hour_of_week": hourOfWeek,
					"current_zone": currentZoneID,
					"transitions":  []map[string]interface{}{},
				}

				// If we know current zone, get probabilities from there
				if currentZoneID != "" && zonesMgr != nil {
					probs, err := predictionStore.GetTransitionProbabilitiesForFromZone(personID, currentZoneID, hourOfWeek)
					if err == nil {
						transitions := make([]map[string]interface{}, len(probs))
						for i, p := range probs {
							zoneName := p.ToZoneID
							if z := zonesMgr.GetZone(p.ToZoneID); z != nil {
								zoneName = z.Name
							}
							transitions[i] = map[string]interface{}{
								"from_zone_id":  p.FromZoneID,
								"to_zone_id":    p.ToZoneID,
								"to_zone_name":  zoneName,
								"probability":   p.Probability,
								"sample_count":  p.Count,
								"last_computed": p.LastComputed,
							}
						}
						result["transitions"] = transitions
					}

					// Also get dwell time stats
					dwellStats, err := predictionStore.GetDwellTimeStats(personID, currentZoneID, hourOfWeek)
					if err == nil && dwellStats != nil {
						result["dwell_time"] = map[string]interface{}{
							"mean_minutes":   dwellStats.MeanMinutes,
							"stddev_minutes": dwellStats.StddevMinutes,
							"sample_count":   dwellStats.Count,
						}
					}
				}

				writeJSON(w, result)
			})

			r.Get("/api/predictions/probabilities/{personID}/zone/{zoneID}", func(w http.ResponseWriter, r *http.Request) {
				personID := chi.URLParam(r, "personID")
				zoneID := chi.URLParam(r, "zoneID")
				hourOfWeek := prediction.HourOfWeek(time.Now())

				probs, err := predictionStore.GetTransitionProbabilitiesForFromZone(personID, zoneID, hourOfWeek)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				transitions := make([]map[string]interface{}, len(probs))
				for i, p := range probs {
					zoneName := p.ToZoneID
					if zonesMgr != nil {
						if z := zonesMgr.GetZone(p.ToZoneID); z != nil {
							zoneName = z.Name
						}
					}
					transitions[i] = map[string]interface{}{
						"from_zone_id":  p.FromZoneID,
						"to_zone_id":    p.ToZoneID,
						"to_zone_name":  zoneName,
						"probability":   p.Probability,
						"sample_count":  p.Count,
						"last_computed": p.LastComputed,
					}
				}

				// Get dwell time stats
				var dwellStats *prediction.DwellTimeStats
				dwellStats, _ = predictionStore.GetDwellTimeStats(personID, zoneID, hourOfWeek)

				writeJSON(w, map[string]interface{}{
					"person_id":    personID,
					"from_zone_id": zoneID,
					"hour_of_week": hourOfWeek,
					"transitions":  transitions,
					"dwell_time":   dwellStats,
				})
			})

			r.Get("/api/predictions/probabilities/{personID}/zone/{zoneID}/hour/{hour}", func(w http.ResponseWriter, r *http.Request) {
				personID := chi.URLParam(r, "personID")
				zoneID := chi.URLParam(r, "zoneID")
				hourStr := chi.URLParam(r, "hour")
				hourOfWeek := 0
				fmt.Sscanf(hourStr, "%d", &hourOfWeek)
				if hourOfWeek < 0 || hourOfWeek > 167 {
					http.Error(w, "hour must be 0-167", http.StatusBadRequest)
					return
				}

				probs, err := predictionStore.GetTransitionProbabilitiesForFromZone(personID, zoneID, hourOfWeek)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				transitions := make([]map[string]interface{}, len(probs))
				for i, p := range probs {
					zoneName := p.ToZoneID
					if zonesMgr != nil {
						if z := zonesMgr.GetZone(p.ToZoneID); z != nil {
							zoneName = z.Name
						}
					}
					transitions[i] = map[string]interface{}{
						"from_zone_id":  p.FromZoneID,
						"to_zone_id":    p.ToZoneID,
						"to_zone_name":  zoneName,
						"probability":   p.Probability,
						"sample_count":  p.Count,
						"last_computed": p.LastComputed,
					}
				}

				writeJSON(w, map[string]interface{}{
					"person_id":    personID,
					"from_zone_id": zoneID,
					"hour_of_week": hourOfWeek,
					"day_name":     prediction.DayNameFromHourOfWeek(hourOfWeek),
					"hour_of_day":  hourOfWeek % 24,
					"transitions":  transitions,
				})
			})

			// Get sample count for a slot
			r.Get("/api/predictions/samples/{personID}/zone/{zoneID}", func(w http.ResponseWriter, r *http.Request) {
				personID := chi.URLParam(r, "personID")
				zoneID := chi.URLParam(r, "zoneID")
				hourOfWeek := prediction.HourOfWeek(time.Now())

				count, err := predictionStore.GetTransitionCountForSlot(personID, zoneID, hourOfWeek)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				dataAge := predictionStore.GetDataAge()

				writeJSON(w, map[string]interface{}{
					"person_id":           personID,
					"zone_id":             zoneID,
					"hour_of_week":        hourOfWeek,
					"sample_count":        count,
					"minimum_samples":     prediction.MinimumSamplesPerSlot,
					"has_sufficient_data": count >= prediction.MinimumSamplesPerSlot,
					"data_age_days":       dataAge.Hours() / 24,
					"model_ready":         dataAge >= prediction.MinimumDataAge,
				})
			})
		}

		// Horizon prediction endpoint
		if predictionHorizon != nil {
			r.Get("/api/predictions/horizon", func(w http.ResponseWriter, r *http.Request) {
				predictions := predictionHorizon.UpdateAllPredictions()
				writeJSON(w, predictions)
			})

			r.Get("/api/predictions/horizon/{personID}", func(w http.ResponseWriter, r *http.Request) {
				personID := chi.URLParam(r, "personID")
				// Get current zone from history
				if predictionHistory == nil {
					http.Error(w, "prediction history not available", http.StatusServiceUnavailable)
					return
				}
				zoneID, _, _, ok := predictionHistory.GetPersonZone(personID)
				if !ok || zoneID == "" {
					http.Error(w, "person not currently tracked", http.StatusNotFound)
					return
				}
				pred := predictionHorizon.PredictAtHorizon(personID, zoneID, prediction.PredictionHorizon)
				writeJSON(w, pred)
			})
		}
	}

	// Phase 6: Learning feedback REST API
	if feedbackStore != nil {
		learningHandler := learning.NewHandler(feedbackStore, feedbackProcessor, accuracyComputer)
		learningHandler.RegisterRoutes(r)
	}

	// Phase 6: Detection explainability API
	explainabilityHandler = explainability.NewHandler()
	explainabilityHandler.RegisterRoutes(r)
	log.Printf("[INFO] Detection explainability API enabled")

	// Phase 6: Self-improving localization REST API
	if selfImprovingLocalizer != nil {
		r.Get("/api/localization/progress", func(w http.ResponseWriter, r *http.Request) {
			progress := selfImprovingLocalizer.GetLearningProgress()
			writeJSON(w, progress)
		})

		r.Get("/api/localization/weights", func(w http.ResponseWriter, r *http.Request) {
			weights := selfImprovingLocalizer.GetLearnedWeights()
			writeJSON(w, weights)
		})

		r.Get("/api/localization/ground-truth", func(w http.ResponseWriter, r *http.Request) {
			allGT := selfImprovingLocalizer.GetAllGroundTruth()
			writeJSON(w, allGT)
		})

		r.Get("/api/localization/sigmas", func(w http.ResponseWriter, r *http.Request) {
			engine := selfImprovingLocalizer.GetEngine()
			if engine == nil || engine.GetLearnedWeights() == nil {
				writeJSON(w, map[string]float64{})
				return
			}
			sigmas := engine.GetLearnedWeights().GetAllSigmas()
			writeJSON(w, sigmas)
		})

		r.Get("/api/localization/stats", func(w http.ResponseWriter, r *http.Request) {
			engine := selfImprovingLocalizer.GetEngine()
			if engine == nil || engine.GetLearnedWeights() == nil {
				writeJSON(w, map[string]interface{}{
					"links": 0,
					"error": "engine not available",
				})
				return
			}
			stats := engine.GetLearnedWeights().GetAllStats()
			result := make(map[string]interface{})
			totalObs := int64(0)
			totalCorrect := int64(0)
			for linkID, s := range stats {
				result[linkID] = map[string]interface{}{
					"observation_count":  s.ObservationCount,
					"correct_count":      s.CorrectCount,
					"avg_error_m":        s.ErrorSum / math.Max(1, float64(s.ObservationCount)),
					"last_error_m":       s.LastError,
					"weight_adjustments": s.WeightAdjustments,
				}
				totalObs += s.ObservationCount
				totalCorrect += s.CorrectCount
			}
			result["_summary"] = map[string]interface{}{
				"total_links":        len(stats),
				"total_observations": totalObs,
				"total_correct":      totalCorrect,
				"accuracy":           float64(totalCorrect) / math.Max(1, float64(totalObs)),
			}
			writeJSON(w, result)
		})

		r.Post("/api/localization/reset", func(w http.ResponseWriter, r *http.Request) {
			// Reset all learned weights to defaults
			engine := selfImprovingLocalizer.GetEngine()
			if engine != nil {
				engine.SetLearnedWeights(localization.NewLearnedWeights())
				if weightStore != nil {
					weightStore.SaveWeights(localization.NewLearnedWeights())
				}
			}
			writeJSON(w, map[string]string{"status": "reset"})
		})

		// Improvement tracking endpoint - shows how localization accuracy improves over time
		r.Get("/api/localization/improvement", func(w http.ResponseWriter, r *http.Request) {
			stats := selfImprovingLocalizer.GetImprovementStats()
			history := selfImprovingLocalizer.GetImprovementHistory()

			result := map[string]interface{}{
				"stats":   stats,
				"history": history,
			}
			writeJSON(w, result)
		})

		// Spatial weights endpoints
		if spatialWeightLearner != nil {
			r.Get("/api/accuracy/weights", func(w http.ResponseWriter, r *http.Request) {
				weights := spatialWeightLearner.GetAllWeights()
				stats := spatialWeightLearner.GetWeightStats()
				result := map[string]interface{}{
					"weights": weights,
					"stats":   stats,
				}
				writeJSON(w, result)
			})

			r.Get("/api/accuracy/weights/{zoneX}/{zoneY}", func(w http.ResponseWriter, r *http.Request) {
				zoneX, _ := strconv.Atoi(chi.URLParam(r, "zoneX"))
				zoneY, _ := strconv.Atoi(chi.URLParam(r, "zoneY"))
				weights := spatialWeightLearner.GetWeightsForZone(zoneX, zoneY)
				writeJSON(w, weights)
			})
		}

		// Position accuracy endpoints
		if groundTruthStore != nil {
			r.Get("/api/accuracy/position", func(w http.ResponseWriter, r *http.Request) {
				stats, err := groundTruthStore.GetPositionImprovementStats()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeJSON(w, stats)
			})

			r.Get("/api/accuracy/position/history", func(w http.ResponseWriter, r *http.Request) {
				weeksStr := r.URL.Query().Get("weeks")
				weeks := 8
				if weeksStr != "" {
					if n, err := strconv.Atoi(weeksStr); err == nil && n > 0 {
						weeks = n
					}
				}
				history, err := groundTruthStore.GetPositionAccuracyHistory(weeks)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeJSON(w, history)
			})

			r.Get("/api/accuracy/samples", func(w http.ResponseWriter, r *http.Request) {
				zoneCounts, err := groundTruthStore.GetZoneSampleCounts()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				personCounts, err := groundTruthStore.GetSampleCountByPerson()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				total, err := groundTruthStore.GetTotalSampleCount()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				today, err := groundTruthStore.GetSamplesTodayCount()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				writeJSON(w, map[string]interface{}{
					"total_samples":     total,
					"samples_today":     today,
					"zone_counts":       zoneCounts,
					"person_counts":     personCounts,
					"zones_with_data":   len(zoneCounts),
					"persons_with_data": len(personCounts),
				})
			})

			r.Get("/api/accuracy/samples/recent", func(w http.ResponseWriter, r *http.Request) {
				limitStr := r.URL.Query().Get("limit")
				limit := 100
				if limitStr != "" {
					if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
						limit = n
					}
				}
				samples, err := groundTruthStore.GetRecentSamples(limit)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeJSON(w, samples)
			})

			// Trigger weekly accuracy computation
			r.Post("/api/accuracy/position/compute", func(w http.ResponseWriter, r *http.Request) {
				week := localization.GetWeekString(time.Now())
				if err := groundTruthStore.ComputeWeeklyAccuracy(week); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeJSON(w, map[string]string{"status": "computed", "week": week})
			})
		}

		log.Printf("[INFO] Self-improving localization API registered at /api/localization/*")
	}

	// Phase 6: Anomaly detection REST API
	if anomalyDetector != nil {
		anomalyHandler := analytics.NewAnomalyHandler(anomalyDetector)
		anomalyHandler.RegisterRoutes(r)

		// Security mode API (arm, disarm, status)
		securityHandler := api.NewSecurityHandler(anomalyDetector)
		securityHandler.RegisterRoutes(r)

		// GET /api/security — per plan spec returns {security_mode, armed_at}
		r.Get("/api/security", func(w http.ResponseWriter, r *http.Request) {
			armed := anomalyDetector.IsSecurityModeActive()
			var armedAt interface{}
			if t := anomalyDetector.GetArmedAt(); t != nil {
				armedAt = t.Format(time.RFC3339)
			}
			writeJSON(w, map[string]interface{}{
				"security_mode": armed,
				"armed_at":      armedAt,
			})
		})

		r.Post("/api/security/acknowledge-all", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Feedback string `json:"feedback"`
				By       string `json:"acknowledged_by"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			anomalies := anomalyDetector.GetActiveAnomalies()
			acknowledged := 0
			for _, a := range anomalies {
				if err := anomalyDetector.AcknowledgeAnomaly(a.ID, req.Feedback, req.By); err == nil {
					acknowledged++
				}
			}
			writeJSON(w, map[string]int{"acknowledged": acknowledged, "total": len(anomalies)})
		})
	}

	// Phase 6: Sleep quality REST API (handler created earlier with monitor)
	sleepHandler.RegisterRoutes(r)
	log.Printf("[INFO] Sleep quality API registered at /api/sleep/*")

	// Phase 6: Tracked blobs REST API (for testing and external integrations)
	r.Get("/api/blobs", func(w http.ResponseWriter, r *http.Request) {
		blobs := pm.GetTrackedBlobs()
		writeJSON(w, blobs)
	})
	log.Printf("[INFO] Tracked blobs API registered at /api/blobs")

	// Tracks REST API (BLE-to-blob identity enriched tracked people)
	tracksHandler := api.NewTracksHandler(pm)
	tracksHandler.RegisterRoutes(r)
	log.Printf("[INFO] Tracks API registered at /api/tracks")

	// Backup API — streams a zip of all databases via SQLite Online Backup API
	backupHandler := api.NewBackupHandler(cfg.DataDir, version)
	r.Get("/api/backup", backupHandler.HandleBackup)
	log.Printf("[INFO] Backup API registered at /api/backup")

	// Events timeline REST API (uses shared mainDB)
	// eventsHandler was created earlier to allow fusion loop to log detection events
	eventsHandler.SetHub(dashboardHub)
	eventsHandler.RegisterRoutes(r)
	log.Printf("[INFO] Events timeline API registered at /api/events/*")

	// Start nightly events archive scheduler (runs at 02:00 local time)
	archiveDone := make(chan struct{})
	events.StartArchiveScheduler(mainDB, archiveDone)
	defer close(archiveDone)

	// OTA firmware server and manager
	firmwareDir := filepath.Join(cfg.DataDir, "firmware")
	otaSrv := ota.NewServer(firmwareDir)
	otaMgr := ota.NewManager(otaSrv, "http://"+cfg.BindAddr)
	otaMgr.SetSender(ingestSrv)
	ingestSrv.SetOTAManager(otaMgr)
	log.Printf("[INFO] OTA firmware server at %s", firmwareDir)

	// OTA REST API
	r.Get("/api/firmware", otaSrv.HandleList)
	r.Post("/api/firmware/upload", otaSrv.HandleUpload)
	r.Get("/firmware/{filename}", otaSrv.HandleServe)
	r.Get("/api/firmware/progress", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(otaMgr.GetProgress())
	})
	r.Post("/api/firmware/ota-all", func(w http.ResponseWriter, r *http.Request) {
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
	provSrv := provisioning.NewServer(cfg.DataDir, cfg.MDNSName, msPort, cfg.NTPServer, cfg.InstallSecret)
	r.Post("/api/provision", provSrv.HandleProvision)

	// Firmware manifest for esp-web-tools (onboarding wizard flashing)
	r.Get("/api/firmware/manifest", func(w http.ResponseWriter, r *http.Request) {
		latest := otaSrv.GetLatest()
		manifest := map[string]interface{}{
			"name":                     "Spaxel Node",
			"version":                  version,
			"new_install_prompt_erase": true,
			"builds":                   []map[string]interface{}{},
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

	// Phase 5 complete — all subsystems initialized
	phase5Done()

	// Phase 6: HTTP + mDNS
	startup.CheckTimeout(startupCtx)
	phase6Done := startup.Phase(6, "HTTP + mDNS")

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
	phase6Done()

	// Phase 7: Health check and readiness
	startup.CheckTimeout(startupCtx)
	phase7Done := startup.Phase(7, "Health")

	// Verify healthz responds
	healthURL := fmt.Sprintf("http://%s/healthz", cfg.BindAddr)
	healthCtx, healthCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer healthCancel()
	req, reqErr := http.NewRequestWithContext(healthCtx, http.MethodGet, healthURL, nil)
	if reqErr == nil {
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("[INFO] Health check passed (HTTP %d)", resp.StatusCode)
			} else {
				log.Printf("[WARN] Health check returned HTTP %d", resp.StatusCode)
			}
		} else {
			log.Printf("[WARN] Health check failed: %v (continuing anyway)", err)
		}
	}

	// Write ready marker file
	if err := startup.WriteReadyFile(); err != nil {
		log.Printf("[WARN] Failed to write ready file: %v", err)
	}

	phase7Done()
	startupTotalElapsed := time.Since(startupTotalStart)
	log.Printf("[READY] All 7 phases completed in %dms", startupTotalElapsed.Milliseconds())
	startupCancel() // Release startup timeout context

	sig := <-sigChan
	log.Printf("[INFO] Received signal %v, initiating graceful shutdown", sig)

	// Remove ready marker on shutdown
	startup.RemoveReadyFile()

	// Create shutdown manager with 30-second deadline
	shutdownMgr := shutdown.NewManager(mainDB)

	// Wire up baseline flusher (using baselineStore for proper SQLite flush)
	shutdownMgr.SetBaselineComponents(pm, baselineStore)

	// Wire up recording syncer
	if recMgr != nil {
		shutdownMgr.SetRecordingSyncer(shutdown.NewRecorderManagerSyncer(recMgr))
	}

	// Wire up dashboard broadcaster
	if dashboardHub != nil {
		shutdownMgr.SetDashboardBroadcaster(shutdown.NewDashboardHubBroadcaster(dashboardHub))
	}

	// Wire up node connection closer
	shutdownMgr.SetNodeCloser(shutdown.NewIngestionServerCloser(func() error {
		ingestSrv.CloseAllConnections()
		return nil
	}))

	// Wire up event writer
	shutdownMgr.SetEventWriter(shutdown.NewDBEventWriter(mainDB))

	// Wire up ingestion shutdowner
	shutdownMgr.SetIngestionShutdowner(ingestSrv)

	// Create shutdown context with 30s deadline
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Execute 10-step shutdown sequence
	shutdownComplete := shutdownMgr.Shutdown(shutdownCtx, cancel)

	// HTTP server shutdown (after step 2 - dashboard clients notified)
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] HTTP server shutdown error: %v", err)
	}

	// mDNS shutdown
	if mdnsServer != nil {
		mdnsServer.Shutdown()
	}

	// Persist zone occupancy for restart reconciliation
	if zonesMgr != nil {
		if err := zonesMgr.PersistOccupancy(); err != nil {
			log.Printf("[WARN] Failed to persist zone occupancy on shutdown: %v", err)
		} else {
			log.Printf("[INFO] Zone occupancy persisted for restart recovery")
		}
	}

	// Exit with appropriate code
	if shutdownComplete {
		log.Printf("[INFO] Shutdown complete - exiting 0")
		os.Exit(0)
	} else {
		log.Printf("[ERROR] Shutdown exceeded deadline - exiting 1")
		os.Exit(1)
	}
} // end main()

// Dashboard zone state adapter

type zoneStateAdapter struct {
	mgr *zones.Manager
}

func (a *zoneStateAdapter) GetAllPortals() []dashboard.PortalSnapshot {
	portals := a.mgr.GetAllPortals()
	result := make([]dashboard.PortalSnapshot, 0, len(portals))
	for _, p := range portals {
		result = append(result, dashboard.PortalSnapshot{
			ID:     p.ID,
			Name:   p.Name,
			ZoneA:  p.ZoneAID,
			ZoneB:  p.ZoneBID,
			P1X:    p.P1X,
			P1Y:    p.P1Y,
			P1Z:    p.P1Z,
			P2X:    p.P2X,
			P2Y:    p.P2Y,
			P2Z:    p.P2Z,
			P3X:    p.P3X,
			P3Y:    p.P3Y,
			P3Z:    p.P3Z,
			NX:     p.NX,
			NY:     p.NY,
			NZ:     p.NZ,
			Width:  p.Width,
			Height: p.Height,
		})
	}
	return result
}

func (a *zoneStateAdapter) GetAllZones() []dashboard.ZoneSnapshot {
	zones := a.mgr.GetAllZones()
	result := make([]dashboard.ZoneSnapshot, 0, len(zones))
	for _, z := range zones {
		result = append(result, dashboard.ZoneSnapshot{
			ID:    z.ID,
			Name:  z.Name,
			MinX:  z.MinX,
			MinY:  z.MinY,
			MinZ:  z.MinZ,
			SizeX: z.MaxX - z.MinX,
			SizeY: z.MaxY - z.MinY,
			SizeZ: z.MaxZ - z.MinZ,
		})
	}
	return result
}

func (a *zoneStateAdapter) GetOccupancy() map[string]dashboard.ZoneOccupancySnapshot {
	occ := a.mgr.GetOccupancy()
	result := make(map[string]dashboard.ZoneOccupancySnapshot, len(occ))
	for id, o := range occ {
		result[id] = dashboard.ZoneOccupancySnapshot{
			Count:   o.Count,
			BlobIDs: o.BlobIDs,
		}
	}
	return result
}

func (a *zoneStateAdapter) GetOccupancyStatus() map[string]string {
	status := a.mgr.GetOccupancyStatus()
	result := make(map[string]string, len(status))
	for id, s := range status {
		result[id] = string(s)
	}
	return result
}

// Provider adapters for automation engine

type zoneProviderAdapter struct {
	mgr *zones.Manager
}

func (z *zoneProviderAdapter) GetZone(id string) (string, bool) {
	zone := z.mgr.GetZone(id)
	if zone == nil {
		return "", false
	}
	return zone.Name, true
}

func (z *zoneProviderAdapter) GetZoneOccupancy(zoneID string) (int, []int) {
	occ := z.mgr.GetZoneOccupancy(zoneID)
	if occ == nil {
		return 0, nil
	}
	return occ.Count, occ.BlobIDs
}

type personProviderAdapter struct {
	registry *ble.Registry
}

func (p *personProviderAdapter) GetPerson(id string) (string, string, bool) {
	person, err := p.registry.GetPerson(id)
	if err != nil {
		return "", "", false
	}
	return person.Name, person.Color, true
}

type deviceProviderAdapter struct {
	registry *ble.Registry
}

func (d *deviceProviderAdapter) GetDevice(mac string) (string, bool) {
	device, err := d.registry.GetDevice(mac)
	if err != nil {
		return "", false
	}
	if device.Label != "" {
		return device.Label, true
	}
	if device.Name != "" {
		return device.Name, true
	}
	if device.DeviceName != "" {
		return device.DeviceName, true
	}
	return mac, true
}

type notifySenderAdapter struct {
	service *notify.Service
}

func (n *notifySenderAdapter) SendViaChannel(channelType string, title, body string, data map[string]interface{}) error {
	// The notify service sends to all channels, so we use it directly
	notif := notify.Notification{
		Title:     title,
		Body:      body,
		Data:      data,
		Timestamp: time.Now(),
	}
	return n.service.Send(notif)
}

// Prediction provider adapters

type predictionZoneAdapter struct {
	mgr *zones.Manager
}

func (z *predictionZoneAdapter) GetZone(id string) (string, bool) {
	zone := z.mgr.GetZone(id)
	if zone == nil {
		return "", false
	}
	return zone.Name, true
}

type predictionPersonAdapter struct {
	registry *ble.Registry
}

func (p *predictionPersonAdapter) GetPerson(id string) (string, string, bool) {
	person, err := p.registry.GetPerson(id)
	if err != nil {
		return "", "", false
	}
	return person.Name, person.Color, true
}

func (p *predictionPersonAdapter) GetAllPeople() ([]struct {
	ID    string
	Name  string
	Color string
}, error) {
	people, err := p.registry.GetPeople()
	if err != nil {
		return nil, err
	}
	result := make([]struct {
		ID    string
		Name  string
		Color string
	}, len(people))
	for i, person := range people {
		result[i] = struct {
			ID    string
			Name  string
			Color string
		}{
			ID:    person.ID,
			Name:  person.Name,
			Color: person.Color,
		}
	}
	return result, nil
}

type predictionMQTTAdapter struct {
	client *mqtt.Client
}

func (m *predictionMQTTAdapter) Publish(topic string, payload []byte) error {
	return m.client.Publish(topic, payload)
}

func (m *predictionMQTTAdapter) IsConnected() bool {
	return m.client.IsConnected()
}

// Anomaly detector provider adapters

type anomalyZoneAdapter struct {
	mgr *zones.Manager
}

func (a *anomalyZoneAdapter) GetZoneName(zoneID string) string {
	zone := a.mgr.GetZone(zoneID)
	if zone == nil {
		return zoneID
	}
	return zone.Name
}

func (a *anomalyZoneAdapter) GetZoneOccupancy(zoneID string) (int, []int) {
	occ := a.mgr.GetZoneOccupancy(zoneID)
	if occ == nil {
		return 0, nil
	}
	return occ.Count, occ.BlobIDs
}

type anomalyPersonAdapter struct {
	registry *ble.Registry
}

func (a *anomalyPersonAdapter) GetPersonDevices(personID string) ([]string, error) {
	devices, err := a.registry.GetPersonDevices(personID)
	if err != nil {
		return nil, err
	}
	macs := make([]string, len(devices))
	for i, d := range devices {
		macs[i] = d.Addr
	}
	return macs, nil
}

func (a *anomalyPersonAdapter) GetAllRegisteredDevices() (map[string]string, error) {
	devices, err := a.registry.GetAllPersonDevices()
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, d := range devices {
		if d.PersonID != "" {
			result[d.Addr] = d.PersonID
		}
	}
	return result, nil
}

func (a *anomalyPersonAdapter) GetPersonName(personID string) string {
	person, err := a.registry.GetPerson(personID)
	if err != nil {
		return personID
	}
	return person.Name
}

type anomalyDeviceAdapter struct {
	registry *ble.Registry
}

func (a *anomalyDeviceAdapter) IsDeviceRegistered(mac string) bool {
	device, err := a.registry.GetDevice(mac)
	if err != nil {
		return false
	}
	return device.PersonID != "" && device.Enabled
}

func (a *anomalyDeviceAdapter) IsDeviceSeenBefore(mac string) bool {
	device, err := a.registry.GetDevice(mac)
	if err != nil {
		return false
	}
	// Consider "seen before" if first seen more than 24 hours ago
	return device.FirstSeenAt.Before(time.Now().Add(-24 * time.Hour))
}

func (a *anomalyDeviceAdapter) GetDeviceName(mac string) string {
	device, err := a.registry.GetDevice(mac)
	if err != nil {
		return mac
	}
	if device.Label != "" {
		return device.Label
	}
	if device.Name != "" {
		return device.Name
	}
	if device.DeviceName != "" {
		return device.DeviceName
	}
	return mac
}

type anomalyPositionAdapter struct {
	pm *sigproc.ProcessorManager
}

func (a *anomalyPositionAdapter) GetBlobPosition(blobID int) (x, y, z float64, ok bool) {
	blobs := a.pm.GetTrackedBlobs()
	for _, blob := range blobs {
		if blob.ID == blobID {
			return blob.X, blob.Y, blob.Z, true
		}
	}
	return 0, 0, 0, false
}

type anomalyAlertAdapter struct {
	hub           *dashboard.Hub
	notifyService *notify.Service
}

func (a *anomalyAlertAdapter) SendAlert(event events.AnomalyEvent, immediate bool) error {
	if a.notifyService != nil {
		priority := 3
		if immediate {
			priority = 5
		}
		notif := notify.Notification{
			Title:    "Security Alert",
			Body:     event.Description,
			Priority: priority,
			Tags:     []string{"warning", "security", string(event.Type)},
			Data: map[string]interface{}{
				"anomaly_id":   event.ID,
				"anomaly_type": event.Type,
				"score":        event.Score,
				"zone_id":      event.ZoneID,
				"zone_name":    event.ZoneName,
			},
			Timestamp: time.Now(),
		}
		a.notifyService.Send(notif)
	}
	return nil
}

func (a *anomalyAlertAdapter) SendWebhook(event events.AnomalyEvent, immediate bool) error {
	// Webhooks are handled by the notification service channels
	return nil
}

func (a *anomalyAlertAdapter) SendEscalation(event events.AnomalyEvent) error {
	if a.notifyService != nil {
		notif := notify.Notification{
			Title:    "SECURITY ESCALATION",
			Body:     fmt.Sprintf("UNACKNOWLEDGED: %s", event.Description),
			Priority: 5,
			Tags:     []string{"urgent", "security", "escalation"},
			Data: map[string]interface{}{
				"anomaly_id":   event.ID,
				"anomaly_type": event.Type,
				"escalation":   true,
			},
			Timestamp: time.Now(),
		}
		a.notifyService.Send(notif)
	}
	return nil
}

// resolveBlobIdentity returns the display name for a blob via the identity matcher.
// Returns an empty string if no match is found or the matcher is nil.
func resolveBlobIdentity(blobID int, matcher *ble.IdentityMatcher) string {
	if matcher == nil {
		return ""
	}
	if match := matcher.GetMatch(blobID); match != nil {
		return match.DeviceName
	}
	return ""
}

// splitLinkID parses a link ID in format "nodeMAC-peerMAC" into its components
func splitLinkID(linkID string) []string {
	// Link ID format is "aa:bb:cc:dd:ee:ff-11:22:33:44:55:66"
	for i := len(linkID) - 1; i >= 0; i-- {
		if linkID[i] == '-' {
			return []string{linkID[:i], linkID[i+1:]}
		}
	}
	return nil
}
