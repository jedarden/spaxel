/**
 * Spaxel Dashboard - Renderer Backward-Compatibility Tests
 *
 * RUNTIME (jsdom) proof for parent bf-2gmx ("Verify backward compatibility for
 * blob identity fields"): the dashboard renderer must render blobs that LACK
 * the identity fields (personName / assignedColor / identityResolved and their
 * snake_case aliases person_name / assigned_color / person_label / person_color)
 * WITHOUT throwing or emitting undefined/NaN, while identity-resolved blobs
 * still render WITH their identity (label + per-person color).
 *
 * Why the ambient Canvas 2D renderer (and not the 3D viz3d renderer):
 * The 3D renderer viz3d.js evaluates `new THREE.Raycaster()` and
 * `new THREE.Vector2()` at module top level and depends on Three.js (WebGL),
 * which is unavailable under jsdom — so it cannot be loaded here. That is
 * exactly why every existing dashboard test suite SKIPS the renderer. The
 * ambient renderer (ambient_renderer.js) is the renderer path that DOES load
 * under jsdom: it is a self-contained IIFE backed by the Canvas 2D context
 * mock in ambient.test.setup.js. Its drawPeople() is the one code path that
 * reads a blob's identity fields (person_label || person) directly during blob
 * rendering and funnels them through the hardened getPersonColor() helper
 * (bf-gcotl added the `if (!personName) return '#6b7280'` guard). Loading it
 * for real and driving a render frame is therefore a genuine runtime proof of
 * the backward-compat contract, not just a type check.
 *
 * Bead: bf-20cl7. Parent: bf-2gmx. Static audit: bf-gcotl.
 */

describe('Renderer backward-compat with identity-less blobs', function () {

    let canvas;
    let renderer;
    let ctx;

    // Identity fields are all OPTIONAL on the wire. A pre-identity (identity-less)
    // blob — e.g. one emitted by the local sim WITHOUT --ble — carries NONE of:
    //   personName / person_name / personLabel / person_label / person
    //   assignedColor / assigned_color / personColor / person_color
    //   identityResolved
    function makeIdentityLessBlob(overrides) {
        return Object.assign({
            id: 1,
            x: 2.5,
            y: 2.5,
            z: 1.0,
            vx: 0,
            vy: 0,
            vz: 0,
            confidence: 0.8
            // NOTE: no identity fields at all
        }, overrides || {});
    }

    function makeIdentityResolvedBlob(overrides) {
        // Identity-resolved blob carries the full identity field set, including
        // both the camelCase (viz3d / ble-panel) and snake_case (ambient renderer)
        // aliases. The renderer must pick up the identity and render with it.
        return Object.assign(makeIdentityLessBlob({
            id: 1,
            person_label: 'Alice',
            person_name: 'Alice',
            personName: 'Alice',
            assignedColor: '#3b82f6',
            assigned_color: '#3b82f6',
            personColor: '#3b82f6',
            person_color: '#3b82f6',
            identityResolved: true
        }), overrides || {});
    }

    // Load the renderer into jsdom (runs the IIFE → window.SpaxelAmbientRenderer).
    function ensureRendererLoaded() {
        if (typeof window.SpaxelAmbientRenderer === 'undefined') {
            require('./ambient_renderer.js');
        }
        return window.SpaxelAmbientRenderer;
    }

    function setupRenderer() {
        renderer = ensureRendererLoaded();

        canvas = document.createElement('canvas');
        canvas.width = 800;
        canvas.height = 600;
        canvas.style.width = '800px';
        canvas.style.height = '600px';
        document.body.appendChild(canvas);

        renderer.init(canvas, { scale: 50, margin: 40 });
        // Determinism: stop the background RAF loop and drive frames manually.
        renderer.stopRenderLoop();

        ctx = canvas.getContext('2d');

        // Capture fillStyle at the moment a filled arc is drawn so we can assert
        // the color used for each blob (the renderer overwrites fillStyle later
        // for the status dot / time text, so reading it post-frame is unreliable).
        arcFillStyles = [];
        const origArc = ctx.arc;
        ctx.arc = function (x, y, radius, startAngle, endAngle) {
            arcFillStyles.push(ctx.fillStyle);
            return origArc.apply(this, arguments);
        };
    }

    let arcFillStyles;

    function renderBlobs(blobs) {
        renderer.updateState({ blobs: blobs });
        renderer.render();
    }

    // The label strings passed to ctx.fillText() across the rendered frame.
    function fillTextArgs() {
        const calls = (ctx.fillText && ctx.fillText.mock && ctx.fillText.mock.calls) || [];
        return calls.map(function (c) { return c[0]; });
    }

    beforeEach(function () {
        setupRenderer();
    });

    afterEach(function () {
        // Flush module singleton state so tests don't leak blob ids into each other.
        try {
            if (renderer) {
                renderer.updateState({ blobs: [], zones: [], portals: [], nodes: [], alerts: [] });
                renderer.destroy();
            }
        } catch (e) { /* swallow teardown errors */ }
        if (canvas && canvas.parentNode) {
            canvas.parentNode.removeChild(canvas);
        }
        renderer = null;
        canvas = null;
        ctx = null;
        arcFillStyles = [];
    });

    // ── module loads under jsdom ───────────────────────────────────────────────

    describe('renderer loads into jsdom', function () {
        it('attaches window.SpaxelAmbientRenderer when the module is required', function () {
            // beforeEach → ensureRendererLoaded() → require('./ambient_renderer.js').
            // The assertion passes iff that require ran the module's IIFE and
            // attached the renderer to window WITHOUT throwing — i.e. the renderer
            // is genuinely loadable under jsdom (the thing viz3d cannot do).
            expect(window.SpaxelAmbientRenderer).toBeDefined();
            expect(typeof window.SpaxelAmbientRenderer).toBe('object');
            expect(typeof window.SpaxelAmbientRenderer.init).toBe('function');
            expect(typeof window.SpaxelAmbientRenderer.render).toBe('function');
            expect(typeof window.SpaxelAmbientRenderer.updateState).toBe('function');
        });
    });

    // ── identity-LESS blobs (the backward-compat core) ─────────────────────────

    describe('blob with NO identity fields', function () {
        it('renders a full frame without throwing', function () {
            expect(function () {
                renderBlobs([makeIdentityLessBlob()]);
            }).not.toThrow();
        });

        it('applies a sane fallback label and never emits "undefined"/"NaN"', function () {
            renderBlobs([makeIdentityLessBlob()]);

            const labels = fillTextArgs();
            expect(labels.length).toBeGreaterThan(0);
            labels.forEach(function (text) {
                const s = String(text);
                expect(s).not.toMatch(/undefined/i);
                expect(s).not.toMatch(/NaN/);
            });
            // Unknown blob falls back to the '?' label, not a name.
            expect(labels).toContain('?');
        });

        it('colors the blob with the neutral fallback, not a per-person color', function () {
            renderBlobs([makeIdentityLessBlob()]);
            // getPersonColor(undefined) -> '#6b7280' (grey) is the fillStyle in
            // effect when the blob's arc is stroked. '#22c55e' is the green status
            // dot drawn afterwards — neither is an identity-derived color.
            expect(arcFillStyles).toContain('#6b7280');
            arcFillStyles.forEach(function (c) {
                // No identity color (hsl hash) should be produced for an identity-less blob.
                expect(String(c)).not.toMatch(/^hsl\(/);
            });
        });

        it('stays safe when every identity field is explicitly null', function () {
            expect(function () {
                renderBlobs([makeIdentityLessBlob({
                    personName: null,
                    person_name: null,
                    personLabel: null,
                    person_label: null,
                    person: null,
                    assignedColor: null,
                    assigned_color: null,
                    personColor: null,
                    person_color: null,
                    identityResolved: null
                })]);
            }).not.toThrow();
            expect(fillTextArgs()).toContain('?');
            expect(arcFillStyles).toContain('#6b7280');
        });
    });

    // ── identity-RESOLVED blobs ────────────────────────────────────────────────

    describe('identity-resolved blob', function () {
        it('renders with the person name label (first name)', function () {
            renderBlobs([makeIdentityResolvedBlob({ person_label: 'Alice Wonderland' })]);
            // drawPeople uses getFirstName() -> 'Alice'
            expect(fillTextArgs()).toContain('Alice');
            expect(fillTextArgs()).not.toContain('?');
        });

        it('colors the blob with a per-person color (not the grey fallback)', function () {
            renderBlobs([makeIdentityResolvedBlob({ person_label: 'Alice' })]);
            // getPersonColor(name) hashes the name into an hsl() color; never '#6b7280'.
            const hasPersonColor = arcFillStyles.some(function (c) {
                return /^hsl\(/.test(String(c));
            });
            expect(hasPersonColor).toBe(true);
            expect(arcFillStyles).not.toContain('#6b7280');
        });

        it('renders distinct resolved people with distinct labels', function () {
            renderBlobs([
                makeIdentityResolvedBlob({ id: 1, person_label: 'Alice' }),
                makeIdentityResolvedBlob({ id: 2, person_label: 'Bob' })
            ]);
            const labels = fillTextArgs();
            expect(labels).toContain('Alice');
            expect(labels).toContain('Bob');
            labels.forEach(function (text) {
                expect(String(text)).not.toMatch(/undefined/i);
                expect(String(text)).not.toMatch(/NaN/);
            });
        });
    });

    // ── the realistic backward-compat scenario: a mixed fleet ──────────────────

    describe('mixed fleet (resolved + identity-less blobs together)', function () {
        it('renders every blob without throwing and with no undefined/NaN', function () {
            expect(function () {
                renderBlobs([
                    makeIdentityResolvedBlob({ id: 1, person_label: 'Alice' }),
                    makeIdentityLessBlob({ id: 2 }),                    // no identity
                    makeIdentityLessBlob({ id: 3, identityResolved: false }) // unresolved
                ]);
            }).not.toThrow();

            const labels = fillTextArgs();
            expect(labels).toContain('Alice');  // resolved
            expect(labels).toContain('?');      // identity-less fallback

            // Both color paths are exercised: a per-person hsl color AND the grey fallback.
            expect(arcFillStyles.some(function (c) { return /^hsl\(/.test(String(c)); })).toBe(true);
            expect(arcFillStyles).toContain('#6b7280');

            labels.forEach(function (text) {
                expect(String(text)).not.toMatch(/undefined/i);
                expect(String(text)).not.toMatch(/NaN/);
            });
        });
    });
});
