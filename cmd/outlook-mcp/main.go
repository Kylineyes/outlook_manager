// Package main 是 outlook-mcp MCP server 的入口。
//
// 当前阶段只实现最小可执行骨架，用于验证 Windows amd64 构建流水线。
// 后续会在这里初始化 MCP server，并把 internal/graph 中的能力注册成 tools。
package main

import (
	"fmt"
	"os"
)

// version 表示当前构建版本。
// 默认值用于本地开发；正式发布时可由 -ldflags 注入。
var version = "dev"

// main 输出最小启动信息后退出。
// 当前不会启动 MCP 监听；真正的 stdio/HTTP 服务将在接入 MCP SDK 后实现。
func main() {
	fmt.Fprintf(os.Stdout, "outlook-mcp %s\n", version)
	fmt.Fprintln(os.Stdout, "当前为项目骨架，MCP server 尚未实现。")
}
