/**
 * Spaxel Dashboard - Router AP Detection Panel
 *
 * Shows when a router AP is auto-detected for passive radar mode.
 * Allows the user to place the virtual router node in the 3D editor.
 */

(function() {
    'use strict';

    // ============================================
    // State
    // ============================================
    const apState = {
        detectedAP: null,
        dismissed: false,
        placementMode: false
    };

    // ============================================
    // API
    // ============================================

    /**
     * Check for detected AP and show notification if found
     */
    function checkForDetectedAP() {
        if (apState.dismissed) {
            return Promise.resolve(null);
        }

        return fetch('/api/nodes')
            .then(res => res.json())
            .then(nodes => {
                const virtualAP = nodes.find(n => n.virtual && n.node_type === 'ap');
                if (virtualAP && !apState.detectedAP) {
                    apState.detectedAP = virtualAP;
                    showAPDetectionBanner(virtualAP);
                }
                return virtualAP;
            })
            .catch(err => {
                console.error('[APDetection] Error checking for AP:', err);
                return null;
            });
    }

    /**
     * Confirm the AP as a signal source and place it in 3D
     */
    function confirmAPAsSignalSource() {
        if (!apState.detectedAP) return;

        hideAPDetectionBanner();
        apState.placementMode = true;

        // Switch to 3D view and enable placement mode
        if (window.SpaxelRouter) {
            window.SpaxelRouter.setMode('3d');
        }

        // Show placement instructions
        showPlacementInstructions();

        // Enable drag-to-place for the virtual AP
        if (window.SpaxelViz3D) {
            window.SpaxelViz3D.enableNodePlacement(apState.detectedAP.mac, {
                onComplete: function(position) {
                    updateAPPosition(apState.detectedAP.mac, position);
                    apState.placementMode = false;
                    hidePlacementInstructions();
                    SpaxelPanels.showSuccess('Router placed successfully! Passive radar is now active.');
                },
                onCancel: function() {
                    apState.placementMode = false;
                    hidePlacementInstructions();
                }
            });
        }
    }

    /**
     * Update AP position in database
     */
    function updateAPPosition(mac, position) {
        return fetch('/api/nodes/' + mac, {
            method: 'PATCH',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                position: {
                    x: position.x,
                    y: position.y,
                    z: position.z
                }
            })
        })
        .then(res => {
            if (!res.ok) {
                throw new Error('Failed to update AP position');
            }
            return res.json();
        })
        .then(node => {
            // Update local state
            if (window.SpaxelState && window.SpaxelState.nodes) {
                window.SpaxelState.nodes[mac] = node;
            }
            return node;
        })
        .catch(err => {
            console.error('[APDetection] Error updating AP position:', err);
            SpaxelPanels.showError('Failed to place router: ' + err.message);
            throw err;
        });
    }

    /**
     * Dismiss the AP detection banner
     */
    function dismissAPDetection() {
        apState.dismissed = true;
        hideAPDetectionBanner();
    }

    // ============================================
    // UI Rendering
    // ============================================

    function showAPDetectionBanner(ap) {
        // Remove existing banner if present
        hideAPDetectionBanner();

        const banner = document.createElement('div');
        banner.id = 'ap-detection-banner';
        banner.className = 'ap-detection-banner';
        banner.innerHTML = `
            <div class="ap-detection-content">
                <div class="ap-detection-icon">📡</div>
                <div class="ap-detection-message">
                    <div class="ap-detection-title">Router Detected!</div>
                    <div class="ap-detection-subtitle">
                        I detected your router (${ap.ap_bssid || 'Unknown'})${ap.ap_channel ? ' on channel ' + ap.channel : ''}.
                        Place it on the floor plan to improve accuracy.
                    </div>
                </div>
                <div class="ap-detection-actions">
                    <button class="ap-btn ap-btn-secondary" id="ap-dismiss-btn">Later</button>
                    <button class="ap-btn ap-btn-primary" id="ap-confirm-btn">Place Router</button>
                </div>
            </div>
        `;

        document.body.appendChild(banner);

        // Attach event listeners
        document.getElementById('ap-dismiss-btn').addEventListener('click', dismissAPDetection);
        document.getElementById('ap-confirm-btn').addEventListener('click', confirmAPAsSignalSource);

        // Animate in
        requestAnimationFrame(() => {
            banner.classList.add('ap-detection-banner-visible');
        });
    }

    function hideAPDetectionBanner() {
        const banner = document.getElementById('ap-detection-banner');
        if (banner) {
            banner.classList.remove('ap-detection-banner-visible');
            setTimeout(() => {
                if (banner.parentNode) {
                    banner.parentNode.removeChild(banner);
                }
            }, 300);
        }
    }

    function showPlacementInstructions() {
        SpaxelPanels.openSidebar({
            title: 'Place Your Router',
            content: `
                <div class="ap-placement-content">
                    <p><strong>Drag the router icon</strong> in the 3D view to its actual location in your home.</p>
                    <ul class="ap-placement-list">
                        <li>The router should be placed at its <strong>actual physical location</strong></li>
                        <li>For best results, place it at <strong>router height</strong> (typically on a desk or shelf)</li>
                        <li>The virtual node helps the system understand signal geometry</li>
                    </ul>
                    <div class="ap-placement-tips">
                        <div class="ap-placement-tip">
                            <strong>Tip:</strong> You can fine-tune the position later in Setup Mode
                        </div>
                    </div>
                    <button class="panel-btn panel-btn-secondary panel-btn-full" id="ap-cancel-placement">
                        Cancel
                    </button>
                </div>
            `,
            width: '350px'
        });

        document.getElementById('ap-cancel-placement').addEventListener('click', () => {
            if (window.SpaxelViz3D) {
                window.SpaxelViz3D.cancelNodePlacement();
            }
            apState.placementMode = false;
            SpaxelPanels.closeSidebar();
        });
    }

    function hidePlacementInstructions() {
        if (window.SpaxelPanels) {
            window.SpaxelPanels.closeSidebar();
        }
    }

    // ============================================
    // Public API
    // ============================================

    window.SpaxelAPDetection = {
        checkForDetectedAP,
        confirmAPAsSignalSource,
        dismissAPDetection,
        updateAPPosition,
        getState: () => apState
    };

    // Auto-check on page load
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => {
            setTimeout(checkForDetectedAP, 2000);
        });
    } else {
        setTimeout(checkForDetectedAP, 2000);
    }

    // Check periodically for new AP detection
    setInterval(checkForDetectedAP, 30000);

    console.log('[APDetection] AP detection module loaded');
})();
