#!/usr/bin/env bash
# 复现探针 9：深色/浅色两套主题的界面走查与截图（无窗口 headless）。
# 覆盖：测速表、设置页、右键菜单、详情弹窗、新建条目对话框。
set -uo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=/tmp/qa-probe9
rm -rf "$WORK"; mkdir -p "$WORK/data" "$WORK/home"
echo "== 构建 vivahead 二进制 =="
(cd "$ROOT" && GOPROXY=${GOPROXY:-https://goproxy.cn,direct} go build -tags vivahead -o "$WORK/app.exe" .) || { echo "构建失败"; exit 2; }
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
YAML
cat > "$WORK/data/entries.json" <<'JSON'
{"version":1,"entries":[{"id":"e1","name":"机场 A","path":"a.yaml"}]}
JSON
# 不带 theme 字段：验证空值按深色处理。
cat > "$WORK/data/settings.json" <<'JSON'
{"version":1,"settings":{"output_mode":"off","image_enabled":false,"rename_enabled":false,"max_latency_ms":0,"max_packet_loss":-1,"min_download_mbps":0,"min_upload_mbps":0}}
JSON
W=$(cygpath -w "$WORK" 2>/dev/null || echo "$WORK")
LOG=/tmp/qa-probe9.log
{
  echo 'size'
  echo 'texts'
  echo "shot \"$W\\91-dark-speedtest.png\""
  echo 'click "浅色"'
  echo 'texts'
  echo "shot \"$W\\92-light-speedtest.png\""
  echo 'click "设置"'
  echo 'layout'
  echo "shot \"$W\\93-light-settings.png\""
  echo 'click "深色"'
  echo "shot \"$W\\94-dark-settings.png\""
  echo 'click "测速"'
  echo 'click "浅色"'
  echo 'click "新建条目"'
  echo 'texts'
  echo "shot \"$W\\95-light-dialog.png\""
  echo 'click "取消"'
  echo 'click "深色"'
  echo 'rightclick "节点 机场 A / HK-01"'
  echo 'texts'
  echo "shot \"$W\\96-dark-menu.png\""
  echo 'key esc'
  echo 'click "打开节点详情 HK-01"'
  echo 'texts'
  echo "shot \"$W\\97-dark-detail.png\""
  echo 'key esc'
  echo 'layout'
  echo 'quit'
} | (cd "$WORK" && VIVA_HEADLESS=1 HOME="$WORK/home" ./app.exe) >"$LOG" 2>&1
echo "exit=$? log=$LOG"
mkdir -p "$ROOT/qa/shots"
cp "$WORK"/*.png "$ROOT/qa/shots/" 2>/dev/null
grep -n "error" "$LOG" | head -20
ls -la "$ROOT/qa/shots" | tail -10
