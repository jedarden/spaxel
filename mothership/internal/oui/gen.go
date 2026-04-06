// Package oui provides IEEE OUI (Organizationally Unique Identifier) lookup
// for MAC addresses to determine manufacturer names.
//
// Generate OUI data with: go generate
//go:generate go run gen_data.go > oui_data.go
package oui

// LookupOUI returns the manufacturer name for the first 3 bytes (OUI) of a MAC address.
// Returns empty string if the OUI is not found.
func LookupOUI(mac []byte) string {
	if len(mac) < 3 {
		return ""
	}
	key := uint32(mac[0])<<16 | uint32(mac[1])<<8 | uint32(mac[2])
	if name, ok := ouiMap[key]; ok {
		return name
	}
	return ""
}
