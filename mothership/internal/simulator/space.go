// Package simulator provides pre-deployment simulation for WiFi CSI-based positioning.
// It enables users to define virtual spaces, place virtual nodes, and simulate walkers
// to predict detection quality before purchasing hardware.
package simulator

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	// WiFi wavelength at 2.4 GHz
	Wavelength = 0.123 // meters
	// Half-wavelength for Fresnel zone calculation
	HalfWavelength = Wavelength / 2.0
	// Frequency for subcarrier phase calculation
	SubcarrierSpacing = 312.5e3 // Hz
	// Speed of light
	C = 3e8 // m/s
)

// WallMaterial represents different wall types and their RF attenuation
type WallMaterial string

const (
	MaterialDrywall  WallMaterial = "drywall"
	MaterialBrick    WallMaterial = "brick"
	MaterialConcrete WallMaterial = "concrete"
	MaterialGlass    WallMaterial = "glass"
	MaterialMetal    WallMaterial = "metal"
)

// WallPenetrationLoss returns dB loss for each material type
func WallPenetrationLoss(m WallMaterial) float64 {
	switch m {
	case MaterialDrywall:
		return 3.0
	case MaterialBrick, MaterialConcrete:
		return 10.0
	case MaterialGlass:
		return 2.0
	case MaterialMetal:
		return 20.0
	default:
		return 3.0 // Default to drywall
	}
}

// Point represents a 3D point in meters
type Point struct{ X, Y, Z float64 }

// NewPoint creates a new 3D point
func NewPoint(x, y, z float64) Point {
	return Point{X: x, Y: y, Z: z}
}

// Distance returns Euclidean distance to another point
func (p Point) Distance(to Point) float64 {
	dx := to.X - p.X
	dy := to.Y - p.Y
	dz := to.Z - p.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// Vector returns the direction vector from p to to (normalized)
func (p Point) Vector(to Point) Point {
	d := p.Distance(to)
	if d < 1e-9 {
		return Point{0, 0, 0}
	}
	return Point{
		X: (to.X - p.X) / d,
		Y: (to.Y - p.Y) / d,
		Z: (to.Z - p.Z) / d,
	}
}

// Add returns a new point with added coordinates
func (p Point) Add(v Point) Point {
	return Point{X: p.X + v.X, Y: p.Y + v.Y, Z: p.Z + v.Z}
}

// Scale returns a new point scaled by factor
func (p Point) Scale(f float64) Point {
	return Point{X: p.X * f, Y: p.Y * f, Z: p.Z * f}
}

// WallSegment represents a flat wall with material properties
type WallSegment struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Material  WallMaterial `json:"material"`
	P1        Point        `json:"p1"` // Corner 1 (floor level)
	P2        Point        `json:"p2"` // Corner 2 (floor level)
	Height    float64      `json:"height"`
}

// Bounds returns the axis-aligned bounding box of this wall
func (w *WallSegment) Bounds() (minX, minY, minZ, maxX, maxY, maxZ float64) {
	minX = math.Min(w.P1.X, w.P2.X)
	maxX = math.Max(w.P1.X, w.P2.X)
	minY = math.Min(w.P1.Y, w.P2.Y)
	maxY = math.Max(w.P1.Y, w.P2.Y)
	minZ = math.Min(w.P1.Z, w.P2.Z)
	maxZ = math.Max(w.P1.Z, w.P2.Z) // Top of wall
	return
}

// IntersectsLine returns true if the wall segment intersects the 2D line segment
// from a to b (projected on the XY plane)
func (w *WallSegment) IntersectsLine(a, b Point) bool {
	// Project to 2D (ignore Z for wall intersection check)
	ax, ay := a.X, a.Y
	bx, by := b.X, b.Y
	wx1, wy1 := w.P1.X, w.P1.Y
	wx2, wy2 := w.P2.X, w.P2.Y

	// Compute orientation
	orientation := func(p1x, p1y, p2x, p2y, px, py float64) float64 {
		return (p2x-p1x)*(py-p1y) - (p2y-p1y)*(px-p1x)
	}

	// Check if line segments intersect
	o1 := orientation(ax, ay, bx, by, wx1, wy1)
	o2 := orientation(ax, ay, bx, by, wx2, wy2)
	o3 := orientation(wx1, wy1, wx2, wy2, ax, ay)
	o4 := orientation(wx1, wy1, wx2, wy2, bx, by)

	if o1*o2 < 0 && o3*o4 < 0 {
		return true
	}

	// Check collinear cases
	if math.Abs(o1) < 1e-9 && onSegment(ax, ay, bx, by, wx1, wy1) {
		return true
	}
	if math.Abs(o2) < 1e-9 && onSegment(ax, ay, bx, by, wx2, wy2) {
		return true
	}
	if math.Abs(o3) < 1e-9 && onSegment(wx1, wy1, wx2, wy2, ax, ay) {
		return true
	}
	if math.Abs(o4) < 1e-9 && onSegment(wx1, wy1, wx2, wy2, bx, by) {
		return true
	}

	return false
}

// onSegment checks if point q lies on segment pr (collinear case)
func onSegment(px, py, qx, qy, rx, ry float64) bool {
	return qx <= math.Max(px, rx) && qx >= math.Min(px, rx) &&
		qy <= math.Max(py, ry) && qy >= math.Min(py, ry)
}

// Room defines a room in the virtual space
type Room struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	MinX    float64 `json:"min_x"`
	MinY    float64 `json:"min_y"`
	MinZ    float64 `json:"min_z"`
	MaxX    float64 `json:"max_x"`
	MaxY    float64 `json:"max_y"`
	MaxZ    float64 `json:"max_z"`
	Walls   []WallSegment `json:"walls,omitempty"`
}

// Center returns the center point of the room
func (r *Room) Center() Point {
	return Point{
		X: (r.MinX + r.MaxX) / 2,
		Y: (r.MinY + r.MaxY) / 2,
		Z: (r.MinZ + r.MaxZ) / 2,
	}
}

// Dimensions returns the width, depth, and height of the room
func (r *Room) Dimensions() (width, depth, height float64) {
	return r.MaxX - r.MinX, r.MaxY - r.MinY, r.MaxZ - r.MinZ
}

// Volume returns the volume of the room in cubic meters
func (r *Room) Volume() float64 {
	w, d, h := r.Dimensions()
	return w * d * h
}

// Contains returns true if the point is inside the room
func (r *Room) Contains(p Point) bool {
	return p.X >= r.MinX && p.X <= r.MaxX &&
		p.Y >= r.MinY && p.Y <= r.MaxY &&
		p.Z >= r.MinZ && p.Z <= r.MaxZ
}

// Space defines the virtual simulation space
type Space struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Rooms  []Room       `json:"rooms"`
	Walls  []WallSegment `json:"walls"`
}

// Bounds returns the overall bounding box of the space
func (s *Space) Bounds() (minX, minY, minZ, maxX, maxY, maxZ float64) {
	if len(s.Rooms) == 0 {
		return 0, 0, 0, 10, 10, 2.5 // Default 10x10x2.5m space
	}

	minX, minY, minZ = s.Rooms[0].MinX, s.Rooms[0].MinY, s.Rooms[0].MinZ
	maxX, maxY, maxZ = s.Rooms[0].MaxX, s.Rooms[0].MaxY, s.Rooms[0].MaxZ

	for _, r := range s.Rooms {
		if r.MinX < minX {
			minX = r.MinX
		}
		if r.MinY < minY {
			minY = r.MinY
		}
		if r.MinZ < minZ {
			minZ = r.MinZ
		}
		if r.MaxX > maxX {
			maxX = r.MaxX
		}
		if r.MaxY > maxY {
			maxY = r.MaxY
		}
		if r.MaxZ > maxZ {
			maxZ = r.MaxZ
		}
	}

	return
}

// TotalVolume returns the total volume of all rooms
func (s *Space) TotalVolume() float64 {
	v := 0.0
	for _, r := range s.Rooms {
		v += r.Volume()
	}
	return v
}

// Dimensions returns the overall width, depth, and height of the space
func (s *Space) Dimensions() (width, depth, height float64) {
	minX, minY, minZ, maxX, maxY, maxZ := s.Bounds()
	return maxX - minX, maxY - minY, maxZ - minZ
}

// GetWalls returns all wall segments from all rooms plus standalone walls
func (s *Space) GetWalls() []WallSegment {
	walls := make([]WallSegment, 0, len(s.Walls))
	walls = append(walls, s.Walls...)
	for _, r := range s.Rooms {
		walls = append(walls, r.Walls...)
	}
	return walls
}

// DefaultSpace creates a default rectangular space
func DefaultSpace() *Space {
	return &Space{
		ID:   "default",
		Name: "Default Space",
		Rooms: []Room{{
			ID:   "room-1",
			Name: "Main Room",
			MinX: 0, MinY: 0, MinZ: 0,
			MaxX: 6, MaxY: 5, MaxZ: 2.5,
		}},
	}
}

// MarshalJSON implements custom JSON marshaling
func (s *Space) MarshalJSON() ([]byte, error) {
	type Alias Space
	return json.Marshal(&struct {
		Type string `json:"type"`
		*Alias
	}{
		Type:  "space",
		Alias: (*Alias)(s),
	})
}

// UnmarshalSpace creates a Space from JSON
func UnmarshalSpace(data []byte) (*Space, error) {
	var space Space
	if err := json.Unmarshal(data, &space); err != nil {
		return nil, fmt.Errorf("unmarshal space: %w", err)
	}
	return &space, nil
}

// Validate checks if the space definition is valid
func (s *Space) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("space ID is required")
	}
	if len(s.Rooms) == 0 {
		return fmt.Errorf("space must have at least one room")
	}
	for i, r := range s.Rooms {
		if r.MinX >= r.MaxX {
			return fmt.Errorf("room %d: MinX (%f) must be less than MaxX (%f)", i, r.MinX, r.MaxX)
		}
		if r.MinY >= r.MaxY {
			return fmt.Errorf("room %d: MinY (%f) must be less than MaxY (%f)", i, r.MinY, r.MaxY)
		}
		if r.MinZ >= r.MaxZ {
			return fmt.Errorf("room %d: MinZ (%f) must be less than MaxZ (%f)", i, r.MinZ, r.MaxZ)
		}
	}
	return nil
}
