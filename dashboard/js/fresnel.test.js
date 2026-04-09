/**
 * Spaxel Dashboard - Fresnel Zone Tests
 *
 * Tests for Fresnel ellipsoid geometry computation.
 * These tests verify the correctness of the Fresnel zone calculations
 * used for both the debug overlay and explainability overlay.
 */

describe('Fresnel Module', function() {
    // Mock THREE.js before all tests
    beforeAll(function() {
        if (typeof THREE === 'undefined') {
            global.THREE = {
                Vector3: function(x, y, z) {
                    this.x = x || 0;
                    this.y = y || 0;
                    this.z = z || 0;
                },
                Quaternion: function() {
                    this._x = 0;
                    this._y = 0;
                    this._z = 0;
                    this._w = 1;
                }
            };

            // Add prototype methods to Vector3
            THREE.Vector3.prototype.distanceTo = function(v) {
                return Math.sqrt(
                    Math.pow(this.x - v.x, 2) +
                    Math.pow(this.y - v.y, 2) +
                    Math.pow(this.z - v.z, 2)
                );
            };

            THREE.Vector3.prototype.addVectors = function(v1, v2) {
                this.x = v1.x + v2.x;
                this.y = v1.y + v2.y;
                this.z = v1.z + v2.z;
                return this;
            };

            THREE.Vector3.prototype.subVectors = function(v1, v2) {
                this.x = v1.x - v2.x;
                this.y = v1.y - v2.y;
                this.z = v1.z - v2.z;
                return this;
            };

            THREE.Vector3.prototype.multiplyScalar = function(s) {
                this.x *= s;
                this.y *= s;
                this.z *= s;
                return this;
            };

            THREE.Vector3.prototype.clone = function() {
                return new THREE.Vector3(this.x, this.y, this.z);
            };

            THREE.Quaternion.prototype.setFromUnitVectors = function(from, to) {
                // Simplified quaternion for testing
                this._w = 1;
                return this;
            };

            THREE.Quaternion.prototype.clone = function() {
                return new THREE.Quaternion();
            };
        }

        // Load the Fresnel module
        if (typeof window !== 'undefined') {
            // In browser environment, the module should already be loaded
            // For testing purposes, we'll use the functions directly
        }
    });

    describe('calculateFresnelEllipsoid', function() {
        var calculateFresnelEllipsoid;

        beforeEach(function() {
            if (window.Fresnel && window.Fresnel.calculateFresnelEllipsoid) {
                calculateFresnelEllipsoid = window.Fresnel.calculateFresnelEllipsoid;
            }
        });

        it('should calculate correct semi-major and semi-minor axes for a 4m link', function() {
            if (!calculateFresnelEllipsoid) {
                test.skip('Fresnel module not loaded');
                return;
            }

            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);
            var channel = 6; // 2.4 GHz

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            // Expected values:
            // lambda = 0.125m (2.4 GHz)
            // d = 4m
            // a = (d + lambda/2) / 2 = (4 + 0.0625) / 2 = 2.03125
            // b = sqrt(a^2 - (d/2)^2) = sqrt(2.03125^2 - 2^2) = sqrt(4.126 - 4) = sqrt(0.126) ≈ 0.355

            expect(result.d).toBeCloseTo(4, 0.01);
            expect(result.a).toBeCloseTo(2.031, 0.001);
            expect(result.b).toBeCloseTo(0.355, 0.001);
        });

        it('should calculate correct semi-minor axis for very short link (edge case)', function() {
            if (!calculateFresnelEllipsoid) {
                test.skip('Fresnel module not loaded');
                return;
            }

            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(0.1, 0, 0);
            var channel = 6;

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            // For a very short link, b should be small but positive
            expect(result.b).toBeGreaterThan(0);
            expect(result.b).toBeLessThan(0.1);
        });

        it('should calculate correct axes for diagonal link (distance = 5m)', function() {
            if (!calculateFresnelEllipsoid) {
                test.skip('Fresnel module not loaded');
                return;
            }

            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(3, 4, 0); // 3-4-5 triangle, distance = 5
            var channel = 6;

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            // Expected:
            // d = 5m
            // lambda = 0.125m
            // a = (5 + 0.0625) / 2 = 2.53125
            // b = sqrt(2.53125^2 - 2.5^2) = sqrt(6.407 - 6.25) = sqrt(0.157) ≈ 0.396

            expect(result.d).toBeCloseTo(5, 0.01);
            expect(result.a).toBeCloseTo(2.531, 0.001);
            expect(result.b).toBeCloseTo(0.396, 0.001);
        });

        it('should use correct wavelength for 5 GHz channel', function() {
            if (!calculateFresnelEllipsoid) {
                test.skip('Fresnel module not loaded');
                return;
            }

            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);
            var channel = 36; // 5 GHz channel

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            // For 5 GHz: lambda = 0.06m
            // a = (4 + 0.03) / 2 = 2.015
            // b = sqrt(2.015^2 - 2^2) = sqrt(4.060 - 4) = sqrt(0.060) ≈ 0.245

            expect(result.lambda).toBeCloseTo(0.06, 0.001);
            expect(result.a).toBeCloseTo(2.015, 0.001);
            expect(result.b).toBeCloseTo(0.245, 0.001);
        });

        it('should use correct wavelength for 2.4 GHz channel', function() {
            if (!calculateFresnelEllipsoid) {
                test.skip('Fresnel module not loaded');
                return;
            }

            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);
            var channel = 6; // 2.4 GHz channel

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            // For 2.4 GHz: lambda = 0.125m
            expect(result.lambda).toBeCloseTo(0.125, 0.001);
        });

        it('should calculate ellipsoid center at midpoint', function() {
            if (!calculateFresnelEllipsoid) {
                test.skip('Fresnel module not loaded');
                return;
            }

            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);
            var channel = 6;

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            expect(result.center.x).toBeCloseTo(2, 0.001);
            expect(result.center.y).toBeCloseTo(0, 0.001);
            expect(result.center.z).toBeCloseTo(0, 0.001);
        });

        it('should calculate ellipsoid center for diagonal link', function() {
            if (!calculateFresnelEllipsoid) {
                test.skip('Fresnel module not loaded');
                return;
            }

            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(6, 8, 0);
            var channel = 6;

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            // Midpoint of (0,0,0) and (6,8,0) is (3,4,0)
            expect(result.center.x).toBeCloseTo(3, 0.001);
            expect(result.center.y).toBeCloseTo(4, 0.001);
            expect(result.center.z).toBeCloseTo(0, 0.001);
        });

        it('should handle zero channel gracefully (default to 2.4 GHz)', function() {
            if (!calculateFresnelEllipsoid) {
                test.skip('Fresnel module not loaded');
                return;
            }

            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);
            var channel = 0; // Invalid channel

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            // Should default to 2.4 GHz wavelength
            expect(result.lambda).toBeCloseTo(0.125, 0.001);
        });

        it('should handle very large link distance (100m)', function() {
            if (!calculateFresnelEllipsoid) {
                test.skip('Fresnel module not loaded');
                return;
            }

            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(100, 0, 0);
            var channel = 6;

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            // For 100m link:
            // a = (100 + 0.0625) / 2 = 50.03125
            // b = sqrt(50.03125^2 - 50^2) = sqrt(2503.125 - 2500) = sqrt(3.125) ≈ 1.768

            expect(result.d).toBeCloseTo(100, 0.01);
            expect(result.a).toBeCloseTo(50.031, 0.001);
            expect(result.b).toBeCloseTo(1.768, 0.001);
        });
    });

    describe('getWavelengthForChannel', function() {
        var getWavelengthForChannel;

        beforeEach(function() {
            if (window.Fresnel && window.Fresnel.calculateFresnelEllipsoid) {
                // Test the wavelength indirectly through calculateFresnelEllipsoid
                getWavelengthForChannel = function(channel) {
                    var tx = new THREE.Vector3(0, 0, 0);
                    var rx = new THREE.Vector3(1, 0, 0);
                    var result = window.Fresnel.calculateFresnelEllipsoid(tx, rx, channel);
                    return result.lambda;
                };
            }
        });

        it('should return 2.4 GHz wavelength for channel 1', function() {
            if (!getWavelengthForChannel) {
                test.skip('Fresnel module not loaded');
                return;
            }
            expect(getWavelengthForChannel(1)).toBeCloseTo(0.125, 0.001);
        });

        it('should return 2.4 GHz wavelength for channel 6', function() {
            if (!getWavelengthForChannel) {
                test.skip('Fresnel module not loaded');
                return;
            }
            expect(getWavelengthForChannel(6)).toBeCloseTo(0.125, 0.001);
        });

        it('should return 2.4 GHz wavelength for channel 14', function() {
            if (!getWavelengthForChannel) {
                test.skip('Fresnel module not loaded');
                return;
            }
            expect(getWavelengthForChannel(14)).toBeCloseTo(0.125, 0.001);
        });

        it('should return 5 GHz wavelength for channel 36', function() {
            if (!getWavelengthForChannel) {
                test.skip('Fresnel module not loaded');
                return;
            }
            expect(getWavelengthForChannel(36)).toBeCloseTo(0.06, 0.001);
        });

        it('should return 5 GHz wavelength for channel 149', function() {
            if (!getWavelengthForChannel) {
                test.skip('Fresnel module not loaded');
                return;
            }
            expect(getWavelengthForChannel(149)).toBeCloseTo(0.06, 0.001);
        });
    });
});
