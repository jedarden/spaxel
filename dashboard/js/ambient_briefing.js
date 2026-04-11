/**
 * Spaxel Dashboard - Ambient Mode Morning Briefing
 *
 * Morning briefing overlay for ambient display mode.
 * Shows sleep summary, expected departures, and system status.
 * Appears once per day when first presence is detected after 6am.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const BRIEFING_DURATION_MS = 15000; // 15 seconds
    const MORNING_START_HOUR = 6;         // 6am
    const BRIEFING_END_HOUR = 12;          // 12pm (noon)
    const LOCAL_STORAGE_KEY = 'ambient_briefing_last_shown';

    // ============================================
    // State
    // ============================================
    let briefingElement = null;
    let briefingTimer = null;
    let isActive = false;
    let isFirstDetectionToday = false;
    let hasShownToday = false;

    // Callbacks
    let onDismiss = null;
    let onFetchBriefing = null;

    // ============================================
    // Public API
    // ============================================
    const AmbientBriefing = {
        /**
         * Initialize the morning briefing module
         */
        init() {
            // Create briefing element if it doesn't exist
            ensureBriefingElement();

            // Set up event listeners
            setupEventListeners();

            // Check if we should be listening for first detection
            checkFirstDetectionToday();

            console.log('[AmbientBriefing] Initialized');
        },

        /**
         * Show the morning briefing
         * @param {Object} briefingData - Briefing content from API
         */
        show(briefingData) {
            if (isActive) {
                return; // Already showing
            }

            ensureBriefingElement();
            populateBriefing(briefingData);

            briefingElement.classList.remove('hidden');
            briefingElement.classList.add('visible');
            isActive = true;

            // Auto-dismiss after duration
            briefingTimer = setTimeout(() => {
                dismiss();
            }, BRIEFING_DURATION_MS);

            console.log('[AmbientBriefing] Showing briefing');
        },

        /**
         * Dismiss the morning briefing
         */
        dismiss() {
            if (!isActive) {
                return;
            }

            if (briefingElement) {
                briefingElement.classList.remove('visible');
                briefingElement.classList.add('hidden');
            }

            isActive = false;

            if (briefingTimer) {
                clearTimeout(briefingTimer);
                briefingTimer = null;
            }

            // Mark as shown for today
            markAsShown();

            if (onDismiss) {
                onDismiss();
            }

            console.log('[AmbientBriefing] Dismissed');
        },

        /**
         * Check if briefing should be shown today
         * @returns {Promise<boolean>} - True if briefing should be shown
         */
        async shouldShowToday() {
            // Check if already shown today
            if (hasShownToday) {
                return false;
            }

            const lastShown = localStorage.getItem(LOCAL_STORAGE_KEY);
            const today = new Date().toISOString().split('T')[0];

            if (lastShown === today) {
                hasShownToday = true;
                return false;
            }

            // Check if it's morning
            const hour = new Date().getHours();
            if (hour < MORNING_START_HOUR || hour >= BRIEFING_END_HOUR) {
                return false;
            }

            return true;
        },

        /**
         * Fetch and show briefing from API
         */
        async fetchAndShow() {
            if (!(await this.shouldShowToday())) {
                return;
            }

            try {
                const today = new Date().toISOString().split('T')[0];
                let briefingData;

                if (onFetchBriefing) {
                    // Use custom fetch function
                    briefingData = await onFetchBriefing(today);
                } else {
                    // Default fetch from API
                    const response = await fetch(`/api/briefing?date=${today}`);
                    if (!response.ok) {
                        throw new Error(`HTTP ${response.status}`);
                    }
                    briefingData = await response.json();
                }

                this.show(briefingData);
            } catch (error) {
                console.error('[AmbientBriefing] Error fetching briefing:', error);
            }
        },

        /**
         * Set dismiss callback
         * @param {Function} callback - Function to call when briefing is dismissed
         */
        setOnDismiss(callback) {
            onDismiss = callback;
        },

        /**
         * Set fetch briefing callback
         * @param {Function} callback - Function to fetch briefing data
         */
        setOnFetchBriefing(callback) {
            onFetchBriefing = callback;
        },

        /**
         * Trigger first detection (call when first person detected)
         */
        onFirstDetection() {
            if (isFirstDetectionToday && !hasShownToday) {
                isFirstDetectionToday = false;
                this.fetchAndShow();
            }
        },

        /**
         * Reset the daily flag (for testing)
         */
        resetDailyFlag() {
            localStorage.removeItem(LOCAL_STORAGE_KEY);
            hasShownToday = false;
            isFirstDetectionToday = false;
        }
    };

    // ============================================
    // Internal Functions
    // ============================================

    function ensureBriefingElement() {
        if (briefingElement) {
            return;
        }

        // Check if element already exists in DOM
        briefingElement = document.getElementById('ambient-briefing');
        if (briefingElement) {
            return;
        }

        // Create briefing element
        briefingElement = document.createElement('div');
        briefingElement.id = 'ambient-briefing';
        briefingElement.className = 'ambient-briefing hidden';
        briefingElement.innerHTML = `
            <div class="ambient-briefing-content">
                <div class="ambient-briefing-greeting" id="briefing-greeting"></div>
                <div id="briefing-content" class="ambient-briefing-sections"></div>
                <button class="ambient-briefing-dismiss" id="briefing-dismiss">Got it</button>
            </div>
        `;

        document.body.appendChild(briefingElement);
    }

    function setupEventListeners() {
        // Dismiss button
        const dismissBtn = document.getElementById('briefing-dismiss');
        if (dismissBtn) {
            // Remove any existing listeners to avoid duplicates
            const newBtn = dismissBtn.cloneNode(true);
            dismissBtn.parentNode.replaceChild(newBtn, dismissBtn);

            newBtn.addEventListener('click', () => {
                AmbientBriefing.dismiss();
            });
        }

        // Dismiss on tap/click outside content
        if (briefingElement) {
            briefingElement.addEventListener('click', (e) => {
                if (e.target === briefingElement) {
                    AmbientBriefing.dismiss();
                }
            });
        }
    }

    function populateBriefing(data) {
        const greetingEl = document.getElementById('briefing-greeting');
        const contentEl = document.getElementById('briefing-content');

        if (!greetingEl || !contentEl) {
            return;
        }

        // Set greeting based on time of day
        const hour = new Date().getHours();
        let greeting = 'Good morning';
        if (hour < 12) {
            greeting = 'Good morning';
        } else if (hour < 17) {
            greeting = 'Good afternoon';
        } else {
            greeting = 'Good evening';
        }

        greetingEl.textContent = greeting;

        // Parse briefing content
        const sections = parseBriefingContent(data);
        contentEl.innerHTML = sections;
    }

    function parseBriefingContent(data) {
        let html = '';

        // Handle different briefing data formats
        if (data.content) {
            // Text content - parse for sections
            const lines = data.content.split('\n').filter(line => line.trim());

            // Group lines into sections
            let currentSection = null;
            let sectionContent = [];

            lines.forEach(line => {
                // Check if this is a section header
                if (line.includes(':') && line.length < 50) {
                    // Save previous section
                    if (currentSection && sectionContent.length > 0) {
                        html += createBriefingSection(currentSection, sectionContent.join('<br>'));
                    }
                    // Start new section
                    const parts = line.split(':');
                    currentSection = parts[0].trim();
                    sectionContent = [parts.slice(1).join(':').trim()];
                } else if (currentSection) {
                    sectionContent.push(line);
                } else {
                    // No section yet, add as general content
                    if (!currentSection) {
                        currentSection = 'Summary';
                        sectionContent = [];
                    }
                    sectionContent.push(line);
                }
            });

            // Don't forget the last section
            if (currentSection && sectionContent.length > 0) {
                html += createBriefingSection(currentSection, sectionContent.join('<br>'));
            }
        } else {
            // Structured data - extract key information
            if (data.sleep_summary) {
                html += createBriefingSection('Sleep Summary', data.sleep_summary);
            }

            if (data.departures && data.departures.length > 0) {
                const departuresText = data.departures.map(d =>
                    `${d.person}: likely leaves at ${d.time}`
                ).join('<br>');
                html += createBriefingSection('Expected Departures', departuresText);
            }

            if (data.system_status) {
                html += createBriefingSection('System Status', data.system_status);
            }

            if (data.accuracy) {
                html += createBriefingSection('Accuracy', data.accuracy);
            }
        }

        return html || '<div class="ambient-briefing-section">No briefing data available</div>';
    }

    function createBriefingSection(label, content) {
        return `
            <div class="ambient-briefing-section">
                <div class="ambient-briefing-section-label">${label}</div>
                <div class="ambient-briefing-section-value">${content}</div>
            </div>
        `;
    }

    function dismiss() {
        AmbientBriefing.dismiss();
    }

    function markAsShown() {
        const today = new Date().toISOString().split('T')[0];
        localStorage.setItem(LOCAL_STORAGE_KEY, today);
        hasShownToday = true;
    }

    function checkFirstDetectionToday() {
        const hour = new Date().getHours();
        if (hour >= MORNING_START_HOUR && hour < BRIEFING_END_HOUR) {
            // It's morning - listen for first detection
            isFirstDetectionToday = true;
        } else {
            isFirstDetectionToday = false;
        }

        // Check if already shown today
        const lastShown = localStorage.getItem(LOCAL_STORAGE_KEY);
        const today = new Date().toISOString().split('T')[0];
        if (lastShown === today) {
            hasShownToday = true;
            isFirstDetectionToday = false;
        }
    }

    // ============================================
    // Export
    // ============================================
    window.SpaxelAmbientBriefing = AmbientBriefing;

    console.log('[AmbientBriefing] Module loaded');
})();
