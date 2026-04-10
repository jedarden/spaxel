#!/usr/bin/env python3
"""Fix BLE handlers in main.go by replacing inline handlers with proper ble.Handler registration."""

# Read the file
with open('mothership/cmd/mothership/main.go', 'r') as f:
    lines = f.readlines()

# Lines 2072-2159 (0-indexed: 2071-2158) contain the inline BLE handlers
# We need to replace these with the proper ble.Handler registration

# Keep everything before line 2072
new_lines = lines[:2071]

# Add the new BLE handler registration
new_lines.append('		// Phase 6: BLE REST API\n')
new_lines.append('		if bleRegistry != nil {\n')
new_lines.append('			bleHandler := ble.NewHandler(bleRegistry)\n')
new_lines.append('			bleHandler.RegisterRoutes(r)\n')
new_lines.append('			log.Printf("[INFO] BLE REST API registered at /api/ble/* and /api/people/*")\n')
new_lines.append('\n')
new_lines.append('			// BLE identity matches endpoint (not in ble.Handler)\n')
new_lines.append('			r.Get("/api/ble/matches", func(w http.ResponseWriter, r *http.Request) {\n')
new_lines.append('				if identityMatcher == nil {\n')
new_lines.append('					writeJSON(w, []*ble.IdentityMatch{})\n')
new_lines.append('					return\n')
new_lines.append('				}\n')
new_lines.append('				matches := identityMatcher.GetAllMatches()\n')
new_lines.append('				writeJSON(w, matches)\n')
new_lines.append('			})\n')
new_lines.append('		}\n')

# Keep everything after line 2159
new_lines.extend(lines[2159:])

# Write back
with open('mothership/cmd/mothership/main.go', 'w') as f:
    f.writelines(new_lines)

print("Successfully updated main.go")
print("Replaced inline BLE handlers (lines 2072-2159) with ble.Handler.RegisterRoutes(r)")
