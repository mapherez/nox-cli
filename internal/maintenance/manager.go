// Package maintenance coordinates public updates independently of Suite credentials.
package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mapherez/nox-cli/internal/config"
	"github.com/mapherez/nox-cli/internal/install"
	"github.com/mapherez/nox-cli/internal/releases"
)

type Source interface {
	Latest(context.Context) (releases.Release, error)
	Download(context.Context, string, string) (string, error)
}
type Manager struct {
	Root, Version string
	Source        Source
	mu            sync.Mutex
}
type Check struct {
	Product   string    `json:"product"`
	CheckedAt time.Time `json:"checkedAt"`
	Latest    string    `json:"latest,omitempty"`
}

func New(root, version string) *Manager {
	return &Manager{Root: root, Version: version, Source: releases.New()}
}
func (m *Manager) Check(ctx context.Context, force bool) (Check, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path := filepath.Join(m.Root, "update-check.json")
	var cached Check
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Size() <= 4096 {
		raw, _ := os.ReadFile(path)
		_ = json.Unmarshal(raw, &cached)
	}
	now := time.Now().UTC()
	if !force && cached.Product == "nox-cli" && !cached.CheckedAt.After(now) && now.Sub(cached.CheckedAt) < 24*time.Hour {
		return cached, nil
	}
	release, err := m.Source.Latest(ctx)
	result := Check{Product: "nox-cli", CheckedAt: now}
	if cached.Product == "nox-cli" && releases.Version(cached.Latest) {
		result.Latest = cached.Latest
	}
	if err == nil {
		result.Latest = release.Version
	}
	if (config.Store{Root: m.Root}).Directory() == nil {
		raw, _ := json.Marshal(result)
		_ = config.WriteFile(path, append(raw, '\n'), 0600)
	}
	return result, err
}

// Update never accesses Gateway settings, tool arguments or the credential store.
func (m *Manager) Update(ctx context.Context, version string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !releases.Newer(version, m.Version) || !install.Managed(m.Root) {
		return false, install.ErrManaged
	}
	// Re-fetch metadata before preparing an explicitly requested update.
	release, err := m.Source.Latest(ctx)
	if err != nil {
		return false, err
	}
	if release.Version != version {
		return false, errors.New("The release changed. Check for updates and confirm the new version.")
	}
	stage, finish, err := install.Stage(m.Root)
	if err != nil {
		return false, err
	}
	keep := false
	defer func() { finish(keep) }()
	if err := install.MarkPending(m.Root); err != nil {
		return false, err
	}
	source, err := m.Source.Download(ctx, version, stage)
	if err != nil {
		return false, err
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	probe := exec.CommandContext(ctx, source, "--version")
	output, err := probe.Output()
	if err != nil || !strings.HasPrefix(string(output), "nox "+version+" (commit ") {
		return false, releases.ErrRelease
	}
	if runtime.GOOS == "windows" {
		if err := install.HandoffWindows(m.Root, source); err != nil {
			return false, err
		}
		keep = true
		return true, nil
	}
	if err := install.Apply(ctx, m.Root, source, version); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) Result() string {
	path := filepath.Join(m.Root, "update-result.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var result struct {
		Product string `json:"product"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Product != "nox-cli" {
		return ""
	}
	return result.Message
}
