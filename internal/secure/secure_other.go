//go:build !windows && !linux && !darwin

// Package secure 在未支持的系统上返回明确错误。
//
// Windows 使用 DPAPI，Linux 使用 Secret Service，macOS 使用 Keychain。
// 其他平台暂未实现安全存储后端。
package secure

import "errors"

// ErrUnsupportedPlatform 表示当前平台尚未实现安全存储后端。
var ErrUnsupportedPlatform = errors.New("当前平台暂不支持安全存储")

// Encrypt 在未支持平台直接返回不支持错误。
func Encrypt(plain []byte) ([]byte, error) {
	return nil, ErrUnsupportedPlatform
}

// Decrypt 在未支持平台直接返回不支持错误。
func Decrypt(encrypted []byte) ([]byte, error) {
	return nil, ErrUnsupportedPlatform
}

// Check 在未支持平台返回明确错误。
func Check() error {
	return ErrUnsupportedPlatform
}

// Backend 返回安全存储后端名称。
func Backend() string {
	return "unsupported"
}
