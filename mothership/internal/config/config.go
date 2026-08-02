// Package config provides environment variable validation and documented defaults
// for the Spaxel mothership. It validates all configuration at startup with
// type checking, range validation, and clear error messages.
package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
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

	// AdvertisedBaseURL is the base URL handed to nodes for fetching firmware
	// (e.g. "http://192.168.1.10:8080"). It MUST be routable from the nodes.
	//
	// This is deliberately distinct from BindAddr: binding to 0.0.0.0 is the
	// correct way to listen on every interface, but 0.0.0.0 is a wildcard bind
	// address and never a valid destination. Deriving the OTA URL from BindAddr
	// handed nodes "http://0.0.0.0:8080/firmware/..." and made OTA impossible in
	// every default deployment. See ADR-004.
	AdvertisedBaseURL string

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
	NTPServer string // NTP server hostname (default "pool.ntp.org", or this host's own address when NTPLocalEnabled)
	Timezone  string // IANA timezone name (default "UTC")

	// NTPLocalEnabled starts an embedded SNTP responder (UDP 123) so nodes on
	// an internet-isolated network still get wall-clock time. Requires
	// CAP_NET_BIND_SERVICE (or root) to bind the privileged port; a bind
	// failure is logged as a non-fatal warning, not a startup failure.
	NTPLocalEnabled bool // default false

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

	// SPAXEL_ADVERTISED_BASE_URL - string, default derived from BindAddr.
	// The base URL nodes are given to fetch firmware. Must be routable FROM the
	// nodes, which a 0.0.0.0 bind address never is. See ADR-004.
	if advertised := os.Getenv("SPAXEL_ADVERTISED_BASE_URL"); advertised != "" {
		if err := validateAdvertisedBaseURL(advertised); err != nil {
			errs = append(errs, err)
		} else {
			cfg.AdvertisedBaseURL = strings.TrimRight(advertised, "/")
		}
	} else {
		// Auto-derivation failing is NOT fatal. The mothership does far more than
		// serve firmware, and refusing to start — taking down CSI ingestion, the
		// dashboard and everything else — because the OTA URL is ambiguous would
		// be wildly disproportionate. Instead leave it empty and warn loudly; the
		// OTA path refuses to send and reports why. An explicitly-set-but-invalid
		// value above IS fatal, because that is an operator typo, not ambiguity.
		derived, err := deriveAdvertisedBaseURL(cfg.BindAddr)
		if err != nil {
			log.Printf("[WARN] OTA disabled: %v", err)
		} else {
			cfg.AdvertisedBaseURL = derived
		}
	}

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

	// SPAXEL_NTP_LOCAL_ENABLED - bool, default false
	ntpLocalEnabled := envOr("SPAXEL_NTP_LOCAL_ENABLED", "false")
	if ntpLocalEnabled == "true" || ntpLocalEnabled == "1" {
		cfg.NTPLocalEnabled = true
	} else if ntpLocalEnabled == "false" || ntpLocalEnabled == "0" {
		cfg.NTPLocalEnabled = false
	} else {
		errs = append(errs, fmt.Errorf("SPAXEL_NTP_LOCAL_ENABLED=%s invalid: must be one of true, false, 1, 0", ntpLocalEnabled))
	}

	// SPAXEL_NTP_SERVER - string, default 'pool.ntp.org', unless
	// SPAXEL_NTP_LOCAL_ENABLED=true and this wasn't explicitly set, in which
	// case nodes are pointed at this host's own (already-validated,
	// node-routable) address instead.
	if ntpServer := os.Getenv("SPAXEL_NTP_SERVER"); ntpServer != "" {
		cfg.NTPServer = ntpServer
	} else if cfg.NTPLocalEnabled && cfg.AdvertisedBaseURL != "" {
		if u, err := url.Parse(cfg.AdvertisedBaseURL); err == nil && u.Hostname() != "" {
			cfg.NTPServer = u.Hostname()
		} else {
			cfg.NTPServer = "pool.ntp.org"
		}
	} else {
		cfg.NTPServer = "pool.ntp.org"
	}

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

// isWildcardHost reports whether host is a wildcard bind address (0.0.0.0, ::)
// or empty. Such an address is valid to bind to but is never reachable as a
// destination, so it must never appear in a URL handed to a node.
func isWildcardHost(host string) bool {
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}

// validateAdvertisedBaseURL rejects URLs that nodes could not fetch from.
func validateAdvertisedBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("SPAXEL_ADVERTISED_BASE_URL=%s invalid: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("SPAXEL_ADVERTISED_BASE_URL=%s invalid: scheme must be http or https", raw)
	}
	if isWildcardHost(u.Hostname()) {
		return fmt.Errorf("SPAXEL_ADVERTISED_BASE_URL=%s invalid: %q is a wildcard bind address, not routable from nodes", raw, u.Hostname())
	}
	return nil
}

// deriveAdvertisedBaseURL builds a node-facing base URL from the bind address.
//
// A concrete bind host is used directly. A wildcard bind host is resolved to a
// routable interface address ONLY when exactly one candidate exists; with zero
// or several it fails at startup instead of guessing. A wrong guess on a
// multi-homed host would reintroduce exactly the silent OTA failure this
// function exists to prevent, and a startup error is far cheaper to diagnose
// than a node that reports a successful trigger and then cannot download.
func deriveAdvertisedBaseURL(bindAddr string) (string, error) {
	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return "", fmt.Errorf("cannot derive advertised base URL from SPAXEL_BIND_ADDR=%s: %w", bindAddr, err)
	}
	if !isWildcardHost(host) {
		return "http://" + net.JoinHostPort(host, port), nil
	}

	candidates, err := routableIPv4s()
	if err != nil {
		return "", fmt.Errorf("cannot derive advertised base URL: %w", err)
	}
	switch len(candidates) {
	case 1:
		derived := "http://" + net.JoinHostPort(candidates[0], port)
		log.Printf("[CONFIG] SPAXEL_ADVERTISED_BASE_URL not set; derived %s from the only routable interface", derived)
		return derived, nil
	case 0:
		return "", fmt.Errorf(
			"SPAXEL_BIND_ADDR=%s is a wildcard and no routable interface was found: "+
				"set SPAXEL_ADVERTISED_BASE_URL to the address nodes should fetch firmware from", bindAddr)
	default:
		return "", fmt.Errorf(
			"SPAXEL_BIND_ADDR=%s is a wildcard and this host has %d routable addresses (%s): "+
				"set SPAXEL_ADVERTISED_BASE_URL explicitly so nodes are not handed a guess",
			bindAddr, len(candidates), strings.Join(candidates, ", "))
	}
}

// routableIPv4s returns non-loopback, non-link-local IPv4 addresses on up interfaces.
func routableIPv4s() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	return out, nil
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
	log.Printf("[CONFIG] SPAXEL_ADVERTISED_BASE_URL=%s", cfg.AdvertisedBaseURL)
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
	log.Printf("[CONFIG] SPAXEL_NTP_LOCAL_ENABLED=%t", cfg.NTPLocalEnabled)
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
