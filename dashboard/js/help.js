/**
 * Spaxel Dashboard - Contextual Help System
 *
 * Provides a searchable help overlay with fuzzy search for 30+ help articles.
 * Each article has a title, explanation, and link to relevant dashboard section.
 */

(function() {
    'use strict';

    // ===== State =====
    let helpArticles = [];
    let helpOverlayVisible = false;
    let searchQuery = '';

    // ===== DOM Elements =====
    let helpOverlay = null;
    let searchInput = null;
    let articlesList = null;

    /**
     * Initialize the help system
     */
    async function init() {
        // Load articles from JSON file
        await loadArticles();

        // Create help overlay
        createHelpOverlay();

        // Add help button to expert mode header
        addHelpButton();

        // Add keyboard shortcut (Ctrl/Cmd + ?)
        document.addEventListener('keydown', handleKeyboardShortcut);

        console.log('[Help] Module initialized with', helpArticles.length, 'articles');
    }

    /**
     * Load help articles from JSON file
     */
    async function loadArticles() {
        try {
            const response = await fetch('help_articles.json');
            if (!response.ok) {
                throw new Error('Failed to load help articles');
            }
            helpArticles = await response.json();
        } catch (err) {
            console.error('[Help] Failed to load articles:', err);
            // Use fallback articles
            helpArticles = getFallbackArticles();
        }
    }

    /**
     * Get fallback articles if JSON file fails to load
     */
    function getFallbackArticles() {
        return [
            {
                id: 'sensing-link',
                title: 'What is a sensing link?',
                content: 'A sensing link is the path between two spaxel nodes (one transmitting, one receiving). Motion in the space between them changes the WiFi signal, which spaxel detects.',
                category: 'Basics',
                action: null
            },
            {
                id: 'detection-quality',
                title: 'Why is my detection quality low?',
                content: 'Low quality usually means interference from other WiFi devices, an obstacle in the sensing zone, or the node is too far from your router. Click "Diagnose" on the link to find the specific cause.',
                category: 'Troubleshooting',
                action: { label: 'View Fleet Status', url: '#/fleet' }
            }
        ];
    }

    /**
     * Create the help overlay
     */
    function createHelpOverlay() {
        helpOverlay = document.createElement('div');
        helpOverlay.id = 'help-overlay';
        helpOverlay.className = 'help-overlay';
        helpOverlay.style.display = 'none';

        helpOverlay.innerHTML = `
            <div class="help-container">
                <div class="help-header">
                    <h2>Help & Documentation</h2>
                    <button class="help-close-btn" onclick="HelpOverlay.close()">&times;</button>
                </div>

                <div class="help-search">
                    <input type="text" id="help-search-input" placeholder="Search for help... (e.g., "fresnel", "detection")" />
                    <div class="search-icon">🔍</div>
                </div>

                <div class="help-categories" id="help-categories">
                    <!-- Category filters will be inserted here -->
                </div>

                <div class="help-articles" id="help-articles">
                    <!-- Articles will be rendered here -->
                </div>
            </div>
        `;

        document.body.appendChild(helpOverlay);

        // Get references to key elements
        searchInput = document.getElementById('help-search-input');
        articlesList = document.getElementById('help-articles');

        // Add search listener
        searchInput.addEventListener('input', handleSearch);
    }

    /**
     * Add help button to expert mode header
     */
    function addHelpButton() {
        // Check if button already exists
        if (document.getElementById('help-button')) {
            return;
        }

        // Look for the status bar button container (margin-left:auto; gap:6px)
        const buttonContainer = document.querySelector('#status-bar > div[style*="margin-left:auto"]');
        if (!buttonContainer) {
            // Try again after a delay
            setTimeout(addHelpButton, 500);
            return;
        }

        const helpBtn = document.createElement('button');
        helpBtn.id = 'help-button';
        helpBtn.className = 'help-button';
        helpBtn.innerHTML = '?';
        helpBtn.title = 'Help & Documentation (Ctrl+?)';
        helpBtn.onclick = () => HelpOverlay.open();

        // Insert before the last status-item (FPS counter)
        const fpsItem = buttonContainer.parentElement.querySelector('.status-item:last-child');
        if (fpsItem) {
            buttonContainer.insertBefore(helpBtn, fpsItem);
        } else {
            buttonContainer.appendChild(helpBtn);
        }
    }

    /**
     * Handle keyboard shortcut (Ctrl+? or Cmd+?)
     */
    function handleKeyboardShortcut(e) {
        if ((e.ctrlKey || e.metaKey) && e.key === '?') {
            e.preventDefault();
            HelpOverlay.toggle();
        } else if (e.key === 'Escape' && helpOverlayVisible) {
            HelpOverlay.close();
        }
    }

    /**
     * Handle search input
     */
    function handleSearch(e) {
        searchQuery = e.target.value.toLowerCase().trim();
        renderArticles();
    }

    /**
     * Render articles list based on search query
     */
    function renderArticles() {
        if (!articlesList) {
            return;
        }

        // Filter articles based on search query
        const filtered = helpArticles.filter(article => {
            if (!searchQuery) {
                return true;
            }

            const searchText = `${article.title} ${article.content} ${article.category}`.toLowerCase();
            return searchText.includes(searchQuery) || fuzzyMatch(searchQuery, searchText);
        });

        // Render filtered articles
        if (filtered.length === 0) {
            articlesList.innerHTML = `
                <div class="help-no-results">
                    <p>No articles found for "${searchQuery}"</p>
                    <p class="help-no-results-hint">Try different keywords or browse categories below.</p>
                </div>
            `;
        } else {
            articlesList.innerHTML = filtered.map(article => renderArticle(article)).join('');
        }
    }

    /**
     * Fuzzy match helper
     */
    function fuzzyMatch(query, text) {
        if (query.length < 3) {
            return false;
        }

        // Simple character-by-character fuzzy matching
        let queryIdx = 0;
        let textIdx = 0;
        let matches = 0;

        while (queryIdx < query.length && textIdx < text.length) {
            if (query[queryIdx] === text[textIdx]) {
                matches++;
                queryIdx++;
            }
            textIdx++;
        }

        // Require at least 80% of query characters to match
        return matches >= query.length * 0.8;
    }

    /**
     * Render a single article
     */
    function renderArticle(article) {
        let actionHTML = '';
        if (article.action) {
            actionHTML = `
                <button class="help-article-action" data-url="${article.action.url}">
                    ${article.action.label} →
                </button>
            `;
        }

        return `
            <div class="help-article" data-category="${article.category}">
                <div class="article-header">
                    <span class="article-category">${article.category}</span>
                    <h3 class="article-title">${article.title}</h3>
                </div>
                <div class="article-content">
                    ${article.content}
                </div>
                ${actionHTML ? `<div class="article-actions">${actionHTML}</div>` : ''}
            </div>
        `;
    }

    /**
     * Open the help overlay
     */
    function open() {
        if (!helpOverlay) {
            createHelpOverlay();
        }

        helpOverlay.style.display = 'flex';
        helpOverlayVisible = true;

        // Focus search input
        if (searchInput) {
            searchInput.focus();
        }

        // Render initial articles
        renderArticles();
        renderCategories();

        // Add action button listeners
        attachActionListeners();
    }

    /**
     * Close the help overlay
     */
    function close() {
        if (helpOverlay) {
            helpOverlay.style.display = 'none';
        }
        helpOverlayVisible = false;
    }

    /**
     * Toggle the help overlay
     */
    function toggle() {
        if (helpOverlayVisible) {
            close();
        } else {
            open();
        }
    }

    /**
     * Render category filter buttons
     */
    function renderCategories() {
        const categories = [...new Set(helpArticles.map(a => a.category))].sort();
        const categoriesContainer = document.getElementById('help-categories');

        if (!categoriesContainer) {
            return;
        }

        categoriesContainer.innerHTML = `
            <button class="category-filter active" data-category="all">All (${helpArticles.length})</button>
            ${categories.map(cat => {
                const count = helpArticles.filter(a => a.category === cat).length;
                return `<button class="category-filter" data-category="${cat}">${cat} (${count})</button>`;
            }).join('')}
        `;

        // Add category filter listeners
        categoriesContainer.querySelectorAll('.category-filter').forEach(btn => {
            btn.addEventListener('click', (e) => {
                // Update active state
                categoriesContainer.querySelectorAll('.category-filter').forEach(b => b.classList.remove('active'));
                e.target.classList.add('active');

                // Filter articles
                const category = e.target.dataset.category;
                filterByCategory(category);
            });
        });
    }

    /**
     * Filter articles by category
     */
    function filterByCategory(category) {
        if (!articlesList) {
            return;
        }

        const filtered = category === 'all'
            ? helpArticles
            : helpArticles.filter(a => a.category === category);

        articlesList.innerHTML = filtered.map(article => renderArticle(article)).join('');
        attachActionListeners();
    }

    /**
     * Attach listeners to action buttons
     */
    function attachActionListeners() {
        document.querySelectorAll('.help-article-action').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const url = e.target.dataset.url;
                if (url) {
                    // Navigate to the URL
                    if (url.startsWith('#')) {
                        // Internal navigation
                        if (window.SpaxelRouter) {
                            SpaxelRouter.navigate(url.substring(2));
                        } else {
                            window.location.hash = url;
                        }
                        close();
                    } else {
                        // External link
                        window.open(url, '_blank');
                    }
                }
            });
        });
    }

    /**
     * Show help for a specific topic (opens overlay and filters)
     */
    function showHelp(topic) {
        open();

        // Set search query
        if (searchInput) {
            searchInput.value = topic;
            searchQuery = topic.toLowerCase().trim();
            renderArticles();
        }
    }

    // ===== Public API =====
    window.HelpOverlay = {
        init: init,
        open: open,
        close: close,
        toggle: toggle,
        showHelp: showHelp,
        isOpen: () => helpOverlayVisible
    };

    // ===== Add Styles =====
    const style = document.createElement('style');
    style.id = 'help-overlay-styles';
    style.textContent = `
        .help-overlay {
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0, 0, 0, 0.9);
            z-index: 1000;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }

        .help-container {
            background: #1e1e3a;
            border-radius: 12px;
            width: 700px;
            max-width: 100%;
            max-height: 80vh;
            display: flex;
            flex-direction: column;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
        }

        .help-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 20px 24px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }

        .help-header h2 {
            margin: 0;
            font-size: 20px;
            color: #eee;
        }

        .help-close-btn {
            background: none;
            border: none;
            color: #888;
            font-size: 28px;
            cursor: pointer;
            padding: 4px 8px;
            line-height: 1;
        }

        .help-close-btn:hover {
            color: #fff;
        }

        .help-search {
            position: relative;
            padding: 16px 24px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }

        .help-search input {
            width: 100%;
            padding: 12px 16px 12px 40px;
            background: rgba(255, 255, 255, 0.08);
            border: 1px solid rgba(255, 255, 255, 0.15);
            border-radius: 8px;
            color: #eee;
            font-size: 14px;
            box-sizing: border-box;
        }

        .help-search input:focus {
            outline: none;
            border-color: #4fc3f7;
            box-shadow: 0 0 0 2px rgba(79, 195, 247, 0.2);
        }

        .search-icon {
            position: absolute;
            left: 36px;
            top: 50%;
            transform: translateY(-50%);
            font-size: 16px;
            opacity: 0.5;
        }

        .help-categories {
            display: flex;
            flex-wrap: wrap;
            gap: 8px;
            padding: 12px 24px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }

        .category-filter {
            padding: 6px 12px;
            background: rgba(255, 255, 255, 0.05);
            border: 1px solid rgba(255, 255, 255, 0.1);
            border-radius: 20px;
            color: #aaa;
            font-size: 12px;
            cursor: pointer;
            transition: all 0.2s;
        }

        .category-filter:hover {
            background: rgba(255, 255, 255, 0.1);
            color: #ddd;
        }

        .category-filter.active {
            background: #4fc3f7;
            color: #1a1a2e;
            border-color: #4fc3f7;
        }

        .help-articles {
            flex: 1;
            overflow-y: auto;
            padding: 16px 24px;
        }

        .help-articles::-webkit-scrollbar {
            width: 8px;
        }

        .help-articles::-webkit-scrollbar-track {
            background: rgba(255, 255, 255, 0.05);
        }

        .help-articles::-webkit-scrollbar-thumb {
            background: rgba(255, 255, 255, 0.2);
            border-radius: 4px;
        }

        .help-article {
            padding: 16px;
            margin-bottom: 12px;
            background: rgba(255, 255, 255, 0.03);
            border: 1px solid rgba(255, 255, 255, 0.08);
            border-radius: 8px;
            transition: background 0.2s;
        }

        .help-article:hover {
            background: rgba(255, 255, 255, 0.05);
        }

        .article-header {
            display: flex;
            align-items: center;
            gap: 10px;
            margin-bottom: 10px;
        }

        .article-category {
            padding: 2px 8px;
            background: rgba(79, 195, 247, 0.2);
            color: #4fc3f7;
            font-size: 10px;
            text-transform: uppercase;
            border-radius: 4px;
            font-weight: 600;
        }

        .article-title {
            margin: 0;
            font-size: 15px;
            color: #eee;
        }

        .article-content {
            font-size: 13px;
            color: #bbb;
            line-height: 1.5;
        }

        .article-content p {
            margin: 0 0 8px 0;
        }

        .article-content p:last-child {
            margin-bottom: 0;
        }

        .article-actions {
            margin-top: 12px;
            padding-top: 12px;
            border-top: 1px solid rgba(255, 255, 255, 0.1);
        }

        .help-article-action {
            padding: 6px 12px;
            background: #4fc3f7;
            color: #1a1a2e;
            border: none;
            border-radius: 4px;
            font-size: 12px;
            cursor: pointer;
            font-weight: 500;
        }

        .help-article-action:hover {
            background: #29b6f6;
        }

        .help-no-results {
            text-align: center;
            padding: 40px 20px;
            color: #888;
        }

        .help-no-results p {
            margin: 0 0 8px 0;
        }

        .help-no-results-hint {
            font-size: 12px;
        }

        .help-button {
            width: 32px;
            height: 32px;
            border-radius: 50%;
            background: rgba(255, 255, 255, 0.1);
            border: 1px solid rgba(255, 255, 255, 0.2);
            color: #888;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s;
            display: flex;
            align-items: center;
            justify-content: center;
            margin-left: auto;
        }

        .help-button:hover {
            background: rgba(79, 195, 247, 0.2);
            border-color: #4fc3f7;
            color: #4fc3f7;
        }

        /* Mobile responsive */
        @media (max-width: 768px) {
            .help-container {
                width: 100%;
                height: 100%;
                max-height: none;
                border-radius: 0;
            }

            .help-header {
                padding: 16px;
            }

            .help-header h2 {
                font-size: 18px;
            }

            .help-search,
            .help-categories,
            .help-articles {
                padding-left: 16px;
                padding-right: 16px;
            }
        }
    `;

    document.head.appendChild(style);

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
