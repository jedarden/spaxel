// Package config provides environment variable validation and documented defaults
// for the Spaxel mothership. It validates all configuration at startup with
// type checking, range validation, and clear error messages.
package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all validated application configuration.
type Config struct {
	// Network
	BindAddr string // HTTP bind address (default "0.0.0.0:8080")

	// Paths
	DataDir         string // Persistent data directory (default "/data")
	StaticDir       string // Dashboard static files directory (default "/dashboard")
	SeedFirmwareDir string // Read-only dir with baked-in firmware binaries (default "/firmware")

	// mDNS
	MDNSName    string // mDNS service name (default "spaxel")
	MDNSEnabled bool   // Enable mDNS advertisement (default true)

	// Logging
	LogLevel string // Log level: debug|info|warn|error (default "info")

	// Processing
	FusionRateHz int // Fusion loop rate in Hz, range [1,20] (default 10)

	// Replay buffer
	ReplayMaxMB int // Maximum replay buffer size in MB, range [10,10000] (default 360)

	// Security
	InstallSecret        string // Installation secret (64-char hex, optional if set must be 32+ bytes)
	MigrationWindowHours int    // How long after startup nodes without tokens are tolerated (default 24, 0 = disabled)

	// Time
	NTPServer string // NTP server hostname (default "pool.ntp.org")
	Timezone  string // IANA timezone name (default "UTC")

	// MQTT (optional)
	MQTTBroker   string // MQTT broker URL (optional, must be valid URL if set)
	MQTTUsername string // MQTT broker username (optional)
	MQTTPassword string // MQTT broker password (optional, never logged)
}

// Load reads all environment variables, validates them, and returns a Config.
// All validation errors are collected and returned together.
func Load() (*Config, error) {
	var errs []error
	cfg := &Config{}

	// SPAXEL_BIND_ADDR - string, default '0.0.0.0:8080'
	cfg.BindAddr = envOr("SPAXEL_BIND_ADDR", "0.0.0.0:8080")

	// SPAXEL_DATA_DIR - string, default '/data'
	cfg.DataDir = envOr("SPAXEL_DATA_DIR", "/data")

	// SPAXEL_STATIC_DIR - string, default '/dashboard'
	cfg.StaticDir = envOr("SPAXEL_STATIC_DIR", "/dashboard")

	// SPAXEL_SEED_FIRMWARE_DIR - string, default '/firmware'
	// Directory containing baked-in firmware binaries copied from the image at startup.
	cfg.SeedFirmwareDir = envOr("SPAXEL_SEED_FIRMWARE_DIR", "/firmware")

	// SPAXEL_MDNS_ENABLED - bool, default true
	mdnsEnabled := envOr("SPAXEL_MDNS_ENABLED", "true")
	if mdnsEnabled == "true" || mdnsEnabled == "1" {
		cfg.MDNSEnabled = true
	} else if mdnsEnabled == "false" || mdnsEnabled == "0" {
		cfg.MDNSEnabled = false
	} else {
		errs = append(errs, fmt.Errorf("SPAXEL_MDNS_ENABLED=%s invalid: must be one of true, false, 1, 0", mdnsEnabled))
	}

	// SPAXEL_MDNS_NAME - string, default 'spaxel'
	cfg.MDNSName = envOr("SPAXEL_MDNS_NAME", "spaxel")

	// SPAXEL_LOG_LEVEL - enum, default 'info' (debug|info|warn|error)
	cfg.LogLevel = envOr("SPAXEL_LOG_LEVEL", "info")
	if !isValidLogLevel(cfg.LogLevel) {
		errs = append(errs, fmt.Errorf("SPAXEL_LOG_LEVEL=%s invalid: must be one of debug, info, warn, error", cfg.LogLevel))
	}

	// SPAXEL_FUSION_RATE_HZ - int, default 10, range [1,20]
	fusionRateStr := os.Getenv("SPAXEL_FUSION_RATE_HZ")
	if fusionRateStr == "" {
		cfg.FusionRateHz = 10
	} else {
		val, err := strconv.Atoi(fusionRateStr)
		if err != nil {
			errs = append(errs, fmt.Errorf("SPAXEL_FUSION_RATE_HZ=%s invalid: must be an integer", fusionRateStr))
		} else if val < 1 || val > 20 {
			errs = append(errs, fmt.Errorf("SPAXEL_FUSION_RATE_HZ=%d invalid: must be in range [1,20]", val))
		} else {
			cfg.FusionRateHz = val
		}
	}

	// SPAXEL_REPLAY_MAX_MB - int, default 360, range [10,10000]
	replayMaxStr := os.Getenv("SPAXEL_REPLAY_MAX_MB")
	if replayMaxStr == "" {
		cfg.ReplayMaxMB = 360
	} else {
		val, err := strconv.Atoi(replayMaxStr)
		if err != nil {
			errs = append(errs, fmt.Errorf("SPAXEL_REPLAY_MAX_MB=%s invalid: must be an integer", replayMaxStr))
		} else if val < 10 || val > 10000 {
			errs = append(errs, fmt.Errorf("SPAXEL_REPLAY_MAX_MB=%d invalid: must be in range [10,10000]", val))
		} else {
			cfg.ReplayMaxMB = val
		}
	}

	// SPAXEL_INSTALL_SECRET - string, optional (32+ chars if set)
	installSecret := os.Getenv("SPAXEL_INSTALL_SECRET")
	if installSecret != "" {
		// Validate hex encoding
		decoded, err := hex.DecodeString(installSecret)
		if err != nil {
			errs = append(errs, fmt.Errorf("SPAXEL_INSTALL_SECRET invalid: must be a hex string"))
		} else if len(decoded) < 32 {
			errs = append(errs, fmt.Errorf("SPAXEL_INSTALL_SECRET invalid: must be at least 32 bytes (64 hex chars)"))
		} else {
			cfg.InstallSecret = installSecret
		}
	}

	// SPAXEL_MIGRATION_WINDOW_HOURS - int, default 24, range [0,168]
	// 0 = disabled (strict token enforcement from startup)
	cfg.MigrationWindowHours = 24
	if mwStr := os.Getenv("SPAXEL_MIGRATION_WINDOW_HOURS"); mwStr != "" {
		if val, err := strconv.Atoi(mwStr); err != nil {
			errs = append(errs, fmt.Errorf("SPAXEL_MIGRATION_WINDOW_HOURS=%s invalid: must be an integer", mwStr))
		} else if val < 0 || val > 168 {
			errs = append(errs, fmt.Errorf("SPAXEL_MIGRATION_WINDOW_HOURS=%d invalid: must be in range [0,168]", val))
		} else {
			cfg.MigrationWindowHours = val
		}
	}

	// SPAXEL_NTP_SERVER - string, default 'pool.ntp.org'
	cfg.NTPServer = envOr("SPAXEL_NTP_SERVER", "pool.ntp.org")

	// SPAXEL_MQTT_BROKER - string, optional (must be valid URL if set)
	mqttBroker := os.Getenv("SPAXEL_MQTT_BROKER")
	if mqttBroker != "" {
		u, err := url.Parse(mqttBroker)
		if err != nil || u.Scheme == "" || u.Scheme == "not-a-url" {
			errs = append(errs, fmt.Errorf("SPAXEL_MQTT_BROKER=%s invalid: must be a valid URL with scheme (e.g., mqtt:// or mqtts://)", mqttBroker))
		} else if u.Scheme != "mqtt" && u.Scheme != "mqtts" {
			errs = append(errs, fmt.Errorf("SPAXEL_MQTT_BROKER=%s invalid: URL scheme must be mqtt:// or mqtts://", mqttBroker))
		} else {
			cfg.MQTTBroker = mqttBroker
		}
	}

	// SPAXEL_MQTT_USERNAME - string, optional
	cfg.MQTTUsername = envOr("SPAXEL_MQTT_USERNAME", "")

	// SPAXEL_MQTT_PASSWORD - string, optional (sensitive - never logged)
	cfg.MQTTPassword = envOr("SPAXEL_MQTT_PASSWORD", "")

	// TZ - string, default 'UTC'
	tz := os.Getenv("TZ")
	if tz == "" {
		tz = "UTC"
	}
	// Validate timezone by attempting to load it
	if _, err := time.LoadLocation(tz); err != nil {
		errs = append(errs, fmt.Errorf("TZ=%s invalid: %w", tz, err))
	} else {
		cfg.Timezone = tz
	}

	// If any errors occurred, return them all
	if len(errs) > 0 {
		return nil, joinErrors(errs)
	}

	// Log all non-sensitive loaded values at INFO
	logConfig(cfg)

	return cfg, nil
}

// envOr returns the environment variable value or the fallback if empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// isValidLogLevel checks if the log level is valid.
func isValidLogLevel(level string) bool {
	switch strings.ToLower(level) {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

// logConfig logs all non-sensitive configuration values at INFO level.
func logConfig(cfg *Config) {
	log.Printf("[CONFIG] SPAXEL_BIND_ADDR=%s", cfg.BindAddr)
	log.Printf("[CONFIG] SPAXEL_DATA_DIR=%s", cfg.DataDir)
	log.Printf("[CONFIG] SPAXEL_STATIC_DIR=%s", cfg.StaticDir)
	log.Printf("[CONFIG] SPAXEL_SEED_FIRMWARE_DIR=%s", cfg.SeedFirmwareDir)
	log.Printf("[CONFIG] SPAXEL_MDNS_ENABLED=%t", cfg.MDNSEnabled)
	log.Printf("[CONFIG] SPAXEL_MDNS_NAME=%s", cfg.MDNSName)
	log.Printf("[CONFIG] SPAXEL_LOG_LEVEL=%s", cfg.LogLevel)
	log.Printf("[CONFIG] SPAXEL_FUSION_RATE_HZ=%d", cfg.FusionRateHz)
	log.Printf("[CONFIG] SPAXEL_REPLAY_MAX_MB=%d", cfg.ReplayMaxMB)
	if cfg.InstallSecret != "" {
		log.Printf("[CONFIG] SPAXEL_INSTALL_SECRET=%s... (truncated)", cfg.InstallSecret[:16])
	} else {
		log.Printf("[CONFIG] SPAXEL_INSTALL_SECRET=(not set, will auto-generate)")
	}
	log.Printf("[CONFIG] SPAXEL_NTP_SERVER=%s", cfg.NTPServer)
	if cfg.MQTTBroker != "" {
		log.Printf("[CONFIG] SPAXEL_MQTT_BROKER=%s", cfg.MQTTBroker)
		log.Printf("[CONFIG] SPAXEL_MQTT_USERNAME=%s", cfg.MQTTUsername)
		log.Printf("[CONFIG] SPAXEL_MQTT_PASSWORD=***")
	}
	log.Printf("[CONFIG] TZ=%s", cfg.Timezone)
}

// joinErrors combines multiple errors into a single error.
func joinErrors(errs []error) error {
	var msg []string
	for _, err := range errs {
		msg = append(msg, err.Error())
	}
	return errors.New(strings.Join(msg, "\n"))
}

// FusionRate returns the fusion rate as a float64 for use in signal processing.
func (c *Config) FusionRate() float64 {
	return float64(c.FusionRateHz)
}

// ReplayMaxBytes returns the replay max size in bytes.
func (c *Config) ReplayMaxBytes() int64 {
	return int64(c.ReplayMaxMB) * 1024 * 1024
}

// TimezoneLocation returns the loaded time.Location for the configured timezone.
func (c *Config) TimezoneLocation() *time.Location {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}
