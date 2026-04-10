// Package render provides floor-plan thumbnail rendering for notifications.
package render

import (
	"bytes"
	"fmt"
	"image/color"
	"math"

	"github.com/fogleman/gg"
)

// NotificationType represents the type of notification event.
type NotificationType string

const (
	NotificationZoneEnter      NotificationType = "zone_enter"
	NotificationZoneLeave      NotificationType = "zone_leave"
	NotificationZoneVacant     NotificationType = "zone_vacant"
	NotificationFallDetected   NotificationType = "fall_detected"
	NotificationFallEscalation NotificationType = "fall_escalation"
	NotificationAnomalyAlert   NotificationType = "anomaly_alert"
	NotificationNodeOffline    NotificationType = "node_offline"
	NotificationSleepSummary   NotificationType = "sleep_summary"
)

// Zone represents a zone in the floor plan.
type Zone struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	D      float64 `json:"d"`
	Color  string  `json:"color"`
	Highlight bool   `json:"highlight"` // Highlight this zone (event location)
}

// Node represents a node position.
type Node struct {
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z"`
}

// Person represents a tracked person.
type Person struct {
	Name      string  `json:"name"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Z         float64 `json:"z"`
	Color     string  `json:"color"`
	Confidence float64 `json:"confidence"`
	IsFall    bool    `json:"is_fall"`
}

// Portal represents a portal between zones.
type Portal struct {
	Name  string  `json:"name"`
	X1    float64 `json:"x1"`
	Y1    float64 `json:"y1"`
	X2    float64 `json:"x2"`
	Y2    float64 `json:"y2"`
}

// RenderConfig holds configuration for floor-plan rendering.
type RenderConfig struct {
	Width        int           // Image width in pixels (default 300)
	Height       int           // Image height in pixels (default 300)
	RoomWidth    float64       // Room width in meters
	RoomDepth    float64       // Room depth in meters
	Zones        []Zone        // Zones to render
	Nodes        []Node        // Nodes to render
	People       []Person      // People to render
	Portals      []Portal      // Portals to render
	EventType    NotificationType // Event type for title overlay
	EventTitle   string        // Event title text
	BackgroundColor color.Color // Background color
}

// DefaultRenderConfig returns a config with sensible defaults.
func DefaultRenderConfig() RenderConfig {
	return RenderConfig{
		Width:        300,
		Height:       300,
		RoomWidth:    10.0,
		RoomDepth:    10.0,
		BackgroundColor: color.RGBA{26, 26, 46, 255}, // Dark background
	}
}

// Renderer generates floor-plan thumbnails.
type Renderer struct {
	config RenderConfig
	dc     *gg.Context
}

// NewRenderer creates a new floor-plan renderer.
func NewRenderer(config RenderConfig) *Renderer {
	if config.Width == 0 {
		config.Width = 300
	}
	if config.Height == 0 {
		config.Height = 300
	}
	if config.RoomWidth == 0 {
		config.RoomWidth = 10.0
	}
	if config.RoomDepth == 0 {
		config.RoomDepth = 10.0
	}
	if config.BackgroundColor.A == 0 {
		config.BackgroundColor = color.RGBA{26, 26, 46, 255}
	}

	return &Renderer{config: config}
}

// Render generates a floor-plan PNG as bytes.
func (r *Renderer) Render() ([]byte, error) {
	// Create drawing context
	r.dc = gg.NewContext(r.config.Width, r.config.Height)

	// Draw background
	r.dc.SetColor(r.config.BackgroundColor)
	r.dc.Clear()

	// Calculate scale factors
	margin := 10.0
	drawWidth := float64(r.config.Width) - 2*margin
	drawHeight := float64(r.config.Height) - 2*margin - 20 // Reserve space for title

	scaleX := drawWidth / r.config.RoomWidth
	scaleY := drawHeight / r.config.RoomDepth
	scale := math.Min(scaleX, scaleY)

	// Center the drawing
	offsetX := margin + (drawWidth - r.config.RoomWidth*scale)/2
	offsetY := margin + (drawHeight - r.config.RoomDepth*scale)/2

	// Draw zones
	for _, zone := range r.config.Zones {
		r.drawZone(zone, offsetX, offsetY, scale)
	}

	// Draw portals
	for _, portal := range r.config.Portals {
		r.drawPortal(portal, offsetX, offsetY, scale)
	}

	// Draw nodes
	for _, node := range r.config.Nodes {
		r.drawNode(node, offsetX, offsetY, scale)
	}

	// Draw people
	for _, person := range r.config.People {
		r.drawPerson(person, offsetX, offsetY, scale)
	}

	// Draw event title overlay
	if r.config.EventTitle != "" {
		r.drawEventTitle()
	}

	// Encode to PNG
	var buf bytes.Buffer
	if err := r.dc.EncodePNG(&buf); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}

	return buf.Bytes(), nil
}

// drawZone draws a zone rectangle with optional highlight.
func (r *Renderer) drawZone(zone Zone, offsetX, offsetY, scale float64) {
	// Calculate screen coordinates
	x := offsetX + zone.X*scale
	y := offsetY + zone.Y*scale
	w := zone.W * scale
	h := zone.D * scale

	// Parse zone color
	zoneColor := r.parseColor(zone.Color)
	if zoneColor.A == 0 {
		zoneColor = color.RGBA{79, 195, 247, 51} // Default blue with 20% opacity
	}

	// Draw zone fill
	if zone.Highlight {
		// Brighter fill for highlighted zone
		r.dc.SetColor(color.RGBA{
			R: uint8(math.Min(255, float64(zoneColor.R) * 1.5)),
			G: uint8(math.Min(255, float64(zoneColor.G) * 1.5)),
			B: uint8(math.Min(255, float64(zoneColor.B) * 1.5)),
			A: 150, // Higher opacity for highlight
		})
		r.dc.DrawRectangle(x, y, w, h)
		r.dc.Fill()

		// White border for highlighted zone
		r.dc.SetLineWidth(2)
		r.dc.SetColor(color.RGBA{255, 255, 255, 255})
		r.dc.DrawRectangle(x, y, w, h)
		r.dc.Stroke()
	} else {
		// Normal semi-transparent fill
		r.dc.SetColor(color.RGBA{
			R: zoneColor.R,
			G: zoneColor.G,
			B: zoneColor.B,
			A: 51, // 20% opacity
		})
		r.dc.DrawRectangle(x, y, w, h)
		r.dc.Fill()

		// Thin white outline
		r.dc.SetLineWidth(1)
		r.dc.SetColor(color.RGBA{255, 255, 255, 100})
		r.dc.DrawRectangle(x, y, w, h)
		r.dc.Stroke()
	}

	// Draw zone label (if space permits)
	if w > 30 && h > 15 {
		r.dc.SetColor(color.RGBA{255, 255, 255, 200})
		r.dc.SetFontSize(8)

		// Truncate name if too long
		label := zone.Name
		if len(label) > 10 {
			label = label[:7] + "..."
		}

		tw, th := r.dc.MeasureString(label)
		r.dc.DrawStringAnchored(label, x+w/2, y+h/2, 0.5, 0.5)
		_ = tw
		_ = th
	}
}

// drawPortal draws a portal as a purple line.
func (r *Renderer) drawPortal(portal Portal, offsetX, offsetY, scale float64) {
	x1 := offsetX + portal.X1*scale
	y1 := offsetY + portal.Y1*scale
	x2 := offsetX + portal.X2*scale
	y2 := offsetY + portal.Y2*scale

	r.dc.SetLineWidth(2)
	r.dc.SetColor(color.RGBA{168, 85, 247, 255}) // Purple
	r.dc.DrawLine(x1, y1, x2, y2)
	r.dc.Stroke()
}

// drawNode draws a node position as a small white circle.
func (r *Renderer) drawNode(node Node, offsetX, offsetY, scale float64) {
	x := offsetX + node.X*scale
	y := offsetY + node.Y*scale

	r.dc.SetColor(color.RGBA{255, 255, 255, 255})
	r.dc.DrawCircle(x, y, 3)
	r.dc.Fill()
}

// drawPerson draws a person as a colored circle with name label.
func (r *Renderer) drawPerson(person Person, offsetX, offsetY, scale float64) {
	x := offsetX + person.X*scale
	y := offsetY + person.Y*scale

	// Parse person color
	personColor := r.parseColor(person.Color)
	if personColor.A == 0 {
		if person.IsFall {
			personColor = color.RGBA{239, 83, 80, 255} // Red for fall
		} else {
			personColor = color.RGBA{136, 136, 136, 255} // Gray for unknown
		}
	}

	// Diameter proportional to confidence (10px to 20px)
	diameter := 10.0 + person.Confidence*10.0
	if diameter > 20 {
		diameter = 20
	}
	if diameter < 10 {
		diameter = 10
	}

	// Draw filled circle
	r.dc.SetColor(personColor)
	r.dc.DrawCircle(x, y, diameter/2)
	r.dc.Fill()

	// Draw white outline
	r.dc.SetLineWidth(1.5)
	r.dc.SetColor(color.RGBA{255, 255, 255, 255})
	r.dc.DrawCircle(x, y, diameter/2)
	r.dc.Stroke()

	// Draw name label above circle
	if person.Name != "" {
		r.dc.SetColor(color.RGBA{255, 255, 255, 255})
		r.dc.SetFontSize(8)
		r.dc.DrawStringAnchored(person.Name, x, y-diameter/2-2, 0.5, 1.0)
	}
}

// drawEventTitle draws the event title at the bottom.
func (r *Renderer) drawEventTitle() {
	r.dc.SetColor(color.RGBA{255, 255, 255, 200})
	r.dc.SetFontSize(10)

	// Draw at bottom-left with margin
	margin := 10.0
	r.dc.DrawStringWrapped(r.config.EventTitle, margin, float64(r.config.Height)-margin-10, 0, float64(r.config.Width)-2*margin, 0, gg.AlignLeft)
}

// parseColor parses a hex color string or returns a default color.
func (r *Renderer) parseColor(hex string) color.RGBA {
	if len(hex) == 0 {
		return color.RGBA{}
	}

	var rVal, gVal, bVal uint8
	n, _ := fmt.Sscanf(hex, "#%02x%02x%02x", &rVal, &gVal, &bVal)
	if n == 3 {
		return color.RGBA{R: rVal, G: gVal, B: bVal, A: 255}
	}

	// Try with alpha
	n, _ = fmt.Sscanf(hex, "#%02x%02x%02x%02x", &rVal, &gVal, &bVal, &n)
	if n == 4 {
		return color.RGBA{R: rVal, G: gVal, B: bVal, A: n}
	}

	return color.RGBA{}
}

// GenerateThumbnail generates a floor-plan thumbnail with the given configuration.
func GenerateThumbnail(config RenderConfig) ([]byte, error) {
	renderer := NewRenderer(config)
	return renderer.Render()
}

// GenerateZoneEnterThumbnail generates a thumbnail for zone entry event.
func GenerateZoneEnterThumbnail(roomWidth, roomDepth float64, zones []Zone, person Person, zoneName string) ([]byte, error) {
	// Highlight the zone where person entered
	highlightedZones := make([]Zone, len(zones))
	for i, z := range zones {
		highlightedZones[i] = z
		if z.Name == zoneName {
			highlightedZones[i].Highlight = true
		}
	}

	config := DefaultRenderConfig()
	config.RoomWidth = roomWidth
	config.RoomDepth = roomDepth
	config.Zones = highlightedZones
	config.People = []Person{person}
	config.EventType = NotificationZoneEnter
	config.EventTitle = fmt.Sprintf("%s entered %s", person.Name, zoneName)

	return GenerateThumbnail(config)
}

// GenerateFallDetectedThumbnail generates a thumbnail for fall detection event.
func GenerateFallDetectedThumbnail(roomWidth, roomDepth float64, zones []Zone, person Person, zoneName string) ([]byte, error) {
	// Highlight the zone where fall occurred
	highlightedZones := make([]Zone, len(zones))
	for i, z := range zones {
		highlightedZones[i] = z
		if z.Name == zoneName {
			highlightedZones[i].Highlight = true
		}
	}

	// Mark person as fallen
	fallenPerson := person
	fallenPerson.IsFall = true

	config := DefaultRenderConfig()
	config.RoomWidth = roomWidth
	config.RoomDepth = roomDepth
	config.Zones = highlightedZones
	config.People = []Person{fallenPerson}
	config.EventType = NotificationFallDetected
	config.EventTitle = fmt.Sprintf("Fall: %s in %s", person.Name, zoneName)

	return GenerateThumbnail(config)
}

// GenerateAnomalyAlertThumbnail generates a thumbnail for anomaly alert.
func GenerateAnomalyAlertThumbnail(roomWidth, roomDepth float64, zones []Zone, zoneName string) ([]byte, error) {
	// Highlight the anomalous zone
	highlightedZones := make([]Zone, len(zones))
	for i, z := range zones {
		highlightedZones[i] = z
		if z.Name == zoneName {
			highlightedZones[i].Highlight = true
		}
	}

	config := DefaultRenderConfig()
	config.RoomWidth = roomWidth
	config.RoomDepth = roomDepth
	config.Zones = highlightedZones
	config.EventType = NotificationAnomalyAlert
	config.EventTitle = fmt.Sprintf("Unusual activity in %s", zoneName)

	return GenerateThumbnail(config)
}

// GenerateSleepSummaryThumbnail generates a thumbnail for sleep summary.
func GenerateSleepSummaryThumbnail(roomWidth, roomDepth float64, zones []Zone, person Person, duration string) ([]byte, error) {
	config := DefaultRenderConfig()
	config.RoomWidth = roomWidth
	config.RoomDepth = roomDepth
	config.Zones = zones
	config.People = []Person{person}
	config.EventType = NotificationSleepSummary
	config.EventTitle = fmt.Sprintf("Sleep: %s (last night)", duration)

	return GenerateThumbnail(config)
}
