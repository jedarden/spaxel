// Package analytics accumulates and analyzes crowd flow data
// for movement pattern visualization and dwell hotspot detection.
package analytics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	// Trajectory sampling thresholds
	minMovementDistance = 0.2 // meters - only record segment if track moved > 0.2m
	dwellSpeedThreshold  = 0.1 // m/s - speed below which counts as "dwell"
	dwellPruneDays       = 90  // days - prune dwell data older than this
	flowPruneDays        = 90  // days - prune trajectory segments older than this

	// Flow computation cache duration
	flowCacheMaxAge = 5 * time.Minute

	// Grid resolution (should match fusion grid)
	defaultGridCellM = 0.25 // meters

	// Corridor detection thresholds
	corridorMinSegments  = 10
	corridorMaxVariance   = 0.3
	corridorMinCellCount  = 3
	corridorRecomputeHours = 168 // 7 days
)

// TrajectorySegment represents a single movement segment for a tracked person.
type TrajectorySegment struct {
	ID        string    `json:"id"`
	PersonID  string    `json:"person_id,omitempty"`
	FromXYZ   [3]float64 `json:"from_xyz"`
	ToXYZ     [3]float64 `json:"to_xyz"`
	Speed     float64   `json:"speed"`      // m/s at this step
	Timestamp time.Time `json:"timestamp"`
}

// DwellAccumulator tracks stationary time per grid cell.
type DwellAccumulator struct {
	GridX       int       `json:"grid_x"`
	GridY       int       `json:"grid_y"`
	PersonID    string    `json:"person_id,omitempty"`
	Count       int       `json:"count"`      // number of stationary observations
	DwellMs     int64     `json:"dwell_ms"`    // total dwell time in milliseconds
	LastUpdated time.Time `json:"last_updated"`
}

// FlowCell represents flow data for a single grid cell.
type FlowCell struct {
	GridX        int     `json:"grid_x"`
	GridY        int     `json:"grid_y"`
	VX           float64 `json:"vx"`           // average X velocity component
	VY           float64 `json:"vy"`           // average Y velocity component
	SegmentCount int     `json:"segment_count"`
}

// FlowMap is the computed flow map for a grid.
type FlowMap struct {
	Cells        []FlowCell `json:"cells"`
	CellSizeM    float64    `json:"cell_size_m"`
	ComputedAt   time.Time  `json:"computed_at"`
	SegmentCount int        `json:"total_segments"`
}

// DwellHeatmap represents dwell time per grid cell.
type DwellHeatmap struct {
	Cells     []DwellCell `json:"cells"`
	CellSizeM float64     `json:"cell_size_m"`
	MaxCount  int         `json:"max_count"`
	ComputedAt time.Time  `json:"computed_at"`
	PersonID  string      `json:"person_id,omitempty"` // if filtered
}

// DwellCell represents dwell data for a single cell in the heatmap.
type DwellCell struct {
	GridX      int     `json:"grid_x"`
	GridY      int     `json:"grid_y"`
	Count      int     `json:"count"`
	Normalized float64 `json:"normalized"` // 0-1 after normalization
}

// DetectedCorridor represents a detected corridor region.
type DetectedCorridor struct {
	ID                string    `json:"id"`
	CentroidXYZ       [3]float64 `json:"centroid_xyz"`
	DominantDirection [2]float64 `json:"dominant_direction_xy"` // normalized vector
	LengthM           float64   `json:"length_m"`
	WidthM            float64   `json:"width_m"`
	CellCount         int       `json:"cell_count"`
	LastComputed      time.Time `json:"last_computed"`
}

// cachedFlowMap holds a cached flow map with its creation time.
type cachedFlowMap struct {
	flowMap  *FlowMap
	cachedAt time.Time
	dirty    bool
}

// FlowAccumulator subscribes to TrackManager updates and accumulates trajectory data.
type FlowAccumulator struct {
	mu      sync.RWMutex
	db      *sql.DB
	ownDB   bool // true if this instance opened the db and should close it
	cellSizeM float64
	flowCache *cachedFlowMap
	lastPrune time.Time

	// In-memory accumulator for batch writes
	trajectoryBuffer []TrajectorySegment
	dwellBuffer      []DwellAccumulator
	maxBufferSize    int

	// Track last waypoint per track for sampling
	lastWaypoints map[string][3]float64 // track_id -> last position
}

// NewFlowAccumulatorFromPath opens a SQLite database at path and creates a new flow accumulator.
// The caller is responsible for calling Close() to flush pending writes.
func NewFlowAccumulatorFromPath(path string) (*FlowAccumulator, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	fa := NewFlowAccumulator(db, 0)
	fa.ownDB = true
	if err := fa.InitSchema(); err != nil {
		db.Close() //nolint:errcheck
		return nil, err
	}
	return fa, nil
}

// NewFlowAccumulator creates a new flow accumulator.
func NewFlowAccumulator(db *sql.DB, cellSizeM float64) *FlowAccumulator {
	if cellSizeM <= 0 {
		cellSizeM = defaultGridCellM
	}
	return &FlowAccumulator{
		db:            db,
		cellSizeM:     cellSizeM,
		lastWaypoints: make(map[string][3]float64),
		maxBufferSize: 100, // flush after 100 segments
		flowCache:     &cachedFlowMap{dirty: true},
		lastPrune:     time.Now(),
	}
}

// InitSchema creates the required database tables if they don't exist.
func (f *FlowAccumulator) InitSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS trajectory_segments (
		id        TEXT PRIMARY KEY,
		person_id TEXT,
		from_x    REAL NOT NULL,
		from_y    REAL NOT NULL,
		from_z    REAL NOT NULL,
		to_x      REAL NOT NULL,
		to_y      REAL NOT NULL,
		to_z      REAL NOT NULL,
		speed     REAL NOT NULL,
		timestamp DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_traj_timestamp ON trajectory_segments(timestamp);
	CREATE INDEX IF NOT EXISTS idx_traj_person ON trajectory_segments(person_id, timestamp);

	CREATE TABLE IF NOT EXISTS dwell_accumulator (
		grid_x      INTEGER NOT NULL,
		grid_y      INTEGER NOT NULL,
		person_id   TEXT,
		count       INTEGER NOT NULL DEFAULT 1,
		dwell_ms    INTEGER NOT NULL DEFAULT 100,
		last_updated DATETIME NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		PRIMARY KEY (grid_x, grid_y, person_id)
	);
	CREATE INDEX IF NOT EXISTS idx_dwell_updated ON dwell_accumulator(last_updated);

	CREATE TABLE IF NOT EXISTS detected_corridors (
		id                TEXT PRIMARY KEY,
		centroid_x        REAL NOT NULL,
		centroid_y        REAL NOT NULL,
		centroid_z        REAL NOT NULL,
		direction_x       REAL NOT NULL,
		direction_y       REAL NOT NULL,
		length_m          REAL NOT NULL,
		width_m           REAL NOT NULL,
		cell_count        INTEGER NOT NULL,
		last_computed     DATETIME NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);
	`
	_, err := f.db.Exec(schema)
	return err
}

// AddTrackUpdate processes a track update from the tracker.
// personID may be empty if identity is unknown.
func (f *FlowAccumulator) AddTrackUpdate(trackID string, x, y, z, vx, vy, vz float64, personID string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Check if movement exceeds threshold
	lastPos, exists := f.lastWaypoints[trackID]
	if exists {
		dx := x - lastPos[0]
		dy := y - lastPos[1]
		dz := z - lastPos[2]
		distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

		// Calculate speed
		speed := math.Sqrt(vx*vx + vy*vy + vz*vz)

		if distance > minMovementDistance {
			// Record trajectory segment
			seg := TrajectorySegment{
				ID:        generateSegmentID(),
				PersonID:  personID,
				FromXYZ:   lastPos,
				ToXYZ:     [3]float64{x, y, z},
				Speed:     speed,
				Timestamp: time.Now(),
			}
			f.trajectoryBuffer = append(f.trajectoryBuffer, seg)
			f.markFlowDirty()
		}

		// Check for dwell (low speed)
		if speed < dwellSpeedThreshold {
			gridX := int(math.Floor(x / f.cellSizeM))
			gridY := int(math.Floor(y / f.cellSizeM))
			dwell := DwellAccumulator{
				GridX:       gridX,
				GridY:       gridY,
				PersonID:    personID,
				Count:       1,
				DwellMs:     100, // 100ms per tick at 10Hz
				LastUpdated: time.Now(),
			}
			f.dwellBuffer = append(f.dwellBuffer, dwell)
		}
	}

	// Update last waypoint
	f.lastWaypoints[trackID] = [3]float64{x, y, z}

	// Flush buffers if they get too large
	if len(f.trajectoryBuffer) >= f.maxBufferSize || len(f.dwellBuffer) >= f.maxBufferSize {
		go f.flushBuffers()
	}
}

// RemoveTrack removes a track from the waypoint registry.
func (f *FlowAccumulator) RemoveTrack(trackID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.lastWaypoints, trackID)
}

// markFlowDirty marks the flow cache as dirty.
func (f *FlowAccumulator) markFlowDirty() {
	f.flowCache.dirty = true
}

// flushBuffers writes buffered data to the database.
func (f *FlowAccumulator) flushBuffers() {
	f.mu.Lock()
	buffers := struct {
		trajectory []TrajectorySegment
		dwell      []DwellAccumulator
	}{
		trajectory: make([]TrajectorySegment, len(f.trajectoryBuffer)),
		dwell:      make([]DwellAccumulator, len(f.dwellBuffer)),
	}
	copy(buffers.trajectory, f.trajectoryBuffer)
	copy(buffers.dwell, f.dwellBuffer)
	f.trajectoryBuffer = f.trajectoryBuffer[:0]
	f.dwellBuffer = f.dwellBuffer[:0]
	f.mu.Unlock()

	// Flush trajectories
	if len(buffers.trajectory) > 0 {
		if err := f.insertTrajectories(buffers.trajectory); err != nil {
			log.Printf("[WARN] Failed to insert trajectory segments: %v", err)
		}
	}

	// Flush dwell data
	if len(buffers.dwell) > 0 {
		if err := f.upsertDwell(buffers.dwell); err != nil {
			log.Printf("[WARN] Failed to upsert dwell data: %v", err)
		}
	}
}

// insertTrajectories inserts trajectory segments into the database.
func (f *FlowAccumulator) insertTrajectories(segments []TrajectorySegment) error {
	tx, err := f.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO trajectory_segments (id, person_id, from_x, from_y, from_z, to_x, to_y, to_z, speed, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	ts := time.Now().UnixNano() / 1e6
	for _, seg := range segments {
		var personID interface{} = seg.PersonID
		if personID == "" {
			personID = nil
		}
		_, err := stmt.Exec(
			seg.ID,
			personID,
			seg.FromXYZ[0], seg.FromXYZ[1], seg.FromXYZ[2],
			seg.ToXYZ[0], seg.ToXYZ[1], seg.ToXYZ[2],
			seg.Speed,
			ts,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// upsertDwell upserts dwell accumulator data.
func (f *FlowAccumulator) upsertDwell(dwell []DwellAccumulator) error {
	tx, err := f.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO dwell_accumulator (grid_x, grid_y, person_id, count, dwell_ms, last_updated)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(grid_x, grid_y, person_id) DO UPDATE SET
			count = count + excluded.count,
			dwell_ms = dwell_ms + excluded.dwell_ms,
			last_updated = excluded.last_updated
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range dwell {
		var personID interface{} = d.PersonID
		if personID == "" {
			personID = nil
		}
		_, err := stmt.Exec(d.GridX, d.GridY, personID, d.Count, d.DwellMs,
			d.LastUpdated.UnixNano()/1e6)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ComputeFlowMap computes the flow map from trajectory segments.
// Optionally filters by personID and time range.
func (f *FlowAccumulator) ComputeFlowMap(personID *string, since, until *time.Time) (*FlowMap, error) {
	f.mu.RLock()
	cache := f.flowCache
	f.mu.RUnlock()

	// For filtered queries, always recompute
	needsRecompute := cache.dirty || time.Since(cache.cachedAt) > flowCacheMaxAge
	if personID != nil || since != nil || until != nil {
		needsRecompute = true
	}

	// Return cached result if valid
	if !needsRecompute {
		return cache.flowMap, nil
	}

	// Build query with filters
	query := `
		SELECT from_x, from_y, from_z, to_x, to_y, to_z
		FROM trajectory_segments
		WHERE 1=1
	`
	args := []interface{}{}

	if personID != nil && *personID != "" {
		query += " AND person_id = ?"
		args = append(args, *personID)
	}
	if since != nil {
		query += " AND timestamp >= ?"
		args = append(args, since.UnixNano()/1e6)
	}
	if until != nil {
		query += " AND timestamp <= ?"
		args = append(args, until.UnixNano()/1e6)
	}

	rows, err := f.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Accumulate flow vectors per cell
	cellVectors := make(map[string]struct {
		vxSum, vySum float64
		count        int
	})

	var segmentCount int
	for rows.Next() {
		var fromX, fromY, fromZ, toX, toY, toZ float64
		if err := rows.Scan(&fromX, &fromY, &fromZ, &toX, &toY, &toZ); err != nil {
			continue
		}
		segmentCount++

		// Get cells this segment passes through using Bresenham's line algorithm
		cells := bresenhamLine(
			int(math.Floor(fromX/f.cellSizeM)),
			int(math.Floor(fromY/f.cellSizeM)),
			int(math.Floor(toX/f.cellSizeM)),
			int(math.Floor(toY/f.cellSizeM)),
		)

		// Vector components
		vx := toX - fromX
		vy := toY - fromY

		for _, cell := range cells {
			key := cellKey(cell.x, cell.y)
			v := cellVectors[key]
			v.vxSum += vx
			v.vySum += vy
			v.count++
			cellVectors[key] = v
		}
	}

	// Build flow map cells
	cells := make([]FlowCell, 0, len(cellVectors))
	for key, v := range cellVectors {
		if v.count == 0 {
			continue
		}
		x, y := parseCellKey(key)
		cells = append(cells, FlowCell{
			GridX:        x,
			GridY:        y,
			VX:           v.vxSum / float64(v.count),
			VY:           v.vySum / float64(v.count),
			SegmentCount: v.count,
		})
	}

	flowMap := &FlowMap{
		Cells:        cells,
		CellSizeM:    f.cellSizeM,
		ComputedAt:   time.Now(),
		SegmentCount: segmentCount,
	}

	// Update cache (only for unfiltered queries)
	if personID == nil && since == nil && until == nil {
		f.mu.Lock()
		f.flowCache = &cachedFlowMap{
			flowMap:  flowMap,
			cachedAt: time.Now(),
			dirty:    false,
		}
		f.mu.Unlock()
	}

	return flowMap, nil
}

// ComputeDwellHeatmap computes a dwell heatmap from dwell accumulator data.
// Optionally filters by personID.
func (f *FlowAccumulator) ComputeDwellHeatmap(personID *string) (*DwellHeatmap, error) {
	query := `
		SELECT grid_x, grid_y, SUM(count) as total_count, SUM(dwell_ms) as total_dwell_ms
		FROM dwell_accumulator
		WHERE 1=1
	`
	args := []interface{}{}

	if personID != nil && *personID != "" {
		query += " AND person_id = ?"
		args = append(args, *personID)
	}

	query += " GROUP BY grid_x, grid_y"

	rows, err := f.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cells []DwellCell
	maxCount := 0

	for rows.Next() {
		var gridX, gridY, count int
		var dwellMs int64
		if err := rows.Scan(&gridX, &gridY, &count, &dwellMs); err != nil {
			continue
		}
		if count > maxCount {
			maxCount = count
		}
		cells = append(cells, DwellCell{
			GridX: gridX,
			GridY: gridY,
			Count: count,
		})
	}

	// Normalize counts to [0, 1]
	for i := range cells {
		if maxCount > 0 {
			cells[i].Normalized = float64(cells[i].Count) / float64(maxCount)
		}
	}

	heatmap := &DwellHeatmap{
		Cells:      cells,
		CellSizeM:  f.cellSizeM,
		MaxCount:   maxCount,
		ComputedAt: time.Now(),
		PersonID:   "",
	}
	if personID != nil {
		heatmap.PersonID = *personID
	}

	return heatmap, nil
}

// DetectCorridors detects corridor regions based on flow data.
func (f *FlowAccumulator) DetectCorridors() ([]DetectedCorridor, error) {
	// Get recent flow data
	flowMap, err := f.ComputeFlowMap(nil, nil, nil)
	if err != nil {
		return nil, err
	}

	// Find corridor cells (high volume, low angular variance)
	corridorCells := make(map[string]struct {
		vx, vy        float64
		angle         float64
		segmentCount  int
	})

	for _, cell := range flowMap.Cells {
		if cell.SegmentCount < corridorMinSegments {
			continue
		}

		// Calculate angle
		angle := math.Atan2(cell.VY, cell.VX)

		key := cellKey(cell.GridX, cell.GridY)
		corridorCells[key] = struct {
			vx, vy         float64
			angle          float64
			segmentCount   int
		}{
			vx:           cell.VX,
			vy:           cell.VY,
			angle:        angle,
			segmentCount: cell.SegmentCount,
		}
	}

	// Group adjacent corridor cells into regions
	regions := f.findConnectedCorridorRegions(corridorCells)

	// Build corridor objects
	corridors := make([]DetectedCorridor, 0, len(regions))
	for _, region := range regions {
		corridor := f.buildCorridorFromRegion(region)
		corridors = append(corridors, corridor)
	}

	// Save to database
	if err := f.saveCorridors(corridors); err != nil {
		log.Printf("[WARN] Failed to save corridors: %v", err)
	}

	return corridors, nil
}

// corridorRegion represents a group of adjacent corridor cells.
type corridorRegion struct {
	cells map[string]struct {
		x, y          int
		vx, vy        float64
		angle         float64
		segmentCount  int
	}
}

// findConnectedCorridorRegions groups adjacent corridor cells into regions.
func (f *FlowAccumulator) findConnectedCorridorRegions(cells map[string]struct {
	vx, vy        float64
	angle         float64
	segmentCount  int
}) []corridorRegion {
	visited := make(map[string]bool)
	regions := []corridorRegion{}

	for key, cell := range cells {
		if visited[key] {
			continue
		}

		// Start a new region with BFS
		region := corridorRegion{
			cells: make(map[string]struct {
				x, y          int
				vx, vy        float64
				angle         float64
				segmentCount  int
			}),
		}

		queue := []string{key}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			if visited[current] {
				continue
			}
			visited[current] = true

			x, y := parseCellKey(current)
			region.cells[current] = struct {
				x, y          int
				vx, vy        float64
				angle         float64
				segmentCount  int
			}{
				x:            x,
				y:            y,
				vx:           cell.vx,
				vy:           cell.vy,
				angle:        cell.angle,
				segmentCount: cell.segmentCount,
			}

			// Check 8 neighbors
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					if dx == 0 && dy == 0 {
						continue
					}
					neighborKey := cellKey(x+dx, y+dy)
					if neighbor, exists := cells[neighborKey]; exists && !visited[neighborKey] {
						// Check angular variance (should be low for corridors)
						angleDiff := math.Abs(cell.angle - neighbor.angle)
						if angleDiff > math.Pi {
							angleDiff = 2*math.Pi - angleDiff
						}
						if angleDiff < corridorMaxVariance {
							queue = append(queue, neighborKey)
						}
					}
				}
			}
		}

		if len(region.cells) >= corridorMinCellCount {
			regions = append(regions, region)
		}
	}

	return regions
}

// buildCorridorFromRegion creates a DetectedCorridor from a region.
func (f *FlowAccumulator) buildCorridorFromRegion(region corridorRegion) DetectedCorridor {
	if len(region.cells) == 0 {
		return DetectedCorridor{}
	}

	// Calculate centroid and dominant direction
	var sumX, sumY, sumZ float64
	var sumVX, sumVY float64
	var totalSegments int

	minX, maxX := math.MaxInt32, math.MinInt32
	minY, maxY := math.MaxInt32, math.MinInt32

	for _, cell := range region.cells {
		sumX += float64(cell.x) * f.cellSizeM
		sumY += float64(cell.y) * f.cellSizeM
		sumZ += 0 // Z is floor-projected
		sumVX += cell.vx * float64(cell.segmentCount)
		sumVY += cell.vy * float64(cell.segmentCount)
		totalSegments += cell.segmentCount

		if cell.x < minX {
			minX = cell.x
		}
		if cell.x > maxX {
			maxX = cell.x
		}
		if cell.y < minY {
			minY = cell.y
		}
		if cell.y > maxY {
			maxY = cell.y
		}
	}

	n := float64(len(region.cells))
	centroidX := sumX / n
	centroidY := sumY / n

	// Normalize dominant direction
	domVX := sumVX / float64(totalSegments)
	domVY := sumVY / float64(totalSegments)
	domMag := math.Sqrt(domVX*domVX + domVY*domVY)
	if domMag > 0 {
		domVX /= domMag
		domVY /= domMag
	}

	// Calculate length and width
	lengthM := math.Sqrt(float64(maxX-minX)*f.cellSizeM*domVX*domVX +
		float64(maxY-minY)*f.cellSizeM*domVY*domVY)
	widthM := math.Sqrt(float64(maxX-minX)*f.cellSizeM*(-domVY)*(-domVY) +
		float64(maxY-minY)*f.cellSizeM*domVX*domVX)

	return DetectedCorridor{
		ID:          generateCorridorID(),
		CentroidXYZ: [3]float64{centroidX, centroidY, 0},
		DominantDirection: [2]float64{domVX, domVY},
		LengthM:     lengthM,
		WidthM:      widthM,
		CellCount:   len(region.cells),
		LastComputed: time.Now(),
	}
}

// saveCorridors saves detected corridors to the database.
func (f *FlowAccumulator) saveCorridors(corridors []DetectedCorridor) error {
	tx, err := f.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear old corridors
	if _, err := tx.Exec("DELETE FROM detected_corridors"); err != nil {
		return err
	}

	// Insert new corridors
	stmt, err := tx.Prepare(`
		INSERT INTO detected_corridors (id, centroid_x, centroid_y, centroid_z, direction_x, direction_y, length_m, width_m, cell_count, last_computed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	ts := time.Now().UnixNano() / 1e6
	for _, c := range corridors {
		_, err := stmt.Exec(
			c.ID,
			c.CentroidXYZ[0], c.CentroidXYZ[1], c.CentroidXYZ[2],
			c.DominantDirection[0], c.DominantDirection[1],
			c.LengthM, c.WidthM, c.CellCount,
			ts,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetCorridors retrieves detected corridors from the database.
func (f *FlowAccumulator) GetCorridors() ([]DetectedCorridor, error) {
	rows, err := f.db.Query(`
		SELECT id, centroid_x, centroid_y, centroid_z, direction_x, direction_y, length_m, width_m, cell_count, last_computed
		FROM detected_corridors
		ORDER BY cell_count DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var corridors []DetectedCorridor
	for rows.Next() {
		var c DetectedCorridor
		var lastComputedMs int64
		err := rows.Scan(
			&c.ID,
			&c.CentroidXYZ[0], &c.CentroidXYZ[1], &c.CentroidXYZ[2],
			&c.DominantDirection[0], &c.DominantDirection[1],
			&c.LengthM, &c.WidthM, &c.CellCount,
			&lastComputedMs,
		)
		if err != nil {
			continue
		}
		c.LastComputed = time.Unix(lastComputedMs/1000, (lastComputedMs%1000)*1e6)
		corridors = append(corridors, c)
	}

	return corridors, nil
}

// PruneOldData removes old trajectory and dwell data.
func (f *FlowAccumulator) PruneOldData() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	if now.Sub(f.lastPrune) < 24*time.Hour {
		return nil // Only prune once per day
	}
	f.lastPrune = now

	cutoffTs := now.AddDate(0, 0, -flowPruneDays).UnixNano() / 1e6

	// Prune trajectory segments
	if _, err := f.db.Exec("DELETE FROM trajectory_segments WHERE timestamp < ?", cutoffTs); err != nil {
		log.Printf("[WARN] Failed to prune trajectory segments: %v", err)
	}

	// Prune dwell accumulator
	dwellCutoffTs := now.AddDate(0, 0, -dwellPruneDays).UnixNano() / 1e6
	if _, err := f.db.Exec("DELETE FROM dwell_accumulator WHERE last_updated < ?", dwellCutoffTs); err != nil {
		log.Printf("[WARN] Failed to prune dwell accumulator: %v", err)
	}

	f.markFlowDirty()
	return nil
}

// Flush flushes any buffered data to the database.
func (f *FlowAccumulator) Flush() error {
	f.flushBuffers()
	return nil
}

// Close cleans up resources.
func (f *FlowAccumulator) Close() error {
	if err := f.Flush(); err != nil {
		return err
	}
	if f.ownDB && f.db != nil {
		return f.db.Close()
	}
	return nil
}

// Helper functions

// cellKey creates a unique key for a grid cell.
func cellKey(x, y int) string {
	return fmt.Sprintf("%d,%d", x, y)
}

// parseCellKey parses a cell key into x, y coordinates.
func parseCellKey(key string) (x, y int) {
	_, err := fmt.Sscanf(key, "%d,%d", &x, &y)
	if err != nil {
		return 0, 0
	}
	return x, y
}

// generateSegmentID generates a unique segment ID.
func generateSegmentID() string {
	return time.Now().Format("20060102150405.000") + "-" + randString(4)
}

// generateCorridorID generates a unique corridor ID.
func generateCorridorID() string {
	return "corridor-" + time.Now().Format("20060102") + "-" + randString(8)
}

// randString generates a random hex string of length n.
func randString(n int) string {
	const chars = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[time.Now().UnixNano()%16]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}

// point represents a 2D grid coordinate.
type point struct {
	x, y int
}

// bresenhamLine implements Bresenham's line algorithm for grid traversal.
func bresenhamLine(x0, y0, x1, y1 int) []point {
	points := []point{}

	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}

	err := dx + dy

	x, y := x0, y0
	for {
		points = append(points, point{x, y})
		if x == x1 && y == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x += sx
		}
		if e2 <= dx {
			err += dx
			y += sy
		}
	}

	return points
}

// abs returns the absolute value of an integer.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ToJSON serializes a FlowMap to JSON.
func (f *FlowMap) ToJSON() ([]byte, error) {
	return json.Marshal(f)
}

// ToJSON serializes a DwellHeatmap to JSON.
func (d *DwellHeatmap) ToJSON() ([]byte, error) {
	return json.Marshal(d)
}

// ToJSON serializes a DetectedCorridor to JSON.
func (d *DetectedCorridor) ToJSON() ([]byte, error) {
	return json.Marshal(d)
}

// ToCorridorsJSON serializes a slice of DetectedCorridors to JSON.
func ToCorridorsJSON(corridors []DetectedCorridor) ([]byte, error) {
	return json.Marshal(corridors)
}

// TrackUpdate represents a single track position update for the flow accumulator.
type TrackUpdate struct {
	ID       int     `json:"id"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	VX       float64 `json:"vx"`
	VY       float64 `json:"vy"`
	VZ       float64 `json:"vz"`
	PersonID string  `json:"person_id,omitempty"`
}

// UpdateTrack processes a track update using the TrackUpdate struct.
// This is a convenience wrapper around AddTrackUpdate.
func (f *FlowAccumulator) UpdateTrack(update TrackUpdate) {
	trackID := fmt.Sprintf("track-%d", update.ID)
	f.AddTrackUpdate(trackID, update.X, update.Y, update.Z, update.VX, update.VY, update.VZ, update.PersonID)
}

// GetFlowMap computes the flow map from trajectory segments.
// Optionally filters by personID and time range.
// This is a convenience wrapper around ComputeFlowMap that accepts string timestamps.
func (f *FlowAccumulator) GetFlowMap(personID string, since, until time.Time) (*FlowMap, error) {
	var personIDPtr *string
	if personID != "" {
		personIDPtr = &personID
	}
	return f.ComputeFlowMap(personIDPtr, &since, &until)
}

// PruneOldSegments removes old trajectory and dwell data.
// This is a convenience wrapper around PruneOldData.
func (f *FlowAccumulator) PruneOldSegments() error {
	return f.PruneOldData()
}

// ComputeCorridors detects corridor regions based on flow data.
// This is a convenience wrapper around DetectCorridors that returns only the corridors.
func (f *FlowAccumulator) ComputeCorridors() error {
	_, err := f.DetectCorridors()
	return err
}
