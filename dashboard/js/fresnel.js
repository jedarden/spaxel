/**
 * Spaxel Dashboard - Fresnel Zone Helper Module
 *
 * Shared Fresnel zone ellipsoid geometry computation for:
 * - Debug overlay (all active links)
 * - Explainability overlay (contributing links for a specific blob)
 *
 * Provides FresnelEllipsoid() function that returns Three.js meshes
 * ready for scene insertion.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const CONFIG = {
        // WiFi wavelengths (c/f in meters)
        wavelength_5ghz: 0.06,      // 5 GHz
        wavelength_2_4ghz: 0.125,   // 2.4 GHz
        // Geometry
        sphereSegments: 32,         // Sphere geometry segments (reduced on mobile)
        sphereHeightSegments: 16,
        wireframeOpacity: 0.6,      // Line opacity for wireframe
        fillOpacity: 0.08,          // Fill opacity
        mobileViewportWidth: 768,    // Width threshold for mobile
        mobileSegments: 16,         // Reduced segments for mobile
        mobileHeightSegments: 8
    };

    // ============================================
    // Private State
    // ============================================
    let _scene = null;

    /**
     * Initialize the Fresnel module with the Three.js scene.
     * @param {THREE.Scene} scene - The Three.js scene
     */
    function init(scene) {
        _scene = scene;
    }

    /**
     * Get WiFi wavelength based on channel number.
     * @param {number} channel - WiFi channel (1-14 for 2.4 GHz, 36-165 for 5 GHz)
     * @returns {number} Wavelength in meters
     */
    function getWavelengthForChannel(channel) {
        if (!channel || channel < 1) return CONFIG.wavelength_2_4ghz;

        // 2.4 GHz channels: 1-14
        if (channel <= 14) {
            return CONFIG.wavelength_2_4ghz;
        }

        // 5 GHz channels: 36 and above
        return CONFIG.wavelength_5ghz;
    }

    /**
     * Calculate Fresnel zone ellipsoid parameters for a link.
     * Based on the first Fresnel zone geometry.
     *
     * For a link with TX at position P1 and RX at position P2:
     * - Link distance d = |P1 - P2|
     * - WiFi wavelength lambda: 5 GHz -> lambda = 0.06m, 2.4 GHz -> lambda = 0.125m
     * - Semi-major axis: a = (d + lambda/2) / 2
     * - Semi-minor axis: b = sqrt(a^2 - (d/2)^2)
     * - Ellipsoid centre: midpoint(P1, P2)
     * - Ellipsoid orientation: major axis along the P1->P2 unit vector
     *
     * @param {THREE.Vector3} tx - Transmitter position
     * @param {THREE.Vector3} rx - Receiver position
     * @param {number} channel - WiFi channel number (for wavelength)
     * @returns {Object} Ellipsoid parameters: { center, semiAxes, rotation, lambda, d, a, b }
     */
    function calculateFresnelEllipsoid(tx, rx, channel) {
        // Get wavelength based on channel
        const lambda = getWavelengthForChannel(channel);

        // Direct distance between TX and RX
        const d = tx.distanceTo(rx);

        // First Fresnel zone ellipsoid parameters
        // Semi-major axis: a = (d + lambda/2) / 2
        const a = (d + lambda / 2) / 2;

        // Semi-minor axis: b = sqrt(a^2 - (d/2)^2)
        // Using the property that for a prolate spheroid with foci at tx and rx:
        // b^2 = a^2 - (d/2)^2
        const b = Math.sqrt(Math.max(0, a * a - (d / 2) * (d / 2)));

        // Center of ellipsoid (midpoint between TX and RX)
        const center = new THREE.Vector3().addVectors(tx, rx).multiplyScalar(0.5);

        // Rotation: align with TX-RX axis
        // X-axis is the major axis of the ellipsoid (a), Y and Z are minor (b)
        const direction = new THREE.Vector3().subVectors(rx, tx).normalize();
        const xAxis = new THREE.Vector3(1, 0, 0);
        const quaternion = new THREE.Quaternion().setFromUnitVectors(xAxis, direction);

        return {
            center: center,
            semiAxes: new THREE.Vector3(a, b, b), // X (major along link), Y, Z (minor)
            rotation: quaternion,
            lambda: lambda,
            d: d,
            a: a,
            b: b,
            channel: channel
        };
    }

    /**
     * Create a Three.js Mesh for a Fresnel zone ellipsoid.
     * Creates both wireframe and fill meshes for proper visualization.
     *
     * @param {THREE.Vector3} tx - Transmitter position
     * @param {THREE.Vector3} rx - Receiver position
     * @param {number} channel - WiFi channel number
     * @param {number} color - Color hex value (e.g., 0x4FC3F7 for blue)
     * @param {Object} options - Optional settings { wireframeOpacity, fillOpacity }
     * @returns {Object} Object containing { wireframe, fill, data } meshes
     */
    function FresnelEllipsoid(tx, rx, channel, color, options) {
        if (!_scene) {
            console.warn('[Fresnel] Scene not initialized. Call Fresnel.init(scene) first.');
            return null;
        }

        options = options || {};
        const wireframeOpacity = options.wireframeOpacity !== undefined ? options.wireframeOpacity : CONFIG.wireframeOpacity;
        const fillOpacity = options.fillOpacity !== undefined ? options.fillOpacity : CONFIG.fillOpacity;

        // Calculate ellipsoid geometry
        const ellipsoid = calculateFresnelEllipsoid(tx, rx, channel);

        // Determine segment count based on viewport (mobile optimization)
        const isMobile = window.innerWidth < CONFIG.mobileViewportWidth;
        const segments = isMobile ? CONFIG.mobileSegments : CONFIG.sphereSegments;
        const heightSegments = isMobile ? CONFIG.mobileHeightSegments : CONFIG.sphereHeightSegments;

        // Create unit sphere geometry — will be scaled via mesh.scale
        const geometry = new THREE.SphereGeometry(1, segments, heightSegments);

        // Create wireframe using EdgesGeometry for crisp edges
        const edgesGeometry = new THREE.EdgesGeometry(geometry);
        const wireframeMaterial = new THREE.LineBasicMaterial({
            color: color,
            transparent: true,
            opacity: wireframeOpacity,
            depthTest: true,
            depthWrite: false
        });
        const wireframe = new THREE.LineSegments(edgesGeometry, wireframeMaterial);

        // Create fill mesh
        const fillMaterial = new THREE.MeshBasicMaterial({
            color: color,
            transparent: true,
            opacity: fillOpacity,
            depthWrite: false,
            side: THREE.DoubleSide
        });
        const fill = new THREE.Mesh(geometry, fillMaterial);

        // Apply non-uniform scaling: X = semi-major (a), Y = semi-minor (b), Z = semi-minor (b)
        wireframe.scale.copy(ellipsoid.semiAxes);
        fill.scale.copy(ellipsoid.semiAxes);

        // Position at ellipsoid center
        wireframe.position.copy(ellipsoid.center);
        fill.position.copy(ellipsoid.center);

        // Apply rotation to align with link axis
        wireframe.quaternion.copy(ellipsoid.rotation);
        fill.quaternion.copy(ellipsoid.rotation);

        // Store metadata for raycasting and interactions
        const data = {
            tx: tx.clone(),
            rx: rx.clone(),
            channel: channel,
            lambda: ellipsoid.lambda,
            d: ellipsoid.d,
            a: ellipsoid.a,
            b: ellipsoid.b,
            semiAxes: ellipsoid.semiAxes.clone(),
            center: ellipsoid.center.clone(),
            rotation: ellipsoid.rotation.clone()
        };

        wireframe.userData = { fresnelEllipsoid: data };
        fill.userData = { fresnelEllipsoid: data };

        return {
            wireframe: wireframe,
            fill: fill,
            data: data
        };
    }

    /**
     * Add a Fresnel ellipsoid to the scene.
     * Convenience function that creates and adds the mesh.
     *
     * @param {THREE.Vector3} tx - Transmitter position
     * @param {THREE.Vector3} rx - Receiver position
     * @param {number} channel - WiFi channel number
     * @param {number} color - Color hex value
     * @param {Object} options - Optional settings
     * @returns {Object} Object containing { wireframe, fill, data }
     */
    function addFresnelEllipsoid(tx, rx, channel, color, options) {
        const ellipsoid = FresnelEllipsoid(tx, rx, channel, color, options);
        if (!ellipsoid) return null;

        if (_scene) {
            _scene.add(ellipsoid.wireframe);
            _scene.add(ellipsoid.fill);
        }

        return ellipsoid;
    }

    /**
     * Remove a Fresnel ellipsoid from the scene.
     *
     * @param {Object} ellipsoid - Object returned from addFresnelEllipsoid or FresnelEllipsoid
     */
    function removeFresnelEllipsoid(ellipsoid) {
        if (!ellipsoid) return;

        if (ellipsoid.wireframe && _scene) {
            _scene.remove(ellipsoid.wireframe);
            ellipsoid.wireframe.geometry.dispose();
            if (ellipsoid.wireframe.material) {
                ellipsoid.wireframe.material.dispose();
            }
        }

        if (ellipsoid.fill && _scene) {
            _scene.remove(ellipsoid.fill);
            if (ellipsoid.fill.geometry) {
                ellipsoid.fill.geometry.dispose();
            }
            if (ellipsoid.fill.material) {
                ellipsoid.fill.material.dispose();
            }
        }
    }

    // ============================================
    // Public API
    // ============================================
    window.Fresnel = {
        init: init,
        calculateFresnelEllipsoid: calculateFresnelEllipsoid,
        FresnelEllipsoid: FresnelEllipsoid,
        addFresnelEllipsoid: addFresnelEllipsoid,
        removeFresnelEllipsoid: removeFresnelEllipsoid,
        // Configuration access
        CONFIG: CONFIG
    };

    console.log('[Fresnel] Module loaded');
})();
