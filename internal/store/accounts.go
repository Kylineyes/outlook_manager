package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// AccountsVersion 是 accounts.json 的结构版本号。
// 后续如果字段发生不兼容变化，可以通过该版本号做迁移。
const AccountsVersion = 1

// AccountRecord 是可以直接阅读的账号档案。
// 该结构不会保存 token；token 只存在于加密的 token-cache.bin 中。
type AccountRecord struct {
	Username             string   `json:"username"`
	HomeAccountID        string   `json:"homeAccountId"`
	Environment          string   `json:"environment,omitempty"`
	TenantID             string   `json:"tenantId,omitempty"`
	DisplayName          string   `json:"displayName,omitempty"`
	Scopes               []string `json:"scopes,omitempty"`
	LastLoginAt          string   `json:"lastLoginAt,omitempty"`
	AccessTokenExpiresAt string   `json:"accessTokenExpiresAt,omitempty"`
	RefreshTokenCached   bool     `json:"refreshTokenCached"`
}

// AccountsFile 是 accounts.json 的顶层结构。
type AccountsFile struct {
	Version        int             `json:"version"`
	DefaultAccount string          `json:"defaultAccount,omitempty"`
	Accounts       []AccountRecord `json:"accounts"`
}

// LoadAccounts 读取 accounts.json。
// 文件不存在时返回一个空的、版本正确的结构，而不是错误。
func LoadAccounts() (AccountsFile, error) {
	path, err := AccountsPath()
	if err != nil {
		return AccountsFile{}, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return AccountsFile{Version: AccountsVersion, Accounts: []AccountRecord{}}, nil
	}
	if err != nil {
		return AccountsFile{}, fmt.Errorf("读取账号档案失败 (%s): %w", path, err)
	}

	var af AccountsFile
	if err := json.Unmarshal(data, &af); err != nil {
		return AccountsFile{}, fmt.Errorf("解析账号档案失败 (%s): %w", path, err)
	}
	if af.Version == 0 {
		af.Version = AccountsVersion
	}
	if af.Accounts == nil {
		af.Accounts = []AccountRecord{}
	}
	return af, nil
}

// SaveAccounts 将账号档案写入 accounts.json。
func SaveAccounts(af AccountsFile) error {
	if af.Version == 0 {
		af.Version = AccountsVersion
	}
	if af.Accounts == nil {
		af.Accounts = []AccountRecord{}
	}
	path, err := AccountsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(af, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化账号档案失败: %w", err)
	}
	data = append(data, '\n')
	if err := WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("写入账号档案失败 (%s): %w", path, err)
	}
	return nil
}

// UpsertAccount 新增或更新一个账号，并保存到 accounts.json。
//
// 匹配顺序：
//  1. HomeAccountID 完全相同。
//  2. Username 忽略大小写后相同。
//
// 如果当前还没有默认账号，会自动把新账号设为默认账号。
func UpsertAccount(record AccountRecord) (AccountsFile, error) {
	af, err := LoadAccounts()
	if err != nil {
		return AccountsFile{}, err
	}

	index := -1
	for i := range af.Accounts {
		sameHomeID := record.HomeAccountID != "" && af.Accounts[i].HomeAccountID == record.HomeAccountID
		sameUsername := record.Username != "" && strings.EqualFold(af.Accounts[i].Username, record.Username)
		if sameHomeID || sameUsername {
			index = i
			break
		}
	}

	if index >= 0 {
		af.Accounts[index] = record
	} else {
		af.Accounts = append(af.Accounts, record)
	}
	if af.DefaultAccount == "" {
		af.DefaultAccount = record.Username
	}
	if err := SaveAccounts(af); err != nil {
		return AccountsFile{}, err
	}
	return af, nil
}
