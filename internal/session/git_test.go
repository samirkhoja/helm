package session

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDescribeWorkspaceIncludesGitBranch(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	root := newGitRepo(t, gitPath)
	runGit(t, gitPath, root, "checkout", "-b", "feature/helm")

	choice, err := DescribeWorkspace(root)
	if err != nil {
		t.Fatalf("DescribeWorkspace() error = %v", err)
	}
	if choice.Name == "" {
		t.Fatalf("DescribeWorkspace() name is empty")
	}
	if choice.GitBranch != "feature/helm" {
		t.Fatalf("DescribeWorkspace() git branch = %q, want feature/helm", choice.GitBranch)
	}
}

func TestDescribeRepoSelectionIncludesExistingWorktrees(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	root := newGitRepo(t, gitPath)
	worktreeRoot := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-feature")
	runGit(t, gitPath, root, "worktree", "add", "-b", "feature/helm", worktreeRoot, "HEAD")

	selection, err := describeRepoSelection(worktreeRoot)
	if err != nil {
		t.Fatalf("describeRepoSelection() error = %v", err)
	}

	if !selection.Repo.IsGitRepo {
		t.Fatalf("selection.Repo.IsGitRepo = false, want true")
	}
	if len(selection.Repo.Worktrees) != 2 {
		t.Fatalf("selection.Repo.Worktrees = %#v, want 2", selection.Repo.Worktrees)
	}
	if !samePath(t, selection.SelectedWorktree.RootPath, worktreeRoot) {
		t.Fatalf("selected worktree = %q, want %q", selection.SelectedWorktree.RootPath, worktreeRoot)
	}
	if selection.SelectedWorktree.GitBranch != "feature/helm" {
		t.Fatalf("selected worktree branch = %q, want feature/helm", selection.SelectedWorktree.GitBranch)
	}
}
