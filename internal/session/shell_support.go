package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"helm-wails/internal/agent"
)

const helmSessionModeOSC = 697

type shellAdapterDefinition struct {
	ID          string
	Label       string
	Family      string
	Command     string
	CommandName string
	Args        []string
	Env         map[string]string
	PeerEnabled bool
	DirectMatch bool
	WrapperName string
	LaunchFunc  string
	WrapFunc    string
}

type shellLaunchPlan struct {
	ShellSpec         agent.LaunchSpec
	RequestedSpec     agent.LaunchSpec
	PeerLaunch        *peerLaunchState
	SupportDir        string
	ManualBootCommand string
	ModeTracking      bool
	UseManagedLaunch  bool
}

type shellAdapterCacheKey struct {
	workspaceRoot string
	peerEnabled   bool
}

type cachedShellAdapterDefinition struct {
	cfg  agent.AdapterConfig
	spec agent.LaunchSpec
}

type cachedShellAdapterDefinitions struct {
	items map[string]cachedShellAdapterDefinition
}

func (m *Manager) prepareShellBackedLaunch(
	workspaceRoot string,
	startPath string,
	requestedAgentID string,
	restoreSession bool,
) (shellLaunchPlan, error) {
	requestedSpec, err := m.registry.Resolve(requestedAgentID, workspaceRoot, m.inheritedEnv)
	if err != nil {
		return shellLaunchPlan{}, err
	}
	requestedSpec.CWD = startPath

	shellSpec, err := m.registry.Resolve("shell", workspaceRoot, m.inheritedEnv)
	if err != nil {
		return shellLaunchPlan{}, err
	}
	shellSpec.CWD = startPath

	var peerLaunch *peerLaunchState
	if m.peerRuntime != nil {
		shellSpec.Env, peerLaunch, err = m.peerRuntime.prepareSessionShellEnv(shellSpec.Env)
		if err != nil {
			return shellLaunchPlan{}, err
		}
	}

	definitions, err := m.shellAdapterDefinitions(workspaceRoot, startPath, shellSpec.Env, peerLaunch)
	if err != nil {
		return shellLaunchPlan{}, err
	}

	if requestedDef, ok := definitions[requestedAgentID]; ok {
		requestedSpec.Command = requestedDef.Command
		requestedSpec.Args = append([]string(nil), requestedDef.Args...)
	}

	useManagedLaunch := m.peerRuntime != nil
	plan := shellLaunchPlan{
		ShellSpec:        shellSpec,
		RequestedSpec:    requestedSpec,
		PeerLaunch:       peerLaunch,
		UseManagedLaunch: useManagedLaunch,
	}

	if requestedAgentID != "shell" {
		requestedDef, ok := definitions[requestedAgentID]
		if !ok {
			return shellLaunchPlan{}, fmt.Errorf("agent %q is not available in shell support", requestedAgentID)
		}
		plan.ManualBootCommand = shellQuotedCommand(
			m.restoreBootDefinition(requestedDef, requestedAgentID, restoreSession),
			useManagedLaunch,
		)
	}

	if useManagedLaunch {
		plan.ShellSpec.Env = mergeLaunchEnv(plan.ShellSpec.Env, map[string]string{
			"HELM_APP_PID": strconv.Itoa(os.Getpid()),
		})
	}

	switch filepath.Base(shellSpec.Command) {
	case "zsh":
		if err := configureZshShellLaunch(&plan, definitions, requestedAgentID); err != nil {
			return shellLaunchPlan{}, err
		}
	case "bash":
		if err := configureBashShellLaunch(&plan, definitions, requestedAgentID); err != nil {
			return shellLaunchPlan{}, err
		}
	case "fish":
		if err := configureFishShellLaunch(&plan, definitions, requestedAgentID); err != nil {
			return shellLaunchPlan{}, err
		}
	}

	return plan, nil
}

func (m *Manager) restoreBootDefinition(
	requestedDef shellAdapterDefinition,
	requestedAgentID string,
	restoreSession bool,
) shellAdapterDefinition {
	if !restoreSession {
		return requestedDef
	}

	cfg, ok := m.registry.Get(requestedAgentID)
	if !ok {
		return requestedDef
	}

	resumeArgs := cfg.Config().ResumeArgs
	if len(resumeArgs) == 0 {
		return requestedDef
	}

	resumeDef := requestedDef
	resumeDef.Args = append([]string(nil), resumeArgs...)
	resumeDef.DirectMatch = false
	return resumeDef
}

func (m *Manager) shellAdapterDefinitions(
	workspaceRoot string,
	startPath string,
	shellEnv []string,
	_ *peerLaunchState,
) (map[string]shellAdapterDefinition, error) {
	resolved, err := m.cachedShellAdapterDefinitions(workspaceRoot)
	if err != nil {
		return nil, err
	}

	out := make(map[string]shellAdapterDefinition)
	wrapperNames := map[string]int{}
	for _, r := range orderedCachedShellAdapterDefinitions(resolved.items) {
		spec := r.spec
		spec.CWD = startPath

		env := launchEnvOverrides(shellEnv, spec.Env)
		wrapperName := nextShellWrapperName(r.cfg.ID, wrapperNames)

		out[r.cfg.ID] = shellAdapterDefinition{
			ID:          r.cfg.ID,
			Label:       r.cfg.Label,
			Family:      r.cfg.Family,
			Command:     spec.Command,
			CommandName: filepath.Base(spec.Command),
			Args:        append([]string(nil), spec.Args...),
			Env:         env,
			PeerEnabled: r.cfg.PeerEnabled,
			DirectMatch: len(spec.Args) == 0 && len(env) == 0,
			WrapperName: wrapperName,
			LaunchFunc:  "__helm_launch_" + sanitizeShellIdentifier(r.cfg.ID),
			WrapFunc:    "__helm_wrap_" + sanitizeShellIdentifier(r.cfg.ID),
		}
	}

	return out, nil
}

func (m *Manager) cachedShellAdapterDefinitions(workspaceRoot string) (cachedShellAdapterDefinitions, error) {
	key := shellAdapterCacheKey{
		workspaceRoot: filepath.Clean(workspaceRoot),
		peerEnabled:   m.peerRuntime != nil,
	}

	m.shellCacheMu.RLock()
	if cached, ok := m.shellCache[key]; ok {
		m.shellCacheMu.RUnlock()
		return cached, nil
	}
	m.shellCacheMu.RUnlock()

	items := m.registry.AvailableList()
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})

	resolved := make(map[string]cachedShellAdapterDefinition, len(items))
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup

	for _, cfg := range items {
		if cfg.ID == "shell" {
			continue
		}
		wg.Add(1)
		go func(cfg agent.AdapterConfig) {
			defer wg.Done()

			spec, err := m.registry.Resolve(cfg.ID, workspaceRoot, m.inheritedEnv)
			if err != nil {
				return
			}
			if m.peerRuntime != nil {
				spec, err = m.peerRuntime.prepareShellAdapterSpec(spec)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
			}

			mu.Lock()
			resolved[cfg.ID] = cachedShellAdapterDefinition{
				cfg:  cfg,
				spec: spec,
			}
			mu.Unlock()
		}(cfg)
	}
	wg.Wait()

	if firstErr != nil {
		return cachedShellAdapterDefinitions{}, firstErr
	}

	cached := cachedShellAdapterDefinitions{items: resolved}
	m.shellCacheMu.Lock()
	if existing, ok := m.shellCache[key]; ok {
		m.shellCacheMu.Unlock()
		return existing, nil
	}
	m.shellCache[key] = cached
	m.shellCacheMu.Unlock()
	return cached, nil
}

func orderedCachedShellAdapterDefinitions(items map[string]cachedShellAdapterDefinition) []cachedShellAdapterDefinition {
	ordered := make([]cachedShellAdapterDefinition, 0, len(items))
	for _, item := range items {
		ordered = append(ordered, item)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].cfg.ID < ordered[j].cfg.ID
	})
	return ordered
}

func configureZshShellLaunch(plan *shellLaunchPlan, definitions map[string]shellAdapterDefinition, requestedAgentID string) error {
	supportDir, err := os.MkdirTemp("", "helm-shell-zsh-")
	if err != nil {
		return err
	}

	zdotDir := filepath.Join(supportDir, "zdot")
	if err := os.MkdirAll(zdotDir, 0o755); err != nil {
		_ = os.RemoveAll(supportDir)
		return err
	}

	helmPath := filepath.Join(supportDir, "helm.zsh")
	if err := os.WriteFile(helmPath, []byte(renderZshIntegration(definitions, requestedAgentID, plan.ManualBootCommand, plan.UseManagedLaunch)), 0o644); err != nil {
		_ = os.RemoveAll(supportDir)
		return err
	}
	originalZdotDir := resolveOriginalZshDotDir(plan.ShellSpec.Env)
	files := map[string]string{
		filepath.Join(zdotDir, ".zshenv"):   renderZshWrapperSourceFile(originalZdotDir, zdotDir, ".zshenv", true),
		filepath.Join(zdotDir, ".zprofile"): renderZshWrapperSourceFile(originalZdotDir, zdotDir, ".zprofile", true),
		filepath.Join(zdotDir, ".zlogin"):   renderZshWrapperSourceFile(originalZdotDir, zdotDir, ".zlogin", true),
		filepath.Join(zdotDir, ".zshrc"): renderZshWrapperRC(
			originalZdotDir,
			zdotDir,
			helmPath,
		),
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			_ = os.RemoveAll(supportDir)
			return err
		}
	}

	plan.SupportDir = supportDir
	plan.ModeTracking = true
	plan.ShellSpec.Env = mergeLaunchEnv(plan.ShellSpec.Env, map[string]string{
		"ZDOTDIR":             zdotDir,
		"HELM_SESSION_OSC_ID": strconv.Itoa(helmSessionModeOSC),
	})
	return nil
}

func resolveOriginalZshDotDir(env []string) string {
	values := launchEnvMap(env)
	if dir := strings.TrimSpace(values["ZDOTDIR"]); dir != "" {
		return filepath.Clean(dir)
	}
	if home := strings.TrimSpace(values["HOME"]); home != "" {
		return filepath.Clean(home)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Clean(home)
	}
	return "."
}

func renderZshWrapperSourceFile(originalZdotDir, wrapperZdotDir, name string, restoreWrapper bool) string {
	lines := []string{
		fmt.Sprintf("ZDOTDIR=%s", shellSingleQuote(originalZdotDir)),
		zshSourceFileLine(originalZdotDir, name),
	}
	if restoreWrapper {
		lines = append(lines, fmt.Sprintf("ZDOTDIR=%s", shellSingleQuote(wrapperZdotDir)))
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func renderZshWrapperRC(originalZdotDir, wrapperZdotDir, helmPath string) string {
	originalHistoryPath := shellSingleQuote(filepath.Join(originalZdotDir, ".zsh_history"))
	wrapperHistoryPath := shellSingleQuote(filepath.Join(wrapperZdotDir, ".zsh_history"))
	lines := []string{
		fmt.Sprintf("ZDOTDIR=%s", shellSingleQuote(originalZdotDir)),
		zshSourceFileLine(originalZdotDir, ".zshrc"),
		fmt.Sprintf(
			`if [[ -z "${HISTFILE:-}" || "$HISTFILE" == %s ]]; then`,
			wrapperHistoryPath,
		),
		fmt.Sprintf("  HISTFILE=%s", originalHistoryPath),
		`  fc -R "$HISTFILE" 2>/dev/null`,
		"fi",
		fmt.Sprintf("source %s", shellSingleQuote(helmPath)),
		"",
	}
	return strings.Join(lines, "\n")
}

func zshSourceFileLine(dotDir, name string) string {
	path := shellSingleQuote(filepath.Join(dotDir, name))
	return fmt.Sprintf(`[[ -r %s ]] && source %s`, path, path)
}

func configureBashShellLaunch(plan *shellLaunchPlan, definitions map[string]shellAdapterDefinition, requestedAgentID string) error {
	supportDir, err := os.MkdirTemp("", "helm-shell-bash-")
	if err != nil {
		return err
	}

	bashrcPath := filepath.Join(supportDir, "bashrc")
	content := strings.Join([]string{
		`[[ -r "$HOME/.bashrc" ]] && source "$HOME/.bashrc"`,
		renderBashIntegration(definitions, requestedAgentID, plan.ManualBootCommand, plan.UseManagedLaunch),
		"",
	}, "\n")
	if err := os.WriteFile(bashrcPath, []byte(content), 0o644); err != nil {
		_ = os.RemoveAll(supportDir)
		return err
	}

	plan.SupportDir = supportDir
	plan.ModeTracking = true
	plan.ShellSpec.Args = []string{"--rcfile", bashrcPath, "-i"}
	plan.ShellSpec.Env = mergeLaunchEnv(plan.ShellSpec.Env, map[string]string{
		"HELM_SESSION_OSC_ID": strconv.Itoa(helmSessionModeOSC),
	})
	return nil
}

func configureFishShellLaunch(plan *shellLaunchPlan, definitions map[string]shellAdapterDefinition, requestedAgentID string) error {
	supportDir, err := os.MkdirTemp("", "helm-shell-fish-")
	if err != nil {
		return err
	}

	helmPath := filepath.Join(supportDir, "helm.fish")
	if err := os.WriteFile(helmPath, []byte(renderFishIntegration(definitions, requestedAgentID, plan.ManualBootCommand, plan.UseManagedLaunch)), 0o644); err != nil {
		_ = os.RemoveAll(supportDir)
		return err
	}

	plan.SupportDir = supportDir
	plan.ModeTracking = true
	plan.ShellSpec.Args = []string{"-i", "-C", fmt.Sprintf("source %s", shellSingleQuote(helmPath))}
	plan.ShellSpec.Env = mergeLaunchEnv(plan.ShellSpec.Env, map[string]string{
		"HELM_SESSION_OSC_ID": strconv.Itoa(helmSessionModeOSC),
	})
	return nil
}

func renderZshIntegration(definitions map[string]shellAdapterDefinition, requestedAgentID string, manualBootCommand string, useManagedLaunch bool) string {
	lines := []string{
		"__helm_emit_mode() {",
		`  printf '\033]%s;adapter=%s\007' "$HELM_SESSION_OSC_ID" "$1"`,
		"}",
		"",
	}
	lines = append(lines, renderBourneAdapterFunctions(definitions, requestedAgentID, manualBootCommand, useManagedLaunch)...)
	lines = append(lines,
		"__helm_match_path_adapter() {",
		`  case "$1" in`,
	)
	lines = append(lines, renderPathCases(definitions, "    ", "print -r --")...)
	lines = append(lines,
		"  esac",
		"}",
		"",
		"__helm_preexec() {",
		"  local -a words",
		`  words=(${(z)1})`,
		`  local first="${words[1]:-}"`,
		`  [[ -n "$first" ]] || return`,
		`  local adapter="$(__helm_match_path_adapter "$first")"`,
		`  [[ -n "$adapter" ]] || return`,
		`  __helm_emit_mode "$adapter"`,
		"}",
		"",
		"typeset -g __helm_boot_done=0",
		"__helm_precmd() {",
		"  if (( ! __helm_boot_done )); then",
		"    __helm_boot_done=1",
	)
	if requestedAgentID != "shell" {
		lines = append(lines, fmt.Sprintf("    __helm_boot_%s", sanitizeShellIdentifier(requestedAgentID)))
		lines = append(lines, "    return")
	}
	lines = append(lines,
		`  fi`,
		`  __helm_emit_mode "shell"`,
		"}",
		"",
		"precmd_functions+=(__helm_precmd)",
		"preexec_functions+=(__helm_preexec)",
		"",
	)
	return strings.Join(lines, "\n")
}

func renderBashIntegration(definitions map[string]shellAdapterDefinition, requestedAgentID string, manualBootCommand string, useManagedLaunch bool) string {
	lines := []string{
		"__helm_emit_mode() {",
		`  printf '\033]%s;adapter=%s\007' "$HELM_SESSION_OSC_ID" "$1"`,
		"}",
		"",
	}
	lines = append(lines, renderBourneAdapterFunctions(definitions, requestedAgentID, manualBootCommand, useManagedLaunch)...)
	lines = append(lines,
		"__helm_match_path_adapter() {",
		`  case "$1" in`,
	)
	lines = append(lines, renderPathCases(definitions, "    ", "printf '%s\\n' --")...)
	lines = append(lines,
		"  esac",
		"}",
		"",
		"__helm_boot_done=0",
		"__helm_allow_next_command=0",
		"__helm_in_internal=0",
		"__helm_debug_trap() {",
		`  [[ "${__helm_in_internal:-0}" -eq 1 ]] && return`,
		`  [[ "${__helm_allow_next_command:-0}" -eq 1 ]] || return`,
		"  __helm_allow_next_command=0",
		`  local command="${BASH_COMMAND-}"`,
		`  local first="${command%%[[:space:]]*}"`,
		`  [[ -n "$first" ]] || return`,
		`  local adapter`,
		`  adapter="$(__helm_match_path_adapter "$first")"`,
		`  [[ -n "$adapter" ]] || return`,
		`  __helm_emit_mode "$adapter"`,
		"}",
		"",
		"__helm_debug_dispatch() {",
		"  __helm_debug_trap",
		`  if [[ -n "${__helm_prev_debug_trap:-}" ]]; then`,
		`    builtin eval -- "$__helm_prev_debug_trap"`,
		"  fi",
		"}",
		"",
		"__helm_prompt_command() {",
		"  __helm_in_internal=1",
		`  if [[ "${__helm_boot_done:-0}" -eq 0 ]]; then`,
		"    __helm_boot_done=1",
	)
	if requestedAgentID != "shell" {
		lines = append(lines,
			"    __helm_in_internal=0",
			fmt.Sprintf("    __helm_boot_%s", sanitizeShellIdentifier(requestedAgentID)),
			"    return",
		)
	}
	lines = append(lines,
		"  fi",
		`  __helm_emit_mode "shell"`,
		"  __helm_allow_next_command=1",
		"  __helm_in_internal=0",
		"}",
		"",
		"__helm_install_prompt_command() {",
		`  local prompt_decl=""`,
		`  if declare -p PROMPT_COMMAND >/dev/null 2>&1; then`,
		`    prompt_decl=$(declare -p PROMPT_COMMAND 2>/dev/null)`,
		"  fi",
		`  case "$prompt_decl" in`,
		`    "declare -a "*|"declare -ax "*)`,
		`      local item`,
		`      for item in "${PROMPT_COMMAND[@]}"; do`,
		`        [[ "$item" == "__helm_prompt_command" ]] && return`,
		"      done",
		`      PROMPT_COMMAND=(__helm_prompt_command "${PROMPT_COMMAND[@]}")`,
		"      ;;",
		"    *)",
		`      [[ ";${PROMPT_COMMAND:-};" == *";__helm_prompt_command;"* ]] && return`,
		`      PROMPT_COMMAND="__helm_prompt_command${PROMPT_COMMAND:+;$PROMPT_COMMAND}"`,
		"      ;;",
		"  esac",
		"}",
		"",
		`__helm_prev_debug_trap=$(trap -p DEBUG)`,
		`if [[ "$__helm_prev_debug_trap" != "trap -- '__helm_debug_dispatch' DEBUG" ]]; then`,
		`  if [[ -n "$__helm_prev_debug_trap" ]]; then`,
		`    __helm_prev_debug_trap=${__helm_prev_debug_trap#trap -- }`,
		`    __helm_prev_debug_trap=${__helm_prev_debug_trap% DEBUG}`,
		"  fi",
		`  trap '__helm_debug_dispatch' DEBUG`,
		"else",
		`  __helm_prev_debug_trap=""`,
		"fi",
		"",
		"__helm_install_prompt_command",
		"",
	)
	return strings.Join(lines, "\n")
}

func renderFishIntegration(definitions map[string]shellAdapterDefinition, requestedAgentID string, manualBootCommand string, useManagedLaunch bool) string {
	lines := []string{
		"function __helm_emit_mode",
		`  printf '\e]%s;adapter=%s\a' "$HELM_SESSION_OSC_ID" "$argv[1]"`,
		"end",
		"",
	}
	lines = append(lines, renderFishAdapterFunctions(definitions, requestedAgentID, manualBootCommand, useManagedLaunch)...)
	lines = append(lines,
		"function __helm_match_path_adapter",
		"  switch $argv[1]",
	)
	lines = append(lines, renderFishPathCases(definitions)...)
	lines = append(lines,
		"  end",
		"end",
		"",
		"set -g __helm_boot_done 0",
		"",
		"function __helm_prompt_hook --on-event fish_prompt",
		"  if test $__helm_boot_done -eq 0",
		"    set -g __helm_boot_done 1",
	)
	if requestedAgentID != "shell" {
		lines = append(lines, fmt.Sprintf("    __helm_boot_%s", sanitizeShellIdentifier(requestedAgentID)))
		lines = append(lines, "    return")
	}
	lines = append(lines,
		"  end",
		"  __helm_emit_mode shell",
		"end",
		"",
		"function __helm_preexec_hook --on-event fish_preexec",
		`  set -l command (string trim -- "$argv[1]")`,
		`  if test -z "$command"`,
		"    return",
		"  end",
		`  set -l first (string split -m1 ' ' -- $command)[1]`,
		`  set -l adapter (__helm_match_path_adapter "$first")`,
		`  if test -n "$adapter"`,
		`    __helm_emit_mode "$adapter"`,
		"  end",
		"end",
		"",
	)
	return strings.Join(lines, "\n")
}

func renderBourneAdapterFunctions(definitions map[string]shellAdapterDefinition, requestedAgentID string, manualBootCommand string, useManagedLaunch bool) []string {
	items := orderedDefinitions(definitions)
	lines := []string{}
	for _, def := range items {
		bootCommand := def.LaunchFunc
		if def.ID == requestedAgentID && manualBootCommand != "" {
			bootCommand = manualBootCommand
		}
		lines = append(lines,
			fmt.Sprintf("%s() {", def.LaunchFunc),
			fmt.Sprintf("  %s", bourneCommandInvocation(def, useManagedLaunch)),
			"}",
			fmt.Sprintf("%s() {", def.WrapFunc),
			fmt.Sprintf("  __helm_emit_mode %s", shellSingleQuote(def.ID)),
			fmt.Sprintf("  %s \"$@\"", def.LaunchFunc),
			"}",
			fmt.Sprintf("alias %s=%s", def.WrapperName, shellSingleQuote(def.WrapFunc)),
			fmt.Sprintf("__helm_boot_%s() {", sanitizeShellIdentifier(def.ID)),
			fmt.Sprintf("  __helm_emit_mode %s", shellSingleQuote(def.ID)),
			fmt.Sprintf("  %s", bootCommand),
			"}",
			"",
		)
	}
	return lines
}

func renderFishAdapterFunctions(definitions map[string]shellAdapterDefinition, requestedAgentID string, manualBootCommand string, useManagedLaunch bool) []string {
	items := orderedDefinitions(definitions)
	lines := []string{}
	for _, def := range items {
		bootCommand := def.LaunchFunc
		if def.ID == requestedAgentID && manualBootCommand != "" {
			bootCommand = manualBootCommand
		}
		lines = append(lines,
			fmt.Sprintf("function %s", def.LaunchFunc),
			fmt.Sprintf("  %s", fishCommandInvocation(def, useManagedLaunch)),
			"end",
			fmt.Sprintf("function %s", def.WrapperName),
			fmt.Sprintf("  __helm_emit_mode %s", def.ID),
			fmt.Sprintf("  %s $argv", def.LaunchFunc),
			"end",
			fmt.Sprintf("function __helm_boot_%s", sanitizeShellIdentifier(def.ID)),
			fmt.Sprintf("  __helm_emit_mode %s", def.ID),
			fmt.Sprintf("  %s", bootCommand),
			"end",
			"",
		)
	}
	return lines
}

func renderPathCases(definitions map[string]shellAdapterDefinition, indent string, emitPrefix string) []string {
	items := directMatchDefinitions(definitions)
	lines := []string{}
	for _, def := range items {
		lines = append(lines, fmt.Sprintf("%s%s)", indent, shellSingleQuote(def.Command)))
		lines = append(lines, fmt.Sprintf("%s  %s %s ;;", indent, emitPrefix, shellSingleQuote(def.ID)))
		if def.CommandName != "" && def.CommandName != def.Command {
			lines = append(lines, fmt.Sprintf("%s%s)", indent, shellSingleQuote(def.CommandName)))
			lines = append(lines, fmt.Sprintf("%s  %s %s ;;", indent, emitPrefix, shellSingleQuote(def.ID)))
		}
	}
	return lines
}

func renderFishPathCases(definitions map[string]shellAdapterDefinition) []string {
	items := directMatchDefinitions(definitions)
	lines := []string{}
	for _, def := range items {
		lines = append(lines,
			fmt.Sprintf("    case %s", fishSingleQuote(def.Command)),
			fmt.Sprintf("      echo %s", def.ID),
			"      return 0",
		)
		if def.CommandName != "" && def.CommandName != def.Command {
			lines = append(lines,
				fmt.Sprintf("    case %s", fishSingleQuote(def.CommandName)),
				fmt.Sprintf("      echo %s", def.ID),
				"      return 0",
			)
		}
	}
	return lines
}

func directMatchDefinitions(definitions map[string]shellAdapterDefinition) []shellAdapterDefinition {
	items := orderedDefinitions(definitions)
	commandCounts := map[string]int{}
	commandNameCounts := map[string]int{}
	for _, def := range items {
		if !def.DirectMatch {
			continue
		}
		if def.Command != "" {
			commandCounts[def.Command]++
		}
		if def.CommandName != "" {
			commandNameCounts[def.CommandName]++
		}
	}

	matches := make([]shellAdapterDefinition, 0, len(items))
	for _, def := range items {
		if !def.DirectMatch {
			continue
		}
		if commandCounts[def.Command] != 1 {
			continue
		}
		if def.CommandName != "" && commandNameCounts[def.CommandName] > 1 {
			def.CommandName = ""
		}
		matches = append(matches, def)
	}
	return matches
}

func nextShellWrapperName(id string, used map[string]int) string {
	base := preferredShellWrapperName(id)
	if count := used[base]; count > 0 {
		count++
		used[base] = count
		return fmt.Sprintf("%s-%d", base, count)
	}
	used[base] = 1
	return base
}

func preferredShellWrapperName(id string) string {
	id = strings.TrimSpace(id)
	if isShellCommandName(id) {
		return id
	}

	var builder strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}

	value := strings.Trim(builder.String(), "-_")
	if value == "" {
		value = "agent"
	}
	if !isShellCommandName(value) {
		value = "agent-" + sanitizeShellIdentifier(id)
	}
	return value
}

func isShellCommandName(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_' || r == '-':
		case r >= '0' && r <= '9':
			if index == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func orderedDefinitions(definitions map[string]shellAdapterDefinition) []shellAdapterDefinition {
	items := make([]shellAdapterDefinition, 0, len(definitions))
	for _, def := range definitions {
		items = append(items, def)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items
}

func sanitizeShellIdentifier(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	out := builder.String()
	if out == "" {
		return "session"
	}
	if out[0] >= '0' && out[0] <= '9' {
		return "session_" + out
	}
	return out
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func fishSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `\'`) + "'"
}

func bourneCommandInvocation(def shellAdapterDefinition, useManagedLaunch bool) string {
	parts := quotedShellCommandParts(def, useManagedLaunch, shellSingleQuote)
	parts = append(parts, `"$@"`)
	return strings.Join(parts, " ")
}

func fishCommandInvocation(def shellAdapterDefinition, useManagedLaunch bool) string {
	parts := quotedShellCommandParts(def, useManagedLaunch, fishSingleQuote)
	parts = append(parts, "$argv")
	return strings.Join(parts, " ")
}

func shellQuotedCommand(def shellAdapterDefinition, useManagedLaunch bool) string {
	return strings.Join(quotedShellCommandParts(def, useManagedLaunch, shellSingleQuote), " ")
}

func quotedShellCommandParts(def shellAdapterDefinition, useManagedLaunch bool, quote func(string) string) []string {
	parts := make([]string, 0, len(def.Env)+len(def.Args)+6)
	envKeys := make([]string, 0, len(def.Env))
	for key := range def.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	if len(envKeys) > 0 {
		parts = append(parts, "env")
		for _, key := range envKeys {
			parts = append(parts, key+"="+quote(def.Env[key]))
		}
	}
	if useManagedLaunch {
		parts = append(parts, "helm", "session-launch", "--")
	}
	parts = append(parts, quote(def.Command))
	for _, arg := range def.Args {
		parts = append(parts, quote(arg))
	}
	return parts
}

func launchEnvOverrides(baseEnv, targetEnv []string) map[string]string {
	base := launchEnvMap(baseEnv)
	target := launchEnvMap(targetEnv)
	if len(target) == 0 {
		return map[string]string{}
	}

	overrides := make(map[string]string)
	for key, value := range target {
		if baseValue, ok := base[key]; ok && baseValue == value {
			continue
		}
		overrides[key] = value
	}
	return overrides
}

func launchEnvMap(items []string) map[string]string {
	out := make(map[string]string, len(items))
	for _, item := range items {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}
