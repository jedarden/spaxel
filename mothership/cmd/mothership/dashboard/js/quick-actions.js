/**
 * Spaxel Dashboard - Spatial Quick Actions
 *
 * Right-click (desktop) or long-press (mobile) context menus
 * on 3D elements for context-sensitive actions.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const LONG_PRESS_DURATION = 500; // ms
    const MAX_DISTANCE = 10; // pixels for touch move

    // ============================================
    // State
    // ============================================
    let contextMenu = null;
    let activeTarget = null;
    let longPressTimer = null;
    let touchStartPos = null;
    let followIndicator = null;

    // ============================================
    // Target Types and Actions
    // ============================================

    // Action items for different target types
    // Dividers are represented by { divider: true }
    const ACTIONS = {
        blob: [
            {
                id: 'identify',
                label: 'Who is this?',
                icon: '&#x1F464;',
                description: 'Assign person identity',
                action: (blob) => identifyPerson(blob)
            },
            {
                id: 'follow',
                label: 'Follow (camera)',
                icon: '&#x1F50D;',
                description: 'Camera tracks this person',
                action: (blob) => followBlob(blob)
            },
            {
                id: 'track-history',
                label: 'View history',
                icon: '&#x1F4C5;',
                description: 'Jump to timeline for this person',
                action: (blob) => showBlobHistory(blob)
            },
            {
                id: 'incorrect',
                label: 'Mark as false positive',
                icon: '&#x1F44E;',
                description: 'Report incorrect detection',
                action: (blob) => markIncorrect(blob)
            },
            {
                id: 'why',
                label: 'Explain detection',
                icon: '&#x2753;',
                description: 'Show why this was detected',
                action: (blob) => explainBlob(blob)
            },
            { divider: true },
            {
                id: 'set-unknown',
                label: 'Set as unknown (anonymous)',
                icon: '&#x2754;',
                description: 'Remove identity assignment',
                action: (blob) => setBlobUnknown(blob)
            }
        ],
        node: [
            {
                id: 'edit-label',
                label: 'Edit label',
                icon: '&#x270F;',
                description: 'Rename this node',
                action: (node) => editNodeLabel(node)
            },
            {
                id: 'diagnostics',
                label: 'View health details',
                icon: '&#x1F4CA;',
                description: 'View link health and CSI',
                action: (node) => showNodeDiagnostics(node)
            },
            {
                id: 'update',
                label: 'Trigger OTA update',
                icon: '&#x2B06;',
                description: 'Update firmware on this node',
                action: (node) => updateNodeFirmware(node)
            },
            {
                id: 'identify-led',
                label: 'Locate node (blink LED)',
                icon: '&#x1F4A1;',
                description: 'Blink node LED for 5 seconds',
                action: (node) => blinkNodeLED(node)
            },
            {
                id: 'reassign-role',
                label: 'Re-assign role',
                icon: '&#x2699;',
                description: 'Change node role (TX/RX/TX-RX/passive)',
                action: (node) => reassignNodeRole(node)
            },
            { divider: true },
            {
                id: 'delete',
                label: 'Remove from fleet',
                icon: '&#x1F5D1;',
                description: 'Disconnect and remove this node',
                action: (node) => removeNode(node)
            }
        ],
        zone: [
            {
                id: 'edit-bounds',
                label: 'Edit zone bounds',
                icon: '&#x274F;',
                description: 'Resize zone boundaries',
                action: (zone) => editZoneBounds(zone)
            },
            {
                id: 'rename',
                label: 'Rename zone',
                icon: '&#x270F;',
                description: 'Change zone name',
                action: (zone) => renameZone(zone)
            },
            {
                id: 'history',
                label: 'View occupancy history',
                icon: '&#x1F4C5;',
                description: 'View occupancy over time',
                action: (zone) => showZoneHistory(zone)
            },
            {
                id: 'automation',
                label: 'Create automation for this zone',
                icon: '&#x2699;',
                description: 'Set up trigger for this zone',
                action: (zone) => createZoneAutomation(zone)
            },
            { divider: true },
            {
                id: 'delete',
                label: 'Delete zone',
                icon: '&#x1F5D1;',
                description: 'Remove this zone',
                action: (zone) => deleteZone(zone)
            }
        ],
        empty: [
            {
                id: 'add-virtual',
                label: 'Add virtual node here',
                icon: '&#x2795;',
                description: 'Place a virtual node at this position',
                action: (pos) => addVirtualNode(pos)
            },
            {
                id: 'create-zone',
                label: 'Create zone here',
                icon: '&#x26F6;',
                description: 'Start zone creation mode',
                action: (pos) => createZoneHere(pos)
            },
            {
                id: 'set-home',
                label: 'Set as home point',
                icon: '&#x1F3E0;',
                description: 'Set coordinate origin to this position',
                action: (pos) => setHomeAsPoint(pos)
            },
            {
                id: 'place-portal',
                label: 'Place portal here',
                icon: '&#x1F6AA;',
                description: 'Start portal creation mode',
                action: (pos) => placePortalHere(pos)
            }
        ],
        portal: [
            {
                id: 'edit',
                label: 'Edit portal',
                icon: '&#x270F;',
                description: 'Enter portal edit mode',
                action: (portal) => editPortal(portal)
            },
            {
                id: 'crossings',
                label: 'View crossing history',
                icon: '&#x1F4C5;',
                description: 'View recent crossings',
                action: (portal) => showPortalCrossings(portal)
            },
            { divider: true },
            {
                id: 'delete',
                label: 'Delete portal',
                icon: '&#x1F5D1;',
                description: 'Remove this portal',
                action: (portal) => deletePortal(portal)
            }
        ],
        trigger: [
            {
                id: 'edit',
                label: 'Edit trigger',
                icon: '&#x270F;',
                description: 'Open automation in builder',
                action: (trigger) => editTrigger(trigger)
            },
            {
                id: 'test',
                label: 'Test fire',
                icon: '&#x1F4AF;',
                description: 'Fire automation with test_mode=true',
                action: (trigger) => testTrigger(trigger)
            },
            {
                id: 'toggle',
                label: 'Enable / Disable',
                icon: '&#x1F6AB;',
                description: 'Toggle automation enabled state',
                action: (trigger) => toggleTrigger(trigger)
            },
            { divider: true },
            {
                id: 'delete',
                label: 'Delete trigger volume',
                icon: '&#x1F5D1;',
                description: 'Delete volume and associated trigger',
                action: (trigger) => deleteTrigger(trigger)
            }
        ]
    };

    // ============================================
    // Context Menu UI
    // ============================================

    /**
     * Create context menu
     */
    function createContextMenu() {
        if (document.getElementById('context-menu')) {
            return;
        }

        const menu = document.createElement('div');
        menu.id = 'context-menu';
        menu.className = 'context-menu';
        menu.innerHTML = `
            <div class="context-backdrop"></div>
            <div class="context-container">
                <div class="context-header">
                    <span class="context-icon" id="context-icon">&#x2699;</span>
                    <span class="context-title" id="context-title">Actions</span>
                </div>
                <div class="context-body" id="context-body">
                    <!-- Actions will be populated dynamically -->
                </div>
            </div>
        `;

        document.body.appendChild(menu);

        // Set up event listeners
        const backdrop = menu.querySelector('.context-backdrop');
        backdrop.addEventListener('click', closeContextMenu);

        console.log('[Quick Actions] Context menu created');
    }

    /**
     * Show context menu
     */
    function showContextMenu(x, y, targetType, target) {
        createContextMenu();

        const menu = document.getElementById('context-menu');
        const iconEl = document.getElementById('context-icon');
        const titleEl = document.getElementById('context-title');
        const bodyEl = document.getElementById('context-body');

        if (!menu) return;

        // Store target
        activeTarget = { type: targetType, data: target };
        contextMenu = menu;

        // Set title based on target type
        const titles = {
            blob: target.person ? `${target.person}` : 'Person',
            node: target.name || target.mac,
            zone: target.name,
            empty: 'Location',
            portal: target.name,
            trigger: target.name
        };

        titleEl.textContent = titles[targetType] || 'Actions';

        // Set icon
        const icons = {
            blob: '&#x1F464;',
            node: '&#x1F4F1;',
            zone: '&#x1F3E0;',
            empty: '&#x1F30E;',
            portal: '&#x1F6AA;',
            trigger: '&#x2699;'
        };
        iconEl.innerHTML = icons[targetType] || '&#x2699;';

        // Populate actions (including dividers)
        const actions = ACTIONS[targetType] || [];
        let actionHTML = '';
        actions.forEach(action => {
            if (action.divider) {
                actionHTML += '<div class="context-divider"></div>';
            } else {
                actionHTML += `
                    <div class="context-item" data-action-id="${action.id}">
                        <span class="item-icon">${action.icon}</span>
                        <div class="item-content">
                            <div class="item-label">${action.label}</div>
                            <div class="item-description">${action.description}</div>
                        </div>
                    </div>
                `;
            }
        });
        bodyEl.innerHTML = actionHTML;

        // Position menu
        positionMenu(x, y);

        // Set target type on menu for styling
        menu.dataset.target = targetType;

        // Show menu
        menu.classList.add('visible');

        // Disable OrbitControls while menu is open
        if (window.Viz3D) {
            const controls = window.Viz3D.controls ? window.Viz3D.controls() : null;
            if (controls) {
                controls.enabled = false;
            }
        }

        // Set up action listeners (only for non-divider items)
        bodyEl.querySelectorAll('.context-item').forEach(item => {
            item.addEventListener('click', () => {
                executeAction(item.dataset.actionId);
                closeContextMenu();
            });
        });

        // Set up Escape key to dismiss
        setupEscapeKeyHandler();
    }

    /**
     * Position context menu intelligently
     */
    function positionMenu(x, y) {
        const menu = document.getElementById('context-menu');
        if (!menu) return;

        const container = menu.querySelector('.context-container');
        const viewportWidth = window.innerWidth;
        const viewportHeight = window.innerHeight;

        // Get container dimensions
        const rect = container.getBoundingClientRect();
        const width = rect.width || 300;
        const height = rect.height || 400;

        // Calculate position (keep within viewport)
        let left = x + 10;
        let top = y + 10;

        // Adjust if off-screen
        if (left + width > viewportWidth) {
            left = x - width - 10;
        }

        if (top + height > viewportHeight) {
            top = y - height - 10;
        }

        // Ensure minimum margins
        left = Math.max(10, Math.min(left, viewportWidth - width - 10));
        top = Math.max(10, Math.min(top, viewportHeight - height - 10));

        container.style.left = left + 'px';
        container.style.top = top + 'px';
    }

    /**
     * Close context menu
     */
    function closeContextMenu() {
        const menu = document.getElementById('context-menu');
        if (menu) {
            menu.classList.remove('visible');
            delete menu.dataset.target;
        }

        activeTarget = null;
        contextMenu = null;

        // Re-enable OrbitControls when menu closes
        if (window.Viz3D) {
            const controls = window.Viz3D.controls ? window.Viz3D.controls() : null;
            if (controls) {
                controls.enabled = true;
            }
        }

        // Remove Escape key handler
        teardownEscapeKeyHandler();
    }

    /**
     * Set up Escape key handler for dismissing context menu
     */
    let escapeKeyHandler = null;

    function setupEscapeKeyHandler() {
        // Remove existing handler if any
        teardownEscapeKeyHandler();

        escapeKeyHandler = function(event) {
            if (event.key === 'Escape') {
                closeContextMenu();
            }
        };

        document.addEventListener('keydown', escapeKeyHandler);
    }

    function teardownEscapeKeyHandler() {
        if (escapeKeyHandler) {
            document.removeEventListener('keydown', escapeKeyHandler);
            escapeKeyHandler = null;
        }
    }

    // ============================================
    // Action Execution
    // ============================================

    /**
     * Execute a context menu action
     */
    async function executeAction(actionId) {
        if (!activeTarget) {
            console.error('[Quick Actions] No active target');
            return;
        }

        const { type, data } = activeTarget;
        const actions = ACTIONS[type] || [];
        const action = actions.find(a => a.id === actionId);

        if (!action) {
            console.error('[Quick Actions] Unknown action:', actionId);
            return;
        }

        console.log('[Quick Actions] Executing:', actionId, 'on', type, data);

        try {
            await action.action(data);
            showToast(`${action.label} executed`, 'info');
        } catch (error) {
            console.error('[Quick Actions] Action error:', error);
            showToast(`Action failed: ${error.message}`, 'warning');
        }
    }

    // ============================================
    // Blob Actions
    // ============================================

    function followBlob(blob) {
        if (window.Viz3D && window.Viz3D.setFollowTarget) {
            window.Viz3D.setFollowTarget(blob.id);
            showFollowIndicator(blob);
        } else {
            showToast('3D view not available', 'warning');
        }
    }

    function showFollowIndicator(blob) {
        // Remove existing indicator
        if (followIndicator) {
            followIndicator.remove();
        }

        // Create follow indicator
        followIndicator = document.createElement('div');
        followIndicator.className = 'follow-mode-indicator';

        const personName = blob.person || 'Blob #' + blob.id;
        followIndicator.innerHTML = `
            <span>&#x1F50D;</span>
            <span>Following ${personName}</span>
            <button class="follow-stop-btn" style="margin-left:12px;padding:4px 8px;border-radius:4px;border:none;background:rgba(255,255,255,0.2);color:white;cursor:pointer;">Stop</button>
        `;

        // Set up stop button handler
        const stopBtn = followIndicator.querySelector('.follow-stop-btn');
        if (stopBtn) {
            stopBtn.addEventListener('click', function() {
                stopFollowing();
            });
        }

        document.body.appendChild(followIndicator);

        // Set up scroll wheel zoom handler for follow mode
        setupFollowZoomHandler();

        // Also set up ESC key to stop following
        document.addEventListener('keydown', handleFollowEscape);
    }

    /**
     * Set up scroll wheel zoom handler for follow mode
     * Allows zoom adjustment while in follow mode
     */
    let followZoomHandler = null;

    function setupFollowZoomHandler() {
        // Remove existing handler if any
        teardownFollowZoomHandler();

        followZoomHandler = function(event) {
            if (!window.Viz3D) return;

            const controls = window.Viz3D.controls ? window.Viz3D.controls() : null;
            if (!controls) return;

            // Adjust zoom by changing camera distance
            // Positive delta (scroll up) = zoom in, Negative delta (scroll down) = zoom out
            const zoomSpeed = 0.001;
            const zoomFactor = Math.exp(-event.deltaY * zoomSpeed);

            // Get current camera position relative to target
            const currentDistance = controls.object.position.distanceTo(controls.target);

            // Apply zoom factor with limits
            const minDistance = 2;  // Minimum 2 meters from target
            const maxDistance = 20; // Maximum 20 meters from target
            const newDistance = Math.max(minDistance, Math.min(maxDistance, currentDistance * zoomFactor));

            // Calculate new camera position along the same direction
            const direction = new THREE.Vector3();
            controls.object.position.sub(controls.target).normalize().multiplyScalar(newDistance).add(controls.target);

            controls.update();
        };

        // Attach to canvas
        const canvas = document.querySelector('#viz-canvas');
        if (canvas) {
            canvas.addEventListener('wheel', followZoomHandler, { passive: false });
        }
    }

    function teardownFollowZoomHandler() {
        if (followZoomHandler) {
            const canvas = document.querySelector('#viz-canvas');
            if (canvas) {
                canvas.removeEventListener('wheel', followZoomHandler);
            }
            followZoomHandler = null;
        }
    }

    function handleFollowEscape(e) {
        if (e.key === 'Escape') {
            stopFollowing();
        }
    }

    function stopFollowing() {
        if (window.Viz3D && window.Viz3D.setFollowTarget) {
            window.Viz3D.setFollowTarget(null);
        }
        if (followIndicator) {
            followIndicator.remove();
            followIndicator = null;
        }
        document.removeEventListener('keydown', handleFollowEscape);

        // Clean up zoom handler
        teardownFollowZoomHandler();
    }

    function explainBlob(blob) {
        if (window.ExplainabilityPanel) {
            window.ExplainabilityPanel.showForBlob(blob.id);
        } else {
            showToast('Explainability not available', 'warning');
        }
    }

    function identifyPerson(blob) {
        // Open BLE panel to assign person
        if (window.BLEPanel) {
            window.BLEPanel.open();
            // Highlight this blob in the panel
            setTimeout(() => {
                const blobEl = document.querySelector(`[data-blob-id="${blob.id}"]`);
                if (blobEl) {
                    blobEl.scrollIntoView({ behavior: 'smooth', block: 'center' });
                    blobEl.classList.add('highlight');
                }
            }, 100);
        }
    }

    function markIncorrect(blob) {
        if (window.FeedbackPanel) {
            window.FeedbackPanel.markIncorrect(blob.id);
        } else {
            // Send feedback directly
            fetch('/api/feedback', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    type: 'incorrect',
                    blob_id: blob.id,
                    timestamp: Date.now()
                })
            }).then(() => {
                showToast('Marked as incorrect. System will learn from this.', 'info');
            });
        }
    }

    function showBlobHistory(blob) {
        // Navigate to timeline and filter for this blob/person
        if (window.SpaxelRouter) {
            window.SpaxelRouter.navigate('timeline');
        }

        // Set filter to this person
        setTimeout(() => {
            const filterSelect = document.getElementById('timeline-filter-person');
            if (filterSelect && blob.person) {
                // Add option if not exists
                let option = filterSelect.querySelector(`option[value="${blob.person}"]`);
                if (!option) {
                    option = document.createElement('option');
                    option.value = blob.person;
                    option.textContent = blob.person;
                    filterSelect.appendChild(option);
                }
                filterSelect.value = blob.person;

                // Trigger filter
                filterSelect.dispatchEvent(new Event('change'));
            }
        }, 100);
    }

    function createPersonAutomation(blob) {
        if (window.AutomationBuilder) {
            window.AutomationBuilder.createNewForPerson(blob);
        } else {
            showToast('Automation builder not available', 'warning');
        }
    }

    function setBlobUnknown(blob) {
        // Remove identity assignment from this blob
        if (window.SpaxelState) {
            const blobs = window.SpaxelState.get('blobs');
            if (blobs && blobs[blob.id]) {
                blobs[blob.id].person = null;
                blobs[blob.id].ble_device = null;
                window.SpaxelState.set('blobs', blob.id, blobs[blob.id]);
                showToast(`Set blob #${blob.id} as unknown`, 'info');
            }
        }
    }

    // ============================================
    // Node Actions
    // ============================================

    function showNodeDiagnostics(node) {
        if (window.LinkHealthPanel) {
            window.LinkHealthPanel.showForNode(node.mac);
        } else {
            showToast('Link health panel not available', 'warning');
        }
    }

    async function blinkNodeLED(node) {
        try {
            const response = await fetch(`/api/nodes/${node.mac}/identify`, {
                method: 'POST'
            });

            if (response.ok) {
                showToast(`Blinking ${node.name || node.mac}`, 'info');
            } else {
                showToast('Failed to blink LED', 'warning');
            }
        } catch (error) {
            console.error('[Quick Actions] Error blinking LED:', error);
            showToast('Failed to blink LED', 'warning');
        }
    }

    function repositionNode(node) {
        if (window.Placement) {
            window.Placement.selectNode(node.mac);
            // Switch to live view if not already
            if (window.SpaxelRouter && window.SpaxelRouter.getMode() !== 'live') {
                window.SpaxelRouter.navigate('live');
            }
        }
    }

    async function updateNodeFirmware(node) {
        // Check if this is the last online node
        const isLastOnline = node.isLastOnline || checkIfLastOnline(node);

        let confirmed;
        if (isLastOnline) {
            confirmed = confirm(
                `WARNING: ${node.name || node.mac} is the last online node!\n\n` +
                `Updating firmware will temporarily disconnect this node.\n` +
                `This may result in loss of detection coverage.\n\n` +
                `Continue with OTA update?`
            );
        } else {
            confirmed = confirm(`Update firmware for ${node.name || node.mac}?`);
        }

        if (!confirmed) return;

        try {
            const response = await fetch(`/api/nodes/${node.mac}/update`, {
                method: 'POST'
            });

            if (response.ok) {
                showToast(`Updating ${node.name || node.mac}`, 'info');
            } else {
                showToast('Failed to start update', 'warning');
            }
        } catch (error) {
            console.error('[Quick Actions] Error updating node:', error);
            showToast('Failed to start update', 'warning');
        }
    }

    /**
     * Check if a node is the last online node
     */
    function checkIfLastOnline(node) {
        if (!node || !node.mac) return false;

        if (window.SpaxelState) {
            const nodes = window.SpaxelState.get('nodes');
            if (!nodes) return false;

            let onlineCount = 0;
            for (let mac in nodes) {
                const n = nodes[mac];
                if (n && n.status === 'online') {
                    onlineCount++;
                }
            }

            // Check if this is the only online node
            return onlineCount === 1 && nodes[node.mac] && nodes[node.mac].status === 'online';
        }

        return false;
    }

    function showNodeLinks(node) {
        if (window.Viz3D && window.Viz3D.highlightNodeLinks) {
            // Clear any existing highlights first
            window.Viz3D.clearLinkHighlights();
            // Highlight links for this node
            window.Viz3D.highlightNodeLinks(node.mac, true, 0x4fc3f7);
            showToast('Links highlighted. Click elsewhere to clear.', 'info');

            // Auto-clear after 5 seconds
            setTimeout(function() {
                if (window.Viz3D && window.Viz3D.clearLinkHighlights) {
                    window.Viz3D.clearLinkHighlights();
                }
            }, 5000);
        }
    }

    async function disableNode(node) {
        const confirmed = confirm(`Disable ${node.name || node.mac}?`);
        if (!confirmed) return;

        try {
            const response = await fetch(`/api/nodes/${node.mac}/disable`, {
                method: 'POST'
            });

            if (response.ok) {
                showToast(`${node.name || node.mac} disabled`, 'info');
            } else {
                showToast('Failed to disable node', 'warning');
            }
        } catch (error) {
            console.error('[Quick Actions] Error disabling node:', error);
            showToast('Failed to disable node', 'warning');
        }
    }

    function removeNode(node) {
        const confirmed = confirm(`Remove ${node.name || node.mac} from the fleet?`);
        if (!confirmed) return;

        // This would normally open a confirmation dialog in the UI
        showToast('Node removal requires confirmation in Fleet panel', 'info');
    }

    function editNodeLabel(node) {
        // Open inline edit field for node label
        const newName = prompt(`Enter new name for ${node.name || node.mac}:`, node.name || '');
        if (newName === null) return; // User cancelled
        if (newName.trim() === '') {
            showToast('Name cannot be empty', 'warning');
            return;
        }

        // Update node name via API
        fetch(`/api/nodes/${node.mac}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: newName.trim() })
        })
        .then(response => {
            if (response.ok) {
                showToast(`Node renamed to "${newName.trim()}"`, 'info');
                // Update state if available
                if (window.SpaxelState) {
                    const nodes = window.SpaxelState.get('nodes');
                    if (nodes && nodes[node.mac]) {
                        nodes[node.mac].name = newName.trim();
                        window.SpaxelState.set('nodes', node.mac, nodes[node.mac]);
                    }
                }
            } else {
                showToast('Failed to rename node', 'warning');
            }
        })
        .catch(error => {
            console.error('[Quick Actions] Error renaming node:', error);
            showToast('Failed to rename node', 'warning');
        });
    }

    function reassignNodeRole(node) {
        // Open role picker dialog
        const roles = ['tx', 'rx', 'tx_rx', 'passive', 'idle'];
        const roleLabels = {
            'tx': 'TX (Transmitter only)',
            'rx': 'RX (Receiver only)',
            'tx_rx': 'TX/RX (Both)',
            'passive': 'Passive (RX from router)',
            'idle': 'Idle (Disabled)'
        };

        const currentRole = node.role || 'tx_rx';
        let message = 'Select new role for ' + (node.name || node.mac) + ':\n\n';
        roles.forEach((r, i) => {
            message += `${i + 1}. ${roleLabels[r]}${r === currentRole ? ' (current)' : ''}\n`;
        });
        message += '\nEnter number (1-' + roles.length + '):';

        const choice = prompt(message);
        if (choice === null) return; // User cancelled

        const choiceNum = parseInt(choice, 10);
        if (isNaN(choiceNum) || choiceNum < 1 || choiceNum > roles.length) {
            showToast('Invalid choice', 'warning');
            return;
        }

        const newRole = roles[choiceNum - 1];
        if (newRole === currentRole) {
            showToast('Role unchanged', 'info');
            return;
        }

        // Update node role via API
        fetch(`/api/nodes/${node.mac}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ role: newRole })
        })
        .then(response => {
            if (response.ok) {
                showToast(`Role changed to ${roleLabels[newRole]}`, 'info');
                // Update state if available
                if (window.SpaxelState) {
                    const nodes = window.SpaxelState.get('nodes');
                    if (nodes && nodes[node.mac]) {
                        nodes[node.mac].role = newRole;
                        window.SpaxelState.set('nodes', node.mac, nodes[node.mac]);
                    }
                }
            } else {
                showToast('Failed to change role', 'warning');
            }
        })
        .catch(error => {
            console.error('[Quick Actions] Error changing role:', error);
            showToast('Failed to change role', 'warning');
        });
    }

    // ============================================
    // Zone Actions
    // ============================================

    function showZoneHistory(zone) {
        // Navigate to timeline and filter for this zone
        if (window.SpaxelRouter) {
            window.SpaxelRouter.navigate('timeline');
        }

        setTimeout(() => {
            const filterSelect = document.getElementById('timeline-filter-zone');
            if (filterSelect) {
                // Add option if not exists
                let option = filterSelect.querySelector(`option[value="${zone.name}"]`);
                if (!option) {
                    option = document.createElement('option');
                    option.value = zone.name;
                    option.textContent = zone.name;
                    filterSelect.appendChild(option);
                }
                filterSelect.value = zone.name;

                // Trigger filter
                filterSelect.dispatchEvent(new Event('change'));
            }
        }, 100);
    }

    function editZoneBounds(zone) {
        if (window.Placement) {
            window.Placement.editZone(zone.id);
        }
    }

    function renameZone(zone) {
        // Inline rename zone
        const newName = prompt(`Enter new name for zone "${zone.name}":`, zone.name);
        if (newName === null) return; // User cancelled
        if (newName.trim() === '') {
            showToast('Name cannot be empty', 'warning');
            return;
        }

        // Update zone name via API
        fetch(`/api/zones/${zone.id}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: newName.trim() })
        })
        .then(response => {
            if (response.ok) {
                showToast(`Zone renamed to "${newName.trim()}"`, 'info');
                // Update state if available
                if (window.SpaxelState) {
                    const zones = window.SpaxelState.get('zones');
                    if (zones && zones[zone.id]) {
                        zones[zone.id].name = newName.trim();
                        window.SpaxelState.set('zones', zone.id, zones[zone.id]);
                    }
                }
            } else {
                showToast('Failed to rename zone', 'warning');
            }
        })
        .catch(error => {
            console.error('[Quick Actions] Error renaming zone:', error);
            showToast('Failed to rename zone', 'warning');
        });
    }

    function createZoneAutomation(zone) {
        if (window.AutomationBuilder) {
            window.AutomationBuilder.createNewForZone(zone);
        }
    }

    function showZoneCrowdFlow(zone) {
        // Toggle crowd flow layer for this zone
        if (window.toggleFlowLayer) {
            window.toggleFlowLayer(true);
            // Could filter to just this zone
        }
    }

    function deleteZone(zone) {
        const confirmed = confirm(`Delete zone "${zone.name}"? This will remove the zone and all its associated data.`);
        if (!confirmed) return;

        fetch(`/api/zones/${zone.id}`, {
            method: 'DELETE'
        })
        .then(response => {
            if (response.ok) {
                showToast(`Zone "${zone.name}" deleted`, 'info');
                // Update state if available
                if (window.SpaxelState) {
                    const zones = window.SpaxelState.get('zones');
                    if (zones && zones[zone.id]) {
                        delete zones[zone.id];
                        window.SpaxelState.set('zones', zone.id, null);
                    }
                }
            } else {
                showToast('Failed to delete zone', 'warning');
            }
        })
        .catch(error => {
            console.error('[Quick Actions] Error deleting zone:', error);
            showToast('Failed to delete zone', 'warning');
        });
    }

    // ============================================
    // Empty Space Actions
    // ============================================

    function showLocationHistory(pos) {
        // Navigate to timeline and filter for events near this location
        if (window.SpaxelRouter) {
            window.SpaxelRouter.navigate('timeline');
        }

        // Would need to implement location-based filtering
        showToast('Location history coming soon', 'info');
    }

    function showCoverageQuality(pos) {
        // Show GDOP value at this point
        if (window.Placement) {
            window.Placement.showGDOPAtPoint(pos);
        }
    }

    function addTriggerZone(pos) {
        if (window.AutomationBuilder) {
            window.AutomationBuilder.createNewAtLocation(pos);
        }
    }

    function addVirtualNode(pos) {
        if (window.Placement) {
            window.Placement.addVirtualNodeAt(pos);
        }
    }

    function createZoneHere(pos) {
        // Start zone creation mode with the clicked position as one corner
        if (window.ZoneEditor) {
            window.ZoneEditor.startCreationAt(pos.x, pos.z);
            showToast('Zone creation mode started. Drag to define zone.', 'info');
        } else {
            showToast('Zone editor not available', 'warning');
        }
    }

    function setHomeAsPoint(pos) {
        // Set the coordinate origin to this position
        const confirmed = confirm(`Set home point to (${pos.x.toFixed(2)}, ${pos.z.toFixed(2)})? This will recenter the floor plan.`);
        if (!confirmed) return;

        // This would update the coordinate system origin
        showToast('Set home point - coming soon', 'info');
    }

    function placePortalHere(pos) {
        // Start portal creation mode centered at this position
        if (window.PortalEditor) {
            window.PortalEditor.startCreationAt(pos.x, pos.z);
            showToast('Portal creation mode started. Drag to define portal.', 'info');
        } else {
            showToast('Portal editor not available', 'warning');
        }
    }

    // ============================================
    // Portal Actions
    // ============================================

    function showPortalCrossings(portal) {
        // Show crossing log for this portal
        fetch(`/api/portals/${portal.id}/crossings?limit=20`)
            .then(response => response.json())
            .then(crossings => {
                if (crossings.length > 0) {
                    const message = crossings.slice(0, 5).map(c =>
                        `${c.person || 'Unknown'} ${c.direction === 'a_to_b' ? '→' : '←'} ${c.timestamp_ms ? formatTimestamp(c.timestamp_ms) : ''}`
                    ).join('\n');
                    alert(`Recent crossings:\n\n${message}`);
                } else {
                    showToast('No crossings recorded yet', 'info');
                }
            });
    }

    function editPortal(portal) {
        if (window.Placement) {
            window.Placement.editPortal(portal.id);
        }
    }

    function reversePortalDirection(portal) {
        // Swap zone labels
        showToast('Reverse portal direction - coming soon', 'info');
    }

    function deletePortal(portal) {
        const confirmed = confirm(`Delete portal "${portal.name}"? This will remove the portal and all its crossing history.`);
        if (!confirmed) return;

        fetch(`/api/portals/${portal.id}`, {
            method: 'DELETE'
        })
        .then(response => {
            if (response.ok) {
                showToast(`Portal "${portal.name}" deleted`, 'info');
                // Update state if available
                if (window.SpaxelState) {
                    const portals = window.SpaxelState.get('portals');
                    if (portals && portals[portal.id]) {
                        delete portals[portal.id];
                        window.SpaxelState.set('portals', portal.id, null);
                    }
                }
            } else {
                showToast('Failed to delete portal', 'warning');
            }
        })
        .catch(error => {
            console.error('[Quick Actions] Error deleting portal:', error);
            showToast('Failed to delete portal', 'warning');
        });
    }

    // ============================================
    // Trigger Actions
    // ============================================

    function editTrigger(trigger) {
        if (window.AutomationBuilder) {
            window.AutomationBuilder.editTrigger(trigger.id);
        }
    }

    function testTrigger(trigger) {
        fetch(`/api/triggers/${trigger.id}/test`, { method: 'POST' })
            .then(response => {
                if (response.ok) {
                    showToast(`Tested "${trigger.name}"`, 'success');
                } else {
                    showToast('Test failed', 'warning');
                }
            });
    }

    function showTriggerLog(trigger) {
        showBlobHistory(trigger); // Reuse blob history logic
    }

    async function toggleTrigger(trigger) {
        // Toggle the automation's enabled flag
        const newState = !trigger.enabled;
        const endpoint = newState ? 'enable' : 'disable';

        fetch(`/api/triggers/${trigger.id}/${endpoint}`, { method: 'POST' })
            .then(response => {
                if (response.ok) {
                    showToast(`Trigger "${trigger.name}" ${newState ? 'enabled' : 'disabled'}`, 'info');
                    // Update state if available
                    if (window.SpaxelState) {
                        const triggers = window.SpaxelState.get('triggers');
                        if (triggers && triggers[trigger.id]) {
                            triggers[trigger.id].enabled = newState;
                            window.SpaxelState.set('triggers', trigger.id, triggers[trigger.id]);
                        }
                    }
                } else {
                    showToast(`Failed to ${newState ? 'enable' : 'disable'} trigger`, 'warning');
                }
            })
            .catch(error => {
                console.error('[Quick Actions] Error toggling trigger:', error);
                showToast(`Failed to ${newState ? 'enable' : 'disable'} trigger`, 'warning');
            });
    }

    function deleteTrigger(trigger) {
        const confirmed = confirm(`Delete trigger volume "${trigger.name}"? This will delete the volume and its associated automation trigger.`);
        if (!confirmed) return;

        fetch(`/api/triggers/${trigger.id}`, {
            method: 'DELETE'
        })
        .then(response => {
            if (response.ok) {
                showToast(`Trigger "${trigger.name}" deleted`, 'info');
                // Update state if available
                if (window.SpaxelState) {
                    const triggers = window.SpaxelState.get('triggers');
                    if (triggers && triggers[trigger.id]) {
                        delete triggers[trigger.id];
                        window.SpaxelState.set('triggers', trigger.id, null);
                    }
                }
            } else {
                showToast('Failed to delete trigger', 'warning');
            }
        })
        .catch(error => {
            console.error('[Quick Actions] Error deleting trigger:', error);
            showToast('Failed to delete trigger', 'warning');
        });
    }

    // ============================================
    // 3D Scene Integration
    // ============================================

    /**
     * Set up raycasting for 3D scene
     */
    function setup3DIntegration() {
        // Wait for 3D view to be ready
        const check3D = setInterval(() => {
            if (window.Viz3D && window.Viz3D.scene) {
                clearInterval(check3D);
                initializeRaycaster();
            }
        }, 100);
    }

    /**
     * Initialize raycaster for right-click detection
     */
    function initializeRaycaster() {
        if (!window.Viz3D) {
            console.error('[Quick Actions] Viz3D not available');
            return;
        }

        const raycaster = new THREE.Raycaster();
        const mouse = new THREE.Vector2();

        // Right-click handler on the canvas
        document.addEventListener('contextmenu', function(event) {
            // Only handle right-clicks on the canvas
            const canvas = document.querySelector('#viz-canvas');
            if (!canvas || !canvas.contains(event.target)) {
                return;
            }

            event.preventDefault();
            event.stopPropagation();

            // If menu is already open, close it and return
            if (contextMenu && contextMenu.classList.contains('visible')) {
                closeContextMenu();
                return;
            }

            // Get mouse position
            const rect = canvas.getBoundingClientRect();
            mouse.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
            mouse.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;

            // Get camera and scene from Viz3D
            const camera = window.Viz3D.camera ? window.Viz3D.camera() : null;
            const scene = window.Viz3D.scene ? window.Viz3D.scene() : null;

            if (!camera || !scene) {
                console.warn('[Quick Actions] Camera or scene not available');
                return;
            }

            // Raycast
            raycaster.setFromCamera(mouse, camera);

            // Priority order: 1. Track blobs, 2. Node spheres, 3. Zone cuboids, 4. Portal planes, 5. Trigger volumes, 6. Ground plane

            // Check for blob intersections (highest priority - users click on people most often)
            const blobMeshes = window.Viz3D.blobMeshes ? window.Viz3D.blobMeshes() : [];
            if (blobMeshes && blobMeshes.length > 0) {
                const blobIntersects = raycaster.intersectObjects(blobMeshes, true);
                if (blobIntersects.length > 0) {
                    // Find the object with blobId in userData
                    for (let i = 0; i < blobIntersects.length; i++) {
                        let obj = blobIntersects[i].object;
                        // Walk up parent chain to find group with blobId
                        while (obj) {
                            if (obj.userData && obj.userData.blobId) {
                                const blob = findBlobById(obj.userData.blobId);
                                if (blob) {
                                    showContextMenu(event.clientX, event.clientY, 'blob', blob);
                                    return;
                                }
                            }
                            obj = obj.parent;
                        }
                    }
                }
            }

            // Check for node intersections
            const nodeMeshes = window.Viz3D.nodeMeshes ? window.Viz3D.nodeMeshes() : [];
            if (nodeMeshes && nodeMeshes.length > 0) {
                const nodeIntersects = raycaster.intersectObjects(nodeMeshes, true);
                if (nodeIntersects.length > 0) {
                    // Find the object with mac in userData
                    for (let i = 0; i < nodeIntersects.length; i++) {
                        let obj = nodeIntersects[i].object;
                        while (obj) {
                            if (obj.userData && obj.userData.mac) {
                                const node = findNodeByMac(obj.userData.mac);
                                if (node) {
                                    showContextMenu(event.clientX, event.clientY, 'node', node);
                                    return;
                                }
                            }
                            obj = obj.parent;
                        }
                    }
                }
            }

            // Check for zone intersections (by position)
            const zones = getZonesFromState();
            for (let zoneId in zones) {
                const zone = zones[zoneId];
                const plane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
                const planeIntersect = new THREE.Vector3();
                raycaster.ray.intersectPlane(plane, planeIntersect);

                if (planeIntersect && isPointInZone(planeIntersect, zone)) {
                    showContextMenu(event.clientX, event.clientY, 'zone', zone);
                    return;
                }
            }

            // Check for portal intersections
            const portalMeshes = window.Viz3D.portalMeshes ? window.Viz3D.portalMeshes() : [];
            if (portalMeshes && portalMeshes.length > 0) {
                const portalIntersects = raycaster.intersectObjects(portalMeshes, true);
                if (portalIntersects.length > 0) {
                    for (let i = 0; i < portalIntersects.length; i++) {
                        let obj = portalIntersects[i].object;
                        while (obj) {
                            if (obj.userData && obj.userData.portalId) {
                                const portal = findPortalById(obj.userData.portalId);
                                if (portal) {
                                    showContextMenu(event.clientX, event.clientY, 'portal', portal);
                                    return;
                                }
                            }
                            obj = obj.parent;
                        }
                    }
                }
            }

            // Check for trigger volume intersections
            const triggerMeshes = window.Viz3D.triggerMeshes ? window.Viz3D.triggerMeshes() : [];
            if (triggerMeshes && triggerMeshes.length > 0) {
                const triggerIntersects = raycaster.intersectObjects(triggerMeshes, true);
                if (triggerIntersects.length > 0) {
                    for (let i = 0; i < triggerIntersects.length; i++) {
                        let obj = triggerIntersects[i].object;
                        while (obj) {
                            if (obj.userData && obj.userData.triggerId) {
                                const trigger = findTriggerById(obj.userData.triggerId);
                                if (trigger) {
                                    showContextMenu(event.clientX, event.clientY, 'trigger', trigger);
                                    return;
                                }
                            }
                            obj = obj.parent;
                        }
                    }
                }
            }

            // Calculate 3D point on ground plane for empty space menu (always intersects last)
            const plane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
            const planeIntersect = new THREE.Vector3();
            raycaster.ray.intersectPlane(plane, planeIntersect);

            // Show empty space menu with 3D position
            showContextMenu(event.clientX, event.clientY, 'empty', {
                x: planeIntersect.x || 0,
                y: 0,
                z: planeIntersect.z || 0,
                point: planeIntersect
            });
        });

        console.log('[Quick Actions] 3D integration ready');
    }

    /**
     * Find blob by ID
     */
    function findBlobById(id) {
        if (window.SpaxelState) {
            const blobs = window.SpaxelState.get('blobs');
            if (!blobs) return null;
            // Convert map to array and find
            for (let blobId in blobs) {
                if (blobs[blobId].id === id) return blobs[blobId];
            }
            return null;
        }
        return null;
    }

    /**
     * Find node by MAC
     */
    function findNodeByMac(mac) {
        if (window.SpaxelState) {
            const nodes = window.SpaxelState.get('nodes');
            if (!nodes) return null;
            return nodes[mac] || null;
        }
        return null;
    }

    /**
     * Find zone by ID
     */
    function findZoneById(id) {
        if (window.SpaxelState) {
            const zones = window.SpaxelState.get('zones');
            if (!zones) return null;
            return zones[id] || null;
        }
        return null;
    }

    /**
     * Find portal by ID
     */
    function findPortalById(id) {
        if (window.SpaxelState) {
            const portals = window.SpaxelState.get('portals');
            if (!portals) return null;
            return portals[id] || null;
        }
        return null;
    }

    /**
     * Find trigger by ID
     */
    function findTriggerById(id) {
        if (window.SpaxelState) {
            const triggers = window.SpaxelState.get('triggers');
            if (!triggers) return null;
            return triggers[id] || null;
        }
        return null;
    }

    // ============================================
    // Touch/Long-Press Support
    // ============================================

    /**
     * Set up touch event handlers for long-press
     */
    function setupTouchSupport() {
        document.addEventListener('touchstart', handleTouchStart, { passive: false });
        document.addEventListener('touchmove', handleTouchMove, { passive: false });
        document.addEventListener('touchend', handleTouchEnd);
        document.addEventListener('touchcancel', handleTouchEnd);
    }

    function handleTouchStart(e) {
        const touch = e.touches[0];
        if (!touch) return;

        touchStartPos = {
            x: touch.clientX,
            y: touch.clientY
        };

        // Store the touch target for later use
        touchStartPos.target = e.target;

        // Start long press timer
        longPressTimer = setTimeout(() => {
            // Long press detected - perform raycast to determine target
            const targetInfo = getTouchTarget(touch.clientX, touch.clientY);
            showContextMenu(touch.clientX, touch.clientY, targetInfo.type, targetInfo.data);
        }, LONG_PRESS_DURATION);
    }

    function handleTouchMove(e) {
        if (!touchStartPos) return;

        const touch = e.touches[0];
        const distance = Math.sqrt(
            Math.pow(touch.clientX - touchStartPos.x, 2) +
            Math.pow(touch.clientY - touchStartPos.y, 2)
        );

        if (distance > MAX_DISTANCE) {
            // Moved too far - cancel long press
            if (longPressTimer) {
                clearTimeout(longPressTimer);
                longPressTimer = null;
            }
        }
    }

    function handleTouchEnd() {
        if (longPressTimer) {
            clearTimeout(longPressTimer);
            longPressTimer = null;
        }
        touchStartPos = null;
    }

    /**
     * Get target type and data from touch position using raycasting
     */
    function getTouchTarget(clientX, clientY) {
        // Default to empty space
        const result = { type: 'empty', data: { x: 0, y: 0, z: 0 } };

        if (!window.Viz3D) {
            return result;
        }

        const canvas = document.querySelector('#viz-canvas');
        if (!canvas) return result;

        const camera = window.Viz3D.camera ? window.Viz3D.camera() : null;
        const scene = window.Viz3D.scene ? window.Viz3D.scene() : null;
        if (!camera || !scene) return result;

        const rect = canvas.getBoundingClientRect();
        const mouse = new THREE.Vector2();
        mouse.x = ((clientX - rect.left) / rect.width) * 2 - 1;
        mouse.y = -((clientY - rect.top) / rect.height) * 2 + 1;

        const raycaster = new THREE.Raycaster();
        raycaster.setFromCamera(mouse, camera);

        // Check for blob intersections
        const blobMeshes = window.Viz3D.blobMeshes ? window.Viz3D.blobMeshes() : [];
        const blobIntersects = raycaster.intersectObjects(blobMeshes, true);
        if (blobIntersects.length > 0) {
            for (let i = 0; i < blobIntersects.length; i++) {
                let obj = blobIntersects[i].object;
                while (obj) {
                    if (obj.userData && obj.userData.blobId) {
                        const blob = findBlobById(obj.userData.blobId);
                        if (blob) {
                            return { type: 'blob', data: blob };
                        }
                    }
                    obj = obj.parent;
                }
            }
        }

        // Check for node intersections
        const nodeMeshes = window.Viz3D.nodeMeshes ? window.Viz3D.nodeMeshes() : [];
        const nodeIntersects = raycaster.intersectObjects(nodeMeshes, true);
        if (nodeIntersects.length > 0) {
            for (let i = 0; i < nodeIntersects.length; i++) {
                let obj = nodeIntersects[i].object;
                while (obj) {
                    if (obj.userData && obj.userData.mac) {
                        const node = findNodeByMac(obj.userData.mac);
                        if (node) {
                            return { type: 'node', data: node };
                        }
                    }
                    obj = obj.parent;
                }
            }
        }

        // Check for zone intersections (by position)
        const zones = getZonesFromState();
        for (let zoneId in zones) {
            const zone = zones[zoneId];
            const plane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
            const planeIntersect = new THREE.Vector3();
            raycaster.ray.intersectPlane(plane, planeIntersect);

            if (planeIntersect && isPointInZone(planeIntersect, zone)) {
                return { type: 'zone', data: zone };
            }
        }

        // Calculate 3D point on ground plane for empty space
        const plane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
        const planeIntersect = new THREE.Vector3();
        raycaster.ray.intersectPlane(plane, planeIntersect);
        result.data = {
            x: planeIntersect.x || 0,
            y: 0,
            z: planeIntersect.z || 0,
            point: planeIntersect
        };

        return result;
    }

    // ============================================
    // Data Fetching
    // ============================================

    /**
     * Fetch current state for actions
     */
    async function fetchCurrentState() {
        try {
            // Fetch zones if not already in state
            if (window.SpaxelState && !window.SpaxelState.get('zones')) {
                const zonesResponse = await fetch('/api/zones');
                if (zonesResponse.ok) {
                    const zones = await zonesResponse.json();
                    zones.forEach(z => {
                        window.SpaxelState.set('zones', z.id, z);
                    });
                }
            }

            // Fetch nodes if not already in state
            if (window.SpaxelState && !window.SpaxelState.get('nodes')) {
                const nodesResponse = await fetch('/api/nodes');
                if (nodesResponse.ok) {
                    const nodes = await nodesResponse.json();
                    nodes.forEach(n => {
                        window.SpaxelState.set('nodes', n.mac, n);
                    });
                }
            }

            // Fetch blobs if not already in state
            if (window.SpaxelState && !window.SpaxelState.get('blobs')) {
                const blobsResponse = await fetch('/api/blobs');
                if (blobsResponse.ok) {
                    const blobs = await blobsResponse.json();
                    blobs.forEach(b => {
                        window.SpaxelState.set('blobs', b.id, b);
                    });
                }
            }

        } catch (error) {
            console.error('[Quick Actions] Error fetching state:', error);
        }
    }

    // ============================================
    // Helper Functions
    // ============================================

    function getZonesFromState() {
        if (window.SpaxelState) {
            return window.SpaxelState.get('zones') || {};
        }
        return {};
    }

    function isPointInZone(point, zone) {
        // Check if point is within zone bounds
        return point.x >= zone.x &&
               point.x <= zone.x + zone.w &&
               point.z >= zone.z &&
               point.z <= zone.z + zone.d;
    }

    function formatTimestamp(ms) {
        const date = new Date(ms);
        const now = new Date();
        const diff = now - date;

        if (diff < 60000) {
            return 'Just now';
        } else if (diff < 3600000) {
            return `${Math.floor(diff / 60000)}m ago`;
        } else {
            return date.toLocaleDateString();
        }
    }

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

    function init() {
        console.log('[Quick Actions] Initializing...');

        // Create context menu
        createContextMenu();

        // Set up 3D integration
        setup3DIntegration();

        // Set up touch support
        setupTouchSupport();

        // Subscribe to state changes to keep data fresh
        if (window.SpaxelState) {
            window.SpaxelState.subscribe('*', function(newValue, oldValue, key) {
                // State changed - our lookups will use fresh data
                console.log('[Quick Actions] State changed:', key);
            });
        }

        console.log('[Quick Actions] Initialized');
    }

    /**
     * Check if a blob was deleted and auto-exit follow mode
     * This should be called when blobs are updated via WebSocket
     */
    function checkBlobDeleted(blobId) {
        if (followIndicator && window.Viz3D) {
            const currentFollowId = window.Viz3D.followId ? window.Viz3D.followId() : null;
            if (currentFollowId === blobId) {
                // The blob we're following was deleted - exit follow mode
                console.log('[Quick Actions] Blob being followed was deleted, exiting follow mode');
                stopFollowing();
            }
        }
    }

    /**
     * Check if blob status changed to DELETED
     * @param {number} blobId - Blob ID to check
     * @param {Object} blobData - Updated blob data
     */
    function checkBlobStatus(blobId, blobData) {
        if (blobData && blobData.status === 'DELETED') {
            checkBlobDeleted(blobId);
        }
    }

    // ============================================
    // Public API
    // ============================================
    window.SpatialQuickActions = {
        init: init,
        show: showContextMenu,
        close: closeContextMenu,
        stopFollowing: stopFollowing,
        checkBlobDeleted: checkBlobDeleted,
        checkBlobStatus: checkBlobStatus,
        registerAction: (type, action) => {
            if (!ACTIONS[type]) {
                ACTIONS[type] = [];
            }
            ACTIONS[type].push(action);
        }
    };

    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[Quick Actions] Module loaded');
})();
