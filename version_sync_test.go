package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func TestTrackedVersionIsSynchronized(t *testing.T) {
	trackedVersion := strings.TrimSpace(trackedAppVersion)
	if trackedVersion == "" {
		t.Fatal("tracked version is empty")
	}
	if !semverPattern.MatchString(trackedVersion) {
		t.Fatalf("tracked version %q is not semantic versioning", trackedVersion)
	}

	if got := currentAppInfo().Version; got != trackedVersion {
		t.Fatalf("currentAppInfo().Version = %q, want %q", got, trackedVersion)
	}

	var wailsConfig struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	readJSONFile(t, "wails.json", &wailsConfig)
	if wailsConfig.Info.ProductVersion != trackedVersion {
		t.Fatalf("wails.json productVersion = %q, want %q", wailsConfig.Info.ProductVersion, trackedVersion)
	}

	var frontendPackage struct {
		Version string `json:"version"`
	}
	readJSONFile(t, "frontend/package.json", &frontendPackage)
	if frontendPackage.Version != trackedVersion {
		t.Fatalf("frontend/package.json version = %q, want %q", frontendPackage.Version, trackedVersion)
	}

	var packageLock struct {
		Version  string `json:"version"`
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	readJSONFile(t, "frontend/package-lock.json", &packageLock)
	if packageLock.Version != trackedVersion {
		t.Fatalf("frontend/package-lock.json version = %q, want %q", packageLock.Version, trackedVersion)
	}
	if rootPackage, ok := packageLock.Packages[""]; !ok || rootPackage.Version != trackedVersion {
		got := ""
		if ok {
			got = rootPackage.Version
		}
		t.Fatalf("frontend/package-lock.json root package version = %q, want %q", got, trackedVersion)
	}
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}
