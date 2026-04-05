package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWorktreeDiffReportsStagedUnstagedAndUntracked(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	root := t.TempDir()
	runGit(t, gitPath, root, "init")
	runGit(t, gitPath, root, "config", "user.email", "helm@example.com")
	runGit(t, gitPath, root, "config", "user.name", "Helm Test")

	tracked := filepath.Join(root, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(tracked) error = %v", err)
	}
	runGit(t, gitPath, root, "add", "tracked.txt")
	runGit(t, gitPath, root, "commit", "-m", "initial")

	if err := os.WriteFile(tracked, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(modified tracked) error = %v", err)
	}
	runGit(t, gitPath, root, "add", "tracked.txt")

	if err := os.WriteFile(tracked, []byte("one\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(unstaged tracked) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(untracked) error = %v", err)
	}

	diff, err := loadWorktreeDiff(9, root, "main")
	if err != nil {
		t.Fatalf("loadWorktreeDiff() error = %v", err)
	}

	if !diff.IsGitRepo {
		t.Fatalf("diff.IsGitRepo = false, want true")
	}
	if len(diff.Staged) == 0 {
		t.Fatalf("diff.Staged = %#v, want staged change", diff.Staged)
	}
	if len(diff.Unstaged) == 0 {
		t.Fatalf("diff.Unstaged = %#v, want unstaged change", diff.Unstaged)
	}
	if len(diff.Untracked) != 1 || diff.Untracked[0] != "untracked.txt" {
		t.Fatalf("diff.Untracked = %#v, want untracked.txt", diff.Untracked)
	}
	if diff.StagedPatch != "" {
		t.Fatalf("diff.StagedPatch = %q, want empty summary payload", diff.StagedPatch)
	}
	if diff.UnstagedPatch != "" {
		t.Fatalf("diff.UnstagedPatch = %q, want empty summary payload", diff.UnstagedPatch)
	}
}

func TestLoadWorktreeDiffNonGitWorkspace(t *testing.T) {
	t.Parallel()

	diff, err := loadWorktreeDiff(1, t.TempDir(), "No git branch")
	if err != nil {
		t.Fatalf("loadWorktreeDiff() error = %v", err)
	}
	if diff.IsGitRepo {
		t.Fatalf("diff.IsGitRepo = true, want false")
	}
	if len(diff.Staged) != 0 || len(diff.Unstaged) != 0 || len(diff.Untracked) != 0 {
		t.Fatalf("unexpected changes: %#v", diff)
	}
}

func TestLoadFileDiffReturnsPatchForSelectedFile(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	root := t.TempDir()
	runGit(t, gitPath, root, "init")
	runGit(t, gitPath, root, "config", "user.email", "helm@example.com")
	runGit(t, gitPath, root, "config", "user.name", "Helm Test")

	tracked := filepath.Join(root, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("one\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(tracked) error = %v", err)
	}
	runGit(t, gitPath, root, "add", "tracked.txt")
	runGit(t, gitPath, root, "commit", "-m", "initial")

	if err := os.WriteFile(tracked, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(modified tracked) error = %v", err)
	}

	fileDiff, err := loadFileDiff(7, root, "tracked.txt", false)
	if err != nil {
		t.Fatalf("loadFileDiff() error = %v", err)
	}
	if !strings.Contains(fileDiff.Patch, "tracked.txt") {
		t.Fatalf("fileDiff.Patch = %q, want tracked.txt", fileDiff.Patch)
	}

	untrackedPath := filepath.Join(root, "new.txt")
	if err := os.WriteFile(untrackedPath, []byte("new file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(untracked) error = %v", err)
	}

	untrackedDiff, err := loadFileDiff(7, root, "new.txt", false)
	if err != nil {
		t.Fatalf("loadFileDiff(untracked) error = %v", err)
	}
	if !strings.Contains(untrackedDiff.Patch, "new.txt") {
		t.Fatalf("untrackedDiff.Patch = %q, want new.txt", untrackedDiff.Patch)
	}
}

func runGit(t *testing.T, gitPath, root string, args ...string) {
	t.Helper()
	cmd := exec.Command(gitPath, append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s error = %v (%s)", strings.Join(args, " "), err, string(out))
	}
}
