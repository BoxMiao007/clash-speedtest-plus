#!/usr/bin/env bash
set -uo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=/tmp/qa-probe6
rm -rf "$WORK"; mkdir -p "$WORK/data"
cp /tmp/tmp.Yb4d3AmNdI/subscription-speedtest.exe "$WORK/app.exe" 2>/dev/null || { echo "缺少 exe"; exit 2; }
cat > "$WORK/a.yaml" <<'YAML'
proxies:
  - {name: "HK-01", type: ss, server: 127.0.0.1, port: 1, cipher: aes-128-gcm, password: fixture}
  - {name: "HK-02", type: ss, server: 127.0.0.1, port: 2, cipher: aes-128-gcm, password: fixture}
  - {name: "HK-03", type: ss, server: 127.0.0.1, port: 3, cipher: aes-128-gcm, password: fixture}
YAML
printf '%s' '{"version":1,"entries":[{"id":"e1","name":"机场 A","path":"a.yaml"}]}' > "$WORK/data/entries.json"
printf '%s' '{"version":1,"settings":{"output_mode":"off"}}' > "$WORK/data/settings.json"
LOG=/tmp/qa-probe6.log
{
  echo 'size'
  echo 'click "打开节点详情 HK-01"'
  echo 'hits'
  echo 'texts'
  echo 'click 600 315'
  echo 'texts'
  echo 'click 749 315'
  echo 'texts'
  echo 'quit'
} | (cd "$WORK" && VIVA_HEADLESS=1 HOME="$WORK/home" ./app.exe) >"$LOG" 2>&1
echo "exit=$? log=$LOG"
