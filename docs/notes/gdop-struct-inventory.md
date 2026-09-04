# GDOP struct inventory — `mothership/internal/simulator/gdop.go`

**Bead:** spaxel-48d1a2ab (split-child of the terminally-closed GDOP verification
parent spaxel-23a4ea1c)
**Verified at:** HEAD `1602ccd2`, 2026-09-04
**File:** `mothership/internal/simulator/gdop.go` — **1110 lines**, tracked,
last modified by `69785e2b` (docs: function signature documentation)

## Method

Three independent passes, all against the working tree at HEAD:

1. `grep -n 'struct {'` — exactly **6** occurrences, no anonymous struct types
   anywhere in the file.
2. `grep -n '^type '` — exactly **5** top-level type declarations, all structs.
   The file declares no interfaces, aliases, or non-struct types.
3. `go doc -all ./internal/simulator` — renders the five exported structs with
   field sets byte-identical to the source (captured below).

## Result

**Six struct definitions total: five top-level (all exported) plus one
function-local (unexported).** The acceptance-criteria list of five names
(`GDOPComputer`, `GDOPResult`, `GridConfig`, `GDOPHeatmapData`, `GDOPColor`)
is the complete set of *top-level* structs; the sixth, `linkAngle`, is declared
inside a function body and is disclosed here so the inventory is complete.

### 1. `GDOPResult` — :10-15 (exported)

GDOP computation results for a single cell.

```go
type GDOPResult struct {
	X, Y, Z           float64  // Cell center position
	GDOP              float64  // Computed GDOP value (Infinity = no coverage)
	Quality           string   // "excellent", "good", "fair", "poor", "none"
	ContributingLinks []string // Link IDs that contributed to this cell
}
```

| Field | Type | Note |
|---|---|---|
| `X` | `float64` | Cell center position (grouped declaration :11) |
| `Y` | `float64` | Cell center position (grouped declaration :11) |
| `Z` | `float64` | Cell center position (grouped declaration :11) |
| `GDOP` | `float64` | Infinity = no coverage |
| `Quality` | `string` | one of `"excellent"`, `"good"`, `"fair"`, `"poor"`, `"none"` |
| `ContributingLinks` | `[]string` | Link IDs that contributed to this cell |

6 named fields across 4 declaration lines. No JSON tags.

### 2. `GridConfig` — :18-23 (exported)

Defines the GDOP computation grid.

```go
type GridConfig struct {
	CellSize   float64 // Grid cell size in meters
	MinX, MinY float64 // Grid origin
	Width      float64 // Grid width
	Depth      float64 // Grid depth
}
```

| Field | Type | Note |
|---|---|---|
| `CellSize` | `float64` | Grid cell size in meters |
| `MinX` | `float64` | Grid origin (grouped declaration :20) |
| `MinY` | `float64` | Grid origin (grouped declaration :20) |
| `Width` | `float64` | Grid width |
| `Depth` | `float64` | Grid depth |

5 named fields across 4 declaration lines. No JSON tags. Distinct from
`Grid` (engine.go:49), which is the simulator's live occupancy grid —
same package, different file, different purpose.

### 3. `GDOPComputer` — :26-30 (exported)

Computes Geometric Dilution of Precision for coverage analysis. Constructed
only via `NewGDOPComputer(links []Link, config GridConfig) *GDOPComputer` (:84).

```go
type GDOPComputer struct {
	links   []Link
	config  GridConfig
	maxZone int // Maximum Fresnel zone to consider (default 3)
}
```

| Field | Type | Note |
|---|---|---|
| `links` | `[]Link` | unexported; TX→RX pairs under analysis |
| `config` | `GridConfig` | unexported; the evaluation grid |
| `maxZone` | `int` | unexported; maximum Fresnel zone to consider (default 3) |

All three fields are unexported, so `go doc` renders this struct as
`// Has unexported fields.` — the field list above comes from the source.

### 4. `GDOPColor` — :508-510 (exported)

A color for GDOP visualization. Populated by `GDOPColorMap(gdop float64)`,
which maps Infinity→gray, `<2`→green, `<4`→yellow, `<8`→orange, else red.

```go
type GDOPColor struct {
	R, G, B uint8 // RGB values 0-255
}
```

| Field | Type | Note |
|---|---|---|
| `R` | `uint8` | red channel, 0-255 (grouped declaration :509) |
| `G` | `uint8` | green channel, 0-255 (grouped declaration :509) |
| `B` | `uint8` | blue channel, 0-255 (grouped declaration :509) |

3 named fields in 1 declaration line. No JSON tags.

### 5. `GDOPHeatmapData` — :531-541 (exported)

Flattened GDOP data for frontend rendering; produced by
`(*GDOPComputer).ToHeatmapData(results [][]GDOPResult) *GDOPHeatmapData` (:543).
The only struct in the file carrying JSON tags — all nine fields are tagged.

```go
type GDOPHeatmapData struct {
	Width       int       `json:"width"`        // Grid width (columns)
	Depth       int       `json:"depth"`        // Grid depth (rows)
	CellSize    float64   `json:"cell_size"`    // Cell size in meters
	OriginX     float64   `json:"origin_x"`     // Grid origin X
	OriginY     float64   `json:"origin_y"`     // Grid origin Y
	GDOPValues  []float64 `json:"gdop_values"`  // Flattened GDOP values (9999 = infinity)
	Qualities   []string  `json:"qualities"`    // Flattened quality strings
	Colors      [][]uint8 `json:"colors"`       // Flattened RGB colors [width*depth*3]
	AccuracyMap []float64 `json:"accuracy_map"` // Expected accuracy in meters per cell
}
```

| Field | Type | JSON tag | Note |
|---|---|---|---|
| `Width` | `int` | `width` | Grid width (columns) |
| `Depth` | `int` | `depth` | Grid depth (rows) |
| `CellSize` | `float64` | `cell_size` | Cell size in meters |
| `OriginX` | `float64` | `origin_x` | Grid origin X |
| `OriginY` | `float64` | `origin_y` | Grid origin Y |
| `GDOPValues` | `[]float64` | `gdop_values` | 9999 encodes infinity |
| `Qualities` | `[]string` | `qualities` | Flattened quality strings |
| `Colors` | `[][]uint8` | `colors` | Flattened RGB `[width*depth*3]` |
| `AccuracyMap` | `[]float64` | `accuracy_map` | Expected accuracy (m) per cell |

### 6. `linkAngle` — :299-302 (function-local, unexported)

Declared inside `(*GDOPComputer).computeGDOPAngular(point Point, links []Link) float64`
(:296) as Step 1 of the angular-diversity computation. Invisible to `go doc`
(local scope) and unreachable outside the function — but it is a genuine
`type ... struct` declaration in this file, so it belongs in a complete
inventory.

```go
	type linkAngle struct {
		theta float64 // angle in radians
		link  Link
	}
```

| Field | Type | Note |
|---|---|---|
| `theta` | `float64` | Link angle in radians |
| `link` | `Link` | The contributing link |

## Scope notes

- **Package vs file:** `go doc -all ./internal/simulator` additionally shows
  `Grid` (engine.go:49) and `Handler` (handler.go:16). Both are in the same
  package but **different files** and are therefore outside this file-scoped
  inventory. `GridConfig` and `Grid` are unrelated types despite the name
  similarity.
- **No anonymous struct types** exist in the file — the 6 `struct {` matches
  above are the complete set.
- **Line count:** 1110, not the "1,111" that circulated in earlier notes in
  this bead family (that stale figure caused four verification failures on the
  parent chain).
- **Unowned side finding, unchanged by this bead:** the file is not gofmt-clean
  (`gofmt -l` lists it); the diff is entirely the gofmt ≥1.19 doc-comment
  reindentation of the `NewGDOPComputer` godoc block. No `gofmt` linter runs in
  CI (root `.golangci.yml`: errcheck/staticcheck/govet/ineffassign/unused), so
  this is cosmetic and deliberately untouched by a documentation bead.

## Verification

```
grep -n 'struct {' internal/simulator/gdop.go   # 6 matches (5 top-level + linkAngle :299)
grep -n '^type '   internal/simulator/gdop.go   # 5 matches, all structs
go doc -all ./internal/simulator | grep '^type ' # renders the 5 exported structs, fields identical
```
