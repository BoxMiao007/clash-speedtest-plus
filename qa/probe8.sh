#!/usr/bin/env bash
set -uo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=/tmp/qa-probe8
rm -rf "$WORK"; mkdir -p "$WORK/data"
cp /tmp/tmp.Yb4d3AmNdI/subscription-speedtest.exe "$WORK/app.exe" 2>/dev/null || { echo "缺少 exe"; exit 2; }
cat > "$WORK/a.yaml" <<'YAML'
proxies:
  - {name: "HK-01", type: ss, server: 127.0.0.1, port: 1, cipher: aes-128-gcm, password: fixture}
YAML
printf '%s' '{"version":1,"entries":[{"id":"e1","name":"机场 A","path":"a.yaml"}]}' > "$WORK/data/entries.json"
printf '%s' '{"version":1,"settings":{"output_mode":"off"}}' > "$WORK/data/settings.json"
LOG=/tmp/qa-probe8.log
{
  echo 'size'
  echo 'click "设置"'
  echo 'click "下载大小（MB）"'
  echo 'select 0 400'
  echo 'type 999999999999'
  echo 'click "保存设置"'
  echo 'texts'
  echo 'click "最低下载（MB/s）"'
  echo 'select 0 400'
  echo 'type NaN'
  echo 'click "保存设置"'
  echo 'texts'
  echo 'click "测速"'
  echo 'click "选择条目 机场 A"'
  echo 'texts'
  echo 'quit'
} | (cd "$WORK" && VIVA_HEADLESS=1 HOME="$WORK/home" ./app.exe) >"$LOG" 2>&1
echo "exit=$? log=$LOG"
