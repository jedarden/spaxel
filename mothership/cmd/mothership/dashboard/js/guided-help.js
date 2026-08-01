/**
 * Spaxel Dashboard - Guided Troubleshooting
 *
 * Proactive contextual help that appears when users encounter problems.
 * Provides step-by-step guidance and explains what went wrong.
 */

(function() {
    'use strict';

    // ===== State =====
    let activeGuide = null;
    let dismissedGuides = new Set();
    let helpPanelVisible = false;

    // ===== DOM Elements =====
    const helpContainer = document.createElement('div');
    helpContainer.id = 'guided-help-container';
    helpContainer.className = 'guided-help-container';
    document.body.appendChild(helpContainer);

    // ===== Guide Definitions =====
    const guides = {
        // No nodes detected
        'no-nodes': {
            title: 'No Nodes Detected',
            icon: '📡',
            trigger: 'no-nodes',
            steps: [
                {
                    title: 'Check Power',
                    content: 'Ensure each Spaxel node is powered on. The LED should be breathing blue.',
                    action: null
                },
                {
                    title: 'Check WiFi',
                    content: 'Nodes must connect to your WiFi network. Verify your WiFi is working.',
                    action: null
                },
                {
                    title: 'Add Missing Nodes',
                    content: 'If nodes are powered but not showing, click "+ Add Node" to onboard them.',
                    action: 'add-node'
                }
            ],
            dismissible: true
        },

        // Poor detection quality
        'poor-detection': {
            title: 'Detection Quality Issues',
            icon: '📉',
            trigger: 'poor-quality',
            steps: [
                {
                    title: 'Check Node Placement',
                    content: 'Nodes should be placed at least 2 meters apart for best accuracy.',
                    action: null
                },
                {
                    title: 'Review Coverage',
                    content: 'Enable GDOP view to see coverage gaps in your space.',
                    action: 'toggle-gdop'
                },
                {
                    title: 'Add More Nodes',
                    content: 'Large spaces or areas with obstacles may need additional nodes.',
                    action: 'add-node'
                }
            ],
            dismissible: true
        },

        // Node went offline
        'node-offline': {
            title: 'Node Offline',
            icon: '⚠️',
            trigger: 'node-offline',
            steps: [
                {
                    title: 'Check Power',
                    content: 'Verify the node is still receiving power. Check cables and connections.',
                    action: null
                },
                {
                    title: 'Check WiFi Signal',
                    content: 'The node may be out of WiFi range. Move it closer to your router.',
                    action: null
                },
                {
                    title: 'Restart Node',
                    content: 'Try power cycling the node by unplugging and replugging it.',
                    action: null
                }
            ],
            dismissible: true
        },

        // First anomaly detected
        'first-anomaly': {
            title: 'Anomaly Detected',
            icon: '🔔',
            trigger: 'first-anomaly',
            steps: [
                {
                    title: 'Review the Event',
                    content: 'An unusual pattern was detected. Check the timeline for details.',
                    action: 'view-timeline'
                },
                {
                    title: 'Verify Accuracy',
                    content: 'Was this a real event or a false positive? Your feedback helps us improve.',
                    action: 'give-feedback'
                },
                {
                    title: 'Adjust Sensitivity',
                    content: 'If this keeps triggering falsely, you can adjust anomaly sensitivity in settings.',
                    action: 'open-settings'
                }
            ],
            dismissible: true
        },

        // Security mode first use
        'security-first-use': {
            title: 'Security Mode Enabled',
            icon: '🔒',
            trigger: 'security-first-use',
            steps: [
                {
                    title: 'What is Security Mode?',
                    content: 'Security mode enhances detection sensitivity and triggers alerts for any presence.',
                    action: null
                },
                {
                    title: 'Set Up Alerts',
                    content: 'Configure webhooks or notifications to be alerted of security events.',
                    action: 'open-automations'
                },
                {
                    title: 'Test the System',
                    content: 'Walk through your space to verify detection is working as expected.',
                    action: null
                }
            ],
            dismissible: true
        },

        // High GDOP area
        'high-gdop': {
            title: 'Poor Positioning Coverage',
            icon: '📍',
            trigger: 'high-gdop',
            steps: [
                {
                    title: 'Understanding GDOP',
                    content: 'GDOP measures positioning accuracy. Red areas have poor accuracy.',
                    action: null
                },
                {
                    title: 'Add Virtual Nodes',
                    content: 'Use the simulator to test if adding a node would improve coverage.',
                    action: 'open-simulator'
                },
                {
                    title: 'Reposition Existing Nodes',
                    content: 'Small adjustments to node placement can significantly improve coverage.',
                    action: null
                }
            ],
            dismissible: true
        },

        // Frequent false positives
        'false-positives': {
            title: 'Reducing False Detections',
            icon: '✅',
            trigger: 'false-positives',
            steps: [
                {
                    title: 'Review Recent Events',
                    content: 'Check the timeline to see when false detections are occurring.',
                    action: 'view-timeline'
                },
                {
                    title: 'Provide Feedback',
                    content: 'Mark false detections to help the system learn and improve.',
                    action: 'give-feedback'
                },
                {
                    title: 'Adjust Diurnal Settings',
                    content: 'Ensure your home patterns are fully learned (7+ days of data).',
                    action: null
                }
            ],
            dismissible: true
        },

        // Sleep tracking not working
        'sleep-not-working': {
            title: 'Sleep Tracking Setup',
            icon: '😴',
            trigger: 'sleep-not-working',
            steps: [
                {
                    title: 'Define Bedroom Zone',
                    content: 'Create a zone for your bedroom to track sleep patterns.',
                    action: 'create-zone'
                },
                {
                    title: 'Add Bed Trigger',
                    content: 'Place a virtual trigger at your bed location for accurate detection.',
                    action: 'add-trigger'
                },
                {
                    title: 'Wait for Learning',
                    content: 'Sleep patterns need 7+ nights of data to establish baselines.',
                    action: null
                }
            ],
            dismissible: true
        },

        // Automation not firing
        'automation-not-firing': {
            title: 'Automation Troubleshooting',
            icon: '⚡',
            trigger: 'automation-failed',
            steps: [
                {
                    title: 'Check Trigger Conditions',
                    content: 'Verify the zone, person, and time conditions match your setup.',
                    action: 'view-automations'
                },
                {
                    title: 'Test Webhook',
                    content: 'Use the test button to verify your webhook endpoint is responding.',
                    action: 'test-webhook'
                },
                {
                    title: 'Check Automation Logs',
                    content: 'Review recent automation events to see why it didn\'t fire.',
                    action: 'view-logs'
                }
            ],
            dismissible: true
        }
    };

    // ===== Guide Execution =====
    function showGuide(guideId, context = {}) {
        if (dismissedGuides.has(guideId)) {
            return; // Don't show dismissed guides
        }

        const guide = guides[guideId];
        if (!guide) {
            console.warn('Guide not found:', guideId);
            return;
        }

        activeGuide = {
            id: guideId,
            data: guide,
            currentStep: 0,
            context: context
        };

        renderGuide();
        helpPanelVisible = true;
    }

    function renderGuide() {
        if (!activeGuide) return;

        const guide = activeGuide.data;
        const step = guide.steps[activeGuide.currentStep];
        const isLastStep = activeGuide.currentStep === guide.steps.length - 1;

        helpContainer.innerHTML = `
            <div class="guided-help-panel">
                <div class="help-header">
                    <div class="help-title">
                        <span class="help-icon">${guide.icon}</span>
                        <h3>${guide.title}</h3>
                    </div>
                    <button class="help-close-btn" onclick="GuidedHelp.dismiss()">&times;</button>
                </div>

                <div class="help-content">
                    <div class="help-progress">
                        ${guide.steps.map((_, i) => `
                            <div class="help-progress-dot ${i === activeGuide.currentStep ? 'active' : ''} ${i < activeGuide.currentStep ? 'completed' : ''}"></div>
                        `).join('')}
                    </div>

                    <div class="help-step">
                        <h4 class="step-title">${step.title}</h4>
                        <p class="step-content">${step.content}</p>
                    </div>

                    ${guide.dismissible ? `
                        <div class="help-dismiss-hint">
                            <label class="help-dismiss-checkbox">
                                <input type="checkbox" id="help-dont-show-again">
                                <span>Don't show this guide again</span>
                            </label>
                        </div>
                    ` : ''}
                </div>

                <div class="help-actions">
                    ${activeGuide.currentStep > 0 ? `
                        <button class="help-btn help-btn-secondary" onclick="GuidedHelp.previousStep()">
                            ← Back
                        </button>
                    ` : `
                        <button class="help-btn help-btn-secondary" onclick="GuidedHelp.dismiss()">
                            Skip
                        </button>
                    `}

                    ${step.action ? `
                        <button class="help-btn help-btn-action" onclick="GuidedHelp.executeAction('${step.action}')">
                            ${getActionLabel(step.action)}
                        </button>
                    ` : ''}

                    ${!isLastStep ? `
                        <button class="help-btn help-btn-primary" onclick="GuidedHelp.nextStep()">
                            Next →
                        </button>
                    ` : `
                        <button class="help-btn help-btn-primary" onclick="GuidedHelp.complete()">
                            Got it
                        </button>
                    `}
                </div>
            </div>
        `;

        helpContainer.classList.add('visible');
    }

    function getActionLabel(action) {
        const labels = {
            'add-node': 'Add Node',
            'toggle-gdop': 'Show GDOP',
            'view-timeline': 'View Timeline',
            'give-feedback': 'Give Feedback',
            'open-settings': 'Open Settings',
            'open-automations': 'Automations',
            'open-simulator': 'Open Simulator',
            'create-zone': 'Create Zone',
            'add-trigger': 'Add Trigger',
            'view-automations': 'View Automations',
            'test-webhook': 'Test Webhook',
            'view-logs': 'View Logs'
        };
        return labels[action] || 'Action';
    }

    function executeAction(action) {
        switch (action) {
            case 'add-node':
                if (window.SpaxelOnboard) {
                    SpaxelOnboard.start();
                }
                break;
            case 'toggle-gdop':
                if (window.Placement) {
                    Placement.toggleGDOP();
                }
                break;
            case 'view-timeline':
                if (window.SpaxelRouter) {
                    SpaxelRouter.navigate('timeline');
                }
                break;
            case 'give-feedback':
                if (window.FeedbackUI) {
                    FeedbackUI.openForContext(activeGuide.context);
                }
                break;
            case 'open-settings':
                if (window.openSettingsPanel) {
                    openSettingsPanel();
                }
                break;
            case 'open-automations':
                if (window.SpaxelRouter) {
                    SpaxelRouter.navigate('automations');
                }
                break;
            case 'open-simulator':
                if (window.Simulate) {
                    Simulate.togglePanel();
                }
                break;
            case 'create-zone':
                // Open zone editor
                break;
            case 'add-trigger':
                // Open trigger editor
                break;
            case 'view-automations':
                if (window.SpaxelRouter) {
                    SpaxelRouter.navigate('automations');
                }
                break;
            case 'test-webhook':
                // Test webhook functionality
                break;
            case 'view-logs':
                // Show automation logs
                break;
        }
    }

    // ===== Navigation =====
    function nextStep() {
        if (!activeGuide) return;
        if (activeGuide.currentStep < activeGuide.data.steps.length - 1) {
            activeGuide.currentStep++;
            renderGuide();
        }
    }

    function previousStep() {
        if (!activeGuide) return;
        if (activeGuide.currentStep > 0) {
            activeGuide.currentStep--;
            renderGuide();
        }
    }

    function complete() {
        const dontShowAgain = document.getElementById('help-dont-show-again');
        if (dontShowAgain && dontShowAgain.checked) {
            dismissedGuides.add(activeGuide.id);
            saveDismissedGuides();
        }
        dismiss();
    }

    function dismiss() {
        helpContainer.classList.remove('visible');
        activeGuide = null;
        helpPanelVisible = false;
    }

    // ===== Persistence =====
    function saveDismissedGuides() {
        try {
            localStorage.setItem('spaxel_dismissed_guides', Array.from(dismissedGuides).join(','));
        } catch (e) {
            console.warn('Failed to save dismissed guides:', e);
        }
    }

    function loadDismissedGuides() {
        try {
            const saved = localStorage.getItem('spaxel_dismissed_guides');
            if (saved) {
                dismissedGuides = new Set(saved.split(',').filter(id => id));
            }
        } catch (e) {
            console.warn('Failed to load dismissed guides:', e);
        }
    }

    // ===== Proactive Triggers =====
    function checkProactiveTriggers() {
        // No nodes
        if (window.Viz3D && Viz3D.getNodes && Viz3D.getNodes().length === 0) {
            showGuide('no-nodes');
        }

        // Poor detection quality
        const qualityGauge = document.getElementById('quality-value');
        if (qualityGauge) {
            const quality = parseInt(qualityGauge.textContent);
            if (!isNaN(quality) && quality < 60) {
                showGuide('poor-detection');
            }
        }
    }

    // ===== Context Menu Integration =====
    function addContextualHelp(target, guideId) {
        // Add help button to context menus
        const helpBtn = document.createElement('button');
        helpBtn.className = 'context-help-btn';
        helpBtn.innerHTML = '?';
        helpBtn.onclick = () => showGuide(guideId);
        return helpBtn;
    }

    // ===== Public API =====
    window.GuidedHelp = {
        show: showGuide,
        dismiss: dismiss,
        nextStep: nextStep,
        previousStep: previousStep,
        complete: complete,
        executeAction: executeAction,
        addContextualHelp: addContextualHelp,
        checkTriggers: checkProactiveTriggers
    };

    // ===== Initialization =====
    loadDismissedGuides();

    // Check triggers after a short delay
    setTimeout(checkProactiveTriggers, 3000);

    // Listen for system events that might trigger guides
    window.addEventListener('spaxel:node-offline', (e) => {
        showGuide('node-offline', { nodeId: e.detail.nodeId });
    });

    window.addEventListener('spaxel:first-anomaly', (e) => {
        showGuide('first-anomaly', { anomalyId: e.detail.anomalyId });
    });

    window.addEventListener('spaxel:security-enabled', () => {
        if (!dismissedGuides.has('security-first-use')) {
            showGuide('security-first-use');
        }
    });

    window.addEventListener('spaxel:automation-failed', (e) => {
        showGuide('automation-not-firing', { automationId: e.detail.automationId });
    });

})();
