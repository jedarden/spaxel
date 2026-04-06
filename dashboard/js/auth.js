/**
 * Spaxel Dashboard - Authentication Module
 *
 * Handles PIN setup, login, and session management for the dashboard.
 * Shows first-run setup page when PIN is not configured.
 */

(function() {
    'use strict';

    // ============================================
    // Auth State
    // ============================================
    const authState = {
        pinConfigured: null,
        isAuthenticated: false,
        isLoading: true,
        setupStep: 'enter', // 'enter' | 'confirm'
        enteredPin: '',
        loginError: '',
        setupError: ''
    };

    // ============================================
    // DOM Elements
    // ============================================
    let authOverlay = null;
    let setupOverlay = null;
    let loginOverlay = null;

    // ============================================
    // Auth API
    // ============================================

    /**
     * Check if PIN is configured
     */
    function checkAuthStatus() {
        authState.isLoading = true;
        renderOverlays();

        return fetch('/api/auth/status')
            .then(function(res) {
                if (!res.ok) {
                    throw new Error('Failed to check auth status: ' + res.status);
                }
                return res.json();
            })
            .then(function(data) {
                authState.pinConfigured = data.pin_configured;
                authState.isLoading = false;

                // If PIN is configured, check if we have a valid session
                if (authState.pinConfigured) {
                    return checkSession();
                } else {
                    // Show first-run setup
                    renderOverlays();
                }
            })
            .catch(function(err) {
                console.error('[Auth] Error checking auth status:', err);
                authState.isLoading = false;
                // On error, assume auth is required
                authState.pinConfigured = true;
                renderOverlays();
            });
    }

    /**
     * Check if current session is valid
     */
    function checkSession() {
        return fetch('/api/settings', { method: 'HEAD' })
            .then(function(res) {
                if (res.ok) {
                    // Session is valid
                    authState.isAuthenticated = true;
                    renderOverlays();
                } else {
                    // Session invalid or expired
                    authState.isAuthenticated = false;
                    renderOverlays();
                }
            })
            .catch(function(err) {
                console.error('[Auth] Error checking session:', err);
                authState.isAuthenticated = false;
                renderOverlays();
            });
    }

    /**
     * Setup PIN on first run
     * @param {string} pin - The PIN to set
     */
    function setupPIN(pin) {
        authState.setupError = '';
        renderOverlays();

        return fetch('/api/auth/setup', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ pin: pin })
        })
        .then(function(res) {
            if (!res.ok) {
                return res.text().then(function(text) {
                    throw new Error(text || 'Failed to setup PIN');
                });
            }
            return res.json();
        })
        .then(function(data) {
            // PIN setup successful, reload to start authenticated session
            window.location.reload();
        })
        .catch(function(err) {
            console.error('[Auth] Error setting up PIN:', err);
            authState.setupError = err.message || 'Failed to setup PIN';
            renderOverlays();
            throw err;
        });
    }

    /**
     * Login with PIN
     * @param {string} pin - The PIN to authenticate with
     */
    function login(pin) {
        authState.loginError = '';
        renderOverlays();

        return fetch('/api/auth/login', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ pin: pin })
        })
        .then(function(res) {
            if (!res.ok) {
                if (res.status === 401) {
                    throw new Error('Invalid PIN');
                }
                return res.text().then(function(text) {
                    throw new Error(text || 'Login failed');
                });
            }
            return res.json();
        })
        .then(function(data) {
            // Login successful, reload to start authenticated session
            window.location.reload();
        })
        .catch(function(err) {
            console.error('[Auth] Error logging in:', err);
            authState.loginError = err.message || 'Login failed';
            renderOverlays();
            throw err;
        });
    }

    /**
     * Logout and clear session
     */
    function logout() {
        return fetch('/api/auth/logout', {
            method: 'POST'
        })
        .then(function(res) {
            if (!res.ok) {
                throw new Error('Logout failed');
            }
            return res.json();
        })
        .then(function(data) {
            // Logout successful, reload to show login page
            window.location.reload();
        })
        .catch(function(err) {
            console.error('[Auth] Error logging out:', err);
            // Even if error, reload to clear local state
            window.location.reload();
        });
    }

    // ============================================
    // Overlay Rendering
    // ============================================

    function renderOverlays() {
        // Remove existing overlays
        if (authOverlay) {
            authOverlay.remove();
            authOverlay = null;
        }

        // If loading, show nothing
        if (authState.isLoading) {
            return;
        }

        // If PIN not configured, show first-run setup
        if (!authState.pinConfigured) {
            renderSetupOverlay();
            return;
        }

        // If PIN configured but not authenticated, show login
        if (authState.pinConfigured && !authState.isAuthenticated) {
            renderLoginOverlay();
            return;
        }
    }

    function renderSetupOverlay() {
        authOverlay = document.createElement('div');
        authOverlay.id = 'auth-overlay';
        authOverlay.innerHTML = `
            <div class="auth-modal">
                <div class="auth-header">
                    <h1>Welcome to Spaxel</h1>
                    <p>Let's secure your dashboard with a PIN</p>
                </div>
                <div class="auth-body">
                    ${authState.setupStep === 'enter' ? `
                        <p class="auth-instruction">Enter a 4-8 digit PIN to secure your dashboard:</p>
                        <div class="pin-inputs" id="setup-pin-inputs">
                            <input type="password" class="pin-digit" maxlength="1" data-index="0" autofocus>
                            <input type="password" class="pin-digit" maxlength="1" data-index="1">
                            <input type="password" class="pin-digit" maxlength="1" data-index="2">
                            <input type="password" class="pin-digit" maxlength="1" data-index="3">
                            <input type="password" class="pin-digit" maxlength="1" data-index="4">
                            <input type="password" class="pin-digit" maxlength="1" data-index="5">
                            <input type="password" class="pin-digit" maxlength="1" data-index="6">
                            <input type="password" class="pin-digit" maxlength="1" data-index="7">
                        </div>
                        <p class="auth-hint">Your PIN should be 4-8 digits</p>
                        ${authState.setupError ? `<p class="auth-error">${authState.setupError}</p>` : ''}
                        <button class="auth-button primary" id="setup-next-btn" disabled>Next</button>
                    ` : `
                        <p class="auth-instruction">Confirm your PIN by entering it again:</p>
                        <div class="pin-inputs" id="confirm-pin-inputs">
                            <input type="password" class="pin-digit" maxlength="1" data-index="0" autofocus>
                            <input type="password" class="pin-digit" maxlength="1" data-index="1">
                            <input type="password" class="pin-digit" maxlength="1" data-index="2">
                            <input type="password" class="pin-digit" maxlength="1" data-index="3">
                            <input type="password" class="pin-digit" maxlength="1" data-index="4">
                            <input type="password" class="pin-digit" maxlength="1" data-index="5">
                            <input type="password" class="pin-digit" maxlength="1" data-index="6">
                            <input type="password" class="pin-digit" maxlength="1" data-index="7">
                        </div>
                        ${authState.setupError ? `<p class="auth-error">${authState.setupError}</p>` : ''}
                        <button class="auth-button primary" id="setup-confirm-btn" disabled>Confirm & Setup</button>
                        <button class="auth-button secondary" id="setup-back-btn">Back</button>
                    `}
                </div>
            </div>
        `;

        document.body.appendChild(authOverlay);
        setupOverlayEvents();
    }

    function renderLoginOverlay() {
        authOverlay = document.createElement('div');
        authOverlay.id = 'auth-overlay';
        authOverlay.innerHTML = `
            <div class="auth-modal">
                <div class="auth-header">
                    <h1>Spaxel Dashboard</h1>
                    <p>Enter your PIN to continue</p>
                </div>
                <div class="auth-body">
                    <div class="pin-inputs" id="login-pin-inputs">
                        <input type="password" class="pin-digit" maxlength="1" data-index="0" autofocus>
                        <input type="password" class="pin-digit" maxlength="1" data-index="1">
                        <input type="password" class="pin-digit" maxlength="1" data-index="2">
                        <input type="password" class="pin-digit" maxlength="1" data-index="3">
                        <input type="password" class="pin-digit" maxlength="1" data-index="4">
                        <input type="password" class="pin-digit" maxlength="1" data-index="5">
                        <input type="password" class="pin-digit" maxlength="1" data-index="6">
                        <input type="password" class="pin-digit" maxlength="1" data-index="7">
                    </div>
                    ${authState.loginError ? `<p class="auth-error">${authState.loginError}</p>` : ''}
                    <button class="auth-button primary" id="login-btn" disabled>Login</button>
                </div>
            </div>
        `;

        document.body.appendChild(authOverlay);
        loginOverlayEvents();
    }

    // ============================================
    // Event Handlers
    // ============================================

    function setupOverlayEvents() {
        var inputs = authOverlay.querySelectorAll('.pin-digit');
        var nextBtn = document.getElementById('setup-next-btn');
        var confirmBtn = document.getElementById('setup-confirm-btn');
        var backBtn = document.getElementById('setup-back-btn');

        // Handle input focus and navigation
        inputs.forEach(function(input, index) {
            input.addEventListener('input', function(e) {
                var value = e.target.value;

                // Only allow digits
                if (!/^\d*$/.test(value)) {
                    e.target.value = '';
                    return;
                }

                // Move to next input if value entered
                if (value.length === 1 && index < inputs.length - 1) {
                    inputs[index + 1].focus();
                }

                // Enable/disable button based on input
                var pin = getPinFromInputs(inputs);
                if (authState.setupStep === 'enter') {
                    nextBtn.disabled = pin.length < 4;
                } else {
                    confirmBtn.disabled = pin.length < 4;
                }
            });

            // Handle backspace navigation
            input.addEventListener('keydown', function(e) {
                if (e.key === 'Backspace' && !e.target.value && index > 0) {
                    inputs[index - 1].focus();
                }
            });

            // Handle paste event
            input.addEventListener('paste', function(e) {
                e.preventDefault();
                var pastedData = (e.clipboardData || window.clipboardData).getData('text');
                var digits = pastedData.replace(/\D/g, '').slice(0, 8);

                for (var i = 0; i < digits.length && index + i < inputs.length; i++) {
                    inputs[index + i].value = digits[i];
                }

                // Focus the next empty input or the last one
                var nextIndex = Math.min(index + digits.length, inputs.length - 1);
                inputs[nextIndex].focus();

                // Trigger input event on last affected input
                inputs[nextIndex].dispatchEvent(new Event('input'));
            });
        });

        // Next button
        if (nextBtn) {
            nextBtn.addEventListener('click', function() {
                var inputs = document.querySelectorAll('#setup-pin-inputs .pin-digit');
                var pin = getPinFromInputs(inputs);
                if (pin.length >= 4) {
                    authState.enteredPin = pin;
                    authState.setupStep = 'confirm';
                    authState.setupError = '';
                    renderSetupOverlay();
                    // Focus first input of confirm step
                    setTimeout(function() {
                        var confirmInputs = document.querySelectorAll('#confirm-pin-inputs .pin-digit');
                        if (confirmInputs.length > 0) {
                            confirmInputs[0].focus();
                        }
                    }, 10);
                }
            });
        }

        // Confirm button
        if (confirmBtn) {
            confirmBtn.addEventListener('click', function() {
                var inputs = document.querySelectorAll('#confirm-pin-inputs .pin-digit');
                var confirmPin = getPinFromInputs(inputs);
                if (confirmPin.length >= 4) {
                    if (confirmPin === authState.enteredPin) {
                        // PINS match, proceed with setup
                        setupPIN(authState.enteredPin);
                    } else {
                        // PINS don't match
                        authState.setupError = 'PINs do not match. Please try again.';
                        authState.setupStep = 'enter';
                        authState.enteredPin = '';
                        renderSetupOverlay();
                    }
                }
            });
        }

        // Back button
        if (backBtn) {
            backBtn.addEventListener('click', function() {
                authState.setupStep = 'enter';
                authState.setupError = '';
                renderSetupOverlay();
            });
        }
    }

    function loginOverlayEvents() {
        var inputs = authOverlay.querySelectorAll('.pin-digit');
        var loginBtn = document.getElementById('login-btn');

        // Handle input focus and navigation
        inputs.forEach(function(input, index) {
            input.addEventListener('input', function(e) {
                var value = e.target.value;

                // Only allow digits
                if (!/^\d*$/.test(value)) {
                    e.target.value = '';
                    return;
                }

                // Move to next input if value entered
                if (value.length === 1 && index < inputs.length - 1) {
                    inputs[index + 1].focus();
                }

                // Enable/disable button based on input
                var pin = getPinFromInputs(inputs);
                loginBtn.disabled = pin.length < 4;
            });

            // Handle backspace navigation
            input.addEventListener('keydown', function(e) {
                if (e.key === 'Backspace' && !e.target.value && index > 0) {
                    inputs[index - 1].focus();
                }
            });

            // Handle paste event
            input.addEventListener('paste', function(e) {
                e.preventDefault();
                var pastedData = (e.clipboardData || window.clipboardData).getData('text');
                var digits = pastedData.replace(/\D/g, '').slice(0, 8);

                for (var i = 0; i < digits.length && index + i < inputs.length; i++) {
                    inputs[index + i].value = digits[i];
                }

                // Focus the next empty input or the last one
                var nextIndex = Math.min(index + digits.length, inputs.length - 1);
                inputs[nextIndex].focus();

                // Trigger input event on last affected input
                inputs[nextIndex].dispatchEvent(new Event('input'));
            });
        });

        // Login button
        loginBtn.addEventListener('click', function() {
            var pin = getPinFromInputs(inputs);
            if (pin.length >= 4) {
                login(pin);
            }
        });
    }

    function getPinFromInputs(inputs) {
        var pin = '';
        for (var i = 0; i < inputs.length; i++) {
            pin += inputs[i].value;
        }
        return pin;
    }

    // ============================================
    // Public API
    // ============================================

    window.SpaxelAuth = {
        init: function() {
            checkAuthStatus();
        },

        logout: function() {
            return logout();
        },

        isAuthenticated: function() {
            return authState.isAuthenticated;
        },

        refreshStatus: function() {
            return checkAuthStatus();
        }
    };

    // Auto-init on load
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function() {
            window.SpaxelAuth.init();
        });
    } else {
        window.SpaxelAuth.init();
    }

})();
