/**
 * Spaxel Detection Explainability Module
 *
 * Provides the "Why is this here?" feature that explains blob detections by:
 * - Showing per-link contribution breakdown
 * - Highlighting contributing links in the 3D view
 * - Rendering Fresnel zone ellipsoids
 * - Displaying BLE match information
 *
 * @module Explainability
 */

const Explainability = (function () {
    'use strict';

    // ── State ───────────────────────────────────────────────────────────────
    let _isActive = false;
    let _currentBlobID = null;
    let _explanationData = null;
    let _originalMaterialStates = new Map();  // obj.uuid -> {opacity, emissive, transparent}
    let _highlightedLinks = [];
    let _fresnelMeshes = [];
    let _sidebarPanel = null;
    let _sceneOverlay = null;
    let _viz3dCallbacks = [];  // Store Viz3D cleanup callbacks

    // ── Configuration ─────────────────────────────────────────────────────────
    const CONFIG = {
        dimOpacity: 0.2,              // Opacity for dimmed elements
        highlightColor: 0xFFD700,     // Gold for contributing links
        highlightEmissive: 0x555500,
        fresnelOpacity: 0.25,         // Opacity for Fresnel zones
        fresnelColor: 0x4FC3F7,       // Blue for Fresnel zones
        animationDuration: 300,       // ms for scene transitions
    };

    // ── UI Components ─────────────────────────────────────────────────────────
    function createSidebarPanel() {
        var panel = document.createElement('div');
        panel.id = 'explainability-sidebar';
        panel.className = 'panel-sidebar panel-sidebar-right';
        panel.style.display = 'none';
        panel.innerHTML =
            '<div class="panel-header">' +
            '  <h2 class="panel-title">Why is this here?</h2>' +
            '  <button class="panel-close" onclick="Explainability.close()">&times;</button>' +
            '</div>' +
            '<div class="panel-content" id="explainability-content">' +
            '  <div class="explainability-loading">' +
            '    <div class="panel-loading-spinner"></div>' +
            '    <span>Loading explanation...</span>' +
            '  </div>' +
            '</div>';

        document.body.appendChild(panel);

        // Create overlay backdrop
        var backdrop = document.createElement('div');
        backdrop.id = 'explainability-backdrop';
        backdrop.className = 'panel-overlay';
        backdrop.style.display = 'none';
        backdrop.addEventListener('click', function() {
            Explainability.close();
        });
        document.body.appendChild(backdrop);

        _sidebarPanel = panel;
        _sceneOverlay = backdrop;

        return panel;
    }

    function renderContent(data) {
        var content = document.getElementById('explainability-content');
        if (!content) return;

        if (!data) {
            content.innerHTML =
                '<div class="panel-empty">' +
                '  <div class="panel-empty-icon">&#128193;</div>' +
                '  <div class="panel-empty-text">No explanation data available</div>' +
                '</div>';
            return;
        }

        // Calculate confidence percentage
        var confidencePercent = Math.round(data.confidence * 100);

        var html =
            '<div class="explainability-confidence">' +
            '  <div class="confidence-gauge">' +
            '    <svg viewBox="0 0 36 36" class="confidence-ring">' +
            '      <circle class="confidence-ring-bg" cx="18" cy="18" r="15"/>' +
            '      <circle class="confidence-ring-fill" cx="18" cy="18" r="15" ' +
            '              style="stroke-dasharray: ' + confidencePercent + ' 100"/>' +
            '    </svg>' +
            '    <div class="confidence-value">' + confidencePercent + '%</div>' +
            '  </div>' +
            '  <span class="confidence-label">Detection confidence</span>' +
            '</div>';

        // Contributing links table
        if (data.contributing_links && data.contributing_links.length > 0) {
            html += '<div class="explainability-section">' +
                '  <h3 class="section-title">Contributing Links</h3>' +
                '  <table class="links-table">' +
                '    <thead>' +
                '      <tr>' +
                '        <th>Link</th>' +
                '        <th>deltaRMS</th>' +
                '        <th>Zone</th>' +
                '        <th>Weight</th>' +
                '        <th>Contributing</th>' +
                '      </tr>' +
                '    </thead>' +
                '    <tbody>';

            data.contributing_links.forEach(function (link) {
                var zoneColor = _getZoneColor(link.zone_number);
                html +=
                    '<tr>' +
                    '  <td class="link-cell"><code class="link-id">' + _shortenMAC(link.node_mac) + ':' + _shortenMAC(link.peer_mac) + '</code></td>' +
                    '  <td class="deltarms-cell">' + link.delta_rms.toFixed(3) + '</td>' +
                    '  <td class="zone-cell"><span class="zone-badge" style="background:' + zoneColor + '">' + link.zone_number + '</span></td>' +
                    '  <td class="weight-cell">' + link.weight.toFixed(2) + '</td>' +
                    '  <td class="contributing-cell"><span class="contributing-badge">✓</span></td>' +
                    '</tr>';
            });

            html += '    </tbody>' +
                '  </table>' +
                '</div>';
        }

        // All links (including non-contributing)
        if (data.all_links && data.all_links.length > 0) {
            html += '<div class="explainability-section collapsed" id="all-links-section">' +
                '  <div class="section-header toggle-header" onclick="Explainability.toggleSection(this)">' +
                '    All Links (' + data.all_links.length + ')' +
                '    <span class="toggle-icon">▼</span>' +
                '  </div>' +
                '  <div class="section-content" style="display:none;">' +
                '    <table class="links-table links-table-detailed">' +
                '      <thead>' +
                '        <tr>' +
                '          <th>Link</th>' +
                '          <th>deltaRMS</th>' +
                '          <th>Zone</th>' +
                '          <th>Weight</th>' +
                '          <th>Contribution</th>' +
                '          <th>Contributing?</th>' +
                '        </tr>' +
                '      </thead>' +
                '      <tbody>';

            data.all_links.forEach(function (link) {
                var isContributing = link.contributing;
                var contribClass = isContributing ? 'contributing-yes' : 'contributing-no';
                var contribText = isContributing ? '✓' : '—';

                html +=
                    '<tr class="' + contribClass + '">' +
                    '  <td class="link-cell"><code class="link-id">' + _shortenMAC(link.node_mac) + ':' + _shortenMAC(link.peer_mac) + '</code></td>' +
                    '  <td class="deltarms-cell">' + link.delta_rms.toFixed(3) + '</td>' +
                    '  <td class="zone-cell">' + link.zone_number + '</td>' +
                    '  <td class="weight-cell">' + link.weight.toFixed(2) + '</td>' +
                    '  <td class="contribution-cell">' + link.contribution.toFixed(4) + '</td>' +
                    '  <td class="contributing-cell"><span class="contributing-badge ' + contribClass + '">' + contribText + '</span></td>' +
                    '</tr>';
            });

            html += '      </tbody>' +
                '    </table>' +
                '  </div>' +
                '</div>';
        }

        // BLE match section
        if (data.ble_match) {
            var ble = data.ble_match;
            html += '<div class="explainability-section">' +
                '  <h3 class="section-title">BLE Identity Match</h3>' +
                '  <div class="ble-match-card">' +
                '    <div class="ble-match-header">' +
                '      <span class="ble-match-indicator" style="background:' + ble.person_color + '"></span>' +
                '      <span class="ble-match-name">' + ble.person_label + '</span>' +
                '      <span class="ble-match-confidence">' + Math.round(ble.confidence * 100) + '% confident</span>' +
                '    </div>' +
                '    <div class="ble-match-details">' +
                '      <div class="ble-match-detail">' +
                '        <span class="detail-label">Device:</span>' +
                '        <code class="detail-value">' + ble.device_addr + '</code>' +
                '      </div>';

            if (ble.reported_by_nodes && ble.reported_by_nodes.length > 0) {
                html += '<div class="ble-match-detail">' +
                    '  <span class="detail-label">Seen by:</span>' +
                    '  <span class="detail-value">' + ble.reported_by_nodes.join(', ') + '</span>' +
                    '</div>';
            }

            html += '    </div>' +
                '  </div>' +
                '</div>';
        }

        // Close button at bottom
        html += '<div class="explainability-footer">' +
            '  <button class="panel-btn panel-btn-primary" onclick="Explainability.close()">Close</button>' +
            '</div>';

        content.innerHTML = html;
    }

    function _shortenMAC(mac) {
        // Shorten MAC to last 6 characters
        if (!mac) return '----';
        if (mac.length <= 8) return mac;
        return '...' + mac.slice(-8);
    }

    function _getZoneColor(zoneNumber) {
        // Color gradient for Fresnel zones
        var colors = [
            '#22c55e', // zone 1 - green
            '#84cc16', // zone 2 - lime
            '#cddc39', // zone 3 - yellow
            '#ffeb3b', // zone 4 - orange
            '#ff9800', // zone 5 - deep orange
        ];
        return colors[Math.min(zoneNumber - 1, colors.length - 1)] || '#999';
    }

    function toggleSection(header) {
        var content = header.nextElementSibling;
        var icon = header.querySelector('.toggle-icon');
        var parentSection = header.parentElement;

        if (content.style.display === 'none') {
            content.style.display = 'block';
            if (icon) icon.textContent = '▲';
            parentSection.classList.remove('collapsed');
        } else {
            content.style.display = 'none';
            if (icon) icon.textContent = '▼';
            parentSection.classList.add('collapsed');
        }
    }

    // ── 3D Scene Manipulation ─────────────────────────────────────────────────

    /**
     * Apply the X-ray overlay effect to the 3D scene.
     * Dims non-contributing elements and highlights contributing links.
     */
    function applyXRayOverlay(explanationData) {
        if (!window.Viz3D) return;

        // Save original material states and apply dimming
        _saveAndDimScene();

        // Highlight contributing links
        _highlightContributingLinks(explanationData);

        // Render Fresnel zone ellipsoids
        _renderFresnelZones(explanationData);
    }

    /**
     * Save original material states and dim the scene.
     */
    function _saveAndDimScene() {
        _originalMaterialStates.clear();

        // Dim room elements using Viz3D callback
        if (window.Viz3D.forEachRoomObject) {
            Viz3D.forEachRoomObject(function(obj) {
                _dimObject(obj);
            });
        }

        // Dim all links first (will highlight contributing ones later)
        if (window.Viz3D.forEachLink) {
            Viz3D.forEachLink(function(line, linkID) {
                _dimObject(line);
            });
        }

        // Dim non-target blobs
        if (window.Viz3D.forEachBlob) {
            Viz3D.forEachBlob(function(obj, blobID) {
                if (blobID !== _currentBlobID && obj.group) {
                    _dimObject(obj.group);
                }
            });
        }
    }

    function _dimObject(obj) {
        if (!obj || !obj.material) return;

        var uuid = obj.uuid;
        if (!uuid) return;

        // Save original state
        if (!_originalMaterialStates.has(uuid)) {
            _originalMaterialStates.set(uuid, {
                opacity: obj.material.opacity,
                transparent: obj.material.transparent,
                emissiveIntensity: obj.material.emissiveIntensity || 0
            });
            if (obj.material.emissive) {
                _originalMaterialStates.get(uuid).emissiveColor = obj.material.emissive.getHex();
            }
            if (obj.material.color) {
                _originalMaterialStates.get(uuid).color = obj.material.color.getHex();
            }
        }

        // Apply dimming
        obj.material.opacity = CONFIG.dimOpacity;
        obj.material.transparent = true;
        if (obj.material.emissive) {
            obj.material.emissive.setHex(0x000000);
            obj.material.emissiveIntensity = 0;
        }
        obj.material.needsUpdate = true;
    }

    /**
     * Highlight contributing links with a glowing effect.
     */
    function _highlightContributingLinks(explanationData) {
        if (!explanationData.contributing_links || !window.Viz3D) return;

        explanationData.contributing_links.forEach(function (link) {
            if (window.Viz3D.highlightLink) {
                Viz3D.highlightLink(link.link_id, CONFIG.highlightColor, CONFIG.highlightEmissive, 0.8);
                _highlightedLinks.push(link.link_id);
            }
        });
    }

    /**
     * Render Fresnel zone ellipsoids for contributing links.
     */
    function _renderFresnelZones(explanationData) {
        if (!explanationData.fresnel_zones) return;

        explanationData.fresnel_zones.forEach(function (zone) {
            if (window.Viz3D.addFresnelZone) {
                var mesh = Viz3D.addFresnelZone(
                    zone.center_pos[0], zone.center_pos[1], zone.center_pos[2],
                    zone.semi_axes[0], zone.semi_axes[1], zone.semi_axes[2],
                    CONFIG.fresnelColor,
                    CONFIG.fresnelOpacity
                );
                if (mesh) {
                    _fresnelMeshes.push(mesh);
                }
            }
        });
    }

    /**
     * Restore the scene to its original state.
     */
    function restoreScene() {
        // Restore material states
        if (window.Viz3D) {
            _originalMaterialStates.forEach(function (state, uuid) {
                if (window.Viz3D.restoreObjectMaterial) {
                    Viz3D.restoreObjectMaterial(uuid, state);
                }
            });
        }

        // Remove Fresnel zone meshes
        _fresnelMeshes.forEach(function (mesh) {
            if (window.Viz3D) {
                Viz3D.removeFresnelZone(mesh);
            }
        });
        _fresnelMeshes = [];
        _highlightedLinks = [];
        _originalMaterialStates.clear();
    }

    // ── API Integration ─────────────────────────────────────────────────────────

    function fetchExplanation(blobID) {
        return fetch('/api/explain/' + encodeURIComponent(blobID))
            .then(function (response) {
                if (!response.ok) {
                    throw new Error('Failed to fetch explanation: ' + response.statusText);
                }
                return response.json();
            })
            .then(function (data) {
                _explanationData = data;
                renderContent(data);
                applyXRayOverlay(data);
            })
            .catch(function (error) {
                console.error('[Explainability] Failed to load explanation:', error);
                renderContent(null);
            });
    }

    // ── Public API ─────────────────────────────────────────────────────────────

    return {
        /**
         * Open the explainability view for a blob.
         * @param {number} blobID - The blob ID to explain
         */
        explain: function (blobID) {
            if (_isActive) {
                // Already open, just switch to new blob
                close();
            }

            _isActive = true;
            _currentBlobID = blobID;

            // Create UI if needed
            if (!_sidebarPanel) {
                createSidebarPanel();
            }

            // Show sidebar
            _sidebarPanel.style.display = 'block';
            _sidebarPanel.classList.add('panel-sidebar-visible');
            _sceneOverlay.style.display = 'block';
            _sceneOverlay.classList.add('panel-overlay-visible');

            // Fetch and display data
            fetchExplanation(blobID);
        },

        /**
         * Close the explainability view.
         */
        close: function () {
            if (!_isActive) return;

            _isActive = false;
            _currentBlobID = null;
            _explanationData = null;

            // Hide sidebar
            if (_sidebarPanel) {
                _sidebarPanel.classList.remove('panel-sidebar-visible');
                _sidebarPanel.style.display = 'none';
            }
            if (_sceneOverlay) {
                _sceneOverlay.classList.remove('panel-overlay-visible');
                _sceneOverlay.style.display = 'none';
            }

            // Restore scene
            restoreScene();
        },

        /**
         * Check if explainability view is currently active.
         */
        isActive: function () {
            return _isActive;
        },

        /**
         * Toggle the all links section.
         */
        toggleSection: toggleSection,

        /**
         * Get current explanation data.
         */
        getData: function () {
            return _explanationData;
        },

        /**
         * Get current blob ID.
         */
        getCurrentBlobID: function () {
            return _currentBlobID;
        }
    };
})();

// Make toggleSection available globally for onclick
window.Explainability = Explainability;
