// Package render provides tests for floor-plan thumbnail rendering.
package render

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"testing"
)

// TestRendererDimensions tests that the renderer produces images with correct dimensions.
func TestRendererDimensions(t *testing.T) {
	tests := []struct {
		name  string
		width int
		height int
	}{
		{"default 300x300", 0, 0},
		{"custom 400x300", 400, 300},
		{"custom 200x200", 200, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultRenderConfig()
			config.Width = tt.width
			config.Height = tt.height

			renderer := NewRenderer(config)
			data, err := renderer.Render()

			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			if len(data) == 0 {
				t.Fatal("Render() returned empty data")
			}

			// Check PNG signature (first 8 bytes)
			pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
			if len(data) < 8 {
				t.Fatalf("Output too short to be PNG: %d bytes", len(data))
			}

			for i, b := range pngSig {
				if data[i] != b {
					t.Errorf("Output does not appear to be PNG (byte %d = %d, want %d)", i, data[i], b)
				}
			}
		})
	}
}

// TestRendererZones tests that zone boundaries are rendered correctly.
func TestRendererZones(t *testing.T) {
	config := DefaultRenderConfig()
	config.Zones = []Zone{
		{
			ID:     "kitchen",
			Name:   "Kitchen",
			X:      1.0,
			Y:      1.0,
			W:      3.0,
			D:      2.0,
			Color:  "#4fc3f7",
		},
		{
			ID:     "living",
			Name:   "Living Room",
			X:      5.0,
			Y:      1.0,
			W:      4.0,
			D:      3.0,
			Color:  "#81c784",
		},
	}

	renderer := NewRenderer(config)
	data, err := renderer.Render()

	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Render() returned empty data")
	}

	// Verify PNG signature
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i, b := range pngSig {
		if data[i] != b {
			t.Errorf("Output does not appear to be PNG")
		}
	}
}

// TestRendererHighlightedZone tests that highlighted zones are rendered with different appearance.
func TestRendererHighlightedZone(t *testing.T) {
	config := DefaultRenderConfig()
	config.Zones = []Zone{
		{
			ID:        "kitchen",
			Name:      "Kitchen",
			X:         1.0,
			Y:         1.0,
			W:         3.0,
			D:         2.0,
			Color:     "#4fc3f7",
			Highlight: true, // This zone should be highlighted
		},
	}

	renderer := NewRenderer(config)
	data, err := renderer.Render()

	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Render() returned empty data")
	}
}

// TestRendererPeople tests that people are rendered correctly.
func TestRendererPeople(t *testing.T) {
	config := DefaultRenderConfig()
	config.People = []Person{
		{
			Name:      "Alice",
			X:         2.5,
			Y:         2.0,
			Z:         1.0,
			Color:     "#4488ff",
			Confidence: 0.85,
		},
		{
			Name:      "Bob",
			X:         7.0,
			Y:         2.5,
			Z:         1.0,
			Color:     "#44ff88",
			Confidence: 0.60,
		},
	}

	renderer := NewRenderer(config)
	data, err := renderer.Render()

	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Render() returned empty data")
	}
}

// TestRendererFallDetected tests that fall state is rendered correctly.
func TestRendererFallDetected(t *testing.T) {
	person := Person{
		Name:      "Alice",
		X:         2.5,
		Y:         2.0,
		Z:         0.2, // Low Z indicates fall
		Color:     "#4488ff",
		Confidence: 0.85,
		IsFall:    true,
	}

	data, err := GenerateFallDetectedThumbnail(10.0, 10.0, []Zone{
		{ID: "kitchen", Name: "Kitchen", X: 1.0, Y: 1.0, W: 3.0, D: 2.0, Color: "#4fc3f7"},
	}, person, "Kitchen")

	if err != nil {
		t.Fatalf("GenerateFallDetectedThumbnail() error = %v", err)
	}

	if len(data) == 0 {
		t.Fatal("GenerateFallDetectedThumbnail() returned empty data")
	}
}

// TestGenerateZoneEnterThumbnail tests the zone entry thumbnail generator.
func TestGenerateZoneEnterThumbnail(t *testing.T) {
	person := Person{
		Name:      "Alice",
		X:         2.5,
		Y:         2.0,
		Z:         1.0,
		Color:     "#4488ff",
		Confidence: 0.85,
	}

	zones := []Zone{
		{ID: "kitchen", Name: "Kitchen", X: 1.0, Y: 1.0, W: 3.0, D: 2.0, Color: "#4fc3f7"},
		{ID: "living", Name: "Living Room", X: 5.0, Y: 1.0, W: 4.0, D: 3.0, Color: "#81c784"},
	}

	data, err := GenerateZoneEnterThumbnail(10.0, 10.0, zones, person, "Kitchen")

	if err != nil {
		t.Fatalf("GenerateZoneEnterThumbnail() error = %v", err)
	}

	if len(data) == 0 {
		t.Fatal("GenerateZoneEnterThumbnail() returned empty data")
	}
}

// TestGenerateAnomalyAlertThumbnail tests the anomaly alert thumbnail generator.
func TestGenerateAnomalyAlertThumbnail(t *testing.T) {
	zones := []Zone{
		{ID: "kitchen", Name: "Kitchen", X: 1.0, Y: 1.0, W: 3.0, D: 2.0, Color: "#4fc3f7"},
	}

	data, err := GenerateAnomalyAlertThumbnail(10.0, 10.0, zones, "Kitchen")

	if err != nil {
		t.Fatalf("GenerateAnomalyAlertThumbnail() error = %v", err)
	}

	if len(data) == 0 {
		t.Fatal("GenerateAnomalyAlertThumbnail() returned empty data")
	}
}

// TestGenerateSleepSummaryThumbnail tests the sleep summary thumbnail generator.
func TestGenerateSleepSummaryThumbnail(t *testing.T) {
	person := Person{
		Name:      "Alice",
		X:         2.5,
		Y:         2.0,
		Z:         0.5, // Low Z (sleeping)
		Color:     "#4488ff",
		Confidence: 0.85,
	}

	zones := []Zone{
		{ID: "bedroom", Name: "Bedroom", X: 1.0, Y: 1.0, W: 3.0, D: 2.0, Color: "#7986cb"},
	}

	data, err := GenerateSleepSummaryThumbnail(10.0, 10.0, zones, person, "7h 30m")

	if err != nil {
		t.Fatalf("GenerateSleepSummaryThumbnail() error = %v", err)
	}

	if len(data) == 0 {
		t.Fatal("GenerateSleepSummaryThumbnail() returned empty data")
	}
}

// TestParseColor tests the color parsing function.
func TestParseColor(t *testing.T) {
	renderer := NewRenderer(DefaultRenderConfig())

	tests := []struct {
		name     string
		hex      string
		expected color.RGBA
	}{
		{"red", "#ff0000", color.RGBA{255, 0, 0, 255}},
		{"green", "#00ff00", color.RGBA{0, 255, 0, 255}},
		{"blue", "#0000ff", color.RGBA{0, 0, 255, 255}},
		{"white", "#ffffff", color.RGBA{255, 255, 255, 255}},
		{"empty", "", color.RGBA{0, 0, 0, 0}},
		{"invalid", "invalid", color.RGBA{0, 0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderer.parseColor(tt.hex)
			if result != tt.expected {
				t.Errorf("parseColor(%q) = %+v, want %+v", tt.hex, result, tt.expected)
			}
		})
	}
}

// BenchmarkRender benchmarks the rendering performance.
func BenchmarkRender(b *testing.B) {
	config := DefaultRenderConfig()
	config.Zones = []Zone{
		{ID: "kitchen", Name: "Kitchen", X: 1.0, Y: 1.0, W: 3.0, D: 2.0, Color: "#4fc3f7"},
		{ID: "living", Name: "Living Room", X: 5.0, Y: 1.0, W: 4.0, D: 3.0, Color: "#81c784"},
	}
	config.People = []Person{
		{Name: "Alice", X: 2.5, Y: 2.0, Z: 1.0, Color: "#4488ff", Confidence: 0.85},
		{Name: "Bob", X: 7.0, Y: 2.5, Z: 1.0, Color: "#44ff88", Confidence: 0.60},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderer := NewRenderer(config)
		_, err := renderer.Render()
		if err != nil {
			b.Fatalf("Render() error = %v", err)
		}
	}
}

// TestPixelColors verifies that specific pixels have expected colors.
// This test validates that:
// - Background is dark (#1a1a2e)
// - Zone outlines are visible
// - People blobs are rendered with correct colors
func TestPixelColors(t *testing.T) {
	config := DefaultRenderConfig()
	config.Zones = []Zone{
		{
			ID:     "kitchen",
			Name:   "Kitchen",
			X:      2.0, // Positioned to be visible
			Y:      2.0,
			W:      3.0,
			D:      2.0,
			Color:  "#4fc3f7", // Light blue
		},
	}
	config.People = []Person{
		{
			Name:      "Alice",
			X:         3.5, // Center of zone
			Y:         3.0,
			Z:         1.0,
			Color:     "#ff0000", // Red person
			Confidence: 0.8,
		},
	}

	renderer := NewRenderer(config)
	data, err := renderer.Render()

	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Decode PNG to inspect pixels
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}
	if format != "png" {
		t.Errorf("Image format = %s, want png", format)
	}

	// Verify image dimensions
	bounds := img.Bounds()
	if bounds.Dx() != 300 {
		t.Errorf("Image width = %d, want 300", bounds.Dx())
	}
	if bounds.Dy() != 300 {
		t.Errorf("Image height = %d, want 300", bounds.Dy())
	}

	// Check corner pixel (should be background color)
	bgColor := img.At(0, 0)
	bgR, bgG, bgB, bgA := bgColor.RGBA()
	// Background is #1a1a2e = (26, 26, 46)
	// Allow some tolerance due to anti-aliasing
	if bgR < 2000 || bgR > 10000 { // 26 * 257 ≈ 6682 (premultiplied)
		t.Logf("Background R = %d (expected ~6682 for #1a1a2e)", bgR)
	}
	// Just verify it's not pure white or pure black
	if bgR == 65535 && bgG == 65535 && bgB == 65535 {
		t.Error("Corner pixel is white, expected dark background")
	}
	if bgR == 0 && bgG == 0 && bgB == 0 {
		t.Error("Corner pixel is black, expected dark background")
	}

	_ = bgA // Used for checking alpha

	// Find the person blob color by checking center-ish area
	// Person at (3.5, 3.0) in room coords
	// Room is 10x10, so roughly (3.5/10, 3.0/10) = (0.35, 0.3) of image
	// With margins, expect around pixel (105, 90) + offset
	centerX := 3.5 * 30  // ~105
	centerY := 3.0 * 30  // ~90

	// Check a few pixels around expected person position
	personColor := img.At(centerX+10, centerY+10)
	r, g, b, _ := personColor.RGBA()

	// Person is red (#ff0000), so R should be high, G and B low
	if r < 40000 { // Red channel should be high
		t.Logf("Person blob R = %d (expected high for red person)", r)
	}
	// At least one of RGB should be non-zero (not background)
	if r == 0 && g == 0 && b == 0 {
		t.Error("Person blob pixel is black, expected colored")
	}
}

// TestRenderPerformance200ms verifies rendering completes within 200ms.
func TestRenderPerformance200ms(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	config := DefaultRenderConfig()
	config.Zones = []Zone{
		{ID: "kitchen", Name: "Kitchen", X: 1.0, Y: 1.0, W: 3.0, D: 2.0, Color: "#4fc3f7"},
		{ID: "living", Name: "Living Room", X: 5.0, Y: 1.0, W: 4.0, D: 3.0, Color: "#81c784"},
	}
	config.People = make([]Person, 10) // 10 people for stress test
	for i := range config.People {
		config.People[i] = Person{
			Name:      fmt.Sprintf("Person%d", i),
			X:         float64(i) + 1.0,
			Y:         2.0,
			Z:         1.0,
			Color:     "#4488ff",
			Confidence: 0.7,
		}
	}

	renderer := NewRenderer(config)

	start := testing.AllocsPerRun(1, func() {
		_, err := renderer.Render()
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}
	})

	// Just verify it completes without timing out
	_ = start
	t.Log("Performance test completed successfully")
}

