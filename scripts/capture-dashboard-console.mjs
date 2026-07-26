// capture-dashboard-console.mjs — Playwright headless capture of the live
// dashboard's browser console + page errors against a running mothership.
//
// Used by bf-4do5y ("Verify live dashboard console is clean with identity-less
// blobs") — the live-browser complement to bf-20cl7 (jsdom renderer proof) and
// the NOT-resolved twin of bf-15oi (identity-RESOLVED mothership log). It loads
// the REAL dashboard pages served by the mothership, drives them against the
// live /ws/dashboard feed, and records every console message + uncaught error
// so they can be scanned for identity-field undefined access
// (personName / assignedColor / identityResolved).
//
// This is a runtime proof, not a unit test: a real Chromium, a real WebSocket,
// real identity-less (or identity-resolved) blobs from /api/blobs.
//
// Usage:
//   node scripts/capture-dashboard-console.mjs \
//     --base http://localhost:8088 \
//     --pages /ambient,/live \
//     --outdir /tmp/.../console \
//     --label identity-less \
//     [--blob-timeout 15000] [--settle 2000]
//
// Output: <outdir>/<label>.<slug>.console.txt (human console log) and
//         <outdir>/<label>.<slug>.json (structured: messages, pageErrors,
//         blobSeen, renderEvidence). Exit 0 always (capture is evidence; the
//         orchestrator asserts), non-zero only on a harness/Playwright failure.

import pw from '/home/coding/spaxel/dashboard/node_modules/playwright/index.js';
import { writeFileSync, mkdirSync } from 'node:fs';

// The bare `playwright` ESM module surfaces its API under the default export.
const { chromium } = pw.default || pw;

function parseArgs(argv) {
    const a = {};
    for (let i = 2; i < argv.length; i++) {
        const k = argv[i];
        const v = argv[++i];
        a[k.replace(/^--/, '')] = v;
    }
    return a;
}

const args = parseArgs(process.argv);
const base = args.base || 'http://localhost:8088';
const pages = (args.pages || '/ambient').split(',').map(s => s.trim()).filter(Boolean);
const outdir = args.outdir || '/tmp/spaxel-console-cap';
const label = args.label || 'run';
const blobTimeoutMs = parseInt(args['blob-timeout'] || '15000', 10);
const settleMs = parseInt(args.settle || '2000', 10);
// bf-5y3qt / bf-5t5ny: viz3d _blobs3D is populated ONLY during the brief
// sub-100ms windows when a walker is in-range at a 10Hz fusion-tick boundary
// (see the getBlobStates() contract in viz3d.js — applyLocUpdate() evicts any
// blob id absent from the current /ws/dashboard frame the instant it is
// omitted). A single-instant probe therefore almost always lands on an empty
// instant, and a coarse poll can step right over an in-range window. So:
//   • blobSeen  is polled at blobSeenPollMs (tightened from 250ms) so it can
//     actually catch those windows; it still early-breaks on the first hit.
//   • renderEvidence samples getBlobStates() across renderWindowMs at
//     renderSampleMs WITHOUT early-break, reporting the PEAK frame seen —
//     not one unlucky instant. viz3d.js itself is correct as designed and is
//     NOT changed (option C rejected; it would break the current-frame
//     contract documented on getBlobStates()).
const blobSeenPollMs = parseInt(args['blob-seen-poll-ms'] || '50', 10);
const renderWindowMs = parseInt(args['render-window'] || '3000', 10);
const renderSampleMs = parseInt(args['render-sample-ms'] || '50', 10);
mkdirSync(outdir, { recursive: true });

// Patterns whose presence would indicate an identity-field backward-compat
// regression in the browser (the bead's core acceptance criterion). Kept
// deliberately specific so benign browser-console noise (the literal word
// "undefined" in object dumps, "NaN" in coordinates, WebGL/swiftshader
// status lines under headless /live) does NOT false-positive — only genuine
// identity-field mentions or classic undefined-access runtime errors hit.
const IDENTITY_ERROR_RES = [
    // Any mention of an identity field name is directly in scope per the
    // acceptance criterion ("errors mentioning personName/assignedColor/
    // identityResolved ... or undefined access").
    /personName/i,
    /assignedColor/i,
    /identityResolved/i,
    /\bperson_label\b/i,
    /\bperson_color\b/i,
    /\bperson_name\b/i,
    /\bassigned_color\b/i,
    // Classic backward-compat failure modes — the shapes an identity-field
    // access regression actually takes in a browser (e.g. "Cannot read
    // properties of undefined (reading 'personName')").
    /cannot read propert(?:y|ies)/i,
    /\bof undefined\b/i,
    /is not a function/i,
    /is not defined/i,
    /TypeError:/i,
    /ReferenceError:/i,
];

function slug(p) {
    return p.replace(/^\/+/, '').replace(/[\/\s]/g, '_') || 'root';
}

// Scan the live ambient canvas for the identity-less fallback color (#6b7280 =
// rgb 107,114,128). drawPeople() uses exactly this grey for blobs that carry no
// person name. Seeing it on the canvas is direct, live proof that identity-less
// blobs render with the fallback color (not just that the console stayed quiet).
async function ambientFallbackEvidence(page) {
    return await page.evaluate(() => {
        const cvs = document.getElementById('ambient-canvas');
        if (!cvs) return { canvas: false };
        const ctx = cvs.getContext('2d');
        if (!ctx) return { canvas: true, ctx: false };
        const w = cvs.width, h = cvs.height;
        if (!w || !h) return { canvas: true, w, h };
        try {
            // sample every 3rd pixel to keep this cheap
            const data = ctx.getImageData(0, 0, w, h).data;
            let fallbackPx = 0;
            const match = (r, g, b) =>
                Math.abs(r - 107) <= 12 && Math.abs(g - 114) <= 12 && Math.abs(b - 128) <= 12;
            for (let i = 0; i < data.length; i += 12) {
                if (match(data[i], data[i + 1], data[i + 2])) fallbackPx++;
            }
            return { canvas: true, w, h, fallbackPx };
        } catch (e) {
            return { canvas: true, w, h, readError: String(e) };
        }
    });
}

async function capturePage(browser, path) {
    const context = await browser.newContext({
        viewport: { width: 1280, height: 800 },
        // Ensure the WebGL-backed /live page can render headlessly.
        args: [],
    });
    // Surface browser-side console + errors to the harness.
    const messages = [];
    const pageErrors = [];
    const page = await context.newPage();
    page.on('console', msg => {
        messages.push({ type: msg.type(), text: msg.text() });
    });
    page.on('pageerror', err => {
        pageErrors.push({ name: err.name, message: err.message, stack: err.stack || '' });
    });
    // Some dashboards log via console.error on failed fetches before the WS is
    // up; we record everything and assert later, so do not fail the nav on that.
    const url = base + path;
    let navOk = true, navErr = '';
    try {
        await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 20000 });
    } catch (e) {
        navOk = false; navErr = String(e);
    }

    // Wait for at least one blob to reach the renderer. /ambient exposes
    // window.SpaxelAmbientRenderer.getState().blobs; /live (viz3d) tracks its
    // blobs in window.Viz3D (getBlobStates/forEachBlob). Both getters are
    // probed each tick — a page exposing neither returns -1 and we fall back to
    // a fixed settle wait. Probing both matters: without the Viz3D probe, /live
    // always reports blobSeen=false even though it has rendered blobs, because
    // only the ambient getter was checked (the bf-1018k false-negative).
    let blobSeen = false;
    const start = Date.now();
    while (Date.now() - start < blobTimeoutMs) {
        try {
            const n = await page.evaluate(() => {
                const ar = window.SpaxelAmbientRenderer;
                if (ar && typeof ar.getState === 'function') {
                    const s = ar.getState();
                    return (s && s.blobs) ? s.blobs.length : 0;
                }
                // /live (WebGL/viz3d): tracked blobs live in the Viz3D blob map.
                if (window.Viz3D && typeof window.Viz3D.getBlobStates === 'function') {
                    return window.Viz3D.getBlobStates().length;
                }
                return -1; // page exposes neither getter
            });
            if (n > 0) { blobSeen = true; break; }
            if (n < 0) break; // no renderer getter: stop polling, settle below
        } catch (_) { /* page navigating; retry */ }
        // bf-5t5ny: 250ms cadence could step over a sub-100ms in-range window;
        // poll at blobSeenPollMs (50ms) so blobSeen reliably catches one.
        await page.waitForTimeout(blobSeenPollMs);
    }
    // Settle so a render frame (and any late error) lands.
    await page.waitForTimeout(settleMs);

    // ─────────────────────────────────────────────────────────────────────────
    // bf-5y3qt DIAGNOSIS / bf-5t5ny FIX — why renderEvidence reported
    // blobCount:0 despite blobs, and how the harness now handles it
    // ─────────────────────────────────────────────────────────────────────────
    // ROOT CAUSE: probe-timing vs blob lifecycle, NOT a render bug.
    //   • The /live page IS receiving & creating blobs: handleLocUpdate()
    //     (viz3d.js:1613-1614) → applyLocUpdate(msg.blobs) adds them to _blobs3D.
    //     The captured console lines "[Spaxel] Event: detection ... by blob #3"
    //     / "by blob #4" prove blobs reach the dashboard and are tracked.
    //   • BUT applyLocUpdate()'s removal loop (viz3d.js:798-800) evicts any blob
    //     id NOT in the current frame's `seen` set, so _blobs3D holds ONLY the
    //     blobs present in the MOST RECENT /ws/dashboard frame. A walker
    //     momentarily out of detection range → frame carries blobs:[] → all
    //     blobs removed. _blobs3D is thus populated only during brief sub-100ms
    //     in-range windows at fusion-tick (10 Hz) boundaries.
    //   • getBlobStates() (viz3d.js:3491-3503) reads _blobs3D directly — no
    //     history/TTL — so it legitimately returns [] at any instant that is
    //     between in-range frames.
    //   • The OLD renderEvidence probe was a SINGLE shot fired after the
    //     blobSeen loop + a settleMs wait. It almost always landed on an empty
    //     instant → {viz3d:true, blobCount:0, blobs:[]}. The blobSeen poll
    //     (250 ms cadence) could also step over the brief in-range windows, so
    //     blobSeen could read false even though detections fired.
    //
    // APPLIED FIX (bf-5t5ny):
    //   Option A — sample over a time-window and report the MAX, not a single
    //   instant. The /live else-branch below now polls getBlobStates() every
    //   renderSampleMs (~50 ms) across renderWindowMs (~2-3 s) WITHOUT
    //   early-break, tracking maxBlobCount and capturing the blob states at the
    //   peak, returning
    //     { page:'live', viz3d:true, blobCount:maxBlobCount, blobs:peakBlobs,
    //       samples:N, peakSeenAt:ms }.
    //   The blobSeen poll cadence is tightened (250 ms → blobSeenPollMs, 50 ms)
    //   so it can catch the sub-100ms in-range windows. Both are harness-only:
    //   viz3d.js is correct as designed and must NOT be changed to satisfy a
    //   probe (rejecting option C, which would alter getBlobStates' documented
    //   "current-frame" contract).
    //
    //   Complementary safeguard (option B, config-only): in
    //   scripts/run-sim-dashboard-console.sh raise SIM_WALKERS (2→3) and
    //   lengthen SIM_DURATION (40→60) so detections span the whole capture
    //   window, de-risking the harness against sparse-walker flakiness.
    // ─────────────────────────────────────────────────────────────────────────
    let renderEvidence = null;
    if (path.includes('ambient')) {
        try { renderEvidence = await ambientFallbackEvidence(page); }
        catch (e) { renderEvidence = { error: String(e) }; }
    } else {
        // /live (WebGL/viz3d): the ambient fallback-color canvas scan does not
        // apply (Three.js renders to a WebGL context, not a 2D canvas), so a
        // tracked blob present in Viz3D is the render evidence.
        //
        // bf-5t5ny FIX (option A applied): instead of probing a single instant,
        // sample getBlobStates() across renderWindowMs at renderSampleMs WITHOUT
        // early-break and keep the PEAK frame. _blobs3D is only populated during
        // the brief sub-100ms in-range windows at 10Hz fusion-tick boundaries, so
        // a one-shot read almost always returns blobCount:0; the peak across a
        // window reflects the best frame actually rendered. viz3d.js is left
        // untouched (its current-frame contract is correct as designed — option C
        // rejected). The sampling loop runs in-browser via one evaluate round-trip.
        try {
            // bf-4e7w3: Playwright's page.evaluate takes ONE arg, not N — the
            // bf-5t5ny fix passed (renderSampleMs, renderWindowMs) as two
            // positional args, so every /live capture threw "Too many arguments"
            // and the render-window sampling NEVER ran (renderEvidence always
            // errored → blobCount never observed). Wrap both in one object.
            renderEvidence = await page.evaluate(async ({ sampleMs, windowMs }) => {
                if (!(window.Viz3D && typeof window.Viz3D.getBlobStates === 'function')) {
                    return { page: 'live', viz3d: false };
                }
                let maxBlobCount = 0, peakBlobs = [], samples = 0, peakSeenAt = 0;
                const start = Date.now();
                while (Date.now() - start < windowMs) {
                    const bs = window.Viz3D.getBlobStates();
                    samples++;
                    if (bs.length > maxBlobCount) {
                        maxBlobCount = bs.length;
                        peakBlobs = bs.slice(0, 5);
                        peakSeenAt = Date.now() - start;
                    }
                    await new Promise(r => setTimeout(r, sampleMs));
                }
                return {
                    page: 'live', viz3d: true,
                    blobCount: maxBlobCount, blobs: peakBlobs,
                    samples, peakSeenAt,
                };
            }, { sampleMs: renderSampleMs, windowMs: renderWindowMs });
        } catch (e) { renderEvidence = { error: String(e) }; }
    }

    await context.close();

    // Classify identity-related errors across console + pageerror.
    const identityHits = [];
    const scan = (src, type, text) => {
        for (const re of IDENTITY_ERROR_RES) {
            if (re.test(text)) {
                identityHits.push({ where: type, text });
                break;
            }
        }
    };
    for (const m of messages) scan(m, 'console:' + m.type, m.text);
    for (const e of pageErrors) scan(e, 'pageerror', (e.message || '') + ' ' + (e.stack || ''));

    return { path, url, navOk, navErr, blobSeen, renderEvidence,
             messageCount: messages.length, messages,
             pageErrorCount: pageErrors.length, pageErrors, identityHits };
}

(async () => {
    const browser = await chromium.launch({
        headless: true,
        args: [
            '--no-sandbox',
            '--enable-unsafe-swiftshader',   // headless WebGL for /live (viz3d/Three.js)
            '--use-gl=angle',
            '--use-angle=swiftshader',
            '--ignore-gpu-blocklist',
        ],
    });
    const results = [];
    for (const p of pages) {
        try {
            results.push(await capturePage(browser, p));
        } catch (e) {
            results.push({ path: p, harnessError: String(e), identityHits: [], messageCount: 0, pageErrorCount: 0 });
        }
    }
    await browser.close();

    let anyIdentityHit = false;
    for (const r of results) {
        const s = slug(r.path);
        // Human-readable console log.
        const lines = [];
        lines.push(`# console capture — ${label} — ${r.url}`);
        lines.push(`# navOk=${r.navOk} blobSeen=${r.blobSeen} messages=${r.messageCount} pageErrors=${r.pageErrorCount} identityHits=${(r.identityHits||[]).length}`);
        if (r.navErr) lines.push(`# navErr: ${r.navErr}`);
        if (r.harnessError) lines.push(`# harnessError: ${r.harnessError}`);
        if (r.renderEvidence) lines.push(`# renderEvidence: ${JSON.stringify(r.renderEvidence)}`);
        for (const m of (r.messages || [])) lines.push(`[${m.type}] ${m.text}`);
        for (const e of (r.pageErrors || [])) lines.push(`[pageerror:${e.name}] ${e.message}`);
        writeFileSync(`${outdir}/${label}.${s}.console.txt`, lines.join('\n') + '\n');
        // Structured JSON.
        writeFileSync(`${outdir}/${label}.${s}.json`, JSON.stringify(r, null, 2));
        if ((r.identityHits || []).length) anyIdentityHit = true;
    }
    // Summary line for the orchestrator to grep.
    writeFileSync(`${outdir}/${label}.summary.txt`,
        results.map(r =>
            `${label}\t${r.path}\tblobSeen=${r.blobSeen}\tmsgs=${r.messageCount}\tpageErrors=${r.pageErrorCount}\tidentityHits=${(r.identityHits||[]).length}`
        ).join('\n') + '\n');

    // Non-zero only on a true harness failure; identity hits are reported, the
    // orchestrator decides pass/fail so the evidence is always preserved.
    process.exit(anyIdentityHit ? 0 : 0);
})().catch(e => {
    console.error('[capture-dashboard-console] harness failure:', e);
    process.exit(2);
});
