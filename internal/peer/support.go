package peer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	supportRootEnv  = "HELM_PEER_SUPPORT_ROOT"
	DefaultScopeEnv = "HELM_PEER_DEFAULT_SCOPE"
	skillName       = "helm-peers"
)

type LaunchSupport struct {
	ExtraArgs []string
	Env       map[string]string
}

type SupportManager struct {
	root           string
	executablePath string

	bundleOnce  sync.Once
	bundleErr   error
	wrapperOnce sync.Once
	wrapperErr  error

	projMu   sync.Mutex
	projDone map[string]struct{}
}

func DefaultSupportRoot() (string, error) {
	if value := strings.TrimSpace(os.Getenv(supportRootEnv)); value != "" {
		return filepath.Clean(value), nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "helm", "peer-support", "v1"), nil
}

func NewSupportManager(executablePath string) (*SupportManager, error) {
	root, err := DefaultSupportRoot()
	if err != nil {
		return nil, err
	}
	return &SupportManager{
		root:           root,
		executablePath: executablePath,
		projDone:       map[string]struct{}{},
	}, nil
}

func (m *SupportManager) Root() string {
	if m == nil {
		return ""
	}
	return m.root
}

func (m *SupportManager) PrepareLaunch(family string, env []string) (LaunchSupport, error) {
	if m == nil {
		return LaunchSupport{}, nil
	}

	m.bundleOnce.Do(func() { m.bundleErr = m.ensureCanonicalBundle() })
	if m.bundleErr != nil {
		return LaunchSupport{}, m.bundleErr
	}

	m.wrapperOnce.Do(func() { m.wrapperErr = m.ensureCLIWrapper() })
	if m.wrapperErr != nil {
		return LaunchSupport{}, m.wrapperErr
	}

	normalized := NormalizeFamily(family)
	if err := m.ensureProjectionOnce(normalized, env); err != nil {
		return LaunchSupport{}, err
	}

	launch := LaunchSupport{
		Env: map[string]string{
			"HELM_PEER_SUPPORT_ROOT": m.root,
			"PATH":                   prependPath(filepath.Join(m.root, "bin"), envPathValue(env)),
		},
	}
	if normalized == FamilyAider {
		launch.ExtraArgs = append(launch.ExtraArgs, "--read", filepath.Join(m.root, "aider", "HELM-PEERS.md"))
	}
	return launch, nil
}

func (m *SupportManager) ensureProjectionOnce(family string, env []string) error {
	key, err := projectionCacheKey(family, env)
	if err != nil {
		return err
	}

	m.projMu.Lock()
	_, done := m.projDone[key]
	m.projMu.Unlock()
	if done {
		return nil
	}

	if err := m.ensureProjection(family, env); err != nil {
		return err
	}

	m.projMu.Lock()
	m.projDone[key] = struct{}{}
	m.projMu.Unlock()
	return nil
}

func (m *SupportManager) ensureCanonicalBundle() error {
	for _, sub := range []string{"references", "agents"} {
		if err := os.MkdirAll(filepath.Join(m.root, skillName, sub), 0o755); err != nil {
			return fmt.Errorf("create peer support directories: %w", err)
		}
	}
	if err := writeFileIfChanged(filepath.Join(m.root, skillName, "SKILL.md"), canonicalSkillMarkdown(), 0o644); err != nil {
		return err
	}
	if err := writeFileIfChanged(filepath.Join(m.root, skillName, "references", "peer-protocol.md"), canonicalProtocolReference(), 0o644); err != nil {
		return err
	}
	if err := writeFileIfChanged(filepath.Join(m.root, skillName, "agents", "openai.yaml"), codexAgentManifest(), 0o644); err != nil {
		return err
	}
	return nil
}

func (m *SupportManager) ensureCLIWrapper() error {
	binDir := filepath.Join(m.root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create support bin dir: %w", err)
	}

	executablePath := strings.TrimSpace(m.executablePath)
	if executablePath == "" {
		var err error
		executablePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("resolve helm executable path: %w", err)
		}
	}

	if runtime.GOOS == "windows" {
		content := fmt.Sprintf("@echo off\r\n\"%s\" %%*\r\n", executablePath)
		return writeFileIfChanged(filepath.Join(binDir, "helm.cmd"), content, 0o755)
	}

	content := fmt.Sprintf("#!/bin/sh\nexec \"%s\" \"$@\"\n", executablePath)
	return writeFileIfChanged(filepath.Join(binDir, "helm"), content, 0o755)
}

func (m *SupportManager) ensureProjection(family string, env []string) error {
	switch NormalizeFamily(family) {
	case FamilyClaude:
		return copyDirectory(filepath.Join(m.root, skillName), filepath.Join(userHomeDir(), ".claude", "skills", skillName))
	case FamilyCodex:
		target, err := codexSkillTarget(env)
		if err != nil {
			return err
		}
		return copyDirectory(filepath.Join(m.root, skillName), target)
	case FamilyCopilot:
		return copyDirectory(filepath.Join(m.root, skillName), filepath.Join(userHomeDir(), ".copilot", "skills", skillName))
	case FamilyKiro:
		return copyDirectory(filepath.Join(m.root, skillName), filepath.Join(userHomeDir(), ".kiro", "skills", skillName))
	case FamilyOpenCode:
		configRoot, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		return copyDirectory(filepath.Join(m.root, skillName), filepath.Join(configRoot, "opencode", "skills", skillName))
	case FamilyGemini:
		path := filepath.Join(m.root, "gemini", "GEMINI.md")
		return writeProjectionFile(path, geminiWrapper())
	case FamilyCursor:
		path := filepath.Join(m.root, "cursor", "AGENTS.md")
		return writeProjectionFile(path, cursorWrapper())
	case FamilyAider:
		path := filepath.Join(m.root, "aider", "HELM-PEERS.md")
		return writeProjectionFile(path, aiderWrapper())
	default:
		return nil
	}
}

func writeProjectionFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create projection directory: %w", err)
	}
	return writeFileIfChanged(path, content, 0o644)
}

func writeFileIfChanged(path, content string, perm os.FileMode) error {
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == content {
		if perm&0o111 != 0 {
			_ = os.Chmod(path, perm)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func copyDirectory(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read support source %s: %w", srcDir, err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create skill target %s: %w", dstDir, err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		if entry.IsDir() {
			if err := copyDirectory(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read support file %s: %w", srcPath, err)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat support file %s: %w", srcPath, err)
		}
		if err := writeFileIfChanged(dstPath, string(data), info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func envPathValue(env []string) string {
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok && key == "PATH" {
			return value
		}
	}
	return os.Getenv("PATH")
}

func prependPath(dir, existing string) string {
	if dir == "" {
		return existing
	}
	if existing == "" {
		return dir
	}
	return dir + string(os.PathListSeparator) + existing
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		return home
	}
	return ""
}

func projectionCacheKey(family string, env []string) (string, error) {
	switch NormalizeFamily(family) {
	case FamilyCodex:
		target, err := codexSkillTarget(env)
		if err != nil {
			return "", err
		}
		return family + "|" + target, nil
	default:
		return family, nil
	}
}

func codexSkillTarget(env []string) (string, error) {
	if value := strings.TrimSpace(envValue(env, "CODEX_HOME")); value != "" {
		return filepath.Join(filepath.Clean(value), "skills", skillName), nil
	}

	home := strings.TrimSpace(envValue(env, "HOME"))
	if home == "" {
		home = userHomeDir()
	}
	if home == "" {
		return "", fmt.Errorf("resolve codex skill target: HOME is unset")
	}

	return filepath.Join(filepath.Clean(home), ".codex", "skills", skillName), nil
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

func canonicalSkillMarkdown() string {
	return "---\n" +
		"name: helm-peers\n" +
		"description: Use when you need to coordinate with another Helm-managed agent, respond to a HELM_PEER_EVENT block, ask a peer for codebase or API details, or publish a short summary of your current work over Helm's peer network.\n" +
		"---\n\n" +
		"# Helm Peers\n\n" +
		"Helm peer messaging is the standard way to coordinate with another Helm-managed agent.\n\n" +
		"## When To Use It\n\n" +
		"- Ask another agent for focused information from their repo or worktree.\n" +
		"- Notify another agent about API changes, schema changes, or handoff details.\n" +
		"- Reply to a `HELM_PEER_EVENT` block printed into the terminal.\n\n" +
		"## Runtime Protocol\n\n" +
		"When Helm delivers a peer message, it prints a machine-shaped block with the tag `HELM_PEER_EVENT`.\n\n" +
		"1. Stop and read the event block.\n" +
		"2. Run the exact `next_command` from the event.\n" +
		"3. Reply with `helm peers send --to <peer-id> --reply-to <message-id> --message \"<text>\"` when a response is needed.\n" +
		"4. Otherwise acknowledge it with `helm peers ack --message-id <message-id>`, or explicitly mark it read with `helm peers inbox --message-id <message-id> --mark-read`.\n\n" +
		"## Waiting For Replies\n\n" +
		"- After you send a peer message, do not poll for a response.\n" +
		"- Continue your current task unless the user explicitly asked you to monitor or wait.\n" +
		"- Helm will deliver a response with a new `HELM_PEER_EVENT` block if the other agent replies.\n" +
		"- Only use `helm peers inbox` proactively to review backlog or recover a missed message.\n\n" +
		"## Sandboxed Sessions\n\n" +
		"- `helm peers list`, `helm peers --help`, and plain `helm peers inbox` are read-only.\n" +
		"- `helm peers send`, `helm peers ack`, `helm peers set-summary`, and `helm peers inbox --mark-read` are write operations.\n" +
		"- In restricted/sandboxed sessions, request approval before running peer write operations instead of trying them once and retrying after failure.\n\n" +
		"## Discovery Scopes\n\n" +
		"- `repo` finds peers in the current repo.\n" +
		"- `worktree` finds peers in the current worktree only.\n" +
		"- `machine` finds peers across the whole machine.\n\n" +
		"## Commands\n\n" +
		"- Show all peer command help: `helm peers --help`\n" +
		"- List peers in the current repo: `helm peers list --scope repo`\n" +
		"- List peers in the current worktree only: `helm peers list --scope worktree`\n" +
		"- List peers across the whole machine: `helm peers list --scope machine`\n" +
		"- Show list help: `helm peers list --help`\n" +
		"- Read inbox: `helm peers inbox --limit 20`\n" +
		"- Read one message: `helm peers inbox --message-id <message-id>`\n" +
		"- Mark read without replying: `helm peers inbox --message-id <message-id> --mark-read`\n" +
		"- Show inbox help: `helm peers inbox --help`\n" +
		"- Reply to the sender of an inbound message: `helm peers send --to <peer-id> --reply-to <message-id> --message \"<text>\"`\n" +
		"- Send to a peer in repo scope: `helm peers send --scope repo --to <peer-id> --message \"<text>\"`\n" +
		"- Send to a peer in worktree scope: `helm peers send --scope worktree --to <peer-id> --message \"<text>\"`\n" +
		"- Send to a peer in machine scope: `helm peers send --scope machine --to <peer-id> --message \"<text>\"`\n" +
		"- Show send help: `helm peers send --help`\n" +
		"- Acknowledge: `helm peers ack --message-id <message-id>`\n" +
		"- Show ack help: `helm peers ack --help`\n" +
		"- Update summary: `helm peers set-summary --summary \"<short status>\"`\n\n" +
		"- Show summary help: `helm peers set-summary --help`\n\n" +
		"## Etiquette\n\n" +
		"- Keep messages short and concrete.\n" +
		"- Include the exact API, file, or question you need help with.\n" +
		"- Always tie replies back to the original `message_id`.\n" +
		"- Update your summary when your focus changes materially.\n\n" +
		"For the protocol details, see [references/peer-protocol.md](references/peer-protocol.md).\n"
}

func canonicalProtocolReference() string {
	return "# Helm Peer Protocol\n\n" +
		"Helm sends peer notifications to local sessions through a terminal envelope:\n\n" +
		"```text\n" +
		"[HELM_PEER_EVENT v1]\n" +
		"message_id=842\n" +
		"from_peer=backend-codex\n" +
		"from_label=Backend Agent\n" +
		"kind=message\n" +
		"ack_required=true\n" +
		"unread_count=3\n" +
		"next_command=helm peers inbox --message-id 842\n" +
		"preview=Tell the client agent the API changed: POST /v2/tasks now requires project_id.\n" +
		"[/HELM_PEER_EVENT]\n" +
		"```\n\n" +
		"The `next_command` is authoritative. Run it exactly. Reply with `helm peers send --reply-to <message_id>`, acknowledge with `helm peers ack --message-id <message_id>`, or mark it read with `helm peers inbox --message-id <message_id> --mark-read`. Do not poll for a response after sending; wait for a future `HELM_PEER_EVENT`.\n"
}

func geminiWrapper() string {
	return "# Helm Peers\n\n" +
		"If Helm peer messaging is enabled for this session, use the Helm CLI for coordination:\n\n" +
		"- `helm peers --help`\n" +
		"- `helm peers list --scope repo`\n" +
		"- `helm peers list --scope worktree`\n" +
		"- `helm peers list --scope machine`\n" +
		"- `helm peers inbox --limit 20`\n" +
		"- `helm peers inbox --message-id <message-id> --mark-read`\n" +
		"- `helm peers send --scope repo --to <peer-id> --message \"<text>\"`\n" +
		"- `helm peers send --scope worktree --to <peer-id> --message \"<text>\"`\n" +
		"- `helm peers send --scope machine --to <peer-id> --message \"<text>\"`\n" +
		"- `helm peers send --to <peer-id> --reply-to <message-id> --message \"<text>\"`\n" +
		"- `helm peers ack --message-id <message-id>`\n\n" +
		"When the terminal prints a `HELM_PEER_EVENT` block, stop and run the `next_command` from that block.\n"
}

func cursorWrapper() string {
	return "# Helm Peers\n\n" +
		"This session may receive peer messages from other Helm-managed agents.\n\n" +
		"When a `HELM_PEER_EVENT` block appears in terminal output:\n\n" +
		"1. Stop and read the block.\n" +
		"2. Run the exact `next_command` shown in the block.\n" +
		"3. Reply with `helm peers send --to <peer-id> --reply-to <message-id> --message \"<text>\"`, or acknowledge with `helm peers ack --message-id <message-id>`.\n"
}

func codexAgentManifest() string {
	return "interface:\n" +
		"  display_name: \"Helm Peers\"\n" +
		"  short_description: \"Coordinate with other Helm-managed agents via peer messaging\"\n"
}

func aiderWrapper() string {
	return "# Helm Peers\n\n" +
		"Use the Helm CLI to coordinate with other Helm-managed agents:\n\n" +
		"- `helm peers --help`\n" +
		"- `helm peers list --scope repo`\n" +
		"- `helm peers list --scope worktree`\n" +
		"- `helm peers list --scope machine`\n" +
		"- `helm peers inbox --limit 20`\n" +
		"- `helm peers inbox --message-id <message-id> --mark-read`\n" +
		"- `helm peers send --scope repo --to <peer-id> --message \"<text>\"`\n" +
		"- `helm peers send --scope worktree --to <peer-id> --message \"<text>\"`\n" +
		"- `helm peers send --scope machine --to <peer-id> --message \"<text>\"`\n" +
		"- `helm peers send --to <peer-id> --reply-to <message-id> --message \"<text>\"`\n" +
		"- `helm peers ack --message-id <message-id>`\n\n" +
		"If a `HELM_PEER_EVENT` block appears in terminal output, run its `next_command` immediately.\n"
}
