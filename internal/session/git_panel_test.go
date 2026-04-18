package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerCreateWorktreeBranchUpdatesSnapshot(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	manager := NewManager(newTestRegistry(t), os.Environ(), newFakeStarter(), &fakeSink{}, newTestStore(t))
	repoRoot := newGitRepo(t, gitPath)

	snapshot, err := manager.CreateWorkspaceSession(repoRoot, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}

	worktree := snapshot.Repos[0].Worktrees[0]
	nextSnapshot, err := manager.CreateWorktreeBranch(worktree.ID, "feature/diff-rail")
	if err != nil {
		t.Fatalf("CreateWorktreeBranch() error = %v", err)
	}

	if got := nextSnapshot.Repos[0].Worktrees[0].GitBranch; got != "feature/diff-rail" {
		t.Fatalf("GitBranch = %q, want feature/diff-rail", got)
	}

	if got := detectGitBranchWithPath(gitPath, repoRoot); got != "feature/diff-rail" {
		t.Fatalf("detectGitBranchWithPath() = %q, want feature/diff-rail", got)
	}
}

func TestManagerGitPanelActionsAndHistory(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	manager := NewManager(newTestRegistry(t), os.Environ(), newFakeStarter(), &fakeSink{}, newTestStore(t))
	repoRoot := newGitRepo(t, gitPath)
	remoteRoot := filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(remoteRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(remoteRoot) error = %v", err)
	}
	runGit(t, gitPath, remoteRoot, "init", "--bare")
	runGit(t, gitPath, repoRoot, "remote", "add", "origin", remoteRoot)

	snapshot, err := manager.CreateWorkspaceSession(repoRoot, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	worktree := snapshot.Repos[0].Worktrees[0]

	readmePath := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(readmePath, []byte("helm\nmore\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "notes.txt"), []byte("note\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	stageResult, err := manager.StageWorktreeAll(worktree.ID)
	if err != nil {
		t.Fatalf("StageWorktreeAll() error = %v", err)
	}
	if stageResult.Message == "" {
		t.Fatalf("StageWorktreeAll() message = empty, want success text")
	}

	diff, err := manager.WorktreeDiff(worktree.ID)
	if err != nil {
		t.Fatalf("WorktreeDiff() error = %v", err)
	}
	if len(diff.Staged) == 0 {
		t.Fatalf("diff.Staged = %#v, want staged changes", diff.Staged)
	}
	if len(diff.Unstaged) != 0 || len(diff.Untracked) != 0 {
		t.Fatalf("unexpected unstaged/untracked changes after stage all: %#v", diff)
	}

	commitResult, err := manager.CommitWorktree(worktree.ID, "Add notes")
	if err != nil {
		t.Fatalf("CommitWorktree() error = %v", err)
	}
	if !strings.Contains(commitResult.Message, "Created commit") {
		t.Fatalf("CommitWorktree() message = %q, want Created commit", commitResult.Message)
	}

	history, err := manager.WorktreeCommitHistory(worktree.ID, 10)
	if err != nil {
		t.Fatalf("WorktreeCommitHistory() error = %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("history = %#v, want at least two commits", history)
	}
	if history[0].Subject != "Add notes" {
		t.Fatalf("history[0].Subject = %q, want Add notes", history[0].Subject)
	}

	pushResult, err := manager.PushWorktree(worktree.ID)
	if err != nil {
		t.Fatalf("PushWorktree() error = %v", err)
	}
	if !strings.Contains(pushResult.Message, "Pushed") {
		t.Fatalf("PushWorktree() message = %q, want Pushed", pushResult.Message)
	}

	branchName := detectGitBranchWithPath(gitPath, repoRoot)
	runGit(t, gitPath, remoteRoot, "rev-parse", "--verify", "refs/heads/"+branchName)

	compareDiff, err := manager.CompareCommits(worktree.ID, history[1].Hash, history[0].Hash)
	if err != nil {
		t.Fatalf("CompareCommits() error = %v", err)
	}
	if !strings.Contains(compareDiff.Patch, "README.md") || !strings.Contains(compareDiff.Patch, "notes.txt") {
		t.Fatalf("CompareCommits().Patch = %q, want README.md and notes.txt", compareDiff.Patch)
	}
}

func TestManagerPushWorktreeUsesGitRemoteResolution(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	manager := NewManager(newTestRegistry(t), os.Environ(), newFakeStarter(), &fakeSink{}, newTestStore(t))
	repoRoot := newGitRepo(t, gitPath)

	originRoot := filepath.Join(t.TempDir(), "origin.git")
	if err := os.MkdirAll(originRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(originRoot) error = %v", err)
	}
	runGit(t, gitPath, originRoot, "init", "--bare")

	backupRoot := filepath.Join(t.TempDir(), "backup.git")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(backupRoot) error = %v", err)
	}
	runGit(t, gitPath, backupRoot, "init", "--bare")

	runGit(t, gitPath, repoRoot, "remote", "add", "origin", originRoot)
	runGit(t, gitPath, repoRoot, "remote", "add", "backup", backupRoot)
	runGit(t, gitPath, repoRoot, "config", "remote.pushDefault", "backup")

	snapshot, err := manager.CreateWorkspaceSession(repoRoot, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	worktree := snapshot.Repos[0].Worktrees[0]

	if err := os.WriteFile(filepath.Join(repoRoot, "push.txt"), []byte("push\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(push.txt) error = %v", err)
	}

	if _, err := manager.StageWorktreeAll(worktree.ID); err != nil {
		t.Fatalf("StageWorktreeAll() error = %v", err)
	}
	if _, err := manager.CommitWorktree(worktree.ID, "Push target"); err != nil {
		t.Fatalf("CommitWorktree() error = %v", err)
	}

	result, err := manager.PushWorktree(worktree.ID)
	if err != nil {
		t.Fatalf("PushWorktree() error = %v", err)
	}
	if !strings.Contains(result.Message, "backup") {
		t.Fatalf("PushWorktree() message = %q, want backup remote", result.Message)
	}

	branchName := detectGitBranchWithPath(gitPath, repoRoot)
	runGit(t, gitPath, backupRoot, "rev-parse", "--verify", "refs/heads/"+branchName)

	cmd := exec.Command(
		gitPath,
		"-C",
		originRoot,
		"show-ref",
		"--verify",
		"--quiet",
		"refs/heads/"+branchName,
	)
	if err := cmd.Run(); err == nil {
		t.Fatalf("origin unexpectedly has branch %s", branchName)
	}
}

func TestManagerGitPanelTracksLiveBranchAfterManualCheckout(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	manager := NewManager(newTestRegistry(t), os.Environ(), newFakeStarter(), &fakeSink{}, newTestStore(t))
	repoRoot := newGitRepo(t, gitPath)
	remoteRoot := filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(remoteRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(remoteRoot) error = %v", err)
	}
	runGit(t, gitPath, remoteRoot, "init", "--bare")
	runGit(t, gitPath, repoRoot, "remote", "add", "origin", remoteRoot)

	snapshot, err := manager.CreateWorkspaceSession(repoRoot, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	worktree := snapshot.Repos[0].Worktrees[0]
	originalBranch := worktree.GitBranch

	runGit(t, gitPath, repoRoot, "checkout", "-b", "feature/manual-checkout")

	diff, err := manager.WorktreeDiff(worktree.ID)
	if err != nil {
		t.Fatalf("WorktreeDiff() error = %v", err)
	}
	if diff.GitBranch != "feature/manual-checkout" {
		t.Fatalf("WorktreeDiff().GitBranch = %q, want feature/manual-checkout", diff.GitBranch)
	}
	if diff.GitBranch == originalBranch {
		t.Fatalf("WorktreeDiff().GitBranch = %q, want branch refreshed from %q", diff.GitBranch, originalBranch)
	}

	refreshedWorktree := manager.Snapshot().Repos[0].Worktrees[0]
	if refreshedWorktree.GitBranch != "feature/manual-checkout" {
		t.Fatalf("Snapshot().Repos[0].Worktrees[0].GitBranch = %q, want feature/manual-checkout", refreshedWorktree.GitBranch)
	}

	if err := os.WriteFile(filepath.Join(repoRoot, "manual.txt"), []byte("manual\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(manual.txt) error = %v", err)
	}
	if _, err := manager.StageWorktreeAll(worktree.ID); err != nil {
		t.Fatalf("StageWorktreeAll() error = %v", err)
	}
	if _, err := manager.CommitWorktree(worktree.ID, "Manual checkout branch"); err != nil {
		t.Fatalf("CommitWorktree() error = %v", err)
	}

	result, err := manager.PushWorktree(worktree.ID)
	if err != nil {
		t.Fatalf("PushWorktree() error = %v", err)
	}
	if !strings.Contains(result.Message, "feature/manual-checkout") {
		t.Fatalf("PushWorktree() message = %q, want feature/manual-checkout", result.Message)
	}

	runGit(t, gitPath, remoteRoot, "rev-parse", "--verify", "refs/heads/feature/manual-checkout")
}
