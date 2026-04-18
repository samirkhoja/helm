package session

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const defaultCommitHistoryLimit = 24

func (m *Manager) CreateWorktreeBranch(worktreeID int, branchName string) (AppSnapshot, error) {
	rootPath, err := m.worktreeRootPath(worktreeID)
	if err != nil {
		return AppSnapshot{}, err
	}

	nextBranch, err := gitCreateBranch(rootPath, branchName)
	if err != nil {
		return AppSnapshot{}, err
	}

	m.mu.Lock()
	worktree := m.findWorktreeByIDLocked(worktreeID)
	if worktree == nil {
		snapshot := m.snapshotLocked()
		m.mu.Unlock()
		return snapshot, fmt.Errorf("worktree %d not found", worktreeID)
	}
	worktree.GitBranch = nextBranch
	snapshot := m.snapshotLocked()
	m.mu.Unlock()

	m.sink.Emit(EventAppSnapshot, snapshot)
	return snapshot, nil
}

func (m *Manager) StageWorktreeAll(worktreeID int) (GitActionResult, error) {
	rootPath, err := m.worktreeRootPath(worktreeID)
	if err != nil {
		return GitActionResult{}, err
	}
	if err := gitStageAll(rootPath); err != nil {
		return GitActionResult{}, err
	}
	return GitActionResult{Message: "Staged all changes."}, nil
}

func (m *Manager) CommitWorktree(worktreeID int, message string) (GitActionResult, error) {
	rootPath, err := m.worktreeRootPath(worktreeID)
	if err != nil {
		return GitActionResult{}, err
	}

	shortHash, err := gitCommit(rootPath, message)
	if err != nil {
		return GitActionResult{}, err
	}

	if shortHash == "" {
		return GitActionResult{Message: "Created commit."}, nil
	}
	return GitActionResult{Message: fmt.Sprintf("Created commit %s.", shortHash)}, nil
}

func (m *Manager) PushWorktree(worktreeID int) (GitActionResult, error) {
	rootPath, branchName, err := m.worktreeRootPathAndBranch(worktreeID)
	if err != nil {
		return GitActionResult{}, err
	}
	if branchName == "" || branchName == noGitBranch || branchName == detachedHead {
		return GitActionResult{}, fmt.Errorf("switch to a branch before pushing")
	}

	remoteName, setUpstream, err := gitPushCurrentBranch(rootPath, branchName)
	if err != nil {
		return GitActionResult{}, err
	}
	if setUpstream {
		return GitActionResult{
			Message: fmt.Sprintf("Pushed %s and set upstream to %s.", branchName, remoteName),
		}, nil
	}
	return GitActionResult{
		Message: fmt.Sprintf("Pushed %s to %s.", branchName, remoteName),
	}, nil
}

func (m *Manager) WorktreeCommitHistory(worktreeID, limit int) ([]GitCommitSummary, error) {
	rootPath, err := m.worktreeRootPath(worktreeID)
	if err != nil {
		return nil, err
	}
	return gitCommitHistory(rootPath, limit)
}

func (m *Manager) CompareCommits(worktreeID int, baseRef, headRef string) (CommitDiff, error) {
	rootPath, err := m.worktreeRootPath(worktreeID)
	if err != nil {
		return CommitDiff{}, err
	}
	return gitCommitDiff(worktreeID, rootPath, baseRef, headRef)
}

func (m *Manager) worktreeRootPath(worktreeID int) (string, error) {
	rootPath, _, err := m.worktreeRootPathAndBranch(worktreeID)
	return rootPath, err
}

func (m *Manager) worktreeRootPathAndBranch(worktreeID int) (string, string, error) {
	m.mu.RLock()
	worktree := m.findWorktreeByIDLocked(worktreeID)
	m.mu.RUnlock()
	if worktree == nil {
		return "", "", fmt.Errorf("worktree %d not found", worktreeID)
	}

	rootPath := worktree.RootPath
	currentBranch := detectGitBranch(rootPath)
	cachedBranch := worktree.GitBranch
	if currentBranch == cachedBranch {
		return rootPath, currentBranch, nil
	}

	var snapshot AppSnapshot
	branchChanged := false

	m.mu.Lock()
	worktree = m.findWorktreeByIDLocked(worktreeID)
	if worktree == nil {
		m.mu.Unlock()
		return "", "", fmt.Errorf("worktree %d not found", worktreeID)
	}

	rootPath = worktree.RootPath
	if worktree.GitBranch != currentBranch {
		worktree.GitBranch = currentBranch
		snapshot = m.snapshotLocked()
		branchChanged = true
	}
	m.mu.Unlock()

	if branchChanged {
		m.sink.Emit(EventAppSnapshot, snapshot)
	}

	return rootPath, currentBranch, nil
}

func gitCreateBranch(rootPath, branchName string) (string, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return "", fmt.Errorf("branch name is required")
	}
	if _, err := runGitText(rootPath, "checkout", "-b", branchName); err != nil {
		return "", err
	}

	nextBranch := detectGitBranch(rootPath)
	if nextBranch == "" || nextBranch == noGitBranch {
		nextBranch = branchName
	}
	return nextBranch, nil
}

func gitStageAll(rootPath string) error {
	_, err := runGitText(rootPath, "add", "-A")
	return err
}

func gitCommit(rootPath, message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", fmt.Errorf("commit message is required")
	}

	if _, err := runGitText(rootPath, "commit", "-m", message); err != nil {
		return "", err
	}

	shortHash, err := runGitText(rootPath, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(shortHash), nil
}

func gitPushCurrentBranch(rootPath, branchName string) (string, bool, error) {
	upstreamRef, err := gitCurrentUpstream(rootPath)
	if err == nil && upstreamRef != "" {
		if _, err := runGitText(rootPath, "push"); err != nil {
			return "", false, err
		}
		return upstreamRemoteName(upstreamRef), false, nil
	}

	remoteName, err := gitDefaultPushRemote(rootPath, branchName)
	if err != nil {
		return "", false, err
	}
	if _, err := runGitText(rootPath, "push", "--set-upstream", remoteName, branchName); err != nil {
		return "", false, err
	}

	upstreamRef, err = gitCurrentUpstream(rootPath)
	if err != nil {
		return remoteName, true, nil
	}
	return upstreamRemoteName(upstreamRef), true, nil
}

func gitCurrentUpstream(rootPath string) (string, error) {
	value, err := runGitText(rootPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func upstreamRemoteName(upstreamRef string) string {
	upstreamRef = strings.TrimSpace(upstreamRef)
	if upstreamRef == "" {
		return "remote"
	}
	parts := strings.SplitN(upstreamRef, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "remote"
	}
	return parts[0]
}

func gitDefaultPushRemote(rootPath, branchName string) (string, error) {
	remotes, err := gitRemoteNames(rootPath)
	if err != nil {
		return "", err
	}
	if len(remotes) == 0 {
		return "", fmt.Errorf("no git remotes are configured")
	}

	candidates := []string{
		gitConfigValue(rootPath, fmt.Sprintf("branch.%s.pushRemote", branchName)),
		gitConfigValue(rootPath, "remote.pushDefault"),
		gitConfigValue(rootPath, fmt.Sprintf("branch.%s.remote", branchName)),
	}

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		for _, remote := range remotes {
			if remote == candidate {
				return remote, nil
			}
		}
	}

	if len(remotes) == 1 {
		return remotes[0], nil
	}
	for _, remote := range remotes {
		if remote == "origin" {
			return remote, nil
		}
	}

	return "", fmt.Errorf("multiple git remotes are configured; set a push remote or upstream in the terminal first")
}

func gitRemoteNames(rootPath string) ([]string, error) {
	remotesOutput, err := runGitText(rootPath, "remote")
	if err != nil {
		return nil, err
	}

	remotes := make([]string, 0)
	for _, line := range strings.Split(remotesOutput, "\n") {
		remote := strings.TrimSpace(line)
		if remote == "" {
			continue
		}
		remotes = append(remotes, remote)
	}
	return remotes, nil
}

func gitConfigValue(rootPath, key string) string {
	value, err := runGitText(rootPath, "config", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func gitCommitHistory(rootPath string, limit int) ([]GitCommitSummary, error) {
	if limit <= 0 {
		limit = defaultCommitHistoryLimit
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git is not installed")
	}

	cmd := exec.Command(
		gitPath,
		"-C",
		rootPath,
		"log",
		fmt.Sprintf("--max-count=%d", limit),
		"--date=short",
		"--pretty=format:%H%x1f%h%x1f%an%x1f%ad%x1f%s",
	)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if strings.Contains(text, "does not have any commits yet") {
			return nil, nil
		}
		if text != "" {
			return nil, fmt.Errorf("%s", text)
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	items := make([]GitCommitSummary, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 5)
		if len(parts) != 5 {
			continue
		}
		items = append(items, GitCommitSummary{
			Hash:       parts[0],
			ShortHash:  parts[1],
			AuthorName: parts[2],
			AuthorDate: parts[3],
			Subject:    parts[4],
		})
	}
	return items, nil
}

func gitCommitDiff(worktreeID int, rootPath, baseRef, headRef string) (CommitDiff, error) {
	diff := CommitDiff{
		WorktreeID: worktreeID,
		BaseRef:    strings.TrimSpace(baseRef),
		HeadRef:    strings.TrimSpace(headRef),
	}
	if diff.BaseRef == "" || diff.HeadRef == "" {
		return diff, fmt.Errorf("select two commits to compare")
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		diff.Message = "git is not installed"
		return diff, nil
	}

	cmd := exec.Command(
		gitPath,
		"-C",
		rootPath,
		"diff",
		"--no-color",
		"--patch",
		"--find-renames",
		diff.BaseRef,
		diff.HeadRef,
	)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text != "" {
			return diff, fmt.Errorf("%s", text)
		}
		return diff, err
	}

	diff.Patch = strings.TrimRight(string(bytes.TrimSpace(out)), "\n")
	if diff.Patch == "" {
		diff.Message = "No diff between the selected commits."
	}
	return diff, nil
}

func runGitText(rootPath string, args ...string) (string, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git is not installed")
	}

	cmd := exec.Command(gitPath, append([]string{"-C", rootPath}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text != "" {
			return "", fmt.Errorf("%s", text)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
