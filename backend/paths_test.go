package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveAppPathsKeepsLegacyLayout(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	if err := os.Mkdir(filepath.Join(temp, "vault"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "vault", "Existing.html"), []byte("legacy note"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(temp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })

	got, err := resolveAppPaths("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.dataDir != temp || got.vaultDir != filepath.Join(temp, "vault") || got.dbPath != filepath.Join(temp, "index.db") {
		t.Fatalf("legacy paths not preserved: %#v", got)
	}
}

func TestResolveAppPathsIgnoresShippedWelcomeOnlyVault(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	if err := os.Mkdir(filepath.Join(temp, "vault"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "vault", "welcome.html"), []byte("seed"), 0644); err != nil {
		t.Fatal(err)
	}
	dataHome := filepath.Join(temp, "xdg-data")
	if err := os.Mkdir(dataHome, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", dataHome)
	if err := os.Chdir(temp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })

	got, err := resolveAppPaths("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	want, err := defaultDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if got.dataDir != want || got.vaultDir != filepath.Join(want, "vault") || got.dbPath != filepath.Join(want, "index.db") {
		t.Fatalf("fresh install should use OS data directory: %#v", got)
	}
}

func TestResolveAppPathsExplicitDataDirectory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "sefirot-data")
	got, err := resolveAppPaths(base, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.dataDir != base || got.vaultDir != filepath.Join(base, "vault") || got.dbPath != filepath.Join(base, "index.db") {
		t.Fatalf("unexpected explicit paths: %#v", got)
	}
}

func TestDefaultDataDirUsesPlatformConvention(t *testing.T) {
	dir, err := defaultDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("data directory must be absolute: %q", dir)
	}
	switch runtime.GOOS {
	case "windows":
		if filepath.Base(dir) != "SEFIROT" {
			t.Fatalf("unexpected Windows data directory: %q", dir)
		}
	case "darwin":
		if filepath.Base(dir) != "SEFIROT" {
			t.Fatalf("unexpected macOS data directory: %q", dir)
		}
	default:
		if filepath.Base(dir) != "sefirot" {
			t.Fatalf("unexpected XDG data directory: %q", dir)
		}
	}
}
