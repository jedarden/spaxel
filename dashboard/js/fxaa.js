/**
 * Spaxel Dashboard - FXAA Post-Processing Module
 *
 * Fast Approximate Anti-Aliasing for mobile devices.
 * Uses Three.js EffectComposer with FXAA shader pass.
 * Only active on screens < 1024px width (mobile devices).
 */

(function() {
    'use strict';

    // ============================================
    // State
    // ============================================
    let composer = null;
    let fxaaPass = null;
    let renderPass = null;
    let scene = null;
    let camera = null;
    let renderer = null;
    let isInitialized = false;
    let isActive = false;

    /**
     * Detect if the current device is a mobile device based on screen width.
     * @returns {boolean} True if screen width < 1024px (mobile)
     */
    function isMobile() {
        return window.innerWidth < 1024;
    }

    /**
     * Initialize the FXAA composer.
     * @param {THREE.Scene} sceneRef - The Three.js scene
     * @param {THREE.Camera} cameraRef - The Three.js camera
     * @param {THREE.WebGLRenderer} rendererRef - The Three.js renderer
     */
    async function init(sceneRef, cameraRef, rendererRef) {
        scene = sceneRef;
        camera = cameraRef;
        renderer = rendererRef;

        // Only initialize FXAA on mobile devices
        if (!isMobile()) {
            console.log('[FXAA] Desktop detected, skipping FXAA initialization (MSAA will be used)');
            isActive = false;
            return;
        }

        // Dynamic import of post-processing modules
        try {
            const module = await import('three/examples/jsm/postprocessing/EffectComposer.js');
            const EffectComposer = module.EffectComposer;

            const renderPassModule = await import('three/examples/jsm/postprocessing/RenderPass.js');
            const RenderPass = renderPassModule.RenderPass;

            const shaderPassModule = await import('three/examples/jsm/postprocessing/ShaderPass.js');
            const ShaderPass = shaderPassModule.ShaderPass;

            const fxaaShaderModule = await import('three/examples/jsm/shaders/FXAAShader.js');
            const FXAAShader = fxaaShaderModule.FXAAShader;

            // Create render pass
            renderPass = new RenderPass(scene, camera);

            // Create FXAA pass
            fxaaPass = new ShaderPass(FXAAShader);

            // Set FXAA resolution (must be updated on resize)
            const width = window.innerWidth * window.devicePixelRatio;
            const height = window.innerHeight * window.devicePixelRatio;
            fxaaPass.uniforms['resolution'].value.x = 1 / width;
            fxaaPass.uniforms['resolution'].value.y = 1 / height;

            // Create composer
            composer = new EffectComposer(renderer);
            composer.addPass(renderPass);
            composer.addPass(fxaaPass);

            isInitialized = true;
            isActive = true;

            console.log('[FXAA] Initialized for mobile device');
        } catch (error) {
            console.error('[FXAA] Failed to initialize:', error);
            isActive = false;
        }
    }

    /**
     * Render the scene using FXAA composer (if active) or direct renderer (if not).
     */
    function render() {
        if (isActive && composer && isInitialized) {
            composer.render();
        }
    }

    /**
     * Update the FXAA resolution after window resize.
     */
    function updateResolution() {
        if (!isActive || !fxaaPass || !isInitialized) {
            return;
        }

        const width = window.innerWidth * window.devicePixelRatio;
        const height = window.innerHeight * window.devicePixelRatio;
        fxaaPass.uniforms['resolution'].value.x = 1 / width;
        fxaaPass.uniforms['resolution'].value.y = 1 / height;

        // Also update composer size
        if (composer) {
            composer.setSize(window.innerWidth, window.innerHeight);
        }
    }

    /**
     * Check if FXAA is currently active.
     * @returns {boolean} True if FXAA is active
     */
    function getIsActive() {
        return isActive;
    }

    /**
     * Clean up resources.
     */
    function dispose() {
        if (composer) {
            composer.dispose();
            composer = null;
        }
        if (renderPass) {
            renderPass.dispose();
            renderPass = null;
        }
        if (fxaaPass) {
            fxaaPass.dispose();
            fxaaPass = null;
        }
        isInitialized = false;
        isActive = false;
        console.log('[FXAA] Disposed');
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelFXAA = {
        init: init,
        render: render,
        updateResolution: updateResolution,
        isActive: getIsActive,
        dispose: dispose
    };

    console.log('[FXAA] Module loaded');
})();
