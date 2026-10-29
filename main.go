package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/gui"
	"github.com/metacubex/mihomo/constant"
	"github.com/vivaui/viva/app"
)

// 版本信息由发布流水线用 ldflags 注入；本地构建保持 dev / unknown。
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	baseDir, err := programDir()
	if err != nil {
		log.Fatalf("无法确定程序目录：%v", err)
	}
	// 内核组件（proxy-provider 缓存等）也写程序目录下的 data/，不写用户主目录。
	mihomoHome := filepath.Join(baseDir, "data", "mihomo")
	if mkdirErr := os.MkdirAll(mihomoHome, 0o755); mkdirErr == nil {
		constant.SetHomeDir(mihomoHome)
	}
	application, err := gui.New(gui.Options{BaseDir: baseDir, Version: version})
	if err != nil {
		log.Fatalf("启动失败：%v", err)
	}
	defer application.Close()
	if err := app.RunCanvas(application.Config(), nil); err != nil {
		log.Fatalf("窗口运行失败：%v", err)
	}
}

// programDir 是程序文件所在目录：所有持久化数据都放在它下面的 data/，
// 整个目录拷走即可迁移（见 docs/specs/0001-viva-gui.md 第 4 节）。
func programDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}
	return filepath.Dir(executable), nil
}
