/**
 * Spaxel Dashboard - OTA Firmware Management
 *
 * Provides UI for firmware updates: list available versions,
 * trigger rolling updates, display progress, and show rollback warnings.
 */

(function() {
    'use strict';

    // State
    const state = {
        firmwareList: [],
        progress: {},
        pollInterval: null,
        otaInProgress: false
    };

    // ============================================
    // DOM Elements
    // ============================================
    let panel, firmwareList, progressList, updateAllBtn, closeBtn;

    // ============================================
    // Initialization
    // ============================================
    function init() {
        createPanel();
        createTriggerButton();
        startPolling();
    }

    function createPanel() {
        // Create OTA panel (hidden by default)
        panel = document.createElement('div');
        panel.id = 'ota-panel';
        panel.style.cssText = `
            position: fixed;
            top: 60px;
            left: 50%;
            transform: translateX(-50%);
            width: 400px;
            max-height: 60vh;
            background: rgba(0, 0, 0, 0.9);
            border-radius: 8px;
            padding: 16px;
            z-index: 200;
            display: none;
            overflow-y: auto;
        `;

        panel.innerHTML = `
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
                <h3 style="font-size: 14px; color: #888; text-transform: uppercase; letter-spacing: 1px; margin: 0;">
                    Firmware Updates
                </h3>
                <button id="ota-close-btn" style="background: none; border: none; color: #888; font-size: 18px; cursor: pointer; padding: 0 4px;">&times;</button>
            </div>

            <div id="ota-firmware-section" style="margin-bottom: 16px;">
                <div style="font-size: 12px; color: #666; margin-bottom: 6px;">Available Firmware</div>
                <div id="ota-firmware-list" style="max-height: 120px; overflow-y: auto;"></div>
            </div>

            <div id="ota-action-section" style="margin-bottom: 16px;">
                <button id="ota-update-all-btn" style="
                    width: 100%;
                    padding: 10px;
                    background: #4fc3f7;
                    color: #1a1a2e;
                    border: none;
                    border-radius: 4px;
                    font-size: 13px;
                    font-weight: 500;
                    cursor: pointer;
                    transition: background 0.2s;
                ">Update All Nodes</button>
            </div>

            <div id="ota-progress-section">
                <div style="font-size: 12px; color: #666; margin-bottom: 6px;">Update Progress</div>
                <div id="ota-progress-list"></div>
            </div>
        `;

        document.body.appendChild(panel);

        // Get references
        firmwareList = document.getElementById('ota-firmware-list');
        progressList = document.getElementById('ota-progress-list');
        updateAllBtn = document.getElementById('ota-update-all-btn');
        closeBtn = document.getElementById('ota-close-btn');

        // Event handlers
        closeBtn.addEventListener('click', hide);
        updateAllBtn.addEventListener('click', triggerUpdateAll);
    }

    function createTriggerButton() {
        // Add OTA button to status bar
        const statusBar = document.getElementById('status-bar');
        if (!statusBar) return;

        const btn = document.createElement('button');
        btn.id = 'ota-btn';
        btn.textContent = 'OTA';
        btn.style.cssText = `
            background: rgba(255, 255, 255, 0.06);
            border: 1px solid rgba(255, 255, 255, 0.12);
            color: #888;
            font-size: 12px;
            padding: 3px 10px;
            border-radius: 4px;
            cursor: pointer;
            transition: background 0.2s, color 0.2s;
        `;
        btn.addEventListener('click', toggle);

        // Insert before FPS counter
        const fpsItem = statusBar.querySelector('.status-item:last-child');
        if (fpsItem) {
            statusBar.insertBefore(btn, fpsItem);
        } else {
            statusBar.appendChild(btn);
        }
    }

    // ============================================
    // Panel Visibility
    // ============================================
    function toggle() {
        if (panel.style.display === 'none') {
            show();
        } else {
            hide();
        }
    }

    function show() {
        panel.style.display = 'block';
        fetchFirmwareList();
        fetchProgress();
    }

    function hide() {
        panel.style.display = 'none';
    }

    // ============================================
    // API Calls
    // ============================================
    async function fetchFirmwareList() {
        try {
            const resp = await fetch('/api/firmware');
            if (!resp.ok) throw new Error('Failed to fetch firmware list');
            state.firmwareList = await resp.json();
            renderFirmwareList();
        } catch (e) {
            console.error('[OTA] Failed to fetch firmware:', e);
            firmwareList.innerHTML = '<div style="color: #666; font-size: 12px;">Failed to load</div>';
        }
    }

    async function fetchProgress() {
        try {
            const resp = await fetch('/api/firmware/progress');
            if (!resp.ok) return;
            state.progress = await resp.json();
            renderProgress();
            updateButtonState();
        } catch (e) {
            console.error('[OTA] Failed to fetch progress:', e);
        }
    }

    async function triggerUpdateAll() {
        if (state.otaInProgress) return;

        updateAllBtn.disabled = true;
        updateAllBtn.textContent = 'Starting...';

        try {
            const resp = await fetch('/api/firmware/ota-all', { method: 'POST' });
            if (!resp.ok) throw new Error('Failed to start OTA');
            state.otaInProgress = true;
            updateAllBtn.textContent = 'Update in Progress...';
            updateAllBtn.style.background = '#ffa726';
        } catch (e) {
            console.error('[OTA] Failed to trigger update:', e);
            updateAllBtn.disabled = false;
            updateAllBtn.textContent = 'Update All Nodes';
            alert('Failed to start firmware update: ' + e.message);
        }
    }

    // ============================================
    // Rendering
    // ============================================
    function renderFirmwareList() {
        if (!state.firmwareList || state.firmwareList.length === 0) {
            firmwareList.innerHTML = '<div style="color: #666; font-size: 12px;">No firmware available. Upload via /api/firmware/upload</div>';
            return;
        }

        let html = '';
        state.firmwareList.forEach(function(fw) {
            const latestBadge = fw.is_latest
                ? '<span style="background: rgba(76, 175, 80, 0.3); color: #81c784; font-size: 10px; padding: 1px 5px; border-radius: 3px; margin-left: 6px;">LATEST</span>'
                : '';
            const sizeKB = Math.round(fw.size_bytes / 1024);

            html += `
                <div style="padding: 6px 8px; margin-bottom: 4px; background: rgba(255,255,255,0.05); border-radius: 4px; font-size: 12px;">
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                        <span style="color: #ccc;">
                            ${escapeHtml(fw.filename)}
                            ${latestBadge}
                        </span>
                        <span style="color: #666; font-size: 11px;">${sizeKB} KB</span>
                    </div>
                    <div style="color: #555; font-size: 10px; margin-top: 2px; font-family: monospace;">
                        SHA256: ${fw.sha256.substring(0, 12)}...
                    </div>
                </div>
            `;
        });
        firmwareList.innerHTML = html;
    }

    function renderProgress() {
        const entries = Object.entries(state.progress);

        if (entries.length === 0) {
            progressList.innerHTML = '<div style="color: #666; font-size: 12px;">No updates in progress</div>';
            return;
        }

        let html = '';
        entries.forEach(function([mac, p]) {
            const stateInfo = getStateInfo(p.state);
            const progressBar = renderProgressBar(p.progress_pct, stateInfo.color);
            const rollbackBadge = p.state === 'rollback'
                ? '<span style="background: rgba(244, 67, 54, 0.4); color: #ef5350; font-size: 10px; padding: 2px 6px; border-radius: 3px; margin-left: 6px;">ROLLBACK</span>'
                : '';

            html += `
                <div style="padding: 8px; margin-bottom: 6px; background: rgba(255,255,255,0.05); border-radius: 4px;">
                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
                        <span style="font-family: monospace; font-size: 12px; color: #4fc3f7;">${mac}</span>
                        <span style="color: ${stateInfo.color}; font-size: 11px; font-weight: 500;">
                            ${stateInfo.label}
                            ${rollbackBadge}
                        </span>
                    </div>
                    ${progressBar}
                    ${p.error ? `<div style="color: #ef5350; font-size: 11px; margin-top: 4px;">${escapeHtml(p.error)}</div>` : ''}
                    ${p.expected_version ? `<div style="color: #666; font-size: 10px; margin-top: 2px;">Target: ${escapeHtml(p.expected_version)}</div>` : ''}
                </div>
            `;
        });
        progressList.innerHTML = html;

        // Check for any active updates
        const hasActive = entries.some(function([_, p]) {
            return ['pending', 'downloading', 'rebooting'].includes(p.state);
        });

        if (!hasActive && state.otaInProgress) {
            state.otaInProgress = false;
            updateButtonState();
        }

        // Update status bar button state
        updateStatusBarButton();

        // Trigger node list refresh if app.js is available
        if (window.SpaxelApp && typeof SpaxelApp.refreshNodeList === 'function') {
            SpaxelApp.refreshNodeList();
        }
    }

    function updateStatusBarButton() {
        const btn = document.getElementById('ota-btn');
        if (!btn) return;

        const entries = Object.entries(state.progress);
        const hasRollback = entries.some(function([_, p]) { return p.state === 'rollback'; });
        const hasActive = entries.some(function([_, p]) {
            return ['pending', 'downloading', 'rebooting'].includes(p.state);
        });

        btn.classList.remove('has-update', 'in-progress');
        if (hasRollback) {
            btn.classList.add('has-update');
            btn.textContent = 'OTA!';
        } else if (hasActive) {
            btn.classList.add('in-progress');
            btn.textContent = 'OTA...';
        } else {
            btn.textContent = 'OTA';
        }
    }

    function renderProgressBar(pct, color) {
        const pctVal = pct || 0;
        return `
            <div style="width: 100%; height: 4px; background: #333; border-radius: 2px; overflow: hidden;">
                <div style="width: ${pctVal}%; height: 100%; background: ${color}; transition: width 0.3s;"></div>
            </div>
        `;
    }

    function getStateInfo(s) {
        switch (s) {
            case 'idle': return { label: 'Idle', color: '#888' };
            case 'pending': return { label: 'Pending', color: '#4fc3f7' };
            case 'downloading': return { label: 'Downloading', color: '#29b6f6' };
            case 'rebooting': return { label: 'Rebooting', color: '#ffa726' };
            case 'verified': return { label: 'Verified', color: '#66bb6a' };
            case 'failed': return { label: 'Failed', color: '#ef5350' };
            case 'rollback': return { label: 'Rollback', color: '#ef5350' };
            default: return { label: s || 'Unknown', color: '#888' };
        }
    }

    function updateButtonState() {
        if (state.otaInProgress) {
            updateAllBtn.disabled = true;
            updateAllBtn.textContent = 'Update in Progress...';
            updateAllBtn.style.background = '#ffa726';
        } else {
            updateAllBtn.disabled = false;
            updateAllBtn.textContent = 'Update All Nodes';
            updateAllBtn.style.background = '#4fc3f7';
        }
    }

    // ============================================
    // Polling
    // ============================================
    function startPolling() {
        if (state.pollInterval) return;
        state.pollInterval = setInterval(function() {
            if (panel.style.display !== 'none' || state.otaInProgress) {
                fetchProgress();
            }
        }, 2000);
    }

    // ============================================
    // Utilities
    // ============================================
    function escapeHtml(s) {
        if (!s) return '';
        return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelOTA = {
        init: init,
        show: show,
        hide: hide,
        toggle: toggle,
        getProgress: function() { return state.progress; },
        isInProgress: function() { return state.otaInProgress; }
    };

    // Auto-init when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
