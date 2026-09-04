# NodeInfo `free_heap_bytes` — verification

**Date:** 2026-09-04
**Bead:** `spaxel-527ed77b` — "Verify NodeInfo free_heap_bytes field"
**Verified at:** working tree on `main` (HEAD `385e3559`), `mothership/internal/ingestion`
package tests green, `go vet` clean.

---

## 1. The premise, corrected

The dispatch asks to verify that `NodeInfo.proto` has `free_heap_bytes`
correctly defined. **`NodeInfo.proto` does not exist, and neither does any
other `.proto` file** — not in the working tree, not anywhere in git history,
and no protobuf package is imported by any Go module in the repo:

```bash
find . -name '*.proto' -not -path './.git/*'            # 0 files
git log --all --diff-filter=A --name-only --pretty=format: -- '*.proto' \
  | grep -c proto                                       # 0 — never existed
grep -rn 'google.golang.org/protobuf' --include='*.go' mothership/ cmd/ test/   # no hits
```

This is the settled conclusion of the five-bead protobuf survey chain
([protobuf-survey-response-messages.md](protobuf-survey-response-messages.md)
is its capstone and carries the full evidence). Spaxel's wire formats are
JSON plus two hand-rolled binary frames; there is no generated protobuf layer
to verify anything against.

Two protobuf-shaped artifacts sit in the dependency graph and neither is
spaxel code, neither reaches the wire: `google.golang.org/protobuf` as an
`// indirect` module requirement, and ESP-IDF's `protobuf-c` component,
which `firmware/main/` never references. Neither changes the answer.

## 2. Which "NodeInfo" the task means

Three distinct Go types are named `NodeInfo` — always read them
package-qualified:

| Type | File | Role | Heap field? |
|---|---|---|---|
| `ingestion.NodeInfo` | `internal/ingestion/server.go:1056` | **Wire** — a connected node's state on the dashboard WebSocket | **yes** |
| `doctor.NodeInfo` | `internal/doctor/doctor.go:46` | Internal — `struct { MAC string }` for token-consistency checks | no |
| `fleet.NodeInfo` | `internal/fleet/optimiser.go:26` | Internal — optimiser input (position + capabilities) | no |

Only the first is a wire message, so it is the one the task can mean. The
other two are internal domain values with no serialiser; the survey's step 4
documented this exact trap (26 of its 63 `*Info`/`*Status`/`*Result`-shaped
types are not messages at all).

## 3. The verification, against the encoding that exists

`ingestion.NodeInfo` is marshalled **directly** by the dashboard hub —
`json.Marshal(nodes)` at `internal/dashboard/hub.go:773` for the 10 Hz delta
feed and via the `snap["nodes"]` map at `hub.go:670` for the first-message
snapshot — so the struct's json tags **are** the wire schema. A traced
serialisation site, the strongest evidence class the survey chain used.

The struct, verbatim:

```go
// internal/ingestion/server.go:1056
type NodeInfo struct {
	MAC             string  `json:"mac"`
	FirmwareVersion string  `json:"firmware_version,omitempty"`
	Chip            string  `json:"chip,omitempty"`
	Unpaired        bool    `json:"unpaired,omitempty"`
	FreeHeapBytes   int64   `json:"free_heap_bytes,omitempty"`
}
```

It is filled at `server.go:1077` from `nc.LastHealth.FreeHeapBytes`, which
arrives on the `health` message (`message.go:43`, firmware sender
`websocket.c:571` via `esp_get_free_heap_size()`).

### Acceptance criteria, each restated for JSON

The dispatch's criteria are protobuf vocabulary. Restated for the encoding
that exists and verified live:

| As written | As verified | Result |
|---|---|---|
| "`free_heap_bytes` field present in NodeInfo" | `NodeInfo.FreeHeapBytes` exists and marshals to the `free_heap_bytes` key | ✅ |
| "field uses appropriate protobuf type" (`uint32` or `int64`) | **`int64`** — matches `HealthMessage.FreeHeapBytes` (`message.go:43`), the value this field is populated from; `uint32` would also have been within range (ESP32-S3 heap is ≪4 GB) but would have introduced a second width for one measurement | ✅ |
| "field numbering is valid and non-conflicting" | JSON has no field numbers. The equivalent hazards are a **duplicated key** (encoding/json silently drops one on unmarshal) and a **case-only collision**. Neither is present: the struct declares five distinct snake_case keys, and the full set marshals to five distinct keys | ✅ |

### Two properties checked beyond the criteria

- **`omitempty` is correct here.** A node that has connected but whose first
  `health` tick has not arrived has *no heap reading*. Omitting the key keeps
  "unknown" distinct from "0 bytes free". This is unlike `HealthMessage`,
  where the field is required (non-`omitempty`) because that message exists
  to carry a reading.
- **No field-number analogue to worry about going forward.** Because JSON
  keys are matched by name and unknown keys are ignored on unmarshal (proven
  by `internal/ingestion/json_fuzz_test.go:117`), the field is
  wire-compatible in both directions without a registry of assigned numbers.

## 4. Conclusion and the residual

**Pass.** `free_heap_bytes` is present on `ingestion.NodeInfo`, typed
`int64`, uniquely and correctly keyed, populated from the node's latest
health report, and distinguishable from a zero reading. Nothing needs
changing.

**Residual — not this bead's scope:** `free_heap_bytes` is the *only*
heap-bearing field on `NodeInfo`, and it is absent from the two node
messages where heap is still most diagnostic: `HelloMessage`
(`message.go:11`) and `OTAStatusMessage` (`message.go:82`). That gap is
owned by the open chain `spaxel-3278a63f` → `spaxel-63ca0e88` documented in
[protobuf-survey-response-messages.md §4.1](protobuf-survey-response-messages.md)
and is **unimplemented as of this writing** — do not mistake this bead's
pass for the chain's completion.

## 5. Durable artifact

The verification is now machine-checkable rather than prose-only:
`mothership/internal/ingestion/nodeinfo_test.go`, table-driven:

- `TestNodeInfo_FreeHeapBytesField` — field present, type `int64`, exact json
  tag, no duplicate wire key among the struct's keys, marshal/unmarshal
  round-trip, and zero-valued omission behaviour.
- `TestGetConnectedNodesInfo_PopulatesFreeHeapBytes` — populated from
  `LastHealth`, left unset when no health report has arrived.

Both document in their comments why the assertions are the JSON restatement
of the protobuf checks the dispatch asked for, so the next reader does not
re-chase `NodeInfo.proto`.

## Reproduce

```bash
# the premise
find . -name '*.proto' -not -path './.git/*'
git log --all --diff-filter=A --name-only --pretty=format: -- '*.proto' | grep -c proto

# the three NodeInfo types
grep -rn '^type NodeInfo struct' mothership/ --include='*.go'

# the wire type and its serialisation site
sed -n '1056,1063p' mothership/internal/ingestion/server.go
grep -n 'GetConnectedNodesInfo' mothership/internal/dashboard/hub.go

# the durable test
cd mothership && go test ./internal/ingestion/ -run 'TestNodeInfo|TestGetConnectedNodesInfo' -v
```
