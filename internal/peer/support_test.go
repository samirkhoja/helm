package peer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSupportManagerEnsureCLIWrapperPreservesPeersCommandLine(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := &SupportManager{
		root:           root,
		executablePath: "/tmp/helm-app",
		projDone:       map[string]struct{}{},
	}

	if err := manager.ensureCLIWrapper(); err != nil {
		t.Fatalf("ensureCLIWrapper() error = %v", err)
	}

	var path string
	if runtime.GOOS == "windows" {
		path = filepath.Join(root, "bin", "helm.cmd")
	} else {
		path = filepath.Join(root, "bin", "helm")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	text := string(content)
	if strings.Contains(text, " peers ") {
		t.Fatalf("wrapper content = %q, want no extra peers prefix", text)
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(text, "\"/tmp/helm-app\" %*") {
			t.Fatalf("wrapper content = %q, want direct passthrough", text)
		}
		return
	}
	if !strings.Contains(text, "exec \"/tmp/helm-app\" \"$@\"") {
		t.Fatalf("wrapper content = %q, want direct passthrough", text)
	}
}

func TestSupportManagerPrepareLaunchForCodexProjectsOnlyToCodexHome(t *testing.T) {
	processHome := t.TempDir()
	t.Setenv("HOME", processHome)
	t.Setenv("CODEX_HOME", filepath.Join(processHome, ".process-codex-home"))

	root := t.TempDir()
	manager := &SupportManager{
		root:           root,
		executablePath: "/tmp/helm-app",
		projDone:       map[string]struct{}{},
	}

	launchCodexHome := filepath.Join(t.TempDir(), ".launch-codex-home")
	launchEnv := []string{
		"PATH=/usr/bin",
		"HOME=" + t.TempDir(),
		"CODEX_HOME=" + launchCodexHome,
	}

	if _, err := manager.PrepareLaunch(FamilyCodex, launchEnv); err != nil {
		t.Fatalf("PrepareLaunch() error = %v", err)
	}

	target, err := codexSkillTarget(launchEnv)
	if err != nil {
		t.Fatalf("codexSkillTarget() error = %v", err)
	}
	skillPath := filepath.Join(target, "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("Stat(%s) error = %v", skillPath, err)
	}
	agentYAML := filepath.Join(target, "agents", "openai.yaml")
	if _, err := os.Stat(agentYAML); err != nil {
		t.Fatalf("Stat(%s) error = %v", agentYAML, err)
	}

	processTarget, err := codexSkillTarget([]string{
		"HOME=" + processHome,
		"CODEX_HOME=" + filepath.Join(processHome, ".process-codex-home"),
	})
	if err != nil {
		t.Fatalf("codexSkillTarget(processEnv) error = %v", err)
	}
	if _, err := os.Stat(processTarget); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want not exists", processTarget, err)
	}

	legacyTarget := filepath.Join(processHome, ".agents", "skills", skillName)
	if _, err := os.Stat(legacyTarget); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want not exists", legacyTarget, err)
	}
}

func TestSupportManagerPrepareLaunchForCodexProjectsTracksDifferentTargets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	manager := &SupportManager{
		root:           root,
		executablePath: "/tmp/helm-app",
		projDone:       map[string]struct{}{},
	}

	firstEnv := []string{
		"PATH=/usr/bin",
		"HOME=" + t.TempDir(),
		"CODEX_HOME=" + filepath.Join(t.TempDir(), ".codex-home-a"),
	}
	secondEnv := []string{
		"PATH=/usr/bin",
		"HOME=" + t.TempDir(),
		"CODEX_HOME=" + filepath.Join(t.TempDir(), ".codex-home-b"),
	}

	if _, err := manager.PrepareLaunch(FamilyCodex, firstEnv); err != nil {
		t.Fatalf("PrepareLaunch(firstEnv) error = %v", err)
	}
	if _, err := manager.PrepareLaunch(FamilyCodex, secondEnv); err != nil {
		t.Fatalf("PrepareLaunch(secondEnv) error = %v", err)
	}

	for _, env := range [][]string{firstEnv, secondEnv} {
		target, err := codexSkillTarget(env)
		if err != nil {
			t.Fatalf("codexSkillTarget() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(target, "SKILL.md")); err != nil {
			t.Fatalf("Stat(%s) error = %v", filepath.Join(target, "SKILL.md"), err)
		}
	}
}
