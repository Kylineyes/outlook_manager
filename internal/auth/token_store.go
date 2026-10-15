package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/secure"
	"github.com/Kylineyes/outlook-manager/internal/store"
)

// tokenCacheVersion 是加密 token 缓存的结构版本号。
const tokenCacheVersion = 1

// cachedAccount 是真正保存 access token 和 refresh token 的结构。
// 整个结构会被序列化、DPAPI 加密后写入 token-cache.bin。
type cachedAccount struct {
	Username      string    `json:"username"`
	HomeAccountID string    `json:"homeAccountId"`
	Environment   string    `json:"environment,omitempty"`
	TenantID      string    `json:"tenantId,omitempty"`
	DisplayName   string    `json:"displayName,omitempty"`
	Scopes        []string  `json:"scopes,omitempty"`
	AccessToken   string    `json:"accessToken"`
	RefreshToken  string    `json:"refreshToken,omitempty"`
	IDToken       string    `json:"idToken,omitempty"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

// tokenCacheFile 是 token 缓存文件的顶层结构。
type tokenCacheFile struct {
	Version  int             `json:"version"`
	Accounts []cachedAccount `json:"accounts"`
}

// Account 是认证模块对外暴露的账号元数据，不包含 token。
type Account struct {
	Username           string
	HomeAccountID      string
	Environment        string
	TenantID           string
	DisplayName        string
	Scopes             []string
	ExpiresAt          time.Time
	RefreshTokenCached bool
}

// LoginResult 是一次登录或静默刷新后的结果。
// AccessToken 只保留在内存中，供调用方立即访问 Graph；不会写进 accounts.json。
type LoginResult struct {
	Account     Account
	AccessToken string
}

// loadTokenCache 读取并解密 token-cache.bin。
// 文件不存在时返回空缓存，表示当前还没有账号登录过。
func loadTokenCache() (*tokenCacheFile, error) {
	path, err := store.TokenCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &tokenCacheFile{Version: tokenCacheVersion, Accounts: []cachedAccount{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 token 缓存失败 (%s): %w", path, err)
	}

	plain, err := secure.Decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("解密 token 缓存失败 (%s): %w", path, err)
	}

	var cache tokenCacheFile
	if err := json.Unmarshal(plain, &cache); err != nil {
		return nil, fmt.Errorf("解析 token 缓存失败 (%s): %w", path, err)
	}
	if cache.Version == 0 {
		cache.Version = tokenCacheVersion
	}
	if cache.Accounts == nil {
		cache.Accounts = []cachedAccount{}
	}
	return &cache, nil
}

// saveTokenCache 序列化、加密并写入 token-cache.bin。
func saveTokenCache(cache *tokenCacheFile) error {
	if cache.Version == 0 {
		cache.Version = tokenCacheVersion
	}
	if cache.Accounts == nil {
		cache.Accounts = []cachedAccount{}
	}

	path, err := store.TokenCachePath()
	if err != nil {
		return err
	}
	plain, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 token 缓存失败: %w", err)
	}
	encrypted, err := secure.Encrypt(plain)
	if err != nil {
		return fmt.Errorf("加密 token 缓存失败: %w", err)
	}
	if err := store.WriteFileAtomic(path, encrypted, 0o600); err != nil {
		return fmt.Errorf("写入 token 缓存失败 (%s): %w", path, err)
	}
	return nil
}

// upsertCachedAccount 在缓存中新增或替换账号。
// 匹配顺序与 accounts.json 保持一致：HomeAccountID 优先，其次 Username。
func upsertCachedAccount(cache *tokenCacheFile, account cachedAccount) {
	for i := range cache.Accounts {
		sameHomeID := account.HomeAccountID != "" && cache.Accounts[i].HomeAccountID == account.HomeAccountID
		sameUsername := account.Username != "" && strings.EqualFold(cache.Accounts[i].Username, account.Username)
		if sameHomeID || sameUsername {
			cache.Accounts[i] = account
			return
		}
	}
	cache.Accounts = append(cache.Accounts, account)
}

// findCachedAccount 按 HomeAccountID 或 Username 查找缓存账号。
func findCachedAccount(cache *tokenCacheFile, target Account) int {
	for i := range cache.Accounts {
		sameHomeID := target.HomeAccountID != "" && cache.Accounts[i].HomeAccountID == target.HomeAccountID
		sameUsername := target.Username != "" && strings.EqualFold(cache.Accounts[i].Username, target.Username)
		if sameHomeID || sameUsername {
			return i
		}
	}
	return -1
}

// toAccount 把内部缓存结构转换为不包含 token 的外部账号结构。
func (a cachedAccount) toAccount() Account {
	return Account{
		Username:           a.Username,
		HomeAccountID:      a.HomeAccountID,
		Environment:        a.Environment,
		TenantID:           a.TenantID,
		DisplayName:        a.DisplayName,
		Scopes:             append([]string(nil), a.Scopes...),
		ExpiresAt:          a.ExpiresAt,
		RefreshTokenCached: a.RefreshToken != "",
	}
}
