package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

// DefaultStartupCheckInterval 是启动更新检查的最小间隔。
const DefaultStartupCheckInterval = 24 * time.Hour

type checkState struct {
	LastChecked time.Time `json:"lastChecked"`
	Latest      string    `json:"latest,omitempty"`
	ReleaseURL  string    `json:"releaseUrl,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
}

// CheckCachePath 返回启动更新检查缓存路径。
func CheckCachePath() (string, error) {
	dir, err := store.EnsureBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "update-check.json"), nil
}

// StartupCheck 使用本地缓存执行每日启动检查。
func StartupCheck(ctx context.Context, current string, interval time.Duration) (*Result, error) {
	if interval <= 0 {
		interval = DefaultStartupCheckInterval
	}
	state, _ := loadCheckState()
	now := time.Now().UTC()
	if !state.LastChecked.IsZero() && now.Sub(state.LastChecked) < interval {
		if state.LastError != "" {
			return nil, fmt.Errorf("上次更新检查失败: %s", state.LastError)
		}
		if state.Latest == "" {
			return nil, nil
		}
		return cachedResult(current, state), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := Check(ctx, current)
	state.LastChecked = now
	if err != nil {
		state.LastError = err.Error()
		_ = saveCheckState(state)
		return nil, err
	}
	state.Latest = result.Latest
	state.LastError = ""
	if result.Release != nil {
		state.ReleaseURL = result.Release.HTMLURL
	}
	_ = saveCheckState(state)
	return result, nil
}

func cachedResult(current string, state checkState) *Result {
	result := &Result{Current: current, Latest: state.Latest, Release: &Release{TagName: state.Latest, HTMLURL: state.ReleaseURL}}
	currentNormalized := normalizeVersion(current)
	latestNormalized := normalizeVersion(state.Latest)
	if currentNormalized == "" || latestNormalized == "" {
		result.IsDevelopment = true
		return result
	}
	result.UpdateAvailable = compareVersion(latestNormalized, currentNormalized) > 0
	return result
}

func loadCheckState() (checkState, error) {
	path, err := CheckCachePath()
	if err != nil {
		return checkState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return checkState{}, err
	}
	var state checkState
	if err := json.Unmarshal(data, &state); err != nil {
		return checkState{}, err
	}
	return state, nil
}

func saveCheckState(state checkState) error {
	path, err := CheckCachePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(path, data, 0o600)
}
