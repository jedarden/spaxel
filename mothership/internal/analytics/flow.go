// Package analytics provides crowd flow visualization and analysis.
package analytics

import (
	"database/sql"
	"math"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	// GridCellSize is the size of each grid cell in metres (0.25m resolution)
	GridCellSize = 0.25
	// MinMovementThreshold is the minimum movement (in metres) to record a trajectory segment
	MinMovementThreshold = 0.2
	// StationarySpeedThreshold is the speed below which a track is considered stationary (m/s)
	StationarySpeedThreshold = 0.1
	// DefaultRetentionDays is the default retention period for trajectory data
	DefaultRetentionDays = 90
	// MinSegmentsForFlow is the minimum segments required to render a flow arrow
	MinSegmentsForFlow = 5
	// MinDwellSamples is the minimum dwell samples required to render a hotspot
	MinDwellSamples = 10
	// CorridorMinSegments is the minimum segments for a cell to be a corridor candidate
	CorridorMinSegments = 10
	// CorridorMaxAngularVariance is the maximum angular variance for corridor classification
	CorridorMaxAngularVariance = 0.3
)

// TrajectorySegment represents a single movement segment.
type TrajectorySegment struct {
	ID        string    `json:"id"`
	PersonID  string    `json:"person_id"`
	FromX     float64   `json:"from_x"`
	FromZ     float64   `json:"from_z"` // Ground plane (Y=0)
	ToX       float64   `json:"to_x"`
	ToZ       float64   `json:"to_z"`
	Speed     float64   `json:"speed"`
	Timestamp time.Time `json:"timestamp"`
}

// DwellAccumulatorKey identifies a dwell accumulator entry.
type DwellAccumulatorKey struct {
	GridX    int
	GridZ    int
	PersonID string
}

// DwellAccumulator represents accumulated dwell time at a location.
type DwellAccumulator struct {
	GridX       int       `json:"grid_x"`
	GridZ       int       `json:"grid_z"`
	PersonID    string    `json:"person_id"`
	Count       int       `json:"count"`
	LastUpdated time.Time `json:"last_updated"`
}

// DetectedCorridor represents a detected corridor region.
type DetectedCorridor struct {
	ID                string    `json:"id"`
	CentroidX         float64   `json:"centroid_x"`
	CentroidZ         float64   `json:"centroid_z"`
	DominantDirX      float64   `json:"dominant_dir_x"`
	DominantDirZ      float64   `json:"dominant_dir_z"`
	LengthM           float64   `json:"length_m"`
	WidthM            float64   `json:"width_m"`
	CellCount         int       `json:"cell_count"`
	LastComputed      time.Time `json:"last_computed"`
}

// FlowCell represents aggregated flow data for a grid cell.
type FlowCell struct {
	GridX       int     `json:"grid_x"`
	GridZ       int     `json:"grid_z"`
	VectorX     float64 `json:"vector_x"`
	VectorZ     float64 `json:"vector_z"`
	SegmentCount int    `json:"segment_count"`
}

// FlowMap is the computed flow map output.
type FlowMap struct {
	Cells      []FlowCell `json:"cells"`
	GridSize   float64    `json:"grid_size"`
	ComputedAt time.Time  `json:"computed_at"`
}

// DwellHeatmapCell represents a cell in the dwell heatmap.
type DwellHeatmapCell struct {
	GridX     int     `json:"grid_x"`
	GridZ     int     `json:"grid_z"`
	Count     int     `json:"count"`
	Normalized float64 `json:"normalized"`
}

// DwellHeatmap is the computed dwell heatmap output.
type DwellHeatmap struct {
	Cells      []DwellHeatmapCell `json:"cells"`
	ComputedAt time.Time          `json:"computed_at"`
}

// TrackUpdate represents a track update from the tracker.
type TrackUpdate struct {
	ID       int
	X, Y, Z  float64
	VX, VY, VZ float64
	PersonID string
}

// FlowAccumulator accumulates trajectory data for flow visualization.
type FlowAccumulator struct {
	mu          sync.RWMutex
	db          *sql.DB
	dbPath      string
	retentionDays int

	// In-memory tracking of last waypoint per track
	lastWaypoints map[int]*waypoint

	// Cache for computed flow map
	flowCache     *FlowMap
	flowCacheTime time.Time
	flowDirty     bool

	// Cache for computed dwell heatmap
	dwellCache     *DwellHeatmap
	dwellCacheTime time.Time
	dwellDirty     bool
}

type waypoint struct {
	x, z     float64
	personID string
}

// NewFlowAccumulator creates a new FlowAccumulator.
func NewFlowAccumulator(dbPath string) (*FlowAccumulator, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	fa := &FlowAccumulator{
		db:            db,
		dbPath:        dbPath,
		retentionDays: DefaultRetentionDays,
		lastWaypoints: make(map[int]*waypoint),
		flowDirty:     true,
		dwellDirty:    true,
	}

	if err := fa.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return fa, nil
}

// Close closes the database connection.
func (fa *FlowAccumulator) Close() error {
	return fa.db.Close()
}

func (fa *FlowAccumulator) migrate() error {
	_, err := fa.db.Exec(`
		CREATE TABLE IF NOT EXISTS trajectory_segments (
			id TEXT PRIMARY KEY,
			person_id TEXT NOT NULL DEFAULT '',
			from_x REAL NOT NULL,
			from_z REAL NOT NULL,
			to_x REAL NOT NULL,
			to_z REAL NOT NULL,
			speed REAL NOT NULL,
			timestamp INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_trajectory_timestamp ON trajectory_segments(timestamp);
		CREATE INDEX IF NOT EXISTS idx_trajectory_person ON trajectory_segments(person_id);
		CREATE INDEX IF NOT EXISTS idx_trajectory_timestamp_person ON trajectory_segments(timestamp, person_id);

		CREATE TABLE IF NOT EXISTS dwell_accumulator (
			grid_x INTEGER NOT NULL,
			grid_z INTEGER NOT NULL,
			person_id TEXT NOT NULL DEFAULT '',
			count INTEGER NOT NULL DEFAULT 0,
			last_updated INTEGER NOT NULL,
			PRIMARY KEY (grid_x, grid_z, person_id)
		);

		CREATE TABLE IF NOT EXISTS detected_corridors (
			id TEXT PRIMARY KEY,
			centroid_x REAL NOT NULL,
			centroid_z REAL NOT NULL,
			dominant_dir_x REAL NOT NULL,
			dominant_dir_z REAL NOT NULL,
			length_m REAL NOT NULL,
			width_m REAL NOT NULL,
			cell_count INTEGER NOT NULL,
			last_computed INTEGER NOT NULL
		);
	`)
	return err
}

// UpdateTrack processes a track update from the tracker.
// It records trajectory segments and dwell accumulator updates.
func (fa *FlowAccumulator) UpdateTrack(update TrackUpdate) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	now := time.Now()
	speed := math.Sqrt(update.VX*update.VX + update.VZ*update.VZ)

	// Project to ground plane (ignore Y)
	x, z := update.X, update.Z

	// Check if this is a stationary update for dwell accumulation
	if speed < StationarySpeedThreshold {
		gridX := int(math.Floor(x / GridCellSize))
		gridZ := int(math.Floor(z / GridCellSize))
		fa.recordDwell(gridX, gridZ, update.PersonID, now)
	}

	// Check for trajectory segment
	last, exists := fa.lastWaypoints[update.ID]
	if exists {
		dx := x - last.x
		dz := z - last.z
		dist := math.Sqrt(dx*dx + dz*dz)

		if dist >= MinMovementThreshold {
			// Record trajectory segment
			segID := generateSegmentID(update.ID, now)
			fa.recordSegment(TrajectorySegment{
				ID:        segID,
				PersonID:  last.personID,
				FromX:     last.x,
				FromZ:     last.z,
				ToX:       x,
				ToZ:       z,
				Speed:     speed,
				Timestamp: now,
			})

			// Mark caches as dirty
			fa.flowDirty = true
		}
	}

	// Update last waypoint
	fa.lastWaypoints[update.ID] = &waypoint{
		x:        x,
		z:        z,
		personID: update.PersonID,
	}
}

// RemoveTrack removes a track's waypoint when it disappears.
func (fa *FlowAccumulator) RemoveTrack(trackID int) {
	fa.mu.Lock()
	delete(fa.lastWaypoints, trackID)
	fa.mu.Unlock()
}

func (fa *FlowAccumulator) recordSegment(seg TrajectorySegment) {
	_, err := fa.db.Exec(`
		INSERT INTO trajectory_segments (id, person_id, from_x, from_z, to_x, to_z, speed, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, seg.ID, seg.PersonID, seg.FromX, seg.FromZ, seg.ToX, seg.ToZ, seg.Speed, seg.Timestamp.UnixNano())
	if err != nil {
		// Log but don't fail - we don't want to crash on DB errors
		return
	}
}

func (fa *FlowAccumulator) recordDwell(gridX, gridZ int, personID string, now time.Time) {
	_, err := fa.db.Exec(`
		INSERT INTO dwell_accumulator (grid_x, grid_z, person_id, count, last_updated)
		VALUES (?, ?, ?, 1, ?)
		ON CONFLICT(grid_x, grid_z, person_id) DO UPDATE SET
			count = count + 1,
			last_updated = excluded.last_updated
	`, gridX, gridZ, personID, now.UnixNano())
	if err != nil {
		return
	}
	fa.dwellDirty = true
}

// GetFlowMap computes and returns the flow map.
// Results are cached for 5 minutes or until data changes.
func (fa *FlowAccumulator) GetFlowMap(personID string, since, until time.Time) (*FlowMap, error) {
	fa.mu.RLock()
	defer fa.mu.RUnlock()

	// Check cache validity (5 minutes)
	cacheDuration := 5 * time.Minute
	now := time.Now()

	// If personID filter is set, bypass cache
	if personID == "" && !fa.flowDirty && fa.flowCache != nil && now.Sub(fa.flowCacheTime) < cacheDuration {
		return fa.flowCache, nil
	}

	// Build query
	query := `
		SELECT from_x, from_z, to_x, to_z
		FROM trajectory_segments
		WHERE timestamp >= ? AND timestamp <= ?
	`
	args := []interface{}{since.UnixNano(), until.UnixNano()}

	if personID != "" {
		query += " AND person_id = ?"
		args = append(args, personID)
	}

	rows, err := fa.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Accumulate flow vectors per cell
	type cellAccumulator struct {
		vectorX, vectorZ float64
		count            int
	}
	cellMap := make(map[[2]int]*cellAccumulator)

	for rows.Next() {
		var fromX, fromZ, toX, toZ float64
		if err := rows.Scan(&fromX, &fromZ, &toX, &toZ); err != nil {
			continue
		}

		// Use Bresenham's line algorithm to find cells the segment passes through
		cells := bresenhamLine(
			int(math.Floor(fromX/GridCellSize)),
			int(math.Floor(fromZ/GridCellSize)),
			int(math.Floor(toX/GridCellSize)),
			int(math.Floor(toZ/GridCellSize)),
		)

		// Accumulate vector contribution for each cell
		dx := toX - fromX
		dz := toZ - fromZ

		for _, cell := range cells {
			key := [2]int{cell[0], cell[1]}
			acc, exists := cellMap[key]
			if !exists {
				acc = &cellAccumulator{}
				cellMap[key] = acc
			}
			acc.vectorX += dx
			acc.vectorZ += dz
			acc.count++
		}
	}

	// Build flow map
	flowMap := &FlowMap{
		Cells:      make([]FlowCell, 0, len(cellMap)),
		GridSize:   GridCellSize,
		ComputedAt: now,
	}

	for key, acc := range cellMap {
		if acc.count < MinSegmentsForFlow {
			continue
		}
		flowMap.Cells = append(flowMap.Cells, FlowCell{
			GridX:        key[0],
			GridZ:        key[1],
			VectorX:      acc.vectorX / float64(acc.count),
			VectorZ:      acc.vectorZ / float64(acc.count),
			SegmentCount: acc.count,
		})
	}

	// Update cache only for unfiltered queries
	if personID == "" {
		fa.flowCache = flowMap
		fa.flowCacheTime = now
		fa.flowDirty = false
	}

	return flowMap, nil
}

// GetDwellHeatmap computes and returns the dwell heatmap.
// Results are cached for 5 minutes or until data changes.
func (fa *FlowAccumulator) GetDwellHeatmap(personID string) (*DwellHeatmap, error) {
	fa.mu.RLock()
	defer fa.mu.RUnlock()

	// Check cache validity (5 minutes)
	cacheDuration := 5 * time.Minute
	now := time.Now()

	// If personID filter is set, bypass cache
	if personID == "" && !fa.dwellDirty && fa.dwellCache != nil && now.Sub(fa.dwellCacheTime) < cacheDuration {
		return fa.dwellCache, nil
	}

	// Build query
	query := "SELECT grid_x, grid_z, count FROM dwell_accumulator"
	args := []interface{}{}

	if personID != "" {
		query += " WHERE person_id = ?"
		args = append(args, personID)
	}

	rows, err := fa.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cells []DwellHeatmapCell
	var maxCount int

	for rows.Next() {
		var gridX, gridZ, count int
		if err := rows.Scan(&gridX, &gridZ, &count); err != nil {
			continue
		}
		if count < MinDwellSamples {
			continue
		}
		cells = append(cells, DwellHeatmapCell{
			GridX: gridX,
			GridZ: gridZ,
			Count: count,
		})
		if count > maxCount {
			maxCount = count
		}
	}

	// Normalize to [0, 1]
	heatmap := &DwellHeatmap{
		Cells:      make([]DwellHeatmapCell, len(cells)),
		ComputedAt: now,
	}

	for i, cell := range cells {
		heatmap.Cells[i] = DwellHeatmapCell{
			GridX:      cell.GridX,
			GridZ:      cell.GridZ,
			Count:      cell.Count,
			Normalized: float64(cell.Count) / float64(maxCount),
		}
	}

	// Update cache only for unfiltered queries
	if personID == "" {
		fa.dwellCache = heatmap
		fa.dwellCacheTime = now
		fa.dwellDirty = false
	}

	return heatmap, nil
}

// GetCorridors returns detected corridors.
func (fa *FlowAccumulator) GetCorridors() ([]DetectedCorridor, error) {
	fa.mu.RLock()
	defer fa.mu.RUnlock()

	rows, err := fa.db.Query(`
		SELECT id, centroid_x, centroid_z, dominant_dir_x, dominant_dir_z, length_m, width_m, cell_count, last_computed
		FROM detected_corridors
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var corridors []DetectedCorridor
	for rows.Next() {
		var c DetectedCorridor
		var lastComputed int64
		if err := rows.Scan(&c.ID, &c.CentroidX, &c.CentroidZ, &c.DominantDirX, &c.DominantDirZ,
			&c.LengthM, &c.WidthM, &c.CellCount, &lastComputed); err != nil {
			continue
		}
		c.LastComputed = time.Unix(0, lastComputed)
		corridors = append(corridors, c)
	}

	return corridors, nil
}

// ComputeCorridors recomputes corridor detection.
// Should be called periodically (e.g., weekly).
func (fa *FlowAccumulator) ComputeCorridors() error {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	// Get all trajectory segments
	rows, err := fa.db.Query(`SELECT from_x, from_z, to_x, to_z, timestamp FROM trajectory_segments`)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Build per-cell angle lists for circular variance computation
	type cellAngles struct {
		angles []float64
		vectorsX []float64
		vectorsZ []float64
	}
	cellMap := make(map[[2]int]*cellAngles)

	for rows.Next() {
		var fromX, fromZ, toX, toZ float64
		var ts int64
		if err := rows.Scan(&fromX, &fromZ, &toX, &toZ, &ts); err != nil {
			continue
		}

		// Find cells the segment passes through
		cells := bresenhamLine(
			int(math.Floor(fromX/GridCellSize)),
			int(math.Floor(fromZ/GridCellSize)),
			int(math.Floor(toX/GridCellSize)),
			int(math.Floor(toZ/GridCellSize)),
		)

		// Compute angle of this segment
		angle := math.Atan2(toZ-fromZ, toX-fromX)
		dx := toX - fromX
		dz := toZ - fromZ

		for _, cell := range cells {
			key := [2]int{cell[0], cell[1]}
			acc, exists := cellMap[key]
			if !exists {
				acc = &cellAngles{}
				cellMap[key] = acc
			}
			acc.angles = append(acc.angles, angle)
			acc.vectorsX = append(acc.vectorsX, dx)
			acc.vectorsZ = append(acc.vectorsZ, dz)
		}
	}

	// Identify corridor candidate cells
	corridorCells := make(map[[2]int]bool)
	for key, acc := range cellMap {
		if len(acc.angles) < CorridorMinSegments {
			continue
		}
		variance := circularVariance(acc.angles)
		if variance < CorridorMaxAngularVariance {
			corridorCells[key] = true
		}
	}

	// Connected component analysis
	regions := findConnectedComponents(corridorCells)

	// Build corridor records
	now := time.Now()
	var corridors []DetectedCorridor

	for i, region := range regions {
		if len(region) < 3 {
			continue // Skip very small regions
		}

		// Compute centroid
		var sumX, sumZ float64
		for _, cell := range region {
			sumX += float64(cell[0])
			sumZ += float64(cell[1])
		}
		centroidX := (sumX / float64(len(region)) + 0.5) * GridCellSize
		centroidZ := (sumZ / float64(len(region)) + 0.5) * GridCellSize

		// Compute dominant direction by averaging vectors
		var avgVX, avgVZ float64
		var count int
		for _, cell := range region {
			if acc, exists := cellMap[cell]; exists {
				for j := range acc.vectorsX {
					avgVX += acc.vectorsX[j]
					avgVZ += acc.vectorsZ[j]
					count++
				}
			}
		}
		if count > 0 {
			avgVX /= float64(count)
			avgVZ /= float64(count)
			// Normalize
			mag := math.Sqrt(avgVX*avgVX + avgVZ*avgVZ)
			if mag > 0 {
				avgVX /= mag
				avgVZ /= mag
			}
		}

		// Compute bounding box for length/width
		var minX, maxX, minZ, maxZ int
		first := true
		for _, cell := range region {
			if first {
				minX, maxX, minZ, maxZ = cell[0], cell[0], cell[1], cell[1]
				first = false
			} else {
				if cell[0] < minX { minX = cell[0] }
				if cell[0] > maxX { maxX = cell[0] }
				if cell[1] < minZ { minZ = cell[1] }
				if cell[1] > maxZ { maxZ = cell[1] }
			}
		}

		length := float64(maxZ-minZ+1) * GridCellSize
		width := float64(maxX-minX+1) * GridCellSize
		if width > length {
			length, width = width, length
		}

		corridors = append(corridors, DetectedCorridor{
			ID:           generateCorridorID(i),
			CentroidX:    centroidX,
			CentroidZ:    centroidZ,
			DominantDirX: avgVX,
			DominantDirZ: avgVZ,
			LengthM:      length,
			WidthM:       width,
			CellCount:    len(region),
			LastComputed: now,
		})
	}

	// Clear existing corridors and insert new ones
	tx, err := fa.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM detected_corridors"); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO detected_corridors (id, centroid_x, centroid_z, dominant_dir_x, dominant_dir_z, length_m, width_m, cell_count, last_computed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range corridors {
		_, err := stmt.Exec(c.ID, c.CentroidX, c.CentroidZ, c.DominantDirX, c.DominantDirZ,
			c.LengthM, c.WidthM, c.CellCount, c.LastComputed.UnixNano())
		if err != nil {
			continue
		}
	}

	return tx.Commit()
}

// PruneOldSegments removes trajectory segments older than retention period.
func (fa *FlowAccumulator) PruneOldSegments() error {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -fa.retentionDays)
	_, err := fa.db.Exec(`DELETE FROM trajectory_segments WHERE timestamp < ?`, cutoff.UnixNano())
	if err == nil {
		fa.flowDirty = true
	}
	return err
}

// bresenhamLine returns all grid cells a line passes through.
func bresenhamLine(x0, z0, x1, z1 int) [][2]int {
	var cells [][2]int

	dx := abs(x1 - x0)
	dz := abs(z1 - z0)
	sx := sign(x1 - x0)
	sz := sign(z1 - z0)

	if dz <= dx {
		err := 2 * dz - dx
		for i := 0; i <= dx; i++ {
			cells = append(cells, [2]int{x0, z0})
			if err > 0 {
				z0 += sz
				err -= 2 * dx
			}
			err += 2 * dz
			x0 += sx
		}
	} else {
		err := 2 * dx - dz
		for i := 0; i <= dz; i++ {
			cells = append(cells, [2]int{x0, z0})
			if err > 0 {
				x0 += sx
				err -= 2 * dz
			}
			err += 2 * dx
			z0 += sz
		}
	}

	return cells
}

// circularVariance computes the circular variance of angles.
// Returns a value in [0, 1] where 0 = all angles aligned, 1 = uniform distribution.
func circularVariance(angles []float64) float64 {
	if len(angles) == 0 {
		return 1.0
	}

	var sumSin, sumCos float64
	for _, a := range angles {
		sumSin += math.Sin(a)
		sumCos += math.Cos(a)
	}

	n := float64(len(angles))
	meanLength := math.Sqrt(sumSin*sumSin+sumCos*sumCos) / n

	// Circular variance = 1 - R where R is mean resultant length
	return 1.0 - meanLength
}

// findConnectedComponents finds connected regions of cells using 4-connectivity.
func findConnectedComponents(cells map[[2]int]bool) [][][2]int {
	if len(cells) == 0 {
		return nil
	}

	visited := make(map[[2]int]bool)
	var regions [][][2]int

	for cell := range cells {
		if visited[cell] {
			continue
		}

		// BFS to find connected component
		var region [][2]int
		queue := [][2]int{cell}
		visited[cell] = true

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			region = append(region, current)

			// Check 4 neighbors
			neighbors := [4][2]int{
				{current[0] - 1, current[1]},
				{current[0] + 1, current[1]},
				{current[0], current[1] - 1},
				{current[0], current[1] + 1},
			}

			for _, n := range neighbors {
				if cells[n] && !visited[n] {
					visited[n] = true
					queue = append(queue, n)
				}
			}
		}

		if len(region) > 0 {
			regions = append(regions, region)
		}
	}

	return regions
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func sign(x int) int {
	if x < 0 {
		return -1
	}
	if x > 0 {
		return 1
	}
	return 0
}

func generateSegmentID(trackID int, t time.Time) string {
	return string(rune(trackID)) + "_" + t.Format("20060102150405.000000000")
}

func generateCorridorID(index int) string {
	return "corridor_" + string(rune('A'+index%26)) + string(rune('0'+index/26))
}
