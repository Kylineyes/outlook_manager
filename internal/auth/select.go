package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

// SelectAccount 根据用户指定值或默认账号选择一个本地已登录账号。
//
// 选择顺序：
//  1. 如果 target 非空，按用户名或 HomeAccountID 查找；
//  2. 否则读取 accounts.json 的默认账号；
//  3. 默认账号缺失时使用列表中的第一个账号。
func SelectAccount(accounts []Account, target string) (Account, error) {
	if len(accounts) == 0 {
		return Account{}, errors.New("本地没有已登录账号，请先运行 outlookctl auth login")
	}

	if target != "" {
		for _, account := range accounts {
			if strings.EqualFold(account.Username, target) || account.HomeAccountID == target {
				return account, nil
			}
		}
		return Account{}, fmt.Errorf("找不到账号: %s", target)
	}

	accountsFile, err := store.LoadAccounts()
	if err == nil && accountsFile.DefaultAccount != "" {
		for _, account := range accounts {
			if strings.EqualFold(account.Username, accountsFile.DefaultAccount) {
				return account, nil
			}
		}
	}
	return accounts[0], nil
}
