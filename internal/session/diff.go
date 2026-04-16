package session

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func (m *Manager) WorktreeDiff(worktreeID int) (WorktreeDiff, error) {
	m.mu.RLock()
	worktree := m.findWorktreeByIDLocked(worktreeID)
	m.mu.RUnlock()
	if worktree == nil {
		return WorktreeDiff{}, fmt.Errorf("worktree %d not found", worktreeID)
	}

	return loadWorktreeDiff(worktree.ID, worktree.RootPath, worktree.GitBranch)
}

func (m *Manager) FileDiff(worktreeID int, path string, staged bool) (FileDiff, error) {
	m.mu.RLock()
	worktree := m.findWorktreeByIDLocked(worktreeID)
	m.mu.RUnlock()
	if worktree == nil {
		return FileDiff{}, fmt.Errorf("worktree %d not found", worktreeID)
	}

	return loadFileDiff(worktree.ID, worktree.RootPath, path, staged)
}

func loadWorktreeDiff(worktreeID int, rootPath, branch string) (WorktreeDiff, error) {
	diff := WorktreeDiff{
		WorktreeID: worktreeID,
		RootPath:   rootPath,
		GitBranch:  branch,
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		return diff, nil
	}

	if !isGitRepo(gitPath, rootPath) {
		return diff, nil
	}

	diff.IsGitRepo = true
	diff.Staged = gitNumstat(gitPath, rootPath, "--cached")
	diff.Unstaged = gitNumstat(gitPath, rootPath)
	diff.Untracked = gitUntracked(gitPath, rootPath)

	return diff, nil
}

func isGitRepo(gitPath, rootPath string) bool {
	cmd := exec.Command(gitPath, "-C", rootPath, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func gitNumstat(gitPath, rootPath string, extraArgs ...string) []GitFileChange {
	args := []string{"-C", rootPath, "diff", "--numstat", "--find-renames"}
	args = append(args[:3], append(extraArgs, args[3:]...)...)
	cmd := exec.Command(gitPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	items := make([]GitFileChange, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		items = append(items, GitFileChange{
			Path:    filepath.Clean(parts[2]),
			Status:  "modified",
			Added:   parseDiffCount(parts[0]),
			Removed: parseDiffCount(parts[1]),
		})
	}
	return items
}

func parseDiffCount(value string) int {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return 0
	}
	count, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return count
}

func gitUntracked(gitPath, rootPath string) []string {
	cmd := exec.Command(gitPath, "-C", rootPath, "ls-files", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		items = append(items, filepath.Clean(line))
	}
	return items
}

func loadFileDiff(worktreeID int, rootPath, path string, staged bool) (FileDiff, error) {
	normalizedPath, err := normalizeWorktreeRelativePath(path, false)
	if err != nil {
		return FileDiff{}, err
	}

	diff := FileDiff{
		WorktreeID: worktreeID,
		Path:       filepath.ToSlash(normalizedPath),
		Staged:     staged,
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		diff.Message = "git is not installed"
		return diff, nil
	}
	if !isGitRepo(gitPath, rootPath) {
		diff.Message = "workspace is not a git repository"
		return diff, nil
	}

	if staged {
		diff.Patch = gitFilePatch(gitPath, rootPath, normalizedPath, true)
	} else {
		diff.Patch = gitFilePatch(gitPath, rootPath, normalizedPath, false)
		if diff.Patch == "" {
			resolvedPath, _, rErr := resolveWorktreePath(rootPath, normalizedPath, false)
			if rErr == nil {
				diff.Patch = gitUntrackedFilePatch(gitPath, rootPath, resolvedPath)
			}
		}
	}

	if diff.Patch == "" {
		diff.Message = "No diff available for this file."
	}
	return diff, nil
}

func gitFilePatch(gitPath, rootPath, path string, staged bool) string {
	args := []string{"-C", rootPath, "diff", "--no-color", "--patch", "--find-renames"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	cmd := exec.Command(gitPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(bytes.TrimSpace(out)), "\n")
}

func gitUntrackedFilePatch(gitPath, rootPath, resolvedPath string) string {
	cmd := exec.Command(gitPath, "-C", rootPath, "diff", "--no-index", "--no-color", "--patch", "--", "/dev/null", resolvedPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return strings.TrimRight(string(bytes.TrimSpace(out)), "\n")
		}
		return ""
	}
	return strings.TrimRight(string(bytes.TrimSpace(out)), "\n")
}
