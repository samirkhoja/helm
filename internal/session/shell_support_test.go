package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"helm-wails/internal/agent"
)

func TestRenderPathCasesSkipsHelperCommandsAndAmbiguousBasenames(t *testing.T) {
	t.Parallel()

	definitions := map[string]shellAdapterDefinition{
		"wrapped-agent": {
			ID:          "wrapped-agent",
			Command:     "/usr/bin/env",
			CommandName: "env",
			DirectMatch: false,
			WrapperName: "wrapped-agent",
		},
		"claude-code": {
			ID:          "claude-code",
			Command:     "/usr/local/bin/claude",
			CommandName: "claude",
			DirectMatch: true,
			WrapperName: "claude-code",
		},
		"other-claude": {
			ID:          "other-claude",
			Command:     "/opt/tools/claude",
			CommandName: "claude",
			DirectMatch: true,
			WrapperName: "other-claude",
		},
	}

	joined := strings.Join(renderPathCases(definitions, "    ", "printf '%s\\n' --"), "\n")

	if strings.Contains(joined, "/usr/bin/env") || strings.Contains(joined, "'env')") {
		t.Fatalf("helper command unexpectedly matched directly:\n%s", joined)
	}
	if !strings.Contains(joined, "/usr/local/bin/claude") || !strings.Contains(joined, "/opt/tools/claude") {
		t.Fatalf("expected direct command paths in rendered cases:\n%s", joined)
	}
	if strings.Contains(joined, "'claude')") {
		t.Fatalf("ambiguous command basename should not be matched directly:\n%s", joined)
	}
}

func TestNextShellWrapperNameUsesAdapterIdentity(t *testing.T) {
	t.Parallel()

	used := map[string]int{}
	if got := nextShellWrapperName("wrapped-agent", used); got != "wrapped-agent" {
		t.Fatalf("first wrapper = %q, want wrapped-agent", got)
	}
	if got := nextShellWrapperName("wrapped agent", used); got != "wrapped-agent-2" {
		t.Fatalf("second wrapper = %q, want wrapped-agent-2", got)
	}
}

func TestShellAdapterDefinitionsUseAdapterIDsForWrapperNames(t *testing.T) {
	t.Parallel()

	registry, err := agent.NewRegistry([]agent.AdapterConfig{
		{
			ID:      "shell",
			Label:   "Shell",
			Command: "/bin/sh",
		},
		{
			ID:      "claude-code",
			Label:   "Claude Code",
			Command: "/usr/bin/env",
		},
		{
			ID:      "wrapped agent",
			Label:   "Wrapped Agent",
			Command: "/usr/bin/env",
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	manager := NewManager(registry, []string{"PATH=/usr/bin:/bin"}, newFakeStarter(), &fakeSink{}, nil)
	root := t.TempDir()

	definitions, err := manager.shellAdapterDefinitions(root, root, []string{"PATH=/usr/bin:/bin"}, nil)
	if err != nil {
		t.Fatalf("shellAdapterDefinitions() error = %v", err)
	}

	claude := definitions["claude-code"]
	if claude.WrapperName != "claude-code" {
		t.Fatalf("claude wrapper = %q, want claude-code", claude.WrapperName)
	}
	if claude.WrapperName == "env" {
		t.Fatalf("claude wrapper = %q, want adapter-based name", claude.WrapperName)
	}

	wrapped := definitions["wrapped agent"]
	if wrapped.WrapperName != "wrapped-agent" {
		t.Fatalf("wrapped wrapper = %q, want wrapped-agent", wrapped.WrapperName)
	}
	if wrapped.WrapperName == "env" {
		t.Fatalf("wrapped wrapper = %q, want adapter-based name", wrapped.WrapperName)
	}
	if wrapped.WrapperName == claude.WrapperName {
		t.Fatalf("wrapper names collided: %#v", definitions)
	}
}

func TestShellAdapterDefinitionsUseResolvedEnvDiffForDirectMatch(t *testing.T) {
	t.Parallel()

	registry, err := agent.NewRegistry([]agent.AdapterConfig{
		{
			ID:      "shell",
			Label:   "Shell",
			Command: "/bin/sh",
		},
		{
			ID:      "codex",
			Label:   "Codex",
			Command: "/bin/echo",
			Env: map[string]string{
				"FOO": "bar",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	manager := NewManager(registry, []string{"PATH=/usr/bin:/bin", "FOO=bar"}, newFakeStarter(), &fakeSink{}, nil)
	root := t.TempDir()

	definitions, err := manager.shellAdapterDefinitions(root, root, []string{"PATH=/usr/bin:/bin", "FOO=bar"}, nil)
	if err != nil {
		t.Fatalf("shellAdapterDefinitions() error = %v", err)
	}

	def, ok := definitions["codex"]
	if !ok {
		t.Fatalf("definitions missing codex: %#v", definitions)
	}
	if len(def.Env) != 0 {
		t.Fatalf("env overrides = %#v, want none when shell already matches resolved env", def.Env)
	}
	if !def.DirectMatch {
		t.Fatalf("DirectMatch = false, want true when resolved launch has no shell-local overrides")
	}
}

func TestShellCommandInvocationsUseManagedLaunchHelper(t *testing.T) {
	t.Parallel()

	definition := shellAdapterDefinition{
		Command: "/bin/echo",
		Args:    []string{"resume", "--last"},
		Env: map[string]string{
			"FOO": "bar",
		},
	}

	bourne := bourneCommandInvocation(definition, true)
	if !strings.Contains(bourne, "helm session-launch --") {
		t.Fatalf("bourne invocation = %q, want managed launch helper", bourne)
	}

	fish := fishCommandInvocation(definition, true)
	if !strings.Contains(fish, "helm session-launch --") {
		t.Fatalf("fish invocation = %q, want managed launch helper", fish)
	}

	quoted := shellQuotedCommand(definition, true)
	if !strings.Contains(quoted, "helm session-launch --") {
		t.Fatalf("quoted command = %q, want managed launch helper", quoted)
	}

	fallback := shellQuotedCommand(definition, false)
	if strings.Contains(fallback, "helm session-launch --") {
		t.Fatalf("fallback quoted command = %q, want direct invocation without managed launch helper", fallback)
	}
}

func TestRenderBashIntegrationPreservesExistingHooks(t *testing.T) {
	t.Parallel()

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}

	definitions := map[string]shellAdapterDefinition{
		"codex": {
			ID:          "codex",
			Command:     "/bin/echo",
			CommandName: "echo",
			DirectMatch: true,
			WrapperName: "codex",
			LaunchFunc:  "__helm_launch_codex",
			WrapFunc:    "__helm_wrap_codex",
		},
	}

	scriptPath := filepath.Join(t.TempDir(), "helm.bash")
	rcfile := strings.Join([]string{
		"existing_prompt() { :; }",
		"PROMPT_COMMAND=(existing_prompt)",
		"trap 'printf previous >/dev/null' DEBUG",
		renderBashIntegration(definitions, "shell", "", false),
		"",
	}, "\n")
	if err := os.WriteFile(scriptPath, []byte(rcfile), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command(bashPath, "--noprofile", "--rcfile", scriptPath, "-i", "-c", `
declare -p PROMPT_COMMAND
printf 'prev:%s\n' "$__helm_prev_debug_trap"
trap -p DEBUG
`)
	cmd.Env = append(os.Environ(), "TERM=dumb")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash exited with %v: %s", err, output)
	}

	text := string(output)
	if !strings.Contains(text, "declare -a PROMPT_COMMAND=") {
		t.Fatalf("PROMPT_COMMAND was not preserved as an array:\n%s", text)
	}
	if !strings.Contains(text, "__helm_prompt_command") || !strings.Contains(text, "existing_prompt") {
		t.Fatalf("PROMPT_COMMAND missing expected entries:\n%s", text)
	}
	if strings.Contains(text, "prev:\n") {
		t.Fatalf("previous DEBUG trap was not captured:\n%s", text)
	}
	if !strings.Contains(text, "__helm_debug_dispatch") {
		t.Fatalf("DEBUG trap was not installed through dispatch:\n%s", text)
	}
}

func TestConfigureZshShellLaunchPreservesOriginalHistoryLocation(t *testing.T) {
	t.Parallel()

	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}

	homeDir := t.TempDir()
	originalZdotDir := filepath.Join(homeDir, ".config", "zsh")
	if err := os.MkdirAll(originalZdotDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(originalZdotDir, ".zshrc"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile(.zshrc) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(originalZdotDir, ".zsh_history"), []byte(": 1700000000:0;fromhistory\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.zsh_history) error = %v", err)
	}

	plan := shellLaunchPlan{
		ShellSpec: agent.LaunchSpec{
			Env: append(
				os.Environ(),
				"HOME="+homeDir,
				"ZDOTDIR="+originalZdotDir,
				"TERM=dumb",
			),
		},
	}

	if err := configureZshShellLaunch(&plan, map[string]shellAdapterDefinition{}, "shell"); err != nil {
		t.Fatalf("configureZshShellLaunch() error = %v", err)
	}
	defer func() {
		if plan.SupportDir != "" {
			_ = os.RemoveAll(plan.SupportDir)
		}
	}()

	if got := filepath.Clean(launchEnvMap(plan.ShellSpec.Env)["ZDOTDIR"]); got != filepath.Join(plan.SupportDir, "zdot") {
		t.Fatalf("ZDOTDIR = %q, want %q", got, filepath.Join(plan.SupportDir, "zdot"))
	}
	cmd := exec.Command(zshPath, "-i", "-c", `
print -r -- "ZDOTDIR=$ZDOTDIR"
print -r -- "HISTFILE=$HISTFILE"
fc -l -n -1
`)
	cmd.Env = plan.ShellSpec.Env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh exited with %v: %s", err, output)
	}

	text := string(output)
	if !strings.Contains(text, "ZDOTDIR="+originalZdotDir) {
		t.Fatalf("zsh did not restore original ZDOTDIR:\n%s", text)
	}
	if !strings.Contains(text, "HISTFILE="+filepath.Join(originalZdotDir, ".zsh_history")) {
		t.Fatalf("zsh did not restore original history path:\n%s", text)
	}
	if !strings.Contains(text, "fromhistory") {
		t.Fatalf("zsh did not load original history contents:\n%s", text)
	}
}

func TestResolveOriginalZshDotDirFallsBackToHome(t *testing.T) {
	t.Parallel()

	got := resolveOriginalZshDotDir([]string{"HOME=/Users/tester"})
	if got != "/Users/tester" {
		t.Fatalf("resolveOriginalZshDotDir() = %q, want /Users/tester", got)
	}
}
