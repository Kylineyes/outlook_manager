//go:build linux || darwin

// Package secure 提供跨平台本地敏感数据加解密能力。
//
// Linux 使用 Secret Service（libsecret/DBus），macOS 使用 Keychain。
// 实际数据使用 AES-256-GCM 加密，随机数据密钥保存在系统钥匙串中。
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "outlook-manager"
	keyringUser    = "token-cache-key"
	keySize        = 32
	magic          = "OMK1"
)

var (
	keyringGet = keyring.Get
	keyringSet = keyring.Set
)

// Encrypt 使用系统钥匙串中的 AES-256 密钥加密明文。
func Encrypt(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, nil
	}
	key, err := loadOrCreateKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("初始化 AES 失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("初始化 AES-GCM 失败: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成随机 nonce 失败: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	result := make([]byte, 0, len(magic)+len(nonce)+len(ciphertext))
	result = append(result, []byte(magic)...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

// Decrypt 使用系统钥匙串中的 AES-256 密钥解密密文。
func Decrypt(encrypted []byte) ([]byte, error) {
	if len(encrypted) == 0 {
		return nil, nil
	}
	if len(encrypted) < len(magic) || string(encrypted[:len(magic)]) != magic {
		return nil, errors.New("不支持的加密数据格式")
	}
	key, err := loadOrCreateKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("初始化 AES 失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("初始化 AES-GCM 失败: %w", err)
	}
	payload := encrypted[len(magic):]
	if len(payload) < gcm.NonceSize() {
		return nil, errors.New("加密数据长度不足")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("AES-GCM 解密失败: %w", err)
	}
	return plain, nil
}

func loadOrCreateKey() ([]byte, error) {
	encoded, err := keyringGet(keyringService, keyringUser)
	if err == nil {
		key, decodeErr := base64.RawStdEncoding.DecodeString(encoded)
		if decodeErr != nil || len(key) != keySize {
			return nil, errors.New("系统钥匙串中的密钥格式无效")
		}
		return key, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		return nil, fmt.Errorf("读取系统钥匙串失败: %w", err)
	}
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("生成随机密钥失败: %w", err)
	}
	encoded = base64.RawStdEncoding.EncodeToString(key)
	if err := keyringSet(keyringService, keyringUser, encoded); err != nil {
		return nil, fmt.Errorf("写入系统钥匙串失败: %w", err)
	}
	return key, nil
}

// Check 验证系统钥匙串和 AES-GCM 加解密是否可用。
func Check() error {
	encrypted, err := Encrypt([]byte("outlook-manager-doctor"))
	if err != nil {
		return err
	}
	plain, err := Decrypt(encrypted)
	if err != nil {
		return err
	}
	if string(plain) != "outlook-manager-doctor" {
		return fmt.Errorf("系统钥匙串自检结果不一致")
	}
	return nil
}

// Backend 返回安全存储后端名称。
func Backend() string {
	return "系统钥匙串 (Secret Service / Keychain)"
}
