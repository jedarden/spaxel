/**
 * Spaxel Dashboard - BLE Device Panel (Phase 6)
 *
 * Provides UI for managing BLE device registry and identity matching.
 */

(function() {
    'use strict';

    // State
    const state = {
        devices: new Map(), // addr -> device record
        matches: new Map(), // blobID -> identity match
        aliases: new Map(), // addr -> list of aliases
        duplicates: [],     // possible duplicate devices
        expanded: false,
        selectedDevice: null,
        editingDevice: null,
        wsConnected: false
    };

    // DOM elements
    let panelEl, listEl, headerEl, countEl;

    // Initialize the panel
    function init() {
        createPanel();
        startPolling();
        console.log('[BLE Panel] Initialized');
    }

    // Create the panel DOM structure
    function createPanel() {
        // Find or create panel container
        let container = document.getElementById('ble-panel');
        if (!container) {
            container = document.createElement('div');
            container.id = 'ble-panel';
            container.className = 'side-panel';
            document.body.appendChild(container);
        }

        container.innerHTML = `
            <div class="panel-header" id="ble-panel-header">
                <span class="panel-title">
                    <span class="panel-icon">👤</span>
                    People & Devices
                </span>
                <span class="panel-count" id="ble-device-count">0</span>
                <button class="panel-toggle" id="ble-panel-toggle">▼</button>
            </div>
            <div class="panel-content" id="ble-panel-content" style="display: none;">
                <div class="panel-section">
                    <div class="section-header">
                        <span>People</span>
                        <button class="btn-small" id="ble-add-person">+ Add</button>
                    </div>
                    <div id="ble-people-list" class="device-list"></div>
                </div>
                <div class="panel-section">
                    <div class="section-header">
                        <span>Discovered Devices</span>
                        <span class="section-info" id="ble-discovered-count">0</span>
                    </div>
                    <div id="ble-devices-list" class="device-list"></div>
                </div>
                <div class="panel-section" id="duplicates-section" style="display: none;">
                    <div class="section-header">
                        <span>Possible Rotations</span>
                        <span class="section-info">🔄</span>
                    </div>
                    <div id="ble-duplicates-list" class="duplicates-list"></div>
                </div>
                <div class="panel-section">
                    <div class="section-header">
                        <span>Recent Crossings</span>
                    </div>
                    <div id="ble-crossings-list" class="crossing-list"></div>
                </div>
            </div>
            <div class="device-modal" id="ble-device-modal" style="display: none;">
                <div class="modal-content">
                    <div class="modal-header">
                        <span id="modal-title">Edit Device</span>
                        <button class="modal-close" id="modal-close">&times;</button>
                    </div>
                    <div class="modal-body">
                        <div class="form-group">
                            <label>Name</label>
                            <input type="text" id="modal-name" placeholder="e.g., Alice's Phone">
                        </div>
                        <div class="form-group">
                            <label>Label</label>
                            <input type="text" id="modal-label" placeholder="e.g., Alice">
                        </div>
                        <div class="form-group">
                            <label>Color</label>
                            <input type="color" id="modal-color" value="#4fc3f7">
                        </div>
                        <div class="form-group">
                            <label>
                                <input type="checkbox" id="modal-is-person">
                                This device represents a person
                            </label>
                        </div>
                        <div class="form-group">
                            <label>Device Type</label>
                            <select id="modal-device-type">
                                <option value="unknown">Unknown</option>
                                <option value="phone">Phone</option>
                                <option value="watch">Watch</option>
                                <option value="tracker">Tracker</option>
                                <option value="tablet">Tablet</option>
                                <option value="laptop">Laptop</option>
                                <option value="headphones">Headphones</option>
                                <option value="other">Other</option>
                            </select>
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn-secondary" id="modal-cancel">Cancel</button>
                        <button class="btn-primary" id="modal-save">Save</button>
                    </div>
                </div>
            </div>
            <div class="device-modal" id="ble-merge-modal" style="display: none;">
                <div class="modal-content">
                    <div class="modal-header">
                        <span id="merge-modal-title">Merge Devices</span>
                        <button class="modal-close" id="merge-modal-close">&times;</button>
                    </div>
                    <div class="modal-body">
                        <div class="merge-info">
                            <p>These two devices appear to be the same device with a rotated MAC address:</p>
                            <div class="merge-devices">
                                <div class="merge-device" id="merge-device-1">
                                    <span class="merge-mac"></span>
                                    <span class="merge-name"></span>
                                </div>
                                <div class="merge-arrow">↓</div>
                                <div class="merge-device" id="merge-device-2">
                                    <span class="merge-mac"></span>
                                    <span class="merge-name"></span>
                                </div>
                            </div>
                            <p class="merge-explanation">Merging will keep the first device and remove the second. All identity associations will be preserved.</p>
                            <p class="merge-confirmation">Are these the same device? Only merge if you're certain.</p>
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn-secondary" id="merge-modal-cancel">Cancel</button>
                        <button class="btn-primary btn-danger" id="merge-modal-confirm">Yes, Merge</button>
                    </div>
                </div>
            </div>
        `;

        panelEl = container;
        headerEl = document.getElementById('ble-panel-header');
        listEl = document.getElementById('ble-panel-content');
        countEl = document.getElementById('ble-device-count');

        // Event listeners
        headerEl.addEventListener('click', togglePanel);
        document.getElementById('ble-add-person').addEventListener('click', showAddPersonModal);
        document.getElementById('modal-close').addEventListener('click', hideModal);
        document.getElementById('modal-cancel').addEventListener('click', hideModal);
        document.getElementById('modal-save').addEventListener('click', saveDevice);

        // Merge modal event listeners
        document.getElementById('merge-modal-close').addEventListener('click', hideMergeModal);
        document.getElementById('merge-modal-cancel').addEventListener('click', hideMergeModal);
        document.getElementById('merge-modal-confirm').addEventListener('click', confirmMerge);
    }

    // Toggle panel expansion
    function togglePanel() {
        state.expanded = !state.expanded;
        listEl.style.display = state.expanded ? 'block' : 'none';
        document.getElementById('ble-panel-toggle').textContent = state.expanded ? '▲' : '▼';
    }

    // Start polling for data
    function startPolling() {
        fetchDevices();
        fetchMatches();
        fetchCrossings();
        fetchDuplicates();

        setInterval(fetchDevices, 10000);
        setInterval(fetchMatches, 5000);
        setInterval(fetchCrossings, 15000);
        setInterval(fetchDuplicates, 30000);
    }

    // Fetch BLE devices
    function fetchDevices() {
        fetch('/api/ble/devices')
            .then(function(res) { return res.json(); })
            .then(function(data) {
                handleDevicesUpdate(data || []);
            })
            .catch(function(err) {
                console.error('[BLE Panel] Failed to fetch devices:', err);
            });
    }

    // Fetch identity matches
    function fetchMatches() {
        fetch('/api/ble/matches')
            .then(function(res) { return res.json(); })
            .then(function(data) {
                handleMatchesUpdate(data || []);
            })
            .catch(function(err) {
                console.error('[BLE Panel] Failed to fetch matches:', err);
            });
    }

    // Fetch recent crossings
    function fetchCrossings() {
        fetch('/api/zones/crossings?limit=10')
            .then(function(res) { return res.json(); })
            .then(function(data) {
                handleCrossingsUpdate(data || []);
            })
            .catch(function(err) {
                // Silently ignore - zones may not be configured
            });
    }

    // Fetch possible duplicate devices (for MAC rotation)
    function fetchDuplicates() {
        fetch('/api/ble/duplicates')
            .then(function(res) { return res.json(); })
            .then(function(data) {
                state.duplicates = data.duplicates || [];
                updateDuplicatesList();
            })
            .catch(function(err) {
                console.error('[BLE Panel] Failed to fetch duplicates:', err);
            });
    }

    // Fetch device aliases
    function fetchDeviceAliases(addr) {
        fetch('/api/ble/devices/' + encodeURIComponent(addr) + '/aliases')
            .then(function(res) { return res.json(); })
            .then(function(data) {
                state.aliases.set(addr, data);
                updateDeviceList(); // Refresh to show rotation icon
            })
            .catch(function(err) {
                // Endpoint may not exist yet
                console.error('[BLE Panel] Failed to fetch aliases:', err);
            });
    }

    // Handle devices update
    function handleDevicesUpdate(devices) {
        state.devices.clear();
        devices.forEach(function(d) {
            state.devices.set(d.addr, d);
        });

        updateDeviceList();
        countEl.textContent = devices.filter(function(d) { return d.is_person; }).length;
    }

    // Handle identity matches update
    function handleMatchesUpdate(matches) {
        state.matches.clear();
        matches.forEach(function(m) {
            state.matches.set(m.blob_id, m);
        });

        // Update 3D visualization
        if (window.Viz3D && window.Viz3D.updateIdentities) {
            Viz3D.updateIdentities(matches);
        }
    }

    // Handle crossings update
    function handleCrossingsUpdate(crossings) {
        var list = document.getElementById('ble-crossings-list');
        if (!crossings || crossings.length === 0) {
            list.innerHTML = '<div class="empty-state">No recent crossings</div>';
            return;
        }

        var html = '';
        crossings.forEach(function(c) {
            var time = formatTime(new Date(c.timestamp));
            var identity = c.identity || 'Unknown';
            var direction = c.direction > 0 ? '→' : '←';
            html += '<div class="crossing-item">' +
                '<span class="crossing-time">' + time + '</span>' +
                '<span class="crossing-identity">' + identity + '</span>' +
                '<span class="crossing-portal">' + direction + ' Portal</span>' +
                '</div>';
        });
        list.innerHTML = html;
    }

    // Update duplicates list
    function updateDuplicatesList() {
        var section = document.getElementById('duplicates-section');
        var list = document.getElementById('ble-duplicates-list');

        if (!state.duplicates || state.duplicates.length === 0) {
            section.style.display = 'none';
            return;
        }

        section.style.display = 'block';
        var html = '';
        state.duplicates.forEach(function(dup) {
            html += '<div class="duplicate-item" data-mac1="' + dup.mac1 + '" data-mac2="' + dup.mac2 + '">' +
                '<div class="duplicate-header">' +
                    '<span class="duplicate-reason">' + dup.reason + '</span>' +
                    '<span class="duplicate-confidence">' + Math.round(dup.confidence * 100) + '% match</span>' +
                '</div>' +
                '<div class="duplicate-devices">' +
                    '<span class="dup-mac">' + dup.mac1.substr(-8) + '</span>' +
                    '<span class="dup-arrow">↔</span>' +
                    '<span class="dup-mac">' + dup.mac2.substr(-8) + '</span>' +
                '</div>' +
                '<div class="duplicate-actions">' +
                    '<button class="btn-small btn-merge" data-mac1="' + dup.mac1 + '" data-mac2="' + dup.mac2 + '">Merge</button>' +
                    '<button class="btn-small btn-dismiss" data-mac1="' + dup.mac1 + '" data-mac2="' + dup.mac2 + '">Dismiss</button>' +
                '</div>' +
            '</div>';
        });
        list.innerHTML = html;

        // Add event listeners
        list.querySelectorAll('.btn-merge').forEach(function(btn) {
            btn.addEventListener('click', function(e) {
                e.stopPropagation();
                var mac1 = this.getAttribute('data-mac1');
                var mac2 = this.getAttribute('data-mac2');
                showMergeConfirm(mac1, mac2);
            });
        });

        list.querySelectorAll('.btn-dismiss').forEach(function(btn) {
            btn.addEventListener('click', function(e) {
                e.stopPropagation();
                var item = this.closest('.duplicate-item');
                item.style.display = 'none';
                // Remove from state
                state.duplicates = state.duplicates.filter(function(d) {
                    return d.mac1 !== item.getAttribute('data-mac1') || d.mac2 !== item.getAttribute('data-mac2');
                });
                if (state.duplicates.length === 0) {
                    section.style.display = 'none';
                }
            });
        });
    }

    // Update device list UI
    function updateDeviceList() {
        var peopleList = document.getElementById('ble-people-list');
        var devicesList = document.getElementById('ble-devices-list');

        var people = [];
        var otherDevices = [];

        state.devices.forEach(function(d) {
            if (d.is_person) {
                people.push(d);
            } else {
                otherDevices.push(d);
            }
        });

        // Sort people by name
        people.sort(function(a, b) { return (a.name || '').localeCompare(b.name || ''); });
        otherDevices.sort(function(a, b) { return (a.device_name || a.addr).localeCompare(b.device_name || b.addr); });

        // Update people list
        if (people.length === 0) {
            peopleList.innerHTML = '<div class="empty-state">No people configured</div>';
        } else {
            var html = '';
            people.forEach(function(p) {
                var color = p.color || '#4fc3f7';
                var loc = p.last_location || {};
                var locStr = '';
                if (loc.confidence > 0) {
                    locStr = '<span class="device-loc">📍</span>';
                }
                var aliasData = state.aliases.get(p.addr);
                var hasAliases = aliasData && aliasData.alias_count > 0;
                var rotationIcon = hasAliases ? '<span class="rotation-icon" title="Has address rotation history">🔄</span>' : '';

                html += '<div class="device-item person" data-addr="' + p.addr + '">' +
                    '<span class="device-color" style="background:' + color + '"></span>' +
                    '<span class="device-name">' + (p.name || p.label || 'Unknown') + '</span>' +
                    rotationIcon +
                    locStr +
                    '<button class="device-expand" data-addr="' + p.addr + '">▼</button>' +
                    '<button class="device-edit" data-addr="' + p.addr + '">✏️</button>' +
                    '</div>';

                // Add aliases section if expanded
                if (hasAliases && p.expanded) {
                    html += '<div class="device-aliases" data-parent="' + p.addr + '">';
                    html += '<div class="aliases-header">Address History</div>';
                    aliasData.aliases.forEach(function(alias) {
                        var age = formatTime(new Date(alias.last_seen));
                        html += '<div class="alias-item">' +
                            '<span class="alias-addr">' + alias.addr + '</span>' +
                            '<span class="alias-age">' + age + '</span>' +
                        '</div>';
                    });
                    html += '<div class="aliases-note">Phones rotate addresses every 15-30 min. All entries above are the same device.</div>';
                    html += '</div>';
                }
            });
            peopleList.innerHTML = html;

            // Add click handlers
            peopleList.querySelectorAll('.device-edit').forEach(function(btn) {
                btn.addEventListener('click', function(e) {
                    e.stopPropagation();
                    var addr = this.getAttribute('data-addr');
                    showEditModal(addr);
                });
            });

            peopleList.querySelectorAll('.device-expand').forEach(function(btn) {
                btn.addEventListener('click', function(e) {
                    e.stopPropagation();
                    var addr = this.getAttribute('data-addr');
                    toggleDeviceExpanded(addr);
                });
            });

            // Make device items clickable to expand
            peopleList.querySelectorAll('.device-item.person').forEach(function(item) {
                item.addEventListener('click', function(e) {
                    if (!e.target.classList.contains('device-edit') && !e.target.classList.contains('device-expand')) {
                        var addr = this.getAttribute('data-addr');
                        toggleDeviceExpanded(addr);
                        // Fetch aliases when expanding
                        if (!state.aliases.has(addr)) {
                            fetchDeviceAliases(addr);
                        }
                    }
                });
            });
        }

        // Update devices list
        document.getElementById('ble-discovered-count').textContent = otherDevices.length;

        if (otherDevices.length === 0) {
            devicesList.innerHTML = '<div class="empty-state">No devices discovered</div>';
        } else {
            var html = '';
            otherDevices.slice(0, 10).forEach(function(d) {
                var deviceName = d.device_name || d.addr.substr(-5);
                var typeIcon = getTypeIcon(d.device_type);
                html += '<div class="device-item" data-addr="' + d.addr + '">' +
                    '<span class="device-icon">' + typeIcon + '</span>' +
                    '<span class="device-name">' + deviceName + '</span>' +
                    '<button class="device-edit" data-addr="' + d.addr + '">+</button>' +
                    '</div>';
            });
            if (otherDevices.length > 10) {
                html += '<div class="more-link">+ ' + (otherDevices.length - 10) + ' more</div>';
            }
            devicesList.innerHTML = html;

            // Add click handlers
            devicesList.querySelectorAll('.device-edit').forEach(function(btn) {
                btn.addEventListener('click', function(e) {
                    e.stopPropagation();
                    var addr = this.getAttribute('data-addr');
                    showEditModal(addr);
                });
            });
        }
    }

    // Toggle device expanded state
    function toggleDeviceExpanded(addr) {
        var device = state.devices.get(addr);
        if (device) {
            device.expanded = !device.expanded;
            updateDeviceList();
        }
    }

    // Show add person modal
    function showAddPersonModal() {
        state.editingDevice = null;
        document.getElementById('modal-title').textContent = 'Add Person';
        document.getElementById('modal-name').value = '';
        document.getElementById('modal-label').value = '';
        document.getElementById('modal-color').value = '#4fc3f7';
        document.getElementById('modal-is-person').checked = true;
        document.getElementById('modal-device-type').value = 'phone';
        document.getElementById('ble-device-modal').style.display = 'flex';
    }

    // Show edit modal
    function showEditModal(addr) {
        var device = state.devices.get(addr);
        if (!device) return;

        state.editingDevice = addr;
        document.getElementById('modal-title').textContent = 'Edit Device';
        document.getElementById('modal-name').value = device.name || '';
        document.getElementById('modal-label').value = device.label || '';
        document.getElementById('modal-color').value = device.color || '#4fc3f7';
        document.getElementById('modal-is-person').checked = device.is_person;
        document.getElementById('modal-device-type').value = device.device_type || 'unknown';
        document.getElementById('ble-device-modal').style.display = 'flex';
    }

    // Hide modal
    function hideModal() {
        document.getElementById('ble-device-modal').style.display = 'none';
        state.editingDevice = null;
    }

    // Save device
    function saveDevice() {
        var data = {
            name: document.getElementById('modal-name').value,
            label: document.getElementById('modal-label').value,
            color: document.getElementById('modal-color').value,
            is_person: document.getElementById('modal-is-person').checked,
            device_type: document.getElementById('modal-device-type').value,
            enabled: true
        };

        var addr = state.editingDevice || 'new-' + Date.now();
        var url = '/api/ble/devices/' + encodeURIComponent(addr);
        var method = state.editingDevice ? 'PUT' : 'POST';

        fetch(url, {
            method: method,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        })
        .then(function(res) {
            if (res.ok) {
                hideModal();
                fetchDevices();
            } else {
                return res.json().then(function(err) {
                    throw new Error(err.error || 'Failed to save');
                });
            }
        })
        .catch(function(err) {
            alert('Failed to save device: ' + err.message);
        });
    }

    // Show merge confirmation modal
    function showMergeConfirm(mac1, mac2) {
        state.pendingMerge = { mac1: mac1, mac2: mac2 };

        var device1 = state.devices.get(mac1);
        var device2 = state.devices.get(mac2);

        document.getElementById('merge-device-1').querySelector('.merge-mac').textContent = mac1;
        document.getElementById('merge-device-1').querySelector('.merge-name').textContent =
            device1 ? (device1.name || device1.device_name || 'Unknown') : 'Unknown';
        document.getElementById('merge-device-2').querySelector('.merge-mac').textContent = mac2;
        document.getElementById('merge-device-2').querySelector('.merge-name').textContent =
            device2 ? (device2.name || device2.device_name || 'Unknown') : 'Unknown';

        document.getElementById('ble-merge-modal').style.display = 'flex';
    }

    // Hide merge modal
    function hideMergeModal() {
        document.getElementById('ble-merge-modal').style.display = 'none';
        state.pendingMerge = null;
    }

    // Confirm and execute merge
    function confirmMerge() {
        if (!state.pendingMerge) {
            return;
        }

        var mac1 = state.pendingMerge.mac1;
        var mac2 = state.pendingMerge.mac2;

        fetch('/api/ble/merge', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac1: mac1, mac2: mac2 })
        })
        .then(function(res) {
            if (res.ok) {
                return res.json();
            } else {
                return res.json().then(function(err) {
                    throw new Error(err.error || 'Failed to merge');
                });
            }
        })
        .then(function(data) {
            hideMergeModal();
            // Remove from duplicates list
            state.duplicates = state.duplicates.filter(function(d) {
                return d.mac1 !== mac1 || d.mac2 !== mac2;
            });
            updateDuplicatesList();
            fetchDevices();
        })
        .catch(function(err) {
            alert('Failed to merge devices: ' + err.message);
        });
    }

    // Get icon for device type
    function getTypeIcon(type) {
        switch (type) {
            case 'phone': return '📱';
            case 'watch': return '⌚';
            case 'tracker': return '📍';
            case 'tablet': return '📱';
            case 'laptop': return '💻';
            case 'headphones': return '🎧';
            default: return '📡';
        }
    }

    // Format time relative to now
    function formatTime(date) {
        var now = new Date();
        var diff = (now - date) / 1000;

        if (diff < 60) return 'just now';
        if (diff < 3600) return Math.floor(diff / 60) + 'm ago';
        if (diff < 86400) return Math.floor(diff / 3600) + 'h ago';
        return date.toLocaleDateString();
    }

    // Export public interface
    window.BLEPanel = {
        init: init,
        updateMatches: handleMatchesUpdate,
        updateDevices: handleDevicesUpdate
    };

})();
