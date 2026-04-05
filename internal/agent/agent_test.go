package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigsMissingFile(t *testing.T) {
	t.Parallel()

	items, err := LoadConfigs(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadConfigs() error = %v", err)
	}
	if items != nil {
		t.Fatalf("LoadConfigs() = %#v, want nil", items)
	}
}

func TestRegistryAvailableListFiltersMissingCommands(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry([]AdapterConfig{
		{
			ID:      "shell",
			Label:   "Shell",
			Command: "/bin/sh",
		},
		{
			ID:      "missing",
			Label:   "Missing",
			Command: "definitely-not-installed-helm-agent",
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	items := registry.AvailableList()
	if len(items) != 1 || items[0].ID != "shell" {
		t.Fatalf("AvailableList() = %#v, want only shell", items)
	}
}

func TestRegistryAvailableListUsesExplicitPATH(t *testing.T) {
	t.Parallel()

	binDir := t.TempDir()
	claudePath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(claudePath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry, err := NewRegistryWithEnv([]AdapterConfig{
		{
			ID:      "claude-code",
			Label:   "Claude Code",
			Command: "claude",
		},
	}, []string{"PATH=" + binDir})
	if err != nil {
		t.Fatalf("NewRegistryWithEnv() error = %v", err)
	}

	items := registry.AvailableList()
	if len(items) != 1 || items[0].ID != "claude-code" {
		t.Fatalf("AvailableList() = %#v, want claude-code", items)
	}
}

func TestRegistryAvailableListHidesMissingExplicitPATHCommand(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistryWithEnv([]AdapterConfig{
		{
			ID:      "claude-code",
			Label:   "Claude Code",
			Command: "claude",
		},
	}, []string{"PATH=" + t.TempDir()})
	if err != nil {
		t.Fatalf("NewRegistryWithEnv() error = %v", err)
	}

	items := registry.AvailableList()
	if len(items) != 0 {
		t.Fatalf("AvailableList() = %#v, want empty", items)
	}
}

func TestRegistryAvailableListSupportsAbsoluteCommandPath(t *testing.T) {
	t.Parallel()

	commandPath := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry, err := NewRegistryWithEnv([]AdapterConfig{
		{
			ID:      "claude-code",
			Label:   "Claude Code",
			Command: commandPath,
		},
	}, []string{"PATH=" + t.TempDir()})
	if err != nil {
		t.Fatalf("NewRegistryWithEnv() error = %v", err)
	}

	items := registry.AvailableList()
	if len(items) != 1 || items[0].Command != commandPath {
		t.Fatalf("AvailableList() = %#v, want absolute path entry", items)
	}
}

func TestRegistryAvailableListUsesAdapterEnvPATH(t *testing.T) {
	t.Parallel()

	binDir := t.TempDir()
	commandPath := filepath.Join(binDir, "custom-agent")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry, err := NewRegistryWithEnv([]AdapterConfig{
		{
			ID:      "custom",
			Label:   "Custom",
			Command: "custom-agent",
			Env: map[string]string{
				"PATH": binDir,
			},
		},
	}, []string{"PATH=/usr/bin:/bin"})
	if err != nil {
		t.Fatalf("NewRegistryWithEnv() error = %v", err)
	}

	items := registry.AvailableList()
	if len(items) != 1 || items[0].ID != "custom" {
		t.Fatalf("AvailableList() = %#v, want custom agent", items)
	}
}

func TestResolveUsesWorkspaceCWDAndMergedEnv(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry([]AdapterConfig{
		{
			ID:      "shell",
			Label:   "Shell",
			Command: "/bin/sh",
			Args:    []string{"-i"},
			Env: map[string]string{
				"TERM": "xterm-256color",
				"FOO":  "bar",
			},
			CWDMode: CWDWorkspaceRoot,
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	spec, err := registry.Resolve("shell", "/tmp/workspace", []string{"FOO=baz", "BAR=qux"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if spec.CWD != "/tmp/workspace" {
		t.Fatalf("spec.CWD = %q, want /tmp/workspace", spec.CWD)
	}
	if spec.Command != "/bin/sh" {
		t.Fatalf("spec.Command = %q, want /bin/sh", spec.Command)
	}

	env := make(map[string]string, len(spec.Env))
	for _, item := range spec.Env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}

	if env["FOO"] != "bar" || env["BAR"] != "qux" || env["TERM"] != "xterm-256color" {
		t.Fatalf("spec.Env = %#v", spec.Env)
	}
}

func TestResolveUsesExplicitPATHForCommandLookup(t *testing.T) {
	t.Parallel()

	binDir := t.TempDir()
	commandPath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry, err := NewRegistry([]AdapterConfig{
		{
			ID:      "claude-code",
			Label:   "Claude Code",
			Command: "claude",
			CWDMode: CWDWorkspaceRoot,
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	spec, err := registry.Resolve("claude-code", "/tmp/workspace", []string{"PATH=" + binDir})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.Command != commandPath {
		t.Fatalf("spec.Command = %q, want %q", spec.Command, commandPath)
	}
}

func TestResolveUsesAdapterEnvPATHForCommandLookup(t *testing.T) {
	t.Parallel()

	binDir := t.TempDir()
	commandPath := filepath.Join(binDir, "helper")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry, err := NewRegistry([]AdapterConfig{
		{
			ID:      "helper",
			Label:   "Helper",
			Command: "helper",
			Env: map[string]string{
				"PATH": binDir,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	spec, err := registry.Resolve("helper", "/tmp/workspace", []string{"PATH=/usr/bin:/bin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.Command != commandPath {
		t.Fatalf("spec.Command = %q, want %q", spec.Command, commandPath)
	}
}

func TestBuiltInConfigsIncludeResumeArgsForRestorableAgents(t *testing.T) {
	t.Parallel()

	configs := BuiltInConfigsWithEnv([]string{"SHELL=/bin/zsh"})

	var codex, claude *AdapterConfig
	for i := range configs {
		switch configs[i].ID {
		case "codex":
			codex = &configs[i]
		case "claude-code":
			claude = &configs[i]
		}
	}

	if codex == nil {
		t.Fatalf("codex config not found in %#v", configs)
	}
	if got := strings.Join(codex.ResumeArgs, " "); got != "resume --last" {
		t.Fatalf("codex ResumeArgs = %q, want %q", got, "resume --last")
	}

	if claude == nil {
		t.Fatalf("claude-code config not found in %#v", configs)
	}
	if got := strings.Join(claude.ResumeArgs, " "); got != "--continue" {
		t.Fatalf("claude-code ResumeArgs = %q, want %q", got, "--continue")
	}
}

func TestResolveFailsWhenCommandCannotBeResolved(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry([]AdapterConfig{
		{
			ID:      "missing",
			Label:   "Missing",
			Command: "definitely-not-installed-helm-agent",
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	if _, err := registry.Resolve("missing", "/tmp/workspace", []string{"PATH=" + t.TempDir()}); err == nil {
		t.Fatalf("Resolve() error = nil, want lookup failure")
	}
}

func TestDefaultConfigPathUsesHelmDir(t *testing.T) {
	t.Parallel()

	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath() error = %v", err)
	}

	if filepath.Base(path) != "agents.json" {
		t.Fatalf("DefaultConfigPath() = %q, want agents.json suffix", path)
	}
	if filepath.Base(filepath.Dir(path)) != "helm" {
		t.Fatalf("DefaultConfigPath() dir = %q, want helm", filepath.Dir(path))
	}
}

func TestInteractiveShellArgs(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		"/bin/zsh":               {"-i"},
		"/bin/bash":              {"-i"},
		"/opt/homebrew/bin/fish": {"-i"},
		"/bin/sh":                nil,
	}

	for shell, want := range cases {
		got := interactiveShellArgs(shell)
		if len(got) != len(want) {
			t.Fatalf("interactiveShellArgs(%q) = %#v, want %#v", shell, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("interactiveShellArgs(%q) = %#v, want %#v", shell, got, want)
			}
		}
	}
}

func TestBuiltInConfigsWithEnvIncludesAdditionalAgentCLIs(t *testing.T) {
	t.Parallel()

	configs := BuiltInConfigsWithEnv([]string{"SHELL=/bin/zsh"})
	byID := make(map[string]AdapterConfig, len(configs))
	for _, cfg := range configs {
		byID[cfg.ID] = cfg
	}

	cases := map[string]string{
		"gemini":         "gemini",
		"github-copilot": "copilot",
		"cursor-agent":   "cursor-agent",
		"kiro":           "kiro-cli",
		"aider":          "aider",
		"opencode":       "opencode",
	}

	for id, command := range cases {
		cfg, ok := byID[id]
		if !ok {
			t.Fatalf("BuiltInConfigsWithEnv() missing adapter %q", id)
		}
		if cfg.Command != command {
			t.Fatalf("BuiltInConfigsWithEnv() command for %q = %q, want %q", id, cfg.Command, command)
		}
		if cfg.CWDMode != CWDWorkspaceRoot {
			t.Fatalf("BuiltInConfigsWithEnv() cwd_mode for %q = %q, want %q", id, cfg.CWDMode, CWDWorkspaceRoot)
		}
	}
}
