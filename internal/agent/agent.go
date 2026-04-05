package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"helm-wails/internal/peer"
)

const (
	CWDWorkspaceRoot = "workspace-root"
	CWDProcess       = "process"
)

type AdapterConfig struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	ResumeArgs  []string          `json:"resume_args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	CWDMode     string            `json:"cwd_mode,omitempty"`
	Detect      []string          `json:"detect,omitempty"`
	Family      string            `json:"family,omitempty"`
	PeerEnabled bool              `json:"peer_enabled,omitempty"`
}

type LaunchSpec struct {
	ID          string
	Label       string
	Command     string
	Args        []string
	Env         []string
	CWD         string
	Family      string
	PeerEnabled bool
}

type Adapter interface {
	Config() AdapterConfig
	Resolve(workspace string, inheritedEnv []string) (LaunchSpec, error)
}

type Registry struct {
	adapters  map[string]StaticAdapter
	lookupEnv []string

	availableOnce sync.Once
	availableList []AdapterConfig
}

type StaticAdapter struct {
	config AdapterConfig
}

func (a StaticAdapter) Config() AdapterConfig {
	return a.config
}

func (a StaticAdapter) Resolve(workspace string, inheritedEnv []string) (LaunchSpec, error) {
	if err := validate(a.config); err != nil {
		return LaunchSpec{}, err
	}

	mergedEnv := mergeEnv(inheritedEnv, a.config.Env)
	command, err := resolveCommandPath(a.config.Command, mergedEnv)
	if err != nil {
		return LaunchSpec{}, fmt.Errorf("resolve command for %q: %w", a.config.ID, err)
	}

	cwd := workspace
	if a.config.CWDMode == CWDProcess {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return LaunchSpec{}, fmt.Errorf("resolve cwd: %w", err)
		}
	}

	return LaunchSpec{
		ID:          a.config.ID,
		Label:       a.config.Label,
		Command:     command,
		Args:        append([]string(nil), a.config.Args...),
		Env:         mergedEnv,
		CWD:         cwd,
		Family:      normalizeFamily(a.config),
		PeerEnabled: a.config.PeerEnabled,
	}, nil
}

func DefaultConfigPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "helm", "agents.json"), nil
}

func BuiltInConfigs() []AdapterConfig {
	return BuiltInConfigsWithEnv(os.Environ())
}

func BuiltInConfigsWithEnv(env []string) []AdapterConfig {
	shell := envValue(env, "SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}

	return []AdapterConfig{
		{
			ID:      "shell",
			Label:   "Shell",
			Command: shell,
			Args:    interactiveShellArgs(shell),
			Env: map[string]string{
				"TERM":      "xterm-256color",
				"COLORTERM": "truecolor",
			},
			CWDMode: CWDWorkspaceRoot,
			Family:  peer.FamilyGeneric,
		},
		{
			ID:          "codex",
			Label:       "Codex",
			Command:     "codex",
			ResumeArgs:  []string{"resume", "--last"},
			CWDMode:     CWDWorkspaceRoot,
			Family:      peer.FamilyCodex,
			PeerEnabled: true,
		},
		{
			ID:          "claude-code",
			Label:       "Claude Code",
			Command:     "claude",
			ResumeArgs:  []string{"--continue"},
			CWDMode:     CWDWorkspaceRoot,
			Family:      peer.FamilyClaude,
			PeerEnabled: true,
		},
		{
			ID:          "cursor-agent",
			Label:       "Cursor",
			Command:     "cursor-agent",
			CWDMode:     CWDWorkspaceRoot,
			Family:      peer.FamilyCursor,
			PeerEnabled: true,
		},
		{
			ID:          "gemini",
			Label:       "Gemini",
			Command:     "gemini",
			CWDMode:     CWDWorkspaceRoot,
			Family:      peer.FamilyGemini,
			PeerEnabled: true,
		},
		{
			ID:          "github-copilot",
			Label:       "GitHub Copilot",
			Command:     "copilot",
			CWDMode:     CWDWorkspaceRoot,
			Family:      peer.FamilyCopilot,
			PeerEnabled: true,
		},
		{
			ID:          "kiro",
			Label:       "Kiro",
			Command:     "kiro-cli",
			CWDMode:     CWDWorkspaceRoot,
			Family:      peer.FamilyKiro,
			PeerEnabled: true,
		},
		{
			ID:          "aider",
			Label:       "Aider",
			Command:     "aider",
			CWDMode:     CWDWorkspaceRoot,
			Family:      peer.FamilyAider,
			PeerEnabled: true,
		},
		{
			ID:          "opencode",
			Label:       "OpenCode",
			Command:     "opencode",
			CWDMode:     CWDWorkspaceRoot,
			Family:      peer.FamilyOpenCode,
			PeerEnabled: true,
		},
	}
}

func interactiveShellArgs(shell string) []string {
	switch filepath.Base(shell) {
	case "zsh", "bash", "fish":
		return []string{"-i"}
	default:
		return nil
	}
}

func LoadRegistry() (*Registry, error) {
	return LoadRegistryWithEnv(os.Environ())
}

func LoadRegistryWithEnv(lookupEnv []string) (*Registry, error) {
	configPath, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}

	custom, err := LoadConfigs(configPath)
	if err != nil {
		return nil, err
	}

	return NewRegistryWithEnv(mergeConfigs(BuiltInConfigsWithEnv(lookupEnv), custom), lookupEnv)
}

func NewRegistry(configs []AdapterConfig) (*Registry, error) {
	return NewRegistryWithEnv(configs, os.Environ())
}

func NewRegistryWithEnv(configs []AdapterConfig, lookupEnv []string) (*Registry, error) {
	registry := &Registry{
		adapters:  make(map[string]StaticAdapter, len(configs)),
		lookupEnv: append([]string(nil), lookupEnv...),
	}
	for _, cfg := range configs {
		if err := validate(cfg); err != nil {
			return nil, err
		}
		cfg.Family = normalizeFamily(cfg)
		registry.adapters[cfg.ID] = StaticAdapter{config: cfg}
	}
	return registry, nil
}

func LoadConfigs(path string) ([]AdapterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var configs []AdapterConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("parse agent config: %w", err)
	}
	return configs, nil
}

func (r *Registry) Get(id string) (StaticAdapter, bool) {
	adapter, ok := r.adapters[id]
	return adapter, ok
}

func (r *Registry) List() []AdapterConfig {
	items := make([]AdapterConfig, 0, len(r.adapters))
	for _, adapter := range r.adapters {
		items = append(items, adapter.Config())
	}
	return sortConfigs(items)
}

func (r *Registry) AvailableList() []AdapterConfig {
	r.availableOnce.Do(func() {
		items := make([]AdapterConfig, 0, len(r.adapters))
		for _, adapter := range r.adapters {
			cfg := adapter.Config()
			lookupEnv := mergeEnv(r.lookupEnv, cfg.Env)
			if isAvailableWithEnv(cfg, lookupEnv) {
				items = append(items, cfg)
			}
		}
		r.availableList = sortConfigs(items)
	})
	return append([]AdapterConfig(nil), r.availableList...)
}

func (r *Registry) Resolve(id, workspace string, inheritedEnv []string) (LaunchSpec, error) {
	adapter, ok := r.adapters[id]
	if !ok {
		return LaunchSpec{}, fmt.Errorf("unknown adapter %q", id)
	}
	return adapter.Resolve(workspace, inheritedEnv)
}

func validate(cfg AdapterConfig) error {
	switch {
	case strings.TrimSpace(cfg.ID) == "":
		return errors.New("adapter id is required")
	case strings.TrimSpace(cfg.Label) == "":
		return fmt.Errorf("adapter %q label is required", cfg.ID)
	case strings.TrimSpace(cfg.Command) == "":
		return fmt.Errorf("adapter %q command is required", cfg.ID)
	}

	switch cfg.CWDMode {
	case "", CWDWorkspaceRoot, CWDProcess:
	default:
		return fmt.Errorf("adapter %q has unsupported cwd_mode %q", cfg.ID, cfg.CWDMode)
	}
	cfg.Family = normalizeFamily(cfg)
	return nil
}

func mergeConfigs(builtins, custom []AdapterConfig) []AdapterConfig {
	merged := make(map[string]AdapterConfig, len(builtins)+len(custom))
	for _, cfg := range builtins {
		merged[cfg.ID] = cfg
	}
	for _, cfg := range custom {
		merged[cfg.ID] = cfg
	}

	out := make([]AdapterConfig, 0, len(merged))
	for _, cfg := range merged {
		if cfg.CWDMode == "" {
			cfg.CWDMode = CWDWorkspaceRoot
		}
		cfg.Family = normalizeFamily(cfg)
		out = append(out, cfg)
	}
	return out
}

func normalizeFamily(cfg AdapterConfig) string {
	if normalized := peer.NormalizeFamily(cfg.Family); normalized != "" {
		return normalized
	}

	switch strings.TrimSpace(strings.ToLower(cfg.ID)) {
	case "claude", "claude-code":
		return peer.FamilyClaude
	case "codex":
		return peer.FamilyCodex
	case "gemini":
		return peer.FamilyGemini
	case "github-copilot", "copilot":
		return peer.FamilyCopilot
	case "cursor", "cursor-agent":
		return peer.FamilyCursor
	case "kiro":
		return peer.FamilyKiro
	case "aider":
		return peer.FamilyAider
	case "opencode":
		return peer.FamilyOpenCode
	default:
		return peer.FamilyGeneric
	}
}

func sortConfigs(items []AdapterConfig) []AdapterConfig {
	sort.Slice(items, func(i, j int) bool {
		if items[i].ID == "shell" {
			return true
		}
		if items[j].ID == "shell" {
			return false
		}
		return items[i].Label < items[j].Label
	})
	return items
}

func isAvailableWithEnv(cfg AdapterConfig, env []string) bool {
	if !commandExistsWithEnv(cfg.Command, env) {
		return false
	}
	if len(cfg.Detect) == 0 {
		return true
	}
	for _, candidate := range cfg.Detect {
		if commandExistsWithEnv(candidate, env) {
			return true
		}
	}
	return false
}

func commandExistsWithEnv(command string, env []string) bool {
	_, err := resolveCommandPath(command, env)
	return err == nil
}

func resolveCommandPath(command string, env []string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("command is required")
	}
	if fields := strings.Fields(command); len(fields) > 0 {
		command = fields[0]
	}

	if strings.Contains(command, string(filepath.Separator)) {
		if isExecutableFile(command) {
			return command, nil
		}
		return "", fmt.Errorf("command %q is not executable", command)
	}

	pathValue := envValue(env, "PATH")
	if pathValue == "" {
		pathValue = os.Getenv("PATH")
	}
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, command)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("command %q not found on PATH", command)
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func envValue(env []string, key string) string {
	for _, item := range env {
		currentKey, currentValue, ok := strings.Cut(item, "=")
		if ok && currentKey == key {
			return currentValue
		}
	}
	return ""
}

func mergeEnv(inherited []string, overrides map[string]string) []string {
	envMap := make(map[string]string, len(inherited)+len(overrides))
	for _, item := range inherited {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		envMap[key] = value
	}
	for key, value := range overrides {
		envMap[key] = value
	}

	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+envMap[key])
	}
	return out
}
