#!/usr/bin/env bash
# spaxel Marathon Launcher — claude-code @ GLM-4.7 via ZAI proxy
#
# Runs the central marathon-coding skill in a dedicated tmux session against this
# repo. Each iteration reads .marathon/instruction.md and invokes headless
# claude-code routed through the ZAI proxy, mirroring the live NEEDLE
# claude-code-glm-4.7 agent.
#
# Usage:
#   ./.marathon/start.sh                 # session "spaxel-marathon"
#   ./.marathon/start.sh <session-name>  # custom session name

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
MARATHON_SKILL="/home/coding/claude-config/skills/marathon-coding"
INSTRUCTION_FILE="$SCRIPT_DIR/instruction.md"
LOG_DIR="$SCRIPT_DIR/logs"
SESSION_NAME="${1:-spaxel-marathon}"

# ZAI proxy — CURRENT endpoint is the apexalgo-iad Traefik vpn-entrypoint, NOT the
# decommissioned ardenone-hub proxy this repo's old start.sh pointed at. This mirrors
# the env of the live `claude-code-glm-4.7` NEEDLE agent.
ZAI_BASE_URL="https://traefik-apexalgo-iad.tail1b1987.ts.net:8444"

command -v tmux >/dev/null 2>&1 || { echo "Error: tmux not installed" >&2; exit 1; }
[ -x "$MARATHON_SKILL/launcher.sh" ] || { echo "Error: marathon launcher missing: $MARATHON_SKILL/launcher.sh" >&2; exit 1; }
[ -f "$INSTRUCTION_FILE" ] || { echo "Error: instruction file missing: $INSTRUCTION_FILE" >&2; exit 1; }

if tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
    echo "Session '$SESSION_NAME' already exists."
    echo "  Attach: tmux attach -t $SESSION_NAME"
    echo "  Kill:   tmux kill-session -t $SESSION_NAME"
    exit 1
fi

# Guard against running concurrently with a NEEDLE worker on the same worktree.
if pgrep -f "needle run --workspace $REPO_DIR" >/dev/null 2>&1; then
    echo "Error: a NEEDLE worker is running against $REPO_DIR." >&2
    echo "       Marathon + NEEDLE share one git worktree → contention." >&2
    echo "       Stop it first:  needle stop -i <identifier>" >&2
    exit 1
fi

# Preflight: any HTTP response = proxy is up; only a connection failure aborts.
if ! curl -sk --max-time 8 -o /dev/null "$ZAI_BASE_URL"; then
    echo "Error: ZAI proxy at $ZAI_BASE_URL is unreachable." >&2
    echo "       Check Tailscale + the proxy on apexalgo-iad." >&2
    exit 1
fi

mkdir -p "$LOG_DIR"

LOOP_CMD="cd '$REPO_DIR' && \
    unset CLAUDECODE && \
    export NODE_TLS_REJECT_UNAUTHORIZED=0 && \
    export ANTHROPIC_BASE_URL='$ZAI_BASE_URL' && \
    export ANTHROPIC_AUTH_TOKEN='proxy-handles-auth' && \
    export ANTHROPIC_MODEL='glm-4.7' && \
    export ANTHROPIC_DEFAULT_OPUS_MODEL='glm-4.7' && \
    export ANTHROPIC_DEFAULT_SONNET_MODEL='glm-4.7' && \
    export ANTHROPIC_DEFAULT_HAIKU_MODEL='glm-4.7' && \
    export CLAUDE_CODE_SUBAGENT_MODEL='glm-4.7' && \
    export API_TIMEOUT_MS='900000' && \
    export DISABLE_AUTOUPDATER=1 && \
    export DISABLE_TELEMETRY=1 && \
    '$MARATHON_SKILL/launcher.sh' \
        --prompt '$INSTRUCTION_FILE' \
        --model glm-4.7 \
        --delay 10 \
        --log-dir '$LOG_DIR'"

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║          spaxel Marathon — claude-code @ GLM-4.7            ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo "  Repo:        $REPO_DIR"
echo "  Instruction: $INSTRUCTION_FILE"
echo "  Session:     $SESSION_NAME"
echo "  Model:       glm-4.7 (all tiers)"
echo "  Proxy:       $ZAI_BASE_URL"
echo "  Logs:        $LOG_DIR"
echo ""

tmux new-session -d -s "$SESSION_NAME" -c "$REPO_DIR" "$LOOP_CMD"

echo "Marathon running in tmux session: $SESSION_NAME"
echo "  Attach:  tmux attach -t $SESSION_NAME"
echo "  Detach:  Ctrl+B, D (while attached)"
echo "  Stop:    tmux kill-session -t $SESSION_NAME"
echo "  Logs:    ls $LOG_DIR/"
