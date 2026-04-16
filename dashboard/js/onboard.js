/**
 * Spaxel Onboarding Wizard
 *
 * Interactive Web Serial-based setup wizard for provisioning ESP32-S3 nodes.
 * States: BROWSER_CHECK → CONNECT_DEVICE → FLASH_FIRMWARE → PROVISION_WIFI
 *         → DETECT_NODE → CALIBRATE → PLACEMENT → COMPLETE
 */

(function () {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    var CONFIG = {
        nodePollInterval: 3000,
        nodePollTimeout: 120000,
        calibrateWalkDuration: 30000,
        calibrateStillDuration: 10000,
        calibrateWalkThroughDuration: 15000,
        storageKey: 'spaxel_onboard',
        serialBaudRate: 115200,
        provisioningEndpoint: '/api/provision',
        nodesEndpoint: '/api/nodes',
    };

    // ============================================
    // Step Definitions
    // ============================================
    var STEPS = [
        { id: 'browser_check', label: 'Browser' },
        { id: 'connect_device', label: 'Connect' },
        { id: 'flash_firmware', label: 'Flash' },
        { id: 'provision_wifi', label: 'WiFi' },
        { id: 'detect_node', label: 'Detect' },
        { id: 'calibrate', label: 'Calibrate' },
        { id: 'placement', label: 'Position' },
        { id: 'complete', label: 'Done' },
    ];

    // ============================================
    // Wizard State
    // ============================================
    var state = {
        currentStepIndex: -1,
        port: null,
        nodeMAC: null,
        knownMACs: [],
        wifiSSID: '',
        wifiPass: '',
        mothershipHost: '',
        mothershipPort: 8080,
        pollTimer: null,
        calibrateTimer: null,
        calibratePhase: 'idle',
        ws: null,
        csiHistory: [],
        calibrationLinks: [],  // unique link IDs seen during calibration
        container: null,
    };

    // ============================================
    // State Persistence (sessionStorage)
    // ============================================
    function saveState() {
        try {
            sessionStorage.setItem(CONFIG.storageKey, JSON.stringify({
                currentStepIndex: state.currentStepIndex,
                nodeMAC: state.nodeMAC,
                knownMACs: state.knownMACs,
                wifiSSID: state.wifiSSID,
                wifiPass: state.wifiPass,
                mothershipHost: state.mothershipHost,
                mothershipPort: state.mothershipPort,
            }));
        } catch (e) { /* ignore */ }
    }

    function loadState() {
        try {
            var raw = sessionStorage.getItem(CONFIG.storageKey);
            return raw ? JSON.parse(raw) : null;
        } catch (e) { return null; }
    }

    function clearState() {
        try { sessionStorage.removeItem(CONFIG.storageKey); } catch (e) { /* ignore */ }
    }

    // ============================================
    // Serial Helpers
    // ============================================
    async function getAuthorizedPort() {
        var ports = await navigator.serial.getPorts();
        return ports.length > 0 ? ports[0] : null;
    }

    async function requestPort() {
        try {
            state.port = await navigator.serial.requestPort();
            return state.port;
        } catch (e) {
            if (e.name === 'NotFoundError') {
                throw new UserError(
                    'No device detected. Did you hold the BOOT button while plugging in? ' +
                    'Try again: hold BOOT, then plug in the USB cable.'
                );
            }
            if (e.name === 'NotAllowedError') {
                throw new UserError(
                    'Browser blocked USB access. Check your browser\'s site permissions ' +
                    'for this address and try again.'
                );
            }
            if (e.name === 'NetworkError') {
                throw new UserError(
                    'Another application is using this USB port. Close Arduino IDE, esptool, ' +
                    'or any other serial monitor and try again.'
                );
            }
            throw new UserError(
                'Could not select a device. Please make sure your ESP32-S3 is connected via USB.'
            );
        }
    }

    async function sendSerialJSON(port, data) {
        var encoder = new TextEncoderStream();
        var writableClosed = encoder.readable.pipeTo(port.writable);
        var writer = encoder.writable.getWriter();
        await writer.write(JSON.stringify(data) + '\n');
        writer.close();
        await writableClosed;
    }

    async function sendSerialJSONAndWaitForResponse(port, data, timeoutMs) {
        timeoutMs = timeoutMs || 15000;

        // Set up reader first
        var decoder = new TextDecoderStream();
        var readableClosed = port.readable.pipeTo(decoder.writable);
        var reader = decoder.readable.getReader();

        // Send the data
        var encoder = new TextEncoderStream();
        var writableClosed = encoder.readable.pipeTo(port.writable);
        var writer = encoder.writable.getWriter();
        await writer.write(JSON.stringify(data) + '\n');
        writer.close();
        await writableClosed;

        // Wait for response with timeout
        var buffer = '';
        var startTime = Date.now();
        var response = null;

        try {
            while (Date.now() - startTime < timeoutMs) {
                var result = await Promise.race([
                    reader.read(),
                    new Promise(function (_, reject) {
                        setTimeout(function () {
                            reject(new Error('Timeout waiting for device response'));
                        }, timeoutMs - (Date.now() - startTime));
                    })
                ]);

                if (result.done) {
                    break;
                }

                buffer += result.value;
                var newlineIndex = buffer.indexOf('\n');
                if (newlineIndex !== -1) {
                    var line = buffer.substring(0, newlineIndex).trim();
                    if (line.length > 0) {
                        try {
                            response = JSON.parse(line);
                            break;
                        } catch (e) {
                            // Not valid JSON, continue reading
                        }
                    }
                    buffer = buffer.substring(newlineIndex + 1);
                }
            }
        } finally {
            reader.cancel();
            try { await readableClosed; } catch (e) { /* ignore */ }
        }

        return response;
    }

    async function closePort(port) {
        try { await port.close(); } catch (e) { /* ignore */ }
    }

    // ============================================
    // User Error (non-technical error for display)
    // ============================================
    function UserError(message) {
        this.name = 'UserError';
        this.message = message;
    }
    UserError.prototype = Object.create(Error.prototype);

    function isUserError(e) {
        return e instanceof UserError || (e.name === 'UserError');
    }

    // ============================================
    // HTML Helpers
    // ============================================
    function escapeAttr(s) {
        return String(s || '').replace(/&/g, '&amp;').replace(/"/g, '&quot;')
            .replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    function formatMAC(bytes, offset) {
        var parts = [];
        for (var i = 0; i < 6; i++) {
            parts.push(bytes[offset + i].toString(16).padStart(2, '0').toUpperCase());
        }
        return parts.join(':');
    }

    // ============================================
    // Step Indicator
    // ============================================
    function renderStepIndicator() {
        var el = document.getElementById('wizard-steps');
        if (!el) return;
        var html = '';
        for (var i = 0; i < STEPS.length; i++) {
            var cls = 'wizard-step-dot';
            if (i < state.currentStepIndex) cls += ' completed';
            else if (i === state.currentStepIndex) cls += ' active';
            html += '<div class="' + cls + '">' + (i + 1) + '</div>';
            if (i < STEPS.length - 1) {
                var lineCls = 'wizard-step-line';
                if (i < state.currentStepIndex) lineCls += ' completed';
                html += '<div class="' + lineCls + '"></div>';
            }
        }
        el.innerHTML = html;
    }

    // ============================================
    // Navigation Buttons
    // ============================================
    function renderNav(showBack, nextLabel, onNext, isPrimary) {
        var nav = document.getElementById('wizard-nav');
        if (!nav) return;
        var html = '';
        if (showBack) {
            html += '<button class="wizard-btn wizard-btn-secondary" id="wizard-back">Back</button>';
        }
        html += '<button class="wizard-btn ' +
            (isPrimary !== false ? 'wizard-btn-primary' : 'wizard-btn-secondary') +
            '" id="wizard-next">' + (nextLabel || 'Next') + '</button>';
        nav.innerHTML = html;

        if (showBack) {
            document.getElementById('wizard-back').addEventListener('click', function () {
                goToStep(state.currentStepIndex - 1);
            });
        }
        document.getElementById('wizard-next').addEventListener('click', onNext);
    }

    function hideNav() {
        var nav = document.getElementById('wizard-nav');
        if (nav) nav.innerHTML = '';
    }

    // ============================================
    // Step Renderers
    // ============================================
    // Each renderer populates the content area and returns { cleanup: fn }

    function renderBrowserCheck(contentEl) {
        if (navigator.serial) {
            contentEl.innerHTML =
                '<div class="wizard-center-msg">' +
                '<div class="spinner"></div>' +
                '<p>Checking browser compatibility...</p>' +
                '</div>';
            setTimeout(function () { goToStep(1); }, 400);
            return { cleanup: function () { } };
        }

        contentEl.innerHTML =
            '<div class="wizard-step-content">' +
            '<div class="wizard-icon-large">⚠</div>' +
            '<h2>Browser Not Supported</h2>' +
            '<p>Please use <strong>Google Chrome</strong> or <strong>Microsoft Edge</strong> to use the setup wizard.</p>' +
            '<p class="wizard-muted">Firefox and Safari do not support USB device access required for this wizard.</p>' +
            '</div>';
        hideNav();
        return { cleanup: function () { } };
    }

    function renderConnectDevice(contentEl) {
        contentEl.innerHTML =
            '<div class="wizard-step-content">' +
            '<h2>Connect Your ESP32-S3</h2>' +
            '<p>Connect the ESP32-S3 to your computer using a USB cable.</p>' +
            '<div class="esp32-illustration">' +
            '<svg viewBox="0 0 200 120" width="200" height="120">' +
            '<rect x="20" y="20" width="160" height="80" rx="4" fill="#2d5a27" stroke="#4a8a3f" stroke-width="1.5"/>' +
            '<rect x="0" y="40" width="25" height="40" rx="3" fill="#888" stroke="#aaa" stroke-width="1"/>' +
            '<rect x="2" y="42" width="21" height="36" rx="2" fill="#666"/>' +
            '<rect x="155" y="15" width="25" height="35" rx="2" fill="#333" stroke="#555" stroke-width="1"/>' +
            '<rect x="40" y="80" width="18" height="12" rx="2" fill="#4fc3f7" stroke="#29b6f6" stroke-width="1.5">' +
            '<animate attributeName="opacity" values="1;0.5;1" dur="2s" repeatCount="indefinite"/>' +
            '</rect>' +
            '<text x="49" y="106" text-anchor="middle" fill="#4fc3f7" font-size="8" font-weight="bold">BOOT</text>' +
            '<rect x="70" y="80" width="18" height="12" rx="2" fill="#666" stroke="#888" stroke-width="1"/>' +
            '<text x="79" y="106" text-anchor="middle" fill="#888" font-size="8">RST</text>' +
            '<circle cx="140" cy="85" r="3" fill="#f44336">' +
            '<animate attributeName="opacity" values="1;0.3;1" dur="1.5s" repeatCount="indefinite"/>' +
            '</circle>' +
            '<rect x="85" y="35" width="35" height="35" rx="2" fill="#1a1a1a" stroke="#333" stroke-width="1"/>' +
            '<text x="102" y="56" text-anchor="middle" fill="#555" font-size="7">ESP32</text>' +
            buildPins(30, 18, 15) +
            buildPins(30, 97, 15) +
            '</svg>' +
            '</div>' +
            '<p class="wizard-muted">Hold the <strong style="color:#4fc3f7">BOOT</strong> button while plugging in if the device does not appear.</p>' +
            '<div id="connect-error" class="wizard-error" style="display:none"></div>' +
            '</div>';

        renderNav(false, 'Select Device', function () {
            document.getElementById('connect-error').style.display = 'none';
            var btn = document.getElementById('wizard-next');
            btn.disabled = true;
            btn.textContent = 'Waiting for device...';

            requestPort().then(function () {
                saveState();
                goToStep(state.currentStepIndex + 1);
            }).catch(function (e) {
                var errEl = document.getElementById('connect-error');
                errEl.style.display = 'block';
                errEl.textContent = isUserError(e) ? e.message : 'Could not select device. Please try again.';
                btn.disabled = false;
                btn.textContent = 'Select Device';
            });
        });

        return {
            cleanup: function () { }
        };
    }

    function buildPins(startX, y, count) {
        var html = '';
        for (var i = 0; i < count; i++) {
            html += '<rect x="' + (startX + i * 9) + '" y="' + y + '" width="3" height="5" fill="#c8a84e"/>';
        }
        return html;
    }

    var BOOTLOADER_SVG =
        '<svg viewBox="0 0 200 120" width="180" height="108" style="display:block;margin:8px auto">' +
        '<rect x="20" y="20" width="160" height="80" rx="4" fill="#2d5a27" stroke="#4a8a3f" stroke-width="1.5"/>' +
        '<rect x="0" y="40" width="25" height="40" rx="3" fill="#888" stroke="#aaa" stroke-width="1"/>' +
        '<rect x="2" y="42" width="21" height="36" rx="2" fill="#666"/>' +
        '<rect x="155" y="15" width="25" height="35" rx="2" fill="#333" stroke="#555" stroke-width="1"/>' +
        '<rect x="40" y="80" width="18" height="12" rx="2" fill="#4fc3f7" stroke="#29b6f6" stroke-width="2">' +
        '<animate attributeName="opacity" values="1;0.4;1" dur="1s" repeatCount="indefinite"/>' +
        '</rect>' +
        '<text x="49" y="106" text-anchor="middle" fill="#4fc3f7" font-size="8" font-weight="bold">BOOT</text>' +
        '<rect x="70" y="80" width="18" height="12" rx="2" fill="#f44336" stroke="#e53935" stroke-width="2">' +
        '<animate attributeName="opacity" values="1;0.4;1" dur="1.4s" repeatCount="indefinite"/>' +
        '</rect>' +
        '<text x="79" y="106" text-anchor="middle" fill="#f44336" font-size="8" font-weight="bold">RST</text>' +
        '<rect x="85" y="35" width="35" height="35" rx="2" fill="#1a1a1a" stroke="#333" stroke-width="1"/>' +
        '<text x="102" y="56" text-anchor="middle" fill="#555" font-size="7">ESP32</text>' +
        buildPins(30, 18, 15) + buildPins(30, 97, 15) +
        '</svg>';

    function renderBootloaderHelp(retryCount) {
        var escalated = retryCount >= 2;
        return '<div class="wizard-bootloader-help" style="background:#1a2a1a;border:1px solid #4fc3f7;border-radius:6px;padding:12px;margin:12px 0;text-align:center">' +
            '<p style="margin:0 0 8px;color:#4fc3f7;font-weight:bold">' + (escalated ? '⚠ Still not working?' : 'Device not in download mode') + '</p>' +
            (escalated
                ? '<p style="font-size:12px;color:#aaa;margin:0 0 8px">Try a different USB cable (data cables only, not charge-only). If using a USB hub, connect directly to your computer.</p>'
                : '<p style="font-size:12px;color:#ccc;margin:0 0 8px">Hold <strong style="color:#4fc3f7">BOOT</strong>, press &amp; release <strong style="color:#f44336">RST</strong>, then release <strong style="color:#4fc3f7">BOOT</strong>.</p>') +
            BOOTLOADER_SVG +
            '</div>';
    }

    function renderFlashFirmware(contentEl) {
        var flashRetryCount = 0;
        var origConsole = { log: console.log, warn: console.warn, error: console.error };

        function appendLog(level, args) {
            var msg = Array.prototype.slice.call(args).map(function (a) {
                try { return (typeof a === 'object') ? JSON.stringify(a) : String(a); } catch (e) { return String(a); }
            }).join(' ');
            var ts = new Date().toISOString().slice(11, 23);
            var logEl = document.getElementById('flash-log-body');
            if (logEl) {
                var color = level === 'error' ? '#ef9a9a' : level === 'warn' ? '#ffe082' : '#b0bec5';
                var line = document.createElement('div');
                line.style.cssText = 'font-size:11px;color:' + color + ';word-break:break-all;margin:1px 0';
                line.textContent = '[' + ts + '] ' + msg;
                logEl.appendChild(line);
                logEl.scrollTop = logEl.scrollHeight;
            }
        }

        function patchConsole() {
            ['log', 'warn', 'error'].forEach(function (m) {
                console[m] = function () { origConsole[m].apply(console, arguments); appendLog(m, arguments); };
            });
        }

        function restoreConsole() {
            ['log', 'warn', 'error'].forEach(function (m) { console[m] = origConsole[m]; });
        }

        // Single container: button → progress bar → status text → log → retry help
        contentEl.innerHTML =
            '<div class="wizard-step-content">' +
            '<h2>Flash Firmware</h2>' +
            '<p>The wizard will flash the Spaxel firmware onto your ESP32-S3. This takes about 45–90 seconds.</p>' +
            '<details style="margin-bottom:12px;font-size:13px;color:#aaa">' +
            '<summary style="cursor:pointer;color:#80cbc4">Having trouble connecting?</summary>' +
            '<div style="padding:8px 0 0">' +
            '<p style="margin:4px 0">Before clicking Start Flashing, put the board in download mode:</p>' +
            '<ol style="margin:4px 0 4px 16px;padding:0">' +
            '<li>Hold <strong style="color:#4fc3f7">BOOT</strong></li>' +
            '<li>Press &amp; release <strong style="color:#f44336">RST</strong></li>' +
            '<li>Release <strong style="color:#4fc3f7">BOOT</strong></li>' +
            '</ol>' +
            BOOTLOADER_SVG +
            '</div></details>' +
            '<div id="flash-main">' +
            '  <div id="flash-btn-area"></div>' +
            '  <div id="flash-progress-bar" style="display:none;margin:12px 0">' +
            '    <div class="progress-bar"><div class="progress-fill" id="flash-progress-fill"></div></div>' +
            '  </div>' +
            '  <p id="flash-status-text" style="display:none;margin:8px 0;font-size:13px;color:#80cbc4"></p>' +
            '  <div id="flash-recovery" style="display:none"></div>' +
            '  <details id="flash-log-details" style="margin-top:12px;font-size:12px">' +
            '    <summary style="cursor:pointer;color:#546e7a">Show install log</summary>' +
            '    <div id="flash-log-body" style="background:#0a0e13;border:1px solid #263238;border-radius:4px;' +
            'padding:8px;margin-top:4px;max-height:160px;overflow-y:auto;font-family:monospace"></div>' +
            '  </details>' +
            '</div>' +
            '</div>';
        hideNav();
        patchConsole();
        appendLog('log', ['Flash step loaded']);

        if (!customElements.get('esp-web-install-button')) {
            restoreConsole();
            document.getElementById('flash-btn-area').innerHTML =
                '<p class="wizard-error">Firmware flashing component failed to load. ' +
                'Please refresh the page and ensure you have a stable internet connection.</p>';
            renderNav(true, 'Skip Flashing', function () { goToStep(state.currentStepIndex + 1); }, false);
            return { cleanup: function () { } };
        }

        function setStatus(msg) {
            var el = document.getElementById('flash-status-text');
            if (!el) { return; }
            el.style.display = msg ? 'block' : 'none';
            el.textContent = msg;
        }

        function mountInstallButton() {
            var btnArea = document.getElementById('flash-btn-area');
            if (!btnArea) { return; }
            btnArea.innerHTML = '';
            var installBtn = document.createElement('esp-web-install-button');
            installBtn.setAttribute('manifest', '/api/firmware/manifest');
            installBtn.innerHTML = '<button class="wizard-btn wizard-btn-primary" slot="activate">' +
                (flashRetryCount > 0 ? 'Try Again' : 'Start Flashing') + '</button>';
            btnArea.appendChild(installBtn);

            var flashCompleteTimer = null;
            var dlgObserver = null;

            function onFlashComplete() {
                if (flashCompleteTimer) { clearTimeout(flashCompleteTimer); flashCompleteTimer = null; }
                if (dlgObserver) { dlgObserver.disconnect(); dlgObserver = null; }
                restoreConsole();
                document.getElementById('flash-progress-bar').style.display = 'none';
                setStatus('✓ Firmware flashed successfully!');
                document.getElementById('flash-status-text').style.color = '#a5d6a7';
                saveState();
                setTimeout(function () { goToStep(state.currentStepIndex + 1); }, 1500);
            }

            // Inject CSS into the shadow root to hide the esp-web-tools dialog once
            // flashing starts — we drive the UI inline from that point on.
            function hideDlgOverlay() {
                var sr = installBtn.shadowRoot;
                if (!sr || sr.querySelector('#ewt-suppress')) { return; }
                var s = document.createElement('style');
                s.id = 'ewt-suppress';
                // Hide the dialog and its backdrop; keep the activate slot visible
                s.textContent = ':host > *:not(slot) { display:none !important; }';
                sr.appendChild(s);
            }

            function showProgressUI() {
                btnArea.style.display = 'none';
                document.getElementById('flash-progress-bar').style.display = 'block';
                document.getElementById('flash-recovery').style.display = 'none';
                document.getElementById('flash-log-details').open = true;
            }

            // esp-web-tools v10 only fires state-changed, and it fires from the dialog
            // element inside the shadow root (not composed), so the event never reaches
            // listeners on the host. We use a MutationObserver to find the dialog element
            // and attach directly to it.
            function attachToDialog(dlg) {
                dlg.addEventListener('state-changed', function (e) {
                    var detail = e.detail || {};
                    var s = detail.state;
                    var det = detail.details || {};
                    appendLog('log', ['state: ' + s]);

                    if (s === 'erasing') {
                        hideDlgOverlay();
                        showProgressUI();
                        document.getElementById('flash-progress-fill').style.width = '5%';
                        setStatus('Erasing flash...');
                    } else if (s === 'writing') {
                        hideDlgOverlay();
                        showProgressUI();
                        // percentage is 0–1
                        var pct = Math.round((det.percentage || 0) * 100);
                        document.getElementById('flash-progress-fill').style.width = pct + '%';
                        setStatus('Flashing... ' + pct + '%');
                        // Fallback: if finished state never fires, show Continue after 4s
                        if (pct >= 100 && !flashCompleteTimer) {
                            flashCompleteTimer = setTimeout(function () {
                                flashCompleteTimer = null;
                                appendLog('log', ['finished state not received — manual continue']);
                                document.getElementById('flash-progress-bar').style.display = 'none';
                                setStatus('✓ Flash complete.');
                                document.getElementById('flash-status-text').style.color = '#a5d6a7';
                                btnArea.style.display = 'block';
                                btnArea.innerHTML = '<button class="wizard-btn wizard-btn-primary" id="flash-continue-btn">Continue →</button>';
                                document.getElementById('flash-continue-btn').addEventListener('click', function () {
                                    saveState();
                                    goToStep(state.currentStepIndex + 1);
                                });
                            }, 4000);
                        }
                    } else if (s === 'finished') {
                        appendLog('log', ['flash-success']);
                        onFlashComplete();
                    } else if (s === 'error') {
                        if (dlgObserver) { dlgObserver.disconnect(); dlgObserver = null; }
                        if (flashCompleteTimer) { clearTimeout(flashCompleteTimer); flashCompleteTimer = null; }
                        hideDlgOverlay();
                        flashRetryCount++;
                        appendLog('error', ['flash-error: ' + (detail.message || JSON.stringify(detail))]);
                        document.getElementById('flash-progress-bar').style.display = 'none';
                        document.getElementById('flash-log-details').open = true;
                        setStatus('');
                        var recovery = document.getElementById('flash-recovery');
                        recovery.style.display = 'block';
                        recovery.innerHTML = renderBootloaderHelp(flashRetryCount);
                        btnArea.style.display = 'block';
                        mountInstallButton();
                    }
                });
            }

            // Watch for the dialog element to appear in the shadow root
            var sr = installBtn.shadowRoot;
            if (sr) {
                dlgObserver = new MutationObserver(function (mutations) {
                    for (var i = 0; i < mutations.length; i++) {
                        var added = mutations[i].addedNodes;
                        for (var j = 0; j < added.length; j++) {
                            var node = added[j];
                            if (node.nodeType === 1 && node.tagName.toLowerCase() !== 'style') {
                                // This is the dialog element — attach and stop observing
                                if (dlgObserver) { dlgObserver.disconnect(); dlgObserver = null; }
                                attachToDialog(node);
                                return;
                            }
                        }
                    }
                });
                dlgObserver.observe(sr, { childList: true });
            }
        }

        mountInstallButton();
        return { cleanup: restoreConsole };
    }

    function renderProvisionWifi(contentEl) {
        contentEl.innerHTML =
            '<div class="wizard-step-content">' +
            '<h2>Configure WiFi</h2>' +
            '<p>Enter your WiFi credentials. The ESP32-S3 needs to connect to the same network as this computer.</p>' +
            '<form id="wifi-form" class="wizard-form">' +
            '<div class="form-group">' +
            '<label for="wifi-ssid">WiFi Network Name (SSID)</label>' +
            '<input type="text" id="wifi-ssid" required placeholder="MyWiFi" value="' + escapeAttr(state.wifiSSID) + '" autocomplete="off">' +
            '</div>' +
            '<div class="form-group">' +
            '<label for="wifi-pass">WiFi Password</label>' +
            '<input type="password" id="wifi-pass" required placeholder="Password" value="' + escapeAttr(state.wifiPass) + '" autocomplete="off">' +
            '</div>' +
            '<details class="wizard-details">' +
            '<summary>Advanced: Mothership Address</summary>' +
            '<div class="form-group" style="margin-top:8px">' +
            '<label for="ms-host">Host (leave blank for mDNS auto-discovery)</label>' +
            '<input type="text" id="ms-host" placeholder="spaxel-mothership.local" value="' + escapeAttr(state.mothershipHost) + '" autocomplete="off">' +
            '</div>' +
            '<div class="form-group">' +
            '<label for="ms-port">Port</label>' +
            '<input type="number" id="ms-port" value="' + state.mothershipPort + '" min="1" max="65535">' +
            '</div>' +
            '</details>' +
            '<div id="provision-error" class="wizard-error" style="display:none"></div>' +
            '<button type="submit" class="wizard-btn wizard-btn-primary">Send Configuration</button>' +
            '</form>' +
            '</div>';

        renderNav(true, '', function () { }, true);
        // Hide the default Next button since we use the form submit
        hideNav();

        document.getElementById('wifi-form').addEventListener('submit', function (e) {
            e.preventDefault();
            var ssid = document.getElementById('wifi-ssid').value.trim();
            var pass = document.getElementById('wifi-pass').value;
            var msHost = document.getElementById('ms-host').value.trim();
            var msPort = parseInt(document.getElementById('ms-port').value, 10) || 8080;

            if (!ssid) {
                showFormError('provision-error', 'Please enter a WiFi network name.');
                return;
            }

            state.wifiSSID = ssid;
            state.wifiPass = pass;
            state.mothershipHost = msHost;
            state.mothershipPort = msPort;

            var btn = e.target.querySelector('button[type="submit"]');
            btn.disabled = true;
            btn.textContent = 'Sending...';

            provisionAndSend(ssid, pass, msHost, msPort)
                .then(function () {
                    // Fetch current known nodes before provisioning
                    return fetch(CONFIG.nodesEndpoint)
                        .then(function (r) { return r.json(); })
                        .then(function (nodes) {
                            state.knownMACs = (nodes || []).map(function (n) { return n.mac; });
                        })
                        .catch(function () { /* ignore */ });
                })
                .then(function () {
                    saveState();
                    goToStep(state.currentStepIndex + 1);
                })
                .catch(function (err) {
                    var msg = isUserError(err) ? err.message :
                        'Could not send configuration. Make sure the device is connected via USB and try again.';
                    showFormError('provision-error', msg);
                    btn.disabled = false;
                    btn.textContent = 'Send Configuration';
                });
        });

        return { cleanup: function () { } };
    }

    function provisionAndSend(ssid, pass, msHost, msPort) {
        // Try server-side provisioning first (generates proper node_id and token)
        return fetch(CONFIG.provisioningEndpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ wifi_ssid: ssid, wifi_pass: pass }),
        })
            .then(function (r) {
                if (!r.ok) throw new Error('provisioning server error');
                return r.json();
            })
            .then(function (payload) {
                // Apply user overrides for mothership address
                if (msHost) payload.ms_mdns = msHost;
                if (msPort) payload.ms_port = msPort;
                return sendPayloadOverSerial(payload);
            })
            .catch(function () {
                // Fallback: assemble payload client-side
                var payload = {
                    version: 1,
                    wifi_ssid: ssid,
                    wifi_pass: pass,
                    node_id: crypto.randomUUID ? crypto.randomUUID() :
                        'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
                            var r = Math.random() * 16 | 0;
                            return (c === 'x' ? r : (r & 0x3 | 0x8)).toString(16);
                        }),
                    node_token: '',
                    ms_mdns: msHost || 'spaxel-mothership.local',
                    ms_port: msPort,
                    debug: false,
                };
                return sendPayloadOverSerial(payload);
            });
    }

    function sendPayloadOverSerial(payload) {
        // Firmware expects {"provision": {...}} format
        var wrappedPayload = { provision: payload };
        return getAuthorizedPort()
            .then(function (port) {
                if (!port) throw new UserError(
                    'No device found. Please go back and connect your ESP32-S3 again.'
                );
                return port;
            })
            .then(function (port) {
                return port.open({ baudRate: CONFIG.serialBaudRate })
                    .then(function () { return port; })
                    .catch(function () {
                        // Port might already be open from a previous step
                        return port;
                    });
            })
            .then(function (port) {
                return sendSerialJSONAndWaitForResponse(port, wrappedPayload, 15000)
                    .then(function (response) {
                        if (!response) {
                            throw new UserError(
                                'No response from device. Please ensure the ESP32-S3 is connected and try again.'
                            );
                        }
                        if (response.ok === false) {
                            var errorMsg = response.error || 'Unknown error';
                            if (errorMsg === 'missing_provision_key') {
                                throw new UserError('Firmware communication error. Please try again.');
                            }
                            if (errorMsg === 'nvs_write_failed') {
                                throw new UserError('Failed to save configuration to device. Please try again.');
                            }
                            throw new UserError('Provisioning failed: ' + errorMsg);
                        }
                        // Success - return the MAC address
                        return response.mac;
                    });
            });
    }

    function showFormError(id, msg) {
        var el = document.getElementById(id);
        if (el) { el.style.display = 'block'; el.textContent = msg; }
    }

    function renderDetectNode(contentEl) {
        contentEl.innerHTML =
            '<div class="wizard-step-content">' +
            '<h2>Detecting Your Node</h2>' +
            '<p>The ESP32-S3 is booting and connecting to your WiFi network. This may take up to 30 seconds.</p>' +
            '<div class="wizard-center-msg">' +
            '<div class="spinner"></div>' +
            '<p id="detect-status">Waiting for node to appear...</p>' +
            '<p id="detect-countdown" class="wizard-muted"></p>' +
            '</div>' +
            '<div id="detect-troubleshoot" style="display:none">' +
            '<h3>Troubleshooting</h3>' +
            '<ul class="wizard-list">' +
            '<li>Make sure your WiFi network is <strong>2.4 GHz</strong> (ESP32-S3 does not support 5 GHz)</li>' +
            '<li>Check that the SSID and password are correct</li>' +
            '<li>Ensure the ESP32-S3 is within range of your WiFi router</li>' +
            '<li>Your router may block device-to-device communication (AP isolation) — check router settings</li>' +
            '<li>If using VLANs, ensure the ESP32-S3 and this computer are on the same VLAN</li>' +
            '</ul>' +
            '<div id="detect-captive" style="display:none" class="wizard-warn">' +
            '<p>If the node cannot connect after 10 failed attempts, it enters <strong>captive portal mode</strong>. ' +
            'Look for a WiFi network named <strong>Spaxel-Setup</strong> and connect to it to reconfigure.</p>' +
            '</div>' +
            '</div>' +
            '</div>';

        hideNav();

        var startTime = Date.now();
        var timeoutMs = CONFIG.nodePollTimeout;

        state.pollTimer = setInterval(function () {
            var elapsed = Date.now() - startTime;
            var remaining = Math.max(0, Math.ceil((timeoutMs - elapsed) / 1000));

            document.getElementById('detect-countdown').textContent =
                'Timeout in ' + remaining + 's';

            if (elapsed >= timeoutMs) {
                clearInterval(state.pollTimer);
                state.pollTimer = null;
                document.getElementById('detect-status').textContent = 'Node not found within timeout.';
                document.getElementById('detect-status').className = 'wizard-error';
                document.getElementById('detect-countdown').style.display = 'none';
                document.getElementById('detect-troubleshoot').style.display = 'block';
                document.getElementById('detect-captive').style.display = 'block';

                renderNav(true, 'Retry Detection', function () {
                    goToStep(state.currentStepIndex);
                });
                return;
            }

            fetch(CONFIG.nodesEndpoint)
                .then(function (r) { return r.json(); })
                .then(function (nodes) {
                    var currentMACs = (nodes || []).map(function (n) { return n.mac; });
                    var newMAC = null;
                    for (var i = 0; i < currentMACs.length; i++) {
                        if (state.knownMACs.indexOf(currentMACs[i]) === -1) {
                            newMAC = currentMACs[i];
                            break;
                        }
                    }

                    // Also accept the first online node if no known MACs were recorded
                    if (!newMAC && state.knownMACs.length === 0 && currentMACs.length > 0) {
                        newMAC = currentMACs[0];
                    }

                    if (newMAC) {
                        clearInterval(state.pollTimer);
                        state.pollTimer = null;
                        state.nodeMAC = newMAC;
                        document.getElementById('detect-status').textContent =
                            'Found node: ' + newMAC;
                        document.getElementById('detect-status').className = 'wizard-success';
                        saveState();
                        setTimeout(function () { goToStep(state.currentStepIndex + 1); }, 1000);
                    }
                })
                .catch(function () { /* network error, will retry */ });
        }, CONFIG.nodePollInterval);

        return {
            cleanup: function () {
                if (state.pollTimer) { clearInterval(state.pollTimer); state.pollTimer = null; }
            }
        };
    }

    function renderCalibrate(contentEl) {
        state.calibratePhase = 'walk';
        state.csiHistory = [];
        state.calibrationLinks = [];

        contentEl.innerHTML =
            '<div class="wizard-step-content">' +
            '<h2>Guided Calibration</h2>' +
            '<div id="calibrate-instructions"></div>' +
            '<canvas id="calibrate-canvas" width="480" height="120" style="width:100%;height:120px;border-radius:4px;margin:12px 0"></canvas>' +
            '<div id="calibrate-status" class="wizard-muted"></div>' +
            '</div>';

        hideNav();

        // Connect to dashboard WebSocket for live CSI data
        connectCalibrationWS();

        // Phase 1: Walk around
        startCalibratePhase('walk');

        return {
            cleanup: function () {
                if (state.calibrateTimer) { clearTimeout(state.calibrateTimer); state.calibrateTimer = null; }
                if (state.ws) { state.ws.close(); state.ws = null; }
                state.calibratePhase = 'idle';
            }
        };
    }

    function connectCalibrationWS() {
        var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        var url = protocol + '//' + window.location.host + '/ws/dashboard';

        try {
            state.ws = new WebSocket(url);
            state.ws.binaryType = 'arraybuffer';

            state.ws.onmessage = function (event) {
                if (event.data instanceof ArrayBuffer && state.nodeMAC) {
                    var frame = parseCSIFrame(event.data);
                    if (frame && (frame.nodeMAC === state.nodeMAC || frame.peerMAC === state.nodeMAC)) {
                        pushCSISample(frame);
                        drawCalibrateWaveform();
                        // Track unique links for post-calibration card
                        var linkKey = frame.nodeMAC + ':' + frame.peerMAC;
                        if (state.calibrationLinks.indexOf(linkKey) === -1) {
                            state.calibrationLinks.push(linkKey);
                        }
                    }
                }
            };

            state.ws.onerror = function () { /* non-critical */ };
        } catch (e) { /* WebSocket not available, non-critical */ }
    }

    // ============================================
    // CSI Frame Parser (matches Go binary format)
    // ============================================
    function parseCSIFrame(buffer) {
        var view = new DataView(buffer);
        var bytes = new Uint8Array(buffer);
        if (bytes.length < 24) return null;

        var nodeMAC = formatMAC(bytes, 0);
        var peerMAC = formatMAC(bytes, 6);
        var nSub = bytes[23];
        var channel = bytes[22];

        if (channel === 0 || channel > 14) return null;
        var expectedLen = 24 + nSub * 2;
        if (bytes.length !== expectedLen) return null;

        var sum = 0;
        for (var i = 0; i < nSub; i++) {
            var offset = 24 + i * 2;
            var iVal = bytes[offset] > 127 ? bytes[offset] - 256 : bytes[offset];
            var qVal = bytes[offset + 1] > 127 ? bytes[offset + 1] - 256 : bytes[offset + 1];
            sum += Math.sqrt(iVal * iVal + qVal * qVal);
        }
        return {
            nodeMAC: nodeMAC,
            peerMAC: peerMAC,
            meanAmplitude: nSub > 0 ? sum / nSub : 0,
            rssi: view.getInt8(20),
        };
    }

    function pushCSISample(frame) {
        state.csiHistory.push({ t: Date.now(), amp: frame.meanAmplitude });
        var cutoff = Date.now() - 10000;
        while (state.csiHistory.length > 0 && state.csiHistory[0].t < cutoff) {
            state.csiHistory.shift();
        }
    }

    function drawCalibrateWaveform() {
        var canvas = document.getElementById('calibrate-canvas');
        if (!canvas) return;
        var ctx = canvas.getContext('2d');
        var w = canvas.width;
        var h = canvas.height;

        ctx.fillStyle = '#12122a';
        ctx.fillRect(0, 0, w, h);

        var data = state.csiHistory;
        if (data.length < 2) {
            ctx.fillStyle = '#444';
            ctx.font = '12px sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('Waiting for CSI data...', w / 2, h / 2 + 4);
            return;
        }

        var maxAmp = 1;
        for (var i = 0; i < data.length; i++) {
            if (data[i].amp > maxAmp) maxAmp = data[i].amp;
        }

        var xStep = w / Math.max(data.length - 1, 1);
        var padTop = 8;
        var padBottom = 8;
        var plotH = h - padTop - padBottom;

        ctx.lineWidth = 1.5;
        ctx.strokeStyle = '#4fc3f7';
        ctx.beginPath();
        for (var j = 0; j < data.length; j++) {
            var x = j * xStep;
            var y = padTop + plotH - (data[j].amp / maxAmp) * plotH;
            if (j === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        }
        ctx.stroke();

        // Fill area
        ctx.lineTo((data.length - 1) * xStep, padTop + plotH);
        ctx.lineTo(0, padTop + plotH);
        ctx.closePath();
        ctx.fillStyle = 'rgba(79,195,247,0.1)';
        ctx.fill();
    }

    // ============================================
    // Calibration Phases
    // ============================================
    function startCalibratePhase(phase) {
        state.calibratePhase = phase;
        var instructions = document.getElementById('calibrate-instructions');
        var statusEl = document.getElementById('calibrate-status');

        switch (phase) {
            case 'walk':
                instructions.innerHTML =
                    '<div class="calibrate-phase">' +
                    '<div class="calibrate-phase-number">1 of 3</div>' +
                    '<h3>Walk Around Your Space</h3>' +
                    '<p>Walk around the room for <strong>30 seconds</strong>. The waveform below should show activity.</p>' +
                    '</div>';
                statusEl.textContent = 'If the waveform stays flat, try rotating the node or moving closer.';
                runCalibrateCountdown(CONFIG.calibrateWalkDuration, function () {
                    if (state.calibratePhase !== 'walk') return;
                    // Check if we got any data
                    if (state.csiHistory.length < 5) {
                        statusEl.innerHTML =
                            '<span class="wizard-warn">Very little CSI data received. ' +
                            'The node connected but is not sensing yet. Check the antenna ' +
                            'orientation \u2014 the PCB antenna should face away from walls.</span>';
                    }
                    startCalibratePhase('still');
                });
                break;

            case 'still':
                instructions.innerHTML =
                    '<div class="calibrate-phase">' +
                    '<div class="calibrate-phase-number">2 of 3</div>' +
                    '<h3>Stand Still</h3>' +
                    '<p>Stand still at the far end of the room. The system will capture a baseline.</p>' +
                    '</div>';
                statusEl.innerHTML = '<span id="still-countdown" style="color:#4fc3f7;font-weight:600"></span>';
                runCalibrateCountdown(CONFIG.calibrateStillDuration, function () {
                    if (state.calibratePhase !== 'still') return;
                    statusEl.innerHTML = '<span class="wizard-success">✓ Baseline captured</span>';
                    startCalibratePhase('walk_through');
                }, function (remaining) {
                    var el = document.getElementById('still-countdown');
                    if (el) el.textContent = remaining + 's remaining';
                });
                break;

            case 'walk_through':
                instructions.innerHTML =
                    '<div class="calibrate-phase">' +
                    '<div class="calibrate-phase-number">3 of 3</div>' +
                    '<h3>Walk Through the Centre</h3>' +
                    '<p>Walk through the centre of the room. The sensor should detect your movement.</p>' +
                    '</div>';
                statusEl.textContent = 'The sensor can see you!';
                runCalibrateCountdown(CONFIG.calibrateWalkThroughDuration, function () {
                    if (state.calibratePhase !== 'walk_through') return;
                    showPostCalibrationCard();
                });
                break;
        }
    }

    function runCalibrateCountdown(durationMs, onComplete, onTick) {
        var startTime = Date.now();
        function tick() {
            var elapsed = Date.now() - startTime;
            var remaining = Math.max(0, Math.ceil((durationMs - elapsed) / 1000));

            if (onTick) onTick(remaining);

            if (elapsed >= durationMs) {
                onComplete();
                return;
            }
            state.calibrateTimer = setTimeout(tick, 200);
        }
        tick();
    }

    // ============================================
    // Post-Calibration Reinforcement Card
    // ============================================
    function showPostCalibrationCard() {
        var instructions = document.getElementById('calibrate-instructions');
        var statusEl = document.getElementById('calibrate-status');
        if (!instructions || !statusEl) return;

        var linkCount = state.calibrationLinks.length || 1;
        var nodeLabel = state.nodeMAC || 'Node';

        instructions.innerHTML =
            '<div class="post-cal-card">' +
            '<div class="wizard-icon-large wizard-success-icon">\u2713</div>' +
            '<h3>You\'re All Set!</h3>' +
            '<p class="post-cal-summary">' + escapeAttr(nodeLabel) + ' calibrated. ' +
            linkCount + ' sensing link' + (linkCount !== 1 ? 's' : '') +
            ' active. Motion detection: Ready.</p>' +
            '<p class="post-cal-expect">You\'ll see the CSI waveform react when someone walks through the room. ' +
            'The system learns your space over the next few hours and becomes more accurate.</p>' +
            '<div class="post-cal-actions">' +
            '<button class="wizard-btn wizard-btn-secondary" id="post-cal-add">Add another node</button>' +
            '<button class="wizard-btn wizard-btn-primary" id="post-cal-done">I\'m done for now</button>' +
            '</div>' +
            '</div>';
        statusEl.innerHTML = '';

        document.getElementById('post-cal-add').addEventListener('click', function () {
            clearState();
            closeWizard();
            // Let the user start a new wizard from the dashboard
        });

        document.getElementById('post-cal-done').addEventListener('click', function () {
            saveState();
            goToStep(state.currentStepIndex + 1);
        });
    }

    function renderPlacement(contentEl) {
        contentEl.innerHTML =
            '<div class="wizard-step-content">' +
            '<h2>Node Placement</h2>' +
            '<p>Your node is online and calibrated. For optimal coverage:</p>' +
            '<ul class="wizard-list">' +
            '<li>Place nodes at <strong>opposite corners</strong> of the room for best coverage</li>' +
            '<li>Keep nodes at least <strong>2 meters</strong> apart</li>' +
            '<li>Avoid placing nodes near <strong>metal objects</strong> or <strong>thick walls</strong></li>' +
            '<li>Mount nodes at <strong>chest height</strong> (1.2-1.5m) for person detection</li>' +
            '<li>Ensure nodes have a <strong>clear line of sight</strong> to each other</li>' +
            '</ul>' +
            '<p class="wizard-muted">You can add more nodes later by running this wizard again.</p>' +
            '</div>';

        renderNav(true, 'Finish Setup', function () {
            saveState();
            goToStep(state.currentStepIndex + 1);
        });

        return { cleanup: function () { } };
    }

    function renderComplete(contentEl) {
        var nodeInfo = state.nodeMAC ?
            '<p>Your node <strong>' + state.nodeMAC + '</strong> is now online.</p>' : '';

        contentEl.innerHTML =
            '<div class="wizard-step-content" style="text-align:center">' +
            '<div class="wizard-icon-large wizard-success-icon">✓</div>' +
            '<h2>Setup Complete!</h2>' +
            nodeInfo +
            '<p>You can now monitor your node and view live CSI data on the dashboard.</p>' +
            '<div style="margin-top:24px">' +
            '<button class="wizard-btn wizard-btn-primary" id="goto-dashboard">Go to Dashboard</button>' +
            '</div>' +
            '</div>';

        hideNav();

        document.getElementById('goto-dashboard').addEventListener('click', function () {
            closeWizard();
        });

        return { cleanup: function () { } };
    }

    // ============================================
    // Step Router
    // ============================================
    var renderers = {
        browser_check: renderBrowserCheck,
        connect_device: renderConnectDevice,
        flash_firmware: renderFlashFirmware,
        provision_wifi: renderProvisionWifi,
        detect_node: renderDetectNode,
        calibrate: renderCalibrate,
        placement: renderPlacement,
        complete: renderComplete,
    };

    var activeCleanup = null;

    function goToStep(index) {
        if (index < 0 || index >= STEPS.length) return;

        // Cleanup previous step
        if (activeCleanup) {
            activeCleanup.cleanup();
            activeCleanup = null;
        }

        state.currentStepIndex = index;
        saveState();
        renderStepIndicator();

        var contentEl = document.getElementById('wizard-content');
        if (!contentEl) return;
        contentEl.innerHTML = '';

        var step = STEPS[index];
        if (renderers[step.id]) {
            activeCleanup = renderers[step.id](contentEl);
        }
    }

    // ============================================
    // Wizard Container
    // ============================================
    function createWizardUI() {
        var overlay = document.createElement('div');
        overlay.id = 'wizard-overlay';

        overlay.innerHTML =
            '<div id="wizard-card">' +
            '<div id="wizard-header">' +
            '<h1>Spaxel Setup</h1>' +
            '<button class="wizard-close" id="wizard-close-btn" title="Close">&times;</button>' +
            '</div>' +
            '<div id="wizard-steps"></div>' +
            '<div id="wizard-content"></div>' +
            '<div id="wizard-nav"></div>' +
            '</div>';

        document.body.appendChild(overlay);
        state.container = overlay;

        document.getElementById('wizard-close-btn').addEventListener('click', closeWizard);
        overlay.addEventListener('click', function (e) {
            if (e.target === overlay) closeWizard();
        });
    }

    function closeWizard() {
        if (activeCleanup) {
            activeCleanup.cleanup();
            activeCleanup = null;
        }
        if (state.pollTimer) { clearInterval(state.pollTimer); state.pollTimer = null; }
        if (state.ws) { state.ws.close(); state.ws = null; }

        if (state.container && state.container.parentNode) {
            state.container.parentNode.removeChild(state.container);
        }
        state.container = null;

        // Don't clear state — allow resume if user navigates back to /onboard
    }

    function startWizard() {
        // Prevent duplicate instances
        if (state.container) {
            state.container.parentNode.removeChild(state.container);
        }

        createWizardUI();

        var saved = loadState();
        if (saved && typeof saved.currentStepIndex === 'number' && saved.currentStepIndex >= 0) {
            state.currentStepIndex = saved.currentStepIndex;
            state.nodeMAC = saved.nodeMAC || null;
            state.knownMACs = saved.knownMACs || [];
            state.wifiSSID = saved.wifiSSID || '';
            state.wifiPass = saved.wifiPass || '';
            state.mothershipHost = saved.mothershipHost || '';
            state.mothershipPort = saved.mothershipPort || 8080;
            goToStep(state.currentStepIndex);
        } else {
            goToStep(0);
        }
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelOnboard = {
        start: startWizard,
        close: closeWizard,
    };

    // Expose internals for testing
    window.SpaxelOnboard._state = state;
    window.SpaxelOnboard._CONFIG = CONFIG;
    window.SpaxelOnboard._STEPS = STEPS;
    window.SpaxelOnboard._parseCSIFrame = parseCSIFrame;
    window.SpaxelOnboard._provisionAndSend = provisionAndSend;
    window.SpaxelOnboard._UserError = UserError;
    window.SpaxelOnboard._isUserError = isUserError;
    window.SpaxelOnboard._showPostCalibrationCard = showPostCalibrationCard;

    // Auto-start if on /onboard path
    if (window.location.pathname === '/onboard') {
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', startWizard);
        } else {
            startWizard();
        }
    }
})();
