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
                },
                SphereGeometry: function() {},
                EdgesGeometry: function() {},
                LineBasicMaterial: function() {},
                LineSegments: function() {
                    this.scale = { copy: function() {} };
                    this.position = { copy: function() {} };
                    this.quaternion = { copy: function() {} };
                    this.userData = {};
                    this.geometry = { dispose: function() {} };
                    this.material = { dispose: function() {} };
                },
                MeshBasicMaterial: function() {},
                Mesh: function() {
                    this.scale = { copy: function() {} };
                    this.position = { copy: function() {} };
                    this.quaternion = { copy: function() {} };
                    this.userData = {};
                    this.geometry = { dispose: function() {} };
                    this.material = { dispose: function() {} };
                },
                DoubleSide: 2
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

            THREE.Vector3.prototype.normalize = function() {
                var len = Math.sqrt(this.x * this.x + this.y * this.y + this.z * this.z);
                if (len > 0) { this.x /= len; this.y /= len; this.z /= len; }
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

            THREE.Vector3.prototype.copy = function(v) {
                this.x = v.x;
                this.y = v.y;
                this.z = v.z;
                return this;
            };

            THREE.Quaternion.prototype.setFromUnitVectors = function(from, to) {
                this._w = 1;
                return this;
            };

            THREE.Quaternion.prototype.clone = function() {
                return new THREE.Quaternion();
            };

            THREE.Quaternion.prototype.copy = function(q) {
                this._w = q._w;
                return this;
            };
        }

        // Load the Fresnel module if not already loaded
        if (typeof window.Fresnel === 'undefined') {
            require('./fresnel.js');
        }
    });

    describe('calculateFresnelEllipsoid', function() {
        var calculateFresnelEllipsoid;

        beforeAll(function() {
            calculateFresnelEllipsoid = window.Fresnel.calculateFresnelEllipsoid;
        });

        it('should calculate correct axes for 4m link at 5 GHz (spec test case)', function() {
            // TX at (0,0,0), RX at (4,0,0), lambda=0.06m (channel 36 = 5 GHz)
            // d = 4, a = (4 + 0.03) / 2 = 2.015
            // b = sqrt(2.015^2 - 2^2) = sqrt(0.060225) = 0.2454
            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);
            var channel = 36;

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            expect(result.lambda).toBeCloseTo(0.06, 3);
            expect(result.d).toBeCloseTo(4, 3);
            expect(result.a).toBeCloseTo(2.015, 3);
            expect(result.b).toBeCloseTo(0.245, 3);
        });

        it('should calculate correct semi-minor axis for very short link (edge case)', function() {
            // d=0.1m -> b should be very small but positive
            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(0.1, 0, 0);
            var channel = 6;

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            expect(result.b).toBeGreaterThan(0);
            expect(result.b).toBeLessThan(0.1);
        });

        it('should calculate correct axes for diagonal link (3-4-5 triangle)', function() {
            // TX at (0,0,0), RX at (3,4,0), distance = 5m, 2.4 GHz
            // d = 5, lambda = 0.125
            // a = (5 + 0.0625) / 2 = 2.53125
            // b = sqrt(2.53125^2 - 2.5^2) = sqrt(0.1572...) = 0.3965
            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(3, 4, 0);
            var channel = 6;

            var result = calculateFresnelEllipsoid(tx, rx, channel);

            expect(result.d).toBeCloseTo(5, 3);
            expect(result.a).toBeCloseTo(2.531, 3);
            expect(result.b).toBeCloseTo(0.397, 3);
        });

        it('should use correct wavelength for 5 GHz channel', function() {
            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);

            var result = calculateFresnelEllipsoid(tx, rx, 36);
            expect(result.lambda).toBeCloseTo(0.06, 3);
        });

        it('should use correct wavelength for 2.4 GHz channel', function() {
            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);

            var result = calculateFresnelEllipsoid(tx, rx, 6);
            expect(result.lambda).toBeCloseTo(0.125, 3);
        });

        it('should default to 2.4 GHz for invalid channel', function() {
            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);

            var result = calculateFresnelEllipsoid(tx, rx, 0);
            expect(result.lambda).toBeCloseTo(0.125, 3);
        });

        it('should calculate ellipsoid center at midpoint', function() {
            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);

            var result = calculateFresnelEllipsoid(tx, rx, 6);

            expect(result.center.x).toBeCloseTo(2, 3);
            expect(result.center.y).toBeCloseTo(0, 3);
            expect(result.center.z).toBeCloseTo(0, 3);
        });

        it('should calculate ellipsoid center for diagonal link', function() {
            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(6, 8, 0);

            var result = calculateFresnelEllipsoid(tx, rx, 6);

            expect(result.center.x).toBeCloseTo(3, 3);
            expect(result.center.y).toBeCloseTo(4, 3);
            expect(result.center.z).toBeCloseTo(0, 3);
        });

        it('should use X-axis as major axis in semiAxes', function() {
            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);

            var result = calculateFresnelEllipsoid(tx, rx, 6);

            // X should be semi-major (a), Y and Z should be semi-minor (b)
            expect(result.semiAxes.x).toBeCloseTo(result.a, 6);
            expect(result.semiAxes.y).toBeCloseTo(result.b, 6);
            expect(result.semiAxes.z).toBeCloseTo(result.b, 6);
        });
    });

    describe('toggle layer on/off', function() {
        it('should add correct number of mesh objects when toggled on', function() {
            // Create a mock scene that tracks added/removed children
            var addedObjects = [];
            var mockScene = {
                add: function(obj) { addedObjects.push(obj); },
                remove: function() {}
            };

            window.Fresnel.init(mockScene);

            // Simulate 3 links
            var ellipsoids = [];
            var links = [
                { tx: new THREE.Vector3(0, 0, 0), rx: new THREE.Vector3(4, 0, 0) },
                { tx: new THREE.Vector3(0, 0, 0), rx: new THREE.Vector3(0, 3, 0) },
                { tx: new THREE.Vector3(1, 1, 0), rx: new THREE.Vector3(5, 4, 0) }
            ];

            links.forEach(function(link) {
                var e = window.Fresnel.addFresnelEllipsoid(link.tx, link.rx, 6, 0x4FC3F7);
                if (e) ellipsoids.push(e);
            });

            // Each ellipsoid adds 2 objects (wireframe + fill)
            expect(addedObjects.length).toBe(6);

            // Clean up
            ellipsoids.forEach(function(e) { window.Fresnel.removeFresnelEllipsoid(e); });
        });

        it('should remove all mesh objects when toggled off', function() {
            var removedObjects = [];
            var mockScene = {
                add: function() {},
                remove: function(obj) { removedObjects.push(obj); }
            };

            window.Fresnel.init(mockScene);

            var e1 = window.Fresnel.addFresnelEllipsoid(
                new THREE.Vector3(0, 0, 0),
                new THREE.Vector3(4, 0, 0),
                6, 0x4FC3F7
            );

            // Remove should dispose of wireframe + fill
            window.Fresnel.removeFresnelEllipsoid(e1);
            expect(removedObjects.length).toBe(2);
        });

        it('should not leave orphan objects when clearing all ellipsoids', function() {
            var sceneChildren = [];
            var mockScene = {
                add: function(obj) { sceneChildren.push(obj); },
                remove: function(obj) {
                    var idx = sceneChildren.indexOf(obj);
                    if (idx !== -1) sceneChildren.splice(idx, 1);
                }
            };

            window.Fresnel.init(mockScene);

            // Add 3 ellipsoids (6 objects)
            var ellipsoids = [];
            for (var i = 0; i < 3; i++) {
                var e = window.Fresnel.addFresnelEllipsoid(
                    new THREE.Vector3(i, 0, 0),
                    new THREE.Vector3(i + 4, 0, 0),
                    6, 0x4FC3F7
                );
                if (e) ellipsoids.push(e);
            }

            expect(sceneChildren.length).toBe(6);

            // Remove all
            ellipsoids.forEach(function(e) { window.Fresnel.removeFresnelEllipsoid(e); });

            expect(sceneChildren.length).toBe(0);
        });
    });

    describe('hover tooltip data', function() {
        it('should include correct link metadata in ellipsoid data', function() {
            var mockScene = { add: function() {}, remove: function() {} };
            window.Fresnel.init(mockScene);

            var tx = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);
            var channel = 6;

            var ellipsoid = window.Fresnel.addFresnelEllipsoid(tx, rx, channel, 0x66bb6a);
            expect(ellipsoid).not.toBeNull();

            var data = ellipsoid.data;
            expect(data.d).toBeCloseTo(4, 3);
            expect(data.b).toBeGreaterThan(0);
            expect(data.lambda).toBeCloseTo(0.125, 3);
            expect(data.channel).toBe(6);

            // Clean up
            window.Fresnel.removeFresnelEllipsoid(ellipsoid);
        });

        it('should store tx and rx positions in ellipsoid data', function() {
            var mockScene = { add: function() {}, remove: function() {} };
            window.Fresnel.init(mockScene);

            var tx = new THREE.Vector3(1, 2, 3);
            var rx = new THREE.Vector3(4, 5, 6);

            var ellipsoid = window.Fresnel.addFresnelEllipsoid(tx, rx, 6, 0x66bb6a);
            expect(ellipsoid).not.toBeNull();

            var data = ellipsoid.data;
            expect(data.tx.x).toBeCloseTo(1, 3);
            expect(data.tx.y).toBeCloseTo(2, 3);
            expect(data.rx.x).toBeCloseTo(4, 3);
            expect(data.rx.y).toBeCloseTo(5, 3);

            // Clean up
            window.Fresnel.removeFresnelEllipsoid(ellipsoid);
        });
    });

    describe('node position update', function() {
        it('should compute different ellipsoids when node position changes', function() {
            var calc = window.Fresnel.calculateFresnelEllipsoid;

            // Original position
            var tx1 = new THREE.Vector3(0, 0, 0);
            var rx = new THREE.Vector3(4, 0, 0);
            var result1 = calc(tx1, rx, 6);

            // Move TX node to (1, 0, 0)
            var tx2 = new THREE.Vector3(1, 0, 0);
            var result2 = calc(tx2, rx, 6);

            // Distance changed from 4 to 3, so ellipsoid should differ
            expect(result1.d).toBeCloseTo(4, 3);
            expect(result2.d).toBeCloseTo(3, 3);
            expect(result1.a).not.toBeCloseTo(result2.a, 3);
            expect(result1.b).not.toBeCloseTo(result2.b, 3);
        });

        it('should produce larger semi-minor axis for shorter links', function() {
            var calc = window.Fresnel.calculateFresnelEllipsoid;

            var tx = new THREE.Vector3(0, 0, 0);
            var rx1 = new THREE.Vector3(1, 0, 0);  // short link
            var rx2 = new THREE.Vector3(10, 0, 0); // long link

            var result1 = calc(tx, rx1, 6);
            var result2 = calc(tx, rx2, 6);

            // Shorter link has relatively larger Fresnel zone
            var ratio1 = result1.b / result1.d;
            var ratio2 = result2.b / result2.d;
            expect(ratio1).toBeGreaterThan(ratio2);
        });
    });
});
