package picker

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/faceair/clash-speedtest/speedtester"
	"gopkg.in/yaml.v2"
)

// ConfigEntry 是程序目录里的一份 .yaml。Selectable 为假时不能勾选。
type ConfigEntry struct {
	Name       string
	Path       string
	Selectable bool
	Reason     string
}

// ListConfigs 只看目录一层的 .yaml，按文件名不区分大小写排序。
// 能解开且 proxies 或 proxy-providers 至少有一项的可以勾选，其余 .yaml 列出但不可选。
func ListConfigs(dir string) ([]ConfigEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var configs []ConfigEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".yaml") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		configs = append(configs, classifyConfig(entry.Name(), path))
	}
	sort.Slice(configs, func(i, j int) bool {
		return strings.ToLower(configs[i].Name) < strings.ToLower(configs[j].Name)
	})
	return configs, nil
}

func classifyConfig(name, path string) ConfigEntry {
	body, err := os.ReadFile(path)
	if err != nil {
		return ConfigEntry{Name: name, Path: path, Reason: "无法读取"}
	}
	var probe map[string]any
	if err := yaml.Unmarshal(body, &probe); err != nil {
		return ConfigEntry{Name: name, Path: path, Reason: "不是合法的 yaml"}
	}
	if !speedtester.HasClashProxies(body) {
		return ConfigEntry{Name: name, Path: path, Reason: "没有节点"}
	}
	return ConfigEntry{Name: name, Path: path, Selectable: true}
}
