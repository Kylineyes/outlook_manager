//go:build windows

// Package secure 提供本地敏感数据的加解密能力。
//
// Windows 平台使用 DPAPI（Data Protection API）：加密结果绑定当前
// Windows 用户，其他用户或复制到其他机器后都无法直接解密。
package secure

import (
	"fmt"

	"github.com/billgraziano/dpapi"
)

// Encrypt 使用当前 Windows 用户的 DPAPI 加密明文。
func Encrypt(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, nil
	}
	encrypted, err := dpapi.EncryptBytes(plain)
	if err != nil {
		return nil, fmt.Errorf("DPAPI 加密失败: %w", err)
	}
	return encrypted, nil
}

// Decrypt 使用当前 Windows 用户的 DPAPI 解密密文。
func Decrypt(encrypted []byte) ([]byte, error) {
	if len(encrypted) == 0 {
		return nil, nil
	}
	plain, err := dpapi.DecryptBytes(encrypted)
	if err != nil {
		return nil, fmt.Errorf("DPAPI 解密失败: %w", err)
	}
	return plain, nil
}
