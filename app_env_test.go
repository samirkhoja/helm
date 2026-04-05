package main

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeStartupEnvUsesResolvedPATH(t *testing.T) {
	t.Parallel()

	baseEnv := []string{"PATH=/usr/bin:/bin", "HOME=/tmp/home"}
	resolvedEnv, notice := normalizeStartupEnv(baseEnv, func(input []string) ([]string, error) {
		if got := envValue(input, "PATH"); got != "/usr/bin:/bin" {
			t.Fatalf("resolver PATH = %q, want original PATH", got)
		}
		return setEnvValue(input, "PATH", "/Users/test/.local/bin:/usr/bin:/bin"), nil
	})

	if notice != "" {
		t.Fatalf("notice = %q, want empty", notice)
	}
	if got := envValue(resolvedEnv, "PATH"); got != "/Users/test/.local/bin:/usr/bin:/bin" {
		t.Fatalf("resolved PATH = %q, want login-shell PATH", got)
	}
	if got := envValue(resolvedEnv, "HOME"); got != "/tmp/home" {
		t.Fatalf("resolved HOME = %q, want /tmp/home", got)
	}
}

func TestNormalizeStartupEnvFallsBackOnResolverError(t *testing.T) {
	t.Parallel()

	baseEnv := []string{"PATH=/usr/bin:/bin"}
	resolvedEnv, notice := normalizeStartupEnv(baseEnv, func([]string) ([]string, error) {
		return nil, errors.New("shell lookup failed")
	})

	if got := envValue(resolvedEnv, "PATH"); got != "/usr/bin:/bin" {
		t.Fatalf("resolved PATH = %q, want original PATH", got)
	}
	if !strings.Contains(notice, "shell lookup failed") {
		t.Fatalf("notice = %q, want resolver error", notice)
	}
}

func TestExtractShellPathIgnoresSurroundingNoise(t *testing.T) {
	t.Parallel()

	output := strings.Join([]string{
		"theme startup noise",
		loginShellPathStartMarker,
		"/Users/test/.local/bin:/usr/bin:/bin",
		loginShellPathEndMarker,
		"post script noise",
	}, "\n")

	got, err := extractShellPath(output)
	if err != nil {
		t.Fatalf("extractShellPath() error = %v", err)
	}
	if got != "/Users/test/.local/bin:/usr/bin:/bin" {
		t.Fatalf("extractShellPath() = %q, want PATH", got)
	}
}
