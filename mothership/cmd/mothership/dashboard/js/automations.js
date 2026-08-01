// AutomationsPanel - Spatial automation builder UI
window.AutomationsPanel = (function() {
    'use strict';

    let automations = [];
    let triggerVolumes = [];
    let zones = [];
    let people = [];
    let editingId = null;
    let testMode = false;

    // Trigger type labels
    const triggerLabels = {
        'zone_enter': 'When someone enters a zone',
        'zone_leave': 'When someone leaves a zone',
        'zone_dwell': 'When someone stays in a zone for a while',
        'zone_vacant': 'When a zone becomes empty',
        'person_count_change': 'When occupant count changes',
        'fall_detected': 'When a fall is detected',
        'anomaly': 'When an anomaly is detected',
        'ble_device_present': 'When a BLE device arrives',
        'ble_device_absent': 'When a BLE device leaves',
        'volume_enter': 'When someone enters a trigger volume',
        'volume_leave': 'When someone leaves a trigger volume'
    };

    // Action type labels
    const actionLabels = {
        'webhook': 'HTTP Webhook',
        'mqtt_publish': 'MQTT Publish',
        'ntfy': 'Ntfy Notification',
        'pushover': 'Pushover Notification'
    };

    // Initialize panel
    function init() {
        createPanelHTML();
        loadInitialData();
        setupEventListeners();
    }

    function createPanelHTML() {
        const container = document.getElementById('automations-panel');
        if (!container) return;

        container.innerHTML = `
            <div class="automations-header">
                <h2>Automations</h2>
                <div class="automations-actions">
                    <button class="btn btn-primary" id="new-automation-btn">
                        <span class="icon">+</span> New Automation
                    </button>
                </div>
            </div>

            <div class="automations-list" id="automations-list">
                <!-- Automations will be loaded here -->
            </div>

            <!-- Automation Editor Modal -->
            <div class="modal" id="automation-modal" style="display: none;">
                <div class="modal-content automation-editor">
                    <div class="modal-header">
                        <h3 id="modal-title">New Automation</h3>
                        <button class="modal-close" id="modal-close">&times;</button>
                    </div>

                    <div class="modal-body">
                        <!-- Step 1: Choose Trigger -->
                        <div class="editor-step" id="step-trigger">
                            <h4>1. Choose Trigger</h4>
                            <select id="trigger-type" class="form-control">
                                <option value="">Select a trigger...</option>
                                ${Object.entries(triggerLabels).map(([k, v]) =>
                                    `<option value="${k}">${v}</option>`
                                ).join('')}
                            </select>
                        </div>

                        <!-- Step 2: Configure Trigger -->
                        <div class="editor-step" id="step-config" style="display: none;">
                            <h4>2. Configure Trigger</h4>
                            <div id="trigger-config-form"></div>
                        </div>

                        <!-- Step 3: Add Conditions -->
                        <div class="editor-step" id="step-conditions" style="display: none;">
                            <h4>3. Add Conditions (Optional)</h4>
                            <div id="conditions-list"></div>
                            <button class="btn btn-secondary btn-sm" id="add-condition-btn">+ Add Condition</button>
                        </div>

                        <!-- Step 4: Add Actions -->
                        <div class="editor-step" id="step-actions" style="display: none;">
                            <h4>4. Add Actions</h4>
                            <div id="actions-list"></div>
                            <button class="btn btn-secondary btn-sm" id="add-action-btn">+ Add Action</button>
                        </div>

                        <!-- Step 5: Summary -->
                        <div class="editor-step" id="step-summary" style="display: none;">
                            <h4>5. Summary</h4>
                            <div class="form-group">
                                <label>Name</label>
                                <input type="text" id="automation-name" class="form-control" placeholder="e.g., Kitchen Light On">
                            </div>
                            <div class="form-group">
                                <label>Cooldown (seconds)</label>
                                <input type="number" id="automation-cooldown" class="form-control" value="60" min="0">
                            </div>
                            <div id="summary-text" class="summary-box"></div>
                        </div>
                    </div>

                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="test-fire-btn">Test Fire</button>
                        <button class="btn btn-secondary" id="modal-cancel">Cancel</button>
                        <button class="btn btn-primary" id="modal-save">Save Automation</button>
                    </div>
                </div>
            </div>

            <!-- Condition Modal -->
            <div class="modal" id="condition-modal" style="display: none;">
                <div class="modal-content modal-sm">
                    <div class="modal-header">
                        <h3>Add Condition</h3>
                        <button class="modal-close" id="condition-modal-close">&times;</button>
                    </div>
                    <div class="modal-body">
                        <div class="form-group">
                            <label>Type</label>
                            <select id="condition-type" class="form-control">
                                <option value="person_filter">Person Filter</option>
                                <option value="time_window">Time Window</option>
                                <option value="day_of_week">Day of Week</option>
                                <option value="system_mode">System Mode</option>
                                <option value="zone_occupancy">Zone Occupancy</option>
                            </select>
                        </div>
                        <div id="condition-value-form"></div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="condition-cancel">Cancel</button>
                        <button class="btn btn-primary" id="condition-save">Add</button>
                    </div>
                </div>
            </div>

            <!-- Action Modal -->
            <div class="modal" id="action-modal" style="display: none;">
                <div class="modal-content">
                    <div class="modal-header">
                        <h3>Add Action</h3>
                        <button class="modal-close" id="action-modal-close">&times;</button>
                    </div>
                    <div class="modal-body">
                        <div class="form-group">
                            <label>Type</label>
                            <select id="action-type" class="form-control">
                                ${Object.entries(actionLabels).map(([k, v]) =>
                                    `<option value="${k}">${v}</option>`
                                ).join('')}
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
        // New automation button
        document.getElementById('new-automation-btn')?.addEventListener('click', () => openEditor());

        // Modal controls
        document.getElementById('modal-close')?.addEventListener('click', closeEditor);
        document.getElementById('modal-cancel')?.addEventListener('click', closeEditor);
        document.getElementById('modal-save')?.addEventListener('click', saveAutomation);
        document.getElementById('test-fire-btn')?.addEventListener('click', testFire);

        // Trigger type change
        document.getElementById('trigger-type')?.addEventListener('change', onTriggerTypeChange);

        // Condition modal
        document.getElementById('add-condition-btn')?.addEventListener('click', openConditionModal);
        document.getElementById('condition-modal-close')?.addEventListener('click', closeConditionModal);
        document.getElementById('condition-cancel')?.addEventListener('click', closeConditionModal);
        document.getElementById('condition-save')?.addEventListener('click', addCondition);
        document.getElementById('condition-type')?.addEventListener('change', onConditionTypeChange);

        // Action modal
        document.getElementById('add-action-btn')?.addEventListener('click', openActionModal);
        document.getElementById('action-modal-close')?.addEventListener('click', closeActionModal);
        document.getElementById('action-cancel')?.addEventListener('click', closeActionModal);
        document.getElementById('action-save')?.addEventListener('click', addAction);
        document.getElementById('action-type')?.addEventListener('change', onActionTypeChange);
    }

    async function loadInitialData() {
        try {
            // Load automations
            const autoRes = await fetch('/api/automations');
            if (autoRes.ok) {
                automations = await autoRes.json();
            }

            // Load zones
            const zonesRes = await fetch('/api/zones');
            if (zonesRes.ok) {
                zones = await zonesRes.json();
            }

            // Load people
            const peopleRes = await fetch('/api/people');
            if (peopleRes.ok) {
                people = await peopleRes.json();
            }

            // Load trigger volumes
            const volumesRes = await fetch('/api/automations/volumes');
            if (volumesRes.ok) {
                triggerVolumes = await volumesRes.json();
            }

            renderAutomationsList();
        } catch (err) {
            console.error('Failed to load data:', err);
        }
    }

    function renderAutomationsList() {
        const list = document.getElementById('automations-list');
        if (!list) return;

        if (automations.length === 0) {
            list.innerHTML = `
                <div class="empty-state">
                    <p>No automations configured yet.</p>
                    <p>Click "New Automation" to create your first automation.</p>
                </div>
            `;
            return;
        }

        list.innerHTML = automations.map(a => `
            <div class="automation-card ${a.enabled ? '' : 'disabled'}" data-id="${a.id}">
                <div class="automation-header">
                    <h3>${escapeHtml(a.name)}</h3>
                    <label class="toggle-switch">
                        <input type="checkbox" ${a.enabled ? 'checked' : ''}
                               onchange="AutomationsPanel.toggleEnabled('${a.id}', this.checked)">
                        <span class="toggle-slider"></span>
                    </label>
                </div>
                <div class="automation-details">
                    <div class="detail-row">
                        <span class="detail-label">Trigger:</span>
                        <span class="detail-value">${triggerLabels[a.trigger_type] || a.trigger_type}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Actions:</span>
                        <span class="detail-value">${a.actions.length} action(s)</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Fired:</span>
                        <span class="detail-value">${a.fire_count || 0} time(s)</span>
                    </div>
                    ${a.last_fired ? `
                        <div class="detail-row">
                            <span class="detail-label">Last fired:</span>
                            <span class="detail-value">${formatTime(a.last_fired)}</span>
                        </div>
                    ` : ''}
                </div>
                <div class="automation-actions">
                    <button class="btn btn-sm btn-secondary" onclick="AutomationsPanel.edit('${a.id}')">Edit</button>
                    <button class="btn btn-sm btn-secondary" onclick="AutomationsPanel.testFire('${a.id}')">Test</button>
                    <button class="btn btn-sm btn-danger" onclick="AutomationsPanel.delete('${a.id}')">Delete</button>
                </div>
            </div>
        `).join('');
    }

    function openEditor(automation = null) {
        editingId = automation?.id || null;

        document.getElementById('modal-title').textContent = editingId ? 'Edit Automation' : 'New Automation';
        document.getElementById('trigger-type').value = automation?.trigger_type || '';
        document.getElementById('automation-name').value = automation?.name || '';
        document.getElementById('automation-cooldown').value = automation?.cooldown || 60;

        // Show/hide steps
        document.getElementById('step-config').style.display = automation ? 'block' : 'none';
        document.getElementById('step-conditions').style.display = automation ? 'block' : 'none';
        document.getElementById('step-actions').style.display = automation ? 'block' : 'none';
        document.getElementById('step-summary').style.display = automation ? 'block' : 'none';

        if (automation) {
            renderTriggerConfig(automation.trigger_type, automation.trigger_config);
            renderConditions(automation.conditions || []);
            renderActions(automation.actions);
            updateSummary();
        } else {
            document.getElementById('conditions-list').innerHTML = '';
            document.getElementById('actions-list').innerHTML = '';
        }

        document.getElementById('automation-modal').style.display = 'flex';
    }

    function closeEditor() {
        document.getElementById('automation-modal').style.display = 'none';
        editingId = null;
    }

    function onTriggerTypeChange() {
        const triggerType = document.getElementById('trigger-type').value;
        if (triggerType) {
            document.getElementById('step-config').style.display = 'block';
            renderTriggerConfig(triggerType);
        } else {
            document.getElementById('step-config').style.display = 'none';
        }
    }

    function renderTriggerConfig(triggerType, config = {}) {
        const form = document.getElementById('trigger-config-form');
        if (!form || !triggerType) return;

        let html = '';

        switch (triggerType) {
            case 'zone_enter':
            case 'zone_leave':
            case 'zone_dwell':
            case 'zone_vacant':
            case 'person_count_change':
                html = `
                    <div class="form-group">
                        <label>Zone</label>
                        <select id="config-zone-id" class="form-control">
                            <option value="">Any Zone</option>
                            ${zones.map(z => `<option value="${z.id}" ${config.zone_id === z.id ? 'selected' : ''}>${escapeHtml(z.name)}</option>`).join('')}
                        </select>
                    </div>
                    <div class="form-group">
                        <label>Person</label>
                        <select id="config-person-id" class="form-control">
                            <option value="anyone">Anyone</option>
                            ${people.map(p => `<option value="${p.id}" ${config.person_id === p.id ? 'selected' : ''}>${escapeHtml(p.name)}</option>`).join('')}
                        </select>
                    </div>
                `;
                if (triggerType === 'zone_dwell') {
                    html += `
                        <div class="form-group">
                            <label>Dwell Time (minutes)</label>
                            <input type="number" id="config-dwell-minutes" class="form-control"
                                   value="${config.dwell_minutes || 5}" min="1">
                        </div>
                    `;
                }
                if (triggerType === 'person_count_change') {
                    html += `
                        <div class="form-group">
                            <label>Count Threshold</label>
                            <input type="number" id="config-count-threshold" class="form-control"
                                   value="${config.count_threshold || 1}" min="0">
                        </div>
                        <div class="form-group">
                            <label>Direction</label>
                            <select id="config-count-direction" class="form-control">
                                <option value="up" ${config.count_direction === 'up' ? 'selected' : ''}>Increasing</option>
                                <option value="down" ${config.count_direction === 'down' ? 'selected' : ''}>Decreasing</option>
                            </select>
                        </div>
                    `;
                }
                break;

            case 'fall_detected':
                html = `
                    <div class="form-group">
                        <label>Zone</label>
                        <select id="config-zone-id" class="form-control">
                            <option value="">Any Zone</option>
                            ${zones.map(z => `<option value="${z.id}" ${config.zone_id === z.id ? 'selected' : ''}>${escapeHtml(z.name)}</option>`).join('')}
                        </select>
                    </div>
                    <div class="form-group">
                        <label>Person</label>
                        <select id="config-person-id" class="form-control">
                            <option value="anyone">Anyone</option>
                            ${people.map(p => `<option value="${p.id}" ${config.person_id === p.id ? 'selected' : ''}>${escapeHtml(p.name)}</option>`).join('')}
                        </select>
                    </div>
                `;
                break;

            case 'ble_device_present':
            case 'ble_device_absent':
                html = `
                    <div class="form-group">
                        <label>Device</label>
                        <select id="config-device-mac" class="form-control">
                            <option value="">Any Device</option>
                            ${people.flatMap(p => (p.devices || []).map(d =>
                                `<option value="${d.mac}" ${config.device_mac === d.mac ? 'selected' : ''}>${escapeHtml(d.name || d.mac)} (${escapeHtml(p.name)})</option>`
                            )).join('')}
                        </select>
                    </div>
                `;
                if (triggerType === 'ble_device_absent') {
                    html += `
                        <div class="form-group">
                            <label>Absent Time (minutes)</label>
                            <input type="number" id="config-absent-minutes" class="form-control"
                                   value="${config.absent_minutes || 15}" min="1">
                        </div>
                    `;
                }
                break;
        }

        form.innerHTML = html;

        // Show next steps
        document.getElementById('step-conditions').style.display = 'block';
        document.getElementById('step-actions').style.display = 'block';
        document.getElementById('step-summary').style.display = 'block';
    }

    function renderConditions(conditions) {
        const list = document.getElementById('conditions-list');
        if (!list) return;

        if (conditions.length === 0) {
            list.innerHTML = '<p class="text-muted">No conditions added.</p>';
            return;
        }

        const conditionLabels = {
            'person_filter': 'Person',
            'time_window': 'Time',
            'day_of_week': 'Days',
            'system_mode': 'Mode',
            'zone_occupancy': 'Zone Occupancy'
        };

        list.innerHTML = conditions.map((c, i) => `
            <div class="condition-item">
                <span>${conditionLabels[c.type] || c.type}: ${escapeHtml(c.value)}</span>
                <button class="btn btn-sm btn-icon" onclick="AutomationsPanel.removeCondition(${i})">&times;</button>
            </div>
        `).join('');
    }

    function openConditionModal() {
        document.getElementById('condition-type').value = 'person_filter';
        onConditionTypeChange();
        document.getElementById('condition-modal').style.display = 'flex';
    }

    function closeConditionModal() {
        document.getElementById('condition-modal').style.display = 'none';
    }

    function onConditionTypeChange() {
        const type = document.getElementById('condition-type').value;
        const form = document.getElementById('condition-value-form');
        if (!form) return;

        switch (type) {
            case 'person_filter':
                form.innerHTML = `
                    <div class="form-group">
                        <label>Person</label>
                        <select id="condition-person" class="form-control">
                            <option value="anyone">Anyone</option>
                            ${people.map(p => `<option value="${p.id}">${escapeHtml(p.name)}</option>`).join('')}
                        </select>
                    </div>
                `;
                break;
            case 'time_window':
                form.innerHTML = `
                    <div class="form-group">
                        <label>Start Time</label>
                        <input type="time" id="condition-time-start" class="form-control" value="22:00">
                    </div>
                    <div class="form-group">
                        <label>End Time</label>
                        <input type="time" id="condition-time-end" class="form-control" value="07:00">
                    </div>
                    <p class="hint">Supports overnight ranges (e.g., 22:00-07:00)</p>
                `;
                break;
            case 'day_of_week':
                form.innerHTML = `
                    <div class="form-group">
                        <label>Days</label>
                        <div class="checkbox-group">
                            <label><input type="checkbox" value="0"> Sun</label>
                            <label><input type="checkbox" value="1"> Mon</label>
                            <label><input type="checkbox" value="2"> Tue</label>
                            <label><input type="checkbox" value="3"> Wed</label>
                            <label><input type="checkbox" value="4"> Thu</label>
                            <label><input type="checkbox" value="5"> Fri</label>
                            <label><input type="checkbox" value="6"> Sat</label>
                        </div>
                    </div>
                `;
                break;
            case 'system_mode':
                form.innerHTML = `
                    <div class="form-group">
                        <label>Mode</label>
                        <select id="condition-mode" class="form-control">
                            <option value="home">Home</option>
                            <option value="away">Away</option>
                            <option value="sleep">Sleep</option>
                        </select>
                    </div>
                `;
                break;
            case 'zone_occupancy':
                form.innerHTML = `
                    <div class="form-group">
                        <label>Zone</label>
                        <select id="condition-occ-zone" class="form-control">
                            ${zones.map(z => `<option value="${z.id}">${escapeHtml(z.name)}</option>`).join('')}
                        </select>
                    </div>
                    <div class="form-group">
                        <label>Condition</label>
                        <select id="condition-occ-op" class="form-control">
                            <option value="lt">Less than</option>
                            <option value="lte">Less than or equal</option>
                            <option value="eq">Equal to</option>
                            <option value="gte">Greater than or equal</option>
                            <option value="gt">Greater than</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>Count</label>
                        <input type="number" id="condition-occ-count" class="form-control" value="1" min="0">
                    </div>
                `;
                break;
        }
    }

    function addCondition() {
        const type = document.getElementById('condition-type').value;
        let value = '';

        switch (type) {
            case 'person_filter':
                value = document.getElementById('condition-person').value;
                break;
            case 'time_window':
                const start = document.getElementById('condition-time-start').value;
                const end = document.getElementById('condition-time-end').value;
                value = `${start}-${end}`;
                break;
            case 'day_of_week':
                const days = [];
                document.querySelectorAll('#condition-value-form input:checked').forEach(cb => {
                    days.push(cb.value);
                });
                value = days.join(',');
                break;
            case 'system_mode':
                value = document.getElementById('condition-mode').value;
                break;
            case 'zone_occupancy':
                const zone = document.getElementById('condition-occ-zone').value;
                const op = document.getElementById('condition-occ-op').value;
                const count = document.getElementById('condition-occ-count').value;
                value = `${zone}:${op}:${count}`;
                break;
        }

        // Add to current automation being edited
        const conditionsList = document.getElementById('conditions-list');
        const conditionItem = document.createElement('div');
        conditionItem.className = 'condition-item';
        conditionItem.innerHTML = `
            <span>${type}: ${escapeHtml(value)}</span>
            <button class="btn btn-sm btn-icon" onclick="this.parentElement.remove(); AutomationsPanel.updateSummary();">&times;</button>
        `;
        conditionsList.appendChild(conditionItem);

        closeConditionModal();
        updateSummary();
    }

    function renderActions(actions) {
        const list = document.getElementById('actions-list');
        if (!list) return;

        if (actions.length === 0) {
            list.innerHTML = '<p class="text-muted">No actions added.</p>';
            return;
        }

        list.innerHTML = actions.map((a, i) => `
            <div class="action-item">
                <span>${actionLabels[a.type] || a.type}</span>
                <small>${escapeHtml(a.url || a.topic || a.server || '')}</small>
                <button class="btn btn-sm btn-icon" onclick="AutomationsPanel.removeAction(${i})">&times;</button>
            </div>
        `).join('');
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
        if (!form) return;

        switch (type) {
            case 'webhook':
                form.innerHTML = `
                    <div class="form-group">
                        <label>URL</label>
                        <input type="url" id="action-url" class="form-control" placeholder="https://example.com/webhook">
                    </div>
                    <div class="form-group">
                        <label>Payload Template (optional)</label>
                        <textarea id="action-template" class="form-control" rows="3"
                            placeholder='{"text": "{{person_name}} entered {{zone_name}}"}'></textarea>
                        <p class="hint">Variables: {{person_name}}, {{zone_name}}, {{from_zone}}, {{to_zone}}, {{timestamp}}, {{occupant_count}}, {{event_type}}</p>
                    </div>
                `;
                break;
            case 'mqtt_publish':
                form.innerHTML = `
                    <div class="form-group">
                        <label>Topic</label>
                        <input type="text" id="action-topic" class="form-control" placeholder="home/living_room/light">
                    </div>
                    <div class="form-group">
                        <label>Payload Template</label>
                        <textarea id="action-template" class="form-control" rows="3"
                            placeholder='{"state": "ON"}'></textarea>
                    </div>
                `;
                break;
            case 'ntfy':
                form.innerHTML = `
                    <div class="form-group">
                        <label>Ntfy Server URL</label>
                        <input type="url" id="action-server" class="form-control" placeholder="https://ntfy.sh/mytopic">
                    </div>
                    <div class="form-group">
                        <label>Message Template (optional)</label>
                        <textarea id="action-template" class="form-control" rows="2"
                            placeholder="{{person_name}} triggered {{event_type}}"></textarea>
                    </div>
                `;
                break;
            case 'pushover':
                form.innerHTML = `
                    <div class="form-group">
                        <label>Application Token</label>
                        <input type="text" id="action-token" class="form-control">
                    </div>
                    <div class="form-group">
                        <label>User Key</label>
                        <input type="text" id="action-user-key" class="form-control">
                    </div>
                    <div class="form-group">
                        <label>Message Template (optional)</label>
                        <textarea id="action-template" class="form-control" rows="2"></textarea>
                    </div>
                `;
                break;
        }
    }

    function addAction() {
        const type = document.getElementById('action-type').value;
        const action = { type };

        switch (type) {
            case 'webhook':
                action.url = document.getElementById('action-url').value;
                action.template = document.getElementById('action-template').value;
                break;
            case 'mqtt_publish':
                action.topic = document.getElementById('action-topic').value;
                action.template = document.getElementById('action-template').value;
                break;
            case 'ntfy':
                action.server = document.getElementById('action-server').value;
                action.template = document.getElementById('action-template').value;
                break;
            case 'pushover':
                action.token = document.getElementById('action-token').value;
                action.user_key = document.getElementById('action-user-key').value;
                action.template = document.getElementById('action-template').value;
                break;
        }

        const actionsList = document.getElementById('actions-list');
        const actionItem = document.createElement('div');
        actionItem.className = 'action-item';
        actionItem.innerHTML = `
            <span>${actionLabels[type]}</span>
            <small>${escapeHtml(action.url || action.topic || action.server || '')}</small>
            <button class="btn btn-sm btn-icon" onclick="this.parentElement.remove(); AutomationsPanel.updateSummary();">&times;</button>
        `;
        actionItem.dataset.action = JSON.stringify(action);
        actionsList.appendChild(actionItem);

        closeActionModal();
        updateSummary();
    }

    function updateSummary() {
        const triggerType = document.getElementById('trigger-type').value;
        const name = document.getElementById('automation-name').value || 'Untitled';

        const conditions = getConditionsFromList();
        const actions = getActionsFromList();

        const summaryText = document.getElementById('summary-text');
        if (summaryText) {
            summaryText.innerHTML = `
                <p><strong>"${escapeHtml(name)}"</strong></p>
                <p>When: <em>${triggerLabels[triggerType] || triggerType}</em></p>
                ${conditions.length > 0 ? `<p>Conditions: ${conditions.length} filter(s)</p>` : ''}
                <p>Then: ${actions.length} action(s)</p>
            `;
        }
    }

    function getConditionsFromList() {
        const conditions = [];
        document.querySelectorAll('#conditions-list .condition-item').forEach(item => {
            const text = item.querySelector('span').textContent;
            const parts = text.split(': ');
            if (parts.length >= 2) {
                conditions.push({
                    type: parts[0].trim(),
                    value: parts.slice(1).join(': ').trim()
                });
            }
        });
        return conditions;
    }

    function getActionsFromList() {
        const actions = [];
        document.querySelectorAll('#actions-list .action-item').forEach(item => {
            if (item.dataset.action) {
                actions.push(JSON.parse(item.dataset.action));
            }
        });
        return actions;
    }

    function getTriggerConfig() {
        const triggerType = document.getElementById('trigger-type').value;
        const config = {};

        const zoneEl = document.getElementById('config-zone-id');
        const personEl = document.getElementById('config-person-id');
        const deviceEl = document.getElementById('config-device-mac');

        if (zoneEl) config.zone_id = zoneEl.value;
        if (personEl) config.person_id = personEl.value;
        if (deviceEl) config.device_mac = deviceEl.value;

        const dwellEl = document.getElementById('config-dwell-minutes');
        const absentEl = document.getElementById('config-absent-minutes');
        const countThresholdEl = document.getElementById('config-count-threshold');
        const countDirEl = document.getElementById('config-count-direction');

        if (dwellEl) config.dwell_minutes = parseInt(dwellEl.value) || 5;
        if (absentEl) config.absent_minutes = parseInt(absentEl.value) || 15;
        if (countThresholdEl) config.count_threshold = parseInt(countThresholdEl.value) || 1;
        if (countDirEl) config.count_direction = countDirEl.value;

        return config;
    }

    async function saveAutomation() {
        const automation = {
            id: editingId || `auto_${Date.now()}`,
            name: document.getElementById('automation-name').value || 'Untitled',
            enabled: true,
            trigger_type: document.getElementById('trigger-type').value,
            trigger_config: getTriggerConfig(),
            conditions: getConditionsFromList(),
            actions: getActionsFromList(),
            cooldown: parseInt(document.getElementById('automation-cooldown').value) || 60
        };

        try {
            const url = '/api/automations' + (editingId ? `/${editingId}` : '');
            const method = editingId ? 'PUT' : 'POST';

            const res = await fetch(url, {
                method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(automation)
            });

            if (!res.ok) throw new Error('Failed to save');

            closeEditor();
            await loadInitialData();
        } catch (err) {
            console.error('Failed to save automation:', err);
            alert('Failed to save automation: ' + err.message);
        }
    }

    async function testFire(id) {
        try {
            const res = await fetch(`/api/automations/${id}/test`, { method: 'POST' });
            if (!res.ok) throw new Error('Test fire failed');
            alert('Test fire sent!');
        } catch (err) {
            console.error('Test fire failed:', err);
            alert('Test fire failed: ' + err.message);
        }
    }

    async function toggleEnabled(id, enabled) {
        try {
            const automation = automations.find(a => a.id === id);
            if (!automation) return;

            automation.enabled = enabled;

            const res = await fetch(`/api/automations/${id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(automation)
            });

            if (!res.ok) throw new Error('Failed to update');

            renderAutomationsList();
        } catch (err) {
            console.error('Failed to toggle automation:', err);
        }
    }

    function edit(id) {
        const automation = automations.find(a => a.id === id);
        if (automation) {
            openEditor(automation);
        }
    }

    async function deleteAutomation(id) {
        if (!confirm('Are you sure you want to delete this automation?')) return;

        try {
            const res = await fetch(`/api/automations/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error('Failed to delete');

            await loadInitialData();
        } catch (err) {
            console.error('Failed to delete automation:', err);
            alert('Failed to delete automation: ' + err.message);
        }
    }

    function removeCondition(index) {
        const conditions = getConditionsFromList();
        conditions.splice(index, 1);
        renderConditions(conditions);
        updateSummary();
    }

    function removeAction(index) {
        const actions = getActionsFromList();
        actions.splice(index, 1);
        renderActions(actions);
        updateSummary();
    }

    // Helper functions
    function escapeHtml(str) {
        if (!str) return '';
        return str.replace(/&/g, '&amp;')
                  .replace(/</g, '&lt;')
                  .replace(/>/g, '&gt;')
                  .replace(/"/g, '&quot;');
    }

    function formatTime(timestamp) {
        if (!timestamp) return 'Never';
        const date = new Date(timestamp);
        return date.toLocaleString();
    }

    // Public API
    return {
        init,
        edit,
        delete: deleteAutomation,
        toggleEnabled,
        testFire,
        removeCondition,
        removeAction,
        updateSummary
    };
})();
