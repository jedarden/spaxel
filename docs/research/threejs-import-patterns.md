# Three.js Import Patterns in Spaxel

**Search Date:** 2026-08-28  
**Scope:** Entire codebase (./spaxel)  
**Patterns searched:** `import.*three`, `from.*three`, `require.*three`

## Complete Search Results

### Files with Three.js References (12 files found)

1. `./dashboard/js/app.js`
2. `./dashboard/js/controls.js`
3. `./dashboard/js/fxaa.js` ✅ **IMPORTS FOUND**
4. `./dashboard/js/home-cards.js`
5. `./dashboard/js/mobile.test.js`
6. `./dashboard/js/portal.js`
7. `./dashboard/js/viz3d.js`
8. `./dashboard/js/volume-editor.js` ✅ **DYNAMIC LOADING FOUND**
9. `./dashboard/live.html` ✅ **SCRIPT TAGS FOUND**
10. `./dashboard/simulator.html` ✅ **SCRIPT TAGS FOUND**
11. `./dashboard/test-transformcontrols.html` ✅ **SCRIPT TAGS FOUND**
12. `./dashboard/types/blob-identity.check.ts`

---

## 1. ES6 Dynamic Imports (`import('three/...')`)

**File:** `dashboard/js/fxaa.js`

```javascript
Line 52:  const module = await import('three/examples/jsm/postprocessing/EffectComposer.js');
Line 55:  const renderPassModule = await import('three/examples/jsm/postprocessing/RenderPass.js');
Line 58:  const shaderPassModule = await import('three/examples/jsm/postprocessing/ShaderPass.js');
Line 61:  const fxaaShaderModule = await import('three/examples/jsm/shaders/FXAAShader.js');
```

**Pattern:** Dynamic ES6 imports from Three.js examples JSM modules  
**Use case:** Post-processing effects (FXAA anti-aliasing)

---

## 2. HTML Script Tags (`<script src="...">`)

### `dashboard/simulator.html` (Lines 188-190)

```html
<script src="https://cdnjs.cloudflare.com/ajax/libs/three.js/r128/three.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/controls/OrbitControls.js"></script>
<script src="https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/controls/TransformControls.js"></script>
```

### `dashboard/test-transformcontrols.html` (Lines 5-7)

```html
<script src="https://cdnjs.cloudflare.com/ajax/libs/three.js/r128/three.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/controls/OrbitControls.js"></script>
<script src="https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/controls/TransformControls.js"></script>
```

### `dashboard/live.html` (Lines 3521, 3523, 3525)

```html
<script src="https://cdnjs.cloudflare.com/ajax/libs/three.js/r128/three.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/controls/OrbitControls.js"></script>
<script src="https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/controls/TransformControls.js"></script>
```

**Pattern:** CDN-based script tags loading Three.js r128 and control modules  
**Version:** r128 (0.128.0)  
**Sources:** cdnjs.cloudflare.com and cdn.jsdelivr.net

---

## 3. Dynamic Script Loading

**File:** `dashboard/js/volume-editor.js` (Line 104)

```javascript
script.src = 'https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/controls/TransformControls.js';
```

**Pattern:** Runtime script injection for TransformControls  
**Use case:** On-demand loading of 3D transform controls

---

## 4. No Matches Found

### ES6 Static Imports (`from 'three'`)
- **No static ES6 imports found** (e.g., `import { Scene } from 'three'`)

### CommonJS Requires (`require('three')`)
- **No CommonJS require statements found**

---

## Summary

| Import Pattern | Count | Files |
|----------------|-------|-------|
| ES6 dynamic imports | 4 | `dashboard/js/fxaa.js` |
| HTML script tags | 9 | 3 HTML files |
| Dynamic script loading | 1 | `dashboard/js/volume-editor.js` |
| ES6 static imports | 0 | — |
| CommonJS requires | 0 | — |
| **TOTAL** | **14** | **5 files** |

## Key Observations

1. **Version consistency:** All imports use Three.js **r128 (0.128.0)**
2. **Loading strategy:** Mix of CDN script tags (HTML) and dynamic imports (JavaScript)
3. **No bundler:** No evidence of npm/Webpack/rollup - all imports are CDN-based
4. **Post-processing:** FXAA anti-aliasing is the only feature using ES6 modules
5. **Controls:** OrbitControls and TransformControls loaded in multiple locations

## Search Commands Executed

```bash
# ES6 imports with 'three'
grep -rn "import.*three" --include="*.js" --include="*.jsx" --include="*.ts" --include="*.tsx" .

# ES6 'from' imports with 'three'  
grep -rn "from.*three" --include="*.js" --include="*.jsx" --include="*.ts" --include="*.tsx" .

# CommonJS requires
grep -rn "require.*three" --include="*.js" --include="*.jsx" --include="*.ts" --include="*.tsx" .

# HTML script tags
grep -rn "<script.*three" --include="*.html" .

# Dynamic script loading
grep -rn "script.*three.*js" --include="*.js" .

# Files mentioning 'three'
find . -type f \( -name "*.js" -o -name "*.jsx" -o -name "*.ts" -o -name "*.tsx" -o -name "*.html" \) ! -path "*/node_modules/*" -exec grep -l "three" {} \;
```
