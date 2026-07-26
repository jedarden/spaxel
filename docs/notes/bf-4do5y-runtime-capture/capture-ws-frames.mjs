// capture-ws-frames.mjs — bf-16tsv diagnosis tool (v3: binary vs text aware).
//
// Captures RAW /ws/dashboard frames directly off the WebSocket (no browser) and
// classifies EVERY frame, correctly separating BINARY frames (raw CSI, broadcast
// by hub.BroadcastCSI) from TEXT frames (JSON). v2 conflated both into a single
// "<no-type>" bucket because it ran JSON.parse on every frame and binary frames
// failed to parse — so its "frames_carrying_blobs: 0" only covered the JSON
// frames it could parse, leaving the high-rate binary stream unaccounted for.
//
// v3 uses the ws message event's isBinary flag so the conclusion is airtight:
//   (a) TEXT/JSON frames CARRY blobs -> emission OK, defect downstream; or
//   (b) TEXT/JSON frames carry NO blobs (while /api/blobs has blobs)
//       -> defect is mothership emission.
//
// Diagnostic capture tool, not production code (bead bf-16tsv is diagnosis-only).
//   node capture-ws-frames.mjs --port 8088 --duration 12000

import { writeFileSync } from 'node:fs';
import WebSocket from '/home/coding/spaxel/dashboard/node_modules/ws/index.js';

const a = {};
for (let i = 2; i < process.argv.length; i++) {
    const k = process.argv[i]; if (k.startsWith('--')) a[k.slice(2)] = process.argv[++i];
}
const PORT = a.port || '8088';
const DURATION_MS = parseInt(a.duration || '12000', 10);
const OUT = a.out || '/home/coding/spaxel/docs/notes/bf-4do5y-runtime-capture/identity-less.ws-frames.json';
const BASE = `http://localhost:${PORT}`;
const WSURL = `ws://localhost:${PORT}/ws/dashboard`;

// Counters split by transport type.
let binaryFrames = 0;          // opcode 0x2 raw frames (CSI from BroadcastCSI)
let textFrames = 0;            // opcode 0x1 JSON frames

const typeHist = {};           // JSON type -> count  ('<no-type>' = JSON with no type field = 10Hz delta ticks)
const blobLenByType = {};      // JSON type -> {seen, maxLen}
const sampleByType = {};       // JSON type -> first raw frame (full, capped 8000 chars)
let snapshotRaw = null;        // full snapshot frame (uncapped-ish, 12000 chars)
const blobFieldCensus = {};    // keys seen across any blob object
const apiSamples = [];

// Find a blobs array at the top level OR one nesting level deep.
function findBlobs(parsed) {
    if (!parsed || typeof parsed !== 'object') return null;
    if (Array.isArray(parsed.blobs)) return parsed.blobs;
    if (parsed.data && Array.isArray(parsed.data.blobs)) return parsed.data.blobs;
    for (const k of Object.keys(parsed)) {
        const v = parsed[k];
        if (v && typeof v === 'object' && !Array.isArray(v) && Array.isArray(v.blobs)) return v.blobs;
    }
    return null;
}

function classifyText(raw) {
    let parsed = null;
    try { parsed = JSON.parse(raw); } catch (_) { typeHist['__unparseable_text__'] = (typeHist['__unparseable_text__'] || 0) + 1; return; }
    const t = (parsed && parsed.type !== undefined) ? parsed.type : '<no-type>';
    typeHist[t] = (typeHist[t] || 0) + 1;
    if (!sampleByType[t]) sampleByType[t] = raw.slice(0, 8000);
    if (parsed && parsed.type === 'snapshot' && !snapshotRaw) snapshotRaw = raw.slice(0, 12000);
    const arr = findBlobs(parsed);
    const len = arr === null ? -1 : arr.length;
    blobLenByType[t] = blobLenByType[t] || { seen: 0, maxLen: 0 };
    if (len >= 0) { blobLenByType[t].seen++; if (len > blobLenByType[t].maxLen) blobLenByType[t].maxLen = len; }
    if (arr) for (const b of arr) if (b && typeof b === 'object') for (const k of Object.keys(b)) blobFieldCensus[k] = (blobFieldCensus[k] || 0) + 1;
}

function sampleApi() {
    const t = Date.now();
    fetch(`${BASE}/api/blobs`).then(r => r.text()).then(txt => {
        let count = -1, first = null;
        try { const arr = JSON.parse(txt); count = Array.isArray(arr) ? arr.length : -1; first = count > 0 ? arr[0] : null; }
        catch (_) { count = -2; }
        apiSamples.push({ t_ms: t, count, first_blob: first });
    }).catch(e => apiSamples.push({ t_ms: t, error: String(e) }));
}

const ws = new WebSocket(WSURL);
const start = Date.now();
let apiTimer = null;

ws.on('open', () => {
    console.error(`[ws-cap] connected to ${WSURL}`);
    sampleApi();
    apiTimer = setInterval(sampleApi, 500);
});

// NOTE: ws v8 emits (data, isBinary). isBinary===true => raw CSI frame.
ws.on('message', (data, isBinary) => {
    if (isBinary) { binaryFrames++; return; }
    textFrames++;
    classifyText(data.toString());
});
ws.on('error', e => console.error('[ws-cap] ws error:', String(e)));
ws.on('close', (c, r) => console.error(`[ws-cap] closed: ${c} ${r}`));

setTimeout(() => {
    try { ws.close(); } catch (_) {}
    if (apiTimer) clearInterval(apiTimer);
    const apiNonZero = apiSamples.filter(s => s.count > 0);
    const textFramesWithBlobs = Object.values(blobLenByType).reduce((n, v) => n + v.seen, 0);
    const out = {
        meta: {
            ws_url: WSURL, port: PORT, captured_for_ms: Date.now() - start,
            binary_frames: binaryFrames,       // raw CSI (opcode 0x2), not JSON — cannot carry blob objects
            text_frames: textFrames,           // JSON frames (opcode 0x1)
            text_frames_carrying_blobs: textFramesWithBlobs,
            type_histogram_text: typeHist,
            blob_array_len_by_type: blobLenByType,
            blob_field_census: blobFieldCensus,
            api_samples: apiSamples.length,
            api_samples_with_blobs: apiNonZero.length,
            api_peak_blob_count: apiNonZero.reduce((m, s) => Math.max(m, s.count), 0),
        },
        snapshot_full: snapshotRaw,
        sample_text_frame_by_type: sampleByType,
        api_blob_samples: apiSamples,
    };
    writeFileSync(OUT, JSON.stringify(out, null, 2));
    console.error(`[ws-cap] wrote ${OUT}`);
    console.error(`[ws-cap] binary=${binaryFrames} text=${textFrames} textWithBlobs=${textFramesWithBlobs}`);
    console.error(`[ws-cap] text type histogram: ${JSON.stringify(typeHist)}`);
    console.error(`[ws-cap] blob len by type: ${JSON.stringify(blobLenByType)}`);
    console.error(`[ws-cap] /api/blobs samples_with_blobs=${apiNonZero.length}/${apiSamples.length} peak=${out.meta.api_peak_blob_count}`);
    process.exit(0);
}, DURATION_MS);
