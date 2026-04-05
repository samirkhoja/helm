package main

import (
	_ "embed"
	"runtime/debug"
	"strings"
)

const appName = "Helm"

//go:embed VERSION
var trackedAppVersion string

// appVersion can be overridden at build time with:
// -ldflags "-X main.appVersion=vX.Y.Z"
var appVersion string

type AppInfo struct {
	Name    string
	Version string
}

func currentAppInfo() AppInfo {
	return AppInfo{
		Name:    appName,
		Version: resolveAppVersion(),
	}
}

func resolveAppVersion() string {
	version := strings.TrimSpace(appVersion)
	if version != "" {
		return version
	}

	version = strings.TrimSpace(trackedAppVersion)
	if version != "" {
		return version
	}

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return fallbackAppVersion()
	}

	revision := ""
	modified := false
	for _, setting := range buildInfo.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}

	if revision == "" {
		return fallbackAppVersion()
	}

	if len(revision) > 7 {
		revision = revision[:7]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}

func fallbackAppVersion() string {
	return "dev"
}
