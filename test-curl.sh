#!/usr/bin/env bash
set -euo pipefail

addr="${VOICE_DAEMON_ADDR:-127.0.0.1:47832}"
token="${VOICE_DAEMON_TOKEN:?VOICE_DAEMON_TOKEN is required}"
text="${1:-你好，世界}"
paste_chord="${VOICE_DAEMON_PASTE_CHORD:-ctrl_v}"

curl -i -X POST \
  --max-time 10 \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: text/plain; charset=utf-8" \
  -H "X-Voice-Paste-Chord: ${paste_chord}" \
  --data-binary "${text}" \
  "http://${addr}/type"
