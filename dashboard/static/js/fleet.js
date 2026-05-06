/**
 * Spaxel Fleet Status Module
 *
 * Core fleet management functionality for:
 * - Fleet data fetching and state management
 * - Node operations (identify, reboot, update, remove)
 * - Bulk operations (update all, re-baseline all)
 * - Role assignment
 * - Config export/import
 * - Camera fly-to integration
 *
 * This module can be used by both the fleet page and fleet panel.
 */

(function() {
    'use strict';

    // ============================================
    // Constants
    // ============================================
    const CONFIG = {
        pollIntervalMs: 10000,    // Poll every 10 seconds
        staleThresholdMs: 30000,  // Node considered stale after 30s
        otaStaggerMs: 30000,      // 30 second stagger between OTA updates
        apiBase: '/api'
    };

    const NODE_STATUS = {
        ONLINE: 'online',
        OFFLINE: 'offline',
        STALE: 'stale',
        UPDATING: 'updating',
        UNPAIRED: 'unpaired'
    };

    const VALID_ROLES = ['tx', 'rx', 'tx_rx', 'passive', 'idle'];

    // ============================================
    // State
    // ============================================
    const state = {
        nodes: [],
        selectedNodes: new Set(),
        latestFirmware: null,
        filters: {
            search: '',
            status: '',
            firmware: '',
            roles: []
        },
        sortColumn: null,
        sortDirection: 'asc'
    };

    // ============================================
    // API Functions
    // ============================================
    async function fetchFleet() {
        const response = await fetch(`${CONFIG.apiBase}/fleet`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        return await response.json();
    }

    async function fetchFirmwareList() {
        const response = await fetch(`${CONFIG.apiBase}/firmware`);
        if (!response.ok) {
            return null;
        }
        return await response.json();
    }

    async function updateNodeLabel(mac, label) {
        const response = await fetch(`${CONFIG.apiBase}/nodes/${mac}/label`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ label })
        });
        if (!response.ok) {
            throw new Error(`Failed to update label: HTTP ${response.status}`);
        }
        return await response.json();
    }

    async function setNodeRole(mac, role) {
        if (!VALID_ROLES.includes(role)) {
            throw new Error(`Invalid role: ${role}`);
        }
        const response = await fetch(`${CONFIG.apiBase}/nodes/${mac}/role`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ role })
        });
        if (!response.ok) {
            throw new Error(`Failed to set role: HTTP ${response.status}`);
        }
        return await response.json();
    }

    async function identifyNode(mac, durationMs = 5000) {
        const response = await fetch(`${CONFIG.apiBase}/nodes/${mac}/identify`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ duration_ms: durationMs })
        });
        if (!response.ok) {
            throw new Error(`Failed to identify node: HTTP ${response.status}`);
        }
        return await response.json();
    }

    async function updateNodeFirmware(mac) {
        const response = await fetch(`${CONFIG.apiBase}/nodes/${mac}/update`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        if (!response.ok) {
            throw new Error(`Failed to start OTA: HTTP ${response.status}`);
        }
        return await response.json();
    }

    async function updateAllFirmware() {
        const response = await fetch(`${CONFIG.apiBase}/nodes/update-all`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        if (!response.ok) {
            throw new Error(`Failed to update all: HTTP ${response.status}`);
        }
        return await response.json();
    }

    async function removeNode(mac) {
        const response = await fetch(`${CONFIG.apiBase}/nodes/${mac}`, {
            method: 'DELETE'
        });
        if (!response.ok) {
            throw new Error(`Failed to remove node: HTTP ${response.status}`);
        }
        return await response.json();
    }

    async function rebaselineNode(mac) {
        const response = await fetch(`${CONFIG.apiBase}/nodes/${mac}/rebaseline`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        if (!response.ok) {
            throw new Error(`Failed to re-baseline: HTTP ${response.status}`);
        }
        return await response.json();
    }

    async function rebaselineAll() {
        const response = await fetch(`${CONFIG.apiBase}/nodes/rebaseline-all`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        if (!response.ok) {
            throw new Error(`Failed to re-baseline all: HTTP ${response.status}`);
        }
        return await response.json();
    }

    async function exportConfig() {
        const response = await fetch(`${CONFIG.apiBase}/export`);
        if (!response.ok) {
            throw new Error(`Failed to export: HTTP ${response.status}`);
        }
        return await response.json();
    }

    async function importConfig(configData) {
        const response = await fetch(`${CONFIG.apiBase}/import`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(configData)
        });
        if (!response.ok) {
            throw new Error(`Failed to import: HTTP ${response.status}`);
        }
        return await response.json();
    }

    // ============================================
    // Helper Functions
    // ============================================
    function getNodeStatus(node) {
        if (node.unpaired) {
            return NODE_STATUS.UNPAIRED;
        }
        if (node.ota_in_progress) {
            return NODE_STATUS.UPDATING;
        }
        if (node.last_seen_ms) {
            const lastSeen = new Date(node.last_seen_ms);
            const now = new Date();
            const diff = now - lastSeen;
            if (diff < CONFIG.staleThresholdMs) {
                return NODE_STATUS.ONLINE;
            }
        }
        return NODE_STATUS.OFFLINE;
    }

    function isFirmwareOutdated(node) {
        if (!state.latestFirmware || !node.firmware_version) {
            return false;
        }
        return node.firmware_version !== state.latestFirmware;
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

    function formatPosition(node) {
        if (node.pos_x !== undefined && node.pos_y !== undefined && node.pos_z !== undefined) {
            return `(${node.pos_x.toFixed(1)}, ${node.pos_y.toFixed(1)}, ${node.pos_z.toFixed(1)})`;
        }
        return '--';
    }

    function getHealthClass(score) {
        if (score >= 0.7) return 'good';
        if (score >= 0.4) return 'fair';
        return 'poor';
    }

    function escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    // ============================================
    // Camera Fly-To Integration
    // ============================================
    function flyToNode(mac) {
        // Store target MAC in localStorage for live view
        localStorage.setItem('fleetFlyToMAC', mac);
        // If on fleet page, navigate to live view
        if (window.location.pathname === '/fleet' || window.location.pathname.endsWith('/fleet.html')) {
            window.location.href = '/?highlight=' + mac;
        } else {
            // Trigger custom event for live view to handle
            window.dispatchEvent(new CustomEvent('fleet-flyto-node', {
                detail: { mac }
            }));
        }
    }

    // ============================================
    // Filtering and Sorting
    // ============================================
    function applyFilters(nodes) {
        let filtered = nodes.slice();

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

        return filtered;
    }

    function getSortValue(node, column) {
        switch (column) {
            case 'label':
                return node.name || node.label || '';
            case 'mac':
                return node.mac;
            case 'status':
                const status = getNodeStatus(node);
                return status === 'online' ? 2 : status === 'updating' ? 1 : 0;
            case 'firmware':
                return node.firmware_version || '';
            case 'uptime':
                return node.uptime_seconds || 0;
            case 'role':
                return node.role;
            case 'health':
                return node.health_score || 0;
            case 'position':
                return (node.pos_x || 0) + (node.pos_y || 0) + (node.pos_z || 0);
            default:
                return '';
        }
    }

    // ============================================
    // Bulk Operations
    // ============================================
    async function performBulkOTA(macs) {
        const results = [];
        for (let i = 0; i < macs.length; i++) {
            const mac = macs[i];
            try {
                await updateNodeFirmware(mac);
                results.push({ mac, success: true });
            } catch (error) {
                results.push({ mac, success: false, error: error.message });
            }
            // Stagger updates (except last one)
            if (i < macs.length - 1) {
                await new Promise(resolve => setTimeout(resolve, CONFIG.otaStaggerMs));
            }
        }
        return results;
    }

    async function performBulkRoleChange(macs, newRole) {
        const results = [];
        for (const mac of macs) {
            try {
                await setNodeRole(mac, newRole);
                results.push({ mac, success: true });
            } catch (error) {
                results.push({ mac, success: false, error: error.message });
            }
        }
        return results;
    }

    async function performBulkRemoval(macs) {
        const results = [];
        for (const mac of macs) {
            try {
                await removeNode(mac);
                results.push({ mac, success: true });
            } catch (error) {
                results.push({ mac, success: false, error: error.message });
            }
        }
        return results;
    }

    // ============================================
    // CSV Export
    // ============================================
    function downloadCSV(nodes) {
        const headers = [
            'MAC',
            'Label',
            'Status',
            'Firmware Version',
            'Uptime (s)',
            'Role',
            'Position (x,y,z)',
            'Health Score',
            'Packet Rate (Hz)',
            'Temperature (C)',
            'Last Seen'
        ];

        const rows = nodes.map(node => [
            node.mac,
            node.name || node.label || '',
            getNodeStatus(node),
            node.firmware_version || '',
            node.uptime_seconds || 0,
            node.role,
            formatPosition(node),
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
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelFleet = {
        // State
        getState: () => state,

        // Constants
        CONFIG,
        NODE_STATUS,
        VALID_ROLES,

        // API Functions
        fetchFleet,
        fetchFirmwareList,
        updateNodeLabel,
        setNodeRole,
        identifyNode,
        updateNodeFirmware,
        updateAllFirmware,
        removeNode,
        rebaselineNode,
        rebaselineAll,
        exportConfig,
        importConfig,

        // Helper Functions
        getNodeStatus,
        isFirmwareOutdated,
        formatMAC,
        truncateMAC,
        formatRole,
        formatUptime,
        formatPosition,
        getHealthClass,
        escapeHtml,

        // Filtering and Sorting
        applyFilters,
        getSortValue,

        // Camera Fly-To
        flyToNode,

        // Bulk Operations
        performBulkOTA,
        performBulkRoleChange,
        performBulkRemoval,

        // CSV Export
        downloadCSV
    };

    console.log('[SpaxelFleet] Fleet module loaded');
})();
