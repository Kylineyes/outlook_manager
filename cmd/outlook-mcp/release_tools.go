package main

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Kylineyes/outlook-manager/internal/update"
)

func registerReleaseTools(server *mcp.Server) {
	addTool(server, newTool("update_check", "检查 GitHub 最新发布版本"), updateCheckHandler)
}

func updateCheckHandler(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := update.Check(ctx, version)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "update": result})
}
