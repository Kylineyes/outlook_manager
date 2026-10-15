// Package store 负责管理本工具的本地数据目录、配置文件和账号档案。
//
// 本包只处理可读的元数据与文件路径；真正的 token 内容由 internal/secure
// 和 internal/auth 负责加密与读写。
package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// appDirName 是工具在用户主目录下使用的数据目录名称。
const appDirName = ".outlook-manager"

// BaseDir 返回本地数据目录。
// 默认路径为 %USERPROFILE%\.outlook-manager；如果无法获取用户主目录，
// 会返回错误，避免把敏感数据写到不可预期的位置。
func BaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录失败: %w", err)
	}
	return filepath.Join(home, appDirName), nil
}

// EnsureBaseDir 确保基础目录存在，并返回目录路径。
// 目录权限使用 0700，尽量限制其他系统用户访问。
func EnsureBaseDir() (string, error) {
	dir, err := BaseDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("创建本地数据目录失败 (%s): %w", dir, err)
	}
	return dir, nil
}

// ConfigPath 返回 config.json 的完整路径。
func ConfigPath() (string, error) {
	dir, err := EnsureBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// AccountsPath 返回 accounts.json 的完整路径。
func AccountsPath() (string, error) {
	dir, err := EnsureBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "accounts.json"), nil
}

// TokenCachePath 返回 token-cache.bin 的完整路径。
// 该文件保存加密后的 MSAL token cache，不能直接以明文形式读取。
func TokenCachePath() (string, error) {
	dir, err := EnsureBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "token-cache.bin"), nil
}
