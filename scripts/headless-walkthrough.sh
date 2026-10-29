#!/usr/bin/env bash
# 无窗口 headless 走查：构建 -tags vivahead 的二进制，用线协议操作真实应用，
# 校验界面文案、右键菜单、过滤、预设持久化与条目新建。全程不打开窗口、不联网
# （夹具节点指向 127.0.0.1:1，延迟探测立即失败）。
#
# 用法：scripts/headless-walkthrough.sh
# 依赖：Go、Git Bash（Windows）或 bash（Linux/macOS）。
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
cleanup() {
  if [[ ${KEEP_TMP:-0} == 1 ]]; then
    echo "保留临时目录：$tmp"
  else
    rm -rf "$tmp"
  fi
}
trap cleanup EXIT
mkdir -p "$tmp/home"

cd "$root"
echo "== 构建 vivahead 二进制 =="
GOPROXY=${GOPROXY:-https://goproxy.cn,direct} go build -tags vivahead -o "$tmp/subscription-speedtest.exe" .
echo "构建完成：$tmp/subscription-speedtest.exe"

cat > "$tmp/a.yaml" <<'YAML'
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

cat > "$tmp/b.yaml" <<'YAML'
proxies:
  - name: "HK-09"
    type: ss
    server: 127.0.0.1
    port: 9
    cipher: aes-128-gcm
    password: fixture
YAML

mkdir -p "$tmp/data"
cat > "$tmp/data/entries.json" <<'JSON'
{"version":1,"entries":[{"id":"e1","name":"机场 A","path":"a.yaml"}]}
JSON

log="$tmp/walkthrough.log"
if command -v cygpath >/dev/null 2>&1; then
  dialog_tmp=$(cygpath -w "$tmp")
else
  dialog_tmp=$tmp
fi
{
  echo 'size'
  echo 'texts'
  echo 'hits'
  echo 'click "选择节点 机场 A / HK-02"'
  echo 'click "选择节点 机场 A / JP-01"'
  echo 'rightclick "节点 机场 A / HK-01"'
  echo 'texts'
  echo 'click "复制此节点信息"'
  echo 'clip'
  echo 'click "下载速度"'
  echo 'waitfor "已完成 1/1" 30000'
  echo 'texts'
  echo 'click "返回选择"'
  echo 'click "过滤表达式"'
  echo 'type HK !02'
  echo 'key enter'
  echo 'texts'
  echo 'find "节点 机场 A / HK-01"'
  echo 'find "节点 机场 A / JP-01"'
  echo 'click "保存预设"'
  echo 'click "预设名称"'
  echo 'type 只看HK'
  echo 'click "保存"'
  echo 'click "新建条目"'
  echo 'click "名称"'
  echo 'type 机场 B'
  echo "openfiles ${dialog_tmp}\\b.yaml"
  echo 'click "选择文件…"'
  echo 'click "保存"'
  echo 'texts'
  echo 'layout'
  echo 'quit'
} | (cd "$tmp" && VIVA_HEADLESS=1 HOME="$tmp/home" ./subscription-speedtest.exe) >"$log" 2>&1

fail=0
expect() {
  local needle=$1
  if ! grep -Fq -- "$needle" "$log"; then
    echo "FAIL：输出里找不到「$needle」" >&2
    fail=1
  fi
}

expect 'ok ready'
expect '机场 A'
expect '节点 机场 A / HK-01'
expect '未测试'
expect '复制此节点信息'
expect 'ok "名称：HK-01'
expect '已完成 1/1'
expect '失败'
expect 'label="节点 机场 A / HK-01"'
expect '只看HK'
expect '机场 B'
expect 'ok 0 problems'
expect 'error no control labelled "节点 机场 A / JP-01"'

error_count=$(grep -c '^error' "$log" || true)
if [[ "$error_count" -ne 1 ]]; then
  echo "FAIL：预期只有过滤后查找 JP-01 的 1 条 error，实际 $error_count 条" >&2
  grep '^error' "$log" >&2 || true
  fail=1
fi

if [[ $fail -ne 0 ]]; then
  echo "== headless 走查失败，完整日志：$log =="
  cat "$log"
  exit 1
fi

echo "== 持久化断言 =="
grep -Fq '"name": "机场 B"' "$tmp/data/entries.json" || { echo "FAIL：新建条目没有落盘" >&2; exit 1; }
grep -Fq '"path": "b.yaml"' "$tmp/data/entries.json" || { echo "FAIL：程序目录内路径没有存成软路径" >&2; exit 1; }
grep -Fq '"name": "只看HK"' "$tmp/data/presets.json" || { echo "FAIL：预设没有落盘" >&2; exit 1; }
grep -Fq '"ok": false' "$tmp/data/results.json" || { echo "FAIL：失败结果没有落盘" >&2; exit 1; }

echo "PASS：headless 走查通过"
echo "== 关键输出 =="
grep -E 'ok ready|ok 0 problems|已完成 1/1|复制此节点信息|只看HK|机场 B' "$log" | head -20
