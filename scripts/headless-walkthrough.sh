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

# 预置输出模式「当前目录下」：测完自动写配置与结果图（结果图在这条走查里关掉，
# 只验配置导出，免得字体渲染受环境影响）。版本 1 的字段缺失由加载时补默认值。
cat > "$tmp/data/settings.json" <<'JSON'
{"version":1,"settings":{"output_mode":"auto","image_enabled":false,"rename_enabled":false,"max_latency_ms":0,"max_packet_loss":-1,"min_download_mbps":0,"min_upload_mbps":0}}
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
  echo 'click "设置"'
  echo 'texts'
  echo 'click "下载大小（MB）"'
  echo 'select 0 4096'
  echo 'type 30'
  echo 'click "保存设置"'
  echo 'texts'
  echo 'click "测速"'
  # 多选语境用 Ctrl+点击标记两行（行选择与勾选分离，ADR-0029）：
  # 菜单要保留「单独测试此节点」，复制后关闭菜单。
  echo 'keydown ctrl'
  echo 'click "节点 机场 A / HK-01"'
  echo 'click "节点 机场 A / HK-02"'
  echo 'keyup ctrl'
  echo 'rightclick "节点 机场 A / HK-01"'
  echo 'texts'
  echo 'click "复制所选节点信息"'
  echo 'keydown ctrl'
  echo 'click "节点 机场 A / HK-01"'
  echo 'keyup ctrl'
  echo 'rightclick "节点 机场 A / HK-01"'
  echo 'texts'
  echo 'click "复制此节点信息"'
  echo 'clip'
  # 行选择不影响勾选：开测前把 HK-02 与 JP-01 移出测速范围，只测 HK-01。
  echo 'click "选择节点 机场 A / HK-02"'
  echo 'click "选择节点 机场 A / JP-01"'
  echo 'click "下载速度"'
  echo 'waitfor "已完成 1/1" 30000'
  echo 'waitfor "已导出" 30000'
  echo 'texts'
  echo 'click "折叠 机场 A 的节点"'
  echo 'texts'
  echo 'click "展开 机场 A 的节点"'
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
  echo 'click "折叠"'
  echo 'texts'
  echo 'click "展开节点来源"'
  echo 'move 324 300'
  echo 'down 324 300'
  echo 'move 424 300'
  echo 'up 424 300'
  echo 'hits'
  echo 'click "设置"'
  echo 'scroll 640 400 0 4000'
  echo 'hits'
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
# 条目路径用徽标显示形态，右键多选菜单带「单独测试此节点」。
expect '软路径'
# 组标题可折叠：折叠后出现「已折叠」，展开后节点行回来。
expect '已折叠'
expect '单独测试此节点'
expect '机场 B'
expect '测速与输出设置'
expect '已导出'
expect 'ok 0 problems'
expect 'error no control labelled "节点 机场 A / JP-01"'
# 折叠/展开与拖动分隔条：折叠后出现窄栏的展开按钮「»」，拖动后分隔条右移 100px。
expect '»'
expect 'entry-splitter x=420'
# 设置内容一定超过视口，滚动条应当存在且能滚。
expect 'settings-scrollbar'

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
grep -Fq '"output_mode": "auto"' "$tmp/data/settings.json" || { echo "FAIL：输出模式没有落盘" >&2; exit 1; }
grep -Fq '"download_mb": 30' "$tmp/data/settings.json" || { echo "FAIL：设置页的下载大小没有落盘" >&2; exit 1; }
export_yaml=$(ls "$tmp"/[0-9]*.机场A-导出.yaml "$tmp"/[0-9]*.a-导出.yaml 2>/dev/null | head -1 || true)
if [[ -z "$export_yaml" ]]; then
  echo "FAIL：没有写出导出配置" >&2
  ls "$tmp" >&2
  exit 1
fi
grep -Fq 'mixed-port: 7890' "$export_yaml" || { echo "FAIL：导出配置缺少 mixed-port" >&2; exit 1; }
grep -Fq 'MATCH,节点选择' "$export_yaml" || { echo "FAIL：导出配置缺少 MATCH 规则" >&2; exit 1; }
grep -Fq '"name": "机场 B"' "$tmp/data/entries.json" || { echo "FAIL：新建条目没有落盘" >&2; exit 1; }
grep -Fq '"path": "b.yaml"' "$tmp/data/entries.json" || { echo "FAIL：程序目录内路径没有存成软路径" >&2; exit 1; }
grep -Fq '"name": "只看HK"' "$tmp/data/presets.json" || { echo "FAIL：预设没有落盘" >&2; exit 1; }
grep -Fq '"ok": false' "$tmp/data/results.json" || { echo "FAIL：失败结果没有落盘" >&2; exit 1; }

echo "PASS：headless 走查通过"
echo "== 关键输出 =="
grep -E 'ok ready|ok 0 problems|已完成 1/1|复制此节点信息|只看HK|机场 B' "$log" | head -20
