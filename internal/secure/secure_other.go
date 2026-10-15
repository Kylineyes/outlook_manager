//go:build !windows

// Package secure 在非 Windows 平台暂时只返回明确错误。
//
// 当前项目第一阶段只交付 Windows 可执行文件，因此没有为 macOS/Linux
// 实现对应的 Keychain 或 Secret Service 后端，避免误用不安全的明文存储。
package secure

import "errors"

// ErrUnsupportedPlatform 表示当前平台尚未实现安全存储后端。
var ErrUnsupportedPlatform = errors.New("当前平台暂不支持安全存储，Windows 下使用 DPAPI")

// Encrypt 在非 Windows 平台直接返回不支持错误。
func Encrypt(plain []byte) ([]byte, error) {
	return nil, ErrUnsupportedPlatform
}

// Decrypt 在非 Windows 平台直接返回不支持错误。
func Decrypt(encrypted []byte) ([]byte, error) {
	return nil, ErrUnsupportedPlatform
}
