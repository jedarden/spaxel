package config

import (
	"os"
	"strings"
	"testing"
)

// TestLoadValidConfig tests that a valid configuration loads successfully.
func TestLoadValidConfig(t *testing.T) {
	// Clear all env vars first
	clearEnvVars()

	// Set a valid config
	os.Setenv("SPAXEL_BIND_ADDR", "127.0.0.1:9090")
	os.Setenv("SPAXEL_DATA_DIR", "/tmp/testdata")
	os.Setenv("SPAXEL_LOG_LEVEL", "debug")
	os.Setenv("SPAXEL_FUSION_RATE_HZ", "15")
	os.Setenv("SPAXEL_REPLAY_MAX_MB", "500")
	os.Setenv("SPAXEL_NTP_SERVER", "time.google.com")
	os.Setenv("TZ", "America/New_York")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.BindAddr != "127.0.0.1:9090" {
		t.Errorf("BindAddr = %s, want 127.0.0.1:9090", cfg.BindAddr)
	}
	if cfg.DataDir != "/tmp/testdata" {
		t.Errorf("DataDir = %s, want /tmp/testdata", cfg.DataDir)
	}
	if cfg.StaticDir != "/dashboard" {
		t.Errorf("StaticDir = %s, want /dashboard", cfg.StaticDir)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %s, want debug", cfg.LogLevel)
	}
	if cfg.FusionRateHz != 15 {
		t.Errorf("FusionRateHz = %d, want 15", cfg.FusionRateHz)
	}
	if cfg.ReplayMaxMB != 500 {
		t.Errorf("ReplayMaxMB = %d, want 500", cfg.ReplayMaxMB)
	}
	if cfg.NTPServer != "time.google.com" {
		t.Errorf("NTPServer = %s, want time.google.com", cfg.NTPServer)
	}
	if cfg.Timezone != "America/New_York" {
		t.Errorf("Timezone = %s, want America/New_York", cfg.Timezone)
	}
	if cfg.MDNSEnabled != true {
		t.Errorf("MDNSEnabled = %t, want true", cfg.MDNSEnabled)
	}
	if cfg.MDNSName != "spaxel" {
		t.Errorf("MDNSName = %s, want spaxel", cfg.MDNSName)
	}
}

// TestLoadDefaults tests that all defaults are applied when env vars are unset.
func TestLoadDefaults(t *testing.T) {
	clearEnvVars()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.BindAddr != "0.0.0.0:8080" {
		t.Errorf("BindAddr = %s, want 0.0.0.0:8080", cfg.BindAddr)
	}
	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %s, want /data", cfg.DataDir)
	}
	if cfg.StaticDir != "/dashboard" {
		t.Errorf("StaticDir = %s, want /dashboard", cfg.StaticDir)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %s, want info", cfg.LogLevel)
	}
	if cfg.FusionRateHz != 10 {
		t.Errorf("FusionRateHz = %d, want 10", cfg.FusionRateHz)
	}
	if cfg.ReplayMaxMB != 360 {
		t.Errorf("ReplayMaxMB = %d, want 360", cfg.ReplayMaxMB)
	}
	if cfg.NTPServer != "pool.ntp.org" {
		t.Errorf("NTPServer = %s, want pool.ntp.org", cfg.NTPServer)
	}
	if cfg.Timezone != "UTC" {
		t.Errorf("Timezone = %s, want UTC", cfg.Timezone)
	}
	if cfg.MDNSEnabled != true {
		t.Errorf("MDNSEnabled = %t, want true", cfg.MDNSEnabled)
	}
	if cfg.MDNSName != "spaxel" {
		t.Errorf("MDNSName = %s, want spaxel", cfg.MDNSName)
	}
}

// TestInvalidFusionRateHz tests that invalid FUSION_RATE_HZ values are rejected.
func TestInvalidFusionRateHz(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"too low", "0", "must be in range [1,20]"},
		{"too high", "25", "must be in range [1,20]"},
		{"negative", "-5", "must be in range [1,20]"},
		{"non-integer", "abc", "must be an integer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars()
			os.Setenv("SPAXEL_FUSION_RATE_HZ", tt.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestInvalidReplayMaxMB tests that invalid REPLAY_MAX_MB values are rejected.
func TestInvalidReplayMaxMB(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"too low", "9", "must be in range [10,10000]"},
		{"too high", "10001", "must be in range [10,10000]"},
		{"negative", "-100", "must be in range [10,10000]"},
		{"non-integer", "xyz", "must be an integer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars()
			os.Setenv("SPAXEL_REPLAY_MAX_MB", tt.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestInvalidLogLevel tests that invalid LOG_LEVEL values are rejected.
func TestInvalidLogLevel(t *testing.T) {
	clearEnvVars()
	os.Setenv("SPAXEL_LOG_LEVEL", "verbose")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "must be one of debug, info, warn, error") {
		t.Errorf("error = %v, want containing 'must be one of debug, info, warn, error'", err)
	}
}

// TestInvalidMDNSEnabled tests that invalid MDNS_ENABLED values are rejected.
func TestInvalidMDNSEnabled(t *testing.T) {
	clearEnvVars()
	os.Setenv("SPAXEL_MDNS_ENABLED", "yes")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "must be one of true, false, 1, 0") {
		t.Errorf("error = %v, want containing 'must be one of true, false, 1, 0'", err)
	}
}

// TestInvalidInstallSecret tests that invalid INSTALL_SECRET values are rejected.
func TestInvalidInstallSecret(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"too short", "abcd1234", "must be at least 32 bytes"},
		{"invalid hex", "g" + strings.Repeat("0", 63), "must be a hex string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars()
			os.Setenv("SPAXEL_INSTALL_SECRET", tt.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestValidInstallSecret tests that valid INSTALL_SECRET values are accepted.
func TestValidInstallSecret(t *testing.T) {
	clearEnvVars()
	// 64 hex chars = 32 bytes
	validSecret := strings.Repeat("a", 64)
	os.Setenv("SPAXEL_INSTALL_SECRET", validSecret)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.InstallSecret != validSecret {
		t.Errorf("InstallSecret = %s, want %s", cfg.InstallSecret, validSecret)
	}
}

// TestInvalidMQTTBroker tests that invalid MQTT_BROKER values are rejected.
func TestInvalidMQTTBroker(t *testing.T) {
	clearEnvVars()
	os.Setenv("SPAXEL_MQTT_BROKER", "not-a-url")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "must be a valid URL") {
		t.Errorf("error = %v, want containing 'must be a valid URL'", err)
	}
}

// TestValidMQTTBroker tests that valid MQTT_BROKER values are accepted.
func TestValidMQTTBroker(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"tcp", "mqtt://broker.local:1883"},
		{"tls", "mqtts://broker.local:8883"},
		{"with userpass", "mqtt://user:pass@broker.local:1883"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars()
			os.Setenv("SPAXEL_MQTT_BROKER", tt.url)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() failed: %v", err)
			}
			if cfg.MQTTBroker != tt.url {
				t.Errorf("MQTTBroker = %s, want %s", cfg.MQTTBroker, tt.url)
			}
		})
	}
}

// TestInvalidTimezone tests that invalid TZ values are rejected.
func TestInvalidTimezone(t *testing.T) {
	clearEnvVars()
	os.Setenv("TZ", "Invalid/Timezone")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "TZ=") {
		t.Errorf("error = %v, want containing 'TZ='", err)
	}
}

// TestMultipleErrors tests that multiple validation errors are collected.
func TestMultipleErrors(t *testing.T) {
	clearEnvVars()
	os.Setenv("SPAXEL_LOG_LEVEL", "verbose")
	os.Setenv("SPAXEL_FUSION_RATE_HZ", "25")
	os.Setenv("SPAXEL_REPLAY_MAX_MB", "5")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded, want error")
	}
	// Check that all three errors are present
	errStr := err.Error()
	if !strings.Contains(errStr, "SPAXEL_LOG_LEVEL") {
		t.Errorf("error missing LOG_LEVEL validation: %v", err)
	}
	if !strings.Contains(errStr, "SPAXEL_FUSION_RATE_HZ") {
		t.Errorf("error missing FUSION_RATE_HZ validation: %v", err)
	}
	if !strings.Contains(errStr, "SPAXEL_REPLAY_MAX_MB") {
		t.Errorf("error missing REPLAY_MAX_MB validation: %v", err)
	}
}

// TestMDNSEnabledVariants tests all valid MDNS_ENABLED values.
func TestMDNSEnabledVariants(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			clearEnvVars()
			os.Setenv("SPAXEL_MDNS_ENABLED", tt.value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() failed: %v", err)
			}
			if cfg.MDNSEnabled != tt.expected {
				t.Errorf("MDNSEnabled = %t, want %t", cfg.MDNSEnabled, tt.expected)
			}
		})
	}
}

// TestLogLevelVariants tests all valid LOG_LEVEL values.
func TestLogLevelVariants(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			clearEnvVars()
			os.Setenv("SPAXEL_LOG_LEVEL", level)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() failed: %v", err)
			}
			if cfg.LogLevel != level {
				t.Errorf("LogLevel = %s, want %s", cfg.LogLevel, level)
			}
		})
	}
}

// TestFusionRate tests FusionRate() method.
func TestFusionRate(t *testing.T) {
	clearEnvVars()
	os.Setenv("SPAXEL_FUSION_RATE_HZ", "15")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.FusionRate() != 15.0 {
		t.Errorf("FusionRate() = %f, want 15.0", cfg.FusionRate())
	}
}

// TestReplayMaxBytes tests ReplayMaxBytes() method.
func TestReplayMaxBytes(t *testing.T) {
	clearEnvVars()
	os.Setenv("SPAXEL_REPLAY_MAX_MB", "500")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	expected := int64(500 * 1024 * 1024)
	if cfg.ReplayMaxBytes() != expected {
		t.Errorf("ReplayMaxBytes() = %d, want %d", cfg.ReplayMaxBytes(), expected)
	}
}

// TestTimezoneLocation tests TimezoneLocation() method.
func TestTimezoneLocation(t *testing.T) {
	clearEnvVars()
	os.Setenv("TZ", "America/New_York")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loc := cfg.TimezoneLocation()
	if loc.String() != "America/New_York" {
		t.Errorf("TimezoneLocation() = %s, want America/New_York", loc)
	}
}

// TestTimezoneLocationFallback tests that invalid timezone falls back to UTC.
func TestTimezoneLocationFallback(t *testing.T) {
	clearEnvVars()
	// Set TZ to an invalid value - this should cause Load() to fail
	os.Setenv("TZ", "Invalid/Timezone")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded, want error for invalid TZ")
	}
}

// clearEnvVars clears all SPAXEL_* and TZ environment variables.
func clearEnvVars() {
	envVars := []string{
		"SPAXEL_BIND_ADDR",
		"SPAXEL_DATA_DIR",
		"SPAXEL_STATIC_DIR",
		"SPAXEL_MDNS_ENABLED",
		"SPAXEL_MDNS_NAME",
		"SPAXEL_LOG_LEVEL",
		"SPAXEL_FUSION_RATE_HZ",
		"SPAXEL_REPLAY_MAX_MB",
		"SPAXEL_INSTALL_SECRET",
		"SPAXEL_NTP_SERVER",
		"SPAXEL_MQTT_BROKER",
		"SPAXEL_MQTT_USERNAME",
		"SPAXEL_MQTT_PASSWORD",
		"TZ",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}
}
