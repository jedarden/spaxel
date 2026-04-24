#!/usr/bin/env python3
"""Replace hard-coded hex colors in CSS files with design system tokens."""
import re, sys, os

# Mapping: lowercase hex → token replacement
# Ordered longest-first so #ffffff matches before #fff
COLOR_MAP = {
    # Slate scale (exact Radix values from tokens.css)
    '#111113': 'var(--slate-1)',
    '#18191b': 'var(--slate-2)',
    '#1e2024': 'var(--slate-3)',
    '#262830': 'var(--slate-4)',
    '#2e3039': 'var(--slate-5)',
    '#363842': 'var(--slate-6)',
    '#43454f': 'var(--slate-7)',
    '#565867': 'var(--slate-8)',
    '#6b6e7b': 'var(--slate-9)',
    '#7c7f8c': 'var(--slate-10)',
    '#9b9daa': 'var(--slate-11)',
    '#ededf0': 'var(--slate-12)',

    # Backgrounds close to slate steps
    '#1e1e2e': 'var(--bg-card)',
    '#1e1e3a': 'var(--bg-card)',
    '#1a1a2e': 'var(--bg-card)',
    '#1a1a1a': 'var(--bg-page)',
    '#121225': 'var(--bg-page)',
    '#141425': 'var(--bg-page)',
    '#252538': 'var(--bg-hover)',
    '#2a2a2a': 'var(--bg-hover)',
    '#2a2a3a': 'var(--bg-hover)',
    '#2a2a4a': 'var(--bg-hover)',

    # Slate approximations (grays)
    '#333333': 'var(--slate-5)',
    '#333':    'var(--slate-5)',
    '#3a3a4a': 'var(--slate-5)',
    '#444444': 'var(--slate-6)',
    '#444':    'var(--slate-6)',
    '#555555': 'var(--slate-7)',
    '#555':    'var(--slate-7)',
    '#666666': 'var(--text-muted)',
    '#666':    'var(--text-muted)',
    '#888888': 'var(--text-muted)',
    '#888':    'var(--text-muted)',
    '#9e9e9e': 'var(--slate-10)',
    '#aaaaaa': 'var(--text-secondary)',
    '#aaa':    'var(--text-secondary)',
    '#bbbbbb': 'var(--slate-11)',
    '#bbbb':   'var(--slate-11)',
    '#cccccc': 'var(--slate-11)',
    '#ccc':    'var(--slate-11)',
    '#d0d0d0': 'var(--slate-11)',
    '#e0e0e0': 'var(--slate-11)',
    '#eeeeee': 'var(--slate-12)',
    '#eee':    'var(--slate-12)',
    '#ffffff': 'var(--text-on-accent)',
    '#fff':    'var(--text-on-accent)',
    '#000000': '#000000',  # keep pure black (rare, used for shadows)
    '#000':    '#000',

    # Blue scale (accent)
    '#4fc3f7': 'var(--blue-10)',
    '#45b7d1': 'var(--blue-9)',
    '#3aa6c3': 'var(--blue-8)',
    '#4ecdc4': 'var(--blue-9)',
    '#4a9eff': 'var(--blue-9)',
    '#3b82f6': 'var(--blue-9)',
    '#29b6f6': 'var(--blue-10)',
    '#68bdfa': 'var(--blue-11)',
    '#aedcff': 'var(--blue-12)',
    '#1a73e8': 'var(--blue-9)',
    '#2196f3': 'var(--blue-9)',
    '#42a5f5': 'var(--blue-10)',

    # Semantic: red/alert
    '#e5484d': 'var(--alert)',
    '#ef5350': 'var(--alert)',
    '#dc3545': 'var(--alert)',
    '#dc2626': 'var(--alert)',
    '#f44336': 'var(--alert)',
    '#ff4757': 'var(--alert)',
    '#c82333': 'var(--alert)',
    '#b91c1c': 'var(--alert)',
    '#f87171': 'var(--alert)',
    '#e74c3c': 'var(--alert)',
    '#ff6b6b': 'var(--alert)',

    # Semantic: green/ok
    '#46a758': 'var(--ok)',
    '#66bb6a': 'var(--ok)',
    '#4caf50': 'var(--ok)',
    '#81c784': 'var(--ok)',
    '#22c55e': 'var(--ok)',
    '#22c65e': 'var(--ok)',
    '#4ade80': 'var(--ok)',
    '#66bb6a': 'var(--ok)',
    '#2e7d32': 'var(--ok)',
    '#43a047': 'var(--ok)',
    '#1b5e20': 'var(--ok)',

    # Semantic: amber/warn
    '#e5a00d': 'var(--warn)',
    '#ffa726': 'var(--warn)',
    '#ffa502': 'var(--warn)',
    '#ffc107': 'var(--warn)',
    '#fbbf24': 'var(--warn)',
    '#ff9800': 'var(--warn)',
    '#ffb300': 'var(--warn)',
    '#f57c00': 'var(--warn)',
    '#ffca28': 'var(--warn)',

    # Purple (replace with blue accent per design system)
    '#ab47bc': 'var(--blue-9)',
    '#ba68c8': 'var(--blue-10)',
    '#9c27b0': 'var(--blue-8)',
    '#a78bfa': 'var(--blue-10)',
    '#9c4cb3': 'var(--blue-9)',

    # Slate for Tailwind-like grays
    '#475569': 'var(--slate-7)',
    '#64748b': 'var(--slate-9)',

    # Gradient endpoints (apdetection.css)
    '#667eea': 'var(--blue-7)',
    '#764ba2': 'var(--blue-9)',

    # Additional blues
    '#3a8ee6': 'var(--blue-9)',
    '#2563eb': 'var(--blue-9)',
    '#16a34a': 'var(--ok)',
    '#7db4ff': 'var(--blue-11)',
    '#a5ccff': 'var(--blue-12)',
    '#4299e1': 'var(--blue-9)',
    '#0066cc': 'var(--blue-9)',
    '#007aff': 'var(--blue-9)',
    '#00bcd4': 'var(--blue-9)',
    '#3498db': 'var(--blue-9)',
    '#3579c5': 'var(--blue-8)',

    # Additional greens
    '#34c759': 'var(--ok)',
    '#2ecc71': 'var(--ok)',
    '#1abc9c': 'var(--ok)',

    # Additional ambers/oranges
    '#fb923c': 'var(--warn)',
    '#ed8936': 'var(--warn)',
    '#ff9500': 'var(--warn)',
    '#ffb432': 'var(--warn)',
    '#e6c84c': 'var(--warn)',
    '#f39c12': 'var(--warn)',
    '#e67e22': 'var(--warn)',
    '#feedba': 'var(--warn)',

    # Additional reds
    '#ff3b30': 'var(--alert)',
    '#f66':    'var(--alert)',
    '#d32f2f': 'var(--alert)',
    '#e57373': 'var(--alert)',

    # Purples → blue accent (dark-only design system)
    '#a55eea': 'var(--blue-10)',
    '#7e57c2': 'var(--blue-9)',
    '#9b59b6': 'var(--blue-9)',
    '#af52de': 'var(--blue-10)',

    # Slate/gray approximations
    '#747d8c': 'var(--slate-9)',
    '#607d8b': 'var(--slate-9)',
    '#95a5a6': 'var(--slate-10)',
    '#34495e': 'var(--slate-6)',
    '#a0a0b0': 'var(--text-secondary)',
    '#16162a': 'var(--bg-card)',
    '#2ed573': 'var(--ok)',
    '#ddd':    'var(--text-secondary)',
    '#0a0a0a': 'var(--slate-1)',
    '#6c6':    'var(--ok)',
    '#f66':    'var(--alert)',
    '#98989d': 'var(--text-secondary)',
    '#86868b': 'var(--text-secondary)',
    '#1d1d1f': 'var(--text-primary)',
    '#38383a': 'var(--border-default)',
    '#2c2c2e': 'var(--bg-card)',
    '#1c1c1e': 'var(--bg-page)',
    '#e5e5ea': 'var(--border-default)',
    '#e5e5e7': 'var(--border-default)',
    '#ebebeb': 'var(--border-default)',
    '#f5f5f7': 'var(--bg-page)',

    # Ambient time-of-day (light colors → dark equivalents)
    '#f0f4f8': 'var(--bg-page)',
    '#fef3e7': 'var(--bg-page)',
    '#1a365d': 'var(--text-primary)',
    '#7c2d12': 'var(--warn)',

    # Exact Radix blue scale values used as non-token references
    '#11181e': 'var(--blue-1)',
    '#152233': 'var(--blue-2)',
    '#1a2e47': 'var(--blue-3)',
    '#1e3a5f': 'var(--blue-4)',
    '#224777': 'var(--blue-5)',
    '#26558f': 'var(--blue-6)',
    '#2e6aad': 'var(--blue-7)',
    '#3e96e8': 'var(--blue-9)',
    '#4daaf5': 'var(--blue-10)',
    '#16213e': 'var(--blue-3)',
}

def replace_hex_in_css(content, filepath):
    """Replace hex colors with tokens, preserving context."""
    lines = content.split('\n')
    result = []
    changes = 0

    for line in lines:
        new_line = line
        # Find all hex colors in the line
        # Match #xxx or #xxxxxx (3 or 6 hex digits)
        hex_pattern = re.compile(r'#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})\b')

        matches = hex_pattern.finditer(line)
        for m in reversed(list(matches)):  # reversed to preserve positions
            hex_val = m.group(0).lower()
            if hex_val in COLOR_MAP:
                replacement = COLOR_MAP[hex_val]
                if replacement != m.group(0):  # skip if mapping is identity
                    new_line = new_line[:m.start()] + replacement + new_line[m.end():]
                    changes += 1

        result.append(new_line)

    return '\n'.join(result), changes

def process_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    new_content, changes = replace_hex_in_css(content, filepath)

    if changes > 0:
        with open(filepath, 'w') as f:
            f.write(new_content)
        print(f"  {os.path.basename(filepath)}: {changes} replacements")
    else:
        print(f"  {os.path.basename(filepath)}: no changes needed")

    return changes

if __name__ == '__main__':
    css_dir = os.path.dirname(os.path.abspath(__file__))
    total = 0

    for fname in sorted(os.listdir(css_dir)):
        if not fname.endswith('.css') or fname == 'tokens.css' or fname.startswith('_'):
            continue
        fpath = os.path.join(css_dir, fname)
        total += process_file(fpath)

    print(f"\nTotal replacements: {total}")
