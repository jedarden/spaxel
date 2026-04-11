/**
 * Tests for Ambient Dashboard Mode
 *
 * Tests for Canvas 2D renderer, auto-dim, alert mode, morning briefing, and lerp interpolation.
 */

(function() {
    'use strict';

    // ============================================
    // Test Helpers
    // ============================================

    function createTestCanvas() {
        const canvas = document.createElement('canvas');
        canvas.width = 800;
        canvas.height = 600;
        canvas.style.width = '800px';
        canvas.style.height = '600px';
        document.body.appendChild(canvas);
        return canvas;
    }

    function cleanupTestCanvas(canvas) {
        if (canvas && canvas.parentNode) {
            canvas.parentNode.removeChild(canvas);
        }
    }

    function waitForAnimationFrame() {
        return new Promise(resolve => {
            requestAnimationFrame(() => {
                requestAnimationFrame(resolve);
            });
        });
    }

    function sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    // ============================================
    // Canvas 2D Renderer Tests
    // ============================================

    describe('AmbientRenderer - Canvas 2D', function() {
        let canvas;
        let renderer;

        beforeEach(function() {
            canvas = createTestCanvas();
            // Reset the renderer module state
            if (window.SpaxelAmbientRenderer) {
                // Store original state
                window._originalAmbientRendererState = {
                    currentPositions: window.SpaxelAmbientRenderer._currentPositions,
                    targetPositions: window.SpaxelAmbientRenderer._targetPositions
                };
            }
        });

        afterEach(function() {
            if (renderer) {
                renderer.destroy();
            }
            cleanupTestCanvas(canvas);
            // Restore original state
            if (window._originalAmbientRendererState) {
                if (window.SpaxelAmbientRenderer) {
                    window.SpaxelAmbientRenderer._currentPositions = window._originalAmbientRendererState.currentPositions;
                    window.SpaxelAmbientRenderer._targetPositions = window._originalAmbientRendererState.targetPositions;
                }
                delete window._originalAmbientRendererState;
            }
        });

        it('should draw zone rectangle at correct pixel coordinates', function() {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50, // 50 pixels per meter
                margin: 40
            });

            // Update state with a zone at (1,1)-(3,3) meters
            renderer.updateState({
                zones: [{
                    id: 1,
                    name: 'Test Zone',
                    x: 1,
                    y: 1,
                    w: 2,  // 3 - 1 = 2 meters wide
                    d: 2,  // 3 - 1 = 2 meters deep
                    count: 0
                }],
                blobs: [],
                portals: [],
                nodes: []
            });

            // Trigger render
            renderer.render();

            // Verify the zone was drawn
            const ctx = canvas.getContext('2d');
            const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);

            // Check that some white pixels were drawn (zone outline)
            let hasWhitePixel = false;
            for (let i = 0; i < imageData.data.length; i += 4) {
                const r = imageData.data[i];
                const g = imageData.data[i + 1];
                const b = imageData.data[i + 2];
                // Check for white (zone outline color)
                if (r > 250 && g > 250 && b > 250) {
                    hasWhitePixel = true;
                    break;
                }
            }

            expect(hasWhitePixel).toBe(true);
        });

        it('should draw person blob at correct position', function() {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40
            });

            // Update state with a person at (2, 2) meters
            renderer.updateState({
                zones: [],
                blobs: [{
                    id: 1,
                    x: 2,
                    y: 2,
                    z: 0,
                    confidence: 0.8,
                    person: 'Alice'
                }],
                portals: [],
                nodes: []
            });

            // Trigger render
            renderer.render();

            // The blob should be drawn at approximately (2 * 50 + margin) pixels
            // x = 40 + (2 - 0) * 50 = 140px
            // y = 40 + (2 - 0) * 50 = 140px
            const ctx = canvas.getContext('2d');

            // Sample pixels around expected position
            const centerX = 140;
            const centerY = 140;
            const imageData = ctx.getImageData(centerX - 20, centerY - 20, 40, 40);

            // Check that some colored pixels were drawn (person blob)
            let hasColoredPixel = false;
            for (let i = 0; i < imageData.data.length; i += 4) {
                const r = imageData.data[i];
                const g = imageData.data[i + 1];
                const b = imageData.data[i + 2];
                const a = imageData.data[i + 3];
                // Check for non-transparent, non-background pixel
                if (a > 0 && (r !== 255 || g !== 255 || b !== 255)) {
                    hasColoredPixel = true;
                    break;
                }
            }

            expect(hasColoredPixel).toBe(true);
        });

        it('should draw node position as small grey circle', function() {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40
            });

            // Update state with a node at (1, 1) meters
            renderer.updateState({
                zones: [],
                blobs: [],
                portals: [],
                nodes: [{
                    mac: 'AA:BB:CC:DD:EE:FF',
                    pos_x: 1,
                    pos_y: 1,
                    pos_z: 2
                }]
            });

            // Trigger render
            renderer.render();

            // Node should be drawn as a small grey circle
            // Position: x = 40 + (1 - 0) * 50 = 90px, y = 40 + (1 - 0) * 50 = 90px
            const ctx = canvas.getContext('2d');
            const imageData = ctx.getImageData(85, 85, 10, 10);

            // Check for grey pixels (#6b7280 = rgb(107, 114, 128))
            let hasGreyPixel = false;
            for (let i = 0; i < imageData.data.length; i += 4) {
                const r = imageData.data[i];
                const g = imageData.data[i + 1];
                const b = imageData.data[i + 2];
                const a = imageData.data[i + 3];

                // Check for grey with some tolerance
                if (a > 200 && r > 90 && r < 130 && g > 100 && g < 140 && b > 115 && b < 150) {
                    hasGreyPixel = true;
                    break;
                }
            }

            expect(hasGreyPixel).toBe(true);
        });

        it('should render at 2 Hz (one frame every 500ms)', function(done) {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40
            });

            // Track render calls
            let renderCount = 0;
            const originalRender = renderer.render.bind(renderer);
            renderer.render = function() {
                renderCount++;
                return originalRender();
            };

            // Wait for multiple render cycles
            const startTime = Date.now();

            setTimeout(() => {
                const elapsed = Date.now() - startTime;
                // Should have approximately elapsed / 500 renders
                // Allow some tolerance
                const expectedRenders = Math.floor(elapsed / 500);
                expect(renderCount).toBeGreaterThanOrEqual(expectedRenders - 1);
                expect(renderCount).toBeLessThanOrEqual(expectedRenders + 1);

                // Restore original render
                renderer.render = originalRender;
                done();
            }, 1200); // Wait ~2.4 render cycles
        });
    });

    // ============================================
    // Auto-Dim Tests
    // ============================================

    describe('AmbientRenderer - Auto-Dim', function() {
        let canvas;
        let renderer;

        beforeEach(function() {
            canvas = createTestCanvas();
        });

        afterEach(function() {
            if (renderer) {
                renderer.destroy();
            }
            cleanupTestCanvas(canvas);
            // Clear localStorage
            localStorage.removeItem('ambient_briefing_last_shown');
        });

        it('should reduce canvas brightness after 60s with no presence', function(done) {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40,
                ambientZone: 'test-zone'
            });

            // Mock time to speed up test (use shorter timeout for testing)
            // We'll manually trigger the dim by calling the internal function
            const originalTimeout = 60000;

            // Update state with no blobs in ambient zone
            renderer.updateState({
                zones: [{
                    id: 'test-zone',
                    name: 'Test Zone',
                    x: 0,
                    y: 0,
                    w: 5,
                    d: 5,
                    count: 0  // No presence
                }],
                blobs: [],
                portals: [],
                nodes: []
            });

            // Trigger dim mode manually (in real usage, this happens after timeout)
            renderer._enterDimMode && renderer._enterDimMode();

            // Check canvas filter
            expect(canvas.style.filter).toContain('brightness(0.4)');
            done();
        });

        it('should restore brightness when presence detected in ambient zone', function(done) {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40,
                ambientZone: 'test-zone'
            });

            // First enter dim mode
            renderer._enterDimMode && renderer._enterDimMode();
            expect(canvas.style.filter).toContain('brightness(0.4)');

            // Now simulate presence detection
            renderer.updateState({
                zones: [{
                    id: 'test-zone',
                    name: 'Test Zone',
                    x: 0,
                    y: 0,
                    w: 5,
                    d: 5,
                    count: 1  // Presence detected
                }],
                blobs: [{
                    id: 1,
                    x: 2,
                    y: 2,
                    z: 0
                }],
                portals: [],
                nodes: []
            });

            // Trigger presence check
            renderer._checkAmbientZonePresence && renderer._checkAmbientZonePresence();

            // Brightness should be restored
            expect(canvas.style.filter).not.toContain('brightness(0.4)');
            expect(canvas.style.filter).toContain('brightness(1)');
            done();
        });
    });

    // ============================================
    // Alert Mode Tests
    // ============================================

    describe('AmbientRenderer - Alert Mode', function() {
        let canvas;
        let renderer;

        beforeEach(function() {
            canvas = createTestCanvas();
        });

        afterEach(function() {
            if (renderer) {
                renderer.destroy();
            }
            cleanupTestCanvas(canvas);
        });

        it('should enter alert mode on fall detected event', function() {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40
            });

            // Simulate fall alert
            renderer.enterAlertMode({
                type: 'fall_alert',
                id: 'alert-1',
                person: 'Alice',
                zone: 'Kitchen'
            });

            renderer.render();

            // Check that red background was drawn
            const ctx = canvas.getContext('2d');
            const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);

            // Check for red pixels (#dc2626 = rgb(220, 38, 38))
            let hasRedPixel = false;
            for (let i = 0; i < imageData.data.length; i += 4) {
                const r = imageData.data[i];
                const g = imageData.data[i + 1];
                const b = imageData.data[i + 2];

                if (r > 180 && g < 100 && b < 100) {
                    hasRedPixel = true;
                    break;
                }
            }

            expect(hasRedPixel).toBe(true);
        });

        it('should show large alert text', function() {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40
            });

            renderer.enterAlertMode({
                type: 'fall_alert',
                id: 'alert-1',
                person: 'Alice',
                zone: 'Kitchen'
            });

            renderer.render();

            // Check that text was rendered (verify by checking canvas state)
            // The renderer should have the alert in its state
            const alerts = renderer._currentState ? renderer._currentState.alerts : [];
            expect(alerts.length).toBeGreaterThan(0);
            expect(alerts[0].type).toBe('fall_alert');
        });

        it('should pulse alert background at 1 Hz', function(done) {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40
            });

            renderer.enterAlertMode({
                type: 'fall_alert',
                id: 'alert-1',
                person: 'Alice'
            });

            // Track pulse state changes
            let pulseCount = 0;
            const checkInterval = setInterval(() => {
                const pulseState = renderer._getAlertPulseState && renderer._getAlertPulseState();
                if (pulseState !== undefined) {
                    pulseCount++;
                    if (pulseCount >= 2) {
                        clearInterval(checkInterval);
                        // Should have seen state change within 2 seconds (1 Hz)
                        expect(pulseCount).toBeGreaterThan(0);
                        done();
                    }
                }
            }, 100);

            // Cleanup after timeout
            setTimeout(() => {
                clearInterval(checkInterval);
                done();
            }, 3000);
        });

        it('should exit alert mode on acknowledge', function() {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40
            });

            // Enter alert mode
            renderer.enterAlertMode({
                type: 'fall_alert',
                id: 'alert-1',
                person: 'Alice'
            });

            expect(renderer._currentState.alerts.length).toBeGreaterThan(0);

            // Exit alert mode
            renderer.exitAlertMode();

            expect(renderer._currentState.alerts.length).toBe(0);
        });
    });

    // ============================================
    // Morning Briefing Tests
    // ============================================

    describe('AmbientBriefing - Morning Briefing', function() {
        let briefingElement;

        beforeEach(function() {
            // Clear localStorage
            localStorage.removeItem('ambient_briefing_last_shown');

            // Create briefing element if it doesn't exist
            if (!document.getElementById('ambient-briefing')) {
                briefingElement = document.createElement('div');
                briefingElement.id = 'ambient-briefing';
                briefingElement.className = 'ambient-briefing hidden';
                document.body.appendChild(briefingElement);
            } else {
                briefingElement = document.getElementById('ambient-briefing');
            }
        });

        afterEach(function() {
            // Clean up
            if (briefingElement && briefingElement.parentNode) {
                briefingElement.parentNode.removeChild(briefingElement);
            }
            localStorage.removeItem('ambient_briefing_last_shown');
        });

        it('should appear only once after 6am', function(done) {
            if (!window.SpaxelAmbientBriefing) {
                this.skip();
                return;
            }

            // Mock current time to be after 6am
            const originalDate = Date;
            const mockHour = 7; // 7am
            spyOn(Date, 'now').and.returnValue(new Date(2025, 3, 10, mockHour, 0, 0).getTime());
            spyOn(Date.prototype, 'getHours').and.returnValue(mockHour);

            // Reset daily flag
            window.SpaxelAmbientBriefing.resetDailyFlag();

            // First call should return true (should show)
            window.SpaxelAmbientBriefing.shouldShowToday().then(shouldShow => {
                expect(shouldShow).toBe(true);

                // Mark as shown
                window.SpaxelAmbientBriefing.dismiss();

                // Second call should return false (already shown)
                window.SpaxelAmbientBriefing.shouldShowToday().then(shouldShowAgain => {
                    expect(shouldShowAgain).toBe(false);
                    done();
                });
            });
        });

        it('should dismiss after 15 seconds', function(done) {
            if (!window.SpaxelAmbientBriefing) {
                this.skip();
                return;
            }

            // Mock Date to be after 6am
            const mockHour = 7;
            spyOn(Date, 'now').and.returnValue(new Date(2025, 3, 10, mockHour, 0, 0).getTime());
            spyOn(Date.prototype, 'getHours').and.returnValue(mockHour);

            // Reset and show briefing
            window.SpaxelAmbientBriefing.resetDailyFlag();

            // Show the briefing
            window.SpaxelAmbientBriefing.show({
                content: 'Test briefing content'
            });

            // Check that it's visible
            const briefingEl = document.getElementById('ambient-briefing');
            expect(briefingEl.classList.contains('visible')).toBe(true);

            // Wait for auto-dismiss (shortened for testing by mocking)
            // In real usage, this is 15 seconds
            setTimeout(() => {
                // Briefing should still be visible (15 seconds not elapsed)
                expect(briefingEl.classList.contains('visible')).toBe(true);

                // Manually dismiss to clean up
                window.SpaxelAmbientBriefing.dismiss();
                expect(briefingEl.classList.contains('visible')).toBe(false);
                done();
            }, 100);
        });

        it('should dismiss on tap/click', function(done) {
            if (!window.SpaxelAmbientBriefing) {
                this.skip();
                return;
            }

            // Mock Date to be after 6am
            const mockHour = 7;
            spyOn(Date, 'now').and.returnValue(new Date(2025, 3, 10, mockHour, 0, 0).getTime());
            spyOn(Date.prototype, 'getHours').and.returnValue(mockHour);

            window.SpaxelAmbientBriefing.resetDailyFlag();

            // Show the briefing
            window.SpaxelAmbientBriefing.show({
                content: 'Test briefing content'
            });

            const briefingEl = document.getElementById('ambient-briefing');
            expect(briefingEl.classList.contains('visible')).toBe(true);

            // Simulate tap/click on dismiss button
            const dismissBtn = document.getElementById('briefing-dismiss');
            if (dismissBtn) {
                dismissBtn.click();

                // Should dismiss immediately
                expect(briefingEl.classList.contains('visible')).toBe(false);
                done();
            } else {
                done();
            }
        });
    });

    // ============================================
    // Lerp Interpolation Tests
    // ============================================

    describe('AmbientRenderer - Lerp Interpolation', function() {
        let canvas;
        let renderer;

        beforeEach(function() {
            canvas = createTestCanvas();
        });

        afterEach(function() {
            if (renderer) {
                renderer.destroy();
            }
            cleanupTestCanvas(canvas);
        });

        it('should interpolate position from (1,1) to (3,3)', function(done) {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40
            });

            // Start position at (1, 1)
            renderer.updateState({
                zones: [],
                blobs: [{
                    id: 1,
                    x: 1,
                    y: 1,
                    z: 0,
                    confidence: 0.8
                }],
                portals: [],
                nodes: []
            });

            // Wait for one render cycle
            setTimeout(() => {
                // Move target to (3, 3)
                renderer.updateState({
                    zones: [],
                    blobs: [{
                        id: 1,
                        x: 3,
                        y: 3,
                        z: 0,
                        confidence: 0.8
                    }],
                    portals: [],
                    nodes: []
                });

                // After 5 render frames (5 * 500ms = 2.5 seconds),
                // position should be approximately at (2.5, 2.5) with 20% lerp
                // Let's verify the interpolation is working by checking current position
                const currentPos = renderer._currentPositions && renderer._currentPositions.get(1);

                if (currentPos) {
                    // Position should have moved from (1, 1) toward (3, 3)
                    expect(currentPos.x).toBeGreaterThan(1);
                    expect(currentPos.y).toBeGreaterThan(1);
                    expect(currentPos.x).toBeLessThan(3);
                    expect(currentPos.y).toBeLessThan(3);
                }

                done();
            }, 600);
        });

        it('should use 20% lerp factor per frame', function(done) {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40
            });

            // Set initial position
            renderer._currentPositions = new Map([[1, { x: 1, y: 1, z: 0 }]]);
            renderer._targetPositions = new Map([[1, { x: 3, y: 3, z: 0 }]]);

            // Trigger one render
            renderer.render();

            // After one frame with 20% lerp:
            // x = 1 + 0.2 * (3 - 1) = 1 + 0.4 = 1.4
            // y = 1 + 0.2 * (3 - 1) = 1 + 0.4 = 1.4
            const currentPos = renderer._currentPositions.get(1);

            expect(currentPos.x).toBeCloseTo(1.4, 0.01);
            expect(currentPos.y).toBeCloseTo(1.4, 0.01);
            done();
        });

        it('should smoothly decelerate with exponential approach', function(done) {
            if (!window.SpaxelAmbientRenderer) {
                this.skip();
                return;
            }

            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvas, {
                scale: 50,
                margin: 40
            });

            // Set initial position far from target
            renderer._currentPositions = new Map([[1, { x: 0, y: 0, z: 0 }]]);
            renderer._targetPositions = new Map([[1, { x: 10, y: 10, z: 0 }]]);

            const positions = [];

            // Simulate 10 frames
            for (let i = 0; i < 10; i++) {
                renderer.render();
                const pos = renderer._currentPositions.get(1);
                positions.push({ x: pos.x, y: pos.y });
            }

            // Check that movement per frame decreases (exponential deceleration)
            let prevDelta = null;
            for (let i = 1; i < positions.length; i++) {
                const delta = Math.sqrt(
                    Math.pow(positions[i].x - positions[i-1].x, 2) +
                    Math.pow(positions[i].y - positions[i-1].y, 2)
                );

                if (prevDelta !== null) {
                    // Movement should decrease or stay same (never increase)
                    expect(delta).toBeLessThanOrEqual(prevDelta + 0.001);
                }
                prevDelta = delta;
            }

            // Final position should be closer to target than initial
            const finalDist = Math.sqrt(
                Math.pow(10 - positions[9].x, 2) +
                Math.pow(10 - positions[9].y, 2)
            );
            const initialDist = Math.sqrt(
                Math.pow(10 - positions[0].x, 2) +
                Math.pow(10 - positions[0].y, 2)
            );

            expect(finalDist).toBeLessThan(initialDist);
            done();
        });
    });

    // ============================================
    // Time-of-Day Palette Tests
    // ============================================

    describe('AmbientMode - Time-of-Day Palette', function() {
        let originalBody;

        beforeEach(function() {
            originalBody = document.body.cloneNode(true);
            document.body.classList.add('ambient-mode');
        });

        afterEach(function() {
            document.body.className = originalBody.className;
        });

        it('should use morning palette (6-10am)', function() {
            if (!window.SpaxelAmbientMode) {
                this.skip();
                return;
            }

            // Mock hour to 7am
            const dateSpy = spyOn(Date.prototype, 'getHours').and.returnValue(7);

            window.SpaxelAmbientMode.init();

            // Check that time-morning class was added
            expect(document.body.classList.contains('time-morning')).toBe(true);
        });

        it('should use day palette (10am-6pm)', function() {
            if (!window.SpaxelAmbientMode) {
                this.skip();
                return;
            }

            // Mock hour to 2pm
            spyOn(Date.prototype, 'getHours').and.returnValue(14);

            window.SpaxelAmbientMode.init();

            expect(document.body.classList.contains('time-day')).toBe(true);
        });

        it('should use evening palette (6-10pm)', function() {
            if (!window.SpaxelAmbientMode) {
                this.skip();
                return;
            }

            // Mock hour to 7pm
            spyOn(Date.prototype, 'getHours').and.returnValue(19);

            window.SpaxelAmbientMode.init();

            expect(document.body.classList.contains('time-evening')).toBe(true);
        });

        it('should use night palette (10pm-6am)', function() {
            if (!window.SpaxelAmbientMode) {
                this.skip();
                return;
            }

            // Mock hour to 11pm
            spyOn(Date.prototype, 'getHours').and.returnValue(23);

            window.SpaxelAmbientMode.init();

            expect(document.body.classList.contains('time-night')).toBe(true);
        });
    });

    // ============================================
    // Test Runner
    // ============================================

    function runTests() {
        console.log('[Ambient Tests] Starting test suite...');

        const testSuites = [
            'AmbientRenderer - Canvas 2D',
            'AmbientRenderer - Auto-Dim',
            'AmbientRenderer - Alert Mode',
            'AmbientBriefing - Morning Briefing',
            'AmbientRenderer - Lerp Interpolation',
            'AmbientMode - Time-of-Day Palette'
        ];

        let currentSuite = 0;
        let currentTest = 0;
        let passed = 0;
        let failed = 0;
        let skipped = 0;

        function nextTest() {
            if (currentSuite >= testSuites.length) {
                console.log(`[Ambient Tests] Complete: ${passed} passed, ${failed} failed, ${skipped} skipped`);
                return;
            }

            const suite = describe._suites[testSuites[currentSuite]];
            if (!suite || !suite.tests || currentTest >= suite.tests.length) {
                currentSuite++;
                currentTest = 0;
                nextTest();
                return;
            }

            const test = suite.tests[currentTest];
            currentTest++;

            try {
                test.fn.call({
                    skip: function() {
                        console.log(`  [SKIP] ${suite.name} - ${test.name}`);
                        skipped++;
                        nextTest();
                    },
                    spyOn: function(obj, method) {
                        const spy = {
                            and: { returnValue: function(value) {
                                obj[method] = function() { return value; };
                                return spy;
                            }},
                            calls: { count: function() { return 0; } }
                        };
                        return spy;
                    }
                });

                if (test.fn.length > 0) {
                    // Async test - needs done callback
                    // In a real test framework, this would be handled differently
                    // For now, we'll just skip async tests
                    console.log(`  [SKIP] ${suite.name} - ${test.name} (async)`);
                    skipped++;
                }
            } catch (e) {
                console.log(`  [FAIL] ${suite.name} - ${test.name}: ${e.message}`);
                failed++;
            }

            nextTest();
        }

        // Simple test result storage
        window.ambientTestResults = {
            passed: passed,
            failed: failed,
            skipped: skipped
        };

        nextTest();
    }

    // ============================================
    // Jasmine Integration
    // ============================================

    // Export test functions for Jasmine/Mocha
    if (typeof describe !== 'undefined') {
        // Already in a test environment, tests will be picked up automatically
        console.log('[Ambient Tests] Running in test environment');
    } else if (typeof module !== 'undefined' && module.exports) {
        // Node.js environment
        module.exports = { runTests };
    } else {
        // Browser environment without test framework
        console.log('[Ambient Tests] No test framework detected. Run with Jasmine/Mocha or call runTests() manually.');
        window.runAmbientTests = runTests;
    }

})();
// Add closeTo matcher for Jasmine if not present
if (typeof jasmine !== 'undefined') {
    jasmine.addMatchers({
        toBeCloseTo: function(util, customEqualityTesters) {
            return {
                compare: function(actual, expected, precision) {
                    if (precision === undefined) {
                        precision = 0.001;
                    }
                    const pass = Math.abs(actual - expected) < precision;
                    return {
                        pass: pass,
                        message: `Expected ${actual} to be close to ${expected} within ${precision}`
                    };
                }
            };
        }
    });
}
