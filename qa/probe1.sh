#!/usr/bin/env bash
# 复现探针 1：加载/设置校验/过滤校验/特殊字符/空名称/窄窗口
set -uo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=/tmp/qa-probe1
rm -rf "$WORK"; mkdir -p "$WORK/data"
cp /tmp/tmp.Yb4d3AmNdI/subscription-speedtest.exe "$WORK/app.exe" 2>/dev/null || { echo "缺少已构建 exe"; exit 2; }
cat > "$WORK/a.yaml" <<'YAML'
proxies:
  - name: "HK-01"
    type: ss
    server: 127.0.0.1
    port: 1
    cipher: aes-128-gcm
    password: fixture
  - name: "HK-02"
    type: ss
    server: 127.0.0.1
    port: 2
    cipher: aes-128-gcm
    password: fixture
  - name: "JP-01"
    type: ss
    server: 127.0.0.1
    port: 3
    cipher: aes-128-gcm
    password: fixture
  - name: "香港 IPLC 01"
    type: ss
    server: 127.0.0.1
    port: 4
    cipher: aes-128-gcm
    password: fixture
  - name: "超长节点名称用于测试省略与截断显示ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    type: ss
    server: 127.0.0.1
    port: 5
    cipher: aes-128-gcm
    password: fixture
YAML
cat > "$WORK/data/entries.json" <<'JSON'
{"version":1,"entries":[{"id":"e1","name":"机场 A","path":"a.yaml"}]}
JSON
cat > "$WORK/data/settings.json" <<'JSON'
{"version":1,"settings":{"output_mode":"off","image_enabled":false,"rename_enabled":false,"max_latency_ms":0,"max_packet_loss":-1,"min_download_mbps":0,"min_upload_mbps":0}}
JSON
W=$(cygpath -w "$WORK" 2>/dev/null || echo "$WORK")
LOG=/tmp/qa-probe1.log
{
  echo 'size'
  echo 'texts'
  echo 'layout'
  echo "shot \"$W\\01-initial.png\""
  echo 'click "设置"'
  echo 'texts'
  echo 'layout'
  echo "shot \"$W\\02-settings.png\""
  echo 'click "测速服务器"'
  echo 'select 0 400'
  echo 'type not-a-url'
  echo 'click "保存设置"'
  echo 'texts'
  echo "shot \"$W\\03-settings-error.png\""
  echo 'click "恢复默认"'
  echo 'click "保存设置"'
  echo 'click "测速"'
  echo 'click "正则"'
  echo 'click "过滤表达式"'
  echo 'type ('
  echo 'key enter'
  echo 'texts'
  echo 'layout'
  echo "shot \"$W\\04-regex-error.png\""
  echo 'click "简化"'
  echo 'click "过滤表达式"'
  echo 'select 0 400'
  echo 'type <script>alert(1)</script>'
  echo 'key enter'
  echo 'texts'
  echo "shot \"$W\\05-special.png\""
  echo 'click "过滤表达式"'
  echo 'select 0 400'
  echo 'key backspace'
  echo 'click "新建条目"'
  echo 'texts'
  echo 'click "保存"'
  echo 'texts'
  echo "shot \"$W\\06-entry-empty.png\""
  echo 'click "取消"'
  echo 'resize 900 600'
  echo 'layout'
  echo "shot \"$W\\07-resize900.png\""
  echo 'resize 640 420'
  echo 'layout'
  echo "shot \"$W\\08-resize640.png\""
  echo 'resize 1180 800'
  echo 'layout'
  echo 'quit'
} | (cd "$WORK" && VIVA_HEADLESS=1 HOME="$WORK/home" ./app.exe) >"$LOG" 2>&1
echo "exit=$? log=$LOG"
mkdir -p "$ROOT/qa/shots"
cp "$WORK"/*.png "$ROOT/qa/shots/" 2>/dev/null
ls -la "$ROOT/qa/shots"
