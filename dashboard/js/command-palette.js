/**
 * Spaxel Dashboard - Command Palette (Ctrl+K / Cmd+K)
 *
 * Universal search and command interface for power users.
 * Fuzzy matching across zones, people, nodes, events, settings, and help topics.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const STORAGE_KEY_RECENT = 'spaxel_command_recent';
    const STORAGE_KEY_HINT_DISMISSED = 'spaxel_command_hint_dismissed';
    const MAX_RECENT = 5;
    const MIN_SEARCH_LENGTH = 1;

    // ============================================
    // State
    // ============================================
    let isOpen = false;
    let selectedIndex = 0;
    let searchQuery = '';
    let recentCommands = [];
    let commandRegistry = new Map();
    let searchResults = [];

    // ============================================
    // Command Registry
    // ============================================

    /**
     * Register a command
     */
    function registerCommand(id, config) {
        commandRegistry.set(id, {
            id: id,
            title: config.title,
            description: config.description || '',
            category: config.category || 'general',
            icon: config.icon || '&#x2699;',
            keywords: config.keywords || [],
            action: config.action,
            shortcut: config.shortcut || null
        });

        console.log('[Command Palette] Registered command:', id);
    }

    /**
     * Initialize default commands
     */
    function initializeCommands() {
        // Navigation commands
        registerCommand('nav-live', {
            title: 'Live View',
            description: 'Go to real-time 3D detection view',
            category: 'navigation',
            icon: '&#x25A0;',
            keywords: ['live', '3d', 'view', 'home', 'dashboard'],
            action: () => navigateToMode('live')
        });

        registerCommand('nav-timeline', {
            title: 'Timeline',
            description: 'View activity history and events',
            category: 'navigation',
            icon: '&#x23F0;',
            keywords: ['timeline', 'history', 'events', 'activity', 'log'],
            action: () => navigateToMode('timeline')
        });

        registerCommand('nav-automations', {
            title: 'Automations',
            description: 'Manage spatial triggers and automation rules',
            category: 'navigation',
            icon: '&#x2699;',
            keywords: ['automations', 'triggers', 'rules', 'automation'],
            action: () => navigateToMode('automations')
        });

        registerCommand('nav-settings', {
            title: 'Settings',
            description: 'Open system configuration and preferences',
            category: 'navigation',
            icon: '&#x2699;',
            keywords: ['settings', 'config', 'preferences', 'options'],
            action: () => {
                if (window.openSettingsPanel) {
                    window.openSettingsPanel();
                }
            }
        });

        registerCommand('nav-ambient', {
            title: 'Ambient Mode',
            description: 'Switch to always-on display mode',
            category: 'navigation',
            icon: '&#x25C9;',
            keywords: ['ambient', 'display', 'wall', 'tablet'],
            action: () => navigateToMode('ambient')
        });

        registerCommand('nav-replay', {
            title: 'Replay Mode',
            description: 'Enter time-travel debugging mode',
            category: 'navigation',
            icon: '&#x23F5;',
            keywords: ['replay', 'debug', 'time', 'travel', 'history'],
            action: () => {
                if (window.SpaxelReplay) {
                    window.SpaxelReplay.pauseLive();
                } else {
                    navigateToMode('replay');
                }
            }
        });

        registerCommand('nav-simulator', {
            title: 'Pre-Deployment Simulator',
            description: 'Open simulator for testing coverage',
            category: 'navigation',
            icon: '&#x269B;',
            keywords: ['simulator', 'simulation', 'test', 'coverage', 'planning'],
            action: () => {
                if (window.Simulate) {
                    window.Simulate.togglePanel();
                } else {
                    navigateToMode('simulate');
                }
            }
        });

        // Fleet commands
        registerCommand('fleet-update-all', {
            title: 'Update All Nodes',
            description: 'Trigger OTA firmware update for all nodes',
            category: 'fleet',
            icon: '&#x2B06;',
            keywords: ['update', 'ota', 'firmware', 'upgrade', 'all nodes'],
            action: () => confirmAndExecute('Update all nodes?', async () => {
                const response = await fetch('/api/nodes/update-all', { method: 'POST' });
                if (response.ok) {
                    showToast('Firmware update started for all nodes', 'success');
                } else {
                    showToast('Failed to start firmware update', 'warning');
                }
            })
        });

        registerCommand('fleet-rebaseline-all', {
            title: 'Re-baseline All',
            description: 'Recalibrate detection baselines for all links',
            category: 'fleet',
            icon: '&#x267B;',
            keywords: ['baseline', 'calibrate', 'recalibrate', 'reset'],
            action: () => confirmAndExecute('Re-baseline all links?', async () => {
                const response = await fetch('/api/nodes/rebaseline-all', { method: 'POST' });
                if (response.ok) {
                    showToast('Re-baseline started for all links', 'success');
                } else {
                    showToast('Failed to start re-baseline', 'warning');
                }
            })
        });

        registerCommand('fleet-export-config', {
            title: 'Export Configuration',
            description: 'Download full system configuration as JSON',
            category: 'fleet',
            icon: '&#x1F4E5;',
            keywords: ['export', 'download', 'backup', 'save', 'config'],
            action: async () => {
                const response = await fetch('/api/export');
                if (response.ok) {
                    const config = await response.json();
                    const blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' });
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = `spaxel-config-${new Date().toISOString().split('T')[0]}.json`;
                    a.click();
                    URL.revokeObjectURL(url);
                    showToast('Configuration exported', 'success');
                } else {
                    showToast('Failed to export configuration', 'warning');
                }
            }
        });

        // Security commands
        registerCommand('security-arm', {
            title: 'Arm Security Mode',
            description: 'Enable security alerts for any motion detection',
            category: 'security',
            icon: '&#x1F512;',
            keywords: ['arm', 'security', 'alert', 'enable', 'protect'],
            action: () => confirmAndExecute('Arm security mode?', async () => {
                const response = await fetch('/api/security/arm', { method: 'POST' });
                if (response.ok) {
                    showToast('Security mode armed', 'success');
                } else {
                    showToast('Failed to arm security mode', 'warning');
                }
            })
        });

        registerCommand('security-disarm', {
            title: 'Disarm Security Mode',
            description: 'Disable security alerts',
            category: 'security',
            icon: '&#x1F513;',
            keywords: ['disarm', 'security', 'disable', 'off'],
            action: () => confirmAndExecute('Disarm security mode?', async () => {
                const response = await fetch('/api/security/disarm', { method: 'POST' });
                if (response.ok) {
                    showToast('Security mode disarmed', 'info');
                } else {
                    showToast('Failed to disarm security mode', 'warning');
                }
            })
        });

        // View commands
        registerCommand('view-toggle-gdop', {
            title: 'Toggle GDOP Overlay',
            description: 'Show/hide coverage quality map',
            category: 'view',
            icon: '&#x1F4C5;',
            keywords: ['gdop', 'coverage', 'map', 'overlay', 'quality'],
            action: () => {
                if (window.Placement) {
                    window.Placement.toggleGDOP();
                }
            }
        });

        registerCommand('view-toggle-fresnel', {
            title: 'Toggle Fresnel Zones',
            description: 'Show/hide Fresnel zone visualization',
            category: 'view',
            icon: '&#x25CA;',
            keywords: ['fresnel', 'zones', 'visualization', 'debug'],
            action: () => {
                if (window.toggleFresnelZones) {
                    window.toggleFresnelZones();
                }
            }
        });

        registerCommand('view-toggle-links', {
            title: 'Toggle Node Links',
            description: 'Show/hide link visualization',
            category: 'view',
            icon: '&#x1F4DE;',
            keywords: ['links', 'nodes', 'connections', 'lines'],
            action: () => {
                if (window.Viz3D) {
                    window.Viz3D.toggleLinks();
                }
            }
        });

        registerCommand('view-top-down', {
            title: 'Top-Down View',
            description: 'Switch to overhead view',
            category: 'view',
            icon: '&#x2195;',
            keywords: ['top', 'down', 'overhead', 'plan', 'birdseye'],
            action: () => {
                if (window.Viz3D) {
                    window.Viz3D.setViewPreset('topdown');
                }
            }
        });

        registerCommand('view-perspective', {
            title: 'Perspective View',
            description: 'Switch to 3D perspective view',
            category: 'view',
            icon: '&#x1F3E0;',
            keywords: ['perspective', '3d', 'angle', 'view'],
            action: () => {
                if (window.Viz3D) {
                    window.Viz3D.setViewPreset('perspective');
                }
            }
        });

        // Mode commands
        registerCommand('mode-simple', {
            title: 'Switch to Simple Mode',
            description: 'Enable card-based mobile-first UI',
            category: 'mode',
            icon: '&#x1F4F1;',
            keywords: ['simple', 'basic', 'easy', 'mobile'],
            action: () => {
                if (window.SpaxelSimpleMode) {
                    window.SpaxelSimpleMode.enable();
                }
            }
        });

        registerCommand('mode-expert', {
            title: 'Switch to Expert Mode',
            description: 'Enable full 3D visualization',
            category: 'mode',
            icon: '&#x2699;',
            keywords: ['expert', 'advanced', 'full', '3d'],
            action: () => {
                if (window.SpaxelSimpleMode) {
                    window.SpaxelSimpleMode.disable();
                }
            }
        });

        // Help commands
        registerCommand('help-fall-detection', {
            title: 'Fall Detection Help',
            description: 'Learn about fall detection settings',
            category: 'help',
            icon: '&#x2753;',
            keywords: ['fall', 'detection', 'help', 'explain', 'how'],
            action: () => showHelpTopic('fall-detection')
        });

        registerCommand('help-accuracy', {
            title: 'Improve Accuracy',
            description: 'Tips for improving detection accuracy',
            category: 'help',
            icon: '&#x2753;',
            keywords: ['accuracy', 'improve', 'better', 'tips', 'help'],
            action: () => showHelpTopic('accuracy')
        });

        registerCommand('help-why-false-positive', {
            title: 'Why False Positive?',
            description: 'Explain most recent incorrect detection',
            category: 'help',
            icon: '&#x2753;',
            keywords: ['why', 'false', 'positive', 'explain', 'help'],
            action: () => showHelpTopic('false-positive')
        });

        registerCommand('help-keyboard', {
            title: 'Keyboard Shortcuts',
            description: 'View all available keyboard shortcuts',
            category: 'help',
            icon: '&#x2328;',
            keywords: ['keyboard', 'shortcuts', 'hotkey', 'keys', 'help'],
            action: () => showHelpTopic('shortcuts')
        });

        // Theme commands
        registerCommand('theme-dark', {
            title: 'Dark Mode',
            description: 'Switch to dark theme',
            category: 'theme',
            icon: '&#x1F319;',
            keywords: ['dark', 'theme', 'night', 'mode'],
            action: () => setTheme('dark')
        });

        registerCommand('theme-light', {
            title: 'Light Mode',
            description: 'Switch to light theme',
            category: 'theme',
            icon: '&#x2600;',
            keywords: ['light', 'theme', 'day', 'mode'],
            action: () => setTheme('light')
        });

        // Node commands (dynamically populated)
        registerCommand('node-add', {
            title: 'Add New Node',
            description: 'Start onboarding for a new ESP32 node',
            category: 'nodes',
            icon: '&#x2795;',
            keywords: ['add', 'node', 'new', 'onboard', 'provision'],
            action: () => {
                if (window.SpaxelOnboard) {
                    window.SpaxelOnboard.start();
                }
            }
        });

        registerCommand('node-restart', {
            title: 'Restart Node',
            description: 'Restart a specific node (select from list)',
            category: 'nodes',
            icon: '&#x1F504;',
            keywords: ['restart', 'reboot', 'node', 'reset'],
            action: () => showNodeSelector('restart')
        });

        registerCommand('node-update', {
            title: 'Update Node Firmware',
            description: 'Trigger OTA update for a specific node',
            category: 'nodes',
            icon: '&#x2B06;',
            keywords: ['update', 'ota', 'firmware', 'upgrade', 'node'],
            action: () => showNodeSelector('update')
        });

        // Zone commands (dynamically populated)
        registerCommand('zone-history', {
            title: 'Zone History',
            description: 'View occupancy history for a zone',
            category: 'zones',
            icon: '&#x1F4C5;',
            keywords: ['zone', 'history', 'occupancy', 'log'],
            action: () => showZoneSelector('history')
        });

        console.log('[Command Palette] Commands initialized:', commandRegistry.size);
    }

    // ============================================
    // UI Creation
    // ============================================

    /**
     * Create the command palette UI
     */
    function createCommandPalette() {
        // Check if already exists
        if (document.getElementById('command-palette')) {
            return;
        }

        const palette = document.createElement('div');
        palette.id = 'command-palette';
        palette.className = 'command-palette';
        palette.innerHTML = `
            <div class="command-backdrop"></div>
            <div class="command-container">
                <div class="command-header">
                    <span class="command-icon">&#x269B;</span>
                    <input
                        type="text"
                        class="command-input"
                        placeholder="Search commands, zones, people, nodes..."
                        autocomplete="off"
                        spellcheck="false"
                    >
                    <span class="command-hint">ESC to close</span>
                </div>
                <div class="command-body">
                    <div class="command-results"></div>
                    <div class="command-empty" style="display: none;">
                        <div class="empty-icon">&#x1F50D;</div>
                        <div class="empty-text">No results found</div>
                        <div class="empty-hint">Try a different search term</div>
                    </div>
                </div>
                <div class="command-footer">
                    <div class="footer-sections">
                        <div class="footer-hints">
                            <span class="hint-item"><kbd>&#x2191;</kbd><kbd>&#x2193;</kbd> Navigate</span>
                            <span class="hint-item"><kbd>Enter</kbd> Select</span>
                            <span class="hint-item"><kbd>Esc</kbd> Close</span>
                        </div>
                    </div>
                </div>
            </div>
        `;

        document.body.appendChild(palette);

        // Set up event listeners
        setupEventListeners();

        console.log('[Command Palette] UI created');
    }

    /**
     * Set up event listeners
     */
    function setupEventListeners() {
        const palette = document.getElementById('command-palette');
        const input = palette.querySelector('.command-input');
        const backdrop = palette.querySelector('.command-backdrop');

        // Close on backdrop click
        backdrop.addEventListener('click', closePalette);

        // Close on Escape
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && isOpen) {
                closePalette();
            }
        });

        // Global keyboard shortcut (Ctrl+K / Cmd+K)
        document.addEventListener('keydown', (e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
                e.preventDefault();
                togglePalette();
            }
        });

        // Input handling
        input.addEventListener('input', (e) => {
            searchQuery = e.target.value;
            selectedIndex = 0;
            performSearch();
        });

        input.addEventListener('keydown', handleInputKeydown);

        // Result clicks
        palette.querySelector('.command-results').addEventListener('click', (e) => {
            const item = e.target.closest('.command-item');
            if (item) {
                executeCommand(item.dataset.commandId);
            }
        });
    }

    /**
     * Handle keyboard navigation in input
     */
    function handleInputKeydown(e) {
        if (!isOpen) return;

        const results = document.querySelectorAll('.command-item');

        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                selectedIndex = Math.min(selectedIndex + 1, results.length - 1);
                updateSelection();
                break;

            case 'ArrowUp':
                e.preventDefault();
                selectedIndex = Math.max(selectedIndex - 1, 0);
                updateSelection();
                break;

            case 'Enter':
                e.preventDefault();
                if (results[selectedIndex]) {
                    executeCommand(results[selectedIndex].dataset.commandId);
                }
                break;

            case 'PageDown':
                e.preventDefault();
                selectedIndex = Math.min(selectedIndex + 5, results.length - 1);
                updateSelection();
                break;

            case 'PageUp':
                e.preventDefault();
                selectedIndex = Math.max(selectedIndex - 5, 0);
                updateSelection();
                break;

            case 'Home':
                e.preventDefault();
                selectedIndex = 0;
                updateSelection();
                break;

            case 'End':
                e.preventDefault();
                selectedIndex = results.length - 1;
                updateSelection();
                break;
        }
    }

    /**
     * Update selection highlight
     */
    function updateSelection() {
        const results = document.querySelectorAll('.command-item');
        results.forEach((item, index) => {
            item.classList.toggle('selected', index === selectedIndex);
        });

        // Scroll selected item into view
        if (results[selectedIndex]) {
            results[selectedIndex].scrollIntoView({
                block: 'nearest',
                behavior: 'smooth'
            });
        }
    }

    // ============================================
    // Palette Control
    // ============================================

    /**
     * Check if command palette should be available
     */
    function isAvailable() {
        // Check if simple mode is active
        if (window.SpaxelSimpleMode && window.SpaxelSimpleMode.isEnabled()) {
            return false;
        }

        // Check if ambient mode is active
        if (window.SpaxelAmbient && window.SpaxelAmbient.isEnabled()) {
            return false;
        }

        return true;
    }

    /**
     * Toggle command palette open/closed
     */
    function togglePalette() {
        if (isOpen) {
            closePalette();
        } else {
            openPalette();
        }
    }

    /**
     * Open command palette
     */
    function openPalette() {
        // Check if command palette is available in current mode
        if (!isAvailable()) {
            showToast('Command palette is only available in expert mode', 'info');
            return;
        }

        createCommandPalette();

        const palette = document.getElementById('command-palette');
        const input = palette.querySelector('.command-input');

        palette.classList.add('visible');
        isOpen = true;

        // Focus input
        setTimeout(() => {
            input.focus();
            input.select();
        }, 100);

        // Load initial results
        searchQuery = '';
        selectedIndex = 0;
        performSearch();

        console.log('[Command Palette] Opened');
    }

    /**
     * Close command palette
     */
    function closePalette() {
        const palette = document.getElementById('command-palette');
        if (palette) {
            palette.classList.remove('visible');
            isOpen = false;

            // Clear input after animation
            setTimeout(() => {
                const input = palette.querySelector('.command-input');
                if (input) {
                    input.value = '';
                    searchQuery = '';
                }
            }, 200);
        }

        console.log('[Command Palette] Closed');
    }

    // ============================================
    // Search
    // ============================================

    /**
     * Perform fuzzy search
     */
    function performSearch() {
        const resultsContainer = document.querySelector('.command-results');
        const emptyState = document.querySelector('.command-empty');

        if (searchQuery.length < MIN_SEARCH_LENGTH) {
            // Show all commands by category
            searchResults = getAllCommands();
        } else {
            // Fuzzy search
            searchResults = fuzzySearch(searchQuery);
        }

        if (searchResults.length === 0) {
            resultsContainer.style.display = 'none';
            emptyState.style.display = 'block';
        } else {
            resultsContainer.style.display = 'block';
            emptyState.style.display = 'none';
            renderResults(searchResults);
        }

        selectedIndex = 0;
        updateSelection();
    }

    /**
     * Get all commands organized by category
     */
    function getAllCommands() {
        const commands = Array.from(commandRegistry.values());
        const categories = {};

        // Group by category
        commands.forEach(cmd => {
            if (!categories[cmd.category]) {
                categories[cmd.category] = [];
            }
            categories[cmd.category].push(cmd);
        });

        // Convert to flat list with category headers
        const results = [];
        const categoryOrder = ['recent', 'navigation', 'fleet', 'security', 'nodes', 'zones', 'view', 'mode', 'theme', 'help'];

        // Add recent commands first
        if (recentCommands.length > 0) {
            results.push({
                type: 'header',
                title: 'Recent'
            });

            recentCommands.forEach(cmdId => {
                const cmd = commandRegistry.get(cmdId);
                if (cmd) {
                    results.push({ type: 'command', ...cmd });
                }
            });
        }

        // Add categorized commands
        categoryOrder.forEach(category => {
            if (categories[category]) {
                results.push({
                    type: 'header',
                    title: formatCategoryTitle(category)
                });

                categories[category].forEach(cmd => {
                    results.push({ type: 'command', ...cmd });
                });
            }
        });

        return results;
    }

    /**
     * Fuzzy search implementation
     */
    function fuzzySearch(query) {
        const searchLower = query.toLowerCase();
        const commands = Array.from(commandRegistry.values());
        const scored = [];

        commands.forEach(cmd => {
            let score = 0;
            const titleLower = cmd.title.toLowerCase();
            const descLower = cmd.description.toLowerCase();
            const keywordsLower = cmd.keywords.map(k => k.toLowerCase());

            // Exact title match
            if (titleLower === searchLower) {
                score = 100;
            }
            // Title starts with query
            else if (titleLower.startsWith(searchLower)) {
                score = 80;
            }
            // Title contains query
            else if (titleLower.includes(searchLower)) {
                score = 60;
            }
            // Keyword match
            else if (keywordsLower.some(k => k.includes(searchLower))) {
                score = 50;
            }
            // Description contains query
            else if (descLower.includes(searchLower)) {
                score = 30;
            }
            // Fuzzy match (consecutive characters)
            else {
                const fuzzyScore = fuzzyMatch(searchLower, titleLower);
                if (fuzzyScore > 0) {
                    score = fuzzyScore * 0.4;
                }
            }

            // Boost recent commands
            if (recentCommands.includes(cmd.id)) {
                score += 10;
            }

            if (score > 0) {
                scored.push({ command: cmd, score });
            }
        });

        // Sort by score and convert to results
        scored.sort((a, b) => b.score - a.score);

        // Group by category for top results
        const results = [];
        const categories = new Set();

        scored.slice(0, 20).forEach(({ command: cmd }) => {
            if (!categories.has(cmd.category)) {
                categories.add(cmd.category);
                results.push({
                    type: 'header',
                    title: formatCategoryTitle(cmd.category)
                });
            }
            results.push({ type: 'command', ...cmd });
        });

        return results;
    }

    /**
     * Simple fuzzy matching
     */
    function fuzzyMatch(query, text) {
        let queryIndex = 0;
        let textIndex = 0;
        let score = 0;
        let consecutive = 0;

        while (queryIndex < query.length && textIndex < text.length) {
            if (query[queryIndex] === text[textIndex]) {
                consecutive++;
                score += consecutive * 2;
                queryIndex++;
            } else {
                consecutive = 0;
            }
            textIndex++;
        }

        return queryIndex === query.length ? score : 0;
    }

    /**
     * Render search results
     */
    function renderResults(results) {
        const container = document.querySelector('.command-results');
        let html = '';

        results.forEach(result => {
            if (result.type === 'header') {
                html += `
                    <div class="command-category-header">
                        ${result.title}
                    </div>
                `;
            } else {
                html += `
                    <div class="command-item" data-command-id="${result.id}">
                        <span class="command-icon">${result.icon}</span>
                        <div class="command-content">
                            <div class="command-title">${highlightMatch(result.title, searchQuery)}</div>
                            <div class="command-description">${highlightMatch(result.description, searchQuery)}</div>
                        </div>
                        ${result.shortcut ? `<span class="command-shortcut">${result.shortcut}</span>` : ''}
                    </div>
                `;
            }
        });

        container.innerHTML = html;
    }

    /**
     * Highlight matching text in search results
     */
    function highlightMatch(text, query) {
        if (!query || query.length < MIN_SEARCH_LENGTH) {
            return text;
        }

        const regex = new RegExp(`(${escapeRegex(query)})`, 'gi');
        return text.replace(regex, '<mark>$1</mark>');
    }

    /**
     * Escape regex special characters
     */
    function escapeRegex(string) {
        return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }

    /**
     * Format category title
     */
    function formatCategoryTitle(category) {
        const titles = {
            'recent': 'Recent',
            'navigation': 'Navigation',
            'fleet': 'Fleet Management',
            'security': 'Security',
            'nodes': 'Node Management',
            'zones': 'Zones',
            'view': 'View Controls',
            'mode': 'Display Mode',
            'theme': 'Theme',
            'help': 'Help & Documentation'
        };

        return titles[category] || category.charAt(0).toUpperCase() + category.slice(1);
    }

    // ============================================
    // Command Execution
    // ============================================

    /**
     * Execute a command
     */
    async function executeCommand(commandId) {
        const command = commandRegistry.get(commandId);
        if (!command) {
            console.error('[Command Palette] Unknown command:', commandId);
            return;
        }

        console.log('[Command Palette] Executing:', commandId);

        // Add to recent commands
        addToRecent(commandId);

        // Close palette
        closePalette();

        // Execute action
        try {
            await command.action();
        } catch (error) {
            console.error('[Command Palette] Command error:', error);
            showToast(`Command failed: ${error.message}`, 'warning');
        }
    }

    /**
     * Add command to recent list
     */
    function addToRecent(commandId) {
        // Remove if already exists
        recentCommands = recentCommands.filter(id => id !== commandId);

        // Add to front
        recentCommands.unshift(commandId);

        // Trim to max
        if (recentCommands.length > MAX_RECENT) {
            recentCommands = recentCommands.slice(0, MAX_RECENT);
        }

        // Save to localStorage
        localStorage.setItem(STORAGE_KEY_RECENT, JSON.stringify(recentCommands));
    }

    /**
     * Load recent commands from storage
     */
    function loadRecentCommands() {
        try {
            const stored = localStorage.getItem(STORAGE_KEY_RECENT);
            if (stored) {
                recentCommands = JSON.parse(stored);
            }
        } catch (e) {
            console.error('[Command Palette] Error loading recent commands:', e);
            recentCommands = [];
        }
    }

    // ============================================
    // Helper Functions
    // ============================================

    /**
     * Navigate to a mode
     */
    function navigateToMode(mode) {
        if (window.SpaxelRouter) {
            window.SpaxelRouter.navigate(mode);
        } else {
            window.location.hash = mode;
        }
    }

    /**
     * Confirm and execute an action
     */
    async function confirmAndExecute(message, action) {
        const confirmed = confirm(message);
        if (confirmed) {
            await action();
        }
    }

    /**
     * Show node selector
     */
    async function showNodeSelector(action) {
        // Fetch nodes
        const response = await fetch('/api/nodes');
        if (!response.ok) {
            showToast('Failed to load nodes', 'warning');
            return;
        }

        const nodes = await response.json();

        // Create selection UI
        const options = nodes.map(node =>
            `<option value="${node.mac}">${node.name || node.mac}</option>`
        ).join('');

        const selected = prompt(`Select node:\n${nodes.map((n, i) => `${i + 1}. ${n.name || n.mac}`).join('\n')}`);

        if (selected) {
            const node = nodes.find(n => n.mac === selected || n.name === selected);
            if (node) {
                executeNodeAction(action, node);
            }
        }
    }

    /**
     * Execute node action
     */
    async function executeNodeAction(action, node) {
        switch (action) {
            case 'restart':
                await fetch(`/api/nodes/${node.mac}/reboot`, { method: 'POST' });
                showToast(`Restarting ${node.name || node.mac}`, 'info');
                break;

            case 'update':
                await fetch(`/api/nodes/${node.mac}/update`, { method: 'POST' });
                showToast(`Updating ${node.name || node.mac}`, 'info');
                break;
        }
    }

    /**
     * Show zone selector
     */
    async function showZoneSelector(action) {
        // Fetch zones
        const response = await fetch('/api/zones');
        if (!response.ok) {
            showToast('Failed to load zones', 'warning');
            return;
        }

        const zones = await response.json();
        const zoneNames = zones.map(z => z.name).join(', ');
        const selected = prompt(`Enter zone name:\nAvailable: ${zoneNames}`);

        if (selected) {
            const zone = zones.find(z => z.name.toLowerCase() === selected.toLowerCase());
            if (zone) {
                // Open zone history or navigate to it
                if (window.SpaxelRouter) {
                    window.SpaxelRouter.navigate('timeline');
                }
            }
        }
    }

    /**
     * Show help topic
     */
    function showHelpTopic(topic) {
        const helpContent = {
            'fall-detection': {
                title: 'Fall Detection',
                content: `
                    <h3>How Fall Detection Works</h3>
                    <p>Spaxel detects falls by monitoring rapid Z-axis movement followed by sustained stillness:</p>
                    <ul>
                        <li><strong>Trigger:</strong> Z velocity exceeds -1.5 m/s AND Z drops below 0.5m</li>
                        <li><strong>Confirmation:</strong> Blob remains below 0.5m with low motion for 10+ seconds</li>
                    </ul>
                    <p><strong>Requirements:</strong></p>
                    <ul>
                        <li>At least 2 nodes above 1.5m height in the zone</li>
                        <li>Mixed-height node placement for Z-axis resolution</li>
                    </ul>
                    <p><strong>Reducing False Positives:</strong></p>
                    <p>Fall detection is designed to distinguish falls from:</p>
                    <ul>
                        <li>Lying on a couch (no rapid descent)</li>
                        <li>Picking something up (person rises within 10s)</li>
                        <li>Bedroom zones suppress alerts during sleep hours (21:00-07:00)</li>
                    </ul>
                `
            },
            'accuracy': {
                title: 'Improving Accuracy',
                content: `
                    <h3>Tips for Better Detection</h3>
                    <p><strong>Node Placement:</strong></p>
                    <ul>
                        <li>Place nodes at different heights (mix of low and high)</li>
                        <li>Avoid collinear placement - create angular diversity</li>
                        <li>Use the GDOP overlay to find optimal positions</li>
                    </ul>
                    <p><strong>Environment:</strong></p>
                    <ul>
                        <li>Minimize sources of RF interference (microwaves, some baby monitors)</li>
                        <li>Keep nodes away from large metal objects</li>
                        <li>Avoid placing nodes too close to WiFi routers</li>
                    </ul>
                    <p><strong>Calibration:</strong></p>
                    <ul>
                        <li>Run re-baseline after moving furniture</li>
                        <li>Allow 7 days for diurnal baseline learning</li>
                        <li>Use the feedback buttons (thumbs up/down) on detections</li>
                    </ul>
                    <p><strong>Advanced:</strong></p>
                    <ul>
                        <li>Add BLE devices for person identification</li>
                        <li>Enable self-improving weights for automatic tuning</li>
                        <li>Check link health panel for degraded links</li>
                    </ul>
                `
            },
            'false-positive': {
                title: 'Why Did This Happen?',
                content: `
                    <p>Analyzing recent detection...</p>
                    <p><strong>Possible causes:</strong></p>
                    <ul>
                        <li>Environmental: HVAC, appliances, or other RF sources</li>
                        <li>Moving objects: fans, curtains, pets</li>
                        <li>Baseline drift: system adapted to a changed environment</li>
                        <li>Link geometry: Fresnel zones may overlap problem areas</li>
                    </ul>
                    <p><strong>What to do:</strong></p>
                    <ul>
                        <li>Use thumbs down feedback to mark this as incorrect</li>
                        <li>Check link health for affected links</li>
                        <li>Consider re-baselining if environment changed</li>
                        <li>Review placement of nodes near the detection area</li>
                    </ul>
                `
            },
            'shortcuts': {
                title: 'Keyboard Shortcuts',
                content: `
                    <h3>Global Shortcuts</h3>
                    <ul>
                        <li><kbd>Ctrl</kbd> + <kbd>K</kbd> - Open command palette</li>
                        <li><kbd>Esc</kbd> - Close modals/palettes</li>
                    </ul>
                    <h3>3D View</h3>
                    <ul>
                        <li><kbd>Mouse drag</kbd> - Rotate camera</li>
                        <li><kbd>Scroll</kbd> - Zoom in/out</li>
                        <li><kbd>Right-click drag</kbd> - Pan camera</li>
                        <li><kbd>Double-click</kbd> - Focus on node/blob</li>
                    </ul>
                    <h3>Touch (Mobile)</h3>
                    <ul>
                        <li><kbd>One finger drag</kbd> - Rotate</li>
                        <li><kbd>Two finger pinch</kbd> - Zoom</li>
                        <li><kbd>Two finger drag</kbd> - Pan</li>
                    </ul>
                    <h3>Replay Mode</h3>
                    <ul>
                        <li><kbd>Space</kbd> - Play/pause</li>
                        <li><kbd>←</kbd> / <kbd>→</kbd> - Step frame</li>
                        <li><kbd>Shift</kbd> + <kbd>←</kbd> / <kbd>→</kbd> - Skip 10 frames</li>
                    </ul>
                `
            }
        };

        const help = helpContent[topic];
        if (help) {
            // Show help modal
            showHelpModal(help.title, help.content);
        } else {
            showToast('Help topic not found', 'warning');
        }
    }

    /**
     * Show help modal
     */
    function showHelpModal(title, content) {
        const modal = document.createElement('div');
        modal.className = 'command-help-modal visible';
        modal.innerHTML = `
            <div class="help-backdrop"></div>
            <div class="help-container">
                <div class="help-header">
                    <h3>${title}</h3>
                    <button class="help-close">&times;</button>
                </div>
                <div class="help-content">${content}</div>
            </div>
        `;

        document.body.appendChild(modal);

        // Close handlers
        modal.addEventListener('click', (e) => {
            if (e.target === modal || e.target.classList.contains('help-backdrop') || e.target.classList.contains('help-close')) {
                modal.remove();
            }
        });
    }

    /**
     * Set theme
     */
    function setTheme(theme) {
        document.documentElement.setAttribute('data-theme', theme);
        localStorage.setItem('spaxel_theme', theme);
        showToast(`Switched to ${theme} mode`, 'info');
    }

    /**
     * Show toast notification
     */
    function showToast(message, type = 'info') {
        if (window.showToast) {
            window.showToast(message, type);
            return;
        }

        // Fallback toast
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.textContent = message;
        toast.style.cssText = `
            position: fixed;
            bottom: 20px;
            left: 50%;
            transform: translateX(-50%);
            background: rgba(0, 0, 0, 0.9);
            color: white;
            padding: 12px 20px;
            border-radius: 8px;
            z-index: 1000;
        `;

        document.body.appendChild(toast);

        setTimeout(() => {
            toast.style.animation = 'fadeOut 0.3s ease-out forwards';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    }

    // ============================================
    // Initialization
    // ============================================

    /**
     * Initialize the command palette
     */
    function init() {
        console.log('[Command Palette] Initializing...');

        // Load recent commands
        loadRecentCommands();

        // Initialize default commands
        initializeCommands();

        // Create UI (hidden)
        createCommandPalette();

        // Show keyboard shortcut hint if not dismissed
        showShortcutHintIfNeeded();

        console.log('[Command Palette] Initialized');
    }

    /**
     * Show dismissible keyboard shortcut hint
     */
    function showShortcutHintIfNeeded() {
        // Check if already dismissed
        if (localStorage.getItem(STORAGE_KEY_HINT_DISMISSED)) {
            return;
        }

        // Check if expert mode is active
        if (!isAvailable()) {
            return;
        }

        // Create hint element
        const hint = document.createElement('div');
        hint.id = 'command-palette-shortcut-hint';
        hint.className = 'command-shortcut-hint';
        hint.innerHTML = `
            <span class="hint-text">Press <kbd>Ctrl</kbd> + <kbd>K</kbd> to open command palette</span>
            <button class="hint-dismiss" aria-label="Dismiss">&times;</button>
        `;

        // Add to body
        document.body.appendChild(hint);

        // Show after a short delay
        setTimeout(() => {
            hint.classList.add('visible');
        }, 1000);

        // Handle dismiss
        hint.querySelector('.hint-dismiss').addEventListener('click', () => {
            hint.classList.remove('visible');
            setTimeout(() => {
                hint.remove();
                localStorage.setItem(STORAGE_KEY_HINT_DISMISSED, 'true');
            }, 300);
        });

        // Auto-hide after 10 seconds
        setTimeout(() => {
            if (hint.parentNode) {
                hint.classList.remove('visible');
                setTimeout(() => {
                    if (hint.parentNode) {
                        hint.remove();
                    }
                }, 300);
            }
        }, 10000);
    }

    // ============================================
    // Public API
    // ============================================
    window.CommandPalette = {
        init: init,
        open: openPalette,
        close: closePalette,
        toggle: togglePalette,
        register: registerCommand,
        execute: executeCommand,
        isOpen: () => isOpen
    };

    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[Command Palette] Module loaded');
})();
