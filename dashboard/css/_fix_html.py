#!/usr/bin/env python3
"""Replace hard-coded CSS values in HTML inline styles with design system tokens."""
import re, sys, os

# Same color map as _tokenize.py but for HTML files
COLOR_MAP = {
    # Backgrounds
    '#1a1a2e': 'var(--bg-page)',
    '#1e1e3a': 'var(--bg-card)',
    '#1e1e2e': 'var(--bg-card)',
    '#121225': 'var(--bg-page)',
    '#18191b': 'var(--bg-page)',

    # Text colors
    '#eee': 'var(--text-primary)',
    '#eeeeee': 'var(--text-primary)',
    '#ccc': 'var(--text-secondary)',
    '#cccccc': 'var(--text-secondary)',
    '#e0e0e0': 'var(--text-primary)',
    '#aaa': 'var(--text-secondary)',
    '#888': 'var(--text-muted)',
    '#888888': 'var(--text-muted)',
    '#666': 'var(--text-muted)',
    '#666666': 'var(--text-muted)',
    '#555': 'var(--text-muted)',
    '#555555': 'var(--text-muted)',
    '#fff': 'var(--text-on-accent)',
    '#ffffff': 'var(--text-on-accent)',

    # Blues
    '#4fc3f7': 'var(--blue-10)',
    '#42a5f5': 'var(--blue-10)',
    '#4a9eff': 'var(--blue-9)',
    '#29b6f6': 'var(--blue-10)',
    '#3b82f6': 'var(--blue-9)',
    '#2196f3': 'var(--blue-9)',
    '#1a73e8': 'var(--blue-9)',

    # Greens
    '#4caf50': 'var(--ok)',
    '#66bb6a': 'var(--ok)',
    '#81c784': 'var(--ok)',
    '#2ecc71': 'var(--ok)',
    '#22c55e': 'var(--ok)',
    '#1abc9c': 'var(--ok)',

    # Reds
    '#f44336': 'var(--alert)',
    '#ef5350': 'var(--alert)',
    '#e57373': 'var(--alert)',
    '#dc3545': 'var(--alert)',
    '#ff4757': 'var(--alert)',
    '#f66': 'var(--alert)',

    # Ambers
    '#ffc107': 'var(--warn)',
    '#ffa726': 'var(--warn)',
    '#fbbf24': 'var(--warn)',
    '#ff9800': 'var(--warn)',

    # Purples → blue accent (dark-only)
    '#ab47bc': 'var(--blue-9)',
    '#9c27b0': 'var(--blue-8)',
    '#ba68c8': 'var(--blue-10)',
}

# RGBA replacements
RGBA_MAP = {
    'rgba(79, 195, 247, 0.2)': 'var(--blue-muted)',
    'rgba(79,195,247,0.2)': 'var(--blue-muted)',
    'rgba(79, 195, 247, 0.25)': 'var(--blue-muted)',
    'rgba(79,195,247,0.25)': 'var(--blue-muted)',
    'rgba(79, 195, 247, 0.5)': 'var(--blue-border)',
    'rgba(79,195,247,0.5)': 'var(--blue-border)',
    'rgba(255, 255, 255, 0.03)': 'var(--bg-hover)',
    'rgba(255,255,255,0.03)': 'var(--bg-hover)',
    'rgba(255, 255, 255, 0.05)': 'var(--bg-hover)',
    'rgba(255,255,255,0.05)': 'var(--bg-hover)',
    'rgba(255, 255, 255, 0.08)': 'var(--border-default)',
    'rgba(255,255,255,0.08)': 'var(--border-default)',
    'rgba(255, 255, 255, 0.1)': 'var(--bg-hover)',
    'rgba(255,255,255,0.1)': 'var(--bg-hover)',
    'rgba(255, 255, 255, 0.12)': 'var(--bg-hover)',
    'rgba(255,255,255,0.12)': 'var(--bg-hover)',
    'rgba(255, 255, 255, 0.15)': 'var(--border-strong)',
    'rgba(255,255,255,0.15)': 'var(--border-strong)',
    'rgba(255, 255, 255, 0.5)': 'var(--text-muted)',
    'rgba(255,255,255,0.5)': 'var(--text-muted)',
    'rgba(255, 255, 255, 0.7)': 'var(--text-secondary)',
    'rgba(255,255,255,0.7)': 'var(--text-secondary)',
    'rgba(255, 255, 255, 0.9)': 'var(--text-primary)',
    'rgba(255,255,255,0.9)': 'var(--text-primary)',
    'rgba(0, 0, 0, 0.85)': 'var(--overlay-panel)',
    'rgba(0,0,0,0.85)': 'var(--overlay-panel)',
    'rgba(0, 0, 0, 0.8)': 'var(--overlay-strong)',
    'rgba(0,0,0,0.8)': 'var(--overlay-strong)',
    'rgba(0, 0, 0, 0.5)': 'var(--overlay)',
    'rgba(0,0,0,0.5)': 'var(--overlay)',
    'rgba(244, 67, 54, 0.3)': 'var(--alert-muted)',
    'rgba(244,67,54,0.3)': 'var(--alert-muted)',
    'rgba(244, 67, 54, 0.25)': 'var(--alert-muted)',
    'rgba(244,67,54,0.25)': 'var(--alert-muted)',
    'rgba(76, 175, 80, 0.3)': 'var(--ok-muted)',
    'rgba(76,175,80,0.3)': 'var(--ok-muted)',
    'rgba(76, 175, 80, 0.2)': 'var(--ok-muted)',
    'rgba(76,175,80,0.2)': 'var(--ok-muted)',
    'rgba(76, 175, 80, 0.15)': 'var(--ok-bg)',
    'rgba(76,175,80,0.15)': 'var(--ok-bg)',
    'rgba(102, 187, 106, 0.5)': 'var(--ok-muted)',
    'rgba(102,187,106,0.5)': 'var(--ok-muted)',
    'rgba(255, 167, 38, 0.5)': 'var(--warn-muted)',
    'rgba(255,167,38,0.5)': 'var(--warn-muted)',
    'rgba(239, 83, 80, 0.5)': 'var(--alert-muted)',
    'rgba(239,83,80,0.5)': 'var(--alert-muted)',
    'rgba(66, 165, 245, 0.7)': 'var(--blue-muted)',
    'rgba(66,165,245,0.7)': 'var(--blue-muted)',
    'rgba(66, 165, 245, 0.9)': 'var(--blue-muted)',
    'rgba(66,165,245,0.9)': 'var(--blue-muted)',
    'rgba(66, 165, 245, 0.15)': 'var(--blue-muted)',
    'rgba(66,165,245,0.15)': 'var(--blue-muted)',
    'rgba(66, 165, 245, 0.25)': 'var(--blue-muted)',
    'rgba(66,165,245,0.25)': 'var(--blue-muted)',
    'rgba(171, 71, 188, 0.5)': 'var(--blue-muted)',
    'rgba(171,71,188,0.5)': 'var(--blue-muted)',
    'rgba(255, 193, 7, 0.2)': 'var(--warn-bg)',
    'rgba(255,193,7,0.2)': 'var(--warn-bg)',
    'rgba(255, 193, 7, 0.4)': 'var(--warn-border)',
    'rgba(255,193,7,0.4)': 'var(--warn-border)',
    'rgba(255, 193, 7, 0.3)': 'var(--warn-muted)',
    'rgba(255,193,7,0.3)': 'var(--warn-muted)',
    'rgba(255, 193, 7, 0.6)': 'var(--warn-border)',
    'rgba(255,193,7,0.6)': 'var(--warn-border)',
    'rgba(255, 193, 7, 0.4)': 'var(--warn-border)',
    'rgba(255,193,7,0.4)': 'var(--warn-border)',
}

def process_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    changes = 0

    # Replace rgba values first (more specific)
    for old, new in RGBA_MAP.items():
        count = content.count(old)
        if count > 0:
            content = content.replace(old, new)
            changes += count

    # Replace hex colors
    lines = content.split('\n')
    result = []
    for line in lines:
        new_line = line
        hex_pattern = re.compile(r'#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})\b')
        matches = list(hex_pattern.finditer(line))
        for m in reversed(matches):
            hex_val = m.group(0).lower()
            # Skip meta theme-color tags
            prefix = line[:m.start()].rstrip()
            if 'content=' in prefix and 'theme-color' in line[:m.start()]:
                continue
            if hex_val in COLOR_MAP:
                replacement = COLOR_MAP[hex_val]
                new_line = new_line[:m.start()] + replacement + new_line[m.end():]
                changes += 1
        result.append(new_line)
    content = '\n'.join(result)

    # Replace font-family
    body_font = "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif"
    if body_font in content:
        count = content.count(body_font)
        content = content.replace(body_font, 'var(--font-body)')
        changes += count

    # Replace bare monospace in CSS context
    content = re.sub(r"font-family:\s*monospace;", "font-family: var(--font-mono);", content)
    changes_add = len(re.findall(r"font-family: var\(--font-mono\);", content))

    # Replace border-radius values
    # 2px, 3px → var(--radius-control)
    content = re.sub(r'border-radius:\s*([2-4])px(?!\s)', r'border-radius: var(--radius-control)', content)
    # 6px → var(--radius-control)
    content = re.sub(r'border-radius:\s*6px(?!\s)', 'border-radius: var(--radius-control)', content)
    # 8px, 10px, 12px → var(--radius-card)
    content = re.sub(r'border-radius:\s*(?:8|10|12)px(?!\s)', 'border-radius: var(--radius-card)', content)
    # 20px, 24px → var(--radius-modal)
    content = re.sub(r'border-radius:\s*(?:20|24)px(?!\s)', 'border-radius: var(--radius-modal)', content)

    if changes > 0:
        with open(filepath, 'w') as f:
            f.write(content)
        print(f"  {os.path.basename(filepath)}: {changes} replacements")
    else:
        print(f"  {os.path.basename(filepath)}: no changes needed")

    return changes

if __name__ == '__main__':
    html_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..')
    total = 0

    for fname in sorted(os.listdir(html_dir)):
        if not fname.endswith('.html'):
            continue
        fpath = os.path.join(html_dir, fname)
        total += process_file(fpath)

    print(f"\nTotal replacements: {total}")
