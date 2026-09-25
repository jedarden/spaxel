/**
 * Builds the self-hosted Agentation bootstrap bundle and its React vendor
 * chunk (spaxel-ba77654a).
 *
 * Agentation is a React component, not a self-mounting script: the workspace
 * recipe (~/.claude/rules/agentation.md) wants an import map plus
 * `<script type="module" src="/agentation.js">` on every entry point, with
 * the mount verified by `#agentation-root` existing after load. This script
 * produces exactly that shape without a CDN (the pages load in CI and on
 * LAN-isolated boxes, so esm.sh is not an option):
 *
 *   agentation.js                      — agentation + react + react-dom +
 *                                        createRoot bootstrap; imports the
 *                                        one bare specifier below, resolved
 *                                        by the import map each page declares
 *   static/vendor/react-jsx-runtime.js — react/jsx-runtime, self-contained
 *
 * One deliberate deviation from the recipe's four-entry esm.sh map (react,
 * react-dom, react-dom/client, react-jsx-runtime): react is inlined into
 * agentation.js rather than left bare. react-dom ships as lazy CommonJS
 * factories, and a require() inside a lazy factory cannot be hoisted to a
 * static ESM import of an external module — esbuild leaves it as a runtime
 * call, which throws "Dynamic require of react is not supported" in ESM
 * output. Inlining react keeps a single instance for the whole react-dom
 * tree. `react/jsx-runtime` stays bare so the import map remains
 * load-bearing: a page whose map is missing or wrong fails module
 * resolution, which the agentation-mount smoke test
 * (tests/agentation-mount.spec.js) detects — silently inlining everything
 * would defeat that signal. The vendor chunk inlines its own react copy,
 * which is safe: jsx runtime functions only create elements (no hooks), and
 * react element/fragment identity is Symbol.for-based, designed for
 * cross-copy use.
 *
 * Run:  cd dashboard && npm run build:agentation
 */

const esbuild = require('esbuild');
const path = require('path');

const jsxRuntime = require('react/jsx-runtime');

const browserBuild = {
  bundle: true,
  minify: true,
  format: 'esm',
  target: 'es2020',
  platform: 'browser',
  // React's production builds branch on process.env.NODE_ENV; without this
  // define the bundle crashes in the browser ("process is not defined").
  define: { 'process.env.NODE_ENV': '"production"' },
  logLevel: 'info',
};

// Virtual entry re-exporting every key of the installed react/jsx-runtime as
// real ESM named exports (react ships CommonJS only, and esbuild emits a CJS
// entry as `export default` alone, which dies on named imports).
function reexportJsxRuntime() {
  const names = Object.keys(jsxRuntime);
  if (names.length === 0) {
    throw new Error('react/jsx-runtime exported nothing — refusing to build an empty vendor chunk');
  }
  const lines = ['import * as m from "react/jsx-runtime";'];
  for (const name of names) {
    if (!/^[A-Za-z_$][A-Za-z0-9_$]*$/.test(name)) {
      throw Error(`react/jsx-runtime export ${JSON.stringify(name)} is not a valid identifier`);
    }
    lines.push(`export const ${name} = m[${JSON.stringify(name)}];`);
  }
  return lines.join('\n');
}

const bootstrapEntry = `
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { Agentation } from "agentation";

// The workspace recipe verifies the mount via #agentation-root. Pages do not
// declare it themselves; the bootstrap owns creating and hosting it.
let root = document.getElementById("agentation-root");
if (!root) {
  root = document.createElement("div");
  root.id = "agentation-root";
}
if (root.parentElement === null) {
  document.body.appendChild(root);
}

createRoot(root).render(createElement(Agentation, { appName: "Spaxel" }));
`;

async function main() {
  await esbuild.build({
    ...browserBuild,
    stdin: {
      contents: reexportJsxRuntime(),
      resolveDir: __dirname,
      sourcefile: 'react-jsx-runtime-vendor-entry.js',
    },
    outfile: path.join(__dirname, 'static/vendor/react-jsx-runtime.js'),
  });

  await esbuild.build({
    ...browserBuild,
    stdin: {
      contents: bootstrapEntry,
      resolveDir: __dirname,
      sourcefile: 'agentation-bootstrap.js',
    },
    external: ['react/jsx-runtime'],
    outfile: path.join(__dirname, 'agentation.js'),
  });

  console.log('✓ agentation.js, static/vendor/react-jsx-runtime.js');
}

main().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
