//go:build !windows

package update

import (
	"fmt"
	"io"
	"os"
)

func replaceBinary(currentPath, newPath, backupPath string) error {
	if backupPath != "" {
		if err := copyFile(currentPath, backupPath, 0o600); err != nil {
			return fmt.Errorf("备份当前二进制失败: %w", err)
		}
	}
	tempPath := currentPath + ".new"
	if err := copyFile(newPath, tempPath, 0o755); err != nil {
		return fmt.Errorf("准备新二进制失败: %w", err)
	}
	if err := os.Rename(tempPath, currentPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("替换当前二进制失败: %w", err)
	}
	return nil
}

func copyFile(source, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	return os.Chmod(target, mode)
}
