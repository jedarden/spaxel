/**
 * Spaxel Dashboard - Proactive Quality Assistance
 *
 * Provides proactive prompts for:
 * - Detection quality degradation warnings with root-cause analysis
 * - Repeated-setting change detection (user struggling to tune)
 * - Post-feedback explanations after false positive reports
 */

(function() {
    'use strict';

    // ===== State =====
    let qualityPromptActive = false;
    let qualityPromptLinkID = null;
    let qualityPromptStartTime = null;
    let dismissedQualityPrompts = new Set();
    let pulsingAnimationID = null; // For link pulsing animation
    let pulseTime = 0;

    // Repeated-setting change tracking
    let settingChangeHistory = JSON.parse(localStorage.getItem('spaxel_setting_changes') || '{}');
    let settingChangeHintShown = JSON.parse(localStorage.getItem('spaxel_setting_hints_shown') || '{}');

    // Settings that qualify for repeated-change detection
    const QUALIFYING_SETTINGS = new Set([
        'delta_rms_threshold',
        'breathing_sensitivity',
        'tau_s',
        'fresnel_decay',
        'n_subcarriers'
    ]);

    // ===== Quality Prompt Detection =====
    /**
     * Monitor link quality and show prompts when quality drops below 0.6 for >5 minutes
     */
    function monitorLinkQuality(links) {
        if (!links || !Array.isArray(links)) {
            return;
        }

        const now = Date.now();
        const QUALITY_THRESHOLD = 0.6;
        const DURATION_MS = 5 * 60 * 1000; // 5 minutes

        // Find links with poor quality
        let poorQualityLinks = links.filter(link => {
            const quality = link.composite_score || link.quality || 0;
            return quality < QUALITY_THRESHOLD;
        });

        if (poorQualityLinks.length === 0) {
            // Quality recovered - clear any active prompt and tracking state
            if (qualityPromptActive || qualityPromptLinkID) {
                dismissQualityPrompt();
                // Clear tracking state to allow re-arming if condition reoccurs
                qualityPromptLinkID = null;
                qualityPromptStartTime = null;
            }
            return;
        }

        // Check each poor quality link
        for (const link of poorQualityLinks) {
            const linkID = link.link_id || (link.node_mac + ':' + link.peer_mac);

            // Check if this link was already being tracked
            if (qualityPromptLinkID === linkID && qualityPromptStartTime) {
                const elapsed = now - qualityPromptStartTime;
                if (elapsed >= DURATION_MS && !qualityPromptActive) {
                    // Show prompt - duration threshold met
                    showQualityPrompt(link);
                }
            } else if (!qualityPromptActive) {
                // Only start tracking if we're not already showing a prompt for another link
                // This prevents rapid switching between different degraded links
                qualityPromptLinkID = linkID;
                qualityPromptStartTime = now;
                qualityPromptActive = false;
            }
        }
    }

    /**
     * Show a quality degradation prompt card
     */
    function showQualityPrompt(link) {
        // Check if already dismissed today
        const today = new Date().toDateString();
        const dismissKey = `${link.link_id}_${today}`;
        if (dismissedQualityPrompts.has(dismissKey)) {
            return;
        }

        qualityPromptActive = true;

        const linkName = link.name || formatLinkID(link.link_id);
        const qualityPercent = Math.round((link.composite_score || link.quality || 0) * 100);

        // Remove existing prompt if present
        const existing = document.getElementById('quality-prompt-card');
        if (existing) {
            existing.remove();
        }

        const prompt = document.createElement('div');
        prompt.id = 'quality-prompt-card';
        prompt.className = 'quality-prompt-card';
        prompt.innerHTML = `
            <div class="quality-prompt-header">
                <span class="quality-prompt-icon">⚠️</span>
                <span class="quality-prompt-title">Detection quality has dropped</span>
                <button class="quality-prompt-close" onclick="Proactive.dismissQualityPrompt()">&times;</button>
            </div>
            <div class="quality-prompt-content">
                <p>Detection quality has dropped on <strong>${linkName}</strong> to <strong>${qualityPercent}%</strong> reliability.</p>
                <p class="quality-prompt-detail">This link is experiencing degraded performance, which may affect detection accuracy in this area.</p>
            </div>
            <div class="quality-prompt-actions">
                <button class="quality-prompt-btn quality-prompt-diagnose" onclick="Proactive.diagnoseLink('${link.link_id}')">
                    <span class="btn-icon">🔍</span> Diagnose
                </button>
                <button class="quality-prompt-btn quality-prompt-dismiss" onclick="Proactive.dismissQualityPromptForToday()">
                    Dismiss for today
                </button>
            </div>
        `;

        // Add to document
        document.body.appendChild(prompt);

        // Highlight the 3D link line if available
        highlightLinkIn3D(link.link_id);
    }

    /**
     * Dismiss the current quality prompt
     */
    function dismissQualityPrompt() {
        qualityPromptActive = false;
        qualityPromptLinkID = null;
        qualityPromptStartTime = null;

        const prompt = document.getElementById('quality-prompt-card');
        if (prompt) {
            prompt.remove();
        }

        // Remove link highlight
        unhighlightLinkIn3D();
    }

    /**
     * Dismiss the quality prompt for today only
     */
    function dismissQualityPromptForToday() {
        if (qualityPromptLinkID) {
            const today = new Date().toDateString();
            const dismissKey = `${qualityPromptLinkID}_${today}`;
            dismissedQualityPrompts.add(dismissKey);

            // Persist to localStorage
            try {
                const dismissed = Array.from(dismissedQualityPrompts);
                localStorage.setItem('spaxel_dismissed_quality_prompts', dismissed.join(','));
            } catch (e) {
                console.warn('Failed to save dismissed prompts:', e);
            }
        }

        dismissQualityPrompt();
    }

    /**
     * Load dismissed prompts from localStorage
     */
    function loadDismissedPrompts() {
        try {
            const saved = localStorage.getItem('spaxel_dismissed_quality_prompts');
            if (saved) {
                dismissedQualityPrompts = new Set(saved.split(',').filter(id => id));
            }
        } catch (e) {
            console.warn('Failed to load dismissed prompts:', e);
        }
    }

    /**
     * Highlight a link in the 3D view with pulsing amber animation
     */
    function highlightLinkIn3D(linkID) {
        // Store original state before highlighting
        if (window.Viz3D && window.Viz3D.forEachLink) {
            window.Viz3D.forEachLink(function(line, id) {
                if (id === linkID && !line.userData.originalState) {
                    // Store original material state
                    line.userData.originalState = {
                        opacity: line.material.opacity,
                        transparent: line.material.transparent,
                        color: line.material.color ? line.material.color.getHex() : null,
                        emissiveColor: line.material.emissive ? line.material.emissive.getHex() : undefined,
                        emissiveIntensity: line.material.emissive ? line.material.emissiveIntensity : 0
                    };
                }
            });
        }

        // Use Viz3D highlightLink with amber color (0xff9800) and emissive glow
        if (window.Viz3D && window.Viz3D.highlightLink) {
            window.Viz3D.highlightLink(linkID, 0xff9800, 0xff9800, 0.5);
        }

        // Dispatch custom event
        window.dispatchEvent(new CustomEvent('spaxel:highlight-link', {
            detail: { linkID: linkID, highlight: true, pulsing: true }
        }));

        // Start pulsing animation for this link
        startLinkPulsing(linkID);
    }

    /**
     * Remove link highlight in 3D view
     */
    function unhighlightLinkIn3D() {
        const linkID = qualityPromptLinkID;

        if (linkID && window.Viz3D) {
            // Restore the link's original appearance using Viz3D's restore mechanism
            if (window.Viz3D.forEachLink) {
                window.Viz3D.forEachLink(function(line, id) {
                    if (id === linkID && line.userData.originalState) {
                        const orig = line.userData.originalState;
                        line.material.opacity = orig.opacity;
                        line.material.transparent = orig.transparent;
                        if (line.material.color && orig.color !== null) {
                            line.material.color.setHex(orig.color);
                        }
                        if (line.material.emissive) {
                            line.material.emissiveIntensity = orig.emissiveIntensity || 0;
                            if (orig.emissiveColor !== undefined) {
                                line.material.emissive.setHex(orig.emissiveColor);
                            }
                        }
                        line.material.needsUpdate = true;
                    }
                });
            }

            window.dispatchEvent(new CustomEvent('spaxel:highlight-link', {
                detail: { linkID: linkID, highlight: false }
            }));
        }

        // Stop pulsing animation
        stopLinkPulsing();
    }

    /**
     * Start pulsing animation for a highlighted link
     */
    function startLinkPulsing(linkID) {
        // Stop any existing animation
        stopLinkPulsing();

        pulseTime = 0;
        const PULSE_CYCLE = 1.5; // 1.5 second pulse cycle

        function animatePulse(timestamp) {
            if (!qualityPromptActive || qualityPromptLinkID !== linkID) {
                return; // Stop animation
            }

            if (!pulsingAnimationID) {
                pulsingAnimationID = requestAnimationFrame(animatePulse);
                return;
            }

            pulseTime += 0.016; // Approximately 60fps

            // Calculate pulse phase (0 to 1)
            const phase = (pulseTime % PULSE_CYCLE) / PULSE_CYCLE;

            // Opacity oscillates: 0.3 -> 0.8 -> 0.3
            const intensity = 0.3 + 0.5 * (1 - Math.abs(phase - 0.5) * 2);

            // Apply pulsing effect to link
            if (window.Viz3D && window.Viz3D.highlightLink) {
                // Vary the emissive intensity for pulsing glow effect
                const baseIntensity = 0.3;
                const pulseIntensity = baseIntensity + (0.7 * intensity);
                window.Viz3D.highlightLink(linkID, 0xff9800, 0xff9800, pulseIntensity);
            }

            pulsingAnimationID = requestAnimationFrame(animatePulse);
        }

        pulsingAnimationID = requestAnimationFrame(animatePulse);
    }

    /**
     * Stop the link pulsing animation
     */
    function stopLinkPulsing() {
        if (pulsingAnimationID) {
            cancelAnimationFrame(pulsingAnimationID);
            pulsingAnimationID = null;
        }
        pulseTime = 0;
    }

    /**
     * Run diagnostics for a link and show results
     */
    function diagnoseLink(linkID) {
        // Fetch diagnostic results from API
        // Using the correct endpoint: /api/links/{linkID}/diagnostics
        fetch(`/api/links/${encodeURIComponent(linkID)}/diagnostics`)
            .then(res => res.json())
            .then(data => {
                showDiagnosticResults(linkID, data);
            })
            .catch(err => {
                console.error('Failed to fetch diagnostics:', err);
                showDiagnosticError(linkID);
            });
    }

    /**
     * Show diagnostic results in a slide-out panel
     */
    function showDiagnosticResults(linkID, response) {
        // Remove existing panel if present
        const existing = document.getElementById('diagnostic-results-panel');
        if (existing) {
            existing.remove();
        }

        const panel = document.createElement('div');
        panel.id = 'diagnostic-results-panel';
        panel.className = 'diagnostic-results-panel';

        // Handle both old format (diagnosis array) and new format (response with diagnosis + health)
        const diagnosis = response.diagnosis || response[0] || {};
        const health = response.health || diagnosis.health || null;
        const hasDiagnosis = diagnosis.title !== undefined;
        const severity = (diagnosis.severity || 'info').toLowerCase();

        panel.innerHTML = `
            <div class="diagnostic-panel-header">
                <h3>Link Diagnostics</h3>
                <button class="diagnostic-close" onclick="document.getElementById('diagnostic-results-panel').remove()">&times;</button>
            </div>
            <div class="diagnostic-panel-content">
                <div class="diagnostic-link-info">
                    <span class="diagnostic-link-label">Link:</span>
                    <span class="diagnostic-link-id">${formatLinkID(linkID)}</span>
                </div>

                ${hasDiagnosis ? `
                    <div class="diagnostic-result diagnostic-severity-${severity}">
                        <div class="diagnostic-result-title">${diagnosis.title}</div>
                        <div class="diagnostic-result-detail">${diagnosis.detail}</div>
                        ${diagnosis.advice ? `<div class="diagnostic-result-advice"><strong>What to do:</strong> ${diagnosis.advice}</div>` : ''}
                        ${diagnosis.confidence !== undefined ? `<div class="diagnostic-confidence">Confidence: ${Math.round(diagnosis.confidence * 100)}%</div>` : ''}
                    </div>
                ` : `
                    <div class="diagnostic-result diagnostic-severity-info">
                        <div class="diagnostic-result-title">No issues detected</div>
                        <div class="diagnostic-result-detail">The link health metrics appear normal. The quality drop may be temporary.</div>
                    </div>
                `}

                ${health ? `
                    <div class="diagnostic-health-metrics">
                        <h4>Current Health Metrics</h4>
                        ${renderHealthMetrics(health)}
                    </div>
                ` : ''}

                ${response.repositioning ? `
                    <div class="diagnostic-repositioning">
                        <h4>Suggested Repositioning</h4>
                        <p>Move <strong>${response.repositioning.node_mac}</strong> to:</p>
                        <div class="repositioning-coords">
                            X: ${response.repositioning.position.x.toFixed(1)}m,
                            Y: ${response.repositioning.position.y.toFixed(1)}m,
                            Z: ${response.repositioning.position.z.toFixed(1)}m
                        </div>
                    </div>
                ` : ''}
            </div>
        `;

        document.body.appendChild(panel);
    }

    /**
     * Show diagnostic error message
     */
    function showDiagnosticError(linkID) {
        const panel = document.createElement('div');
        panel.id = 'diagnostic-results-panel';
        panel.className = 'diagnostic-results-panel';
        panel.innerHTML = `
            <div class="diagnostic-panel-header">
                <h3>Link Diagnostics</h3>
                <button class="diagnostic-close" onclick="this.closest('#diagnostic-results-panel').remove()">&times;</button>
            </div>
            <div class="diagnostic-panel-content">
                <p class="diagnostic-error">Unable to fetch diagnostic data. Please try again.</p>
            </div>
        `;
        document.body.appendChild(panel);
    }

    /**
     * Render health metrics
     */
    function renderHealthMetrics(health) {
        if (!health) {
            return '';
        }

        // Handle both snapshot format and report format
        const snr = health.snr !== undefined ? health.snr : (health.SNR || 'N/A');
        const phaseStability = health.phase_stability !== undefined ? health.phase_stability : (health.PhaseStability || 'N/A');
        const packetRate = health.packet_rate !== undefined ? health.packet_rate : (health.PacketRate || 'N/A');
        const driftRate = health.drift_rate !== undefined ? health.drift_rate : (health.DriftRate || 'N/A');
        const compositeScore = health.composite_score !== undefined ? health.composite_score : (health.CompositeScore || 'N/A');

        return `
            <div class="health-metric">
                <span class="metric-label">Packet Rate:</span>
                <span class="metric-value">${typeof packetRate === 'number' ? packetRate.toFixed(1) : packetRate} Hz</span>
            </div>
            <div class="health-metric">
                <span class="metric-label">SNR:</span>
                <span class="metric-value">${typeof snr === 'number' ? snr.toFixed(2) : snr}</span>
            </div>
            <div class="health-metric">
                <span class="metric-label">Phase Stability:</span>
                <span class="metric-value">${typeof phaseStability === 'number' ? phaseStability.toFixed(2) : phaseStability}</span>
            </div>
            <div class="health-metric">
                <span class="metric-label">Drift Rate:</span>
                <span class="metric-value">${typeof driftRate === 'number' ? driftRate.toFixed(4) : driftRate}</span>
            </div>
            ${typeof compositeScore === 'number' ? `
            <div class="health-metric">
                <span class="metric-label">Composite Score:</span>
                <span class="metric-value">${Math.round(compositeScore * 100)}%</span>
            </div>
            ` : ''}
        `;
    }

    // ===== Repeated-Setting Change Detection =====

    /**
     * Track a settings change for repeated-change detection
     */
    function trackSettingChange(settingKey, settingValue) {
        if (!QUALIFYING_SETTINGS.has(settingKey)) {
            return; // Not a qualifying setting
        }

        const now = Date.now();
        const windowMs = 24 * 60 * 60 * 1000; // 24 hours

        // Initialize history for this setting if needed
        if (!settingChangeHistory[settingKey]) {
            settingChangeHistory[settingKey] = [];
        }

        // Add new change
        settingChangeHistory[settingKey].push({
            timestamp: now,
            value: settingValue
        });

        // Remove old changes outside the 24h window
        const cutoff = now - windowMs;
        settingChangeHistory[settingKey] = settingChangeHistory[settingKey].filter(
            change => change.timestamp > cutoff
        );

        // Save to localStorage
        saveSettingHistory();

        // Check if we should show a hint
        checkSettingChangeHint(settingKey);
    }

    /**
     * Check if a setting change hint should be shown
     */
    function checkSettingChangeHint(settingKey) {
        // Check if hint was already shown for this setting
        if (settingChangeHintShown[settingKey]) {
            const lastShown = settingChangeHintShown[settingKey];
            const cooldownMs = 24 * 60 * 60 * 1000; // 24 hour cooldown
            if (Date.now() - lastShown < cooldownMs) {
                return; // Still in cooldown
            }
        }

        const changes = settingChangeHistory[settingKey] || [];
        if (changes.length >= 3) {
            // Show the hint
            showSettingChangeHint(settingKey);
            settingChangeHintShown[settingKey] = Date.now();
            saveSettingHintShown();
        }
    }

    /**
     * Show a hint for repeated setting changes
     */
    function showSettingChangeHint(settingKey) {
        const settingName = formatSettingName(settingKey);

        // Remove existing hint if present
        const existing = document.getElementById('setting-change-hint');
        if (existing) {
            existing.remove();
        }

        const hint = document.createElement('div');
        hint.id = 'setting-change-hint';
        hint.className = 'setting-change-hint';
        hint.innerHTML = `
            <div class="hint-header">
                <span class="hint-icon">💡</span>
                <span class="hint-title">Fine-tuning assistance</span>
                <button class="hint-close" onclick="Proactive.dismissSettingHint()">&times;</button>
            </div>
            <div class="hint-content">
                <p>Looks like you're fine-tuning the <strong>${settingName}</strong>. Would you like help finding the right value for your space?</p>
            </div>
            <div class="hint-actions">
                <button class="hint-btn hint-btn-primary" onclick="Proactive.startCalibrationFlow('${settingKey}')">
                    Help me tune this
                </button>
                <button class="hint-btn hint-btn-secondary" onclick="Proactive.dismissSettingHint()">
                    No thanks
                </button>
            </div>
        `;

        document.body.appendChild(hint);
    }

    /**
     * Dismiss the setting change hint
     */
    function dismissSettingHint() {
        const hint = document.getElementById('setting-change-hint');
        if (hint) {
            hint.remove();
        }
    }

    /**
     * Start the guided calibration flow for a setting
     */
    function startCalibrationFlow(settingKey) {
        // Remove the hint
        dismissSettingHint();

        // Show the calibration flow modal
        showCalibrationFlow(settingKey);
    }

    /**
     * Show the guided calibration flow modal
     */
    function showCalibrationFlow(settingKey) {
        // Remove existing calibration modal if present
        const existing = document.getElementById('calibration-flow-modal');
        if (existing) {
            existing.remove();
        }

        const settingName = formatSettingName(settingKey);

        const modal = document.createElement('div');
        modal.id = 'calibration-flow-modal';
        modal.className = 'calibration-flow-modal';
        modal.innerHTML = `
            <div class="calibration-flow-backdrop"></div>
            <div class="calibration-flow-container">
                <div class="calibration-flow-header">
                    <h2>Guided Calibration: ${settingName}</h2>
                    <button class="calibration-close" onclick="Proactive.closeCalibrationFlow()">&times;</button>
                </div>
                <div class="calibration-flow-content" id="calibration-flow-content">
                    <!-- Steps will be rendered here -->
                </div>
            </div>
        `;

        document.body.appendChild(modal);

        // Start with the introduction step
        renderCalibrationStep(settingKey, 'intro');
    }

    /**
     * Render a calibration step
     */
    function renderCalibrationStep(settingKey, step, data = {}) {
        const content = document.getElementById('calibration-flow-content');
        if (!content) return;

        const steps = {
            intro: `
                <div class="calibration-step calibration-step-intro">
                    <div class="calibration-icon">🎯</div>
                    <h3>Let's find the optimal value together</h3>
                    <p>I'll guide you through two quick tests to analyze your space and suggest the best ${formatSettingName(settingKey)} value.</p>
                    <div class="calibration-steps-overview">
                        <div class="overview-step">
                            <span class="overview-number">1</span>
                            <span class="overview-text">Walk around your room to test for false positives</span>
                        </div>
                        <div class="overview-step">
                            <span class="overview-number">2</span>
                            <span class="overview-text">Sit still to test for missed motion detection</span>
                        </div>
                        <div class="overview-step">
                            <span class="overview-number">3</span>
                            <span class="overview-text">I'll suggest an optimal value based on your space</span>
                        </div>
                    </div>
                    <button class="calibration-btn calibration-btn-primary" onclick="Proactive.startCalibrationStep('${settingKey}', 'false-positive-test')">
                        Start Calibration
                    </button>
                </div>
            `,

            'false-positive-test': `
                <div class="calibration-step calibration-step-active">
                    <div class="calibration-progress">
                        <div class="progress-bar">
                            <div class="progress-fill" id="calibration-progress-fill" style="width: 0%"></div>
                        </div>
                        <span class="progress-text">Step 1 of 2</span>
                    </div>
                    <div class="calibration-icon">🚶</div>
                    <h3>Walk around your room</h3>
                    <p id="calibration-instruction">For the next <strong>15 seconds</strong>, walk around your room normally. This helps me understand your space's baseline signal patterns.</p>
                    <div class="calibration-timer" id="calibration-timer">15</div>
                    <div class="calibration-status" id="calibration-status">Getting ready...</div>
                </div>
            `,

            'missed-motion-test': `
                <div class="calibration-step calibration-step-active">
                    <div class="calibration-progress">
                        <div class="progress-bar">
                            <div class="progress-fill" id="calibration-progress-fill" style="width: 50%"></div>
                        </div>
                        <span class="progress-text">Step 2 of 2</span>
                    </div>
                    <div class="calibration-icon">🧘</div>
                    <h3>Sit perfectly still</h3>
                    <p id="calibration-instruction">For the next <strong>10 seconds</strong>, sit or stand perfectly still. This helps me detect motion sensitivity issues.</p>
                    <div class="calibration-timer" id="calibration-timer">10</div>
                    <div class="calibration-status" id="calibration-status">Getting ready...</div>
                </div>
            `,

            analyzing: `
                <div class="calibration-step">
                    <div class="calibration-progress">
                        <div class="progress-bar">
                            <div class="progress-fill" style="width: 100%"></div>
                        </div>
                    </div>
                    <div class="calibration-spinner"></div>
                    <h3>Analyzing your space...</h3>
                    <p>Collecting diurnal baseline data and link health metrics to calculate the optimal value.</p>
                </div>
            `,

            suggestion: `
                <div class="calibration-step">
                    <div class="calibration-icon">✨</div>
                    <h3>Here's my recommendation</h3>
                    <div class="calibration-suggestion">
                        <div class="suggestion-value">
                            <span class="suggestion-label">Suggested ${formatSettingName(settingKey)}:</span>
                            <span class="suggestion-number" id="suggested-value">--</span>
                        </div>
                        <div class="suggestion-reason" id="suggestion-reason">
                            Calculating...
                        </div>
                    </div>
                    <div class="calibration-metrics" id="calibration-metrics">
                        <!-- Metrics will be populated -->
                    </div>
                    <div class="calibration-actions">
                        <button class="calibration-btn calibration-btn-primary" id="apply-suggestion-btn" onclick="Proactive.applySuggestedValue('${settingKey}')" disabled>
                            Apply Suggested Value
                        </button>
                        <button class="calibration-btn calibration-btn-secondary" onclick="Proactive.closeCalibrationFlow()">
                            Cancel
                        </button>
                    </div>
                </div>
            `
        };

        content.innerHTML = steps[step] || steps.intro;

        // Execute step-specific logic
        if (step === 'false-positive-test') {
            runCalibrationTest(settingKey, 'false-positive', 15);
        } else if (step === 'missed-motion-test') {
            runCalibrationTest(settingKey, 'missed-motion', 10);
        } else if (step === 'analyzing') {
            analyzeAndSuggest(settingKey);
        }
    }

    /**
     * Run a calibration test with countdown timer
     */
    function runCalibrationTest(settingKey, testType, duration) {
        const timerEl = document.getElementById('calibration-timer');
        const statusEl = document.getElementById('calibration-status');
        const progressFill = document.getElementById('calibration-progress-fill');
        const startTime = Date.now();

        // Update progress bar fill
        if (progressFill) {
            const targetWidth = testType === 'false-positive' ? '0%' : '50%';
            progressFill.style.width = targetWidth;
        }

        // Start collecting test data
        collectTestData(settingKey, testType);

        const interval = setInterval(() => {
            const elapsed = (Date.now() - startTime) / 1000;
            const remaining = Math.max(0, Math.ceil(duration - elapsed));

            if (timerEl) {
                timerEl.textContent = remaining;
            }

            // Update progress
            if (progressFill) {
                const baseWidth = testType === 'false-positive' ? 0 : 50;
                const progress = (elapsed / duration) * 50;
                progressFill.style.width = (baseWidth + progress) + '%';
            }

            if (elapsed >= 1 && elapsed < 3) {
                if (statusEl) statusEl.textContent = 'Recording baseline...';
            } else if (elapsed >= 3 && elapsed < duration - 2) {
                if (statusEl) statusEl.textContent = testType === 'false-positive' ? 'Walking... keep moving!' : 'Holding still...';
            } else if (elapsed >= duration - 2) {
                if (statusEl) statusEl.textContent = 'Finishing up...';
            }

            if (elapsed >= duration) {
                clearInterval(interval);
                finalizeTestData(settingKey, testType);

                // Move to next step
                if (testType === 'false-positive') {
                    renderCalibrationStep(settingKey, 'missed-motion-test');
                } else {
                    renderCalibrationStep(settingKey, 'analyzing');
                }
            }
        }, 100);
    }

    /**
     * Collect test data from the system
     */
    function collectTestData(settingKey, testType) {
        // Store test metadata for later analysis
        if (!window.calibrationTestData) {
            window.calibrationTestData = {};
        }
        window.calibrationTestData[testType] = {
            startTime: Date.now(),
            settingKey: settingKey
        };

        // Notify the system to start recording for this test
        window.dispatchEvent(new CustomEvent('spaxel:calibration-start', {
            detail: { testType: testType, settingKey: settingKey }
        }));
    }

    /**
     * Finalize test data collection
     */
    function finalizeTestData(settingKey, testType) {
        if (!window.calibrationTestData || !window.calibrationTestData[testType]) {
            return;
        }

        window.calibrationTestData[testType].endTime = Date.now();

        // Notify the system to stop recording
        window.dispatchEvent(new CustomEvent('spaxel:calibration-end', {
            detail: { testType: testType, settingKey: settingKey }
        }));
    }

    /**
     * Analyze test data and suggest optimal value
     */
    function analyzeAndSuggest(settingKey) {
        // Fetch diurnal baseline status and link health data
        Promise.all([
            fetch('/api/diurnal/status').then(r => r.json()).catch(() => null),
            fetch('/api/links').then(r => r.json()).catch(() => null)
        ]).then(([diurnalData, linksData]) => {
            const suggestion = calculateSuggestedValue(settingKey, diurnalData, linksData);
            renderSuggestion(settingKey, suggestion);
        }).catch(err => {
            console.error('Failed to fetch calibration data:', err);
            // Show fallback suggestion
            renderSuggestion(settingKey, getFallbackSuggestion(settingKey));
        });
    }

    /**
     * Calculate suggested value based on system data
     */
    function calculateSuggestedValue(settingKey, diurnalData, linksData) {
        const suggestion = {
            value: null,
            reason: '',
            metrics: []
        };

        const now = new Date();
        const currentHour = now.getHours();

        // Get average health score across all links (higher is better, 0-1 range)
        let avgHealthScore = 0.5; // default fallback
        let activeLinkCount = 0;

        if (Array.isArray(linksData)) {
            const healthScores = linksData.map(l => l.health_score || 0.5).filter(s => s >= 0);
            if (healthScores.length > 0) {
                avgHealthScore = healthScores.reduce((a, b) => a + b, 0) / healthScores.length;
                activeLinkCount = linksData.length;
            }
        }

        // Get diurnal baseline readiness/learning status
        let diurnalReady = false;
        let learningProgress = 0;

        if (Array.isArray(diurnalData) && diurnalData.length > 0) {
            // diurnalData is an array of DiurnalLearningStatus objects
            const readyCount = diurnalData.filter(d => d.is_ready).length;
            diurnalReady = readyCount > (diurnalData.length / 2); // Majority ready

            // Average learning progress (backend returns 'progress' field, 0-100 range)
            const progressValues = diurnalData.map(d => (d.progress || 0) / 100).filter(p => p >= 0);
            if (progressValues.length > 0) {
                learningProgress = progressValues.reduce((a, b) => a + b, 0) / progressValues.length;
            }
        }

        // Calculate suggestion based on setting type
        // Note: avgHealthScore is 0-1 where higher is better (unlike raw SNR)
        switch (settingKey) {
            case 'delta_rms_threshold':
                // Lower threshold for high health scores, higher for low health scores
                // Range: 0.01 - 0.10
                if (avgHealthScore > 0.8) {
                    suggestion.value = 0.02;
                    suggestion.reason = 'Your space has excellent link health. A lower threshold will detect subtle movements while minimizing false positives.';
                } else if (avgHealthScore > 0.6) {
                    suggestion.value = 0.03;
                    suggestion.reason = 'Your space has good link health. This threshold balances sensitivity with noise immunity.';
                } else {
                    suggestion.value = 0.05;
                    suggestion.reason = 'Your space has lower link health. A higher threshold reduces false positives from environmental noise.';
                }
                // Adjust based on diurnal readiness
                if (diurnalReady && learningProgress < 0.5) {
                    suggestion.value *= 1.3; // Still learning needs higher threshold
                    suggestion.reason += ' Adjusted up since your baselines are still learning.';
                }
                break;

            case 'breathing_sensitivity':
                // Range: 0.001 - 0.02
                if (avgHealthScore > 0.75) {
                    suggestion.value = 0.005;
                    suggestion.reason = 'High link health enables fine-grained breathing detection.';
                } else if (avgHealthScore > 0.55) {
                    suggestion.value = 0.008;
                    suggestion.reason = 'Good link health for reliable breathing detection.';
                } else {
                    suggestion.value = 0.015;
                    suggestion.reason = 'Lower link health requires higher sensitivity for breathing detection.';
                }
                break;

            case 'tau_s':
                // Baseline time constant: 5 - 120 seconds
                if (diurnalReady && learningProgress > 0.8) {
                    suggestion.value = 30;
                    suggestion.reason = 'Your diurnal baselines are well-calibrated. A 30-second time constant adapts quickly to real changes.';
                } else if (diurnalReady || learningProgress > 0.5) {
                    suggestion.value = 60;
                    suggestion.reason = 'Moderate baseline stability. A 60-second time constant provides stable adaptation.';
                } else {
                    suggestion.value = 120;
                    suggestion.reason = 'Your environment is still learning baselines. A longer time constant (120s) provides more stable detection.';
                }
                break;

            case 'fresnel_decay':
                // Zone decay rate: 1.0 - 4.0
                if (avgHealthScore > 0.75) {
                    suggestion.value = 2.5;
                    suggestion.reason = 'Strong link health supports tighter zone focus for better localization accuracy.';
                } else if (avgHealthScore > 0.55) {
                    suggestion.value = 2.0;
                    suggestion.reason = 'Balanced decay rate for your link health.';
                } else {
                    suggestion.value = 1.5;
                    suggestion.reason = 'Lower link health benefits from broader zone contribution.';
                }
                break;

            case 'n_subcarriers':
                // Subcarrier count: 8 - 16
                if (avgHealthScore > 0.75) {
                    suggestion.value = 16;
                    suggestion.reason = 'Excellent link health supports using all 16 subcarriers for maximum detail.';
                } else if (avgHealthScore > 0.55) {
                    suggestion.value = 12;
                    suggestion.reason = 'Good link health. 12 subcarriers balance detail with noise immunity.';
                } else {
                    suggestion.value = 8;
                    suggestion.reason = 'Lower link health. Fewer subcarriers reduce noise impact.';
                }
                break;

            default:
                suggestion.value = 0.03;
                suggestion.reason = 'Default suggestion based on average system performance.';
        }

        // Build metrics display
        const healthPercent = Math.round(avgHealthScore * 100);
        suggestion.metrics = [
            { label: 'Link Health', value: healthPercent + '%' },
            { label: 'Diurnal Ready', value: diurnalReady ? 'Yes' : 'Learning' },
            { label: 'Active Links', value: activeLinkCount.toString() }
        ];

        return suggestion;
    }

    /**
     * Get fallback suggestion when API data is unavailable
     */
    function getFallbackSuggestion(settingKey) {
        const defaults = {
            'delta_rms_threshold': { value: 0.03, reason: 'Default value for typical home environments.' },
            'breathing_sensitivity': { value: 0.008, reason: 'Default sensitivity for reliable breathing detection.' },
            'tau_s': { value: 60, reason: 'Default 60-second time constant for stable adaptation.' },
            'fresnel_decay': { value: 2.0, reason: 'Default inverse-square decay for balanced localization.' },
            'n_subcarriers': { value: 12, reason: 'Default 12 subcarriers for balanced performance.' }
        };
        return defaults[settingKey] || { value: 0.03, reason: 'Default value.' };
    }

    /**
     * Render the suggestion step with calculated values
     */
    function renderSuggestion(settingKey, suggestion) {
        const content = document.getElementById('calibration-flow-content');
        if (!content) return;

        const formattedValue = formatSuggestedValue(settingKey, suggestion.value);

        content.innerHTML = `
            <div class="calibration-step">
                <div class="calibration-icon">✨</div>
                <h3>Here's my recommendation</h3>
                <div class="calibration-suggestion">
                    <div class="suggestion-value">
                        <span class="suggestion-label">Suggested ${formatSettingName(settingKey)}:</span>
                        <span class="suggestion-number">${formattedValue}</span>
                    </div>
                    <div class="suggestion-reason">${suggestion.reason}</div>
                </div>
                <div class="calibration-metrics">
                    <h4>Based on your space:</h4>
                    ${suggestion.metrics.map(m => `
                        <div class="calibration-metric">
                            <span class="metric-label">${m.label}:</span>
                            <span class="metric-value">${m.value}</span>
                        </div>
                    `).join('')}
                </div>
                <div class="calibration-actions">
                    <button class="calibration-btn calibration-btn-primary" onclick="Proactive.applySuggestedValue('${settingKey}', ${suggestion.value})">
                        Apply Suggested Value
                    </button>
                    <button class="calibration-btn calibration-btn-secondary" onclick="Proactive.closeCalibrationFlow()">
                        Cancel
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * Format a suggested value for display
     */
    function formatSuggestedValue(settingKey, value) {
        if (settingKey === 'tau_s') {
            return value + ' seconds';
        } else if (settingKey === 'n_subcarriers') {
            return value + ' subcarriers';
        } else if (settingKey === 'fresnel_decay') {
            return value.toFixed(1);
        }
        return value.toFixed(3);
    }

    /**
     * Apply the suggested value to the setting
     */
    function applySuggestedValue(settingKey, value) {
        // Show loading state
        const applyBtn = document.querySelector('.calibration-btn-primary');
        if (applyBtn) {
            applyBtn.disabled = true;
            applyBtn.textContent = 'Applying...';
        }

        // Build settings payload
        const settings = {};
        settings[settingKey] = value;

        // Send to API
        fetch('/api/settings', {
            method: 'PATCH',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(settings)
        })
        .then(res => res.json())
        .then(data => {
            // Show success message
            showCalibrationSuccess(settingKey, value);

            // Notify other components of settings change
            window.dispatchEvent(new CustomEvent('spaxel:settings-changed', {
                detail: { key: settingKey, value: value }
            }));
        })
        .catch(err => {
            console.error('Failed to apply setting:', err);
            showCalibrationError();
        });
    }

    /**
     * Show calibration success message
     */
    function showCalibrationSuccess(settingKey, value) {
        const content = document.getElementById('calibration-flow-content');
        if (!content) return;

        const formattedValue = formatSuggestedValue(settingKey, value);

        content.innerHTML = `
            <div class="calibration-step">
                <div class="calibration-icon calibration-success">✓</div>
                <h3>Value applied!</h3>
                <p>Your <strong>${formatSettingName(settingKey)}</strong> has been set to <strong>${formattedValue}</strong>.</p>
                <p class="calibration-note">The system will now use this new value for detection. If you experience issues, you can adjust it again in Settings.</p>
                <div class="calibration-actions">
                    <button class="calibration-btn calibration-btn-primary" onclick="Proactive.closeCalibrationFlow()">
                        Done
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * Show calibration error message
     */
    function showCalibrationError() {
        const content = document.getElementById('calibration-flow-content');
        if (!content) return;

        content.innerHTML = `
            <div class="calibration-step">
                <div class="calibration-icon calibration-error">⚠</div>
                <h3>Couldn't apply the value</h3>
                <p>There was a problem saving the setting. Please try again or adjust it manually in Settings.</p>
                <div class="calibration-actions">
                    <button class="calibration-btn calibration-btn-primary" onclick="Proactive.closeCalibrationFlow()">
                        Close
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * Close the calibration flow modal
     */
    function closeCalibrationFlow() {
        const modal = document.getElementById('calibration-flow-modal');
        if (modal) {
            modal.classList.add('calibration-closing');
            setTimeout(() => modal.remove(), 300);
        }
        // Clean up test data
        if (window.calibrationTestData) {
            delete window.calibrationTestData;
        }
    }

    /**
     * Save setting change history to localStorage
     */
    function saveSettingHistory() {
        try {
            localStorage.setItem('spaxel_setting_changes', JSON.stringify(settingChangeHistory));
        } catch (e) {
            console.warn('Failed to save setting history:', e);
        }
    }

    /**
     * Save setting hint shown timestamps
     */
    function saveSettingHintShown() {
        try {
            localStorage.setItem('spaxel_setting_hints_shown', JSON.stringify(settingChangeHintShown));
        } catch (e) {
            console.warn('Failed to save hint shown state:', e);
        }
    }

    /**
     * Format a setting key for display
     */
    function formatSettingName(key) {
        const names = {
            'delta_rms_threshold': 'Motion Threshold',
            'breathing_sensitivity': 'Breathing Sensitivity',
            'tau_s': 'Baseline Time Constant',
            'fresnel_decay': 'Fresnel Weight Decay',
            'n_subcarriers': 'Subcarrier Count'
        };
        return names[key] || key;
    }

    // ===== Post-Feedback Explanations =====

    /**
     * Show explanation after false positive feedback
     */
    function showFeedbackExplanation(eventData) {
        if (!eventData || !eventData.explainability) {
            return;
        }

        const explanation = eventData.explainability;
        const contributingLinks = explanation.contributing_links || [];
        const allLinks = explanation.all_links || [];

        // Find the primary contributing link
        const primaryLink = contributingLinks.length > 0 ? contributingLinks[0] : null;

        let explanationText = '';

        if (primaryLink) {
            const linkName = formatLinkID(primaryLink.link_id);
            const deltaRMS = primaryLink.delta_rms?.toFixed(4) || 'N/A';
            const threshold = 0.02; // Standard threshold
            const ratio = (primaryLink.delta_rms / threshold).toFixed(1);

            explanationText = `The system detected motion here because: <strong>${linkName}</strong>'s signal (deltaRMS: ${deltaRMS}) exceeded the motion threshold by <strong>${ratio}x</strong>.`;

            // Add root cause from diagnostic if available
            if (primaryLink.diagnosis) {
                const diagnosis = primaryLink.diagnosis;
                explanationText += `<br><br><strong>Possible cause:</strong> ${diagnosis.detail}`;

                if (diagnosis.advice) {
                    explanationText += `<br><strong>What to do:</strong> ${diagnosis.advice}`;
                }
            } else {
                explanationText += `<br><br><strong>Possible cause:</strong> Ambient RF interference or environmental changes. We've noted this and will apply corrections.`;
            }
        } else {
            explanationText = 'The system detected motion based on signal patterns across multiple links. We\'ve noted this feedback to improve accuracy.';
        }

        // Create explanation element
        const explanationDiv = document.createElement('div');
        explanationDiv.className = 'feedback-explanation';
        explanationDiv.innerHTML = `
            <div class="explanation-header">
                <span class="explanation-title">Why did this happen?</span>
            </div>
            <div class="explanation-content">
                <p>${explanationText}</p>
                ${contributingLinks.length > 1 ? `<p class="explanation-additional-links">Contributing links: ${contributingLinks.map(l => formatLinkID(l.link_id)).join(', ')}</p>` : ''}
            </div>
        `;

        return explanationDiv;
    }

    /**
     * Fetch diagnostic info for a link at a specific time
     */
    function fetchDiagnosticForLink(linkID, timestamp) {
        // Using the correct endpoint: /api/diagnostics/link/{linkID}?timestamp={ms}
        return fetch(`/api/diagnostics/link/${encodeURIComponent(linkID)}?timestamp=${timestamp}`)
            .then(res => {
                if (!res.ok) {
                    throw new Error(`HTTP ${res.status}: ${res.statusText}`);
                }
                return res.json();
            })
            .catch(err => {
                console.error('Failed to fetch diagnostic:', err);
                return null;
            });
    }

    // ===== Helpers =====

    function formatLinkID(linkID) {
        if (!linkID) {
            return 'Unknown Link';
        }
        // Format MAC address pairs nicely
        const parts = linkID.split(':');
        if (parts.length === 2) {
            const mac1 = parts[0].substring(0, 8); // First 8 chars of first MAC
            const mac2 = parts[1].substring(0, 8); // First 8 chars of second MAC
            return `${mac1} → ${mac2}`;
        }
        return linkID.substring(0, 16) + '...';
    }

    // ===== Public API =====
    window.Proactive = {
        // Quality monitoring
        monitorLinkQuality: monitorLinkQuality,
        dismissQualityPrompt: dismissQualityPrompt,
        dismissQualityPromptForToday: dismissQualityPromptForToday,
        diagnoseLink: diagnoseLink,

        // Setting change tracking
        trackSettingChange: trackSettingChange,
        dismissSettingHint: dismissSettingHint,
        startCalibrationFlow: startCalibrationFlow,

        // Calibration flow
        startCalibrationStep: renderCalibrationStep,
        closeCalibrationFlow: closeCalibrationFlow,
        applySuggestedValue: applySuggestedValue,

        // Feedback explanations
        showFeedbackExplanation: showFeedbackExplanation,
        fetchDiagnosticForLink: fetchDiagnosticForLink,

        // Initialization
        init: function() {
            loadDismissedPrompts();

            // Listen for WebSocket messages with link health data
            window.addEventListener('spaxel:update', (e) => {
                if (e.detail && e.detail.links) {
                    monitorLinkQuality(e.detail.links);
                }
            });

            // Listen for feedback submissions
            window.addEventListener('spaxel:feedback', (e) => {
                if (e.detail && e.detail.type === 'incorrect') {
                    const explanation = this.showFeedbackExplanation(e.detail);
                    if (explanation) {
                        // Show explanation in feedback confirmation
                        const feedbackPanel = document.getElementById('feedback-panel');
                        if (feedbackPanel) {
                            const contentArea = feedbackPanel.querySelector('.feedback-content');
                            if (contentArea) {
                                contentArea.appendChild(explanation);
                            }
                        }
                    }
                }
            });

            // Check for server-side repeated edit hint flag
            checkServerEditHint();
        }
    };

    /**
     * Check for server-side repeated edit hint flag
     */
    function checkServerEditHint() {
        fetch('/api/settings')
            .then(res => res.json())
            .then(settings => {
                if (settings.repeated_edit_hint) {
                    // Show the hint - the server detected repeated edits
                    showSettingChangeHint('detected_by_server');
                }
            })
            .catch(err => {
                // Ignore errors - this is a nice-to-have feature
            });
    }

    // Initialize on load
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => window.Proactive.init());
    } else {
        window.Proactive.init();
    }

    // Add styles
    const style = document.createElement('style');
    style.id = 'proactive-styles';
    style.textContent = `
        .quality-prompt-card {
            position: fixed;
            bottom: 100px;
            right: 20px;
            width: 380px;
            max-width: calc(100vw - 40px);
            background: rgba(255, 152, 0, 0.15);
            border: 1px solid rgba(255, 152, 0, 0.5);
            border-radius: 12px;
            box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);
            z-index: 200;
            animation: qualityPromptSlideIn 0.3s ease-out;
        }

        @keyframes qualityPromptSlideIn {
            from {
                opacity: 0;
                transform: translateY(20px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }

        .quality-prompt-header {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 14px 16px;
            border-bottom: 1px solid rgba(255, 152, 0, 0.3);
        }

        .quality-prompt-icon {
            font-size: 20px;
        }

        .quality-prompt-title {
            flex: 1;
            font-weight: 600;
            font-size: 14px;
            color: #ff9800;
        }

        .quality-prompt-close {
            background: none;
            border: none;
            color: #888;
            font-size: 20px;
            cursor: pointer;
            padding: 4px;
        }

        .quality-prompt-close:hover {
            color: #fff;
        }

        .quality-prompt-content {
            padding: 14px 16px;
        }

        .quality-prompt-content p {
            margin: 0 0 8px 0;
            font-size: 13px;
            line-height: 1.4;
            color: #ddd;
        }

        .quality-prompt-detail {
            color: #bbb;
            font-size: 12px;
        }

        .quality-prompt-actions {
            display: flex;
            gap: 8px;
            padding: 12px 16px;
            border-top: 1px solid rgba(255, 152, 0, 0.3);
        }

        .quality-prompt-btn {
            flex: 1;
            padding: 8px 14px;
            border-radius: 6px;
            font-size: 13px;
            cursor: pointer;
            border: none;
            transition: background 0.2s;
        }

        .quality-prompt-diagnose {
            background: #ff9800;
            color: #1a1a2e;
            font-weight: 500;
        }

        .quality-prompt-diagnose:hover {
            background: #fb8c00;
        }

        .quality-prompt-dismiss {
            background: rgba(255, 255, 255, 0.1);
            color: #ccc;
        }

        .quality-prompt-dismiss:hover {
            background: rgba(255, 255, 255, 0.15);
        }

        .diagnostic-results-panel {
            position: fixed;
            top: 0;
            right: 0;
            bottom: 0;
            width: 400px;
            max-width: calc(100vw - 40px);
            background: #1e1e3a;
            box-shadow: -4px 0 20px rgba(0, 0, 0, 0.5);
            z-index: 300;
            overflow-y: auto;
            animation: diagnosticSlideIn 0.3s ease-out;
        }

        @keyframes diagnosticSlideIn {
            from {
                transform: translateX(100%);
            }
            to {
                transform: translateX(0);
            }
        }

        .diagnostic-panel-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 16px 20px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }

        .diagnostic-panel-header h3 {
            margin: 0;
            font-size: 16px;
            color: #eee;
        }

        .diagnostic-close {
            background: none;
            border: none;
            color: #888;
            font-size: 24px;
            cursor: pointer;
            padding: 4px;
        }

        .diagnostic-close:hover {
            color: #fff;
        }

        .diagnostic-panel-content {
            padding: 20px;
        }

        .diagnostic-link-info {
            margin-bottom: 16px;
            padding: 12px;
            background: rgba(255, 255, 255, 0.05);
            border-radius: 6px;
        }

        .diagnostic-link-label {
            font-size: 11px;
            color: #888;
            text-transform: uppercase;
        }

        .diagnostic-link-id {
            margin-left: 8px;
            font-size: 14px;
            color: #4fc3f7;
            font-family: monospace;
        }

        .diagnostic-result {
            padding: 16px;
            border-radius: 8px;
            margin-bottom: 16px;
        }

        .diagnostic-severity-actionable {
            background: rgba(244, 67, 54, 0.1);
            border: 1px solid rgba(244, 67, 54, 0.3);
        }

        .diagnostic-severity-warning {
            background: rgba(255, 152, 0, 0.1);
            border: 1px solid rgba(255, 152, 0, 0.3);
        }

        .diagnostic-severity-info {
            background: rgba(76, 175, 80, 0.1);
            border: 1px solid rgba(76, 175, 80, 0.3);
        }

        .diagnostic-result-title {
            font-weight: 600;
            font-size: 14px;
            color: #eee;
            margin-bottom: 8px;
        }

        .diagnostic-result-detail {
            font-size: 13px;
            color: #bbb;
            line-height: 1.4;
            margin-bottom: 8px;
        }

        .diagnostic-result-advice {
            font-size: 12px;
            color: #aaa;
            padding-top: 8px;
            border-top: 1px solid rgba(255, 255, 255, 0.1);
        }

        .diagnostic-confidence {
            margin-top: 8px;
            padding-top: 8px;
            border-top: 1px solid rgba(255, 255, 255, 0.1);
            font-size: 11px;
            color: #888;
        }

        .diagnostic-health-metrics {
            margin-top: 20px;
        }

        .diagnostic-repositioning {
            margin-top: 16px;
            padding: 12px;
            background: rgba(79, 195, 247, 0.1);
            border: 1px solid rgba(79, 195, 247, 0.3);
            border-radius: 6px;
        }

        .diagnostic-repositioning h4 {
            margin: 0 0 8px 0;
            font-size: 12px;
            color: #4fc3f7;
            text-transform: uppercase;
        }

        .diagnostic-repositioning p {
            margin: 0 0 8px 0;
            font-size: 13px;
            color: #ccc;
        }

        .repositioning-coords {
            font-family: monospace;
            font-size: 12px;
            color: #4fc3f7;
            background: rgba(0, 0, 0, 0.2);
            padding: 8px;
            border-radius: 4px;
        }

        .diagnostic-health-metrics h4 {
            margin: 0 0 12px 0;
            font-size: 13px;
            color: #888;
            text-transform: uppercase;
        }

        .health-metric {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            border-bottom: 1px solid rgba(255, 255, 255, 0.05);
            font-size: 13px;
        }

        .health-metric:last-child {
            border-bottom: none;
        }

        .metric-label {
            color: #888;
        }

        .metric-value {
            color: #4fc3f7;
            font-family: monospace;
        }

        .setting-change-hint {
            position: fixed;
            bottom: 100px;
            left: 50%;
            transform: translateX(-50%);
            width: 400px;
            max-width: calc(100vw - 40px);
            background: rgba(79, 195, 247, 0.15);
            border: 1px solid rgba(79, 195, 247, 0.5);
            border-radius: 12px;
            box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);
            z-index: 200;
            animation: hintSlideIn 0.3s ease-out;
        }

        @keyframes hintSlideIn {
            from {
                opacity: 0;
                transform: translateX(-50%) translateY(20px);
            }
            to {
                opacity: 1;
                transform: translateX(-50%) translateY(0);
            }
        }

        .hint-header {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 14px 16px;
            border-bottom: 1px solid rgba(79, 195, 247, 0.3);
        }

        .hint-icon {
            font-size: 20px;
        }

        .hint-title {
            flex: 1;
            font-weight: 600;
            font-size: 14px;
            color: #4fc3f7;
        }

        .hint-close {
            background: none;
            border: none;
            color: #888;
            font-size: 20px;
            cursor: pointer;
            padding: 4px;
        }

        .hint-content {
            padding: 14px 16px;
        }

        .hint-content p {
            margin: 0;
            font-size: 13px;
            line-height: 1.4;
            color: #ddd;
        }

        .hint-actions {
            display: flex;
            gap: 8px;
            padding: 12px 16px;
            border-top: 1px solid rgba(79, 195, 247, 0.3);
        }

        .hint-btn {
            flex: 1;
            padding: 8px 14px;
            border-radius: 6px;
            font-size: 13px;
            cursor: pointer;
            border: none;
        }

        .hint-btn-primary {
            background: #4fc3f7;
            color: #1a1a2e;
            font-weight: 500;
        }

        .hint-btn-secondary {
            background: rgba(255, 255, 255, 0.1);
            color: #ccc;
        }

        .feedback-explanation {
            margin-top: 12px;
            padding: 12px;
            background: rgba(79, 195, 247, 0.1);
            border: 1px solid rgba(79, 195, 247, 0.3);
            border-radius: 6px;
        }

        .explanation-header {
            margin-bottom: 8px;
        }

        .explanation-title {
            font-weight: 600;
            font-size: 13px;
            color: #4fc3f7;
        }

        .explanation-content {
            font-size: 12px;
            color: #bbb;
            line-height: 1.4;
        }

        .explanation-content strong {
            color: #ddd;
        }

        .explanation-additional-links {
            margin-top: 8px;
            padding-top: 8px;
            border-top: 1px solid rgba(255, 255, 255, 0.1);
            font-size: 11px;
            color: #888;
        }

        /* Calibration Flow Modal */
        .calibration-flow-modal {
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            z-index: 1000;
            display: flex;
            align-items: center;
            justify-content: center;
            animation: modalFadeIn 0.2s ease-out;
        }

        .calibration-flow-modal.calibration-closing {
            animation: modalFadeOut 0.3s ease-in forwards;
        }

        @keyframes modalFadeIn {
            from { opacity: 0; }
            to { opacity: 1; }
        }

        @keyframes modalFadeOut {
            from { opacity: 1; }
            to { opacity: 0; }
        }

        .calibration-flow-backdrop {
            position: absolute;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0, 0, 0, 0.7);
            backdrop-filter: blur(4px);
        }

        .calibration-flow-container {
            position: relative;
            width: 500px;
            max-width: calc(100vw - 40px);
            max-height: calc(100vh - 40px);
            background: #1e1e3a;
            border-radius: 16px;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.6);
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }

        .calibration-flow-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 20px 24px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }

        .calibration-flow-header h2 {
            margin: 0;
            font-size: 18px;
            color: #eee;
        }

        .calibration-close {
            background: none;
            border: none;
            color: #888;
            font-size: 24px;
            cursor: pointer;
            padding: 4px 8px;
            line-height: 1;
        }

        .calibration-close:hover {
            color: #fff;
        }

        .calibration-flow-content {
            padding: 24px;
            overflow-y: auto;
            flex: 1;
        }

        .calibration-step {
            text-align: center;
        }

        .calibration-icon {
            font-size: 48px;
            margin-bottom: 16px;
        }

        .calibration-icon.calibration-success {
            color: #4caf50;
        }

        .calibration-icon.calibration-error {
            color: #f44336;
        }

        .calibration-step h3 {
            margin: 0 0 12px 0;
            font-size: 20px;
            color: #eee;
        }

        .calibration-step p {
            margin: 0 0 16px 0;
            font-size: 14px;
            color: #bbb;
            line-height: 1.5;
        }

        .calibration-steps-overview {
            margin: 20px 0;
            text-align: left;
        }

        .overview-step {
            display: flex;
            align-items: center;
            gap: 12px;
            padding: 10px 0;
        }

        .overview-number {
            width: 28px;
            height: 28px;
            border-radius: 50%;
            background: rgba(79, 195, 247, 0.2);
            color: #4fc3f7;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 13px;
            font-weight: 600;
        }

        .overview-text {
            font-size: 13px;
            color: #ccc;
        }

        .calibration-progress {
            margin-bottom: 20px;
        }

        .progress-bar {
            height: 6px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 3px;
            overflow: hidden;
            margin-bottom: 8px;
        }

        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, #4fc3f7, #29b6f6);
            border-radius: 3px;
            transition: width 0.3s ease;
        }

        .progress-text {
            font-size: 12px;
            color: #888;
        }

        .calibration-timer {
            font-size: 48px;
            font-weight: 700;
            color: #4fc3f7;
            margin: 20px 0;
        }

        .calibration-status {
            font-size: 14px;
            color: #bbb;
        }

        .calibration-spinner {
            width: 48px;
            height: 48px;
            border: 4px solid rgba(79, 195, 247, 0.2);
            border-top-color: #4fc3f7;
            border-radius: 50%;
            margin: 20px auto;
            animation: spin 1s linear infinite;
        }

        @keyframes spin {
            to { transform: rotate(360deg); }
        }

        .calibration-suggestion {
            background: rgba(79, 195, 247, 0.1);
            border: 1px solid rgba(79, 195, 247, 0.3);
            border-radius: 12px;
            padding: 20px;
            margin: 20px 0;
        }

        .suggestion-value {
            display: flex;
            justify-content: center;
            align-items: baseline;
            gap: 8px;
            margin-bottom: 12px;
        }

        .suggestion-label {
            font-size: 14px;
            color: #ccc;
        }

        .suggestion-number {
            font-size: 32px;
            font-weight: 700;
            color: #4fc3f7;
        }

        .suggestion-reason {
            font-size: 13px;
            color: #bbb;
            line-height: 1.5;
        }

        .calibration-metrics {
            margin: 20px 0;
            text-align: left;
        }

        .calibration-metrics h4 {
            margin: 0 0 12px 0;
            font-size: 12px;
            color: #888;
            text-transform: uppercase;
        }

        .calibration-metric {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            border-bottom: 1px solid rgba(255, 255, 255, 0.05);
            font-size: 13px;
        }

        .calibration-metric:last-child {
            border-bottom: none;
        }

        .calibration-actions {
            display: flex;
            gap: 12px;
            margin-top: 20px;
        }

        .calibration-btn {
            flex: 1;
            padding: 12px 20px;
            border-radius: 8px;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            border: none;
            transition: background 0.2s;
        }

        .calibration-btn:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }

        .calibration-btn-primary {
            background: #4fc3f7;
            color: #1a1a2e;
        }

        .calibration-btn-primary:hover:not(:disabled) {
            background: #29b6f6;
        }

        .calibration-btn-secondary {
            background: rgba(255, 255, 255, 0.1);
            color: #ccc;
        }

        .calibration-btn-secondary:hover {
            background: rgba(255, 255, 255, 0.15);
        }

        .calibration-note {
            font-size: 12px;
            color: #888;
            margin-top: 12px;
        }

        .calibration-step-active {
            animation: pulse 2s ease-in-out infinite;
        }

        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.7; }
        }
    `;

    document.head.appendChild(style);

})();
