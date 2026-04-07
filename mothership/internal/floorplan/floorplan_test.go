package floorplan

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestHandlerUploadAndGetImage(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Create a small test PNG (1x1 red pixel)
	testPNG := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE, 0x00, 0x00, 0x00,
		0x0C, 0x49, 0x44, 0x41, 0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
		0x00, 0x03, 0x01, 0x01, 0x00, 0x18, 0xDD, 0x8D, 0xB4, 0x00, 0x00, 0x00,
		0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	// Test upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test.png")
	if err != nil {
		t.Fatal(err)
	}
	_, err = part.Write(testPNG)
	if err != nil {
		t.Fatal(err)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/api/floorplan/image", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	h.uploadImage(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("uploadImage status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Parse response
	var uploadResp map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		t.Fatal(err)
	}
	if uploadResp["ok"] != "true" {
		t.Errorf("upload response ok = %s, want true", uploadResp["ok"])
	}

	// Test get image
	req = httptest.NewRequest("GET", "/api/floorplan/image", nil)
	w = httptest.NewRecorder()

	h.getImage(w, req)

	resp = w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("getImage status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if resp.Header.Get("Content-Type") != "image/png" {
		t.Errorf("getImage Content-Type = %s, want image/png", resp.Header.Get("Content-Type"))
	}
}

func TestHandlerCalibrate(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Test calibration
	calReq := calibrateRequest{
		AX:        100,
		AY:        100,
		BX:        500,
		BY:        100,
		DistanceM: 5.0,
	}

	body, _ := json.Marshal(calReq)
	req := httptest.NewRequest("POST", "/api/floorplan/calibrate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.calibrate(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("calibrate status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var calResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&calResp); err != nil {
		t.Fatal(err)
	}
	if calResp["ok"] != "true" {
		t.Errorf("calibrate response ok = %v, want true", calResp["ok"])
	}

	// Verify meters per pixel calculation
	// Pixel distance = 400, Real distance = 5m, so m/pixel = 0.0125
	expectedMPP := 5.0 / 400.0
	mpp, ok := calResp["meters_per_pixel"].(float64)
	if !ok {
		t.Fatal("meters_per_pixel not a number")
	}
	if mpp != expectedMPP {
		t.Errorf("meters_per_pixel = %f, want %f", mpp, expectedMPP)
	}
}

func TestHandlerGetCalibrationNotFound(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema (empty)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Test get calibration when none exists
	req := httptest.NewRequest("GET", "/api/floorplan/calibrate", nil)
	w := httptest.NewRecorder()

	h.getCalibration(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("getCalibration status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandlerUploadTooLarge(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Create a "file" that exceeds the limit
	largeData := make([]byte, MaxUploadSize+1)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "large.png")
	if err != nil {
		t.Fatal(err)
	}
	_, err = part.Write(largeData)
	if err != nil {
		t.Fatal(err)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/api/floorplan/image", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	h.uploadImage(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("uploadImage status = %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
}

func TestHandlerGetCalibration(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert calibration data
	_, err = db.Exec(`
		INSERT INTO floorplan (id, cal_ax, cal_ay, cal_bx, cal_by, distance_m, rotation_deg)
		VALUES (1, 100, 100, 500, 100, 5.0, 0.0)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Test get calibration
	req := httptest.NewRequest("GET", "/api/floorplan/calibrate", nil)
	w := httptest.NewRecorder()

	h.getCalibration(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("getCalibration status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var calResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&calResp); err != nil {
		t.Fatal(err)
	}

	// Verify values
	if calResp["cal_ax"].(float64) != 100 {
		t.Errorf("cal_ax = %v, want 100", calResp["cal_ax"])
	}
	if calResp["distance_m"].(float64) != 5.0 {
		t.Errorf("distance_m = %v, want 5.0", calResp["distance_m"])
	}
}

func TestHandlerGetFloorplanEmpty(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Test get floorplan when empty
	req := httptest.NewRequest("GET", "/api/floorplan", nil)
	w := httptest.NewRecorder()

	h.getFloorplan(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("getFloorplan status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var fpResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&fpResp); err != nil {
		t.Fatal(err)
	}

	if fpResp["image_url"] != nil {
		t.Errorf("image_url = %v, want nil", fpResp["image_url"])
	}
}

func TestGetCalibration(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert calibration data
	_, err = db.Exec(`
		INSERT INTO floorplan (id, cal_ax, cal_ay, cal_bx, cal_by, distance_m, rotation_deg)
		VALUES (1, 100, 100, 500, 100, 5.0, 0.0)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Test GetCalibration method
	mpp, rot, ok := h.GetCalibration(context.Background())
	if !ok {
		t.Fatal("GetCalibration returned ok=false, want true")
	}

	expectedMPP := 5.0 / 400.0 // 5 meters / 400 pixels
	if mpp != expectedMPP {
		t.Errorf("meters_per_pixel = %f, want %f", mpp, expectedMPP)
	}
	if rot != 0.0 {
		t.Errorf("rotation_deg = %f, want 0.0", rot)
	}
}

func TestGetCalibrationNotSet(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema (empty)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Test GetCalibration method when not set
	mpp, rot, ok := h.GetCalibration(context.Background())
	if ok {
		t.Fatal("GetCalibration returned ok=true, want false")
	}
	if mpp != 0 || rot != 0 {
		t.Errorf("GetCalibration returned non-zero values when not set: mpp=%f, rot=%f", mpp, rot)
	}
}

func TestGetImagePath(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create handler
	h := NewHandler(db, tmpDir)

	// Initially, no image
	path := h.GetImagePath()
	if path != "" {
		t.Errorf("GetImagePath = %s, want empty string", path)
	}

	// Create a test image file
	imagePath := filepath.Join(tmpDir, "floorplan", DefaultImageFilename)
	if err := os.MkdirAll(filepath.Dir(imagePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Now should return the path
	path = h.GetImagePath()
	if path == "" {
		t.Error("GetImagePath returned empty string, want non-empty")
	}
}

func TestUploadImageMissingFile(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Test upload without file field
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/floorplan/image", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	h.uploadImage(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("uploadImage status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestCalibrateInvalidDistance(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Test with negative distance
	calReq := calibrateRequest{
		AX:        100,
		AY:        100,
		BX:        500,
		BY:        100,
		DistanceM: -1.0,
	}

	body, _ := json.Marshal(calReq)
	req := httptest.NewRequest("POST", "/api/floorplan/calibrate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.calibrate(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("calibrate status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestCalibratePointsTooClose(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := NewHandler(db, tmpDir)

	// Test with points too close (5 pixels apart)
	calReq := calibrateRequest{
		AX:        100,
		AY:        100,
		BX:        105,
		BY:        100,
		DistanceM: 1.0,
	}

	body, _ := json.Marshal(calReq)
	req := httptest.NewRequest("POST", "/api/floorplan/calibrate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.calibrate(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("calibrate status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestGetImageNotFound(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "floorplan-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS floorplan (
			id              INTEGER PRIMARY KEY CHECK (id = 1),
			image_path      TEXT,
			cal_ax          REAL,
			cal_ay          REAL,
			cal_bx          REAL,
			cal_by          REAL,
			distance_m      REAL,
			rotation_deg    REAL,
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Create handler (no image file exists)
	h := NewHandler(db, tmpDir)

	// Test get image when none exists
	req := httptest.NewRequest("GET", "/api/floorplan/image", nil)
	w := httptest.NewRecorder()

	h.getImage(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("getImage status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
