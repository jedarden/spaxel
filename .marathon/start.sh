#!/bin/bash
# Start Spaxel marathon coding session with GLM-5 via ZAI proxy
#
# Launches in a dedicated tmux session. The marathon loop runs claude-code
# configured to use GLM-5 through the ZAI Tailscale proxy.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MARATHON_DIR="/home/coding/Research/.claude/skills/marathon-coding"
INSTRUCTION_FILE="$SCRIPT_DIR/instruction.md"
SESSION_NAME="spaxel-glm5"

# Verify prerequisites
if ! command -v tmux &>/dev/null; then
    echo "Error: tmux not installed"
    exit 1
fi

if tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
    echo "Session '$SESSION_NAME' already exists."
    echo "  Attach: tmux attach -t $SESSION_NAME"
    echo "  Kill:   tmux kill-session -t $SESSION_NAME"
    exit 1
fi

if [ ! -f "$INSTRUCTION_FILE" ]; then
    echo "Error: Instruction file not found: $INSTRUCTION_FILE"
    exit 1
fi

if [ ! -f "$MARATHON_DIR/launcher.sh" ]; then
    echo "Error: Marathon launcher not found: $MARATHON_DIR/launcher.sh"
    exit 1
fi

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║         Spaxel Marathon — GLM-5 via ZAI Proxy               ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""
echo "  Model:       GLM-5 (via zai-proxy-apexalgo)"
echo "  Subagents:   GLM-5-Turbo"
echo "  Instruction: $INSTRUCTION_FILE"
echo "  Session:     $SESSION_NAME"
echo "  Working dir: /home/coding/spaxel"
echo ""

# Create tmux session with GLM-5 env vars and run the marathon launcher
tmux new-session -d -s "$SESSION_NAME" -c "/home/coding/spaxel" \
    "export ANTHROPIC_BASE_URL='http://zai-proxy-apexalgo.tail1b1987.ts.net:8080' && \
     export ANTHROPIC_AUTH_TOKEN='proxy-handles-auth' && \
     export ANTHROPIC_MODEL='glm-5' && \
     export ANTHROPIC_DEFAULT_OPUS_MODEL='glm-5' && \
     export ANTHROPIC_DEFAULT_SONNET_MODEL='glm-5-turbo' && \
     export ANTHROPIC_DEFAULT_HAIKU_MODEL='glm-4.7' && \
     export CLAUDE_CODE_SUBAGENT_MODEL='glm-5-turbo' && \
     export DISABLE_AUTOUPDATER=1 && \
     export DISABLE_TELEMETRY=1 && \
     $MARATHON_DIR/launcher.sh --instruction '$INSTRUCTION_FILE' --delay 10 --log-dir '$SCRIPT_DIR/logs'"

echo "Session started in tmux: $SESSION_NAME"
echo ""
echo "  Attach:  tmux attach -t $SESSION_NAME"
echo "  Detach:  Ctrl+B, D"
echo "  Kill:    tmux kill-session -t $SESSION_NAME"
echo "  Logs:    $SCRIPT_DIR/logs/"
