/**
 * Feedback UI Components for Detection Accuracy
 * Provides thumbs-up/down buttons, feedback forms, and missed detection reporting
 */
(function() {
    'use strict';

    var Feedback = {
        // State
        pendingFeedback: null,
        feedbackPanelVisible: false,

        // Event types
        EventTypes: {
            BLOB_DETECTION: 'blob_detection',
            ZONE_TRANSITION: 'zone_transition',
            FALL_ALERT: 'fall_alert',
            ANOMALY: 'anomaly'
        },

        // Feedback types
        FeedbackTypes: {
            TRUE_POSITIVE: 'TRUE_POSITIVE',
            FALSE_POSITIVE: 'FALSE_POSITIVE',
            FALSE_NEGATIVE: 'FALSE_NEGATIVE',
            WRONG_IDENTITY: 'WRONG_IDENTITY',
            WRONG_ZONE: 'WRONG_ZONE'
        },

        /**
         * Initialize the feedback module
         */
        init: function() {
            this.createFeedbackPanel();
            this.createMissedDetectionButton();
            console.log('[Feedback] Module initialized');
        },

        /**
         * Create the feedback panel (hidden by default)
         */
        createFeedbackPanel: function() {
            var panel = document.createElement('div');
            panel.id = 'feedback-panel';
            panel.className = 'feedback-panel';
            panel.style.display = 'none';
            panel.innerHTML = '\
                <div class="feedback-header">\
                    <span class="feedback-title">Report Feedback</span>\
                    <button class="feedback-close" onclick="Feedback.hideFeedbackPanel()">&times;</button>\
                </div>\
                <div class="feedback-content">\
                    <div class="feedback-event-info">\
                        <span class="feedback-event-type"></span>\
                        <span class="feedback-event-time"></span>\
                    </div>\
                    <div class="feedback-form">\
                        <p class="feedback-question">What was wrong?</p>\
                        <div class="feedback-options">\
                            <label class="feedback-option">\
                                <input type="radio" name="feedback-type" value="FALSE_POSITIVE">\
                                <span>No one was there (false alarm)</span>\
                            </label>\
                            <label class="feedback-option">\
                                <input type="radio" name="feedback-type" value="FALSE_NEGATIVE">\
                                <span>Someone was missed at this location</span>\
                            </label>\
                            <label class="feedback-option">\
                                <input type="radio" name="feedback-type" value="WRONG_IDENTITY">\
                                <span>Wrong person identified</span>\
                            </label>\
                            <label class="feedback-option">\
                                <input type="radio" name="feedback-type" value="WRONG_ZONE">\
                                <span>Wrong zone/location</span>\
                            </label>\
                        </div>\
                        <div class="feedback-notes">\
                            <label>Notes (optional)</label>\
                            <textarea id="feedback-notes" placeholder="Additional details..."></textarea>\
                        </div>\
                        <div class="feedback-actions">\
                            <button class="feedback-btn feedback-btn-cancel" onclick="Feedback.hideFeedbackPanel()">Cancel</button>\
                            <button class="feedback-btn feedback-btn-submit" onclick="Feedback.submitFeedback()">Submit</button>\
                        </div>\
                    </div>\
                </div>';
            document.body.appendChild(panel);

            // Add styles
            this.addStyles();
        },

        /**
         * Create the "Report missed detection" button
         */
        createMissedDetectionButton: function() {
            var btn = document.createElement('button');
            btn.id = 'missed-detection-btn';
            btn.className = 'missed-detection-btn';
            btn.innerHTML = '&#x26A0; Report missed detection';
            btn.onclick = function() { Feedback.showMissedDetectionForm(); };
            document.body.appendChild(btn);
        },

        /**
         * Show feedback panel for a specific event (thumbs-down clicked)
         */
        showFeedbackPanel: function(eventID, eventType, eventTime, details) {
            this.pendingFeedback = {
                eventID: eventID,
                eventType: eventType,
                eventTime: eventTime,
                details: details || {}
            };

            var panel = document.getElementById('feedback-panel');
            if (!panel) return;

            // Update event info
            panel.querySelector('.feedback-event-type').textContent = this.formatEventType(eventType);
            panel.querySelector('.feedback-event-time').textContent = eventTime ? new Date(eventTime).toLocaleString() : 'Now';

            // Reset form
            var radios = panel.querySelectorAll('input[name="feedback-type"]');
            radios.forEach(function(r) { r.checked = false; });
            document.getElementById('feedback-notes').value = '';

            // Show panel
            panel.style.display = 'block';
            this.feedbackPanelVisible = true;
        },

        /**
         * Hide the feedback panel
         */
        hideFeedbackPanel: function() {
            var panel = document.getElementById('feedback-panel');
            if (panel) {
                panel.style.display = 'none';
            }
            this.pendingFeedback = null;
            this.feedbackPanelVisible = false;
        },

        /**
         * Submit feedback to the server
         */
        submitFeedback: function() {
            if (!this.pendingFeedback) return;

            var panel = document.getElementById('feedback-panel');
            var selected = panel.querySelector('input[name="feedback-type"]:checked');

            if (!selected) {
                alert('Please select what was wrong with this detection.');
                return;
            }

            var feedbackType = selected.value;
            var notes = document.getElementById('feedback-notes').value;

            var details = Object.assign({}, this.pendingFeedback.details);
            if (notes) {
                details.notes = notes;
            }

            this.sendFeedback(
                this.pendingFeedback.eventID,
                this.pendingFeedback.eventType,
                feedbackType,
                details
            );

            this.hideFeedbackPanel();
        },

        /**
         * Submit thumbs-up (true positive) feedback
         */
        submitThumbsUp: function(eventID, eventType, details) {
            this.sendFeedback(eventID, eventType, this.FeedbackTypes.TRUE_POSITIVE, details || {});
        },

        /**
         * Send feedback to the API
         */
        sendFeedback: function(eventID, eventType, feedbackType, details) {
            // Map feedback types to API types
            var apiType = 'correct';
            if (feedbackType === this.FeedbackTypes.FALSE_POSITIVE || feedbackType === 'FALSE_POSITIVE') {
                apiType = 'incorrect';
            } else if (feedbackType === this.FeedbackTypes.FALSE_NEGATIVE || feedbackType === 'FALSE_NEGATIVE') {
                apiType = 'missed';
            } else if (feedbackType === this.FeedbackTypes.TRUE_POSITIVE || feedbackType === 'TRUE_POSITIVE') {
                apiType = 'correct';
            }

            // Extract blob_id from details if available
            var blobID = details && details.blob_id ? details.blob_id : 0;

            // Extract position from details if available
            var position = null;
            if (details && (details.position_x !== undefined || details.zone_id !== undefined)) {
                position = {
                    x: details.position_x || 0,
                    y: details.position_y || 0,
                    z: details.position_z || 0
                };
            }

            var data = {
                type: apiType,
                event_id: eventID || 0,
                blob_id: blobID
            };

            // Add position if present
            if (position) {
                data.position = position;
            }

            fetch('/api/feedback', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(data)
            })
            .then(function(res) { return res.json(); })
            .then(function(result) {
                if (result.ok) {
                    console.log('[Feedback] Submitted:', apiType, 'for event', eventID);

                    // Show inline response if provided
                    if (result.inline_response) {
                        Feedback.showInlineResponse(result.inline_response);
                    } else if (window.SpaxelApp && window.SpaxelApp.showToast) {
                        window.SpaxelApp.showToast('Thank you for your feedback!', 'success');
                    }
                }
            })
            .catch(function(err) {
                console.error('[Feedback] Failed to submit:', err);
            });
        },

        /**
         * Show missed detection form
         */
        showMissedDetectionForm: function() {
            var modal = document.createElement('div');
            modal.id = 'missed-detection-modal';
            modal.className = 'missed-detection-modal';
            modal.innerHTML = '\
                <div class="missed-detection-card">\
                    <div class="missed-detection-header">\
                        <h2>Report Missed Detection</h2>\
                        <button class="modal-close" onclick="Feedback.closeMissedDetectionModal()">&times;</button>\
                    </div>\
                    <div class="missed-detection-form">\
                        <div class="form-group">\
                            <label>When did this happen?</label>\
                            <input type="datetime-local" id="missed-time">\
                        </div>\
                        <div class="form-group">\
                            <label>Where? (Zone)</label>\
                            <select id="missed-zone">\
                                <option value="">Select zone...</option>\
                            </select>\
                        </div>\
                        <div class="form-group">\
                            <label>Position (optional)</label>\
                            <div class="position-inputs">\
                                <input type="number" id="missed-x" placeholder="X" step="0.1">\
                                <input type="number" id="missed-y" placeholder="Y" step="0.1">\
                                <input type="number" id="missed-z" placeholder="Z" step="0.1">\
                            </div>\
                        </div>\
                        <div class="form-group">\
                            <label>Notes (optional)</label>\
                            <textarea id="missed-notes" placeholder="Any additional details..."></textarea>\
                        </div>\
                        <div class="form-actions">\
                            <button class="btn btn-secondary" onclick="Feedback.closeMissedDetectionModal()">Cancel</button>\
                            <button class="btn btn-primary" onclick="Feedback.submitMissedDetection()">Submit</button>\
                        </div>\
                    </div>\
                </div>';

            document.body.appendChild(modal);

            // Set default time to now
            var timeInput = document.getElementById('missed-time');
            var now = new Date();
            timeInput.value = now.toISOString().slice(0, 16);

            // Populate zones
            this.populateZoneSelector(document.getElementById('missed-zone'));
        },

        /**
         * Close the missed detection modal
         */
        closeMissedDetectionModal: function() {
            var modal = document.getElementById('missed-detection-modal');
            if (modal) {
                modal.remove();
            }
        },

        /**
         * Submit missed detection report
         */
        submitMissedDetection: function() {
            var timeStr = document.getElementById('missed-time').value;
            var zoneID = document.getElementById('missed-zone').value;
            var posX = parseFloat(document.getElementById('missed-x').value) || 0;
            var posY = parseFloat(document.getElementById('missed-y').value) || 0;
            var posZ = parseFloat(document.getElementById('missed-z').value) || 0;
            var notes = document.getElementById('missed-notes').value;

            var details = {
                zone_id: zoneID,
                position_x: posX,
                position_y: posY,
                position_z: posZ,
                user_reported: true
            };

            if (notes) {
                details.notes = notes;
            }

            this.sendFeedback(
                '', // No event ID for missed detections
                this.EventTypes.BLOB_DETECTION,
                this.FeedbackTypes.FALSE_NEGATIVE,
                details
            );

            this.closeMissedDetectionModal();
        },

        /**
         * Populate zone selector from API
         */
        populateZoneSelector: function(select) {
            if (!select) return;

            fetch('/api/zones')
                .then(function(res) { return res.json(); })
                .then(function(zones) {
                    zones = zones || [];
                    zones.forEach(function(zone) {
                        var opt = document.createElement('option');
                        opt.value = zone.id;
                        opt.textContent = zone.name || zone.id;
                        select.appendChild(opt);
                    });
                })
                .catch(function(err) {
                    console.error('[Feedback] Failed to load zones:', err);
                });
        },

        /**
         * Show inline response message for feedback
         */
        showInlineResponse: function(response) {
            // Create inline response element
            var inline = document.createElement('div');
            inline.className = 'feedback-inline-response feedback-inline-' + (response.type || 'info');

            var content = '\
                <div class="feedback-inline-header">' + this.escapeHTML(response.title || 'Feedback recorded') + '</div>\
                <div class="feedback-inline-message">' + this.escapeHTML(response.message || '') + '</div>\
            ';

            // Add explanation section if available (for FALSE_POSITIVE feedback)
            if (response.explainability && response.type === 'adjustment') {
                content += this.renderFeedbackExplanation(response.explainability);
            }

            content += '<button class="feedback-inline-close" onclick="this.parentElement.remove()">\u00D7</button>';

            inline.innerHTML = content;

            // Add to timeline or body
            var timeline = document.querySelector('.timeline-events') || document.body;
            timeline.insertBefore(inline, timeline.firstChild);

            // Auto-dismiss after 15 seconds for explanations (longer to read)
            var dismissTime = response.explainability ? 15000 : 8000;
            setTimeout(function() {
                if (inline.parentNode) {
                    inline.classList.add('feedback-inline-fadeout');
                    setTimeout(function() {
                        if (inline.parentNode) {
                            inline.parentNode.removeChild(inline);
                        }
                    }, 500);
                }
            }, dismissTime);
        },

        /**
         * Render the feedback explanation for FALSE_POSITIVE detections
         */
        renderFeedbackExplanation: function(explainability) {
            var contributingLinks = explainability.contributing_links || [];
            var primaryLink = contributingLinks.length > 0 ? contributingLinks[0] : null;
            var diagnosis = explainability.diagnosis || null;

            var explanationHTML = '\
                <div class="feedback-explanation">\
                    <div class="explanation-toggle" onclick="this.classList.toggle(\'expanded\');">\
                        <span class="explanation-icon">?</span>\
                        <span class="explanation-label">Why did this happen?</span>\
                        <span class="explanation-arrow">▼</span>\
                    </div>\
                    <div class="explanation-content">\
            ';

            if (primaryLink) {
                var linkName = this.formatLinkID(primaryLink.link_id);
                var deltaRMS = primaryLink.delta_rms ? primaryLink.delta_rms.toFixed(4) : 'N/A';
                var threshold = 0.02; // Standard threshold
                var ratio = primaryLink.delta_rms ? (primaryLink.delta_rms / threshold).toFixed(1) : 'N/A';
                var timestamp = explainability.timestamp_ms ? new Date(explainability.timestamp_ms).toLocaleTimeString() : 'unknown';

                explanationHTML += '\
                    <p class="explanation-text">\
                        The system detected motion here because:\
                        <strong>' + linkName + '</strong>\'s signal (deltaRMS: ' + deltaRMS + ') exceeded the motion threshold by\
                        <strong>' + ratio + 'x</strong> at ' + timestamp + '.\
                    </p>\
                ';

                // Add diagnostic info if available
                if (diagnosis) {
                    explanationHTML += '\
                        <div class="explanation-diagnosis">\
                            <span class="diagnosis-label">Possible cause:</span>\
                            <span class="diagnosis-detail">' + this.escapeHTML(diagnosis.detail || diagnosis.title || 'Ambient RF interference') + '</span>\
                    ';

                    if (diagnosis.advice) {
                        explanationHTML += '\
                            <div class="diagnosis-advice">\
                                <span class="advice-label">What to do:</span>\
                                <span class="advice-text">' + this.escapeHTML(diagnosis.advice) + '</span>\
                            </div>\
                        ';
                    }

                    explanationHTML += '</div>';
                } else {
                    explanationHTML += '\
                        <p class="explanation-root-cause">\
                            <strong>Possible cause:</strong> Ambient RF interference or environmental changes.\
                        </p>\
                    ';
                }

                // Add additional contributing links if any
                if (contributingLinks.length > 1) {
                    var linkNames = contributingLinks.slice(1).map(function(l) {
                        return this.formatLinkID(l.link_id);
                    }.bind(this)).join(', ');

                    explanationHTML += '\
                        <p class="explanation-additional-links">\
                            Contributing links: ' + linkNames + '\
                        </p>\
                    ';
                }

                // Add correction note
                explanationHTML += '\
                    <p class="explanation-correction">\
                        <em>We\'ve noted this feedback and will apply corrections to improve future detection accuracy.</em>\
                    </p>\
                ';
            } else {
                explanationHTML += '\
                    <p class="explanation-text">\
                        The system detected motion based on signal patterns across multiple links.\
                        We\'ve noted this feedback to improve accuracy.\
                    </p>\
                ';
            }

            explanationHTML += '\
                    </div>\
                </div>\
            ';

            return explanationHTML;
        },

        /**
         * Format a link ID for display
         */
        formatLinkID: function(linkID) {
            if (!linkID) {
                return 'Unknown Link';
            }
            // Format MAC address pairs nicely
            var parts = linkID.split(':');
            if (parts.length === 2) {
                var mac1 = parts[0].substring(0, 8); // First 8 chars of first MAC
                var mac2 = parts[1].substring(0, 8); // First 8 chars of second MAC
                return mac1 + ' → ' + mac2;
            }
            return linkID.substring(0, 16) + '...';
        },

        /**
         * Create thumbs up/down buttons for an event
         */
        createFeedbackButtons: function(eventID, eventType, eventTime, details) {
            var container = document.createElement('div');
            container.className = 'feedback-buttons';

            var thumbsUp = document.createElement('button');
            thumbsUp.className = 'feedback-btn-icon feedback-thumbs-up';
            thumbsUp.innerHTML = '&#x1F44D;';
            thumbsUp.title = 'Correct detection';
            thumbsUp.onclick = function(e) {
                e.stopPropagation();
                Feedback.submitThumbsUp(eventID, eventType, details);
            };

            var thumbsDown = document.createElement('button');
            thumbsDown.className = 'feedback-btn-icon feedback-thumbs-down';
            thumbsDown.innerHTML = '&#x1F44E;';
            thumbsDown.title = 'Incorrect detection';
            thumbsDown.onclick = function(e) {
                e.stopPropagation();
                Feedback.showFeedbackPanel(eventID, eventType, eventTime, details);
            };

            container.appendChild(thumbsUp);
            container.appendChild(thumbsDown);

            return container;
        },

        /**
         * Format event type for display
         */
        formatEventType: function(eventType) {
            var types = {
                'blob_detection': 'Detection',
                'zone_transition': 'Zone Change',
                'fall_alert': 'Fall Alert',
                'anomaly': 'Anomaly'
            };
            return types[eventType] || eventType;
        },

        /**
         * Add CSS styles for feedback UI
         */
        addStyles: function() {
            if (document.getElementById('feedback-styles')) return;

            var style = document.createElement('style');
            style.id = 'feedback-styles';
            style.textContent = '\
                .feedback-panel {\
                    position: fixed;\
                    bottom: 100px;\
                    right: 20px;\
                    width: 320px;\
                    background: rgba(0, 0, 0, 0.95);\
                    border-radius: 8px;\
                    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.5);\
                    z-index: 200;\
                    font-size: 13px;\
                }\
                .feedback-header {\
                    display: flex;\
                    justify-content: space-between;\
                    align-items: center;\
                    padding: 12px 16px;\
                    border-bottom: 1px solid rgba(255, 255, 255, 0.1);\
                }\
                .feedback-title {\
                    font-weight: 600;\
                    color: #eee;\
                }\
                .feedback-close {\
                    background: none;\
                    border: none;\
                    color: #888;\
                    font-size: 20px;\
                    cursor: pointer;\
                }\
                .feedback-close:hover { color: #fff; }\
                .feedback-content {\
                    padding: 16px;\
                }\
                .feedback-event-info {\
                    display: flex;\
                    justify-content: space-between;\
                    margin-bottom: 12px;\
                    font-size: 11px;\
                    color: #888;\
                }\
                .feedback-question {\
                    font-size: 13px;\
                    color: #ccc;\
                    margin-bottom: 10px;\
                }\
                .feedback-options {\
                    display: flex;\
                    flex-direction: column;\
                    gap: 8px;\
                    margin-bottom: 16px;\
                }\
                .feedback-option {\
                    display: flex;\
                    align-items: center;\
                    gap: 8px;\
                    cursor: pointer;\
                    padding: 6px 8px;\
                    border-radius: 4px;\
                    transition: background 0.2s;\
                }\
                .feedback-option:hover {\
                    background: rgba(255, 255, 255, 0.05);\
                }\
                .feedback-option input {\
                    margin: 0;\
                }\
                .feedback-option span {\
                    color: #bbb;\
                    font-size: 12px;\
                }\
                .feedback-notes {\
                    margin-bottom: 16px;\
                }\
                .feedback-notes label {\
                    display: block;\
                    font-size: 11px;\
                    color: #888;\
                    margin-bottom: 4px;\
                }\
                .feedback-notes textarea {\
                    width: 100%;\
                    height: 60px;\
                    background: rgba(255, 255, 255, 0.08);\
                    border: 1px solid rgba(255, 255, 255, 0.15);\
                    border-radius: 4px;\
                    color: #eee;\
                    font-size: 12px;\
                    padding: 8px;\
                    resize: none;\
                    box-sizing: border-box;\
                }\
                .feedback-actions {\
                    display: flex;\
                    justify-content: flex-end;\
                    gap: 8px;\
                }\
                .feedback-btn {\
                    padding: 6px 14px;\
                    border-radius: 4px;\
                    font-size: 12px;\
                    cursor: pointer;\
                    border: none;\
                }\
                .feedback-btn-cancel {\
                    background: rgba(255, 255, 255, 0.1);\
                    color: #ccc;\
                }\
                .feedback-btn-submit {\
                    background: #4fc3f7;\
                    color: #1a1a2e;\
                    font-weight: 500;\
                }\
                .feedback-btn-icon {\
                    background: rgba(255, 255, 255, 0.1);\
                    border: none;\
                    width: 28px;\
                    height: 28px;\
                    border-radius: 4px;\
                    cursor: pointer;\
                    font-size: 14px;\
                    display: flex;\
                    align-items: center;\
                    justify-content: center;\
                    transition: background 0.2s;\
                }\
                .feedback-btn-icon:hover {\
                    background: rgba(255, 255, 255, 0.2);\
                }\
                .feedback-thumbs-up:hover {\
                    background: rgba(76, 175, 80, 0.3);\
                }\
                .feedback-thumbs-down:hover {\
                    background: rgba(244, 67, 54, 0.3);\
                }\
                .feedback-buttons {\
                    display: inline-flex;\
                    gap: 4px;\
                }\
                .missed-detection-btn {\
                    position: fixed;\
                    bottom: 20px;\
                    right: 440px;\
                    background: rgba(255, 167, 38, 0.2);\
                    border: 1px solid rgba(255, 167, 38, 0.5);\
                    color: #ffa726;\
                    padding: 6px 12px;\
                    border-radius: 4px;\
                    font-size: 11px;\
                    cursor: pointer;\
                    z-index: 100;\
                    transition: background 0.2s;\
                }\
                .missed-detection-btn:hover {\
                    background: rgba(255, 167, 38, 0.3);\
                }\
                .missed-detection-modal {\
                    position: fixed;\
                    top: 0;\
                    left: 0;\
                    right: 0;\
                    bottom: 0;\
                    background: rgba(0, 0, 0, 0.8);\
                    display: flex;\
                    align-items: center;\
                    justify-content: center;\
                    z-index: 300;\
                }\
                .missed-detection-card {\
                    background: #1e1e3a;\
                    border-radius: 12px;\
                    padding: 24px;\
                    width: 400px;\
                    max-width: 90%;\
                }\
                .missed-detection-header {\
                    display: flex;\
                    justify-content: space-between;\
                    align-items: center;\
                    margin-bottom: 16px;\
                }\
                .missed-detection-header h2 {\
                    font-size: 16px;\
                    color: #eee;\
                    margin: 0;\
                }\
                .modal-close {\
                    background: none;\
                    border: none;\
                    color: #888;\
                    font-size: 24px;\
                    cursor: pointer;\
                }\
                .modal-close:hover { color: #fff; }\
                .missed-detection-form .form-group {\
                    margin-bottom: 14px;\
                }\
                .missed-detection-form label {\
                    display: block;\
                    font-size: 12px;\
                    color: #888;\
                    margin-bottom: 4px;\
                }\
                .missed-detection-form input,\
                .missed-detection-form select,\
                .missed-detection-form textarea {\
                    width: 100%;\
                    padding: 8px 10px;\
                    background: rgba(255, 255, 255, 0.08);\
                    border: 1px solid rgba(255, 255, 255, 0.15);\
                    border-radius: 4px;\
                    color: #eee;\
                    font-size: 13px;\
                    box-sizing: border-box;\
                }\
                .position-inputs {\
                    display: flex;\
                    gap: 8px;\
                }\
                .position-inputs input {\
                    flex: 1;\
                }\
                .missed-detection-form textarea {\
                    height: 60px;\
                    resize: none;\
                }\
                .form-actions {\
                    display: flex;\
                    justify-content: flex-end;\
                    gap: 10px;\
                    margin-top: 20px;\
                }\
                .btn {\
                    padding: 8px 16px;\
                    border-radius: 4px;\
                    font-size: 13px;\
                    cursor: pointer;\
                    border: none;\
                }\
                .btn-secondary {\
                    background: rgba(255, 255, 255, 0.1);\
                    color: #ccc;\
                }\
                .btn-primary {\
                    background: #4fc3f7;\
                    color: #1a1a2e;\
                    font-weight: 500;\
                }\
                .feedback-inline-response {\
                    position: relative;\
                    background: rgba(76, 175, 80, 0.1);\
                    border-left: 3px solid #4caf50;\
                    border-radius: 4px;\
                    padding: 12px 16px;\
                    margin-bottom: 12px;\
                    animation: feedbackSlideIn 0.3s ease-out;\
                }\
                .feedback-inline-info {\
                    background: rgba(79, 195, 247, 0.1);\
                    border-left-color: #4fc3f7;\
                }\
                .feedback-inline-adjustment {\
                    background: rgba(255, 167, 38, 0.1);\
                    border-left-color: #ffa726;\
                }\
                .feedback-inline-header {\
                    font-weight: 600;\
                    font-size: 13px;\
                    color: #eee;\
                    margin-bottom: 4px;\
                }\
                .feedback-inline-message {\
                    font-size: 12px;\
                    color: #bbb;\
                    line-height: 1.4;\
                }\
                .feedback-inline-close {\
                    position: absolute;\
                    top: 8px;\
                    right: 8px;\
                    background: none;\
                    border: none;\
                    color: #888;\
                    font-size: 16px;\
                    cursor: pointer;\
                    padding: 4px;\
                }\
                .feedback-inline-close:hover {\
                    color: #fff;\
                }\
                .feedback-inline-fadeout {\
                    animation: feedbackFadeOut 0.5s ease-out forwards;\
                }\
                @keyframes feedbackSlideIn {\
                    from {\
                        opacity: 0;\
                        transform: translateY(-10px);\
                    }\
                    to {\
                        opacity: 1;\
                        transform: translateY(0);\
                    }\
                }\
                @keyframes feedbackFadeOut {\
                    to {\
                        opacity: 0;\
                        max-height: 0;\
                        margin: 0;\
                        padding: 0;\
                    }\
                }\
                .feedback-explanation {\
                    margin-top: 12px;\
                    padding-top: 12px;\
                    border-top: 1px solid rgba(255, 255, 255, 0.1);\
                }\
                .explanation-toggle {\
                    display: flex;\
                    align-items: center;\
                    gap: 8px;\
                    cursor: pointer;\
                    user-select: none;\
                    padding: 6px 8px;\
                    background: rgba(255, 255, 255, 0.05);\
                    border-radius: 4px;\
                    transition: background 0.2s;\
                }\
                .explanation-toggle:hover {\
                    background: rgba(255, 255, 255, 0.08);\
                }\
                .explanation-icon {\
                    display: flex;\
                    align-items: center;\
                    justify-content: center;\
                    width: 20px;\
                    height: 20px;\
                    background: rgba(79, 195, 247, 0.2);\
                    color: #4fc3f7;\
                    border-radius: 50%;\
                    font-weight: 600;\
                    font-size: 12px;\
                }\
                .explanation-label {\
                    flex: 1;\
                    font-size: 12px;\
                    font-weight: 500;\
                    color: #4fc3f7;\
                }\
                .explanation-arrow {\
                    font-size: 10px;\
                    color: #888;\
                    transition: transform 0.2s;\
                }\
                .explanation-toggle.expanded .explanation-arrow {\
                    transform: rotate(180deg);\
                }\
                .explanation-content {\
                    display: none;\
                    margin-top: 8px;\
                    padding: 10px;\
                    background: rgba(79, 195, 247, 0.05);\
                    border-radius: 4px;\
                    font-size: 11px;\
                }\
                .explanation-toggle.expanded + .explanation-content {\
                    display: block;\
                    animation: explanationSlideIn 0.2s ease-out;\
                }\
                @keyframes explanationSlideIn {\
                    from {\
                        opacity: 0;\
                        max-height: 0;\
                    }\
                    to {\
                        opacity: 1;\
                        max-height: 500px;\
                    }\
                }\
                .explanation-text {\
                    color: #bbb;\
                    line-height: 1.4;\
                    margin: 0 0 8px 0;\
                }\
                .explanation-text strong {\
                    color: #4fc3f7;\
                    font-weight: 600;\
                }\
                .explanation-root-cause {\
                    color: #bbb;\
                    line-height: 1.4;\
                    margin: 0 0 8px 0;\
                }\
                .explanation-root-cause strong {\
                    color: #ffa726;\
                }\
                .explanation-diagnosis {\
                    margin: 8px 0;\
                    padding: 8px;\
                    background: rgba(255, 167, 38, 0.08);\
                    border-left: 2px solid #ffa726;\
                    border-radius: 3px;\
                }\
                .diagnosis-label {\
                    display: block;\
                    font-size: 10px;\
                    text-transform: uppercase;\
                    color: #ffa726;\
                    font-weight: 600;\
                    margin-bottom: 3px;\
                }\
                .diagnosis-detail {\
                    display: block;\
                    color: #ccc;\
                    line-height: 1.3;\
                    margin-bottom: 4px;\
                }\
                .diagnosis-advice {\
                    margin-top: 6px;\
                    padding-top: 6px;\
                    border-top: 1px solid rgba(255, 255, 255, 0.1);\
                }\
                .advice-label {\
                    display: block;\
                    font-size: 10px;\
                    text-transform: uppercase;\
                    color: #4caf50;\
                    font-weight: 600;\
                    margin-bottom: 2px;\
                }\
                .advice-text {\
                    color: #bbb;\
                    line-height: 1.3;\
                }\
                .explanation-additional-links {\
                    margin: 8px 0;\
                    padding-top: 8px;\
                    border-top: 1px solid rgba(255, 255, 255, 0.1);\
                    font-size: 10px;\
                    color: #888;\
                }\
                .explanation-correction {\
                    margin: 8px 0 0 0;\
                    padding: 6px 8px;\
                    background: rgba(76, 175, 80, 0.08);\
                    border-radius: 3px;\
                    font-size: 10px;\
                    color: #a5d6a7;\
                }';
            document.head.appendChild(style);
        }
    };

    // Expose globally
    window.Feedback = Feedback;

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function() { Feedback.init(); });
    } else {
        Feedback.init();
    }
})();
