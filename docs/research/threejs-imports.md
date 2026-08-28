# Three.js Imports in Spaxel Codebase

**Search Date:** 2026-08-28  
**Scope:** /home/coding/spaxel codebase

## Summary

Found Three.js imports in **5 files** total across the dashboard directory:

### Files with Direct Three.js Imports

#### HTML Files (CDN Script Tags)

1. **`dashboard/live.html`** (lines 3521-3535)
   - Three.js r128 from cdnjs.cloudflare.com
   - OrbitControls from jsdelivr (0.128.0)
   - TransformControls from jsdelivr (0.128.0)
   - Import map for post-processing modules

2. **`dashboard/simulator.html`** (lines 188-190)
   - Three.js r128 from cdnjs.cloudflare.com
   - OrbitControls from jsdelivr (0.128.0)
   - TransformControls from jsdelivr (0.128.0)

3. **`dashboard/test-transformcontrols.html`** (lines 5-7)
   - Three.js r128 from cdnjs.cloudflare.com
   - OrbitControls from jsdelivr (0.128.0)
   - TransformControls from jsdelivr (0.128.0)

#### JavaScript Files (Dynamic Imports)

4. **`dashboard/js/fxaa.js`** (lines 52-62)
   - ES6 dynamic imports for Three.js post-processing modules:
     - EffectComposer
     - RenderPass
     - ShaderPass
     - FXAAShader

5. **`dashboard/js/volume-editor.js`** (line 104)
   - Dynamic script loading for TransformControls

## Primary Entry Points for Three.js Usage

Based on import patterns and architecture:

1. **`dashboard/live.html`** - Main live view 3D scene (primary entry point)
2. **`dashboard/js/viz3d.js`** - Core 3D visualization module (uses global THREE)
3. **`dashboard/js/fxaa.js`** - Post-processing anti-aliasing (ES6 module imports)

## Additional Files Using Three.js

These files use Three.js but rely on the global `THREE` object loaded by HTML files:

- `dashboard/js/app.js` - Application logic
- `dashboard/js/controls.js` - Camera controls
- `dashboard/js/portal.js` - Portal rendering
- `dashboard/js/home-cards.js` - UI components
- `dashboard/js/mobile.test.js` - Testing

## Import Patterns Summary

1. **CDN Script Tags** (most common): Loading Three.js r128 via cdnjs.cloudflare.com
2. **ES6 Dynamic Imports**: Used in `fxaa.js` for post-processing modules
3. **Dynamic Script Loading**: Used in `volume-editor.js` for TransformControls
4. **Import Maps**: Used in `live.html` for module resolution

## Version

All imports use **Three.js version 0.128.0** from CDN sources.
