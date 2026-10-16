package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Config 保存可公开的认证配置。
// 这里不保存 client secret、密码或 token；个人 Outlook.com 账号使用
// Device Code 公共客户端流程，只需要 ClientID、Authority 和 Scopes。
type Config struct {
	ClientID  string   `json:"clientId"`
	Authority string   `json:"authority"`
	Scopes    []string `json:"scopes"`
}

const (
	// DefaultClientID 是当前项目在 Azure 注册的公共客户端 ID。
	// 该值不是密钥，可以放在配置文件中。
	DefaultClientID = "af1d10a2-657d-4859-bcd4-bbf37836c68a"

	// DefaultAuthority 是个人 Microsoft 账号使用的认证端点别名。
	// 个人账号没有独立租户，必须使用 consumers 而不是 Azure 门户里的目录 ID。
	DefaultAuthority = "https://login.microsoftonline.com/consumers"
)

// defaultScopes 是第一阶段需要的最小权限集合。
// offline_access 用于获取 refresh token，确保登录状态可以本地持久化。
var defaultScopes = []string{"User.Read", "Mail.Read", "Mail.ReadWrite", "Mail.Send", "Calendars.Read", "Calendars.ReadWrite", "Contacts.Read", "Contacts.ReadWrite", "offline_access"}

// DefaultConfig 返回项目默认认证配置。
func DefaultConfig() Config {
	return Config{
		ClientID:  DefaultClientID,
		Authority: DefaultAuthority,
		Scopes:    append([]string(nil), defaultScopes...),
	}
}

// Validate 检查配置是否满足认证所需的最低条件。
func (c Config) Validate() error {
	if c.ClientID == "" {
		return errors.New("config.clientId 不能为空")
	}
	if c.Authority == "" {
		return errors.New("config.authority 不能为空")
	}
	if len(c.Scopes) == 0 {
		return errors.New("config.scopes 不能为空")
	}
	return nil
}

// ensureDefaultScopes 确保配置中包含所有默认 scope。
// 新增能力后，老配置文件会自动补齐，避免升级后请求不到新权限。
func ensureDefaultScopes(cfg *Config) bool {
	changed := false
	for _, scope := range defaultScopes {
		found := false
		for _, existing := range cfg.Scopes {
			if strings.EqualFold(existing, scope) {
				found = true
				break
			}
		}
		if !found {
			cfg.Scopes = append(cfg.Scopes, scope)
			changed = true
		}
	}
	return changed
}

// LoadConfig 读取本地 config.json。
//
// 如果配置文件不存在，会写入一份默认配置后再返回；如果文件存在但内容
// 非法，则直接返回错误，避免用户以为使用了默认值而实际上配置已损坏。
func LoadConfig() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg := DefaultConfig()
		if saveErr := SaveConfig(cfg); saveErr != nil {
			return Config{}, fmt.Errorf("创建默认配置失败: %w", saveErr)
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("读取配置文件失败 (%s): %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置文件失败 (%s): %w", path, err)
	}
	if ensureDefaultScopes(&cfg) {
		if err := SaveConfig(cfg); err != nil {
			return Config{}, fmt.Errorf("更新默认 scopes 失败 (%s): %w", path, err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("配置文件内容不合法 (%s): %w", path, err)
	}
	return cfg, nil
}

// SaveConfig 将配置写入 config.json。
func SaveConfig(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("拒绝保存不合法配置: %w", err)
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	data = append(data, '\n')
	if err := WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("写入配置文件失败 (%s): %w", path, err)
	}
	return nil
}
