# CSI Recording Files Search Results

**Search Date:** 2026-08-29  
**Bead ID:** spaxel-f7b5d01c  
**Search Method:** grep and find commands with basic patterns (csi, recording, replay)

## Summary Statistics
- **Total Go files with recording/replay code:** 46
- **Total CSI-related documentation files:** 11
- **Binary recording files found:** 13

## Raw File Paths from Search Results

### Binary Recording Files
```
./data/csi_replay.bin
./testdata/csi_session_mixed_activity.bin
```

### Core Recording Modules (Go)
```
./mothership/internal/recording/benchmark.go
./mothership/internal/recording/buffer.go
./mothership/internal/recording/buffer_test.go
./mothership/internal/recording/compression.go
./mothership/internal/recorder/manager.go
./mothership/internal/recorder/segment.go
```

### Replay Engine Modules (Go)
```
./mothership/internal/replay/buffer_adapter.go
./mothership/internal/replay/engine.go
./mothership/internal/replay/engine_test.go
./mothership/internal/replay/integration_test.go
./mothership/internal/replay/pipeline.go
./mothership/internal/replay/pipeline_test.go
./mothership/internal/replay/seek_fuzz_test.go
./mothership/internal/replay/session.go
./mothership/internal/replay/store.go
./mothership/internal/replay/store_test.go
./mothership/internal/replay/types.go
./mothership/internal/replay/verify_recording_test.go
./mothership/internal/replay/worker.go
```

### API and Integration Files (Go)
```
./mothership/internal/api/replay.go
./mothership/internal/api/replay_test.go
./mothership/internal/ingestion/server.go
./mothership/cmd/mothership/main.go
```

### Test Data Generation Tools
```
./testdata/generate_csi_recording.go
./testdata/verify_recording.go
```

### Test Files
```
./test/acceptance/as6_replay_test.go
./mothership/test/acceptance/as6_replay_test.go
```

### Firmware CSI Implementation
```
./firmware/main/csi.c
./firmware/main/csi.h
./firmware/test/test_csi_frame.c
```

### Dashboard Replay UI
```
./dashboard/css/replay.css
./dashboard/js/replay.js
./dashboard/js/replay.test.js
```

### Scripts and Utilities
```
./scripts/measure_csi_rate.py
```

### Documentation Files (Markdown)
```
./docs/notes/csi-format-examples.md
./docs/notes/csi-format-validation-notes.md
./docs/notes/csi-io-code-paths.md
./docs/notes/csi-recording-file-format.md
./docs/notes/csi-recording-format.md
./docs/notes/csi-recording-io-code-locations.md
./docs/notes/csi-recording-io-code-paths.md
./docs/notes/csi-recording-module-structure.md
./docs/research/csi-recording-module-structure.md
./docs/research/01-csi-fundamentals.md
./docs/research/papers/intel-5300-csi-tool.md
```

## Search Commands Used

1. **Find by filename patterns:**
   ```bash
   find . -type f \( -name "*csi*" -o -name "*recording*" \)
   find . -type f -name "*replay*"
   ```

2. **Grep for content patterns in Go files:**
   ```bash
   find . -type f -name "*.go" -exec grep -l "csi.*recording\|recording.*csi\|csi_replay\|replay.*csi" {} \;
   find . -type f -name "*.go" -exec grep -l "replay\|recording" {} \;
   ```

3. **Search for binary file references:**
   ```bash
   grep -r "csi_replay\.bin\|recording.*\.bin" . --include="*.go" --include="*.md"
   ```

4. **Documentation search:**
   ```bash
   find ./docs -type f -name "*csi*" -o -name "*recording*" -o -name "*replay*"
   find ./docs -type f \( -name "*.md" \) -exec grep -l "csi.*record\|record.*csi\|replay.*csi\|csi.*replay" {} \;
   ```

## Key Findings

1. **Primary recording storage:** `/data/csi_replay.bin` (append-only binary format)
2. **Core recording implementation:** `mothership/internal/recording/` module
3. **Replay engine:** `mothership/internal/replay/` module with full pipeline support
4. **Test data generation:** `testdata/` contains generation and verification tools
5. **CSI firmware implementation:** `firmware/main/csi.c` and `csi.h`
6. **Dashboard UI:** Complete replay interface with CSS/JS/test files

## Component 14 Reference

Per plan.md, CSI recording functionality includes:
- **Retention:** 48 hours default (configurable via `SPAXEL_REPLAY_MAX_MB`)
- **Storage estimate:** ~150 bytes/frame × 30 Hz × 20 links = ~7.5 MB/hour
- **File format:** Append-only binary with 64-byte header + per-frame records
- **Features:** Time-travel debugging, parameter tuning overlay, seek/replay controls

---

**Search completed successfully. All CSI recording files identified using basic patterns.**
