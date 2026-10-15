package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic 以尽量安全的方式写入文件。
//
// 实现方式是先在同目录创建临时文件，写入并同步后再替换目标文件。
// Windows 下 os.Rename 不能保证覆盖已有文件，因此这里先删除旧文件，
// 再执行重命名。该实现不是严格的跨平台原子操作，但可以避免目标文件
// 出现只写入一半内容的情况。
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建目标目录失败 (%s): %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("同步临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}

	if err := os.Chmod(tmpName, perm); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("设置临时文件权限失败: %w", err)
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmpName)
		return fmt.Errorf("移除旧文件失败: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("替换目标文件失败: %w", err)
	}
	return nil
}
