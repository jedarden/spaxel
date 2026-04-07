// Package startup provides phase-sequenced initialization with timeout enforcement
// for the Spaxel mothership. It ensures the mothership fails fast and clearly
// on misconfiguration by wrapping all startup phases in a 30-second deadline.
package startup

import (
	"context"
	"log"
	"os"
	"time"
)

const (
	// TotalPhases is the number of startup phases.
	TotalPhases = 7

	// TotalTimeout is the maximum time for all startup phases.
	TotalTimeout = 30 * time.Second

	// SubsystemTimeout is the maximum time for each subsystem start in Phase 5.
	SubsystemTimeout = 5 * time.Second

	// ReadyFile is written on successful startup (Phase 7).
	ReadyFile = "/tmp/spaxel.ready"
)

// Phase logs the start of a startup phase and returns a function that logs
// completion with elapsed time. The returned function should be called via
// defer or after the phase work completes.
//
// Usage:
//
//	done := startup.Phase(1, "Data directory")
//	err := doWork()
//	done()
func Phase(num int, description string) func() {
	log.Printf("[PHASE %d/%d — %s]", num, TotalPhases, description)
	start := time.Now()
	return func() {
		log.Printf("[PHASE %d/%d OK] (%dms)", num, TotalPhases, time.Since(start).Milliseconds())
	}
}

// CheckTimeout checks if the startup context has exceeded its deadline.
// If so, it logs a fatal message and exits. This should be called before
// each phase to enforce the 30-second total startup timeout.
func CheckTimeout(ctx context.Context) {
	if ctx.Err() != nil {
		log.Fatalf("[STARTUP TIMEOUT] Failed to reach ready state in 30s")
	}
}

// SubsystemStart runs a subsystem initialization function with a 5-second
// timeout. It logs the subsystem name, elapsed time, and any error.
func SubsystemStart(ctx context.Context, name string, fn func(context.Context) error) error {
	subCtx, cancel := context.WithTimeout(ctx, SubsystemTimeout)
	defer cancel()

	start := time.Now()
	err := fn(subCtx)
	elapsed := time.Since(start)

	if err != nil {
		log.Printf("[PHASE 5/%d] Subsystem %s failed after %dms: %v", TotalPhases, name, elapsed.Milliseconds(), err)
		return err
	}
	log.Printf("[PHASE 5/%d] Subsystem %s started (%dms)", TotalPhases, name, elapsed.Milliseconds())
	return nil
}

// WriteReadyFile writes the ready marker file at /tmp/spaxel.ready.
// This is called on successful Phase 7 completion.
func WriteReadyFile() error {
	return os.WriteFile(ReadyFile, []byte("ready"), 0644)
}

// RemoveReadyFile removes the ready marker file.
// This is called on shutdown.
func RemoveReadyFile() {
	os.Remove(ReadyFile)
}
