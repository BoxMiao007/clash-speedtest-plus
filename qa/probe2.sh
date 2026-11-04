#!/usr/bin/env bash
# 复现探针 2：跑一轮失败测速 → 结果表/详情弹窗/右键菜单/清空结果确认/删除条目确认
set -uo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=/tmp/qa-probe2
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
YAML
cat > "$WORK/data/entries.json" <<'JSON'
{"version":1,"entries":[{"id":"e1","name":"机场 A","path":"a.yaml"}]}
JSON
cat > "$WORK/data/settings.json" <<'JSON'
{"version":1,"settings":{"output_mode":"off","image_enabled":false,"rename_enabled":false,"max_latency_ms":0,"max_packet_loss":-1,"min_download_mbps":0,"min_upload_mbps":0}}
JSON
W=$(cygpath -w "$WORK" 2>/dev/null || echo "$WORK")
LOG=/tmp/qa-probe2.log
{
  echo 'size'
  echo 'texts'
  echo 'click "下载速度"'
  echo 'waitfor "已完成 3/3" 30000'
  echo 'texts'
  echo 'layout'
  echo "shot \"$W\\10-results.png\""
  echo 'click "打开节点详情 HK-01"'
  echo 'texts'
  echo "shot \"$W\\11-detail.png\""
  echo 'key esc'
  echo 'rightclick "节点 机场 A / HK-01"'
  echo 'texts'
  echo "shot \"$W\\12-contextmenu.png\""
  echo 'key esc'
  echo 'dblclick "下载速度"'
  echo 'waitfor "已完成 3/3" 30000'
  echo 'texts'
  echo 'click "清空结果"'
  echo 'texts'
  echo "shot \"$W\\13-clear-confirm.png\""
  echo 'click "取消"'
  echo 'click "删除"'
  echo 'texts'
  echo "shot \"$W\\14-delete-confirm.png\""
  echo 'click "取消"'
  echo 'quit'
} | (cd "$WORK" && VIVA_HEADLESS=1 HOME="$WORK/home" ./app.exe) >"$LOG" 2>&1
echo "exit=$? log=$LOG"
mkdir -p "$ROOT/qa/shots"
cp "$WORK"/*.png "$ROOT/qa/shots/" 2>/dev/null
ls -la "$ROOT/qa/shots"
