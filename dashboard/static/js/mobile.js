/**
 * Spaxel Dashboard - Mobile Expert Mode
 *
 * Mobile-responsive enhancements for expert mode:
 * - Hamburger menu for panel navigation
 * - Bottom sheet panels for mobile
 * - Touch-optimized interactions
 * - No hover-dependent UI
 */

(function() {
    'use strict';

    // ============================================
    // State
    // ============================================
    let hamburgerMenu = null;
    let mobileNavOpen = false;
    let activeBottomSheet = null;

    // ============================================
    // Configuration
    // ============================================
    const MOBILE_BREAKPOINT = 768; // px
    const BOTTOM_SHEET_MAX_HEIGHT = '70vh';
    const BOTTOM_SHEET_MIN_HEIGHT = '40vh';

    // ============================================
    // Hamburger Menu
    // ============================================

    /**
     * Create hamburger menu button
     */
    function createHamburgerMenu() {
        if (document.getElementById('mobile-hamburger')) {
            return;
        }

        const hamburger = document.createElement('button');
        hamburger.id = 'mobile-hamburger';
        hamburger.className = 'mobile-hamburger';
        hamburger.setAttribute('aria-label', 'Open menu');
        hamburger.setAttribute('aria-expanded', 'false');
        hamburger.innerHTML = `
            <span class="hamburger-line"></span>
            <span class="hamburger-line"></span>
            <span class="hamburger-line"></span>
        `;

        hamburger.addEventListener('click', toggleMobileNav);

        // Insert into status bar
        const statusBar = document.getElementById('status-bar');
        if (statusBar) {
            statusBar.insertBefore(hamburger, statusBar.firstChild);
        }

        hamburgerMenu = hamburger;
        console.log('[Mobile] Hamburger menu created');
    }

    /**
     * Toggle mobile navigation
     */
    function toggleMobileNav() {
        mobileNavOpen = !mobileNavOpen;

        if (mobileNavOpen) {
            openMobileNav();
        } else {
            closeMobileNav();
        }
    }

    /**
     * Open mobile navigation
     */
    function openMobileNav() {
        mobileNavOpen = true;

        if (hamburgerMenu) {
            hamburgerMenu.classList.add('active');
            hamburgerMenu.setAttribute('aria-expanded', 'true');
        }

        // Close any open bottom sheets
        closeActiveBottomSheet();

        // Create mobile nav overlay
        let navOverlay = document.getElementById('mobile-nav-overlay');
        if (!navOverlay) {
            navOverlay = document.createElement('div');
            navOverlay.id = 'mobile-nav-overlay';
            navOverlay.className = 'mobile-nav-overlay';
            navOverlay.addEventListener('click', closeMobileNav);
            document.body.appendChild(navOverlay);
        }

        // Create mobile nav panel
        let navPanel = document.getElementById('mobile-nav-panel');
        if (!navPanel) {
            navPanel = createMobileNavPanel();
            document.body.appendChild(navPanel);
        }

        // Show navigation
        requestAnimationFrame(() => {
            navOverlay.classList.add('visible');
            navPanel.classList.add('visible');
        });

        // Disable OrbitControls
        if (window.Viz3D && window.Viz3D.controls) {
            const controls = window.Viz3D.controls();
            if (controls) controls.enabled = false;
        }
    }

    /**
     * Close mobile navigation
     */
    function closeMobileNav() {
        mobileNavOpen = false;

        if (hamburgerMenu) {
            hamburgerMenu.classList.remove('active');
            hamburgerMenu.setAttribute('aria-expanded', 'false');
        }

        const navOverlay = document.getElementById('mobile-nav-overlay');
        const navPanel = document.getElementById('mobile-nav-panel');

        if (navOverlay) navOverlay.classList.remove('visible');
        if (navPanel) navPanel.classList.remove('visible');

        // Re-enable OrbitControls
        if (window.Viz3D && window.Viz3D.controls) {
            const controls = window.Viz3D.controls();
            if (controls) controls.enabled = true;
        }
    }

    /**
     * Create mobile navigation panel
     */
    function createMobileNavPanel() {
        const panel = document.createElement('div');
        panel.id = 'mobile-nav-panel';
        panel.className = 'mobile-nav-panel';

        // Panel sections
        const sections = [
            {
                title: 'Panels',
                items: [
                    { id: 'fleet-status', label: 'Fleet Status', icon: '📡', action: openFleetStatus },
                    { id: 'zones', label: 'Zones', icon: '🏠', action: openZonesPanel },
                    { id: 'triggers', label: 'Automation', icon: '⚡', action: openTriggersPanel },
                    { id: 'settings', label: 'Settings', icon: '⚙️', action: openSettingsPanel }
                ]
            },
            {
                title: 'View',
                items: [
                    { id: 'reset-view', label: 'Reset View', icon: '🎯', action: resetView },
                    { id: 'toggle-layers', label: 'Toggle Layers', icon: '👁️', action: toggleLayers },
                    { id: 'simple-mode', label: 'Simple Mode', icon: '📱', action: switchToSimpleMode }
                ]
            }
        ];

        let html = '<div class="mobile-nav-content">';

        sections.forEach(section => {
            html += `
                <div class="mobile-nav-section">
                    <h3 class="mobile-nav-section-title">${section.title}</h3>
                    <div class="mobile-nav-items">
            `;

            section.items.forEach(item => {
                html += `
                    <button class="mobile-nav-item" data-action="${item.id}">
                        <span class="mobile-nav-icon">${item.icon}</span>
                        <span class="mobile-nav-label">${item.label}</span>
                    </button>
                `;
            });

            html += `
                    </div>
                </div>
            `;
        });

        html += '</div>';

        panel.innerHTML = html;

        // Add event listeners
        panel.querySelectorAll('.mobile-nav-item').forEach(item => {
            item.addEventListener('click', (e) => {
                const actionId = e.currentTarget.dataset.action;
                handleMobileNavAction(actionId);
            });
        });

        return panel;
    }

    /**
     * Handle mobile navigation actions
     */
    function handleMobileNavAction(actionId) {
        closeMobileNav();

        // Small delay to allow nav to close before opening panel
        setTimeout(() => {
            switch (actionId) {
                case 'fleet-status':
                    openFleetStatus();
                    break;
                case 'zones':
                    openZonesPanel();
                    break;
                case 'triggers':
                    openTriggersPanel();
                    break;
                case 'settings':
                    openSettingsPanel();
                    break;
                case 'reset-view':
                    resetView();
                    break;
                case 'toggle-layers':
                    toggleLayers();
                    break;
                case 'simple-mode':
                    if (window.SpaxelRouter) {
                        window.SpaxelRouter.navigate('simple');
                    }
                    break;
            }
        }, 300);
    }

    // ============================================
    // Panel Actions
    // ============================================

    function openFleetStatus() {
        if (window.SpaxelPanels) {
            openBottomSheet('fleet-status', 'Fleet Status', createFleetStatusContent());
        }
    }

    function openZonesPanel() {
        if (window.SpaxelPanels) {
            openBottomSheet('zones', 'Zones', createZonesContent());
        }
    }

    function openTriggersPanel() {
        if (window.SpaxelPanels) {
            openBottomSheet('triggers', 'Automation Triggers', createTriggersContent());
        }
    }

    function openSettingsPanel() {
        if (window.SpaxelPanels) {
            openBottomSheet('settings', 'Settings', createSettingsContent());
        }
    }

    function resetView() {
        if (window.Viz3D && window.Viz3D.resetView) {
            window.Viz3D.resetView();
        }
    }

    function toggleLayers() {
        if (window.Viz3D && window.Viz3D.toggleLayers) {
            window.Viz3D.toggleLayers();
        }
    }

    // ============================================
    // Bottom Sheet Panels (Mobile)
    // ============================================

    /**
     * Open bottom sheet panel on mobile
     */
    function openBottomSheet(id, title, content) {
        if (!isMobile()) {
            // On desktop, use regular sidebar
            window.SpaxelPanels.openSidebar({
                title: title,
                content: content,
                width: '360px',
                position: 'right'
            });
            return;
        }

        // Close existing bottom sheet
        closeActiveBottomSheet();

        // Create bottom sheet
        const sheet = document.createElement('div');
        sheet.id = `bottom-sheet-${id}`;
        sheet.className = 'mobile-bottom-sheet';

        sheet.innerHTML = `
            <div class="bottom-sheet-handle"></div>
            <div class="bottom-sheet-header">
                <h2 class="bottom-sheet-title">${title}</h2>
                <button class="bottom-sheet-close" aria-label="Close">&times;</button>
            </div>
            <div class="bottom-sheet-content"></div>
        `;

        // Add content
        const contentEl = sheet.querySelector('.bottom-sheet-content');
        if (typeof content === 'string') {
            contentEl.innerHTML = content;
        } else if (content instanceof HTMLElement) {
            contentEl.appendChild(content);
        } else if (typeof content === 'function') {
            const result = content(contentEl);
            if (result instanceof HTMLElement) {
                contentEl.innerHTML = '';
                contentEl.appendChild(result);
            }
        }

        // Close button handler
        sheet.querySelector('.bottom-sheet-close').addEventListener('click', closeActiveBottomSheet);

        // Handle drag to close
        setupBottomSheetDrag(sheet);

        // Add to DOM
        document.body.appendChild(sheet);

        // Disable OrbitControls
        if (window.Viz3D && window.Viz3D.controls) {
            const controls = window.Viz3D.controls();
            if (controls) controls.enabled = false;
        }

        // Show with animation
        requestAnimationFrame(() => {
            sheet.classList.add('visible');
        });

        activeBottomSheet = sheet;

        console.log('[Mobile] Bottom sheet opened:', id);
    }

    /**
     * Close active bottom sheet
     */
    function closeActiveBottomSheet() {
        if (!activeBottomSheet) return;

        const sheet = activeBottomSheet;

        sheet.classList.remove('visible');

        setTimeout(() => {
            if (sheet.parentNode) {
                sheet.parentNode.removeChild(sheet);
            }

            // Re-enable OrbitControls
            if (window.Viz3D && window.Viz3D.controls) {
                const controls = window.Viz3D.controls();
                if (controls) controls.enabled = true;
            }

            activeBottomSheet = null;
        }, 300);
    }

    /**
     * Set up bottom sheet drag to close
     */
    function setupBottomSheetDrag(sheet) {
        const handle = sheet.querySelector('.bottom-sheet-handle');
        const content = sheet.querySelector('.bottom-sheet-content');
        let startY = 0;
        let currentY = 0;
        let isDragging = false;

        handle.addEventListener('touchstart', (e) => {
            startY = e.touches[0].clientY;
            isDragging = true;
        }, { passive: true });

        handle.addEventListener('touchmove', (e) => {
            if (!isDragging) return;

            currentY = e.touches[0].clientY;
            const deltaY = currentY - startY;

            // Only allow dragging down
            if (deltaY > 0) {
                const translateY = Math.min(deltaY, window.innerHeight * 0.7);
                sheet.style.transform = `translateY(${translateY}px)`;
            }
        }, { passive: true });

        handle.addEventListener('touchend', () => {
            if (!isDragging) return;
            isDragging = false;

            const deltaY = currentY - startY;

            // If dragged more than 100px down, close the sheet
            if (deltaY > 100) {
                closeActiveBottomSheet();
            } else {
                // Reset position
                sheet.style.transform = '';
            }
        });
    }

    // ============================================
    // Content Generators
    // ============================================

    function createFleetStatusContent(container) {
        // Fetch fleet status
        fetch('/api/nodes')
            .then(response => response.json())
            .then(nodes => {
                if (nodes.length === 0) {
                    container.innerHTML = '<div class="mobile-empty">No nodes found</div>';
                    return;
                }

                let html = '<div class="mobile-list">';

                nodes.forEach(node => {
                    const statusClass = node.status === 'online' ? 'status-online' :
                                       node.status === 'stale' ? 'status-stale' : 'status-offline';
                    html += `
                        <div class="mobile-list-item" data-mac="${node.mac}">
                            <div class="mobile-list-status ${statusClass}"></div>
                            <div class="mobile-list-content">
                                <div class="mobile-list-title">${node.name || node.mac}</div>
                                <div class="mobile-list-subtitle">${node.role || 'Unknown'} • ${node.firmware_version || 'Unknown'}</div>
                            </div>
                            <button class="mobile-list-action" aria-label="Options">›</button>
                        </div>
                    `;
                });

                html += '</div>';

                container.innerHTML = html;

                // Add click handlers
                container.querySelectorAll('.mobile-list-item').forEach(item => {
                    const mac = item.dataset.mac;
                    item.addEventListener('click', () => {
                        // Open node details
                        if (window.SpatialQuickActions) {
                            const node = nodes.find(n => n.mac === mac);
                            if (node) {
                                window.SpatialQuickActions.show(
                                    window.innerWidth / 2,
                                    window.innerHeight / 2,
                                    'node',
                                    node
                                );
                            }
                        }
                    });
                });
            })
            .catch(error => {
                console.error('[Mobile] Error fetching fleet status:', error);
                container.innerHTML = '<div class="mobile-error">Failed to load fleet status</div>';
            });
    }

    function createZonesContent(container) {
        // Fetch zones
        fetch('/api/zones')
            .then(response => response.json())
            .then(zones => {
                if (zones.length === 0) {
                    container.innerHTML = '<div class="mobile-empty">No zones defined</div>';
                    return;
                }

                let html = '<div class="mobile-list">';

                zones.forEach(zone => {
                    const occupancy = zone.occupancy || 0;
                    const people = zone.people || [];
                    html += `
                        <div class="mobile-list-item" data-zone-id="${zone.id}">
                            <div class="mobile-list-icon">🏠</div>
                            <div class="mobile-list-content">
                                <div class="mobile-list-title">${zone.name}</div>
                                <div class="mobile-list-subtitle">${occupancy} person${occupancy !== 1 ? 's' : ''}</div>
                            </div>
                            <button class="mobile-list-action" aria-label="Options">›</button>
                        </div>
                    `;
                });

                html += '</div>';

                container.innerHTML = html;

                // Add click handlers
                container.querySelectorAll('.mobile-list-item').forEach(item => {
                    const zoneId = item.dataset.zoneId;
                    item.addEventListener('click', () => {
                        const zone = zones.find(z => z.id == zoneId);
                        if (zone && window.SpatialQuickActions) {
                            window.SpatialQuickActions.show(
                                window.innerWidth / 2,
                                window.innerHeight / 2,
                                'zone',
                                zone
                            );
                        }
                    });
                });
            })
            .catch(error => {
                console.error('[Mobile] Error fetching zones:', error);
                container.innerHTML = '<div class="mobile-error">Failed to load zones</div>';
            });
    }

    function createTriggersContent(container) {
        // Fetch triggers
        fetch('/api/triggers')
            .then(response => response.json())
            .then(triggers => {
                if (triggers.length === 0) {
                    container.innerHTML = '<div class="mobile-empty">No automation triggers</div>';
                    return;
                }

                let html = '<div class="mobile-list">';

                triggers.forEach(trigger => {
                    const enabledClass = trigger.enabled ? 'trigger-enabled' : 'trigger-disabled';
                    html += `
                        <div class="mobile-list-item" data-trigger-id="${trigger.id}">
                            <div class="mobile-list-icon ${enabledClass}">⚡</div>
                            <div class="mobile-list-content">
                                <div class="mobile-list-title">${trigger.name}</div>
                                <div class="mobile-list-subtitle">${trigger.condition || 'Unknown condition'}</div>
                            </div>
                            <button class="mobile-list-action" aria-label="Options">›</button>
                        </div>
                    `;
                });

                html += '</div>';

                container.innerHTML = html;

                // Add click handlers
                container.querySelectorAll('.mobile-list-item').forEach(item => {
                    const triggerId = item.dataset.triggerId;
                    item.addEventListener('click', () => {
                        const trigger = triggers.find(t => t.id == triggerId);
                        if (trigger && window.SpatialQuickActions) {
                            window.SpatialQuickActions.show(
                                window.innerWidth / 2,
                                window.innerHeight / 2,
                                'trigger',
                                trigger
                            );
                        }
                    });
                });
            })
            .catch(error => {
                console.error('[Mobile] Error fetching triggers:', error);
                container.innerHTML = '<div class="mobile-error">Failed to load triggers</div>';
            });
    }

    function createSettingsContent() {
        return `
            <div class="mobile-list">
                <div class="mobile-list-item" data-setting="detection">
                    <div class="mobile-list-icon">🎯</div>
                    <div class="mobile-list-content">
                        <div class="mobile-list-title">Detection</div>
                        <div class="mobile-list-subtitle">Thresholds & sensitivity</div>
                    </div>
                    <button class="mobile-list-action">›</button>
                </div>
                <div class="mobile-list-item" data-setting="display">
                    <div class="mobile-list-icon">🎨</div>
                    <div class="mobile-list-content">
                        <div class="mobile-list-title">Display</div>
                        <div class="mobile-list-subtitle">Appearance & layers</div>
                    </div>
                    <button class="mobile-list-action">›</button>
                </div>
                <div class="mobile-list-item" data-setting="notifications">
                    <div class="mobile-list-icon">🔔</div>
                    <div class="mobile-list-content">
                        <div class="mobile-list-title">Notifications</div>
                        <div class="mobile-list-subtitle">Alerts & push</div>
                    </div>
                    <button class="mobile-list-action">›</button>
                </div>
                <div class="mobile-list-item" data-setting="integrations">
                    <div class="mobile-list-icon">🔗</div>
                    <div class="mobile-list-content">
                        <div class="mobile-list-title">Integrations</div>
                        <div class="mobile-list-subtitle">Home Assistant, MQTT</div>
                    </div>
                    <button class="mobile-list-action">›</button>
                </div>
                <div class="mobile-list-item" data-setting="help">
                    <div class="mobile-list-icon">❓</div>
                    <div class="mobile-list-content">
                        <div class="mobile-list-title">Help & Troubleshooting</div>
                        <div class="mobile-list-subtitle">Guides & diagnostics</div>
                    </div>
                    <button class="mobile-list-action">›</button>
                </div>
            </div>
        `;
    }

    // ============================================
    // Utilities
    // ============================================

    /**
     * Check if on mobile device
     */
    function isMobile() {
        return window.innerWidth < MOBILE_BREAKPOINT ||
               ('ontouchstart' in window && navigator.maxTouchPoints > 0);
    }

    /**
     * Update hamburger menu visibility based on screen size
     */
    function updateHamburgerVisibility() {
        if (!hamburgerMenu) return;

        if (isMobile()) {
            hamburgerMenu.style.display = 'flex';
        } else {
            hamburgerMenu.style.display = 'none';
            closeMobileNav();
        }
    }

    // ============================================
    // Public API
    // ============================================
    window.MobileExpertMode = {
        init: init,
        openNav: openMobileNav,
        closeNav: closeMobileNav,
        openBottomSheet: openBottomSheet,
        closeBottomSheet: closeActiveBottomSheet,
        isMobile: isMobile
    };

    // ============================================
    // Initialization
    // ============================================
    function init() {
        console.log('[Mobile] Initializing mobile expert mode...');

        // Create hamburger menu
        createHamburgerMenu();

        // Update visibility on resize
        window.addEventListener('resize', updateHamburgerVisibility);
        updateHamburgerVisibility();

        // Handle orientation changes
        window.addEventListener('orientationchange', () => {
            setTimeout(updateHamburgerVisibility, 100);
        });

        console.log('[Mobile] Mobile expert mode initialized');
    }

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[Mobile] Module loaded');
})();
