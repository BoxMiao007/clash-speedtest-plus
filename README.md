# Clash-SpeedTest

本仓库是 [faceair/clash-speedtest](https://github.com/faceair/clash-speedtest) 的下游版本，会持续合并上游更新。协议仍为 [GPL-3.0](LICENSE)。

基于 Clash/Mihomo 核心的测速工具，快速测试你的节点速度。

Features:
1. 无需额外的配置，直接将 Clash/Mihomo 配置本地文件路径或者订阅地址作为参数传入即可
2. 支持 Proxies 和 Proxy Provider 中定义的全部类型代理节点，兼容性跟 Mihomo 一致
3. 不依赖额外的 Clash/Mihomo 进程实例，单一工具即可完成测试
4. 代码开源。各平台二进制在本仓库 [Releases](https://github.com/BoxMiao007/clash-speedtest-plus/releases) 下载，也可自行从源码构建

<img width="1346" height="682" alt="Image" src="https://github.com/user-attachments/assets/9fea1d47-251f-4c49-b059-05b5962d4e72" />

## Prerequisites/注意事项

### OpenWRT 环境
在 OpenWRT 环境下使用本工具时，建议临时关闭 OpenClash/Clash/Mihomo 等代理服务，以避免路由冲突影响测速结果的准确性。或者给 OpenClash/Clash/Mihomo 配置进程规则绕过代理：
```
rules:
  - PROCESS-NAME,clash-speedtest,DIRECT
```

### Windows CMD 用户
在 Windows CMD 中使用时，如果订阅地址包含 `&` 字符，必须使用双引号而非单引号：
```bash
# 正确
> clash-speedtest -c "https://domain.com/api/v1/client/subscribe?token=secret&flag=meta"

# 错误
> clash-speedtest -c 'https://domain.com/api/v1/client/subscribe?token=secret&flag=meta'
```

## 双击启动（无参数选源界面）

不带任何参数启动（例如在资源管理器里双击）会先打开选源界面：
- 左侧列出程序所在目录里所有能解析出节点的 `.yaml`，空格或点击勾选，可以多选；右侧是全部命令行选项，两栏各自滚动；
- 中间一行填订阅地址，多条之间用「逗号加空格」或换行分开，配置和地址可以混在一次测速里；
- 空格勾选、切换开关；选项栏里左右键调整当前参数（模式循环切换、数字按步长增减），文件栏里左右键切到选项栏；可调行的值带 `< >` 符号，**鼠标点 `<` 减、点 `>` 加，点值本身只选中**，开关行点哪儿都切换；数字类选项只能输入数字；不适用的选项会标成斜体「不可用」；
- 回车开始测速，进入和命令行相同的测速表格界面——选源界面只负责把参数整理成命令行，实际运行走同一条底层路径；测试界面里按 **Esc 返回选源界面**（中断本轮、不写产物），勾选、地址和选项全部保留，改完可以再测；命令行方式启动的测试界面没有这一级，Esc 行为不变；
- 鼠标可用：点击文件行勾选、点击地址行定位；点击选项行的 `<`/`>` 符号增减参数（精准命中，点其他位置只选中），右键无操作；帮助行的「Enter 开始测速」和「Ctrl+C 退出」两段高亮显示，点击即执行；右键上报需要 Windows Terminal 等支持 SGR 鼠标模式的终端；
- 界面放不下时自动滚动到光标行，鼠标滚轮也可以滚动；选源界面里只能用 Ctrl+C 离开。选项不记忆，每轮从界面默认值开始：节点并行默认 6、结果图只留有速度默认开（命令行参数的默认值不受影响）。

脚本、管道等没有终端的环境里不带参数启动，则打印用法并以非 0 退出，不会假装打开了界面。

## 订阅地址自动规范化

订阅内容不需要已经是 Clash/Mihomo 配置，程序会按顺序自动尝试：
1. 原样当 Clash 配置解析；
2. 不行则整段 base64 解码后再解析；
3. 还不行且地址里没有 `flag` 参数时，补上 `flag=meta` 重新请求，再重复前两步。

一旦解析出节点就停止；都不行时报错说明。实际用了补过参数的地址会提示出来。选源界面和命令行 `-c` 用同一套规则。分享链接（`vmess://` 等）和 JSON 订阅不支持。

拉取请求默认带 `clash.meta` User-Agent——多数机场订阅按 UA 分流，clash 系标识才返回 Clash/Mihomo 格式。选源界面可在「拉取订阅 UA」改成任意标识，改完重试立即生效；命令行用 `-ua`。

## 产物写到程序目录

结果图，以及相对路径的 `-o` 输出，都写到程序文件所在目录，而不是启动时的当前目录。绝对路径不受影响。

## 使用方法

```bash
# 从本仓库 Release 下载对应系统的二进制，或从源码安装
> go install github.com/BoxMiao007/clash-speedtest-plus@latest

# 查看版本
> clash-speedtest -v

# 查看帮助
> clash-speedtest -h
用法：clash-speedtest [选项]
  -c string
        配置文件路径，也支持 http(s) 地址
  -f string
        按节点名过滤，使用正则（默认: .+）
  -b string
        按关键字屏蔽节点，多个关键字用 | 分隔
  -o string
        输出配置文件路径（也可写 -output）
  -p int
        同时测试的节点数（也可写 -parallel | 默认: 1）
  -v
        显示版本信息

  -fast
        快速模式（等同 --speed-mode fast）
  -speed-mode string
        测速模式：fast、download、full（默认: download）
  -concurrent int
        同一节点的下载并发连接数（默认: 4）
  -download-size int
        下载测试大小（默认: 50 | 单位：MB）
  -upload-size int
        上传测试大小，仅完整模式（默认: 20 | 单位：MB）
  -timeout duration
        单个请求超时（默认: 5s）
  -early-stop int
        过筛结果达到该数量后提前结束（0 为关闭）
  -no-image
        关闭自动导出结果图；交互界面按 s 仍可手动保存
  -image-speed-only
        结果图只保留下载或上传速度大于 0 的行；快速模式会忽略

完整参数以 `clash-speedtest -h` 为准。`-o` 与 `-output` 相同，`-p` 与 `-parallel` 相同。下载、上传大小按 MB 填写。交互界面里空格暂停或继续，q 或 Ctrl+C 退出，点某一行打开详情，拖滚动条只滚动不改选中。测试进行中列表**默认跟随最新条目滚动**：向上滚取消跟随，滚回底部自动恢复。整轮结束后会在程序所在目录写出结果图；类型列按类型名撑开。结果图顶部的文件名或链接超宽会自动折行；速度筛选开启时，摘要括号里给出有效、无效、测试中和未测试四项计数（某项为 0 时省略）。结果图里各指标列的颜色深浅按**本批结果该列的最小到最大值**独立归一化：最小值最浅、最大值最深，下载列和上传列互不干扰。

# 演示：

# 1. 测试全部节点，使用 HTTP 订阅地址
# 不是 Clash 配置的订阅会自动尝试 base64 和补 flag=meta（见上文「订阅地址自动规范化」）
> clash-speedtest -c 'https://domain.com/api/v1/client/subscribe?token=secret&flag=meta'

# 2. 测试香港节点，使用正则表达式过滤，使用本地文件
> clash-speedtest -c ~/.config/clash/config.yaml -f 'HK|港'
节点                                        	带宽          	延迟
Premium|广港|IEPL|01                        	484.80KB/s  	815.00ms
Premium|广港|IEPL|02                        	N/A         	N/A
Premium|广港|IEPL|03                        	2.62MB/s    	333.00ms
Premium|广港|IEPL|04                        	1.46MB/s    	272.00ms
Premium|广港|IEPL|05                        	3.87MB/s    	249.00ms

# 3. 当然你也可以混合使用
> clash-speedtest -c "https://domain.com/api/v1/client/subscribe?token=secret&flag=meta,/home/.config/clash/config.yaml"

# 4. 筛选出延迟低于 800ms 且下载速度大于 5MB/s 的节点，并输出到 filtered.yaml
> clash-speedtest -c "https://domain.com/api/v1/client/subscribe?token=secret&flag=meta" -output filtered.yaml -max-latency 800ms -min-download-speed 5
# 筛选后的配置文件可以直接粘贴到 Clash/Mihomo 中使用，或是贴到 Github\Gist 上通过 Proxy Provider 引用。
# 如果只需要前 20 个满足筛选条件的节点，可以加 -early-stop 20，达到数量后会停止继续测速。

# 5. 使用 -rename 选项按照 IP 地区和下载速度重命名节点
> clash-speedtest -c config.yaml -output result.yaml -rename
# 重命名后的节点名称格式：🇺🇸 US 001 | ⬇️ 15.67MB/s
# 包含国旗 emoji、国家代码和下载速度

# 6. 快速测试模式
> clash-speedtest -f 'HK' -fast -c ~/.config/clash/config.yaml
# 此命令将只测试节点延迟，跳过其他测试项目，适用于：
# - 快速检查节点是否可用
# - 只需要检查延迟的场景
# - 需要快速得到测试结果的场景
🇭🇰 香港 HK-10 100% |██████████████████| (20/20, 13 it/min)
序号    节点名称                类型            延迟
1.      🇭🇰 香港 HK-01           Trojan          657ms
2.      🇭🇰 香港 HK-20           Trojan          649ms
3.      🇭🇰 香港 HK-15           Trojan          674ms
4.      🇭🇰 香港 HK-19           Trojan          649ms
5.      🇭🇰 香港 HK-12           Trojan          667ms

# 7. 上传到 GitHub Gist
> clash-speedtest -c config.yaml -output result.yaml -gist-token "ghp_xxx" -gist-address "https://gist.github.com/user/abc123"
# 测试完成后，会将 result.yaml 上传到指定的 Gist，文件名与 -output 保持一致（去除目录前缀）
# gist-address 可以是完整的 Gist URL，也可以是 Gist ID（如 abc123）
# Gist/Repo 上传与远程配置 URL 加载默认遵循环境代理变量（HTTPS_PROXY/HTTP_PROXY）。

# 8. 上传到 GitHub 仓库文件（默认写入 output 文件名）
> clash-speedtest -c config.yaml -output result.yaml -repo-token "ghp_xxx" -repo-address "user/repo"
# 测试完成后，会将 result.yaml 上传到仓库默认分支下的 result.yaml

# 9. 上传到 GitHub 仓库指定分支与路径
> clash-speedtest -c config.yaml -output result.yaml -repo-token "ghp_xxx" -repo-address "https://github.com/user/repo" -repo-file-path "configs/subscriptions/result.yaml" -repo-branch "main"
```

## GitHub Token 创建与权限

### 1) 更新 Gist（`-gist-token`）

推荐使用 **Personal access tokens (classic)**：

1. 打开 GitHub `Settings` → `Developer settings` → `Personal access tokens` → `Tokens (classic)`。
2. 点击 `Generate new token (classic)`。
3. 仅勾选最小权限：`gist`。
4. 生成后复制 token，作为 `-gist-token` 传入。

最小权限结论：
- `gist`：必需（用于通过 API 更新 Gist 文件）。

### 2) 更新仓库文件（`-repo-token`）

可选两种 token：

#### A. Fine-grained PAT（推荐）

1. 打开 GitHub `Settings` → `Developer settings` → `Personal access tokens` → `Fine-grained tokens`。
2. `Repository access` 选择目标仓库（建议 `Only select repositories`）。
3. 在 `Repository permissions` 中设置：
   - `Contents`: **Read and write**（必需）
4. 生成后复制 token，作为 `-repo-token` 传入。

#### B. Tokens (classic)

- 更新**公开仓库**文件：至少 `public_repo`。
- 更新**私有仓库**文件：至少 `repo`。

最小权限结论：
- Fine-grained PAT：`Contents: Read and write`。
- Classic PAT：公有仓库 `public_repo`，私有仓库 `repo`。

### 常见权限问题

- `401 Unauthorized`：token 无效、过期，或复制时有空格/换行。
- `403 Forbidden`：token 权限不足，或目标分支启用了保护策略（可能禁止直接 push/commit）。
- `404 Not Found`：仓库地址/路径/分支不正确，或 token 对该仓库不可见。

> 安全建议：不要把 token 提交到仓库；优先通过环境变量或 CI Secret 注入。

## 测速原理

通过 HTTP GET 请求下载指定大小的文件，默认使用 https://dl.google.com/chrome/mac/universal/stable/GGRO/googlechrome.dmg 进行测试，计算下载时间得到下载速度。因为 speed.cloudflare.com 容易返回 403，所以默认不再使用它作为测速入口。

当 server-url 不带 path 时 (使用 https://speed.cloudflare.com 或自建测速服务)，使用 /__down 和 /__up 完成下载与上传测试。
当 server-url 带 path 时，会被识别为直接下载地址，只进行下载测速。

如果你确认 https://speed.cloudflare.com 可以访问并希望测试上传，请显式设置为 full 模式，例如：
```shell
clash-speedtest --server-url "https://speed.cloudflare.com" --speed-mode full
```
或者你也可以自己搭建一个测速服务器，用来测试下载和上传速度：

```shell
# 在您需要进行测速的服务器上安装和启动测速服务器
> go install github.com/faceair/clash-speedtest/download-server@latest
> download-server

# 此时在本地使用 http://your-server-ip:8080 作为 server-url 即可
> clash-speedtest --server-url "http://your-server-ip:8080" --speed-mode full
```


测试结果：
1. 带宽 是指下载指定大小文件的速度，即一般理解中的下载速度。当这个数值越高时表明节点的出口带宽越大。
2. 延迟 是指 HTTP GET 请求拿到第一个字节的的响应时间，即一般理解中的 TTFB。当这个数值越低时表明你本地到达节点的延迟越低，可能意味着中转节点有 BGP 部署、出海线路是 IEPL、IPLC 等。

请注意带宽跟延迟是两个独立的指标，两者并不关联：
1. 可能带宽很高但是延迟也很高，这种情况下你下载速度很快但是打开网页的时候却很慢，可能是是中转节点没有 BGP 加速，但出海线路带宽很充足。
2. 可能带宽很低但是延迟也很低，这种情况下你打开网页的时候很快但是下载速度很慢，可能是中转节点有 BGP 加速，但出海线路的 IEPL、IPLC 带宽很小。

## License

[GPL-3.0](LICENSE)
