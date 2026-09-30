package picker

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrOutputPathDirectory 是输出路径以 / 或 \ 结尾（目录意图）的校验错误，
// 文案就是原因本身：界面状态行把它接在「输出路径」不合法之后，命令行
// 把它接在「输出路径不合法」之后。
var ErrOutputPathDirectory = errors.New("以 / 或 \\ 结尾是目录意图，请写到具体文件名")

// ValidateOutputPath 是输出路径落定前的统一校验口径，选源界面与命令行
// -o 共用：先去首尾空白，剩下的为空视同留空（不输出、不补全），返回
// 空值；以 / 或 \ 结尾是目录意图，报 ErrOutputPathDirectory（见
// GLOSSARY.md「后缀补全」词条）。其余原样返回，后缀补全由调用方接
// CompleteYAMLSuffix 做。
func ValidateOutputPath(path string) (string, error) {
	value := strings.TrimSpace(path)
	if value == "" {
		return "", nil
	}
	if strings.HasSuffix(value, "/") || strings.HasSuffix(value, `\`) {
		return "", ErrOutputPathDirectory
	}
	return value, nil
}

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

// CompleteYAMLSuffix 按「后缀补全」规则补全输出路径最后一段文件名的后缀：
// 以 .yaml / .yml 结尾（不分大小写）原样保留；词干非空且以点结尾时直接接
// yaml（result. → result.yaml）；其余一律补 .yaml（result.txt →
// result.txt.yaml）；词干为空（.yaml、.yml、裸点）原样保留。目录段里的点
// 不影响判定（./v1.2/out → ./v1.2/out.yaml）。空路径原样返回，表示不输出。
// 补全恒等于原路径接 MissingYAMLSuffix 的返回值。
func CompleteYAMLSuffix(path string) string {
	return path + missingYAMLSuffix(path)
}

// MissingYAMLSuffix 返回输出路径最后一段文件名还缺的后缀段：
// result 缺 .yaml，result. 缺 yaml，已写全或词干为空（含空路径）返回空串。
// 供选源界面把缺的段灰显在光标后（见 GLOSSARY.md「后缀补全」词条）。
func MissingYAMLSuffix(path string) string {
	return missingYAMLSuffix(path)
}

// missingYAMLSuffix 是补全与灰显共用的判定核心，只看最后一段文件名。
func missingYAMLSuffix(path string) string {
	base := lastPathSegment(path)
	ext := filepath.Ext(base)
	switch {
	case strings.EqualFold(ext, ".yaml") || strings.EqualFold(ext, ".yml"):
		// 后缀已写全，与合格配置识别的 EqualFold 口径一致。
		return ""
	case ext == "." && strings.TrimSuffix(base, ".") != "":
		// 词干非空、以点结尾：半截扩展名，直接接 yaml。
		return "yaml"
	case base == "" || base == ".":
		// 空路径，或最后一段为空（以分隔符结尾的目录意图）：不补，由上层校验报错。
		return ""
	default:
		return ".yaml"
	}
}

// lastPathSegment 取路径最后一段文件名，/ 和 \ 都算分隔符，
// 让补全规则在 Linux 与 Windows 上口径一致。
func lastPathSegment(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}
