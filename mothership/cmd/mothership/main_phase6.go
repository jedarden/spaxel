// Package main provides the mothership entry point
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
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/hashicorp/mdns"
	"github.com/spaxel/mothership/internal/analytics"
	"github.com/spaxel/mothership/internal/automation"
	"github.com/spaxel/mothership/internal/ble"
	"github.com/spaxel/mothership/internal/dashboard"
	"github.com/spaxel/mothership/internal/diagnostics"
	"github.com/spaxel/mothership/internal/falldetect"
	"github.com/spaxel/mothership/internal/fleet"
	"github.com/spaxel/mothership/internal/ingestion"
	"github.com/spaxel/mothership/internal/learning"
	"github.com/spaxel/mothership/internal/mqtt"
	"github.com/spaxel/mothership/internal/notify"
	"github.com/spaxel/mothership/internal/ota"
	"github.com/spaxel/mothership/internal/prediction"
	"github.com/spaxel/mothership/internal/provisioning"
	"github.com/spaxel/mothership/internal/recorder"
	"github.com/spaxel/mothership/internal/replay"
	"github.com/spaxel/mothership/internal/zones"
	sigproc "github.com/spaxel/mothership/internal/signal"
)

// Phase 5: Configuration constants
const (
	baselineSaveInterval  = 30 * time.Second
	healthComputeInterval = 5 * time.Second
	 weatherRecordInterval = 60 * time.Second
)

// Build-time version injection
var version = "dev"

// Config holds application configuration
type Config struct {
	BindAddr     string
	DataDir      string
	StaticDir    string
	MDNSName     string
	MDNSEnabled  bool
	LogLevel     string
	ReplayMaxMB  int

	// MQTT configuration
	MQTTBroker   string
	MQTTClientID string
	MQTTUsername string
	MQTTPassword string
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

    // Fleet node registry
    fleetReg, err := fleet.NewRegistry(filepath.Join(cfg.DataDir, "fleet.db"))
    if err != nil {
        log.Fatalf("[FATAL] Failed to open fleet registry: %v", err)
    }
    defer fleetReg.Close()
    log.Printf("[INFO] Fleet registry at %s", filepath.Join(cfg.DataDir, "fleet.db"))

    // Phase 6: BLE device registry
    bleRegistry, err := ble.NewRegistry(filepath.Join(cfg.DataDir, "ble.db"))
    if err != nil {
        log.Printf("[WARN] Failed to open BLE registry: %v", err)
    } else {
        defer bleRegistry.Close()
        log.Printf("[INFO] BLE registry at %s", filepath.Join(cfg.DataDir, "ble.db"))
    }

    // Phase 6: RSSI cache for BLE triangulation
    rssiCache := ble.NewRSSICache(10 * time.Second)

    // Phase 6: BLE identity matcher
    var identityMatcher *ble.IdentityMatcher
    if bleRegistry != nil {
        identityMatcher = ble.NewIdentityMatcher(bleRegistry, rssiCache, fleetReg)
    }

    // Phase 6: Zones manager
    zonesMgr, err := zones.NewManager(filepath.Join(cfg.DataDir, "zones.db"))
    if err != nil {
        log.Printf("[WARN] Failed to open zones database: %v", err)
    } else {
        defer zonesMgr.Close()
        log.Printf("[INFO] Zones manager at %s", filepath.Join(cfg.DataDir, "zones.db"))
    }

    // Phase 6: Flow analytics accumulator
    flowAccumulator, err := analytics.NewFlowAccumulator(filepath.Join(cfg.DataDir, "analytics.db"))
    if err != nil {
        log.Printf("[WARN] Failed to open analytics database: %v", err)
    } else {
        defer flowAccumulator.Close()
        log.Printf("[INFO] Flow analytics at %s", filepath.Join(cfg.DataDir, "analytics.db"))
    }

    // Phase 6: Automation engine
    automationEngine, err := automation.NewEngine(filepath.Join(cfg.DataDir, "automation.db"))
    if err != nil {
        log.Printf("[WARN] Failed to open automation database: %v", err)
    } else {
        defer automationEngine.Close()
        log.Printf("[INFO] Automation engine at %s", filepath.Join(cfg.DataDir, "automation.db"))
    }

    // Phase 6: Fall detector
    fallDetector := falldetect.NewDetector()
    log.Printf("[INFO] Fall detector initialized")

    // Phase 6: Prediction module for presence prediction
    var predictionStore *prediction.ModelStore
    var predictionHistory *prediction.HistoryUpdater
    var predictionPredictor *prediction.Predictor
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

        // Create predictor
        predictionPredictor = prediction.NewPredictor(predictionStore)

        log.Printf("[INFO] Presence prediction initialized")
    }

    // Phase 6: Notification service
    notifyService, err := notify.NewService(filepath.Join(cfg.DataDir, "notify.db"))
    if err != nil {
        log.Printf("[WARN] Failed to open notification database: %v", err)
    } else {
        defer notifyService.Close()
        log.Printf("[INFO] Notification service at %s", filepath.Join(cfg.DataDir, "notify.db"))

        // Set room config provider for floor plan thumbnails
        notifyService.SetRoomConfig(fleetReg)
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
            ClientID:         cfg.MQTTClientID,
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

    // Phase 6: Wire BLE messages to registry and identity matcher
    ingestSrv.SetBLEHandler(func(nodeMAC string, devices []ingestion.BLEDevice) {
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
    diagnosticEngine.SetGDOPImprovementAccessor(func(nodeMAC string, targetPos diagnostics.Vec3) float64) {
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
    go func() {
        ticker := time.NewTicker(100 * time.Millisecond) // 10 Hz
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                // Get tracked blobs from fusion/tracker
                blobs := pm.GetTrackedBlobs()
                if len(blobs) == 0 {
                    continue
                }

                // Update identity matcher
                if identityMatcher != nil {
                    identityMatcher.UpdateBlobs(blobs)
                }

                // Update zones occupancy
                if zonesMgr != nil {
                    for _, blob := range blobs {
                        zonesMgr.UpdateBlobPosition(blob.ID, blob.X, blob.Y, blob.Z)
                    }
                }

                // Update flow analytics
                if flowAccumulator != nil {
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

                // Run fall detection
                for _, blob := range blobs {
                    fallDetector.Update([]struct {
                        ID      int
                        X, Y, Z  float64
                        VX, VY, VZ float64
                        Posture string
                    }{blob}, time.Now())
                }

                // Evaluate automations
                if automationEngine != nil {
                    automationEngine.Evaluate(blobs, func(blobID int) string {
                        if zonesMgr != nil {
                            return zonesMgr.GetBlobZone(blobID)
                        }
                        return ""
                    })
                }
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
                    "blob_id":   event.BlobID,
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
                    "blob_id":   event.BlobID,
                    "position":  []float64{event.Position.X, event.Position.Y, event.Position.Z},
                },
            })
        }
    })

    // Set identity function for fall detector
    fallDetector.SetIdentityFunc(func(blobID int) string {
        if identityMatcher == nil {
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
                    Priority:  1,
                Tags:     []string{"zone", "movement"},
                Data: map[string]interface{}{
                    "portal_id":  event.PortalID,
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
        })
    }

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
                    }
                }
            }
        }()
        log.Printf("[INFO] Prediction update loop started (interval: 60s)")
    }

    // Fleet REST API
    fleetHandler := fleet.NewHandler(fleetMgr)
    fleetHandler.RegisterRoutes(r)

    // Phase 6: BLE REST API
    if bleRegistry != nil {
        r.Get("/api/ble/devices", func(w http.ResponseWriter, r *http.Request) {
            devices := bleRegistry.GetAllDevices()
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
            if err := bleRegistry.UpsertDevice(&device); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            writeJSON(w, device)
        })
        r.Put("/api/ble/devices/{addr}", func(w http.ResponseWriter, r *http.Request) {
            addr := chi.URLParam(r, "addr")
            var device ble.DeviceRecord
            if err := json.NewDecoder(r.Body).Decode(&device); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
            }
            device.Addr = addr
            if err := bleRegistry.UpsertDevice(&device); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            writeJSON(w, device)
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
                matches := identityMatcher.GetMatches()
                writeJSON(w, matches)
                return
            }
            writeJSON(w, map[int]*ble.IdentityMatch{})
        })
    }

    // Phase 6: Zones REST API
    if zonesMgr != nil {
        r.Get("/api/zones", func(w http.ResponseWriter, r *http.Request) {
            zones := zonesMgr.GetAllZones()
            writeJSON(w, zones)
        })
        r.Post("/api/zones", func(w http.ResponseWriter, r *http.Request) {
            var zone zones.Zone
            if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
            }
            if zone.ID == "" {
                zone.ID = fmt.Sprintf("zone_%d", time.Now().UnixNano())
            }
            if err := zonesMgr.CreateZone(&zone); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            writeJSON(w, zone)
        })
        r.Put("/api/zones/{id}", func(w http.ResponseWriter, r *http.Request) {
            id := chi.URLParam(r, "id")
            var zone zones.Zone
            if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
            }
            zone.ID = id
            if err := zonesMgr.UpdateZone(&zone); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            writeJSON(w, zone)
        })
        r.Delete("/api/zones/{id}", func(w http.ResponseWriter, r *http.Request) {
            id := chi.URLParam(r, "id")
            if err := zonesMgr.DeleteZone(id); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            w.WriteHeader(http.StatusNoContent)
        })
        r.Get("/api/zones/occupancy", func(w http.ResponseWriter, r *http.Request) {
            occupancy := zonesMgr.GetOccupancy()
            writeJSON(w, occupancy)
        })
        r.Get("/api/zones/crossings", func(w http.ResponseWriter, r *http.Request) {
            crossings := zonesMgr.GetRecentCrossings(20)
            writeJSON(w, crossings)
        })
    }

    // Phase 6: Portals REST API
    r.Get("/api/portals", func(w http.ResponseWriter, r *http.Request) {
            if zonesMgr == nil {
                writeJSON(w, zonesMgr.GetAllPortals())
                return
            }
            writeJSON(w, []*zones.Portal{})
        })
        r.Post("/api/portals", func(w http.ResponseWriter, r *http.Request) {
            if zonesMgr == nil {
                http.Error(w, "zones manager not available", http.StatusServiceUnavailable)
                return
            }
            var portal zones.Portal
            if err := json.NewDecoder(r.Body).Decode(&portal); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
            }
            if portal.ID == "" {
                portal.ID = fmt.Sprintf("portal_%d", time.Now().UnixNano())
            }
            if err := zonesMgr.CreatePortal(&portal); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            writeJSON(w, portal)
        })
        r.Put("/api/portals/{id}", func(w http.ResponseWriter, r *http.Request) {
            id := chi.URLParam(r, "id")
            if zonesMgr == nil {
                http.Error(w, "zones manager not available", http.StatusServiceUnavailable)
                return
            }
            var portal zones.Portal
            if err := json.NewDecoder(r.Body).Decode(&portal); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
            }
            portal.ID = id
            if err := zonesMgr.UpdatePortal(&portal); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            writeJSON(w, portal)
        })
        r.Delete("/api/portals/{id}", func(w http.ResponseWriter, r *http.Request) {
            id := chi.URLParam(r, "id")
            if zonesMgr == nil {
                http.Error(w, "zones manager not available", http.StatusServiceUnavailable)
                return
            }
            if err := zonesMgr.DeletePortal(id); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            w.WriteHeader(http.StatusNoContent)
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
                Token:   req.Token,
                User:    req.User,
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
            "suggested_position":     map[string]float64{"x": x, "z": z},
            "expected_improvement":   improvement,
            "worst_coverage_zone":    map[string]float64{"x": worstX, "z": worstZ, "gdop": worstGDOP},
        })
    })

    // Phase 5: System health API
    r.Get("/api/health/system", func(w http.ResponseWriter, r *http.Request) {
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
    }

    // Phase 6: Learning feedback REST API
    if feedbackStore != nil {
        learningHandler := learning.NewHandler(feedbackStore, feedbackProcessor, accuracyComputer)
        learningHandler.RegisterRoutes(r)
    }

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
    provSrv := provisioning.NewServer(cfg.DataDir, cfg.MDNSName, msPort)
    r.Post("/api/provision", provSrv.HandleProvision)

    // Firmware manifest for esp-web-tools (onboarding wizard flashing)
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
        }{ID: person.ID, Name: person.Name, Color: person.Color}
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

