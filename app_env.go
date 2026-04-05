package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	loginShellPathStartMarker = "__HELM_PATH_START__"
	loginShellPathEndMarker   = "__HELM_PATH_END__"
)

type loginShellEnvResolver func(baseEnv []string) ([]string, error)

func inheritedEnvironment() []string {
	return append([]string(nil), os.Environ()...)
}

func normalizeStartupEnv(baseEnv []string, resolver loginShellEnvResolver) ([]string, string) {
	env := append([]string(nil), baseEnv...)
	if resolver == nil {
		return env, ""
	}

	resolvedEnv, err := resolver(env)
	if err != nil {
		return env, fmt.Sprintf("Unable to load login-shell PATH; using app PATH instead: %v", err)
	}
	return resolvedEnv, ""
}

func resolveLoginShellEnv(baseEnv []string) ([]string, error) {
	shell := envValue(baseEnv, "SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-ilc", loginShellPathScript())
	cmd.Env = append([]string(nil), baseEnv...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("timed out resolving PATH with %s", filepath.Base(shell))
	}
	if err != nil {
		return nil, fmt.Errorf("resolve PATH with %s: %w", filepath.Base(shell), err)
	}

	pathValue, err := extractShellPath(string(output))
	if err != nil {
		return nil, fmt.Errorf("parse PATH from %s: %w", filepath.Base(shell), err)
	}
	if pathValue == "" {
		return nil, fmt.Errorf("resolved empty PATH from %s", filepath.Base(shell))
	}

	return setEnvValue(baseEnv, "PATH", pathValue), nil
}

func loginShellPathScript() string {
	return fmt.Sprintf(
		"command printf '%%s\\n' '%s'; command printf '%%s\\n' \"$PATH\"; command printf '%%s\\n' '%s'",
		loginShellPathStartMarker,
		loginShellPathEndMarker,
	)
}

func extractShellPath(output string) (string, error) {
	start := strings.Index(output, loginShellPathStartMarker)
	if start < 0 {
		return "", fmt.Errorf("missing PATH start marker")
	}
	rest := output[start+len(loginShellPathStartMarker):]
	end := strings.Index(rest, loginShellPathEndMarker)
	if end < 0 {
		return "", fmt.Errorf("missing PATH end marker")
	}

	value := strings.TrimSpace(rest[:end])
	value = strings.Trim(value, "\r\n")
	return value, nil
}

func setEnvValue(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, item := range env {
		currentKey, _, ok := strings.Cut(item, "=")
		if ok && currentKey == key {
			out = append(out, key+"="+value)
			replaced = true
			continue
		}
		out = append(out, item)
	}
	if !replaced {
		out = append(out, key+"="+value)
	}
	return out
}

func envValue(env []string, key string) string {
	for _, item := range env {
		currentKey, currentValue, ok := strings.Cut(item, "=")
		if ok && currentKey == key {
			return currentValue
		}
	}
	return ""
}
