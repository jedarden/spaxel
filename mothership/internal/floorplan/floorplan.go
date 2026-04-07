// Package floorplan handles floor plan image upload and pixel-to-meter calibration.
package floorplan

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	// MaxUploadSize is the maximum allowed file size (10 MB)
	MaxUploadSize = 10 * 1024 * 1024
	// DefaultImageFilename is the name of the stored floor plan image
	DefaultImageFilename = "image.png"
)

// Handler provides floor plan HTTP endpoints.
type Handler struct {
	db          *sql.DB
	dataDir     string
	floorplanDir string
}

// NewHandler creates a new floorplan handler.
func NewHandler(db *sql.DB, dataDir string) *Handler {
	fpDir := filepath.Join(dataDir, "floorplan")
	if err := os.MkdirAll(fpDir, 0755); err != nil {
		log.Printf("[WARN] Failed to create floorplan directory: %v", err)
	}
	return &Handler{
		db:          db,
		dataDir:     dataDir,
		floorplanDir: fpDir,
	}
}

// RegisterRoutes mounts the floorplan endpoints on r.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/floorplan/image", h.uploadImage)
	r.Get("/api/floorplan/image", h.getImage)
	r.Post("/api/floorplan/calibrate", h.calibrate)
	r.Get("/api/floorplan/calibrate", h.getCalibration)
	r.Get("/api/floorplan", h.getFloorplan)
}

// floorplanRecord represents the floorplan table row.
type floorplanRecord struct {
	ImagePath   string  `json:"image_path"`
	CalAX       float64 `json:"cal_ax"`
	CalAY       float64 `json:"cal_ay"`
	CalBX       float64 `json:"cal_bx"`
	CalBY       float64 `json:"cal_by"`
	DistanceM   float64 `json:"distance_m"`
	RotationDeg float64 `json:"rotation_deg"`
	UpdatedAt   int64   `json:"updated_at"`
}

// uploadImage handles POST /api/floorplan/image
// Accepts a multipart form with a file field "file" (PNG/JPG, max 10 MB)
func (h *Handler) uploadImage(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)

	// Parse multipart form (max 32 MB in memory)
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		http.Error(w, "file too large (max 10 MB)", http.StatusRequestEntityTooLarge)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read entire file into memory for validation and saving
	// multipart.File doesn't support Seek, so we need to buffer
	fileData, err := io.ReadAll(file)
	if err != nil {
		log.Printf("[ERROR] Failed to read uploaded file: %v", err)
		http.Error(w, "failed to read file", http.StatusInternalServerError)
		return
	}

	log.Printf("[DEBUG] Read %d bytes from uploaded file", len(fileData))

	// Check file size
	if len(fileData) > MaxUploadSize {
		http.Error(w, "file too large (max 10 MB)", http.StatusRequestEntityTooLarge)
		return
	}

	// Decode image to validate format
	_, format, err := image.DecodeConfig(bytes.NewReader(fileData))
	if err != nil {
		http.Error(w, "invalid image format (PNG/JPG only)", http.StatusBadRequest)
		return
	}

	// Validate format
	if format != "jpeg" && format != "png" {
		http.Error(w, "invalid image format (PNG/JPG only)", http.StatusBadRequest)
		return
	}

	// Save to disk
	imagePath := filepath.Join(h.floorplanDir, DefaultImageFilename)
	if err := os.WriteFile(imagePath, fileData, 0644); err != nil {
		log.Printf("[ERROR] Failed to write floorplan image: %v", err)
		http.Error(w, "failed to save image", http.StatusInternalServerError)
		return
	}

	// Update database record
	ctx := r.Context()
	now := currentTimestamp()
	query := `
		INSERT INTO floorplan (id, image_path, updated_at)
		VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			image_path = excluded.image_path,
			updated_at = excluded.updated_at
	`
	_, err = h.db.ExecContext(ctx, query, "/floorplan/image.png", now)
	if err != nil {
		log.Printf("[ERROR] Failed to update floorplan record: %v", err)
		http.Error(w, "failed to update record", http.StatusInternalServerError)
		return
	}

	// Return success with image URL
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"ok":       "true",
		"image_url": "/floorplan/image.png",
	})
}

// getImage handles GET /api/floorplan/image (redirect) and /floorplan/image.png (serve file)
func (h *Handler) getImage(w http.ResponseWriter, r *http.Request) {
	imagePath := filepath.Join(h.floorplanDir, DefaultImageFilename)

	// Check if file exists
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		http.Error(w, "no floor plan image uploaded", http.StatusNotFound)
		return
	}

	// Detect content type
	ext := filepath.Ext(imagePath)
	contentType := "image/png"
	if ext == ".jpg" || ext == ".jpeg" {
		contentType = "image/jpeg"
	}

	w.Header().Set("Content-Type", contentType)
	http.ServeFile(w, r, imagePath)
}

// calibrateRequest contains calibration point data.
type calibrateRequest struct {
	AX         float64 `json:"ax"`
	AY         float64 `json:"ay"`
	BX         float64 `json:"bx"`
	BY         float64 `json:"by"`
	DistanceM  float64 `json:"distance_m"`
	RotationDeg float64 `json:"rotation_deg,omitempty"`
}

// calibrate handles POST /api/floorplan/calibrate
// Accepts two pixel coordinates and their real-world distance
func (h *Handler) calibrate(w http.ResponseWriter, r *http.Request) {
	var req calibrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate inputs
	if req.DistanceM <= 0 || req.DistanceM > 1000 {
		http.Error(w, "distance_m must be between 0 and 1000 meters", http.StatusBadRequest)
		return
	}

	// Compute pixel distance
	pixelDist := sqrt(req.BX-req.AX, req.BY-req.AY)
	if pixelDist < 10 {
		http.Error(w, "calibration points too close (minimum 10 pixels)", http.StatusBadRequest)
		return
	}

	// Calculate rotation angle if not provided
	if req.RotationDeg == 0 {
		angleRad := atan2(req.BY-req.AY, req.BX-req.AX)
		req.RotationDeg = angleRad * 180.0 / 3.141592653589793
	}

	// Update database record
	ctx := r.Context()
	now := currentTimestamp()
	query := `
		INSERT INTO floorplan (id, cal_ax, cal_ay, cal_bx, cal_by, distance_m, rotation_deg, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			cal_ax = excluded.cal_ax,
			cal_ay = excluded.cal_ay,
			cal_bx = excluded.cal_bx,
			cal_by = excluded.cal_by,
			distance_m = excluded.distance_m,
			rotation_deg = excluded.rotation_deg,
			updated_at = excluded.updated_at
	`
	_, err := h.db.ExecContext(ctx, query, req.AX, req.AY, req.BX, req.BY, req.DistanceM, req.RotationDeg, now)
	if err != nil {
		log.Printf("[ERROR] Failed to update floorplan calibration: %v", err)
		http.Error(w, "failed to save calibration", http.StatusInternalServerError)
		return
	}

	// Compute derived values for response
	metersPerPixel := req.DistanceM / pixelDist

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":               "true",
		"meters_per_pixel": metersPerPixel,
		"rotation_deg":     req.RotationDeg,
	})
}

// getCalibration handles GET /api/floorplan/calibrate
// Returns the current calibration or 404 if not calibrated
func (h *Handler) getCalibration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var rec floorplanRecord
	query := `
		SELECT cal_ax, cal_ay, cal_bx, cal_by, distance_m, rotation_deg
		FROM floorplan WHERE id = 1
	`
	err := h.db.QueryRowContext(ctx, query).Scan(
		&rec.CalAX, &rec.CalAY, &rec.CalBX, &rec.CalBY,
		&rec.DistanceM, &rec.RotationDeg,
	)
	if err == sql.ErrNoRows {
		http.Error(w, "no calibration data", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("[ERROR] Failed to get calibration: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Validate calibration is complete
	if rec.CalAX == 0 && rec.CalAY == 0 && rec.CalBX == 0 && rec.CalBY == 0 {
		http.Error(w, "calibration incomplete", http.StatusNotFound)
		return
	}

	// Compute derived values
	pixelDist := sqrt(rec.CalBX-rec.CalAX, rec.CalBY-rec.CalAY)
	metersPerPixel := rec.DistanceM / pixelDist

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cal_ax":           rec.CalAX,
		"cal_ay":           rec.CalAY,
		"cal_bx":           rec.CalBX,
		"cal_by":           rec.CalBY,
		"distance_m":       rec.DistanceM,
		"rotation_deg":     rec.RotationDeg,
		"meters_per_pixel": metersPerPixel,
	})
}

// getFloorplan handles GET /api/floorplan
// Returns complete floorplan data including image URL and calibration
func (h *Handler) getFloorplan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var rec floorplanRecord
	query := `
		SELECT image_path, cal_ax, cal_ay, cal_bx, cal_by, distance_m, rotation_deg, updated_at
		FROM floorplan WHERE id = 1
	`
	err := h.db.QueryRowContext(ctx, query).Scan(
		&rec.ImagePath, &rec.CalAX, &rec.CalAY, &rec.CalBX, &rec.CalBY,
		&rec.DistanceM, &rec.RotationDeg, &rec.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// Return empty state
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"image_url":   nil,
			"calibration": nil,
		})
		return
	}
	if err != nil {
		log.Printf("[ERROR] Failed to get floorplan: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Check if image file exists
	imageURL := rec.ImagePath
	if imageURL != "" {
		imagePath := filepath.Join(h.floorplanDir, DefaultImageFilename)
		if _, err := os.Stat(imagePath); os.IsNotExist(err) {
			imageURL = ""
		}
	}

	// Build calibration data
	var calibration map[string]interface{}
	if rec.CalAX != 0 || rec.CalAY != 0 || rec.CalBX != 0 || rec.CalBY != 0 {
		pixelDist := sqrt(rec.CalBX-rec.CalAX, rec.CalBY-rec.CalAY)
		metersPerPixel := rec.DistanceM / pixelDist
		calibration = map[string]interface{}{
			"cal_ax":           rec.CalAX,
			"cal_ay":           rec.CalAY,
			"cal_bx":           rec.CalBX,
			"cal_by":           rec.CalBY,
			"distance_m":       rec.DistanceM,
			"rotation_deg":     rec.RotationDeg,
			"meters_per_pixel": metersPerPixel,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"image_url":   imageURL,
		"calibration": calibration,
	})
}

// Helper functions

func currentTimestamp() int64 {
	return time.Now().UnixMilli()
}

func sqrt(dx, dy float64) float64 {
	// Calculate Euclidean distance: sqrt(dx² + dy²)
	// Use math.Sqrt for proper calculation
	return math.Sqrt(dx*dx + dy*dy)
}

func atan2(y, x float64) float64 {
	// Simplified atan2 implementation
	if x > 0 {
		return atan(y / x)
	}
	if x < 0 && y >= 0 {
		return atan(y/x) + 3.141592653589793
	}
	if x < 0 && y < 0 {
		return atan(y/x) - 3.141592653589793
	}
	if x == 0 && y > 0 {
		return 3.141592653589793 / 2
	}
	if x == 0 && y < 0 {
		return -3.141592653589793 / 2
	}
	return 0
}

func atan(x float64) float64 {
	// Taylor series approximation for atan
	if x > 1 {
		return 1.5707963267948966 - atan(1/x)
	}
	if x < -1 {
		return -1.5707963267948966 - atan(-1/x)
	}
	// Approximate using x - x³/3 + x⁵/5
	x2 := x * x
	return x - x*x2/3 + x2*x2*x/5
}

// GetCalibration returns the current calibration data for use by other packages.
func (h *Handler) GetCalibration(ctx context.Context) (metersPerPixel float64, rotationDeg float64, ok bool) {
	var rec floorplanRecord
	query := `
		SELECT cal_ax, cal_ay, cal_bx, cal_by, distance_m, rotation_deg
		FROM floorplan WHERE id = 1
	`
	err := h.db.QueryRowContext(ctx, query).Scan(
		&rec.CalAX, &rec.CalAY, &rec.CalBX, &rec.CalBY,
		&rec.DistanceM, &rec.RotationDeg,
	)
	if err != nil {
		return 0, 0, false
	}

	// Validate calibration is complete
	if rec.CalAX == 0 && rec.CalAY == 0 && rec.CalBX == 0 && rec.CalBY == 0 {
		return 0, 0, false
	}

	pixelDist := sqrt(rec.CalBX-rec.CalAX, rec.CalBY-rec.CalAY)
	if pixelDist < 1 {
		return 0, 0, false
	}

	return rec.DistanceM / pixelDist, rec.RotationDeg, true
}

// GetImagePath returns the path to the floor plan image file, or empty if not set.
func (h *Handler) GetImagePath() string {
	imagePath := filepath.Join(h.floorplanDir, DefaultImageFilename)
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return ""
	}
	return imagePath
}
