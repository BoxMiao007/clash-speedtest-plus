package gui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
	"gopkg.in/yaml.v2"
)

// nodeIdentity 是节点的稳定身份：协议 + 完整配置摘要。配置用 yaml.v2 序列化：
// 它会把 map 键排序，且能处理 yaml 解析出的 map[interface{}]interface{} 嵌套值；
// 因此身份不依赖节点在列表里的顺序，也不依赖运行时指针。
func nodeIdentity(proxy *speedtester.CProxy) string {
	if proxy == nil || proxy.Proxy == nil {
		return "invalid"
	}
	sum := sha256.New()
	sum.Write([]byte(proxy.Type().String()))
	sum.Write([]byte{0})
	if proxy.Config != nil {
		if data, err := yaml.Marshal(proxy.Config); err == nil {
			sum.Write(data)
		}
	}
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// formatSpeed 把字节每秒变成界面文案；非正值显示破折号，避免把「没测到」写成 0。
func formatSpeed(bytesPerSecond float64) string {
	if bytesPerSecond <= 0 {
		return "—"
	}
	return speedtester.FormatSpeed(bytesPerSecond)
}

func formatLatency(value time.Duration) string {
	if value <= 0 {
		return "—"
	}
	return fmt.Sprintf("%dms", value.Milliseconds())
}

func formatLoss(value float64) string {
	if value < 0 {
		value = 0
	}
	return fmt.Sprintf("%.1f%%", value)
}

// formatTested 是「上次测速时间」列；零值明确写「未测试」。
func formatTested(at time.Time) string {
	if at.IsZero() {
		return "未测试"
	}
	return "上次 " + at.Local().Format("01-02 15:04")
}

func formatServer(proxy *speedtester.CProxy) string {
	if proxy == nil || proxy.Config == nil {
		return "—"
	}
	server := fmt.Sprintf("%v", proxy.Config["server"])
	port := fmt.Sprintf("%v", proxy.Config["port"])
	if server == "" || server == "<nil>" {
		return "—"
	}
	if port == "" || port == "<nil>" {
		return server
	}
	return server + ":" + port
}

func formatPathKind(stored string) string {
	if stored == "" {
		return "未设置路径"
	}
	if filepath.IsAbs(stored) {
		return "硬路径 · " + stored
	}
	return "软路径 · " + stored
}

// clipboardText 预计算节点信息，复制动作只读字段，不在输入路径拼字符串。
func clipboardText(n *nodeState) string {
	lines := []string{
		"名称：" + n.name,
		"条目：" + n.entry.entry.Name,
		"类型：" + n.typ,
		"服务器：" + n.serverText,
	}
	if n.hasResult {
		lines = append(lines,
			"上次测速："+n.result.TestedAt.Local().Format("2006-01-02 15:04:05"),
			"延迟："+n.latencyText,
			"抖动："+n.jitterText,
			"丢包："+n.lossText,
			"下载速度："+n.speedText,
			"上传速度："+n.uploadText,
		)
		if n.result.DownloadError != "" {
			lines = append(lines, "下载错误："+n.result.DownloadError)
		}
		if n.result.UploadError != "" {
			lines = append(lines, "上传错误："+n.result.UploadError)
		}
	} else {
		lines = append(lines, "上次测速：未测试")
	}
	return joinLines(lines)
}

func joinLines(lines []string) string {
	total := 0
	for _, line := range lines {
		total += len(line) + 1
	}
	buf := make([]byte, 0, total)
	for index, line := range lines {
		if index > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, line...)
	}
	return string(buf)
}

// resultOK 把一次测速结果折算成「上次是否失败」，供「排除上次失败」使用。
func resultOK(result *speedtester.Result) bool {
	if result == nil {
		return false
	}
	if result.Latency <= 0 {
		return false
	}
	if result.DownloadError != "" {
		return false
	}
	return result.DownloadSpeed > 0
}

func resultToStored(result *speedtester.Result, testedAt time.Time) nodeResult {
	return nodeResult{
		LatencyMS:     float64(result.Latency.Milliseconds()),
		JitterMS:      float64(result.Jitter.Milliseconds()),
		PacketLoss:    result.PacketLoss,
		DownloadBps:   result.DownloadSpeed,
		UploadBps:     result.UploadSpeed,
		DownloadError: result.DownloadError,
		UploadError:   result.UploadError,
		TestedAt:      testedAt,
		OK:            resultOK(result),
	}
}

// versionBadge 与旧界面保持同一规则：注入值补 v，本地 dev 原样，空串不显示。
func versionBadge(version string) string {
	if version == "" || version == "dev" {
		return version
	}
	if len(version) > 0 && version[0] == 'v' {
		return version
	}
	return "v" + version
}
