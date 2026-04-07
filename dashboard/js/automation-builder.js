/**
 * Spatial Automation Builder - Trigger volume editor and automation UI
 * Integrates with VolumeEditor for 3D volume creation and editing
 */

(function() {
    'use strict';

    // ── Module state ─────────────────────────────────────────────────────────────
    let _triggers = [];
    let _currentVolume = null; // Currently drawn volume shape
    let _editingId = null;
    let _volumeEditorReady = false;

    // Trigger condition labels
    const CONDITION_LABELS = {
        'enter': 'When someone enters the volume',
        'leave': 'When someone leaves the volume',
        'dwell': 'When someone stays in the volume for a while',
        'vacant': 'When the volume becomes empty',
        'count': 'When occupant count crosses threshold'
    };

    // ── Initialization ─────────────────────────────────────────────────────────────
    function init() {
        console.log('[AutomationBuilder] Initializing');
        createPanelHTML();
        setupEventListeners();
        loadTriggers();

        // Subscribe to state changes for triggers
        if (window.SpaxelState) {
            SpaxelState.subscribe('triggers', onTriggersChanged);
        }

        // Initialize volume editor callback
        if (window.VolumeEditor) {
            VolumeEditor.onVolumeCreated(onVolumeCreated);
            VolumeEditor.onVolumeChanged(onVolumeChanged);
            VolumeEditor.onVolumeDeleted(onVolumeDeleted);
            _volumeEditorReady = true;
        } else {
            console.warn('[AutomationBuilder] VolumeEditor not loaded yet');
        }
    }

    function createPanelHTML() {
        const container = document.getElementById('automations-panel');
        if (!container) {
            console.warn('[AutomationBuilder] #automations-panel not found');
            return;
        }

        container.innerHTML = `
            <div class="automations-header">
                <h2>Spatial Triggers</h2>
                <div class="volume-tools">
                    <button class="btn btn-secondary" id="draw-box-btn" title="Draw box volume">
                        ▢ Box
                    </button>
                    <button class="btn btn-secondary" id="draw-cylinder-btn" title="Draw cylinder volume">
                        ○ Cylinder
                    </button>
                    <button class="btn btn-primary" id="new-trigger-btn">
                        + New Trigger
                    </button>
                </div>
            </div>

            <div class="triggers-list" id="triggers-list">
                <p class="text-muted">Loading triggers...</p>
            </div>

            <div class="trigger-log-section" id="trigger-log-section">
                <h3>Trigger Log</h3>
                <div id="trigger-log" class="trigger-log">
                    <p class="text-muted">No trigger firings yet.</p>
                </div>
            </div>

            <!-- Trigger Editor Modal -->
            <div class="modal" id="trigger-modal" style="display: none;">
                <div class="modal-content">
                    <div class="modal-header">
                        <h3 id="modal-title">New Trigger</h3>
                        <button class="modal-close" id="modal-close">&times;</button>
                    </div>

                    <div class="modal-body">
                        <div class="form-group">
                            <label>Name</label>
                            <input type="text" id="trigger-name" class="form-control" placeholder="e.g., Kitchen Couch Dwell">
                        </div>

                        <div class="form-group">
                            <label>Volume</label>
                            <div id="current-volume-shape" class="volume-shape-info">
                                <em class="text-muted">No volume selected. Click "Box" or "Cylinder" to draw one.</em>
                            </div>
                            <small class="text-muted" id="volume-dimensions"></small>
                        </div>

                        <div class="form-group">
                            <label>Condition</label>
                            <select id="trigger-condition" class="form-control">
                                ${Object.entries(CONDITION_LABELS).map(([k, v]) =>
                                    `<option value="${k}">${v}</option>`
                                ).join('')}
                            </select>
                        </div>

                        <div class="form-group" id="condition-params-group">
                            <label>Parameters</label>
                            <div id="condition-params-form">
                                <!-- Dynamically filled based on condition -->
                            </div>
                        </div>

                        <div class="form-group">
                            <label>Time Constraint (Optional)</label>
                            <div class="time-constraint-inputs">
                                <input type="time" id="time-from" class="form-control" placeholder="22:00">
                                <span>to</span>
                                <input type="time" id="time-to" class="form-control" placeholder="07:00">
                            </div>
                            <small class="text-muted">Leave empty for always-active</small>
                        </div>

                        <div class="form-group">
                            <label>Actions</label>
                            <div id="actions-list"></div>
                            <button class="btn btn-secondary btn-sm" id="add-action-btn">+ Add Action</button>
                        </div>
                    </div>

                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="test-trigger-btn">Test</button>
                        <button class="btn btn-secondary" id="modal-cancel">Cancel</button>
                        <button class="btn btn-primary" id="modal-save">Save</button>
                    </div>
                </div>
            </div>

            <!-- Action Modal -->
            <div class="modal" id="action-modal" style="display: none;">
                <div class="modal-content modal-sm">
                    <div class="modal-header">
                        <h3>Add Action</h3>
                        <button class="modal-close" id="action-modal-close">&times;</button>
                    </div>

                    <div class="modal-body">
                        <div class="form-group">
                            <label>Action Type</label>
                            <select id="action-type" class="form-control">
                                <option value="webhook">HTTP Webhook</option>
                                <option value="mqtt">MQTT Publish</option>
                                <option value="ntfy">Ntfy Notification</option>
                            </select>
                        </div>

                        <div id="action-config-form"></div>
                    </div>

                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="action-cancel">Cancel</button>
                        <button class="btn btn-primary" id="action-save">Add</button>
                    </div>
                </div>
            </div>
        `;
    }

    function setupEventListeners() {
        // Volume drawing buttons
        document.getElementById('draw-box-btn')?.addEventListener('click', () => startDrawVolume('box'));
        document.getElementById('draw-cylinder-btn')?.addEventListener('click', () => startDrawVolume('cylinder'));

        // New trigger button
        document.getElementById('new-trigger-btn')?.addEventListener('click', openTriggerEditor);

        // Modal controls
        document.getElementById('modal-close')?.addEventListener('click', closeTriggerEditor);
        document.getElementById('modal-cancel')?.addEventListener('click', closeTriggerEditor);
        document.getElementById('modal-save')?.addEventListener('click', saveTrigger);
        document.getElementById('test-trigger-btn')?.addEventListener('click', () => test(_editingId));

        // Condition change
        document.getElementById('trigger-condition')?.addEventListener('change', onConditionChange);

        // Action modal
        document.getElementById('add-action-btn')?.addEventListener('click', openActionModal);
        document.getElementById('action-cancel')?.addEventListener('click', closeActionModal);
        document.getElementById('action-save')?.addEventListener('click', saveAction);
        document.getElementById('action-type')?.addEventListener('change', onActionTypeChange);
        document.getElementById('action-modal-close')?.addEventListener('click', closeActionModal);
    }

    // ── Volume Drawing ────────────────────────────────────────────────────────────────
    function startDrawVolume(type) {
        if (_volumeEditorReady && window.VolumeEditor) {
            VolumeEditor.startDrawMode(type);

            // Update UI to show drawing state
            document.querySelectorAll('.volume-tools button').forEach(btn => {
                if (btn.id === `draw-${type}-btn`) {
                    btn.classList.add('active');
                } else {
                    btn.disabled = true;
                }
            });

            // Update cursor in 3D scene
            document.getElementById('scene-container').style.cursor = 'crosshair';

            // Close any open modal
            closeTriggerEditor();
        } else {
            alert('3D scene not ready. Please wait for the dashboard to fully load.');
        }
    }

    function stopDrawVolume() {
        if (_volumeEditorReady && window.VolumeEditor) {
            VolumeEditor.cancelDrawMode();
        }

        // Reset UI
        document.querySelectorAll('.volume-tools button').forEach(btn => {
            btn.classList.remove('active');
            btn.disabled = false;
        });

        document.getElementById('scene-container').style.cursor = 'default';
    }

    function onVolumeCreated(shape) {
        _currentVolume = shape;
        updateVolumeDisplay();

        // Stop drawing mode
        stopDrawVolume();

        // Open trigger editor with the new volume
        openTriggerEditor();
    }

    function onVolumeChanged(volumeId, shape) {
        if (_editingId) {
            _currentVolume = shape;
            updateVolumeDisplay();
        }
    }

    function onVolumeDeleted(volumeId) {
        if (_editingId && _currentVolume) {
            _currentVolume = null;
            updateVolumeDisplay();
        }
    }

    function updateVolumeDisplay() {
        const display = document.getElementById('current-volume-shape');
        const dims = document.getElementById('volume-dimensions');

        if (!_currentVolume) {
            display.innerHTML = '<em class="text-muted">No volume selected. Click "Box" or "Cylinder" to draw one.</em>';
            dims.textContent = '';
            return;
        }

        let typeText, dimsText;
        if (_currentVolume.type === 'box') {
            typeText = 'Box';
            dimsText = `${_currentVolume.w?.toFixed(1)}m × ${_currentVolume.d?.toFixed(1)}m × ${_currentVolume.h?.toFixed(1)}m`;
        } else if (_currentVolume.type === 'cylinder') {
            typeText = 'Cylinder';
            dimsText = `r: ${_currentVolume.r?.toFixed(1)}m, h: ${_currentVolume.h?.toFixed(1)}m`;
        } else {
            typeText = 'Unknown';
            dimsText = '';
        }

        display.innerHTML = `<strong>${typeText}</strong> volume created`;
        dims.textContent = dimsText;
    }

    // ── Trigger Editor ───────────────────────────────────────────────────────────────
    function openTriggerEditor(trigger = null) {
        _editingId = trigger?.id || null;

        document.getElementById('modal-title').textContent = trigger ? 'Edit Trigger' : 'New Trigger';
        document.getElementById('trigger-name').value = trigger?.name || '';
        document.getElementById('trigger-condition').value = trigger?.condition || 'enter';
        document.getElementById('trigger-cooldown')?.value = trigger?.cooldown || 60;

        // Load volume if editing
        if (trigger?.shape) {
            _currentVolume = trigger.shape;
            updateVolumeDisplay();
        }

        // Load time constraint
        if (trigger?.time_constraint) {
            document.getElementById('time-from').value = trigger.time_constraint.from || '';
            document.getElementById('time-to').value = trigger.time_constraint.to || '';
        }

        // Render condition params
        onConditionChange();

        // Render actions
        renderActions(trigger?.actions || []);

        document.getElementById('trigger-modal').style.display = 'flex';
    }

    function closeTriggerEditor() {
        document.getElementById('trigger-modal').style.display = 'none';
        _editingId = null;
        _currentVolume = null;
        updateVolumeDisplay();
    }

    function onConditionChange() {
        const condition = document.getElementById('trigger-condition').value;
        const form = document.getElementById('condition-params-form');

        let html = '';
        switch (condition) {
            case 'dwell':
                html = `
                    <label>Dwell Time (seconds)</label>
                    <input type="number" id="dwell-duration" class="form-control" value="30" min="1">
                    <small class="text-muted">Trigger after staying inside for this many seconds</small>
                `;
                break;
            case 'count':
                html = `
                    <label>Count Threshold</label>
                    <input type="number" id="count-threshold" class="form-control" value="1" min="1">
                    <small class="text-muted">Trigger when this many people are inside</small>
                `;
                break;
            case 'vacant':
                html = `
                    <label>Vacant Time (seconds)</label>
                    <input type="number" id="vacant-duration" class="form-control" value="10" min="1">
                    <small class="text-muted">Trigger after empty for this many seconds</small>
                `;
                break;
            default:
                html = '<small class="text-muted">No parameters needed for this condition</small>';
        }
        form.innerHTML = html;
    }

    function getConditionParams() {
        const condition = document.getElementById('trigger-condition').value;
        const params = {};

        switch (condition) {
            case 'dwell':
                const duration = document.getElementById('dwell-duration')?.value;
                if (duration) params.duration_s = parseInt(duration);
                break;
            case 'count':
                const threshold = document.getElementById('count-threshold')?.value;
                if (threshold) params.count_threshold = parseInt(threshold);
                break;
            case 'vacant':
                const vacantDuration = document.getElementById('vacant-duration')?.value;
                if (vacantDuration) params.duration_s = parseInt(vacantDuration);
                break;
        }

        return params;
    }

    // ── Actions ─────────────────────────────────────────────────────────────────────
    function renderActions(actions) {
        const list = document.getElementById('actions-list');
        if (!actions || actions.length === 0) {
            list.innerHTML = '<p class="text-muted">No actions added</p>';
            return;
        }

        list.innerHTML = actions.map((action, i) => `
            <div class="action-item" data-index="${i}">
                <span class="action-type">${action.type}</span>
                <span class="action-details">${getActionSummary(action)}</span>
                <button class="btn btn-sm btn-icon" onclick="AutomationBuilder.removeAction(${i})">&times;</button>
            </div>
        `).join('');
    }

    function getActionSummary(action) {
        switch (action.type) {
            case 'webhook':
                return action.params?.url || 'No URL';
            case 'mqtt':
                return action.params?.topic || 'No topic';
            case 'ntfy':
                return action.params?.server || 'Default server';
            default:
                return 'Unknown action';
        }
    }

    function openActionModal() {
        document.getElementById('action-type').value = 'webhook';
        onActionTypeChange();
        document.getElementById('action-modal').style.display = 'flex';
    }

    function closeActionModal() {
        document.getElementById('action-modal').style.display = 'none';
    }

    function onActionTypeChange() {
        const type = document.getElementById('action-type').value;
        const form = document.getElementById('action-config-form');

        let html = '';
        switch (type) {
            case 'webhook':
                html = `
                    <label>URL</label>
                    <input type="url" id="action-url" class="form-control" placeholder="https://example.com/webhook">
                    <label>JSON Payload Template (optional)</label>
                    <textarea id="action-payload" class="form-control" rows="3" placeholder='{"text": "{{trigger_name}} fired"}'></textarea>
                    <small class="text-muted">Variables: trigger_name, condition, fired_at, blob_ids</small>
                `;
                break;
            case 'mqtt':
                html = `
                    <label>Topic</label>
                    <input type="text" id="action-topic" class="form-control" placeholder="home/trigger">
                    <label>Payload Template</label>
                    <textarea id="action-payload" class="form-control" rows="3" placeholder='{"state": "ON"}'></textarea>
                `;
                break;
            case 'ntfy':
                html = `
                    <label>Ntfy Server</label>
                    <input type="url" id="action-ntfy-server" class="form-control" placeholder="https://ntfy.sh/mytopic">
                    <label>Message</label>
                    <input type="text" id="action-message" class="form-control" placeholder="{{trigger_name}} fired">
                `;
                break;
        }
        form.innerHTML = html;
    }

    function saveAction() {
        const type = document.getElementById('action-type').value;
        const action = { type, params: {} };

        switch (type) {
            case 'webhook':
                action.params.url = document.getElementById('action-url').value;
                const payload = document.getElementById('action-payload').value;
                if (payload) action.params.template = payload;
                break;
            case 'mqtt':
                action.params.topic = document.getElementById('action-topic').value;
                const mqttPayload = document.getElementById('action-payload').value;
                if (mqttPayload) action.params.template = mqttPayload;
                break;
            case 'ntfy':
                action.params.server = document.getElementById('action-ntfy-server').value;
                action.params.message = document.getElementById('action-message').value;
                break;
        }

        // Add to actions list
        const list = document.getElementById('actions-list');
        const currentCount = list.querySelectorAll('.action-item').length;
        const actionItem = document.createElement('div');
        actionItem.className = 'action-item';
        actionItem.dataset.index = currentCount;
        actionItem.innerHTML = `
            <span class="action-type">${type}</span>
            <span class="action-details">${getActionSummary(action)}</span>
            <button class="btn btn-sm btn-icon" onclick="AutomationBuilder.removeAction(${currentCount})">&times;</button>
        `;
        actionItem.dataset.action = JSON.stringify(action);
        list.appendChild(actionItem);

        closeActionModal();
    }

    function removeAction(index) {
        const item = document.querySelector(`.action-item[data-index="${index}"]`);
        if (item) {
            item.remove();
        }
    }

    function getActionsFromList() {
        const actions = [];
        document.querySelectorAll('.action-item').forEach(item => {
            if (item.dataset.action) {
                actions.push(JSON.parse(item.dataset.action));
            }
        });
        return actions;
    }

    // ── Trigger CRUD ────────────────────────────────────────────────────────────────
    async function loadTriggers() {
        try {
            const res = await fetch('/api/triggers');
            if (res.ok) {
                _triggers = await res.json();
                renderTriggersList();
            }
        } catch (err) {
            console.error('[AutomationBuilder] Failed to load triggers:', err);
        }
    }

    function renderTriggersList() {
        const list = document.getElementById('triggers-list');
        if (!list) return;

        if (_triggers.length === 0) {
            list.innerHTML = `
                <div class="empty-state">
                    <p>No triggers configured yet.</p>
                    <p>Click "Box" or "Cylinder" to draw a volume, then click "New Trigger".</p>
                </div>
            `;
            return;
        }

        list.innerHTML = _triggers.map(t => `
            <div class="trigger-card ${t.enabled ? '' : 'disabled'}" data-id="${t.id}">
                <div class="trigger-header">
                    <h3>
                        ${escapeHtml(t.name)}
                        ${t.error_message ? '<span class="error-badge" title="' + escapeHtml(t.error_message) + '">ERR</span>' : ''}
                        ${!t.enabled && t.error_message ? '<span class="disabled-badge" title="Disabled due to webhook error">DISABLED</span>' : ''}
                        ${t.error_count > 0 && t.enabled ? '<span class="warning-badge" title="' + t.error_count + ' recent error(s)">WARN ' + t.error_count + '</span>' : ''}
                    </h3>
                    <label class="toggle-switch">
                        <input type="checkbox" ${t.enabled ? 'checked' : ''}
                               onchange="AutomationBuilder.toggleEnabled('${t.id}', this.checked)">
                        <span class="toggle-slider"></span>
                    </label>
                </div>
                <div class="trigger-details">
                    <div class="detail-row">
                        <span class="detail-label">Condition:</span>
                        <span class="detail-value">${CONDITION_LABELS[t.condition] || t.condition}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Actions:</span>
                        <span class="detail-value">${t.actions?.length || 0} action(s)</span>
                    </div>
                    ${t.last_fired ? `
                        <div class="detail-row">
                            <span class="detail-label">Last fired:</span>
                            <span class="detail-value">${formatTime(t.last_fired)}</span>
                        </div>
                    ` : ''}
                    ${t.elapsed !== undefined ? `
                        <div class="detail-row">
                            <span class="detail-label">Elapsed:</span>
                            <span class="detail-value">${t.elapsed}s ago</span>
                        </div>
                    ` : ''}
                    ${t.error_message ? `
                        <div class="detail-row error-row">
                            <span class="detail-label">Error:</span>
                            <span class="detail-value">${escapeHtml(t.error_message)}</span>
                        </div>
                    ` : ''}
                </div>
                <div class="trigger-actions">
                    ${!t.enabled && t.error_message ? `
                        <button class="btn btn-sm btn-warning" onclick="AutomationBuilder.reenable('${t.id}')">Re-enable</button>
                    ` : ''}
                    <button class="btn btn-sm btn-secondary" onclick="AutomationBuilder.edit('${t.id}')">Edit</button>
                    <button class="btn btn-sm btn-secondary" onclick="AutomationBuilder.showWebhookLog('${t.id}')">Log</button>
                    <button class="btn btn-sm btn-secondary" onclick="AutomationBuilder.test('${t.id}')">Test</button>
                    <button class="btn btn-sm btn-danger" onclick="AutomationBuilder.delete('${t.id}')">Delete</button>
                </div>
            </div>
        `).join('');
    }

    async function saveTrigger() {
        const name = document.getElementById('trigger-name').value.trim();
        if (!name) {
            alert('Please enter a name for the trigger');
            return;
        }

        if (!_currentVolume) {
            alert('Please draw a volume first by clicking "Box" or "Cylinder"');
            return;
        }

        const trigger = {
            name: name,
            shape: _currentVolume,
            condition: document.getElementById('trigger-condition').value,
            condition_params: getConditionParams(),
            time_constraint: getTimeConstraint(),
            actions: getActionsFromList(),
            enabled: true
        };

        try {
            const url = _editingId ? `/api/triggers/${_editingId}` : '/api/triggers';
            const method = _editingId ? 'PUT' : 'POST';

            const res = await fetch(url, {
                method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(trigger)
            });

            if (!res.ok) {
                const error = await res.text();
                throw new Error(error || 'Failed to save trigger');
            }

            closeTriggerEditor();
            await loadTriggers();
            _currentVolume = null;

            console.log('[AutomationBuilder] Trigger saved successfully');
        } catch (err) {
            console.error('[AutomationBuilder] Failed to save trigger:', err);
            alert('Failed to save trigger: ' + err.message);
        }
    }

    function getTimeConstraint() {
        const from = document.getElementById('time-from').value;
        const to = document.getElementById('time-to').value;

        if (!from && !to) return null;

        return { from: from || '00:00', to: to || '23:59' };
    }

    async function toggleEnabled(id, enabled) {
        try {
            const trigger = _triggers.find(t => t.id === id);
            if (!trigger) return;

            const res = await fetch(`/api/triggers/${id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ...trigger, enabled })
            });

            if (res.ok) {
                await loadTriggers();
            }
        } catch (err) {
            console.error('[AutomationBuilder] Failed to toggle trigger:', err);
        }
    }

    async function test(id) {
        try {
            const res = await fetch(`/api/triggers/${id}/test`, { method: 'POST' });
            const result = await res.json();

            // Show detailed results
            let summary = '';
            if (result.status === 'ok') {
                summary = `Test completed in ${result.response_ms}ms`;
            } else {
                summary = 'Test failed';
            }

            let details = '';
            if (result.actions && result.actions.length > 0) {
                details = result.actions.map(a => {
                    const statusIcon = a.error
                        ? '<span style="color:#f66">FAIL</span>'
                        : '<span style="color:#6c6">OK</span>';
                    const statusText = a.status ? ` (HTTP ${a.status}, ${a.response_ms}ms)` : '';
                    const errorText = a.error ? ` — ${escapeHtml(a.error)}` : '';
                    const urlText = a.url ? ` ${escapeHtml(a.url)}` : '';
                    return `<div style="margin:4px 0">${statusIcon}${statusText}${errorText}${urlText}</div>`;
                }).join('');
            }

            alert(`${summary}\n\n${details}`);
        } catch (err) {
            console.error('[AutomationBuilder] Test failed:', err);
            alert('Test failed: ' + err.message);
        }
    }

    async function reenable(id) {
        try {
            const res = await fetch(`/api/triggers/${id}/enable`, { method: 'POST' });
            if (res.ok) {
                await loadTriggers();
            }
        } catch (err) {
            console.error('[AutomationBuilder] Failed to re-enable trigger:', err);
            alert('Failed to re-enable trigger: ' + err.message);
        }
    }

    async function showWebhookLog(id) {
        try {
            const res = await fetch(`/api/triggers/${id}/webhook-log?limit=20`);
            if (!res.ok) return;

            const entries = await res.json();
            if (!entries || entries.length === 0) {
                alert('No webhook firings recorded for this trigger.');
                return;
            }

            const lines = entries.map(e => {
                const time = new Date(e.fired_at_ms).toLocaleString();
                const status = e.status_code ? `HTTP ${e.status_code}` : 'ERR';
                const latency = e.latency_ms + 'ms';
                const error = e.error ? ` [${e.error}]` : '';
                return `${time} | ${status} | ${latency}${error}`;
            }).join('\n');

            alert(`Webhook Log (last ${entries.length}):\n\n${lines}`);
        } catch (err) {
            console.error('[AutomationBuilder] Failed to load webhook log:', err);
            alert('Failed to load webhook log: ' + err.message);
        }
    }

    async function deleteTrigger(id) {
        if (!confirm('Are you sure you want to delete this trigger?')) return;

        try {
            const res = await fetch(`/api/triggers/${id}`, { method: 'DELETE' });
            if (res.ok) {
                await loadTriggers();
            }
        } catch (err) {
            console.error('[AutomationBuilder] Failed to delete trigger:', err);
            alert('Failed to delete trigger: ' + err.message);
        }
    }

    function edit(id) {
        const trigger = _triggers.find(t => t.id === id);
        if (trigger) {
            openTriggerEditor(trigger);
        }
    }

    function onTriggersChanged(triggers) {
        _triggers = triggers;
        renderTriggersList();
    }

    // ── Trigger Log ────────────────────────────────────────────────────────────────────
    async function loadTriggerLog() {
        try {
            const res = await fetch('/api/triggers/log?limit=10');
            if (res.ok) {
                const log = await res.json();
                renderTriggerLog(log);
            }
        } catch (err) {
            console.error('[AutomationBuilder] Failed to load trigger log:', err);
        }
    }

    function renderTriggerLog(log) {
        const container = document.getElementById('trigger-log');
        if (!container) return;

        if (!log || log.length === 0) {
            container.innerHTML = '<p class="text-muted">No trigger firings yet.</p>';
            return;
        }

        container.innerHTML = log.map(entry => `
            <div class="trigger-log-entry">
                <span class="log-time">${formatTime(entry.fired_at)}</span>
                <span class="log-trigger">${escapeHtml(entry.trigger_name)}</span>
                <span class="log-condition">${entry.condition}</span>
            </div>
        `).join('');

        // Auto-refresh every 30 seconds
        setTimeout(loadTriggerLog, 30000);
    }

    // ── Helpers ───────────────────────────────────────────────────────────────────────
    function escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    function formatTime(timestamp) {
        if (!timestamp) return 'Never';
        const date = new Date(timestamp);
        return date.toLocaleTimeString();
    }

    // ── Open Panel (SpaxelPanels integration) ──────────────────────────────────────────
    function openPanel() {
        console.log('[AutomationBuilder] Opening panel');

        // Ensure panel HTML exists
        const container = document.getElementById('automations-panel');
        if (!container) {
            console.warn('[AutomationBuilder] #automations-panel not found, creating it');
            const panelContainer = document.createElement('div');
            panelContainer.id = 'automations-panel';
            panelContainer.style.display = 'none';
            document.body.appendChild(panelContainer);
            createPanelHTML();
        }

        // Open using SpaxelPanels framework
        if (window.SpaxelPanels) {
            SpaxelPanels.open('automations', {
                title: 'Spatial Triggers',
                content: document.getElementById('automations-panel').innerHTML,
                width: '420px',
                className: 'automations-sidebar',
                onOpen: function() {
                    console.log('[AutomationBuilder] Panel opened, setting up events');
                    setupEventListeners();
                    loadTriggers();

                    // Initialize VolumeEditor integration if not already done
                    if (!_volumeEditorReady && window.VolumeEditor) {
                        VolumeEditor.onVolumeCreated(onVolumeCreated);
                        VolumeEditor.onVolumeChanged(onVolumeChanged);
                        VolumeEditor.onVolumeDeleted(onVolumeDeleted);
                        _volumeEditorReady = true;
                    }
                },
                onClose: function() {
                    console.log('[AutomationBuilder] Panel closed');
                }
            });
        } else {
            // Fallback: show the panel directly
            console.warn('[AutomationBuilder] SpaxelPanels not available, showing panel directly');
            const panel = document.getElementById('automations-panel');
            if (panel) {
                panel.style.display = 'block';
                setupEventListeners();
                loadTriggers();
            }
        }
    }

    // ── Public API ─────────────────────────────────────────────────────────────────────
    window.AutomationBuilder = {
        init,
        open: openPanel,
        loadTriggers,
        renderTriggersList,
        openTriggerEditor,
        closeTriggerEditor,
        saveTrigger,
        toggleEnabled,
        test,
        reenable,
        showWebhookLog,
        delete: deleteTrigger,
        edit,
        removeAction
    };

    // Register with SpaxelPanels
    if (window.SpaxelPanels) {
        SpaxelPanels.register('automations', openPanel);
        console.log('[AutomationBuilder] Registered with SpaxelPanels');
    }

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[AutomationBuilder] Module loaded');
})();
