package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// openDirectory 是打开目录的接缝：生产代码用 openDirectoryPath，测试替换它
// 以免真的拉起文件管理器。
var openDirectory = openDirectoryPath

// openDirectoryPath 用系统文件管理器打开一个目录。Viva 的 app.OpenPath 只
// 接受普通文件（并会拒绝可执行类型），打开目录只能交给各平台自己的命令；
// 参数直接传给进程，不经过 shell，路径里的空格与特殊字符不会被解释。
func openDirectoryPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s 不是目录", path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("explorer.exe", absolute)
	case "darwin":
		command = exec.Command("open", absolute)
	default:
		command = exec.Command("xdg-open", absolute)
	}
	if err := command.Start(); err != nil {
		return err
	}
	// 父进程不等文件管理器退出：explorer 会一直挂着，等它等于卡住界面。
	_ = command.Process.Release()
	return nil
}
