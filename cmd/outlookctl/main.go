// Package main 是 outlookctl 命令行工具的入口。
//
// 当前阶段只实现最小可执行骨架，用于验证：
//  1. Go module 初始化正确。
//  2. Windows x86 交叉编译流水线可用。
//  3. 后续 auth、account、mail 等子命令有统一挂载位置。
//
// 该文件暂时不包含业务逻辑，避免在认证与数据存储方案落地前
// 过早固化命令结构。
package main

import (
	"fmt"
	"os"
)

// version 表示当前构建版本。
// 默认值用于本地开发；正式发布时可由构建脚本通过 -ldflags 注入。
var version = "dev"

// main 输出最小启动信息后退出。
// 后续会在这里接入命令行解析库，并按子命令分发到 internal 包。
func main() {
	fmt.Fprintf(os.Stdout, "outlookctl %s\n", version)
	fmt.Fprintln(os.Stdout, "当前为项目骨架，auth、account、mail 等命令尚未实现。")
}
