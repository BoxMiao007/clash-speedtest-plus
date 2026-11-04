#!/usr/bin/env bash
# 复现探针 3：键盘焦点/ Esc 关对话框 / 窗口宽度阈值 / 打开目录接缝
set -uo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=/tmp/qa-probe3
rm -rf "$WORK"; mkdir -p "$WORK/data"
cp /tmp/tmp.Yb4d3AmNdI/subscription-speedtest.exe "$WORK/app.exe" 2>/dev/null || { echo "缺少 exe"; exit 2; }
cat > "$WORK/a.yaml" <<'YAML'
proxies:
  - name: "HK-01"
    type: ss
    server: 127.0.0.1
    port: 1
    cipher: aes-128-gcm
    password: fixture
YAML
printf '%s' '{"version":1,"entries":[{"id":"e1","name":"机场 A","path":"a.yaml"}]}' > "$WORK/data/entries.json"
printf '%s' '{"version":1,"settings":{"output_mode":"off"}}' > "$WORK/data/settings.json"
W=$(cygpath -w "$WORK" 2>/dev/null || echo "$WORK")
LOG=/tmp/qa-probe3.log
{
  echo 'size'
  echo 'click "设置"'
  echo 'focus'
  echo 'key tab'; echo 'focus'
  echo 'key tab'; echo 'focus'
  echo 'key tab'; echo 'focus'
  echo 'key tab'; echo 'focus'
  echo 'key tab'; echo 'focus'
  echo 'key tab'; echo 'focus'
  echo 'key tab'; echo 'focus'
  echo 'key tab'; echo 'focus'
  echo 'key shift+tab'; echo 'focus'
  echo 'click "测速"'
  echo 'key tab'; echo 'focus'
  echo 'key tab'; echo 'focus'
  echo 'click "新建条目"'
  echo 'texts'
  echo 'key esc'
  echo 'texts'
  echo 'resize 800 600'
  echo 'layout'
  echo "shot \"$W\\20-resize800.png\""
  echo 'resize 1000 650'
  echo 'layout'
  echo "shot \"$W\\21-resize1000.png\""
  echo 'resize 1180 800'
  echo 'rightclick "节点 机场 A / HK-01"'
  echo 'click "打开配置文件所在目录"'
  echo 'openedpath'
  echo 'quit'
} | (cd "$WORK" && VIVA_HEADLESS=1 HOME="$WORK/home" ./app.exe) >"$LOG" 2>&1
echo "exit=$? log=$LOG"
mkdir -p "$ROOT/qa/shots"
cp "$WORK"/*.png "$ROOT/qa/shots/" 2>/dev/null
cd "$ROOT/qa/shots"; for f in *probe3*.png; do [ -e "$f" ] || continue; n=${f##*qa-probe3}; mv "$f" "p3-$n"; done
ls -1 "$ROOT/qa/shots" | grep p3 || true
