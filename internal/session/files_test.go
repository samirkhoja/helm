package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestListWorktreeEntriesSortsAndHidesGitDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("Mkdir(.git) error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "zeta"), 0o755); err != nil {
		t.Fatalf("Mkdir(zeta) error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "alpha"), 0o755); err != nil {
		t.Fatalf("Mkdir(alpha) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta.txt"), []byte("beta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(beta.txt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "aardvark.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(aardvark.txt) error = %v", err)
	}

	entries, err := listWorktreeEntries(root, "")
	if err != nil {
		t.Fatalf("listWorktreeEntries() error = %v", err)
	}

	if len(entries) != 4 {
		t.Fatalf("len(entries) = %d, want 4", len(entries))
	}

	gotNames := []string{entries[0].Name, entries[1].Name, entries[2].Name, entries[3].Name}
	wantNames := []string{"alpha", "zeta", "aardvark.txt", "beta.txt"}
	for index := range wantNames {
		if gotNames[index] != wantNames[index] {
			t.Fatalf("entries[%d].Name = %q, want %q", index, gotNames[index], wantNames[index])
		}
	}

	if entries[0].Kind != "directory" || !entries[0].Expandable {
		t.Fatalf("entries[0] = %#v, want expandable directory", entries[0])
	}
	if entries[2].Kind != "file" || entries[2].Expandable {
		t.Fatalf("entries[2] = %#v, want non-expandable file", entries[2])
	}
}

func TestListWorktreeEntriesRejectsTraversal(t *testing.T) {
	t.Parallel()

	_, err := listWorktreeEntries(t.TempDir(), "../outside")
	if err == nil {
		t.Fatalf("listWorktreeEntries() error = nil, want traversal rejection")
	}
	if !strings.Contains(err.Error(), "escapes worktree root") {
		t.Fatalf("err = %v, want traversal rejection", err)
	}
}

func TestListWorktreeFilesRecursesAndSkipsGitMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll(src) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git", "objects"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git/objects) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(src/main.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[core]\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.git/config) error = %v", err)
	}

	files, err := listWorktreeFiles(root)
	if err != nil {
		t.Fatalf("listWorktreeFiles() error = %v", err)
	}

	want := []string{"README.md", "src/main.go"}
	if len(files) != len(want) {
		t.Fatalf("len(files) = %d, want %d (%#v)", len(files), len(want), files)
	}
	for index := range want {
		if files[index] != want[index] {
			t.Fatalf("files[%d] = %q, want %q", index, files[index], want[index])
		}
	}
}

func TestListWorktreeFilesUsesGitWhenAvailable(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	root := newGitRepo(t, gitPath)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("dist/\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.gitignore) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("tracked\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(tracked.txt) error = %v", err)
	}
	runGit(t, gitPath, root, "add", ".gitignore", "tracked.txt")
	runGit(t, gitPath, root, "commit", "-m", "track files")

	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll(src) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(src/main.go) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatalf("MkdirAll(dist) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "bundle.js"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(dist/bundle.js) error = %v", err)
	}

	files, err := listWorktreeFiles(root)
	if err != nil {
		t.Fatalf("listWorktreeFiles() error = %v", err)
	}

	want := []string{".gitignore", "README.md", "src/main.go", "tracked.txt"}
	if len(files) != len(want) {
		t.Fatalf("len(files) = %d, want %d (%#v)", len(files), len(want), files)
	}
	for index := range want {
		if files[index] != want[index] {
			t.Fatalf("files[%d] = %q, want %q", index, files[index], want[index])
		}
	}
}

func TestReadWorktreeFileRejectsBinaryContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "image.bin"), []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("WriteFile(image.bin) error = %v", err)
	}

	_, err := readWorktreeFile(root, "image.bin")
	if err == nil {
		t.Fatalf("readWorktreeFile() error = nil, want binary rejection")
	}
	if !strings.Contains(err.Error(), "UTF-8 text files") {
		t.Fatalf("err = %v, want UTF-8 rejection", err)
	}
}

func TestReadWorktreeFileAllowsLargeTextFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	content := strings.Repeat("a", 256*1024+1)
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(large.txt) error = %v", err)
	}

	file, err := readWorktreeFile(root, "large.txt")
	if err != nil {
		t.Fatalf("readWorktreeFile() error = %v", err)
	}
	if len(file.Content) != len(content) {
		t.Fatalf("len(file.Content) = %d, want %d", len(file.Content), len(content))
	}
}

func TestReadWorktreeFileRejectsSymlinkOutsideWorktree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outsideRoot := t.TempDir()
	outsidePath := filepath.Join(outsideRoot, "secret.txt")
	if err := os.WriteFile(outsidePath, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(secret.txt) error = %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(root, "linked.txt")); err != nil {
		t.Skipf("Symlink(linked.txt) unsupported: %v", err)
	}

	_, err := readWorktreeFile(root, "linked.txt")
	if err == nil {
		t.Fatalf("readWorktreeFile() error = nil, want symlink escape rejection")
	}
	if !strings.Contains(err.Error(), "escapes worktree root") {
		t.Fatalf("err = %v, want symlink escape rejection", err)
	}
}

func TestSaveWorktreeFileRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("one\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(note.txt) error = %v", err)
	}

	loaded, err := readWorktreeFile(root, "note.txt")
	if err != nil {
		t.Fatalf("readWorktreeFile() error = %v", err)
	}

	saved, err := saveWorktreeFile(root, "note.txt", "one\ntwo\n", loaded.VersionToken)
	if err != nil {
		t.Fatalf("saveWorktreeFile() error = %v", err)
	}

	if saved.Content != "one\ntwo\n" {
		t.Fatalf("saved.Content = %q, want updated content", saved.Content)
	}
	if saved.VersionToken == loaded.VersionToken {
		t.Fatalf("saved.VersionToken = %q, want updated token", saved.VersionToken)
	}

	diskContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(note.txt) error = %v", err)
	}
	if string(diskContent) != "one\ntwo\n" {
		t.Fatalf("disk content = %q, want updated content", string(diskContent))
	}
}

func TestSaveWorktreeFileRejectsConflict(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("one\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(note.txt) error = %v", err)
	}

	loaded, err := readWorktreeFile(root, "note.txt")
	if err != nil {
		t.Fatalf("readWorktreeFile() error = %v", err)
	}

	if err := os.WriteFile(path, []byte("external change\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(note.txt external change) error = %v", err)
	}

	_, err = saveWorktreeFile(root, "note.txt", "my change\n", loaded.VersionToken)
	if err == nil {
		t.Fatalf("saveWorktreeFile() error = nil, want conflict")
	}
	if !strings.Contains(err.Error(), "Reload before saving") {
		t.Fatalf("err = %v, want conflict rejection", err)
	}
}

func TestSaveWorktreeFileRejectsSymlinkOutsideWorktree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outsideRoot := t.TempDir()
	outsidePath := filepath.Join(outsideRoot, "secret.txt")
	if err := os.WriteFile(outsidePath, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(secret.txt) error = %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(root, "linked.txt")); err != nil {
		t.Skipf("Symlink(linked.txt) unsupported: %v", err)
	}

	_, err := saveWorktreeFile(root, "linked.txt", "updated\n", fileVersionToken([]byte("secret\n")))
	if err == nil {
		t.Fatalf("saveWorktreeFile() error = nil, want symlink escape rejection")
	}
	if !strings.Contains(err.Error(), "escapes worktree root") {
		t.Fatalf("err = %v, want symlink escape rejection", err)
	}

	content, readErr := os.ReadFile(outsidePath)
	if readErr != nil {
		t.Fatalf("ReadFile(secret.txt) error = %v", readErr)
	}
	if string(content) != "secret\n" {
		t.Fatalf("outside file content = %q, want unchanged content", string(content))
	}
}
