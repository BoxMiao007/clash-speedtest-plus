package picker

import (
	"path/filepath"
	"strings"
)

// ResolveOutputPath 把输出路径锚到程序目录。
// 空路径保持为空，表示不写文件。绝对路径不动。相对路径含 ../ 时按程序目录解析。
func ResolveOutputPath(executableDir, outputPath string) string {
	if strings.TrimSpace(outputPath) == "" {
		return ""
	}
	if filepath.IsAbs(outputPath) {
		return outputPath
	}
	return filepath.Clean(filepath.Join(executableDir, outputPath))
}
