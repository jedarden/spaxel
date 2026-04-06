/**
 * Spaxel Dashboard - BLE Device Panel (Phase 6)
 *
 * People & Devices panel for discovering, registering, and labeling BLE devices.
 * Uses the SpaxelPanels framework for integration.
 */

(function() {
    'use strict';

    // ============================================
    // State
    // ============================================
    const state = {
        devices: [],
        registeredDevices: [],
        discoveredDevices: [],
        people: [],
        selectedDevice: null,
        loading: false,
        error: null,
        filters: {
            showRegistered: true,
            showDiscovered: true
        },
        // Auto-type detection hints
        typeHints: {
            'apple_phone': { icon: '📱', label: 'iPhone' },
            'apple_watch': { icon: '⌚', label: 'Apple Watch' },
            'apple_earbuds': { icon: '🎧', label: 'AirPods' },
            'fitbit': { icon: '⌚', label: 'Fitbit' },
            'garmin': { icon: '⌚', label: 'Garmin' },
            'tile': { icon: '📍', label: 'Tile Tracker' },
            'microsoft': { icon: '💻', label: 'Microsoft' },
            'samsung': { icon: '📱', label: 'Samsung' },
            'google': { icon: '📱', label: 'Google' },
            'ruuvi': { icon: '🌡️', label: 'Ruuvi Sensor' }
        }
    };

    // ============================================
    // API Functions
    // ============================================

    function fetchDevices(filter) {
        let url = '/api/ble/devices';
        const params = [];

        // Default to last 24 hours
        params.push('hours=24');

        if (filter === 'registered') {
            params.push('registered=true');
        } else if (filter === 'discovered') {
            params.push('discovered=true');
        }

        if (params.length > 0) {
            url += '?' + params.join('&');
        }

        return fetch(url)
            .then(function(res) {
                if (!res.ok) {
                    throw new Error('Failed to fetch devices: ' + res.status);
                }
                return res.json();
            })
            .then(function(data) {
                return data.devices || [];
            });
    }

    function fetchPeople() {
        return fetch('/api/people')
            .then(function(res) {
                if (!res.ok) {
                    return []; // People API might not exist yet
                }
                return res.json();
            })
            .catch(function() {
                return []; // Graceful degradation
            });
    }

    function loadAllDevices() {
        state.loading = true;

        // Fetch devices and people in parallel
        return Promise.all([
            fetchDevices().then(function(devices) {
                // Split into registered and discovered based on person_id
                state.registeredDevices = devices.filter(function(d) {
                    return d.person_id && d.person_id !== '';
                });
                state.discoveredDevices = devices.filter(function(d) {
                    return !d.person_id || d.person_id === '';
                });
                return devices;
            }),
            fetchPeople().then(function(people) {
                state.people = people || [];
                return people;
            })
        ]).then(function() {
            state.loading = false;
            updateUnregisteredCount();
        }).catch(function(err) {
            state.loading = false;
            state.error = err.message;
            console.error('[BLEPanel] Error loading devices:', err);
        });
    }

    function updateDevice(mac, data) {
        return fetch('/api/ble/devices/' + encodeURIComponent(mac), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        })
        .then(function(res) {
            if (!res.ok) {
                return res.json().then(function(err) {
                    throw new Error(err.error || 'Failed to update device');
                });
            }
            return res.json();
        })
        .then(function() {
            return loadAllDevices();
        });
    }

    function getDeviceHistory(mac) {
        return fetch('/api/ble/devices/' + encodeURIComponent(mac) + '/history?limit=50')
            .then(function(res) {
                if (!res.ok) {
                    throw new Error('Failed to fetch device history');
                }
                return res.json();
            });
    }

    // ============================================
    // UI Rendering
    // ============================================

    function renderLoading() {
        return '<div class="ble-loading">Loading devices...</div>';
    }

    function renderError(error) {
        return '<div class="ble-error">Error: ' + escapeHtml(error) +
               '<button onclick="window.BLEPanel && window.BLEPanel.refresh()">Retry</button></div>';
    }

    function renderDeviceList(devices, title, showType) {
        if (!devices || devices.length === 0) {
            return '<div class="ble-empty">No ' + title.toLowerCase() + ' devices</div>';
        }

        // Sort by sighting frequency (rssi_count), then by last_seen
        var sortedDevices = devices.slice().sort(function(a, b) {
            var countDiff = (b.rssi_count || 0) - (a.rssi_count || 0);
            if (countDiff !== 0) return countDiff;
            return (b.last_seen_at || 0) - (a.last_seen_at || 0);
        });

        var html = '<div class="ble-section-header">' + title + ' (' + sortedDevices.length + ')</div>';
        html += '<div class="ble-device-list">';

        sortedDevices.forEach(function(device) {
            var name = device.name || device.label || device.device_name || formatMAC(device.mac);
            var typeIcon = getDeviceTypeIcon(device.device_type);
            var typeLabel = device.device_type ? getTypeLabel(device.device_type) : '';
            var rssiText = device.rssi_avg !== 0 ? device.rssi_avg + ' dBm' : '';
            var lastSeenText = formatTime(device.last_seen_at);
            var personName = device.person_name || '';

            var color = device.color || '#888';
            var cssClass = device.person_id ? 'ble-device-person' : 'ble-device-unregistered';

            html += '<div class="ble-device ' + cssClass + '" data-mac="' + device.mac + '">' +
                '<span class="ble-device-icon" style="color:' + color + '">' + typeIcon + '</span>' +
                '<span class="ble-device-info">' +
                    '<span class="ble-device-name">' + escapeHtml(name) + '</span>';

            if (typeLabel) {
                html += '<span class="ble-device-type">' + typeLabel + '</span>';
            }

            if (personName) {
                html += '<span class="ble-device-person-label">(' + escapeHtml(personName) + ')</span>';
            }

            html += '</span>' + // Close ble-device-info
                '<span class="ble-device-meta">' +
                    (rssiText ? '<span class="ble-device-rssi">' + rssiText + '</span>' : '') +
                    '<span class="ble-device-time">' + lastSeenText + '</span>' +
                '</span>';

            // Add action buttons
            if (device.person_id) {
                // Registered device - show expand for details
                html += '<button class="ble-device-expand" data-mac="' + device.mac + '" title="View details">▼</button>';
            } else {
                // Unregistered device - show add button
                html += '<button class="ble-device-add" data-mac="' + device.mac + '" title="Register device">+</button>';
            }

            html += '</div>';
        });

        html += '</div>';
        return html;
    }

    function renderPanelContent() {
        if (state.loading) {
            return renderLoading();
        }

        if (state.error) {
            return renderError(state.error);
        }

        var html = '';

        // Add privacy notice
        html += '<div class="ble-privacy-notice">' +
            '📱 Phones may appear multiple times due to address rotation. ' +
            'Wearables and tracker tags have stable addresses.</div>';

        // Add manual pre-registration button
        html += '<div class="ble-preregister-section">' +
            '<button class="ble-preregister-btn" id="ble-preregister-btn">+ Pre-register Device</button>' +
            '</div>';

        // Add person section
        if (state.filters.showRegistered) {
            html += renderDeviceList(state.registeredDevices, 'People', 'person');
        }

        // Add discovered devices section
        if (state.filters.showDiscovered) {
            html += renderDeviceList(state.discoveredDevices, 'Discovered', 'unregistered');
        }

        if (html === '') {
            html = '<div class="ble-empty">No BLE devices discovered yet. ' +
                   'Devices will appear here automatically when nodes detect them.</div>';
        }

        return html;
    }

    function renderEditModal(device) {
        var isNew = !device;
        var title = isNew ? 'Register Device' : 'Edit Device';
        var mac = device ? device.mac : '';
        var deviceName = device ? (device.name || device.device_name || '') : '';
        var label = device ? (device.label || '') : '';
        var deviceType = device ? (device.device_type || 'unknown') : 'unknown';
        var color = device ? (device.color || '#4fc3f7') : '#4fc3f7';

        return '<div class="ble-edit-form">' +
            '<h3>' + title + '</h3>' +
            (isNew ? '<p class="ble-hint">Register this BLE device to track a person, pet, or object.</p>' : '') +
            (device && device.device_name ? '<p class="ble-info"><strong>Detected:</strong> ' + escapeHtml(device.device_name) +
                (device.manufacturer ? ' (' + escapeHtml(device.manufacturer) + ')' : '') + '</p>' : '') +
            '<div class="ble-form-group">' +
                '<label>Device Label</label>' +
                '<input type="text" id="ble-edit-label" value="' + escapeHtml(label) + '" placeholder="e.g., Alice\'s Phone">' +
            '</div>' +
            '<div class="ble-form-group">' +
                '<label>Assign to Person</label>' +
                '<select id="ble-edit-person">' +
                    '<option value="">-- Unassigned --</option>' +
                    getPeopleOptions(device ? device.person_id : '') +
                '</select>' +
                '<p class="ble-hint">Or create a new person below</p>' +
            '</div>' +
            '<div class="ble-form-group">' +
                '<label>New Person Name</label>' +
                '<input type="text" id="ble-edit-new-person" placeholder="e.g., Alice">' +
            '</div>' +
            '<div class="ble-form-group">' +
                '<label>Display Color</label>' +
                '<input type="color" id="ble-edit-color" value="' + color + '">' +
            '</div>' +
            '<div class="ble-form-group">' +
                '<label>Device Type</label>' +
                '<select id="ble-edit-type">' +
                    '<option value="unknown">Unknown</option>' +
                    '<option value="person">Person (Phone)</option>' +
                    '<option value="pet">Pet (Tracker)</option>' +
                    '<option value="object">Object (Tag)</option>' +
                    '<option value="wearable">Wearable (Watch/Tracker)</option>' +
                    '<option value="headphones">Headphones/Earbuds</option>' +
                    '<option value="tracker">Tracker Tag (Tile/AirTag)</option>' +
                '</select>' +
            '</div>' +
            '<div class="ble-form-actions">' +
                '<button class="btn-cancel" id="ble-edit-cancel">Cancel</button>' +
                '<button class="btn-primary" id="ble-edit-save">Save</button>' +
            '</div>' +
            '</div>';
    }

    function getPeopleOptions(selectedPersonId) {
        var html = '';
        state.people.forEach(function(p) {
            var selected = p.id === selectedPersonId ? ' selected' : '';
            html += '<option value="' + escapeHtml(p.id) + '"' + selected + '>' +
                escapeHtml(p.name) + '</option>';
        });
        return html;
    }

    // ============================================
    // Panel Opening
    // ============================================

    function openBLEPanel() {
        // Refresh data when opening
        loadAllDevices();

        // Open the panel using SpaxelPanels
        return SpaxelPanels.openSidebar({
            title: 'People & Devices',
            content: function(contentEl) {
                // Render initial content
                contentEl.innerHTML = renderPanelContent();

                // Set up event listeners
                setupEventListeners(contentEl);

                // Store reference for updates
                contentEl._blePanelRefresh = refreshContent;
            },
            width: '380px',
            className: 'ble-panel',
            onOpen: function(contentEl) {
                // Panel opened - refresh data
                loadAllDevices();
            },
            onClose: function() {
                // Panel closed - clean up
            }
        });
    }

    function refreshContent() {
        return loadAllDevices().then(function() {
            // After loading, re-render the panel content
            var panel = document.querySelector('.ble-panel .panel-content');
            if (panel) {
                panel.innerHTML = renderPanelContent();
                setupEventListeners(panel);
            }
        });
    }

    function setupEventListeners(contentEl) {
        // Pre-register button
        var preregBtn = contentEl.querySelector('#ble-preregister-btn');
        if (preregBtn) {
            preregBtn.addEventListener('click', showPreregisterModal);
        }

        // Device add buttons
        var addBtns = contentEl.querySelectorAll('.ble-device-add');
        addBtns.forEach(function(btn) {
            btn.addEventListener('click', function(e) {
                e.stopPropagation();
                var mac = this.getAttribute('data-mac');
                showRegisterModal(mac);
            });
        });

        // Device expand buttons
        var expandBtns = contentEl.querySelectorAll('.ble-device-expand');
        expandBtns.forEach(function(btn) {
            btn.addEventListener('click', function(e) {
                e.stopPropagation();
                var mac = this.getAttribute('data-mac');
                var device = findDevice(mac);
                if (device) {
                    showDeviceDetails(device);
                }
            });
        });

        // Device items (for clicking through to details)
        var devices = contentEl.querySelectorAll('.ble-device-person');
        devices.forEach(function(item) {
            item.addEventListener('click', function() {
                var mac = this.getAttribute('data-mac');
                var device = findDevice(mac);
                if (device) {
                    showDeviceDetails(device);
                }
            });
        });

        // Setup modal action button handlers
        setupModalActionHandlers(contentEl);
    }

    function showPreregisterModal() {
        SpaxelPanels.openModal({
            title: 'Pre-register Device',
            content: renderPreregisterForm(),
            width: '400px',
            showCancel: true,
            showConfirm: false,
            onOpen: function(modalEl) {
                var registerBtn = modalEl.querySelector('#ble-preregister-submit');
                var cancelBtn = modalEl.querySelector('#ble-preregister-cancel');

                cancelBtn.addEventListener('click', function() {
                    SpaxelPanels.closeModal();
                });

                registerBtn.addEventListener('click', function() {
                    var mac = modalEl.querySelector('#ble-preregister-mac').value.trim();
                    var label = modalEl.querySelector('#ble-preregister-label').value.trim();

                    if (!mac) {
                        SpaxelPanels.showError('Please enter a MAC address');
                        return;
                    }

                    // Validate MAC format (basic validation)
                    var macPattern = /^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$/;
                    if (!macPattern.test(mac)) {
                        SpaxelPanels.showError('Invalid MAC address format. Use format: AA:BB:CC:DD:EE:FF');
                        return;
                    }

                    var data = { mac: mac };
                    if (label) {
                        data.label = label;
                    }

                    // Pre-register the device
                    preregisterDevice(data).then(function() {
                        SpaxelPanels.showSuccess('Device pre-registered. When your tracker tag is detected, it will be automatically associated with this entry.');
                        SpaxelPanels.closeModal();
                        loadAllDevices();
                    }).catch(function(err) {
                        SpaxelPanels.showError('Failed to pre-register device: ' + err.message);
                    });
                });
            }
        });
    }

    function renderPreregisterForm() {
        return '<div class="ble-edit-form">' +
            '<h3>Pre-register Device</h3>' +
            '<p class="ble-hint">Manually register a tracker tag by its MAC address. Useful for pre-registering Tile, AirTag, or other trackers before they are detected.</p>' +
            '<div class="ble-form-group">' +
                '<label>MAC Address</label>' +
                '<input type="text" id="ble-preregister-mac" placeholder="AA:BB:CC:DD:EE:FF" maxlength="17">' +
            '</div>' +
            '<div class="ble-form-group">' +
                '<label>Label</label>' +
                '<input type="text" id="ble-preregister-label" placeholder="e.g., Car Keys Tile">' +
            '</div>' +
            '<div class="ble-form-actions">' +
                '<button class="btn-cancel" id="ble-preregister-cancel">Cancel</button>' +
                '<button class="btn-primary" id="ble-preregister-submit">Pre-register</button>' +
            '</div>' +
            '</div>';
    }

    function preregisterDevice(data) {
        return fetch('/api/ble/devices/preregister', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        })
        .then(function(res) {
            if (!res.ok) {
                return res.json().then(function(err) {
                    throw new Error(err.error || 'Failed to pre-register device');
                });
            }
            return res.json();
        });
    }

    function setupModalActionHandlers(modalEl) {
        // Edit button
        var editBtn = modalEl.querySelector('#ble-edit-btn');
        if (editBtn) {
            editBtn.addEventListener('click', function() {
                var mac = this.getAttribute('data-mac') || modalEl.getAttribute('data-device-mac');
                if (mac) {
                    var device = findDevice(mac);
                    SpaxelPanels.closeModal();
                    if (device) {
                        showRegisterModal(mac);
                    }
                }
            });
        }

        // Unregister button
        var unregisterBtn = modalEl.querySelector('#ble-unregister-btn');
        if (unregisterBtn) {
            unregisterBtn.addEventListener('click', function() {
                var mac = this.getAttribute('data-mac') || modalEl.getAttribute('data-device-mac');
                if (mac && confirm('Unregister this device? This will remove the person association.')) {
                    unregisterDevice(mac).then(function() {
                        SpaxelPanels.showSuccess('Device unregistered');
                        SpaxelPanels.closeModal();
                        loadAllDevices();
                    }).catch(function(err) {
                        SpaxelPanels.showError('Failed to unregister device: ' + err.message);
                    });
                }
            });
        }
    }

    function unregisterDevice(mac) {
        return fetch('/api/ble/devices/' + encodeURIComponent(mac), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ person_id: null })
        })
        .then(function(res) {
            if (!res.ok) {
                return res.json().then(function(err) {
                    throw new Error(err.error || 'Failed to unregister device');
                });
            }
            return res.json();
        });
    }

    // ============================================
    // Modals
    // ============================================

    function showRegisterModal(mac) {
        var device = findDevice(mac);
        var isNew = !device;

        SpaxelPanels.openModal({
            title: isNew ? 'Register Device' : 'Edit Device',
            content: renderEditModal(device),
            width: '400px',
            showCancel: true,
            showConfirm: false,
            onOpen: function(modalEl) {
                var saveBtn = modalEl.querySelector('#ble-edit-save');
                var cancelBtn = modalEl.querySelector('#ble-edit-cancel');

                cancelBtn.addEventListener('click', function() {
                    SpaxelPanels.closeModal();
                });

                saveBtn.addEventListener('click', function() {
                    var label = modalEl.querySelector('#ble-edit-label').value;
                    var personId = modalEl.querySelector('#ble-edit-person').value;
                    var newPersonName = modalEl.querySelector('#ble-edit-new-person').value;
                    var color = modalEl.querySelector('#ble-edit-color').value;
                    var deviceType = modalEl.querySelector('#ble-edit-type').value;

                    // Create new person if specified
                    var registrationPromise = Promise.resolve();

                    if (newPersonName && !personId) {
                        // Need to create a new person first
                        registrationPromise = createPerson(newPersonName, color).then(function(person) {
                            return person.id;
                        });
                    } else if (newPersonName && personId) {
                        // User selected a person AND entered a new name - prefer new person
                        registrationPromise = createPerson(newPersonName, color).then(function(person) {
                            return person.id;
                        });
                    } else {
                        registrationPromise = Promise.resolve(personId);
                    }

                    registrationPromise.then(function(finalPersonId) {
                        var data = {
                            label: label || newPersonName || deviceName,
                            device_type: deviceType
                        };

                        // Set color for the person (not the device)
                        if (finalPersonId) {
                            data.person_id = finalPersonId;
                        }

                        updateDevice(mac, data).then(function() {
                            SpaxelPanels.showSuccess('Device registered successfully');
                            SpaxelPanels.closeModal();
                            // Refresh the panel and reload people
                            loadAllDevices();
                        }).catch(function(err) {
                            SpaxelPanels.showError('Failed to register device: ' + err.message);
                        });
                    }).catch(function(err) {
                        SpaxelPanels.showError('Failed to create person: ' + err.message);
                    });
                });
            }
        });
    }

    function createPerson(name, color) {
        return fetch('/api/people', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: name, color: color })
        })
        .then(function(res) {
            if (!res.ok) {
                return res.json().then(function(err) {
                    throw new Error(err.error || 'Failed to create person');
                });
            }
            return res.json();
        });
    }

    function showDeviceDetails(device) {
        var deviceName = device.name || device.label || device.device_name || formatMAC(device.mac);

        // Fetch device history
        getDeviceHistory(device.mac).then(function(historyData) {
            SpaxelPanels.openModal({
                title: deviceName,
                content: renderDeviceDetails(device, historyData),
                width: '400px',
                showCancel: true,
                showConfirm: false,
                className: 'ble-details-modal'
            });
        }).catch(function(err) {
            // Show details without history if fetch fails
            console.error('[BLEPanel] Failed to fetch device history:', err);
            SpaxelPanels.openModal({
                title: deviceName,
                content: renderDeviceDetails(device, null),
                width: '400px',
                showCancel: true,
                showConfirm: false,
                className: 'ble-details-modal'
            });
        });
    }

    function renderDeviceDetails(device, historyData) {
        var html = '<div class="ble-device-details">' +
            '<div class="ble-detail-row">' +
                '<span class="ble-detail-label">MAC Address</span>' +
                '<span class="ble-detail-value">' + escapeHtml(device.mac) + '</span>' +
            '</div>';

        if (device.manufacturer) {
            html += '<div class="ble-detail-row">' +
                '<span class="ble-detail-label">Manufacturer</span>' +
                '<span class="ble-detail-value">' + escapeHtml(device.manufacturer) + '</span>' +
                '</div>';
        }

        if (device.device_type && device.device_type !== 'unknown') {
            html += '<div class="ble-detail-row">' +
                '<span class="ble-detail-label">Device Type</span>' +
                '<span class="ble-detail-value">' + getTypeLabel(device.device_type) + '</span>' +
                '</div>';
        }

        if (device.person_name) {
            html += '<div class="ble-detail-row">' +
                '<span class="ble-detail-label">Assigned To</span>' +
                '<span class="ble-detail-value">' + escapeHtml(device.person_name) + '</span>' +
                '</div>';
        }

        html += '<div class="ble-detail-row">' +
            '<span class="ble-detail-label">First Seen</span>' +
            '<span class="ble-detail-value">' + formatTime(device.first_seen_at) + '</span>' +
        '</div>' +
            '<div class="ble-detail-row">' +
            '<span class="ble-detail-label">Last Seen</span>' +
            '<span class="ble-detail-value">' + formatTime(device.last_seen_at) + '</span>' +
            '</div>';

        if (device.last_seen_node) {
            html += '<div class="ble-detail-row">' +
                '<span class="ble-detail-label">Last Seen By</span>' +
                '<span class="ble-detail-value">' + escapeHtml(device.last_seen_node) + '</span>' +
                '</div>';
        }

        if (device.rssi_count > 0) {
            html += '<div class="ble-detail-row">' +
                '<span class="ble-detail-label">Sighting Count</span>' +
                '<span class="ble-detail-value">' + device.rssi_count + ' times</span>' +
                '</div>';
        }

        if (device.rssi_avg !== 0) {
            html += '<div class="ble-detail-row">' +
                '<span class="ble-detail-label">Average RSSI</span>' +
                '<span class="ble-detail-value">' + device.rssi_avg + ' dBm</span>' +
                '</div>';
        }

        html += '</div>'; // Close ble-device-details

        // Add recent history if available
        if (historyData && historyData.history && historyData.history.length > 0) {
            html += '<div class="ble-section-header">Recent Sightings</div>';
            html += '<div class="ble-history-list">';
            historyData.history.slice(0, 10).forEach(function(entry) {
                html += '<div class="ble-history-entry">' +
                    '<span class="ble-history-time">' + formatTime(entry.timestamp) + '</span>' +
                    '<span class="ble-history-rssi">' + entry.rssi_dbm + ' dBm</span>' +
                    '<span class="ble-history-node">from ' + escapeHtml(entry.node_mac) + '</span>' +
                    '</div>';
            });
            html += '</div>';
        }

        // Add action buttons
        html += '<div class="ble-details-actions" data-device-mac="' + escapeHtml(device.mac) + '">' +
            '<button class="btn-secondary" id="ble-edit-btn" data-mac="' + escapeHtml(device.mac) + '">Edit</button> ';

        if (device.person_id) {
            html += '<button class="btn-danger" id="ble-unregister-btn" data-mac="' + escapeHtml(device.mac) + '">Unregister</button> ';
        }

        html += '</div>';

        return html;
    }

    // Store reference to current device for modal handlers
    state.currentModalDevice = device;
}

    // ============================================
    // Utility Functions
    // ============================================

    function findDevice(mac) {
        return state.registeredDevices.concat(state.discoveredDevices).find(function(d) {
            return d.mac === mac;
        });
    }

    function formatMAC(mac) {
        if (!mac) return '';
        // Show truncated MAC (last 4 segments) for privacy
        var parts = mac.split(':');
        if (parts.length === 6) {
            return 'XX:XX:' + parts.slice(2).join(':');
        }
        return mac;
    }

    function getTypeLabel(type) {
        switch (type) {
            case 'apple_phone': return 'iPhone';
            case 'apple_watch': return 'Apple Watch';
            case 'apple_earbuds': return 'AirPods';
            case 'fitbit': return 'Fitbit';
            case 'garmin': return 'Garmin';
            case 'tile': return 'Tile';
            case 'microsoft': return 'Microsoft';
            case 'samsung': return 'Samsung';
            case 'google': return 'Google';
            case 'ruuvi': return 'Ruuvi';
            case 'person': return 'Phone';
            case 'pet': return 'Pet Tracker';
            case 'object': return 'Object';
            case 'wearable': return 'Wearable';
            case 'headphones': return 'Headphones';
            case 'tracker': return 'Tracker Tag';
            default: return '';
        }
    }

    function getDeviceTypeIcon(type) {
        if (!type) return '📡';

        // Check if we have a type hint for this device type
        if (state.typeHints[type]) {
            return state.typeHints[type].icon;
        }

        // Fallback icons based on type category
        switch (type) {
            case 'apple_phone':
            case 'person':
                return '📱';
            case 'apple_watch':
            case 'fitbit':
            case 'garmin':
            case 'wearable':
                return '⌚';
            case 'apple_earbuds':
            case 'headphones':
                return '🎧';
            case 'tile':
            case 'tracker':
                return '📍';
            case 'microsoft':
                return '💻';
            case 'ruuvi':
                return '🌡️';
            default:
                return '📡';
        }
    }

    function getColorForPerson(personName) {
        // Check if we have a person with this name in our people list
        var person = state.people.find(function(p) { return p.name === personName; });
        if (person && person.color) {
            return person.color;
        }

        // Generate consistent color based on person name
        var hash = 0;
        for (var i = 0; i < personName.length; i++) {
            hash = personName.charCodeAt(i) + ((hash << 5) - hash);
        }
        var hue = Math.abs(hash) % 360;
        return 'hsl(' + hue + ', 70%, 60%)';
    }

    function formatTime(timestamp) {
        if (!timestamp) return 'Unknown';
        var date;
        // Handle both Unix timestamps (in nanoseconds from Go) and JS dates
        if (typeof timestamp === 'number') {
            // If it's in nanoseconds (Go time), convert to milliseconds
            if (timestamp > 10000000000) {
                date = new Date(timestamp / 1000000);
            } else {
                date = new Date(timestamp);
            }
        } else {
            date = new Date(timestamp);
        }

        var now = new Date();
        var diff = now - date;

        if (diff < 60000) return 'just now';
        if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
        if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
        if (diff < 604800000) return Math.floor(diff / 86400000) + 'd ago';
        return date.toLocaleDateString();
    }

    function escapeHtml(text) {
        if (!text) return '';
        return String(text)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    function updateUnregisteredCount() {
        var count = state.discoveredDevices.length;
        var badge = document.getElementById('ble-unregistered-badge');
        if (badge) {
            badge.textContent = count > 0 ? count : '';
            badge.style.display = count > 0 ? 'inline' : 'none';
        }
    }

    // ============================================
    // Public API
    // ============================================

    window.BLEPanel = {
        // Open the BLE panel
        open: openBLEPanel,

        // Refresh device list
        refresh: loadAllDevices,

        // Update devices (called from WebSocket)
        updateDevices: function(devices) {
            state.registeredDevices = devices.filter(function(d) { return d.person_id; });
            state.discoveredDevices = devices.filter(function(d) { return !d.person_id; });
            updateUnregisteredCount();

            // If panel is open, refresh content
            var panelContent = document.querySelector('.ble-panel .panel-content');
            if (panelContent && panelContent._blePanelRefresh) {
                panelContent.innerHTML = renderPanelContent();
                setupEventListeners(panelContent);
            }
        }
    };

    // ============================================
    // Registration & Initialization
    // ============================================

    // Register the BLE panel
    if (window.SpaxelPanels) {
        SpaxelPanels.register('ble', openBLEPanel);
    }

    // Also register as a global function for direct access
    window.openBLEPanel = openBLEPanel;

    // Initial data load
    loadAllDevices();

    // Update unregistered count badge periodically
    setInterval(loadAllDevices, 30000); // Every 30 seconds

    console.log('[BLEPanel] BLE device panel module loaded');
})();
