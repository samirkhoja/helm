package session

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	noGitBranch   = "No git branch"
	detachedHead  = "Detached HEAD"
	defaultSource = "HEAD"
)

func normalizeWorktreeSourceRef(sourceRef string) string {
	switch strings.TrimSpace(sourceRef) {
	case "", noGitBranch, detachedHead:
		return defaultSource
	default:
		return strings.TrimSpace(sourceRef)
	}
}

type repoDescriptor struct {
	Name         string
	RootPath     string
	GitCommonDir string
	IsGitRepo    bool
	Worktrees    []worktreeDescriptor
}

type worktreeDescriptor struct {
	Name      string
	RootPath  string
	GitBranch string
	IsPrimary bool
}

type repoSelection struct {
	Repo             repoDescriptor
	SelectedWorktree worktreeDescriptor
}

func DescribeWorkspace(root string) (WorkspaceChoice, error) {
	selection, err := describeRepoSelection(root)
	if err != nil {
		return WorkspaceChoice{}, err
	}

	return WorkspaceChoice{
		RootPath:  selection.SelectedWorktree.RootPath,
		Name:      selection.SelectedWorktree.Name,
		GitBranch: selection.SelectedWorktree.GitBranch,
	}, nil
}

func describeRepoSelection(root string) (repoSelection, error) {
	absRoot, err := normalizeRootPath(root)
	if err != nil {
		return repoSelection{}, err
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		return syntheticRepoSelection(absRoot), nil
	}

	selectedWorktreeRoot, err := gitTopLevel(gitPath, absRoot)
	if err != nil {
		return syntheticRepoSelection(absRoot), nil
	}

	gitCommonDir, err := gitPathResolve(gitPath, selectedWorktreeRoot, "--git-common-dir")
	if err != nil {
		return repoSelection{}, err
	}

	worktrees, err := listGitWorktrees(gitPath, selectedWorktreeRoot, gitCommonDir)
	if err != nil || len(worktrees) == 0 {
		worktrees = []worktreeDescriptor{describeSingleWorktree(gitPath, selectedWorktreeRoot, gitCommonDir)}
	}

	selected := worktrees[0]
	for _, worktree := range worktrees {
		if filepath.Clean(worktree.RootPath) == filepath.Clean(selectedWorktreeRoot) {
			selected = worktree
			break
		}
	}

	repoRoot := selectedWorktreeRoot
	for _, worktree := range worktrees {
		if worktree.IsPrimary {
			repoRoot = worktree.RootPath
			break
		}
	}

	return repoSelection{
		Repo: repoDescriptor{
			Name:         pathLabel(repoRoot),
			RootPath:     repoRoot,
			GitCommonDir: gitCommonDir,
			IsGitRepo:    true,
			Worktrees:    worktrees,
		},
		SelectedWorktree: selected,
	}, nil
}

func syntheticRepoSelection(root string) repoSelection {
	worktree := worktreeDescriptor{
		Name:      pathLabel(root),
		RootPath:  root,
		GitBranch: noGitBranch,
		IsPrimary: true,
	}
	return repoSelection{
		Repo: repoDescriptor{
			Name:      worktree.Name,
			RootPath:  root,
			IsGitRepo: false,
			Worktrees: []worktreeDescriptor{worktree},
		},
		SelectedWorktree: worktree,
	}
}

func detectGitBranch(root string) string {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return noGitBranch
	}
	return detectGitBranchWithPath(gitPath, root)
}

func detectGitBranchWithPath(gitPath, root string) string {
	out, err := exec.Command(gitPath, "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return noGitBranch
	}

	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return detachedHead
	}
	return branch
}

func listGitWorktrees(gitPath, root, gitCommonDir string) ([]worktreeDescriptor, error) {
	out, err := exec.Command(gitPath, "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, err
	}

	blocks := strings.Split(strings.TrimSpace(string(out)), "\n\n")
	items := make([]worktreeDescriptor, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "worktree ") {
				continue
			}
			worktreeRoot := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			items = append(items, describeGitWorktreeBlock(block, worktreeRoot, gitCommonDir))
			break
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsPrimary != items[j].IsPrimary {
			return items[i].IsPrimary
		}
		return items[i].RootPath < items[j].RootPath
	})

	return items, nil
}

func describeGitWorktreeBlock(block, root, gitCommonDir string) worktreeDescriptor {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	branch := noGitBranch
	detached := false
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "branch "):
			branch = parseGitWorktreeBranch(strings.TrimSpace(strings.TrimPrefix(line, "branch ")))
		case line == "detached":
			detached = true
		}
	}
	if detached {
		branch = detachedHead
	}

	return worktreeDescriptor{
		Name:      pathLabel(absRoot),
		RootPath:  absRoot,
		GitBranch: branch,
		IsPrimary: filepath.Clean(filepath.Join(absRoot, ".git")) == filepath.Clean(gitCommonDir),
	}
}

func describeSingleWorktree(gitPath, root, gitCommonDir string) worktreeDescriptor {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	return worktreeDescriptor{
		Name:      pathLabel(absRoot),
		RootPath:  absRoot,
		GitBranch: detectGitBranchWithPath(gitPath, absRoot),
		IsPrimary: filepath.Clean(filepath.Join(absRoot, ".git")) == filepath.Clean(gitCommonDir),
	}
}

func parseGitWorktreeBranch(value string) string {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return noGitBranch
	case value == "(detached)" || value == "detached":
		return detachedHead
	case strings.HasPrefix(value, "refs/heads/"):
		return strings.TrimPrefix(value, "refs/heads/")
	default:
		return value
	}
}

func gitTopLevel(gitPath, root string) (string, error) {
	return gitPathResolve(gitPath, root, "--show-toplevel")
}

func gitPathResolve(gitPath, root string, arg string) (string, error) {
	out, err := exec.Command(gitPath, "-C", root, "rev-parse", arg).Output()
	if err != nil {
		return "", err
	}

	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("git rev-parse %s returned empty output", arg)
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Clean(filepath.Join(root, value)), nil
}

func SuggestWorktreePath(repoRoot, branchName string) string {
	parent := filepath.Dir(repoRoot)
	repoName := pathLabel(repoRoot)
	suffix := sanitizeBranchName(branchName)
	if suffix == "" {
		suffix = "worktree"
	}
	return filepath.Join(parent, fmt.Sprintf("%s-%s", repoName, suffix))
}

func CreateWorktree(repoRoot string, request WorktreeCreateRequest) (string, error) {
	repoRoot, err := normalizeRootPath(repoRoot)
	if err != nil {
		return "", err
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git is not installed")
	}
	if !isGitRepo(gitPath, repoRoot) {
		return "", fmt.Errorf("worktrees are only available for git repositories")
	}

	branchName := strings.TrimSpace(request.BranchName)
	if branchName == "" {
		return "", fmt.Errorf("branch name is required")
	}

	targetPath := strings.TrimSpace(request.Path)
	if targetPath == "" {
		targetPath = SuggestWorktreePath(repoRoot, branchName)
	}
	targetPath, err = filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve worktree path: %w", err)
	}

	args := []string{"-C", repoRoot, "worktree", "add"}
	switch strings.TrimSpace(request.Mode) {
	case "", WorktreeModeNewBranch:
		sourceRef := normalizeWorktreeSourceRef(request.SourceRef)
		args = append(args, "-b", branchName, targetPath, sourceRef)
	case WorktreeModeExistingBranch:
		args = append(args, targetPath, branchName)
	default:
		return "", fmt.Errorf("unknown worktree mode %q", request.Mode)
	}

	cmd := exec.Command(gitPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message != "" {
			return "", fmt.Errorf("%s", message)
		}
		return "", fmt.Errorf("git worktree add: %w", err)
	}

	return targetPath, nil
}

func pathLabel(root string) string {
	name := filepath.Base(root)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return root
	}
	return name
}

func sanitizeBranchName(branchName string) string {
	var builder strings.Builder
	lastDash := false

	for _, r := range branchName {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '.' || r == '_':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(builder.String(), "-")
}
