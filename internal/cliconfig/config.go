// Package cliconfig 管理 CLI 的用户偏好设置。
package cliconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

// Settings 表示 CLI 偏好设置。
type Settings struct {
	Output          string `json:"output"`
	Color           string `json:"color"`
	AutoUpdateCheck bool   `json:"autoUpdateCheck"`
}

// Default 返回默认设置。
func Default() Settings {
	return Settings{Output: "text", Color: "auto"}
}

// Path 返回 CLI 配置文件路径。
func Path() (string, error) {
	dir, err := store.EnsureBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cli-config.json"), nil
}

// Load 读取 CLI 配置。
func Load() Settings {
	settings := Default()
	path, err := Path()
	if err != nil {
		return settings
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return settings
	}
	var file Settings
	if err := json.Unmarshal(data, &file); err != nil {
		return settings
	}
	if file.Output != "" {
		settings.Output = file.Output
	}
	if file.Color != "" {
		settings.Color = file.Color
	}
	settings.AutoUpdateCheck = file.AutoUpdateCheck
	return settings
}

// Save 保存 CLI 配置。
func Save(settings Settings) error {
	if settings.Output == "" {
		settings.Output = "text"
	}
	if settings.Color == "" {
		settings.Color = "auto"
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 CLI 配置失败: %w", err)
	}
	path, err := Path()
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(path, data, 0o600)
}
