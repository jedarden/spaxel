// Package volume provides tests for trigger volume geometry and point-in-volume testing.
package volume

import (
	"testing"
	"time"
)

// TestShapeJSON_BoxInside tests point-inside-box detection.
func TestShapeJSON_BoxInside(t *testing.T) {
	shape := ShapeJSON{
		Type: ShapeBox,
		X:    float64Ptr(0),
		Y:    float64Ptr(0),
		Z:    float64Ptr(0),
		W:    float64Ptr(2),
		D:    float64Ptr(2),
		H:    float64Ptr(1),
	}

	tests := []struct {
		name     string
		point    Point3D
		expected bool
	}{
		{"origin inside", Point3D{0, 0, 0}, true},
		{"center inside", Point3D{1, 0.5, 1}, true},
		{"corner inside", Point3D{1.999, 0.999, 1.999}, true},
		{"outside x-", Point3D{-0.001, 0.5, 1}, false},
		{"outside x+", Point3D{2.001, 0.5, 1}, false},
		{"outside y-", Point3D{1, -0.001, 1}, false},
		{"outside y+", Point3D{1, 1.001, 1}, false},
		{"outside z-", Point3D{1, 0.5, -0.001}, false},
		{"outside z+", Point3D{1, 0.5, 2.001}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shape.IsInside(tt.point)
			if got != tt.expected {
				t.Errorf("IsInside(%v) = %v, want %v", tt.point, got, tt.expected)
			}
		})
	}
}

// TestShapeJSON_CylinderInside tests point-inside-cylinder detection.
func TestShapeJSON_CylinderInside(t *testing.T) {
	shape := ShapeJSON{
		Type: ShapeCylinder,
		CX:   float64Ptr(0),
		CY:   float64Ptr(0),
		Z:    float64Ptr(0),
		R:    float64Ptr(1),
		H:    float64Ptr(2),
	}

	tests := []struct {
		name     string
		point    Point3D
		expected bool
	}{
		{"center bottom", Point3D{0, 0, 0}, true},
		{"center top", Point3D{0, 0, 1.999}, true},
		{"edge inside", Point3D{0.999, 0, 1}, true},
		{"inside at height", Point3D{0, 0, 1}, true},
		{"outside radius", Point3D{1.001, 0, 1}, false},
		{"outside height-", Point3D{0, 0, -0.001}, false},
		{"outside height+", Point3D{0, 0, 2.001}, false},
		{"diagonal outside", Point3D{0.8, 0.8, 1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shape.IsInside(tt.point)
			if got != tt.expected {
				t.Errorf("IsInside(%v) = %v, want %v", tt.point, got, tt.expected)
			}
		})
	}
}

// TestShapeJSON_InvalidShape tests that invalid shapes return false.
func TestShapeJSON_InvalidShape(t *testing.T) {
	tests := []struct {
		name  string
		shape ShapeJSON
		point Point3D
	}{
		{
			"missing box fields",
			ShapeJSON{Type: ShapeBox, X: float64Ptr(0)},
			Point3D{0, 0, 0},
		},
		{
			"missing cylinder fields",
			ShapeJSON{Type: ShapeCylinder, CX: float64Ptr(0)},
			Point3D{0, 0, 0},
		},
		{
			"unknown type",
			ShapeJSON{Type: "unknown"},
			Point3D{0, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.shape.IsInside(tt.point)
			if got {
				t.Errorf("IsInside(%v) = true, want false for invalid shape", tt.point)
			}
		})
	}
}

// TestStore_EvaluateEnter tests enter trigger evaluation.
func TestStore_EvaluateEnter(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test enter",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0),
			Y:    float64Ptr(0),
			Z:    float64Ptr(0),
			W:    float64Ptr(1),
			D:    float64Ptr(1),
			H:    float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Blob outside - should not fire
	blobsOutside := []BlobPos{{ID: 1, X: 2, Y: 2, Z: 2}}
	fired := store.Evaluate(blobsOutside, time.Now())
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings, got %d", len(fired))
	}

	// Blob enters - should fire once
	blobsEnter := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	fired = store.Evaluate(blobsEnter, time.Now())
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing, got %d", len(fired))
	}
	if fired[0] != id {
		t.Errorf("Expected trigger %s to fire, got %s", id, fired[0])
	}

	// Blob still inside - should not fire again
	fired = store.Evaluate(blobsEnter, time.Now())
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (blob already inside), got %d", len(fired))
	}

	// Blob leaves and re-enters - should fire again
	blobsOutside = []BlobPos{{ID: 1, X: 2, Y: 2, Z: 2}}
	store.Evaluate(blobsOutside, time.Now())
	blobsEnter = []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	fired = store.Evaluate(blobsEnter, time.Now())
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing (re-entry), got %d", len(fired))
	}
}

// TestStore_EvaluateLeave tests leave trigger evaluation.
func TestStore_EvaluateLeave(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test leave",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0),
			Y:    float64Ptr(0),
			Z:    float64Ptr(0),
			W:    float64Ptr(1),
			D:    float64Ptr(1),
			H:    float64Ptr(1),
		},
		Condition: "leave",
		Enabled:  true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Blob inside first
	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	store.Evaluate(blobsInside, time.Now())

	// Blob leaves - should fire
	blobsOutside := []BlobPos{{ID: 1, X: 2, Y: 2, Z: 2}}
	fired := store.Evaluate(blobsOutside, time.Now())
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing, got %d", len(fired))
	}
	if fired[0] != id {
		t.Errorf("Expected trigger %s to fire, got %s", id, fired[0])
	}

	// Blob still outside - should not fire again
	fired = store.Evaluate(blobsOutside, time.Now())
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (blob already outside), got %d", len(fired))
	}
}

// TestStore_EvaluateDwell tests dwell trigger evaluation.
// Per spec: fires once per entry; re-fires after blob leaves and re-enters.
func TestStore_EvaluateDwell(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	durationSec := 1
	trigger := &Trigger{
		Name: "test dwell",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0),
			Y:    float64Ptr(0),
			Z:    float64Ptr(0),
			W:    float64Ptr(1),
			D:    float64Ptr(1),
			H:    float64Ptr(1),
		},
		Condition: "dwell",
		ConditionParams: ConditionParams{
			DurationS: &durationSec,
		},
		Enabled: true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	blobsOutside := []BlobPos{{ID: 1, X: 5, Y: 5, Z: 5}}
	now := time.Now()

	// First evaluation - blob enters, no fire yet
	fired := store.Evaluate(blobsInside, now)
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (just entered), got %d", len(fired))
	}

	// Before dwell threshold - no fire
	fired = store.Evaluate(blobsInside, now.Add(500*time.Millisecond))
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (before threshold), got %d", len(fired))
	}

	// After dwell threshold - should fire
	fired = store.Evaluate(blobsInside, now.Add(time.Duration(durationSec)*time.Second))
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing (after threshold), got %d", len(fired))
	}
	if fired[0] != id {
		t.Errorf("Expected trigger %s to fire, got %s", id, fired[0])
	}

	// Still inside after fire - should NOT fire again (must exit first)
	fired = store.Evaluate(blobsInside, now.Add(time.Duration(durationSec)*time.Second+5*time.Second))
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (still inside, must exit first), got %d", len(fired))
	}

	// Blob leaves and stays outside for a bit
	fired = store.Evaluate(blobsOutside, now.Add(time.Duration(durationSec)*time.Second+6*time.Second))
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (blob outside), got %d", len(fired))
	}

	// Blob re-enters and stays for duration threshold - should fire again
	reEntry := now.Add(time.Duration(durationSec)*time.Second+10*time.Second)
	store.Evaluate(blobsInside, reEntry) // enters
	fired = store.Evaluate(blobsInside, reEntry.Add(time.Duration(durationSec)*time.Second))
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing (re-entry after dwell), got %d", len(fired))
	}
}

// TestStore_EvaluateDwell_Accuracy tests that dwell fires at correct time ±1s.
func TestStore_EvaluateDwell_Accuracy(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	durationSec := 3
	trigger := &Trigger{
		Name: "test dwell accuracy",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "dwell",
		ConditionParams: ConditionParams{
			DurationS: &durationSec,
		},
		Enabled: true,
	}

	_, err = store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	now := time.Now()

	// Blob enters
	store.Evaluate(blobsInside, now)

	// Check every 100ms around the threshold
	var firedAt time.Time
	for offset := 0; offset <= 4000; offset += 100 {
		checkTime := now.Add(time.Duration(offset) * time.Millisecond)
		fired := store.Evaluate(blobsInside, checkTime)
		if len(fired) > 0 {
			firedAt = checkTime
			break
		}
	}

	if firedAt.IsZero() {
		t.Fatal("Expected trigger to fire within dwell duration")
	}

	actualDuration := firedAt.Sub(now)
	// Should fire within durationSec ± 200ms (100ms evaluation granularity)
	if actualDuration < time.Duration(durationSec-1)*time.Second || actualDuration > time.Duration(durationSec+1)*time.Second {
		t.Errorf("Dwell fired at %v, expected ~%v ± 1s", actualDuration, time.Duration(durationSec)*time.Second)
	}
}

// TestStore_EvaluateDwell_Cylinder tests dwell with a cylinder volume.
func TestStore_EvaluateDwell_Cylinder(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	durationSec := 1
	trigger := &Trigger{
		Name: "test dwell cylinder",
		Shape: ShapeJSON{
			Type: ShapeCylinder,
			CX:   float64Ptr(0),
			CY:   float64Ptr(0),
			Z:    float64Ptr(0),
			R:    float64Ptr(1),
			H:    float64Ptr(2),
		},
		Condition: "dwell",
		ConditionParams: ConditionParams{
			DurationS: &durationSec,
		},
		Enabled: true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	// Blob at center of cylinder - enters
	blobsInside := []BlobPos{{ID: 1, X: 0, Y: 0, Z: 1.0}}
	store.Evaluate(blobsInside, now)

	// After duration - should fire
	fired := store.Evaluate(blobsInside, now.Add(time.Duration(durationSec)*time.Second))
	if len(fired) != 1 || fired[0] != id {
		t.Errorf("Expected trigger %s to fire after dwell in cylinder, got %v", id, fired)
	}
}

// TestStore_EvaluateLeave_BlobDisappears tests that leave fires when a tracked blob vanishes.
func TestStore_EvaluateLeave_BlobDisappears(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test leave disappear",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X: float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "leave",
		Enabled: true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	// Blob enters
	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	store.Evaluate(blobsInside, now)

	// Blob disappears (not in the blobs list at all) - should fire leave
	blobsEmpty := []BlobPos{}
	fired := store.Evaluate(blobsEmpty, now.Add(1*time.Second))
	if len(fired) != 1 || fired[0] != id {
		t.Errorf("Expected trigger %s to fire when blob disappears, got %v", id, fired)
	}
}

// TestStore_EvaluateVacant_Cancelled tests that vacant timer resets if blob returns.
func TestStore_EvaluateVacant_Cancelled(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	durationSec := 2
	trigger := &Trigger{
		Name: "test vacant cancelled",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X: float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "vacant",
		ConditionParams: ConditionParams{
			DurationS: &durationSec,
		},
		Enabled: true,
	}

	_, err = store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	// Blob inside - no fire
	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	store.Evaluate(blobsInside, now)

	// Blob leaves - starts vacant timer
	blobsOutside := []BlobPos{{ID: 1, X: 5, Y: 5, Z: 5}}
	store.Evaluate(blobsOutside, now.Add(1*time.Second))

	// Before threshold - no fire
	fired := store.Evaluate(blobsOutside, now.Add(1500*time.Millisecond))
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (before threshold), got %d", len(fired))
	}

	// Blob returns before threshold - should cancel timer
	store.Evaluate(blobsInside, now.Add(1800*time.Millisecond))

	// Wait past original threshold - should NOT fire (timer was cancelled)
	fired = store.Evaluate(blobsInside, now.Add(3*time.Second))
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (timer cancelled by blob return), got %d", len(fired))
	}
}

// TestStore_MultipleBlobs tests trigger evaluation with multiple blobs.
func TestStore_MultipleBlobs(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test multi-blob enter",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X: float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(4), D: float64Ptr(4), H: float64Ptr(2),
		},
		Condition: "enter",
		Enabled: true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	// Two blobs enter simultaneously
	blobs := []BlobPos{
		{ID: 1, X: 1.0, Y: 1.0, Z: 1.0},
		{ID: 2, X: 2.0, Y: 2.0, Z: 1.0},
	}
	fired := store.Evaluate(blobs, now)
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing (two blobs entering), got %d", len(fired))
	}
	if fired[0] != id {
		t.Errorf("Expected trigger %s to fire, got %s", id, fired[0])
	}

	// Same blobs still inside - no fire
	fired = store.Evaluate(blobs, now.Add(1*time.Second))
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (blobs still inside), got %d", len(fired))
	}

	// One blob leaves, one enters - fires for the entering blob
	blobs2 := []BlobPos{
		{ID: 2, X: 2.0, Y: 2.0, Z: 1.0}, // still inside
		{ID: 3, X: 1.5, Y: 1.5, Z: 1.0}, // new blob entering
	}
	fired = store.Evaluate(blobs2, now.Add(2*time.Second))
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing (new blob entering), got %d", len(fired))
	}
}

// TestStore_Cylinder_MultiplePoints tests cylinder volume with many points.
func TestStore_Cylinder_MultiplePoints(t *testing.T) {
	shape := ShapeJSON{
		Type: ShapeCylinder,
		CX:   float64Ptr(5),
		CY:   float64Ptr(5),
		Z:    float64Ptr(0),
		R:    float64Ptr(2),
		H:    float64Ptr(3),
	}

	tests := []struct {
		name     string
		point    Point3D
		expected bool
	}{
		{"center base", Point3D{5, 5, 0}, true},
		{"center mid-height", Point3D{5, 5, 1.5}, true},
		{"center top", Point3D{5, 5, 2.999}, true},
		{"on radius", Point3D{7, 5, 1}, true},
		{"on radius diagonal", Point3D{5, 7, 1}, true},
		{"just outside radius", Point3D{7.001, 5, 1}, false},
		{"outside radius", Point3D{8, 5, 1}, false},
		{"above top", Point3D{5, 5, 3.001}, false},
		{"below bottom", Point3D{5, 5, -0.001}, false},
		{"far away", Point3D{100, 100, 100}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shape.IsInside(tt.point)
			if got != tt.expected {
				t.Errorf("IsInside(%v) = %v, want %v", tt.point, got, tt.expected)
			}
		})
	}
}

// TestStore_Box_Edges tests boundary conditions for box volume.
func TestStore_Box_Edges(t *testing.T) {
	shape := ShapeJSON{
		Type: ShapeBox,
		X:    float64Ptr(1),
		Y:    float64Ptr(2),
		Z:    float64Ptr(3),
		W:    float64Ptr(4),
		D:    float64Ptr(5),
		H:    float64Ptr(6),
	}
	// Box spans: x=[1,5), y=[2,8), z=[3,8)

	// All 6 faces — on the low edge should be inside, just outside should not
	tests := []struct {
		name     string
		point    Point3D
		expected bool
	}{
		// X faces
		{"x_min inside", Point3D{1, 4, 6}, true},
		{"x_min outside", Point3D{0.999, 4, 6}, false},
		{"x_max inside", Point3D{4.999, 4, 6}, true},
		{"x_max outside", Point3D{5, 4, 6}, false},
		// Y faces
		{"y_min inside", Point3D{3, 2, 6}, true},
		{"y_min outside", Point3D{3, 1.999, 6}, false},
		{"y_max inside", Point3D{3, 7.999, 6}, true},
		{"y_max outside", Point3D{3, 8, 6}, false},
		// Z faces
		{"z_min inside", Point3D{3, 4, 3}, true},
		{"z_min outside", Point3D{3, 4, 2.999}, false},
		{"z_max inside", Point3D{3, 4, 7.999}, true},
		{"z_max outside", Point3D{3, 4, 8}, false},
		// Exact center
		{"center", Point3D{3, 4.5, 6}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shape.IsInside(tt.point)
			if got != tt.expected {
				t.Errorf("IsInside(%v) = %v, want %v", tt.point, got, tt.expected)
			}
		})
	}
}

// TestStore_Cylinder_Edges tests boundary conditions for cylinder volume.
func TestStore_Cylinder_Edges(t *testing.T) {
	shape := ShapeJSON{
		Type: ShapeCylinder,
		CX:   float64Ptr(10),
		CY:   float64Ptr(10),
		Z:    float64Ptr(5),
		R:    float64Ptr(3),
		H:    float64Ptr(4),
	}
	// Cylinder: center (10,10), z=[5,9), r=3

	tests := []struct {
		name     string
		point    Point3D
		expected bool
	}{
		// On boundary
		{"on radius edge", Point3D{13, 10, 7}, true},
		{"outside radius", Point3D{13.001, 10, 7}, false},
		{"on base edge", Point3D{10, 10, 5}, true},
		{"below base", Point3D{10, 10, 4.999}, false},
		{"below top", Point3D{10, 10, 8.999}, true},
		{"above top", Point3D{10, 10, 9}, false},
		{"center", Point3D{10, 10, 7}, true},
		{"opposite edge", Point3D{7, 10, 7}, true},
		{"opposite outside", Point3D{6.999, 10, 7}, false}, // sqrt(3.001^2) = 3.001... distSq = 9.006 > 9 (r^2)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shape.IsInside(tt.point)
			if got != tt.expected {
				t.Errorf("IsInside(%v) = %v, want %v", tt.point, got, tt.expected)
			}
		})
	}
}

// TestStore_NilDurationParams tests triggers with nil duration params.
func TestStore_NilDurationParams(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Dwell with no duration — should not fire
	dwellTrigger := &Trigger{
		Name: "dwell no duration",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X: float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "dwell",
		Enabled:  true,
	}

	_, err = store.Create(dwellTrigger)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	fired := store.Evaluate(blobsInside, now.Add(10*time.Second))
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings for dwell with no duration, got %d", len(fired))
	}

	// Count with no threshold — should not fire
	countTrigger := &Trigger{
		Name: "count no threshold",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X: float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "count",
		Enabled:  true,
	}

	_, err = store.Create(countTrigger)
	if err != nil {
		t.Fatal(err)
	}

	fired = store.Evaluate(blobsInside, now)
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings for count with no threshold, got %d", len(fired))
	}
}

// TestStore_DisabledTrigger tests that disabled triggers are not evaluated.
func TestStore_DisabledTrigger(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "disabled trigger",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X: float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  false,
	}

	_, err = store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	fired := store.Evaluate(blobsInside, time.Now())
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings for disabled trigger, got %d", len(fired))
	}
}

// TestStore_FiringCallback tests that the firing callback is invoked correctly.
func TestStore_FiringCallback(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var receivedEvents []FiredEvent
	store.SetOnFired(func(event FiredEvent) {
		receivedEvents = append(receivedEvents, event)
	})

	trigger := &Trigger{
		Name: "callback test",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X: float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	blobsOutside := []BlobPos{{ID: 1, X: 5, Y: 5, Z: 5}}
	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}

	// Enter — should invoke callback
	store.Evaluate(blobsOutside, now)
	store.Evaluate(blobsInside, now.Add(100*time.Millisecond))

	if len(receivedEvents) != 1 {
		t.Fatalf("Expected 1 callback event, got %d", len(receivedEvents))
	}

	evt := receivedEvents[0]
	if evt.TriggerID != id {
		t.Errorf("Expected trigger ID %s, got %s", id, evt.TriggerID)
	}
	if evt.TriggerName != "callback test" {
		t.Errorf("Expected trigger name 'callback test', got %s", evt.TriggerName)
	}
	if evt.Condition != "enter" {
		t.Errorf("Expected condition 'enter', got %s", evt.Condition)
	}
	if len(evt.BlobIDs) == 0 {
		t.Error("Expected at least one blob ID in event")
	}
}

// TestStore_BlobVolumeTracking tests that blobVolumes tracks which trigger contains a blob.
func TestStore_BlobVolumeTracking(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	box1 := &Trigger{
		Name: "box1",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X: float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	box2 := &Trigger{
		Name: "box2",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X: float64Ptr(5), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	id1, _ := store.Create(box1)
	id2, _ := store.Create(box2)

	now := time.Now()

	// Blob in box1
	blobsInBox1 := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	store.Evaluate(blobsInBox1, now)

	// Check IsInVolume
	if !store.IsInVolume(id1, 0.5, 0.5, 0.5) {
		t.Error("Expected blob at (0.5, 0.5, 0.5) to be in box1")
	}
	if store.IsInVolume(id2, 0.5, 0.5, 0.5) {
		t.Error("Expected blob at (0.5, 0.5, 0.5) to NOT be in box2")
	}

	// Blob moves to box2
	blobsInBox2 := []BlobPos{{ID: 1, X: 5.5, Y: 0.5, Z: 0.5}}
	store.Evaluate(blobsInBox2, now.Add(1*time.Second))

	if store.IsInVolume(id1, 5.5, 0.5, 0.5) {
		t.Error("Expected blob at (5.5, 0.5, 0.5) to NOT be in box1")
	}
	if !store.IsInVolume(id2, 5.5, 0.5, 0.5) {
		t.Error("Expected blob at (5.5, 0.5, 0.5) to be in box2")
	}
}

// TestStore_EvaluateCount tests count trigger evaluation.
func TestStore_EvaluateCount(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	threshold := 2
	trigger := &Trigger{
		Name: "test count",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0),
			Y:    float64Ptr(0),
			Z:    float64Ptr(0),
			W:    float64Ptr(2),
			D:    float64Ptr(2),
			H:    float64Ptr(1),
		},
		Condition: "count",
		ConditionParams: ConditionParams{
			CountThreshold: &threshold,
		},
		Enabled: true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// One blob inside - no fire
	blobs := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	fired := store.Evaluate(blobs, time.Now())
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (count 1 < threshold 2), got %d", len(fired))
	}

	// Two blobs inside - should fire
	blobs = []BlobPos{
		{ID: 1, X: 0.5, Y: 0.5, Z: 0.5},
		{ID: 2, X: 1.0, Y: 0.5, Z: 1.0},
	}
	fired = store.Evaluate(blobs, time.Now())
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing (count 2 >= threshold 2), got %d", len(fired))
	}
	if fired[0] != id {
		t.Errorf("Expected trigger %s to fire, got %s", id, fired[0])
	}

	// Still two blobs - should not fire again (cooldown)
	fired = store.Evaluate(blobs, time.Now().Add(6*time.Second))
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (cooldown), got %d", len(fired))
	}
}

// TestStore_EvaluateVacant tests vacant trigger evaluation.
func TestStore_EvaluateVacant(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	durationSec := 1
	trigger := &Trigger{
		Name: "test vacant",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0),
			Y:    float64Ptr(0),
			Z:    float64Ptr(0),
			W:    float64Ptr(1),
			D:    float64Ptr(1),
			H:    float64Ptr(1),
		},
		Condition: "vacant",
		ConditionParams: ConditionParams{
			DurationS: &durationSec,
		},
		Enabled: true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Blob inside - no fire
	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	now := time.Now()
	fired := store.Evaluate(blobsInside, now)
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (blob inside), got %d", len(fired))
	}

	// Blob leaves - starts vacant timer
	blobsOutside := []BlobPos{{ID: 1, X: 2, Y: 2, Z: 2}}
	fired = store.Evaluate(blobsOutside, now)
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (just left), got %d", len(fired))
	}

	// Before threshold - no fire
	fired = store.Evaluate(blobsOutside, now.Add(500*time.Millisecond))
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (before threshold), got %d", len(fired))
	}

	// After threshold - should fire
	fired = store.Evaluate(blobsOutside, now.Add(time.Duration(durationSec)*time.Second))
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing (after threshold), got %d", len(fired))
	}
	if fired[0] != id {
		t.Errorf("Expected trigger %s to fire, got %s", id, fired[0])
	}

	// Blob returns - resets vacant timer, no fire
	blobsInside = []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	fired = store.Evaluate(blobsInside, now.Add(time.Duration(durationSec)*time.Second+100*time.Millisecond))
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (blob returned), got %d", len(fired))
	}
}

// TestStore_CRUD tests basic CRUD operations.
func TestStore_CRUD(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create
	trigger := &Trigger{
		Name: "test trigger",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0),
			Y:    float64Ptr(0),
			Z:    float64Ptr(0),
			W:    float64Ptr(1),
			D:    float64Ptr(1),
			H:    float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []Action{
			{Type: "webhook", Params: map[string]interface{}{"url": "http://example.com"}},
		},
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if id == "" {
		t.Fatal("Expected non-empty ID")
	}

	// Get
	retrieved, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Name != trigger.Name {
		t.Errorf("Expected name %s, got %s", trigger.Name, retrieved.Name)
	}

	// Update
	retrieved.Name = "updated name"
	err = store.Update(retrieved)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	retrieved, _ = store.Get(id)
	if retrieved.Name != "updated name" {
		t.Errorf("Expected updated name, got %s", retrieved.Name)
	}

	// GetAll
	all := store.GetAll()
	if len(all) != 1 {
		t.Errorf("Expected 1 trigger, got %d", len(all))
	}

	// Delete
	err = store.Delete(id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(id)
	if err == nil {
		t.Error("Expected error when getting deleted trigger")
	}

	all = store.GetAll()
	if len(all) != 0 {
		t.Errorf("Expected 0 triggers after delete, got %d", len(all))
	}
}

// TestStore_TimeConstraint tests time constraint filtering.
func TestStore_TimeConstraint(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test time constraint",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0),
			Y:    float64Ptr(0),
			Z:    float64Ptr(0),
			W:    float64Ptr(1),
			D:    float64Ptr(1),
			H:    float64Ptr(1),
		},
		Condition: "enter",
		TimeConstraint: &TimeConstraint{
			From: "09:00",
			To:   "17:00",
		},
		Enabled: true,
	}

	_, err = store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	blobsOutside := []BlobPos{{ID: 1, X: 2, Y: 2, Z: 2}}
	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}

	// Before time window - blob enters but no fire due to time constraint
	beforeTime := time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC)
	store.Evaluate(blobsOutside, beforeTime) // Blob outside
	fired := store.Evaluate(blobsInside, beforeTime) // Blob enters
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (before time window), got %d", len(fired))
	}

	// During time window - blob enters and should fire
	duringTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	// First reset by moving outside
	store.Evaluate(blobsOutside, duringTime)
	// Then enter during the time window
	fired = store.Evaluate(blobsInside, duringTime)
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing (during time window), got %d", len(fired))
	}

	// After time window - no fire
	afterTime := time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC)
	store.Evaluate(blobsOutside, afterTime)
	fired = store.Evaluate(blobsInside, afterTime)
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (after time window), got %d", len(fired))
	}

	// Overnight window (22:00-07:00)
	trigger.TimeConstraint = &TimeConstraint{
		From: "22:00",
		To:   "07:00",
	}
	err = store.Update(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Should fire at 23:00
	nightTime := time.Date(2024, 1, 1, 23, 0, 0, 0, time.UTC)
	store.Evaluate(blobsOutside, nightTime)
	fired = store.Evaluate(blobsInside, nightTime)
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing (overnight window), got %d", len(fired))
	}

	// Should fire at 03:00
	earlyMorning := time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC)
	store.Evaluate(blobsOutside, earlyMorning)
	fired = store.Evaluate(blobsInside, earlyMorning)
	if len(fired) != 1 {
		t.Errorf("Expected 1 firing (early morning), got %d", len(fired))
	}

	// Should not fire at 12:00
	dayTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	store.Evaluate(blobsOutside, dayTime)
	fired = store.Evaluate(blobsInside, dayTime)
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings (outside overnight window), got %d", len(fired))
	}
}

// TestStore_ErrorCountManagement tests error count increment, reset, and trigger disable on 4xx.
func TestStore_ErrorCountManagement(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test error count",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []Action{
			{Type: "webhook", Params: map[string]interface{}{"url": "http://example.com"}},
		},
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Initially error_count should be 0
	tg, err := store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if tg.ErrorCount != 0 {
		t.Errorf("Expected initial error_count 0, got %d", tg.ErrorCount)
	}

	// Increment error count (simulating 5xx/timeout)
	store.IncrementErrorCount(id)
	store.IncrementErrorCount(id)
	store.IncrementErrorCount(id)

	tg, _ = store.Get(id)
	if tg.ErrorCount != 3 {
		t.Errorf("Expected error_count 3, got %d", tg.ErrorCount)
	}
	// Trigger should still be enabled after 5xx errors
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled after 5xx errors")
	}

	// Reset error count (simulating successful 2xx response)
	store.ResetErrorCount(id)

	tg, _ = store.Get(id)
	if tg.ErrorCount != 0 {
		t.Errorf("Expected error_count 0 after reset, got %d", tg.ErrorCount)
	}
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled after error count reset")
	}
}

// TestStore_DisableTriggerWithError tests that a 4xx response disables the trigger.
func TestStore_DisableTriggerWithError(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test 4xx disable",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate 4xx response disabling the trigger
	errMsg := "Webhook returned HTTP 404 — trigger disabled. Fix the URL and re-enable."
	store.DisableTriggerWithError(id, errMsg)

	tg, err := store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if tg.Enabled {
		t.Error("Expected trigger to be disabled after 4xx response")
	}
	if tg.ErrorMessage != errMsg {
		t.Errorf("Expected error_message %q, got %q", errMsg, tg.ErrorMessage)
	}

	// Verify the trigger is still disabled even after error count increments
	store.IncrementErrorCount(id)
	tg, _ = store.Get(id)
	if tg.Enabled {
		t.Error("Expected trigger to remain disabled")
	}
}

// TestStore_EnableTriggerClearsErrorState tests re-enabling clears error_message and error_count.
func TestStore_EnableTriggerClearsErrorState(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test re-enable",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate 4xx disable
	store.DisableTriggerWithError(id, "HTTP 400 error")
	store.IncrementErrorCount(id)

	// Re-enable via API
	err = store.EnableTrigger(id)
	if err != nil {
		t.Fatal(err)
	}

	tg, _ := store.Get(id)
	if !tg.Enabled {
		t.Error("Expected trigger to be re-enabled")
	}
	if tg.ErrorMessage != "" {
		t.Errorf("Expected error_message to be cleared, got %q", tg.ErrorMessage)
	}
	if tg.ErrorCount != 0 {
		t.Errorf("Expected error_count to be reset to 0, got %d", tg.ErrorCount)
	}
}

// TestStore_ErrorCountResetsOnFirst2xx tests that error_count resets on first 2xx.
func TestStore_ErrorCountResetsOnFirst2xx(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test reset on 2xx",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Accumulate errors
	for i := 0; i < 10; i++ {
		store.IncrementErrorCount(id)
	}
	tg, _ := store.Get(id)
	if tg.ErrorCount != 10 {
		t.Errorf("Expected error_count 10, got %d", tg.ErrorCount)
	}

	// Single success resets
	store.ResetErrorCount(id)
	tg, _ = store.Get(id)
	if tg.ErrorCount != 0 {
		t.Errorf("Expected error_count 0 after reset, got %d", tg.ErrorCount)
	}
}

// TestStore_WebhookLogAudit tests writing and reading webhook log entries.
func TestStore_WebhookLogAudit(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "audit trigger",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Write log entries
	store.WriteWebhookLog(id, "http://example.com/hook", 1700000000000, 200, 45, "")
	store.WriteWebhookLog(id, "http://example.com/hook", 1700000001000, 500, 120, "server error")
	store.WriteWebhookLog(id, "http://example.com/hook", 1700000002000, 404, 30, "not found")

	// Read back - should be in reverse chronological order
	entries := store.GetWebhookLog(id, 10)
	if len(entries) != 3 {
		t.Fatalf("Expected 3 webhook log entries, got %d", len(entries))
	}

	// Most recent first
	if entries[0].Status != 404 {
		t.Errorf("Expected first entry status 404, got %d", entries[0].Status)
	}
	if entries[0].LatencyMs != 30 {
		t.Errorf("Expected first entry latency 30ms, got %d", entries[0].LatencyMs)
	}
	if entries[0].Error != "not found" {
		t.Errorf("Expected first entry error 'not found', got %q", entries[0].Error)
	}

	if entries[1].Status != 500 {
		t.Errorf("Expected second entry status 500, got %d", entries[1].Status)
	}
	if entries[2].Status != 200 {
		t.Errorf("Expected third entry status 200, got %d", entries[2].Status)
	}

	// Test limit
	entries = store.GetWebhookLog(id, 2)
	if len(entries) != 2 {
		t.Errorf("Expected 2 entries with limit 2, got %d", len(entries))
	}
}

// TestStore_5xxDoesNotDisable tests that 5xx errors increment count but don't disable.
func TestStore_5xxDoesNotDisable(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test 5xx no disable",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate many 5xx errors
	for i := 0; i < 100; i++ {
		store.IncrementErrorCount(id)
	}

	tg, _ := store.Get(id)
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled despite 100 5xx errors")
	}
	if tg.ErrorCount != 100 {
		t.Errorf("Expected error_count 100, got %d", tg.ErrorCount)
	}
}

// TestStore_DisabledTriggerSkippedInEvaluate tests that disabled triggers are not evaluated.
func TestStore_DisabledTriggerSkippedInEvaluate(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	trigger := &Trigger{
		Name: "test disabled skip",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	id, err := store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Disable the trigger
	store.DisableTriggerWithError(id, "test error")

	// Blob enters volume — should NOT fire since trigger is disabled
	blobsInside := []BlobPos{{ID: 1, X: 0.5, Y: 0.5, Z: 0.5}}
	fired := store.Evaluate(blobsInside, time.Now())
	if len(fired) != 0 {
		t.Errorf("Expected 0 firings for disabled trigger, got %d", len(fired))
	}
}

// TestStore_ErrorStatePersistsAcrossRestart tests error_message and error_count survive reload.
func TestStore_ErrorStatePersistsAcrossRestart(t *testing.T) {
	// Use a temp file so we can reopen it
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	store1, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	trigger := &Trigger{
		Name: "persist test",
		Shape: ShapeJSON{
			Type: ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W: float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
	}

	id, err := store1.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Set error state
	store1.IncrementErrorCount(id)
	store1.IncrementErrorCount(id)
	store1.DisableTriggerWithError(id, "HTTP 403 forbidden")

	store1.Close()

	// Reopen store
	store2, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	tg, err := store2.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	if tg.ErrorCount != 2 {
		t.Errorf("Expected error_count 2 after restart, got %d", tg.ErrorCount)
	}
	if !tg.Enabled {
		// Note: DisableTriggerWithError sets Enabled=false, so after reload this should be false
		// But if columns are loaded correctly from DB, Enabled should be false
	}
	if tg.ErrorMessage != "HTTP 403 forbidden" {
		t.Errorf("Expected error_message 'HTTP 403 forbidden', got %q", tg.ErrorMessage)
	}

	// The trigger should be disabled after reload (persistence of enabled=false)
	if tg.Enabled {
		t.Error("Expected trigger to be disabled after restart (error state persisted)")
	}
}

// Helper function
func float64Ptr(f float64) *float64 {
	return &f
}
