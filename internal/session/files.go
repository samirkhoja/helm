package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

func (m *Manager) ListWorktreeFiles(worktreeID int) ([]string, error) {
	m.mu.RLock()
	worktree := m.findWorktreeByIDLocked(worktreeID)
	m.mu.RUnlock()
	if worktree == nil {
		return nil, fmt.Errorf("worktree %d not found", worktreeID)
	}

	return listWorktreeFiles(worktree.RootPath)
}

func (m *Manager) ListWorktreeEntries(worktreeID int, relativeDir string) ([]WorktreeEntry, error) {
	m.mu.RLock()
	worktree := m.findWorktreeByIDLocked(worktreeID)
	m.mu.RUnlock()
	if worktree == nil {
		return nil, fmt.Errorf("worktree %d not found", worktreeID)
	}

	return listWorktreeEntries(worktree.RootPath, relativeDir)
}

func (m *Manager) ReadWorktreeFile(worktreeID int, relativePath string) (WorktreeFile, error) {
	m.mu.RLock()
	worktree := m.findWorktreeByIDLocked(worktreeID)
	m.mu.RUnlock()
	if worktree == nil {
		return WorktreeFile{}, fmt.Errorf("worktree %d not found", worktreeID)
	}

	return readWorktreeFile(worktree.RootPath, relativePath)
}

func (m *Manager) SaveWorktreeFile(worktreeID int, relativePath, content, expectedVersion string) (WorktreeFile, error) {
	m.mu.RLock()
	worktree := m.findWorktreeByIDLocked(worktreeID)
	m.mu.RUnlock()
	if worktree == nil {
		return WorktreeFile{}, fmt.Errorf("worktree %d not found", worktreeID)
	}

	return saveWorktreeFile(worktree.RootPath, relativePath, content, expectedVersion)
}

func listWorktreeEntries(rootPath, relativeDir string) ([]WorktreeEntry, error) {
	dirPath, normalizedDir, err := resolveWorktreePath(rootPath, relativeDir, true)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", displayWorktreePath(normalizedDir))
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	items := make([]WorktreeEntry, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" || name == ".git" {
			continue
		}

		relativePath := name
		if normalizedDir != "." {
			relativePath = filepath.Join(normalizedDir, name)
		}

		kind := "file"
		if entry.IsDir() {
			kind = "directory"
		}

		items = append(items, WorktreeEntry{
			Name:       name,
			Path:       filepath.ToSlash(filepath.Clean(relativePath)),
			Kind:       kind,
			Expandable: entry.IsDir(),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind == "directory"
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return items, nil
}

func listWorktreeFiles(rootPath string) ([]string, error) {
	rootPath, err := normalizeRootPath(rootPath)
	if err != nil {
		return nil, err
	}

	gitPath, err := exec.LookPath("git")
	if err == nil && isGitRepo(gitPath, rootPath) {
		files, err := gitListedFiles(gitPath, rootPath)
		if err == nil {
			return files, nil
		}
	}

	files := []string{}
	err = filepath.WalkDir(rootPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == rootPath {
			return nil
		}

		if entry.Name() == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(filepath.Clean(relativePath)))
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

func gitListedFiles(gitPath, rootPath string) ([]string, error) {
	out, err := exec.Command(
		gitPath,
		"-C",
		rootPath,
		"ls-files",
		"--cached",
		"--others",
		"--exclude-standard",
		"-z",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	raw := strings.TrimRight(string(out), "\x00")
	if raw == "" {
		return []string{}, nil
	}

	files := strings.Split(raw, "\x00")
	sort.Strings(files)
	return files, nil
}

func readWorktreeFile(rootPath, relativePath string) (WorktreeFile, error) {
	filePath, normalizedPath, err := resolveWorktreePath(rootPath, relativePath, false)
	if err != nil {
		return WorktreeFile{}, err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return WorktreeFile{}, err
	}
	if info.IsDir() {
		return WorktreeFile{}, fmt.Errorf("%s is a directory", displayWorktreePath(normalizedPath))
	}
	if !info.Mode().IsRegular() {
		return WorktreeFile{}, fmt.Errorf("%s is not a regular file", displayWorktreePath(normalizedPath))
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return WorktreeFile{}, err
	}
	if isUnsupportedEditableContent(content) {
		return WorktreeFile{}, fmt.Errorf("Only UTF-8 text files can be opened in the file editor.")
	}

	return WorktreeFile{
		Path:         filepath.ToSlash(normalizedPath),
		Content:      string(content),
		VersionToken: fileVersionToken(content),
	}, nil
}

func saveWorktreeFile(rootPath, relativePath, content, expectedVersion string) (WorktreeFile, error) {
	filePath, normalizedPath, err := resolveWorktreePath(rootPath, relativePath, false)
	if err != nil {
		return WorktreeFile{}, err
	}

	expectedVersion = strings.TrimSpace(expectedVersion)
	if expectedVersion == "" {
		return WorktreeFile{}, fmt.Errorf("version token is required")
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return WorktreeFile{}, err
	}
	if info.IsDir() {
		return WorktreeFile{}, fmt.Errorf("%s is a directory", displayWorktreePath(normalizedPath))
	}
	if !info.Mode().IsRegular() {
		return WorktreeFile{}, fmt.Errorf("%s is not a regular file", displayWorktreePath(normalizedPath))
	}
	currentContent, err := os.ReadFile(filePath)
	if err != nil {
		return WorktreeFile{}, err
	}
	if isUnsupportedEditableContent(currentContent) {
		return WorktreeFile{}, fmt.Errorf("Only UTF-8 text files can be opened in the file editor.")
	}

	if fileVersionToken(currentContent) != expectedVersion {
		return WorktreeFile{}, fmt.Errorf("File changed on disk. Reload before saving.")
	}

	nextContent := []byte(content)
	if err := os.WriteFile(filePath, nextContent, info.Mode().Perm()); err != nil {
		return WorktreeFile{}, err
	}

	return WorktreeFile{
		Path:         filepath.ToSlash(normalizedPath),
		Content:      content,
		VersionToken: fileVersionToken(nextContent),
	}, nil
}

func resolveWorktreePath(rootPath, relativePath string, allowRoot bool) (string, string, error) {
	rootPath, err := normalizeRootPath(rootPath)
	if err != nil {
		return "", "", err
	}
	resolvedRootPath, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", "", err
	}

	normalizedPath, err := normalizeWorktreeRelativePath(relativePath, allowRoot)
	if err != nil {
		return "", "", err
	}
	if normalizedPath == "." {
		return resolvedRootPath, normalizedPath, nil
	}

	targetPath := filepath.Join(rootPath, normalizedPath)
	relative, err := filepath.Rel(rootPath, targetPath)
	if err != nil {
		return "", "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path escapes worktree root")
	}

	resolvedTargetPath, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return "", "", err
	}
	if err := ensurePathWithinWorktreeRoot(resolvedRootPath, resolvedTargetPath); err != nil {
		return "", "", err
	}

	return resolvedTargetPath, filepath.Clean(relative), nil
}

func ensurePathWithinWorktreeRoot(rootPath, targetPath string) error {
	relative, err := filepath.Rel(rootPath, targetPath)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes worktree root")
	}
	return nil
}

func normalizeWorktreeRelativePath(path string, allowRoot bool) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		if allowRoot {
			return ".", nil
		}
		return "", fmt.Errorf("file path is required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not supported")
	}

	cleaned := filepath.Clean(path)
	if cleaned == "." {
		if allowRoot {
			return ".", nil
		}
		return "", fmt.Errorf("file path is required")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes worktree root")
	}
	return cleaned, nil
}

func displayWorktreePath(path string) string {
	if path == "" || path == "." {
		return "worktree root"
	}
	return filepath.ToSlash(path)
}

func isUnsupportedEditableContent(content []byte) bool {
	return bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content)
}

func fileVersionToken(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
