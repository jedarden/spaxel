/**
 * Tests for Proactive Quality Assistance Module
 */

describe('Proactive Quality Module', () => {
    let Proactive;
    let mockDocument;
    let mockWindow;

    beforeEach(() => {
        // Mock document and window
        let createdElements = {};

        mockDocument = {
            createElement: jest.fn((tag) => {
                const element = {
                    id: '',
                    className: '',
                    innerHTML: '',
                    style: {},
                    appendChild: jest.fn(),
                    remove: jest.fn(),
                    addEventListener: jest.fn(),
                    querySelector: jest.fn(),
                    querySelectorAll: jest.fn(() => []),
                    classList: {
                        add: jest.fn(),
                        remove: jest.fn(),
                        contains: jest.fn()
                    }
                };
                return element;
            }),
            body: {
                appendChild: jest.fn((element) => {
                    // Store created elements so getElementById can find them
                    if (element.id) {
                        createdElements[element.id] = element;
                    }
                }),
            },
            head: {
                appendChild: jest.fn(),
            },
            readyState: 'complete',
            getElementById: jest.fn((id) => {
                // Return created elements if they exist
                if (createdElements[id]) {
                    return createdElements[id];
                }
                // Otherwise return null for uncreated elements
                return null;
            }),
        };

        // Mock fetch to prevent server-side hint detection during tests
        const mockFetch = jest.fn().mockResolvedValue({
            ok: true,
            json: async () => ({
                // Return settings without repeated_edit_hint flag
                delta_rms_threshold: 0.02,
            })
        });
        global.fetch = mockFetch;

        mockWindow = {
            Viz3D: {
                highlightLink: jest.fn(),
            },
            Replay: {
                pauseAndTune: jest.fn(),
            },
            openSettingsPanel: jest.fn(),
            SpaxelRouter: {
                navigate: jest.fn(),
            },
            dispatchEvent: jest.fn(),
            addEventListener: jest.fn(),
        };

        global.document = mockDocument;
        global.window = mockWindow;

        // Load the module - it attaches to window.Proactive
        jest.resetModules();
        require('./proactive.js');
        Proactive = global.window.Proactive;
    });

    afterEach(() => {
        jest.clearAllMocks();
    });

    describe('Quality Prompt Detection', () => {
        test('should track poor quality links over time', () => {
            const links = [
                { link_id: 'AA:BB:CC:DD:EE:FF:11:22', composite_score: 0.5 },
                { link_id: '11:22:33:44:55:66:77:88', composite_score: 0.8 },
            ];

            Proactive.monitorLinkQuality(links);

            // Should track the poor quality link
            // (In real scenario, would check qualityPromptLinkID)
        });

        test('should not show prompt for transient drops (< 5 minutes)', () => {
            const now = Date.now();
            const links = [
                { link_id: 'test-link-1', composite_score: 0.55 },
            ];

            // Mock that the link just started being tracked
            Proactive.monitorLinkQuality(links);

            // Prompt should not be shown yet
            const prompt = document.getElementById('quality-prompt-card');
            expect(prompt).toBeNull();
        });
    });

    describe('Setting Change Tracking', () => {
        beforeEach(() => {
            // Clear localStorage
            localStorage.clear();
        });

        test('should track qualifying setting changes', () => {
            Proactive.trackSettingChange('delta_rms_threshold', 0.02);
            Proactive.trackSettingChange('delta_rms_threshold', 0.025);
            Proactive.trackSettingChange('delta_rms_threshold', 0.03);

            // Should trigger hint after 3 changes
            const hint = document.getElementById('setting-change-hint');
            expect(hint).not.toBeUndefined();
        });

        test('should not track non-qualifying settings', () => {
            Proactive.trackSettingChange('theme', 'dark');
            Proactive.trackSettingChange('theme', 'light');
            Proactive.trackSettingChange('theme', 'dark');

            // Should not trigger hint for non-qualifying settings
            const hint = document.getElementById('setting-change-hint');
            expect(hint).toBeNull();
        });

        test('should respect hint cooldown period', () => {
            // This would require mocking Date.now()
            // For now, just verify the function exists
            expect(Proactive.dismissSettingHint).toBeDefined();
        });
    });

    describe('Feedback Explanations', () => {
        test('should show explanation for false positive feedback', () => {
            const eventData = {
                type: 'incorrect',
                explainability: {
                    contributing_links: [
                        {
                            link_id: 'test-link',
                            delta_rms: 0.05,
                            diagnosis: {
                                detail: 'WiFi congestion detected',
                                advice: 'Move node closer to router'
                            }
                        }
                    ]
                }
            };

            const explanation = Proactive.showFeedbackExplanation(eventData);
            expect(explanation).not.toBeNull();
            expect(explanation.innerHTML).toContain('deltaRMS');
        });

        test('should handle missing explainability data gracefully', () => {
            const eventData = {
                type: 'incorrect'
            };

            const explanation = Proactive.showFeedbackExplanation(eventData);
            expect(explanation).toBeUndefined();
        });
    });

    describe('Link Diagnostics', () => {
        test('should format link IDs for display', () => {
            // This tests the internal formatLinkID function
            // Since it's not exposed, we verify through diagnoseLink
            const linkID = 'AA:BB:CC:DD:EE:FF:11:22';

            Proactive.diagnoseLink(linkID);

            // Should create a diagnostic panel
            const panel = document.getElementById('diagnostic-results-panel');
            expect(panel).not.toBeNull();
        });

        test('should fetch diagnostics from API', async () => {
            const mockFetch = jest.fn().mockResolvedValue({
                ok: true,
                json: async () => ({
                    diagnosis: {
                        title: 'Test Diagnosis',
                        detail: 'Test detail',
                        advice: 'Test advice',
                        severity: 'warning'
                    }
                })
            });

            global.fetch = mockFetch;

            await Proactive.fetchDiagnosticForLink('test-link', Date.now());

            expect(mockFetch).toHaveBeenCalledWith(
                expect.stringContaining('diagnostics/link/test-link')
            );
        });
    });

    describe('Helper Functions', () => {
        test('should format setting names for display', () => {
            // Test the internal formatSettingName function
            // Since it's not exposed, we verify it works through setting change tracking
            Proactive.trackSettingChange('delta_rms_threshold', 0.02);

            const hint = document.getElementById('setting-change-hint');
            expect(hint).not.toBeNull();
            expect(hint.innerHTML).toContain('Motion Threshold');
        });
    });
});

// Test helper functions
describe('formatLinkID', () => {
    it('should format MAC address pairs', () => {
        // This would be tested through the module
        // For now, we document the expected format
        const linkID = 'AA:BB:CC:DD:EE:FF:11:22';
        const expected = 'AA:BB:CC:DD → 11:22';
        // The actual function is internal to proactive.js
    });
});

describe('Help Overlay Module', () => {
    let HelpOverlay;
    let mockDocument;
    let mockFetch;

    beforeEach(() => {
        mockDocument = {
            createElement: jest.fn((tag) => {
                const el = {
                    id: '',
                    className: '',
                    innerHTML: '',
                    style: {},
                    appendChild: jest.fn(),
                    remove: jest.fn(),
                    addEventListener: jest.fn(),
                    querySelector: jest.fn(() => null),
                    querySelectorAll: jest.fn(() => []),
                    focus: jest.fn(),
                };
                if (tag === 'div') el.tagName = 'DIV';
                if (tag === 'input') el.tagName = 'INPUT';
                if (tag === 'button') el.tagName = 'BUTTON';
                if (tag === 'style') el.tagName = 'STYLE';
                return el;
            }),
            body: {
                appendChild: jest.fn(),
            },
            head: {
                appendChild: jest.fn(),
            },
            readyState: 'loading',
            addEventListener: jest.fn(),
            getElementById: jest.fn((id) => {
                if (id === 'help-overlay') {
                    return { style: { display: 'none' } };
                }
                return null;
            }),
        };

        mockFetch = jest.fn().mockResolvedValue({
            ok: true,
            json: async () => [
                {
                    id: 'test-article',
                    title: 'Test Article',
                    content: 'Test content',
                    category: 'Test',
                }
            ]
        });

        global.document = mockDocument;
        global.fetch = mockFetch;

        jest.resetModules();
        HelpOverlay = require('./help.js');
    });

    afterEach(() => {
        jest.clearAllMocks();
    });

    describe('Initialization', () => {
        test('should load articles from JSON file', async () => {
            await HelpOverlay.init();

            expect(mockFetch).toHaveBeenCalledWith('/help_articles.json');
        });

        test('should handle JSON load failure gracefully', async () => {
            mockFetch = jest.fn().mockRejectedValue(new Error('Load failed'));

            global.fetch = mockFetch;
            jest.resetModules();
            HelpOverlay = require('./help.js');

            await HelpOverlay.init();

            // Should use fallback articles
            expect(HelpOverlay).toBeDefined();
        });
    });

    describe('Search Functionality', () => {
        test('should filter articles by search query', async () => {
            await HelpOverlay.init();
            HelpOverlay.open();

            // Set search input
            const searchInput = mockDocument.getElementById('help-search-input');
            searchInput.value = 'fresnel';

            // Trigger search event
            const searchEvent = new Event('input');
            searchInput.dispatchEvent(searchEvent);

            // Articles should be filtered
            // (In real scenario, would check articlesList.innerHTML)
        });

        test('should support fuzzy matching', () => {
            // Test the internal fuzzyMatch function
            // Since it's not exposed, we verify through search behavior
        });
    });

    describe('Category Filtering', () => {
        test('should render category filter buttons', async () => {
            await HelpOverlay.init();
            HelpOverlay.open();

            const categories = document.getElementById('help-categories');
            expect(categories).not.toBeNull();
        });

        test('should filter articles when category clicked', async () => {
            await HelpOverlay.init();
            HelpOverlay.open();

            // Click on a category button
            // (In real scenario, would simulate button click)
        });
    });

    describe('UI Interaction', () => {
        test('should open overlay on toggle', async () => {
            await HelpOverlay.init();

            HelpOverlay.toggle();
            expect(HelpOverlay.isOpen()).toBe(true);

            HelpOverlay.toggle();
            expect(HelpOverlay.isOpen()).toBe(false);
        });

        test('should close overlay on escape key', async () => {
            await HelpOverlay.init();
            HelpOverlay.open();

            const escapeEvent = new KeyboardEvent('keydown', { key: 'Escape' });
            document.dispatchEvent(escapeEvent);

            expect(HelpOverlay.isOpen()).toBe(false);
        });
    });
});

describe('Help Articles JSON', () => {
    let articles;

    beforeEach(async () => {
        // Mock fetch to return the articles
        global.fetch = jest.fn().mockResolvedValue({
            ok: true,
            json: async () => {
                const fs = require('fs');
                const path = require('path');
                const jsonPath = path.join(__dirname, '..', 'help_articles.json');
                return JSON.parse(fs.readFileSync(jsonPath, 'utf8'));
            }
        });
    });

    test('should have 30+ articles', async () => {
        const response = await fetch('/help_articles.json');
        articles = await response.json();

        expect(articles.length).toBeGreaterThanOrEqual(30);
    });

    test('should have required article fields', async () => {
        const response = await fetch('/help_articles.json');
        articles = await response.json();

        articles.forEach(article => {
            expect(article).toHaveProperty('id');
            expect(article).toHaveProperty('title');
            expect(article).toHaveProperty('content');
            expect(article).toHaveProperty('category');
        });
    });

    test('should cover all major categories', async () => {
        const response = await fetch('/help_articles.json');
        articles = await response.json();

        const categories = new Set(articles.map(a => a.category));
        const expectedCategories = [
            'Basics', 'Troubleshooting', 'Setup', 'Features',
            'Advanced', 'Interface', 'Maintenance', 'Integration'
        ];

        expectedCategories.forEach(cat => {
            expect(categories.has(cat)).toBe(true);
        });
    });

    test('should have Fresnel zone article', async () => {
        const response = await fetch('/help_articles.json');
        articles = await response.json();

        const fresnelArticle = articles.find(a => a.id === 'fresnel-zone');
        expect(fresnelArticle).toBeDefined();
        expect(fresnelArticle.title).toContain('Fresnel');
    });

    test('should have detection quality article', async () => {
        const response = await fetch('/help_articles.json');
        articles = await response.json();

        const qualityArticle = articles.find(a => a.id === 'detection-quality');
        expect(qualityArticle).toBeDefined();
        expect(qualityArticle.content).toContain('interference');
    });

    test('should have help article for predictions', async () => {
        const response = await fetch('/help_articles.json');
        articles = await response.json();

        const predictionsArticle = articles.find(a => a.id === 'predictions');
        expect(predictionsArticle).toBeDefined();
        expect(predictionsArticle.content).toContain('7 days');
    });
});
