package ble

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ble-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "ble.db")
	reg, err := NewRegistry(dbPath)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}

func TestProcessRelayMessage(t *testing.T) {
	reg := setupTestRegistry(t)
	defer reg.Close()

	// Create sample BLE observations
	devices := []BLEObservation{
		{
			Addr:       "aa:bb:cc:dd:ee:01",
			Name:       "iPhone",
			MfrID:      0x004C, // Apple
			MfrDataHex: "4c001005031c9e6b43",
			RSSIdBm:    -65,
		},
		{
			Addr:       "aa:bb:cc:dd:ee:02",
			Name:       "Fitbit Charge",
			MfrID:      0x009E, // Fitbit
			MfrDataHex: "",
			RSSIdBm:    -70,
		},
	}

	// Process the relay message
	err := reg.ProcessRelayMessage("node:11:22:33:44:55", devices)
	if err != nil {
		t.Fatalf("ProcessRelayMessage failed: %v", err)
	}

	// Verify devices were created
	devs, err := reg.GetDevices(false)
	if err != nil {
		t.Fatalf("GetDevices failed: %v", err)
	}

	if len(devs) != 2 {
		t.Errorf("Expected 2 devices, got %d", len(devs))
	}

	// Check Apple device type was detected
	appleDev, err := reg.GetDevice("aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	if appleDev.Manufacturer != "Apple" {
		t.Errorf("Expected manufacturer 'Apple', got '%s'", appleDev.Manufacturer)
	}

	// Check Fitbit device type was detected
	fitbitDev, err := reg.GetDevice("aa:bb:cc:dd:ee:02")
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	if fitbitDev.DeviceType != DeviceTypeFitbit {
		t.Errorf("Expected device type 'fitbit', got '%s'", fitbitDev.DeviceType)
	}
}

func TestDeviceTypeDetection(t *testing.T) {
	tests := []struct {
		name       string
		mfrID      int
		mfrDataHex string
		deviceName string
		wantType   DeviceType
		wantMfr    string
	}{
		{
			name:       "Apple iPhone",
			mfrID:      0x004C,
			mfrDataHex: "4c001005031c9e6b43",
			deviceName: "iPhone",
			wantType:   DeviceTypeApplePhone,
			wantMfr:    "Apple",
		},
		{
			name:       "Apple AirPods",
			mfrID:      0x004C,
			mfrDataHex: "4c0009",
			deviceName: "AirPods",
			wantType:   DeviceTypeAppleEarbuds,
			wantMfr:    "Apple",
		},
		{
			name:       "Fitbit",
			mfrID:      0x009E,
			mfrDataHex: "",
			deviceName: "Fitbit Charge",
			wantType:   DeviceTypeFitbit,
			wantMfr:    "Fitbit",
		},
		{
			name:       "Garmin",
			mfrID:      0x0157,
			mfrDataHex: "",
			deviceName: "Garmin Forerunner",
			wantType:   DeviceTypeGarmin,
			wantMfr:    "Garmin",
		},
		{
			name:       "Samsung",
			mfrID:      0x0075,
			mfrDataHex: "",
			deviceName: "Galaxy S21",
			wantType:   DeviceTypeSamsung,
			wantMfr:    "Samsung",
		},
		{
			name:       "Microsoft",
			mfrID:      0x0006,
			mfrDataHex: "",
			deviceName: "Surface",
			wantType:   DeviceTypeMicrosoft,
			wantMfr:    "Microsoft",
		},
		{
			name:       "Ruuvi",
			mfrID:      0x0499,
			mfrDataHex: "",
			deviceName: "RuuviTag",
			wantType:   DeviceTypeRuuvi,
			wantMfr:    "Ruuvi",
		},
		{
			name:       "Nordic/Tile",
			mfrID:      0x0059,
			mfrDataHex: "",
			deviceName: "Tile",
			wantType:   DeviceTypeTile,
			wantMfr:    "Nordic",
		},
		{
			name:       "Google",
			mfrID:      0x00E0,
			mfrDataHex: "",
			deviceName: "Pixel",
			wantType:   DeviceTypeGoogle,
			wantMfr:    "Google",
		},
		{
			name:       "Unknown manufacturer",
			mfrID:      0xFFFF,
			mfrDataHex: "",
			deviceName: "Some Device",
			wantType:   DeviceTypeUnknown,
			wantMfr:    "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deviceType, manufacturer := detectDeviceTypeAndManufacturer(tt.mfrID, tt.mfrDataHex, tt.deviceName)
			if deviceType != tt.wantType {
				t.Errorf("detectDeviceTypeAndManufacturer() type = %v, want %v", deviceType, tt.wantType)
			}
			if manufacturer != tt.wantMfr {
				t.Errorf("detectDeviceTypeAndManufacturer() mfr = %v, want %v", manufacturer, tt.wantMfr)
			}
		})
	}
}

func TestArchiveStale(t *testing.T) {
	reg := setupTestRegistry(t)
	defer reg.Close()

	// Create a device and process it
	devices := []BLEObservation{
		{
			Addr:    "aa:bb:cc:dd:ee:01",
			Name:    "Test Device",
			RSSIdBm: -65,
		},
	}
	reg.ProcessRelayMessage("node:11:22:33:44:55", devices)

	// Verify device is not archived
	devs, _ := reg.GetDevices(false)
	if len(devs) != 1 {
		t.Fatalf("Expected 1 device, got %d", len(devs))
	}
	if devs[0].IsArchived {
		t.Error("Device should not be archived")
	}

	// Archive stale devices older than 1 nanosecond (essentially all devices)
	count, err := reg.ArchiveStale(1 * time.Nanosecond)
	if err != nil {
		t.Fatalf("ArchiveStale failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 device archived, got %d", count)
	}

	// Verify device is now archived
	devs, _ = reg.GetDevices(false)
	if len(devs) != 0 {
		t.Errorf("Expected 0 non-archived devices, got %d", len(devs))
	}

	// Verify device appears when including archived
	devs, _ = reg.GetDevices(true)
	if len(devs) != 1 {
		t.Errorf("Expected 1 device including archived, got %d", len(devs))
	}
}

func TestPeople(t *testing.T) {
	reg := setupTestRegistry(t)
	defer reg.Close()

	// Create a person
	person, err := reg.CreatePerson("Alice", "#ff5722")
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	if person.Name != "Alice" {
		t.Errorf("Expected name 'Alice', got '%s'", person.Name)
	}
	if person.Color != "#ff5722" {
		t.Errorf("Expected color '#ff5722', got '%s'", person.Color)
	}
	if person.ID == "" {
		t.Error("Person ID should not be empty")
	}

	// Get people
	people, err := reg.GetPeople()
	if err != nil {
		t.Fatalf("GetPeople failed: %v", err)
	}
	if len(people) != 1 {
		t.Errorf("Expected 1 person, got %d", len(people))
	}

	// Get single person
	p, err := reg.GetPerson(person.ID)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}
	if p.Name != "Alice" {
		t.Errorf("Expected name 'Alice', got '%s'", p.Name)
	}

	// Update person
	err = reg.UpdatePerson(person.ID, "Bob", "#3b82f6")
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}
	p, _ = reg.GetPerson(person.ID)
	if p.Name != "Bob" {
		t.Errorf("Expected name 'Bob', got '%s'", p.Name)
	}
	if p.Color != "#3b82f6" {
		t.Errorf("Expected color '#3b82f6', got '%s'", p.Color)
	}
}

func TestAssignToPerson(t *testing.T) {
	reg := setupTestRegistry(t)
	defer reg.Close()

	// Create a person
	person, _ := reg.CreatePerson("Alice", "#ff5722")

	// Create a device
	devices := []BLEObservation{
		{
			Addr:    "aa:bb:cc:dd:ee:01",
			Name:    "Alice's iPhone",
			MfrID:   0x004C,
			RSSIdBm: -65,
		},
	}
	reg.ProcessRelayMessage("node:11:22:33:44:55", devices)

	// Assign device to person
	err := reg.AssignToPerson("aa:bb:cc:dd:ee:01", person.ID)
	if err != nil {
		t.Fatalf("AssignToPerson failed: %v", err)
	}

	// Verify assignment
	dev, err := reg.GetDevice("aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	if dev.PersonID != person.ID {
		t.Errorf("Expected person_id '%s', got '%s'", person.ID, dev.PersonID)
	}

	// Get person's devices
	personDevs, err := reg.GetPersonDevices(person.ID)
	if err != nil {
		t.Fatalf("GetPersonDevices failed: %v", err)
	}
	if len(personDevs) != 1 {
		t.Errorf("Expected 1 device, got %d", len(personDevs))
	}

	// Unassign device
	err = reg.UnassignFromPerson("aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("UnassignFromPerson failed: %v", err)
	}
	dev, _ = reg.GetDevice("aa:bb:cc:dd:ee:01")
	if dev.PersonID != "" {
		t.Errorf("Expected empty person_id, got '%s'", dev.PersonID)
	}
}

func TestDeletePerson(t *testing.T) {
	reg := setupTestRegistry(t)
	defer reg.Close()

	// Create a person and device
	person, _ := reg.CreatePerson("Alice", "#ff5722")
	devices := []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "Test", RSSIdBm: -65},
	}
	reg.ProcessRelayMessage("node:11:22:33:44:55", devices)
	reg.AssignToPerson("aa:bb:cc:dd:ee:01", person.ID)

	// Delete person
	err := reg.DeletePerson(person.ID)
	if err != nil {
		t.Fatalf("DeletePerson failed: %v", err)
	}

	// Verify person is gone
	_, err = reg.GetPerson(person.ID)
	if err == nil {
		t.Error("Expected error getting deleted person")
	}

	// Verify device still exists but is unassigned
	dev, err := reg.GetDevice("aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("Device should still exist: %v", err)
	}
	if dev.PersonID != "" {
		t.Errorf("Device should be unassigned, got person_id '%s'", dev.PersonID)
	}
}

func TestDetectPossibleDuplicates(t *testing.T) {
	reg := setupTestRegistry(t)
	defer reg.Close()

	// Create two devices with the same name (MAC rotation scenario)
	devices := []BLEObservation{
		{
			Addr:       "aa:bb:cc:dd:ee:01",
			Name:       "iPhone",
			MfrID:      0x004C,
			MfrDataHex: "4c001005031c9e6b43",
			RSSIdBm:    -65,
		},
	}
	reg.ProcessRelayMessage("node:11:22:33:44:55", devices)

	// Wait a bit to ensure time difference
	time.Sleep(100 * time.Millisecond)

	devices2 := []BLEObservation{
		{
			Addr:       "aa:bb:cc:dd:ee:02",
			Name:       "iPhone",
			MfrID:      0x004C,
			MfrDataHex: "4c001005031c9e6b43",
			RSSIdBm:    -68,
		},
	}
	reg.ProcessRelayMessage("node:11:22:33:44:55", devices2)

	// Detect duplicates
	duplicates, err := reg.DetectPossibleDuplicates()
	if err != nil {
		t.Fatalf("DetectPossibleDuplicates failed: %v", err)
	}

	if len(duplicates) == 0 {
		t.Error("Expected to detect possible duplicates")
	}

	if len(duplicates) > 0 {
		dup := duplicates[0]
		if dup.Confidence < 0.3 {
			t.Errorf("Expected confidence >= 0.3, got %f", dup.Confidence)
		}
		if dup.Reason == "" {
			t.Error("Expected reason to be set")
		}
	}
}

func TestMergeDevices(t *testing.T) {
	reg := setupTestRegistry(t)
	defer reg.Close()

	// Create two devices
	devices := []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "iPhone", MfrID: 0x004C, RSSIdBm: -65},
	}
	reg.ProcessRelayMessage("node:11:22:33:44:55", devices)

	devices2 := []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:02", Name: "iPhone", MfrID: 0x004C, RSSIdBm: -68},
	}
	reg.ProcessRelayMessage("node:11:22:33:44:55", devices2)

	// Create a person and assign to second device
	person, _ := reg.CreatePerson("Alice", "#ff5722")
	reg.AssignToPerson("aa:bb:cc:dd:ee:02", person.ID)

	// Merge devices (keep 01, remove 02)
	err := reg.MergeDevices("aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02")
	if err != nil {
		t.Fatalf("MergeDevices failed: %v", err)
	}

	// Verify first device exists and has person assigned
	dev, err := reg.GetDevice("aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	if dev.PersonID != person.ID {
		t.Errorf("Expected merged device to have person_id, got '%s'", dev.PersonID)
	}

	// Verify second device is gone
	_, err = reg.GetDevice("aa:bb:cc:dd:ee:02")
	if err == nil {
		t.Error("Expected second device to be deleted")
	}
}

func TestArchiveDevice(t *testing.T) {
	reg := setupTestRegistry(t)
	defer reg.Close()

	// Create a device
	devices := []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "Test", RSSIdBm: -65},
	}
	reg.ProcessRelayMessage("node:11:22:33:44:55", devices)

	// Archive the device
	err := reg.ArchiveDevice("aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("ArchiveDevice failed: %v", err)
	}

	// Verify device is archived
	devs, _ := reg.GetDevices(false)
	if len(devs) != 0 {
		t.Error("Expected device to be archived")
	}

	devs, _ = reg.GetDevices(true)
	if len(devs) != 1 {
		t.Error("Expected device to exist when including archived")
	}

	// Unarchive the device
	err = reg.UnarchiveDevice("aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("UnarchiveDevice failed: %v", err)
	}

	devs, _ = reg.GetDevices(false)
	if len(devs) != 1 {
		t.Error("Expected device to be unarchived")
	}
}

func TestRSSICache(t *testing.T) {
	cache := NewRSSICache(5 * time.Second)

	// Add observations
	cache.Add("aa:bb:cc:dd:ee:01", "node:11:22:33:44:55", -65)
	cache.Add("aa:bb:cc:dd:ee:01", "node:66:77:88:99:aa", -70)

	// Get observations
	obs := cache.Get("aa:bb:cc:dd:ee:01")
	if len(obs) != 2 {
		t.Errorf("Expected 2 observations, got %d", len(obs))
	}

	// Get recent observations
	obs = cache.GetRecent("aa:bb:cc:dd:ee:01", 1*time.Second)
	if len(obs) != 2 {
		t.Errorf("Expected 2 recent observations, got %d", len(obs))
	}
}

func TestGetPeopleWithDevices(t *testing.T) {
	reg := setupTestRegistry(t)
	defer reg.Close()

	// Create two people
	person1, _ := reg.CreatePerson("Alice", "#ff5722")
	person2, _ := reg.CreatePerson("Bob", "#3b82f6")

	// Create devices and assign to people
	devices := []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "Alice's iPhone", MfrID: 0x004C, RSSIdBm: -65},
		{Addr: "aa:bb:cc:dd:ee:02", Name: "Alice's Watch", MfrID: 0x004C, RSSIdBm: -70},
		{Addr: "aa:bb:cc:dd:ee:03", Name: "Bob's Phone", MfrID: 0x004C, RSSIdBm: -68},
	}
	reg.ProcessRelayMessage("node:11:22:33:44:55", devices)

	reg.AssignToPerson("aa:bb:cc:dd:ee:01", person1.ID)
	reg.AssignToPerson("aa:bb:cc:dd:ee:02", person1.ID)
	reg.AssignToPerson("aa:bb:cc:dd:ee:03", person2.ID)

	// Get people with devices
	peopleWithDevices, err := reg.GetPeopleWithDevices()
	if err != nil {
		t.Fatalf("GetPeopleWithDevices failed: %v", err)
	}

	if len(peopleWithDevices) != 2 {
		t.Errorf("Expected 2 people, got %d", len(peopleWithDevices))
	}

	// Find Alice and check device count
	for _, p := range peopleWithDevices {
		if p["name"] == "Alice" {
			if p["device_count"].(int) != 2 {
				t.Errorf("Expected Alice to have 2 devices, got %d", p["device_count"])
			}
		}
		if p["name"] == "Bob" {
			if p["device_count"].(int) != 1 {
				t.Errorf("Expected Bob to have 1 device, got %d", p["device_count"])
			}
		}
	}
}

func TestIsLikelyWearable(t *testing.T) {
	tests := []struct {
		deviceType DeviceType
		expected   bool
	}{
		{DeviceTypeAppleWatch, true},
		{DeviceTypeFitbit, true},
		{DeviceTypeGarmin, true},
		{DeviceTypeApplePhone, false},
		{DeviceTypeTile, false},
		{DeviceTypeUnknown, false},
	}

	for _, tt := range tests {
		result := isLikelyWearable(tt.deviceType)
		if result != tt.expected {
			t.Errorf("isLikelyWearable(%v) = %v, want %v", tt.deviceType, result, tt.expected)
		}
	}
}

// Helper function to set up a test registry
func setupTestRegistry(t *testing.T) *Registry {
	tmpDir, err := os.MkdirTemp("", "ble-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "ble.db")
	reg, err := NewRegistry(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Set cleanup to remove temp dir after registry is closed
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	return reg
}
