/**
 * Spaxel Dashboard - Panel Framework
 *
 * Provides slide-in sidebar, modal overlay, toast notifications,
 * and a panel registry for opening panels by name.
 */

(function() {
    'use strict';

    // ============================================
    // Panel Registry
    // ============================================
    const registeredPanels = new Map();

    /**
     * Register a panel constructor/creator function
     * @param {string} name - Panel identifier
     * @param {Function|Object} creator - Function that returns panel config, or panel config object
     */
    function registerPanel(name, creator) {
        registeredPanels.set(name, creator);
    }

    /**
     * Open a panel by name
     * @param {string} name - Panel identifier
     * @param {Object} options - Panel options (title, content, onOpen, onClose, etc.)
     */
    function openPanel(name, options) {
        const creator = registeredPanels.get(name);
        if (!creator) {
            console.error('[Panels] Unknown panel:', name);
            return null;
        }

        let config;
        if (typeof creator === 'function') {
            config = creator(options);
        } else {
            config = { ...creator, ...options };
        }

        return openSidebar(config);
    }

    /**
     * Open a modal dialog
     * @param {Object} options - Modal options (title, content, width, onConfirm, onCancel, etc.)
     */
    function openModal(options) {
        return createModal(options);
    }

    // ============================================
    // Sidebar Panel
    // ============================================
    let currentSidebar = null;
    let sidebarElement = null;
    let sidebarOverlay = null;

    const defaultSidebarOptions = {
        title: 'Panel',
        content: '',
        width: '360px',
        position: 'right', // 'right' or 'left'
        closeOnEscape: true,
        closeOnOverlayClick: true,
        onOpen: null,
        onClose: null,
        className: ''
    };

    /**
     * Open a slide-in sidebar panel
     * @param {Object} options - Sidebar options
     * @returns {Object} Panel control object with close() method
     */
    function openSidebar(options) {
        // Close existing sidebar first
        if (currentSidebar) {
            closeSidebar();
        }

        const config = { ...defaultSidebarOptions, ...options };
        const position = config.position;

        // Create overlay
        sidebarOverlay = document.createElement('div');
        sidebarOverlay.className = 'panel-overlay';
        sidebarOverlay.addEventListener('click', function(e) {
            if (config.closeOnOverlayClick && e.target === sidebarOverlay) {
                closeSidebar();
            }
        });

        // Create sidebar panel
        sidebarElement = document.createElement('div');
        sidebarElement.className = 'panel-sidebar panel-sidebar-' + position + ' ' + config.className;
        sidebarElement.style.width = config.width;

        // Sidebar header
        const header = document.createElement('div');
        header.className = 'panel-header';

        const title = document.createElement('h2');
        title.className = 'panel-title';
        title.textContent = config.title;

        const closeButton = document.createElement('button');
        closeButton.className = 'panel-close';
        closeButton.innerHTML = '&times;';
        closeButton.setAttribute('aria-label', 'Close');
        closeButton.addEventListener('click', closeSidebar);

        header.appendChild(title);
        header.appendChild(closeButton);

        // Sidebar content
        const content = document.createElement('div');
        content.className = 'panel-content';

        if (typeof config.content === 'string') {
            content.innerHTML = config.content;
        } else if (config.content instanceof HTMLElement) {
            content.appendChild(config.content);
        } else if (typeof config.content === 'function') {
            const result = config.content(content);
            if (result instanceof HTMLElement) {
                content.innerHTML = '';
                content.appendChild(result);
            }
        }

        // Assemble sidebar
        sidebarElement.appendChild(header);
        sidebarElement.appendChild(content);

        // Add touch event listeners to prevent propagation to canvas
        // This prevents OrbitControls from responding to touches on the panel
        sidebarElement.addEventListener('touchstart', function(e) {
            e.stopPropagation();
        }, { passive: true });

        sidebarElement.addEventListener('touchmove', function(e) {
            e.stopPropagation();
        }, { passive: false }); // Non-passive to allow preventDefault if needed

        sidebarElement.addEventListener('touchend', function(e) {
            e.stopPropagation();
        }, { passive: true });

        // Also add to overlay to prevent canvas touches through backdrop
        sidebarOverlay.addEventListener('touchstart', function(e) {
            e.stopPropagation();
        }, { passive: true });

        sidebarOverlay.addEventListener('touchmove', function(e) {
            e.stopPropagation();
        }, { passive: false });

        sidebarOverlay.addEventListener('touchend', function(e) {
            e.stopPropagation();
        }, { passive: true });

        // Add to DOM
        document.body.appendChild(sidebarOverlay);
        document.body.appendChild(sidebarElement);

        // Trigger animation
        requestAnimationFrame(function() {
            sidebarOverlay.classList.add('panel-overlay-visible');
            sidebarElement.classList.add('panel-sidebar-visible');
        });

        // Store panel control
        currentSidebar = {
            close: closeSidebar,
            element: sidebarElement,
            config: config
        };

        // Call onOpen callback
        if (config.onOpen) {
            config.onOpen(content, currentSidebar);
        }

        // Handle escape key
        if (config.closeOnEscape) {
            document.addEventListener('keydown', handleEscape);
        }

        return currentSidebar;
    }

    /**
     * Close the current sidebar
     */
    function closeSidebar() {
        if (!currentSidebar) return;

        const config = currentSidebar.config;

        // Trigger close animation
        if (sidebarOverlay) {
            sidebarOverlay.classList.remove('panel-overlay-visible');
        }
        if (sidebarElement) {
            sidebarElement.classList.remove('panel-sidebar-visible');
        }

        // Remove from DOM after animation
        setTimeout(function() {
            if (sidebarOverlay && sidebarOverlay.parentNode) {
                sidebarOverlay.parentNode.removeChild(sidebarOverlay);
            }
            if (sidebarElement && sidebarElement.parentNode) {
                sidebarElement.parentNode.removeChild(sidebarElement);
            }

            sidebarOverlay = null;
            sidebarElement = null;
        }, 300);

        // Call onClose callback
        if (config.onClose) {
            config.onClose();
        }

        // Remove escape handler
        document.removeEventListener('keydown', handleEscape);

        currentSidebar = null;
    }

    function handleEscape(e) {
        if (e.key === 'Escape' && currentSidebar) {
            closeSidebar();
        }
    }

    /**
     * Check if a sidebar is currently open
     */
    function isSidebarOpen() {
        return currentSidebar !== null;
    }

    // ============================================
    // Modal Overlay
    // ============================================
    let currentModal = null;
    let modalElement = null;
    let modalBackdrop = null;

    const defaultModalOptions = {
        title: '',
        content: '',
        width: '600px',
        maxWidth: '90vw',
        closeOnEscape: true,
        closeOnBackdropClick: true,
        showConfirm: false,
        showCancel: false,
        confirmText: 'OK',
        cancelText: 'Cancel',
        onOpen: null,
        onClose: null,
        onConfirm: null,
        onCancel: null,
        className: ''
    };

    /**
     * Open a modal dialog
     * @param {Object} options - Modal options
     * @returns {Object} Modal control object with close() method
     */
    function createModal(options) {
        // Close existing modal first
        if (currentModal) {
            closeModal();
        }

        const config = { ...defaultModalOptions, ...options };

        // Create backdrop
        modalBackdrop = document.createElement('div');
        modalBackdrop.className = 'modal-backdrop';
        modalBackdrop.addEventListener('click', function(e) {
            if (config.closeOnBackdropClick && e.target === modalBackdrop) {
                closeModal();
            }
        });

        // Create modal container
        modalElement = document.createElement('div');
        modalElement.className = 'modal-container ' + config.className;
        modalElement.style.width = config.width;
        modalElement.style.maxWidth = config.maxWidth;

        // Modal header (if title provided)
        if (config.title) {
            const header = document.createElement('div');
            header.className = 'modal-header';

            const title = document.createElement('h3');
            title.className = 'modal-title';
            title.textContent = config.title;

            const closeButton = document.createElement('button');
            closeButton.className = 'modal-close';
            closeButton.innerHTML = '&times;';
            closeButton.setAttribute('aria-label', 'Close');
            closeButton.addEventListener('click', closeModal);

            header.appendChild(title);
            header.appendChild(closeButton);
            modalElement.appendChild(header);
        }

        // Modal content
        const content = document.createElement('div');
        content.className = 'modal-content';

        if (typeof config.content === 'string') {
            content.innerHTML = config.content;
        } else if (config.content instanceof HTMLElement) {
            content.appendChild(config.content);
        } else if (typeof config.content === 'function') {
            const result = config.content(content);
            if (result instanceof HTMLElement) {
                content.innerHTML = '';
                content.appendChild(result);
            }
        }

        modalElement.appendChild(content);

        // Modal footer (if buttons requested)
        if (config.showConfirm || config.showCancel) {
            const footer = document.createElement('div');
            footer.className = 'modal-footer';

            if (config.showCancel) {
                const cancelButton = document.createElement('button');
                cancelButton.className = 'modal-btn modal-btn-cancel';
                cancelButton.textContent = config.cancelText;
                cancelButton.addEventListener('click', function() {
                    if (config.onCancel) {
                        config.onCancel();
                    }
                    closeModal();
                });
                footer.appendChild(cancelButton);
            }

            if (config.showConfirm) {
                const confirmButton = document.createElement('button');
                confirmButton.className = 'modal-btn modal-btn-confirm';
                confirmButton.textContent = config.confirmText;
                confirmButton.addEventListener('click', function() {
                    if (config.onConfirm) {
                        const result = config.onConfirm();
                        // If onConfirm returns false, don't close modal
                        if (result === false) return;
                    }
                    closeModal();
                });
                footer.appendChild(confirmButton);
            }

            modalElement.appendChild(footer);
        }

        // Add to DOM
        modalBackdrop.appendChild(modalElement);
        document.body.appendChild(modalBackdrop);

        // Trigger animation
        requestAnimationFrame(function() {
            modalBackdrop.classList.add('modal-backdrop-visible');
            modalElement.classList.add('modal-container-visible');
        });

        // Store modal control
        currentModal = {
            close: closeModal,
            element: modalElement,
            backdrop: modalBackdrop,
            config: config
        };

        // Call onOpen callback
        if (config.onOpen) {
            config.onOpen(content, currentModal);
        }

        // Handle escape key
        if (config.closeOnEscape) {
            document.addEventListener('keydown', handleModalEscape);
        }

        return currentModal;
    }

    /**
     * Close the current modal
     */
    function closeModal() {
        if (!currentModal) return;

        const config = currentModal.config;

        // Trigger close animation
        if (modalBackdrop) {
            modalBackdrop.classList.remove('modal-backdrop-visible');
        }
        if (modalElement) {
            modalElement.classList.remove('modal-container-visible');
        }

        // Remove from DOM after animation
        setTimeout(function() {
            if (modalBackdrop && modalBackdrop.parentNode) {
                modalBackdrop.parentNode.removeChild(modalBackdrop);
            }

            modalBackdrop = null;
            modalElement = null;
        }, 300);

        // Call onClose callback
        if (config.onClose) {
            config.onClose();
        }

        // Remove escape handler
        document.removeEventListener('keydown', handleModalEscape);

        currentModal = null;
    }

    function handleModalEscape(e) {
        if (e.key === 'Escape' && currentModal) {
            closeModal();
        }
    }

    /**
     * Check if a modal is currently open
     */
    function isModalOpen() {
        return currentModal !== null;
    }

    // ============================================
    // Toast Notifications
    // ============================================
    const toastContainer = document.getElementById('toast-container');

    if (!toastContainer) {
        console.error('[Panels] Toast container element not found');
    }

    /**
     * Show a toast notification
     * @param {string} message - Toast message
     * @param {Object} options - Toast options (type, duration, icon, etc.)
     * @returns {Object} Toast control object with dismiss() method
     */
    function showToast(message, options) {
        if (!toastContainer) return null;

        const config = {
            type: 'info', // 'success', 'info', 'warning', 'error'
            duration: 5000,
            icon: null,
            dismissible: true,
            ...options
        };

        const toast = document.createElement('div');
        toast.className = 'toast toast-' + config.type;

        // Icon
        let iconHtml = '';
        if (config.icon) {
            iconHtml = '<span class="toast-icon">' + config.icon + '</span>';
        } else {
            // Default icons by type
            const defaultIcons = {
                success: '&#x2713;',
                info: '&#x2139;',
                warning: '&#x26A0;',
                error: '&#x2717;'
            };
            iconHtml = '<span class="toast-icon">' + (defaultIcons[config.type] || defaultIcons.info) + '</span>';
        }

        // Dismiss button
        let dismissHtml = '';
        if (config.dismissible) {
            dismissHtml = '<button class="toast-dismiss" aria-label="Dismiss">&times;</button>';
        }

        toast.innerHTML = iconHtml + '<span class="toast-message">' + escapeHtml(message) + '</span>' + dismissHtml;

        toastContainer.appendChild(toast);

        // Trigger animation
        requestAnimationFrame(function() {
            toast.classList.add('toast-visible');
        });

        // Auto-dismiss after duration
        let dismissTimer = null;
        if (config.duration > 0) {
            dismissTimer = setTimeout(function() {
                dismissToast(toast);
            }, config.duration);
        }

        // Handle dismiss button
        const dismissBtn = toast.querySelector('.toast-dismiss');
        if (dismissBtn) {
            dismissBtn.addEventListener('click', function() {
                dismissToast(toast);
            });
        }

        // Create toast control object
        const toastControl = {
            element: toast,
            dismiss: function() { dismissToast(toast); }
        };

        return toastControl;
    }

    /**
     * Dismiss a toast notification
     * @param {HTMLElement} toast - Toast element to dismiss
     */
    function dismissToast(toast) {
        if (!toast || !toast.parentNode) return;

        toast.classList.remove('toast-visible');
        toast.classList.add('toast-dismissed');

        setTimeout(function() {
            if (toast.parentNode) {
                toast.parentNode.removeChild(toast);
            }
        }, 300);
    }

    /**
     * Show success toast
     */
    function showSuccess(message, options) {
        return showToast(message, { ...options, type: 'success' });
    }

    /**
     * Show info toast
     */
    function showInfo(message, options) {
        return showToast(message, { ...options, type: 'info' });
    }

    /**
     * Show warning toast
     */
    function showWarning(message, options) {
        return showToast(message, { ...options, type: 'warning' });
    }

    /**
     * Show error toast
     */
    function showError(message, options) {
        return showToast(message, { ...options, type: 'error' });
    }

    // ============================================
    // Utility Functions
    // ============================================
    function escapeHtml(text) {
        if (!text) return '';
        return String(text)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelPanels = {
        // Panel registry
        register: registerPanel,
        open: openPanel,

        // Direct panel opening
        openSidebar: openSidebar,
        closeSidebar: closeSidebar,
        isSidebarOpen: isSidebarOpen,

        // Modal
        openModal: openModal,
        closeModal: closeModal,
        isModalOpen: isModalOpen,

        // Toasts
        showToast: showToast,
        showSuccess: showSuccess,
        showInfo: showInfo,
        showWarning: showWarning,
        showError: showError,

        // Helper to create content element from HTML string
        createContent: function(html) {
            const wrapper = document.createElement('div');
            wrapper.innerHTML = html;
            return wrapper;
        }
    };

    console.log('[Panels] Panel framework initialized');
})();
