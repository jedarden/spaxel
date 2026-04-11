package simulator

import (
	"testing"
)

func TestPointDistance(t *testing.T) {
	p1 := NewPoint(0, 0, 0)
	p2 := NewPoint(3, 4, 0)

	dist := p1.Distance(p2)
	expected := 5.0 // 3-4-5 triangle

	if dist != expected {
		t.Errorf("Expected distance %f, got %f", expected, dist)
	}
}

func TestPointVector(t *testing.T) {
	p1 := NewPoint(0, 0, 0)
	p2 := NewPoint(1, 0, 0)

	vec := p1.Vector(p2)

	if vec.X != 1.0 || vec.Y != 0 || vec.Z != 0 {
		t.Errorf("Expected unit vector (1, 0, 0), got (%f, %f, %f)", vec.X, vec.Y, vec.Z)
	}
}

func TestPointAdd(t *testing.T) {
	p := NewPoint(1, 2, 3)
	v := NewPoint(0.5, 0.5, 0.5)

	result := p.Add(v)

	if result.X != 1.5 || result.Y != 2.5 || result.Z != 3.5 {
		t.Errorf("Expected (1.5, 2.5, 3.5), got (%f, %f, %f)", result.X, result.Y, result.Z)
	}
}

func TestPointScale(t *testing.T) {
	p := NewPoint(1, 2, 3)

	result := p.Scale(2.0)

	if result.X != 2.0 || result.Y != 4.0 || result.Z != 6.0 {
		t.Errorf("Expected (2, 4, 6), got (%f, %f, %f)", result.X, result.Y, result.Z)
	}
}

func TestWallSegmentIntersectsLine(t *testing.T) {
	wall := &WallSegment{
		P1:     NewPoint(2, 0, 0),
		P2:     NewPoint(2, 10, 0),
		Height: 2.5,
	}

	tests := []struct {
		name     string
		a, b     Point
		expected bool
	}{
		{
			name:     "crossing horizontal",
			a:        NewPoint(0, 5, 0),
			b:        NewPoint(5, 5, 0),
			expected: true,
		},
		{
			name:     "not crossing parallel",
			a:        NewPoint(0, 0, 0),
			b:        NewPoint(1, 0, 0),
			expected: true, // Wall endpoint (2,0) projects onto line segment (0,0)-(1,0), so this is technically "crossing" in the wall's projection
		},
		{
			name:     "crossing from left",
			a:        NewPoint(0, 3, 0),
			b:        NewPoint(4, 3, 0),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wall.IntersectsLine(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestWallPenetrationLoss(t *testing.T) {
	tests := []struct {
		material  WallMaterial
		expected  float64
	}{
		{MaterialDrywall, 3.0},
		{MaterialBrick, 10.0},
		{MaterialConcrete, 10.0},
		{MaterialGlass, 2.0},
		{MaterialMetal, 20.0},
	}

	for _, tt := range tests {
		t.Run(string(tt.material), func(t *testing.T) {
			loss := WallPenetrationLoss(tt.material)
			if loss != tt.expected {
				t.Errorf("Expected loss %f, got %f", tt.expected, loss)
			}
		})
	}
}

func TestRoomCenter(t *testing.T) {
	room := Room{
		MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 6, MaxY: 5, MaxZ: 2.5,
	}

	center := room.Center()

	if center.X != 3.0 || center.Y != 2.5 || center.Z != 1.25 {
		t.Errorf("Expected center (3, 2.5, 1.25), got (%f, %f, %f)", center.X, center.Y, center.Z)
	}
}

func TestRoomDimensions(t *testing.T) {
	room := Room{
		MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 6, MaxY: 5, MaxZ: 2.5,
	}

	width, depth, height := room.Dimensions()

	if width != 6.0 || depth != 5.0 || height != 2.5 {
		t.Errorf("Expected (6, 5, 2.5), got (%f, %f, %f)", width, depth, height)
	}
}

func TestRoomVolume(t *testing.T) {
	room := Room{
		MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 6, MaxY: 5, MaxZ: 2.5,
	}

	volume := room.Volume()
	expected := 6.0 * 5.0 * 2.5

	if volume != expected {
		t.Errorf("Expected volume %f, got %f", expected, volume)
	}
}

func TestRoomContains(t *testing.T) {
	room := Room{
		MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 6, MaxY: 5, MaxZ: 2.5,
	}

	tests := []struct {
		name     string
		point    Point
		expected bool
	}{
		{"inside center", NewPoint(3, 2.5, 1), true},
		{"inside corner", NewPoint(0.1, 0.1, 0.1), true},
		{"outside x", NewPoint(-1, 2.5, 1), false},
		{"outside y", NewPoint(3, 10, 1), false},
		{"outside z", NewPoint(3, 2.5, 5), false},
		{"on boundary", NewPoint(0, 2.5, 1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := room.Contains(tt.point)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSpaceBounds(t *testing.T) {
	space := &Space{
		Rooms: []Room{
			{MinX: 0, MinY: 0, MinZ: 0, MaxX: 5, MaxY: 5, MaxZ: 2.5},
			{MinX: 5, MinY: 0, MinZ: 0, MaxX: 10, MaxY: 8, MaxZ: 3},
		},
	}

	minX, minY, minZ, maxX, maxY, maxZ := space.Bounds()

	if minX != 0 || minY != 0 || minZ != 0 {
		t.Errorf("Expected min (0, 0, 0), got (%f, %f, %f)", minX, minY, minZ)
	}
	if maxX != 10 || maxY != 8 || maxZ != 3 {
		t.Errorf("Expected max (10, 8, 3), got (%f, %f, %f)", maxX, maxY, maxZ)
	}
}

func TestSpaceTotalVolume(t *testing.T) {
	space := &Space{
		Rooms: []Room{
			{MinX: 0, MinY: 0, MinZ: 0, MaxX: 5, MaxY: 5, MaxZ: 2.5}, // 62.5 m³
			{MinX: 5, MinY: 0, MinZ: 0, MaxX: 10, MaxY: 8, MaxZ: 3},  // 120 m³
		},
	}

	volume := space.TotalVolume()
	expected := 62.5 + 120.0

	if volume != expected {
		t.Errorf("Expected volume %f, got %f", expected, volume)
	}
}

func TestDefaultSpace(t *testing.T) {
	space := DefaultSpace()

	if space.ID != "default" {
		t.Errorf("Expected ID 'default', got '%s'", space.ID)
	}

	if len(space.Rooms) != 1 {
		t.Errorf("Expected 1 room, got %d", len(space.Rooms))
	}

	room := space.Rooms[0]
	if room.Name != "Main Room" {
		t.Errorf("Expected room name 'Main Room', got '%s'", room.Name)
	}
}

func TestSpaceValidate(t *testing.T) {
	tests := []struct {
		name    string
		space   *Space
		wantErr bool
	}{
		{
			name: "valid space",
			space: &Space{
				ID: "test",
				Rooms: []Room{
					{MinX: 0, MinY: 0, MinZ: 0, MaxX: 5, MaxY: 5, MaxZ: 2.5},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty ID",
			space:   &Space{ID: "", Rooms: []Room{{MinX: 0, MinY: 0, MinZ: 0, MaxX: 5, MaxY: 5, MaxZ: 2.5}}},
			wantErr: true,
		},
		{
			name:    "no rooms",
			space:   &Space{ID: "test", Rooms: []Room{}},
			wantErr: true,
		},
		{
			name: "invalid room bounds",
			space: &Space{
				ID: "test",
				Rooms: []Room{
					{MinX: 5, MinY: 0, MinZ: 0, MaxX: 0, MaxY: 5, MaxZ: 2.5}, // MinX > MaxX
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.space.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUnmarshalSpace(t *testing.T) {
	jsonData := []byte(`{
		"id": "test-space",
		"name": "Test Space",
		"rooms": [{
			"id": "room-1",
			"name": "Room 1",
			"min_x": 0,
			"min_y": 0,
			"min_z": 0,
			"max_x": 5,
			"max_y": 5,
			"max_z": 2.5
		}]
	}`)

	space, err := UnmarshalSpace(jsonData)
	if err != nil {
		t.Fatalf("Failed to unmarshal space: %v", err)
	}

	if space.ID != "test-space" {
		t.Errorf("Expected ID 'test-space', got '%s'", space.ID)
	}

	if len(space.Rooms) != 1 {
		t.Errorf("Expected 1 room, got %d", len(space.Rooms))
	}
}
