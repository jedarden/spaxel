/**
 * Spaxel Fleet Status Page
 *
 * Provides a comprehensive table view of all nodes with:
 * - Sorting and filtering
 * - Bulk actions
 * - Inline label editing
 * - Camera fly-to integration
 * - CSV export
 */

(function() {
    'use strict';

    // ============================================
    // State
    // ============================================
    const state = {
        nodes: [],                // All nodes from API
        filteredNodes: [],        // Nodes after filtering
        selectedNodes: new Set(), // Selected MAC addresses
        sortColumn: null,
        sortDirection: 'asc',     // 'asc' or 'desc'
        filters: {
            search: '',
            status: '',
            firmware: '',
            roles: []
        },
        latestFirmware: null,     // Latest firmware version
        wsConnected: false
    };

    const CONFIG = {
        pollIntervalMs: 10000,    // Poll every 10 seconds
        staleThresholdMs: 30000   // Node considered stale after 30s
    };

    // ============================================
    // DOM Elements
    // ============================================
    let elements = {};

    // ============================================
    // Initialization
    // ============================================
    async function init() {
        console.log('[FleetPage] Initializing fleet status page');

        // Wait for authentication
        if (window.SpaxelAuth) {
            const isAuthenticated = await SpaxelAuth.checkStatus();
            if (!isAuthenticated) {
                // Redirect to login
                window.location.href = '/';
                return;
            }
        }

        cacheElements();
        bindEvents();
        await fetchFleetData();
        startPolling();

        console.log('[FleetPage] Fleet status page ready');
    }

    function cacheElements() {
        elements = {
            // Table elements
            tableBody: document.getElementById('fleet-table-body'),
            selectAllCheckbox: document.getElementById('select-all-checkbox'),

            // Summary elements
            totalSummary: document.getElementById('fleet-total'),
            onlineSummary: document.getElementById('fleet-online'),

            // Toolbar elements
            toolbar: document.getElementById('fleet-toolbar'),
            searchInput: document.getElementById('fleet-search'),
            filterStatus: document.getElementById('filter-status'),
            filterFirmware: document.getElementById('filter-firmware'),
            filterRole: document.getElementById('filter-role'),
            activeFilters: document.getElementById('active-filters'),

            // Bulk actions
            bulkActionsBar: document.getElementById('bulk-actions-bar'),
            bulkSelectedCount: document.getElementById('bulk-selected-count'),
            bulkOtaBtn: document.getElementById('bulk-ota-btn'),
            bulkRoleBtn: document.getElementById('bulk-role-btn'),
            bulkRemoveBtn: document.getElementById('bulk-remove-btn'),
            bulkClearBtn: document.getElementById('bulk-clear-btn'),

            // Header actions
            refreshBtn: document.getElementById('fleet-refresh-btn'),
            updateAllBtn: document.getElementById('fleet-update-all-btn'),
            downloadBtn: document.getElementById('fleet-download-btn'),

            // Empty state
            emptyState: document.getElementById('empty-state'),

            // Modals
            otaModal: document.getElementById('ota-modal'),
            otaConfirmBtn: document.getElementById('ota-confirm-btn'),
            otaNodeLabel: document.getElementById('ota-node-label'),
            otaCurrentVersion: document.getElementById('ota-current-version'),
            otaLatestVersion: document.getElementById('ota-latest-version'),
            otaInfoCurrent: document.getElementById('ota-info-current'),
            otaInfoLatest: document.getElementById('ota-info-latest'),
            otaInfoStatus: document.getElementById('ota-info-status'),

            roleModal: document.getElementById('role-modal'),
            roleConfirmBtn: document.getElementById('role-confirm-btn'),
            roleNodeCount: document.getElementById('role-node-count'),

            removeModal: document.getElementById('remove-modal'),
            removeConfirmBtn: document.getElementById('remove-confirm-btn'),
            removeNodeCount: document.getElementById('remove-node-count'),
            removeNodeList: document.getElementById('remove-node-list'),

            toastContainer: document.getElementById('toast-container')
        };
    }

    function bindEvents() {
        // Refresh button
        elements.refreshBtn.addEventListener('click', () => {
            fetchFleetData();
        });

        // Update all button
        elements.updateAllBtn.addEventListener('click', () => {
            updateAllFirmware();
        });

        // Download button
        elements.downloadBtn.addEventListener('click', () => {
            downloadCSV();
        });

        // Select all checkbox
        elements.selectAllCheckbox.addEventListener('change', (e) => {
            toggleSelectAll(e.target.checked);
        });

        // Filters
        elements.searchInput.addEventListener('input', debounce((e) => {
            state.filters.search = e.target.value.toLowerCase();
            applyFilters();
        }, 300));

        elements.filterStatus.addEventListener('change', (e) => {
            state.filters.status = e.target.value;
            applyFilters();
        });

        elements.filterFirmware.addEventListener('change', (e) => {
            state.filters.firmware = e.target.value;
            applyFilters();
        });

        elements.filterRole.addEventListener('change', (e) => {
            const selected = Array.from(e.target.selectedOptions).map(opt => opt.value);
            state.filters.roles = selected;
            applyFilters();
        });

        // Bulk action buttons
        elements.bulkOtaBtn.addEventListener('click', () => {
            showBulkOTAModal();
        });

        elements.bulkRoleBtn.addEventListener('click', () => {
            showRoleModal();
        });

        elements.bulkRemoveBtn.addEventListener('click', () => {
            showRemoveModal();
        });

        elements.bulkClearBtn.addEventListener('click', () => {
            clearSelection();
        });

        // Table sort handlers
        document.querySelectorAll('th.sortable').forEach(th => {
            th.addEventListener('click', () => {
                const column = th.dataset.sort;
                handleSort(column);
            });
        });

        // Modal close buttons
        document.querySelectorAll('.modal-close').forEach(btn => {
            btn.addEventListener('click', () => {
                const modalId = btn.dataset.modal;
                closeModal(modalId);
            });
        });

        // Modal backdrop click
        document.querySelectorAll('.modal').forEach(modal => {
            modal.addEventListener('click', (e) => {
                if (e.target === modal) {
                    closeModal(modal.id);
                }
            });
        });

        // OTA confirm
        elements.otaConfirmBtn.addEventListener('click', () => {
            confirmOTA();
        });

        // Role confirm
        elements.roleConfirmBtn.addEventListener('click', () => {
            confirmRoleAssignment();
        });

        // Remove confirm
        elements.removeConfirmBtn.addEventListener('click', () => {
            confirmRemove();
        });
    }

    // ============================================
    // Data Fetching
    // ============================================
    async function fetchFleetData() {
        try {
            const response = await fetch('/api/nodes');
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            const nodes = await response.json();
            state.nodes = nodes || [];

            // Get latest firmware version
            await fetchLatestFirmware();

            applyFilters();
            updateSummary();
            updateBulkActions();

        } catch (error) {
            console.error('[FleetPage] Failed to fetch fleet data:', error);
            showToast('Failed to load fleet data', 'error');
        }
    }

    async function fetchLatestFirmware() {
        try {
            const response = await fetch('/api/firmware');
            if (!response.ok) {
                return; // Firmware endpoint might not be implemented yet
            }

            const firmware = await response.json();
            if (firmware && firmware.length > 0) {
                const latest = firmware.find(f => f.is_latest);
                if (latest) {
                    state.latestFirmware = latest.version;
                }
            }
        } catch (error) {
            console.debug('[FleetPage] Firmware endpoint not available:', error);
        }
    }

    function startPolling() {
        setInterval(() => {
            fetchFleetData();
        }, CONFIG.pollIntervalMs);
    }

    // ============================================
    // Filtering and Sorting
    // ============================================
    function applyFilters() {
        let filtered = state.nodes.slice();

        // Search filter
        if (state.filters.search) {
            const search = state.filters.search.toLowerCase();
            filtered = filtered.filter(node => {
                const label = node.name || node.label || '';
                const mac = node.mac.toLowerCase();
                return label.toLowerCase().includes(search) || mac.includes(search);
            });
        }

        // Status filter
        if (state.filters.status) {
            filtered = filtered.filter(node => {
                return getNodeStatus(node) === state.filters.status;
            });
        }

        // Firmware filter
        if (state.filters.firmware === 'outdated') {
            filtered = filtered.filter(node => {
                return isFirmwareOutdated(node);
            });
        }

        // Role filter
        if (state.filters.roles.length > 0) {
            filtered = filtered.filter(node => {
                return state.filters.roles.includes(node.role);
            });
        }

        // Apply sorting
        if (state.sortColumn) {
            filtered.sort((a, b) => {
                const aVal = getSortValue(a, state.sortColumn);
                const bVal = getSortValue(b, state.sortColumn);

                let comparison = 0;
                if (typeof aVal === 'string') {
                    comparison = aVal.localeCompare(bVal);
                } else {
                    comparison = aVal - bVal;
                }

                return state.sortDirection === 'asc' ? comparison : -comparison;
            });
        }

        state.filteredNodes = filtered;
        renderTable();
        renderActiveFilters();
    }

    function getSortValue(node, column) {
        switch (column) {
            case 'label':
                return node.name || node.label || '';
            case 'mac':
                return node.mac;
            case 'status':
                const status = getNodeStatus(node);
                // Priority: online > updating > offline
                return status === 'online' ? 2 : status === 'updating' ? 1 : 0;
            case 'firmware':
                return node.firmware_version || '';
            case 'uptime':
                return node.uptime_seconds || 0;
            case 'role':
                return node.role;
            case 'health':
                return node.health_score || 0;
            case 'packetRate':
                return node.packet_rate || 0;
            case 'temperature':
                return node.temperature || 0;
            default:
                return '';
        }
    }

    function handleSort(column) {
        if (state.sortColumn === column) {
            state.sortDirection = state.sortDirection === 'asc' ? 'desc' : 'asc';
        } else {
            state.sortColumn = column;
            state.sortDirection = 'asc';
        }

        // Update sort indicators
        document.querySelectorAll('th.sortable').forEach(th => {
            th.classList.remove('sort-asc', 'sort-desc');
            if (th.dataset.sort === column) {
                th.classList.add(state.sortDirection === 'asc' ? 'sort-asc' : 'sort-desc');
            }
        });

        applyFilters();
    }

    function renderActiveFilters() {
        let filters = [];

        if (state.filters.search) {
            filters.push({
                type: 'search',
                label: `Search: "${state.filters.search}"`,
                value: 'search'
            });
        }

        if (state.filters.status) {
            const label = elements.filterStatus.options[elements.filterStatus.selectedIndex].text;
            filters.push({
                type: 'status',
                label: `Status: ${label}`,
                value: 'status'
            });
        }

        if (state.filters.firmware) {
            const label = elements.filterFirmware.options[elements.filterFirmware.selectedIndex].text;
            filters.push({
                type: 'firmware',
                label: `Firmware: ${label}`,
                value: 'firmware'
            });
        }

        if (state.filters.roles.length > 0) {
            filters.push({
                type: 'roles',
                label: `Roles: ${state.filters.roles.join(', ')}`,
                value: 'roles'
            });
        }

        if (filters.length === 0) {
            elements.activeFilters.innerHTML = '';
            elements.toolbar.style.display = 'none';
            return;
        }

        elements.toolbar.style.display = 'block';
        elements.activeFilters.innerHTML = filters.map(f => `
            <span class="filter-chip" data-filter-type="${f.type}">
                ${f.label}
                <span class="filter-dismiss" data-filter="${f.value}">&times;</span>
            </span>
        `).join('');

        // Add dismiss handlers
        elements.activeFilters.querySelectorAll('.filter-dismiss').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                clearFilter(btn.dataset.filter);
            });
        });
    }

    function clearFilter(filterType) {
        switch (filterType) {
            case 'search':
                elements.searchInput.value = '';
                state.filters.search = '';
                break;
            case 'status':
                elements.filterStatus.value = '';
                state.filters.status = '';
                break;
            case 'firmware':
                elements.filterFirmware.value = '';
                state.filters.firmware = '';
                break;
            case 'roles':
                Array.from(elements.filterRole.options).forEach(opt => opt.selected = false);
                state.filters.roles = [];
                break;
        }
        applyFilters();
    }

    // ============================================
    // Table Rendering
    // ============================================
    function renderTable() {
        const { filteredNodes, selectedNodes } = state;
        const { tableBody, emptyState } = elements;

        // Show empty state if no nodes
        if (filteredNodes.length === 0) {
            tableBody.innerHTML = '';
            emptyState.style.display = 'block';
            return;
        }

        emptyState.style.display = 'none';

        // Render rows
        tableBody.innerHTML = filteredNodes.map(node => {
            const isSelected = selectedNodes.has(node.mac);
            const status = getNodeStatus(node);
            const isOutdated = isFirmwareOutdated(node);
            const healthClass = getHealthClass(node.health_score);
            const healthPercent = Math.round((node.health_score || 0) * 100);
            const packetRateClass = getPacketRateClass(node);
            const tempClass = getTemperatureClass(node.temperature);

            return `
                <tr class="fleet-row${isSelected ? ' selected' : ''}" data-mac="${node.mac}">
                    <td class="col-checkbox">
                        <input type="checkbox" class="checkbox node-checkbox"
                               data-mac="${node.mac}"${isSelected ? ' checked' : ''}>
                    </td>
                    <td class="col-label">
                        <div class="node-label" data-mac="${node.mac}" title="Double-click to edit">
                            ${escapeHtml(node.name || node.label || 'Unnamed')}
                        </div>
                    </td>
                    <td class="col-mac">
                        <span class="mac-address mac-tooltip" title="${node.mac}">
                            ${truncateMAC(node.mac)}
                        </span>
                    </td>
                    <td class="col-status">
                        <span class="status-badge ${status}">
                            <span class="status-dot"></span>
                            ${capitalize(status)}
                        </span>
                    </td>
                    <td class="col-firmware">
                        <div class="firmware-version">
                            <span class="${isOutdated ? 'firmware-outdated' : 'firmware-current'}">
                                ${escapeHtml(node.firmware_version || '--')}
                            </span>
                            ${isOutdated ? `
                                <span class="firmware-indicator">
                                    <span class="firmware-arrow">&rarr;</span>
                                    ${escapeHtml(state.latestFirmware || '?')}
                                </span>
                            ` : ''}
                        </div>
                    </td>
                    <td class="col-uptime">
                        ${formatUptime(node.uptime_seconds)}
                    </td>
                    <td class="col-role">
                        <span class="role-badge ${node.role}">${formatRole(node.role)}</span>
                    </td>
                    <td class="col-health">
                        <div class="health-bar-container">
                            <div class="health-bar">
                                <div class="health-bar-fill ${healthClass}" style="width: ${healthPercent}%"></div>
                            </div>
                            <span class="health-value">${healthPercent}%</span>
                        </div>
                    </td>
                    <td class="col-packet-rate">
                        <span class="packet-rate ${packetRateClass}">
                            ${formatPacketRate(node)}
                        </span>
                    </td>
                    <td class="col-temperature">
                        <span class="temperature ${tempClass}">
                            ${formatTemperature(node.temperature)}
                        </span>
                    </td>
                    <td class="col-actions">
                        <div class="action-buttons">
                            <button class="action-btn btn-locate" data-mac="${node.mac}"
                                    title="Locate (flash LED)" ${status !== 'online' ? 'disabled' : ''}>
                                &#x26A1;
                            </button>
                            <button class="action-btn btn-ota" data-mac="${node.mac}"
                                    title="Update Firmware" ${!isOutdated ? 'disabled' : ''}>
                                &#x2191;
                            </button>
                            <button class="action-btn btn-flyto" data-mac="${node.mac}"
                                    title="Fly to Node">
                                &#x26F6;
                            </button>
                            <button class="action-btn btn-more" data-mac="${node.mac}"
                                    title="More Actions">
                                &#x2026;
                            </button>
                        </div>
                    </td>
                </tr>
            `;
        }).join('');

        // Bind row events
        bindRowEvents();
    }

    function bindRowEvents() {
        // Checkbox handlers
        document.querySelectorAll('.node-checkbox').forEach(checkbox => {
            checkbox.addEventListener('change', (e) => {
                const mac = e.target.dataset.mac;
                if (e.target.checked) {
                    state.selectedNodes.add(mac);
                } else {
                    state.selectedNodes.delete(mac);
                }
                updateBulkActions();
                renderTable(); // Re-render to update selected state
            });
        });

        // Row click for fly-to
        document.querySelectorAll('.fleet-row').forEach(row => {
            row.addEventListener('click', (e) => {
                // Don't trigger if clicking on checkbox, actions, or label
                if (e.target.closest('.checkbox') ||
                    e.target.closest('.action-buttons') ||
                    e.target.closest('.node-label')) {
                    return;
                }
                const mac = row.dataset.mac;
                flyToNode(mac);
            });
        });

        // Label double-click for inline edit
        document.querySelectorAll('.node-label').forEach(labelEl => {
            labelEl.addEventListener('dblclick', (e) => {
                e.stopPropagation();
                const mac = labelEl.dataset.mac;
                startLabelEdit(labelEl, mac);
            });
        });

        // Action buttons
        document.querySelectorAll('.btn-locate').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const mac = btn.dataset.mac;
                identifyNode(mac);
            });
        });

        document.querySelectorAll('.btn-ota').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const mac = btn.dataset.mac;
                showOTAModal(mac);
            });
        });

        document.querySelectorAll('.btn-flyto').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const mac = btn.dataset.mac;
                flyToNode(mac);
            });
        });

        document.querySelectorAll('.btn-more').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const mac = btn.dataset.mac;
                showMoreActions(btn, mac);
            });
        });
    }

    // ============================================
    // Inline Label Edit
    // ============================================
    function startLabelEdit(element, mac) {
        const currentValue = element.textContent;
        const node = state.nodes.find(n => n.mac === mac);
        const existingLabel = node.name || node.label || '';

        element.contentEditable = true;
        element.classList.add('editing');
        element.textContent = existingLabel;

        // Focus and select all text
        element.focus();

        const range = document.createRange();
        range.selectNodeContents(element);
        const selection = window.getSelection();
        selection.removeAllRanges();
        selection.addRange(range);

        const handleBlur = async () => {
            element.removeEventListener('blur', handleBlur);
            element.removeEventListener('keydown', handleKeydown);

            const newValue = element.textContent.trim();
            element.contentEditable = false;
            element.classList.remove('editing');

            // Validate
            if (newValue.length > 32) {
                showToast('Label must be 32 characters or less', 'warning');
                element.textContent = currentValue;
                return;
            }

            if (newValue === currentValue) {
                // No change
                element.textContent = currentValue;
                return;
            }

            // Save to server
            try {
                const response = await fetch(`/api/nodes/${mac}/label`, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ label: newValue })
                });

                if (!response.ok) {
                    throw new Error(`HTTP ${response.status}`);
                }

                // Update local state
                const nodeIndex = state.nodes.findIndex(n => n.mac === mac);
                if (nodeIndex !== -1) {
                    state.nodes[nodeIndex].name = newValue;
                    state.nodes[nodeIndex].label = newValue;
                }

                showToast('Label updated', 'success');
                applyFilters();

            } catch (error) {
                console.error('[FleetPage] Failed to update label:', error);
                showToast('Failed to update label', 'error');
                element.textContent = currentValue;
            }
        };

        const handleKeydown = (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                element.blur();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                element.textContent = currentValue;
                element.blur();
            }
        };

        element.addEventListener('blur', handleBlur);
        element.addEventListener('keydown', handleKeydown);
    }

    // ============================================
    // Bulk Actions
    // ============================================
    function toggleSelectAll(checked) {
        state.filteredNodes.forEach(node => {
            if (checked) {
                state.selectedNodes.add(node.mac);
            } else {
                state.selectedNodes.delete(node.mac);
            }
        });
        updateBulkActions();
        renderTable();
    }

    function clearSelection() {
        state.selectedNodes.clear();
        updateBulkActions();
        renderTable();
    }

    function updateBulkActions() {
        const count = state.selectedNodes.size;
        elements.bulkSelectedCount.textContent = count;
        elements.selectAllCheckbox.checked = count > 0 && count === state.filteredNodes.length;
        elements.bulkActionsBar.style.display = count > 0 ? 'block' : 'none';

        // Update button states
        const hasOutdated = Array.from(state.selectedNodes).some(mac => {
            const node = state.nodes.find(n => n.mac === mac);
            return node && isFirmwareOutdated(node);
        });
        elements.bulkOtaBtn.disabled = !hasOutdated;
    }

    // ============================================
    // Node Actions
    // ============================================
    async function identifyNode(mac) {
        try {
            showToast('Sending identify command...', 'info');

            const response = await fetch(`/api/nodes/${mac}/identify`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ duration_ms: 5000 })
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            showToast(`Identifying ${formatMAC(mac)}`, 'success');

        } catch (error) {
            console.error('[FleetPage] Identify failed:', error);
            showToast('Failed to send identify command', 'error');
        }
    }

    function flyToNode(mac) {
        // Store target MAC in localStorage for expert mode
        localStorage.setItem('fleetFlyToMAC', mac);

        // Redirect to expert mode
        window.location.href = '/?highlight=' + mac;
    }

    function showOTAModal(mac) {
        const node = state.nodes.find(n => n.mac === mac);
        if (!node) return;

        elements.otaNodeLabel.textContent = node.name || node.label || formatMAC(mac);
        elements.otaCurrentVersion.textContent = node.firmware_version || 'Unknown';
        elements.otaLatestVersion.textContent = state.latestFirmware || 'Unknown';
        elements.otaInfoCurrent.textContent = node.firmware_version || 'Unknown';
        elements.otaInfoLatest.textContent = state.latestFirmware || 'Unknown';
        elements.otaInfoStatus.textContent = getNodeStatus(node);
        elements.otaInfoStatus.className = 'info-value ' + getNodeStatus(node);

        elements.otaModal.dataset.targetMAC = mac;
        openModal('ota-modal');
    }

    function showBulkOTAModal() {
        const macs = Array.from(state.selectedNodes);
        const outdated = macs.filter(mac => {
            const node = state.nodes.find(n => n.mac === mac);
            return node && isFirmwareOutdated(node);
        });

        if (outdated.length === 0) {
            showToast('No selected nodes have outdated firmware', 'warning');
            return;
        }

        // Show confirmation
        if (outdated.length === 1) {
            showOTAModal(outdated[0]);
        } else {
            // For multiple nodes, confirm without showing modal
            if (confirm(`Update ${outdated.length} nodes to firmware ${state.latestFirmware || 'latest'}?`)) {
                updateMultipleNodes(outdated);
            }
        }
    }

    async function confirmOTA() {
        const mac = elements.otaModal.dataset.targetMAC;
        if (!mac) return;

        closeModal('ota-modal');

        try {
            showToast('Starting firmware update...', 'info');

            const response = await fetch(`/api/nodes/${mac}/ota`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' }
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            showToast(`Firmware update started for ${formatMAC(mac)}`, 'success');
            fetchFleetData(); // Refresh to show updating status

        } catch (error) {
            console.error('[FleetPage] OTA failed:', error);
            showToast('Failed to start firmware update', 'error');
        }
    }

    async function updateMultipleNodes(macs) {
        try {
            showToast(`Starting updates for ${macs.length} nodes...`, 'info');

            // Update nodes in sequence with 30s stagger
            for (let i = 0; i < macs.length; i++) {
                setTimeout(async () => {
                    try {
                        await fetch(`/api/nodes/${macs[i]}/ota`, {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' }
                        });

                        if (i === macs.length - 1) {
                            showToast(`Firmware updates started for ${macs.length} nodes`, 'success');
                            fetchFleetData();
                        }
                    } catch (error) {
                        console.error(`[FleetPage] OTA failed for ${macs[i]}:`, error);
                    }
                }, i * 30000); // 30 second stagger
            }

            showToast(`Firmware updates queued for ${macs.length} nodes`, 'info');

        } catch (error) {
            console.error('[FleetPage] Bulk OTA failed:', error);
            showToast('Failed to start firmware updates', 'error');
        }
    }

    async function updateAllFirmware() {
        if (!confirm('Update all nodes to the latest firmware?\n\nThis will update nodes in sequence with a 30-second gap between each node.')) {
            return;
        }

        try {
            const response = await fetch('/api/nodes/update-all', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' }
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const data = await response.json();
            showToast(`Firmware updates started for ${data.count || 0} nodes`, 'success');
            fetchFleetData();

        } catch (error) {
            console.error('[FleetPage] Update all failed:', error);
            showToast('Failed to start firmware updates', 'error');
        }
    }

    function showRoleModal() {
        const count = state.selectedNodes.size;
        elements.roleNodeCount.textContent = count;
        openModal('role-modal');
    }

    async function confirmRoleAssignment() {
        const selected = document.querySelector('input[name="role-assignment"]:checked');
        if (!selected) {
            showToast('Please select a role', 'warning');
            return;
        }

        const newRole = selected.value;
        closeModal('role-modal');

        try {
            const macs = Array.from(state.selectedNodes);
            let successCount = 0;

            for (const mac of macs) {
                try {
                    const response = await fetch(`/api/nodes/${mac}/role`, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ role: newRole })
                    });

                    if (response.ok) {
                        successCount++;

                        // Update local state
                        const nodeIndex = state.nodes.findIndex(n => n.mac === mac);
                        if (nodeIndex !== -1) {
                            state.nodes[nodeIndex].role = newRole;
                        }
                    }
                } catch (error) {
                    console.error(`[FleetPage] Failed to update role for ${mac}:`, error);
                }
            }

            if (successCount > 0) {
                showToast(`Role updated for ${successCount} nodes`, 'success');
                fetchFleetData();
            }

            if (successCount < macs.length) {
                showToast(`Failed to update ${macs.length - successCount} nodes`, 'warning');
            }

        } catch (error) {
            console.error('[FleetPage] Role assignment failed:', error);
            showToast('Failed to assign roles', 'error');
        }
    }

    function showRemoveModal() {
        const macs = Array.from(state.selectedNodes);
        elements.removeNodeCount.textContent = macs.length;

        // List nodes to be removed
        const listHTML = '<ul>' + macs.map(mac => {
            const node = state.nodes.find(n => n.mac === mac);
            const label = node ? (node.name || node.label || formatMAC(mac)) : formatMAC(mac);
            return `<li>${escapeHtml(label)} (${escapeHtml(mac)})</li>`;
        }).join('') + '</ul>';

        elements.removeNodeList.innerHTML = listHTML;
        openModal('remove-modal');
    }

    async function confirmRemove() {
        closeModal('remove-modal');

        if (!confirm('This action cannot be undone. Remove selected nodes from the fleet?')) {
            return;
        }

        try {
            const macs = Array.from(state.selectedNodes);
            let successCount = 0;

            for (const mac of macs) {
                try {
                    const response = await fetch(`/api/nodes/${mac}`, {
                        method: 'DELETE'
                    });

                    if (response.ok) {
                        successCount++;
                        state.selectedNodes.delete(mac);
                    }
                } catch (error) {
                    console.error(`[FleetPage] Failed to remove ${mac}:`, error);
                }
            }

            if (successCount > 0) {
                showToast(`Removed ${successCount} nodes from fleet`, 'success');
                fetchFleetData();
            }

        } catch (error) {
            console.error('[FleetPage] Remove failed:', error);
            showToast('Failed to remove nodes', 'error');
        }
    }

    function showMoreActions(button, mac) {
        // For now, just show the same actions as the main buttons
        // This could be expanded to show a dropdown menu
        const node = state.nodes.find(n => n.mac === mac);
        if (!node) return;

        showToast(`More actions for ${node.name || node.label || formatMAC(mac)} coming soon`, 'info');
    }

    // ============================================
    // CSV Export
    // ============================================
    function downloadCSV() {
        const headers = [
            'MAC',
            'Label',
            'Status',
            'Firmware Version',
            'Uptime (s)',
            'Role',
            'Health Score',
            'Packet Rate (Hz)',
            'Temperature (C)',
            'Last Seen'
        ];

        const rows = state.filteredNodes.map(node => [
            node.mac,
            node.name || node.label || '',
            getNodeStatus(node),
            node.firmware_version || '',
            node.uptime_seconds || 0,
            node.role,
            (node.health_score || 0).toFixed(2),
            node.packet_rate || 0,
            node.temperature || '',
            new Date(node.last_seen_ms || 0).toISOString()
        ]);

        const csvContent = [
            headers.join(','),
            ...rows.map(row => row.map(v => `"${v}"`).join(','))
        ].join('\n');

        const blob = new Blob([csvContent], { type: 'text/csv' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `spaxel-fleet-${new Date().toISOString().slice(0, 10)}.csv`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);

        showToast(`Downloaded ${state.filteredNodes.length} nodes to CSV`, 'success');
    }

    // ============================================
    // Summary Update
    // ============================================
    function updateSummary() {
        const total = state.nodes.length;
        const online = state.nodes.filter(n => getNodeStatus(n) === 'online').length;

        elements.totalSummary.textContent = total;
        elements.onlineSummary.textContent = online;
    }

    // ============================================
    // Helper Functions
    // ============================================
    function getNodeStatus(node) {
        // Check if node is currently updating
        if (node.ota_in_progress) {
            return 'updating';
        }

        // Check if online (last seen within threshold)
        if (node.last_seen_ms) {
            const lastSeen = new Date(node.last_seen_ms);
            const now = new Date();
            const diff = now - lastSeen;

            if (diff < CONFIG.staleThresholdMs) {
                return 'online';
            }
        }

        return 'offline';
    }

    function isFirmwareOutdated(node) {
        if (!state.latestFirmware || !node.firmware_version) {
            return false;
        }
        return node.firmware_version !== state.latestFirmware;
    }

    function getHealthClass(score) {
        if (score >= 0.7) return 'good';
        if (score >= 0.4) return 'fair';
        return 'poor';
    }

    function getPacketRateClass(node) {
        const rate = node.packet_rate || 0;
        const configured = node.configured_rate || 20;
        const ratio = rate / configured;

        if (ratio > 0.9) return 'good';
        if (ratio > 0.7) return 'fair';
        return 'poor';
    }

    function getTemperatureClass(temp) {
        if (!temp) return '';
        if (temp > 75) return 'alert';
        return '';
    }

    function formatMAC(mac) {
        const parts = mac.split(':');
        if (parts.length === 6) {
            return parts.slice(0, 4).join(':');
        }
        return mac;
    }

    function truncateMAC(mac) {
        const parts = mac.split(':');
        if (parts.length === 6) {
            return parts.slice(0, 3).join(':') + '...';
        }
        return mac;
    }

    function formatRole(role) {
        const roleMap = {
            'tx': 'TX',
            'rx': 'RX',
            'tx_rx': 'TX-RX',
            'passive': 'Passive',
            'idle': 'Idle'
        };
        return roleMap[role] || role;
    }

    function formatUptime(seconds) {
        if (!seconds) return '--';

        const days = Math.floor(seconds / 86400);
        const hours = Math.floor((seconds % 86400) / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);

        if (days > 0) {
            return `${days}d ${hours}h`;
        } else if (hours > 0) {
            return `${hours}h ${minutes}m`;
        } else {
            return `${minutes}m`;
        }
    }

    function formatPacketRate(node) {
        const actual = node.packet_rate || 0;
        const configured = node.configured_rate || 20;
        return `${actual} / ${configured} Hz`;
    }

    function formatTemperature(temp) {
        if (!temp && temp !== 0) return '--';
        return `${Math.round(temp)}°C`;
    }

    function capitalize(str) {
        return str.charAt(0).toUpperCase() + str.slice(1);
    }

    function escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    // ============================================
    // Modal Functions
    // ============================================
    function openModal(modalId) {
        const modal = document.getElementById(modalId);
        if (modal) {
            modal.style.display = 'flex';
        }
    }

    function closeModal(modalId) {
        const modal = document.getElementById(modalId);
        if (modal) {
            modal.style.display = 'none';
        }
    }

    // ============================================
    // Toast Notifications
    // ============================================
    function showToast(message, type = 'info') {
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;

        const icons = {
            success: '&#x2714;',
            error: '&#x2716;',
            warning: '&#x26A0;',
            info: '&#x2139;'
        };

        toast.innerHTML = `
            <span class="toast-icon">${icons[type] || icons.info}</span>
            <span class="toast-message">${escapeHtml(message)}</span>
            <button class="toast-dismiss">&times;</button>
        `;

        elements.toastContainer.appendChild(toast);

        // Auto-dismiss after 5 seconds
        setTimeout(() => {
            toast.style.animation = 'slideIn 0.3s ease reverse';
            setTimeout(() => {
                toast.remove();
            }, 300);
        }, 5000);

        // Dismiss on click
        toast.querySelector('.toast-dismiss').addEventListener('click', () => {
            toast.remove();
        });
    }

    // ============================================
    // Utility Functions
    // ============================================
    function debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }

    // ============================================
    // Public API
    // ============================================
    window.FleetPage = {
        init,
        refresh: fetchFleetData,
        getState: () => state
    };

    // ============================================
    // Auto-init
    // ============================================
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
