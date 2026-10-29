//go:build vivahead

package main

// 只在 -tags vivahead 构建里导入；发布产物不含 headless 协议。
import _ "github.com/vivaui/viva/headless/auto"
